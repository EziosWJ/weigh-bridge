// agent-printer connects the SK-820 host and optional D12 scale to the existing
// Agent protocol. One process owns the single scale identity and WSS connection;
// printing and serial reading must not compete for the same weighing point.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentcamera"
	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/agentscale"
	"golang.org/x/net/websocket"
)

func main() {
	cfg, state, err := printerOptions(os.Args[1:], os.Getenv)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		slog.Error("invalid printer configuration", "error", err)
		os.Exit(1)
	}
	digest := agentprinter.ConfigDigest(cfg)
	identityDir, identityErr := agentprinter.IdentityDirectory(state, cfg.ServerURL, cfg.AgentID, cfg.ScaleID)
	if identityErr != nil {
		slog.Error("attempt identity initialization failed", "error", identityErr)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dir := filepath.Dir(state)
	environment := environmentArgument(os.Args[1:])
	if environment == "" {
		environment = filepath.Join(dir, "agent.env")
	}
	runtime := &deviceRuntime{parent: ctx, dir: dir, environment: environment, cfg: cfg}
	maintenance, err := agentmaintenance.OpenDevice(dir, environment, cfg, deviceRunner{runtime: runtime})
	if err != nil {
		slog.Error("open Agent configuration", "error", err)
		return
	}
	runtime.maintenance = maintenance
	maintenance.SetCameraTester(runtime.testCameras)
	if err = runtime.start(cfg); err != nil {
		slog.Error("start devices", "error", err)
		return
	}
	defer runtime.stop()
	for ctx.Err() == nil {
		statusPath := filepath.Join(filepath.Dir(state), "maintenance-status.json")
		pausePath := filepath.Join(filepath.Dir(state), "maintenance.pause")
		current, scale, _ := runtime.snapshot()
		if err := runManaged(ctx, current, scale, identityDir, statusPath, pausePath, runtime); errors.Is(err, p.ErrKicked) {
			writeAgentStatus(statusPath, false, digest)
			slog.Info("Agent 已被后台踢出，停止运行")
			return
		} else if err != nil {
			writeAgentStatus(statusPath, false, digest)
			slog.Warn("connection ended", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func run(parent context.Context, cfg agentprinter.Config, scale *agentscale.Monitor, state, statusPath, pausePath string) error {
	return runManaged(parent, cfg, scale, state, statusPath, pausePath, nil)
}
func runManaged(parent context.Context, cfg agentprinter.Config, scale *agentscale.Monitor, state, statusPath, pausePath string, control *deviceRuntime) (resultErr error) {
	ctx, cancelRun := context.WithCancelCause(parent)
	defer cancelRun(nil)
	defer func() {
		if errors.Is(context.Cause(ctx), p.ErrKicked) {
			resultErr = p.ErrKicked
		}
	}()
	server, queue := cfg.ServerURL, cfg.Queue
	scaleID, agentID := cfg.ScaleID, cfg.AgentID
	digest := agentprinter.ConfigDigest(cfg)
	u, err := url.Parse(server)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return fmt.Errorf("expected HTTP(S) Server URL")
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/api/agent/ws"
	wsConfig, err := websocket.NewConfig(u.String(), server)
	if err != nil {
		return err
	}
	// Optional legacy diagnostics only. A keyless Agent sends no identity
	// headers at all; the Server resolves the local site in agent.hello.ack.
	if cfg.AgentKey != "" {
		wsConfig.Header.Set("Authorization", "Bearer "+cfg.AgentKey)
	}
	if cfg.AgentID != "" {
		wsConfig.Header.Set("X-Agent-ID", cfg.AgentID)
	}
	if cfg.ScaleID > 0 {
		wsConfig.Header.Set("X-Scale-ID", strconv.FormatInt(cfg.ScaleID, 10))
	}
	dial, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := wsConfig.DialContext(dial)
	cancel()
	if err != nil {
		return err
	}
	defer conn.Close()
	ack, err := helloHandshake(conn)
	if err != nil {
		return err
	}
	if ack.ScaleID > 0 {
		scaleID = ack.ScaleID
		cfg.ScaleID = ack.ScaleID
	}
	if ack.AgentID != "" {
		agentID = ack.AgentID
		cfg.AgentID = ack.AgentID
	}
	if !containsString(ack.Capabilities, p.CapabilityMeasurementWindow) {
		slog.Warn("Server 未接受测量窗口能力，正式采集将不可用", "scale", scaleID)
	}
	if monitor, err := activeMonitor(control, scale); err == nil {
		monitor.InvalidateMeasurements("connection_changed")
		defer monitor.InvalidateMeasurements("connection_changed")
	}
	writeAgentStatus(statusPath, true, digest)
	defer writeAgentStatus(statusPath, false, digest)
	conn.MaxPayloadBytes = 128 << 10
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	slog.Info("printer Agent connected", "queue", queue, "scale", scaleID, "agent", agentID, "serial", cfg.SerialPort)
	var writes sync.Mutex
	sendMessage := func(message p.Message) error {
		writes.Lock()
		defer writes.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return websocket.JSON.Send(conn, message)
	}
	sendVersion := func(version int, kind string, payload any) error {
		return sendMessage(p.NewVersion(version, kind, fmt.Sprintf("printer-%d", time.Now().UnixNano()), "", payload))
	}
	send := func(kind string, payload any) error {
		return sendVersion(p.LegacyProtocolVersion, kind, payload)
	}
	sendRequest := func(version int, kind, request string, payload any) error {
		return sendMessage(p.NewVersion(version, kind, fmt.Sprintf("printer-%d", time.Now().UnixNano()), request, payload))
	}
	sendMaintenance := func() error {
		if control == nil {
			return nil
		}
		state := control.report()
		state.Config.ScaleID = scaleID
		state.Config.AgentID = agentID
		return send("maintenance.state", state)
	}
	// Effective configuration and the last success backup are unsolicited
	// reports. Server persists them; they never make the candidate an applied
	// fact until the device reload was verified locally.
	sendConfigState := func() error {
		if control == nil {
			return nil
		}
		return sendVersion(p.ProtocolVersion, p.TypeAgentConfigState, control.maintenance.EffectiveConfig())
	}
	sendConfigBackup := func() error {
		if control == nil {
			return nil
		}
		backup, ok := control.maintenance.ConfigBackup()
		if !ok {
			return nil
		}
		return sendVersion(p.ProtocolVersion, p.TypeAgentConfigBackup, backup)
	}
	sendInvalidations := func(monitor *agentscale.Monitor) {
		for _, invalidation := range monitor.InvalidatedMeasurements(time.Now()) {
			err := sendVersion(p.ProtocolVersion, p.TypeMeasurementInvalidated, p.MeasurementInvalidated{
				Generation: invalidation.Generation,
				Reason:     invalidation.Reason,
				State:      measuredState(invalidation.Snapshot),
				At:         invalidation.At.UTC(),
			})
			if err != nil {
				cancelRun(err)
				return
			}
		}
	}
	sendState := func() error {
		if control != nil {
			return send("device.state", control.deviceState())
		}
		return send("device.state", currentDeviceState(scale, time.Now()))
	}
	if err = sendState(); err != nil {
		return err
	}
	if control != nil {
		if err = sendMaintenance(); err != nil {
			return err
		}
		if err = sendConfigState(); err != nil {
			return err
		}
		if err = sendConfigBackup(); err != nil {
			return err
		}
	}
	incoming := make(chan p.Message, 32)
	readErr := make(chan error, 1)
	go func() {
		for {
			_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
			var m p.Message
			if err := websocket.JSON.Receive(conn, &m); err != nil {
				readErr <- err
				return
			}
			if m.Type == "agent.kick" {
				_ = sendMessage(p.New("agent.kicked", fmt.Sprintf("kick-%d", time.Now().UnixNano()), m.RequestID, struct{}{}))
				cancelRun(p.ErrKicked)
				_ = conn.Close()
				return
			}
			select {
			case incoming <- m:
			case <-done:
				return
			}
		}
	}()

	// Printing is serialized in its own worker. A long render/lp call or a
	// maintenance pause must not stop serial sampling, device.state or kick.
	prints := make(chan agentprinter.Request, 16)
	managedPrints := make(chan devicePrint, 16)
	if control == nil {
		go printWorker(ctx, cfg, state, queue, pausePath, prints)
	} else {
		go control.printWorker(ctx, state, pausePath, managedPrints, func(request agentprinter.Request) {
			_ = send("print.result", p.PrintResult{AttemptID: request.AttemptID, Status: "failed", Reason: "设备执行已停止，未提交打印"})
		})
	}

	captureBusy := make(chan struct{}, 1)

	stateTicker := time.NewTicker(500 * time.Millisecond)
	defer stateTicker.Stop()
	heartbeatTicker := time.NewTicker(5 * time.Second)
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return err
		case <-scale.HealthChanges():
			if err := sendState(); err != nil {
				return err
			}
			if monitor, err := activeMonitor(control, scale); err == nil {
				sendInvalidations(monitor)
			}
		case <-stateTicker.C:
			if control == nil {
				writeAgentStatus(statusPath, true, digest)
			} else {
				current, _, _ := control.snapshot()
				writeAgentStatus(statusPath, true, agentprinter.ConfigDigest(current))
				if err := sendMaintenance(); err != nil {
					return err
				}
			}
			if err := sendState(); err != nil {
				return err
			}
			if monitor, err := activeMonitor(control, scale); err == nil {
				sendInvalidations(monitor)
			}
		case <-heartbeatTicker.C:
			if err := send("heartbeat", struct{}{}); err != nil {
				return err
			}
		case m := <-incoming:
			switch m.Type {
			case "maintenance.request":
				if control == nil {
					continue
				}
				var request p.MaintenanceRequest
				if json.Unmarshal(m.Payload, &request) != nil {
					continue
				}
				go func(id string, in p.MaintenanceRequest) {
					result := control.maintenance.DeviceRequest(ctx, in)
					if err := sendMaintenance(); err != nil {
						cancelRun(err)
						return
					}
					if err := sendMessage(p.New("maintenance.result", fmt.Sprintf("maintenance-%d", time.Now().UnixNano()), id, result)); err != nil {
						cancelRun(err)
					}
				}(m.RequestID, request)
			case "camera.test.request":
				select {
				case captureBusy <- struct{}{}:
					go func(requestID string) {
						defer func() { <-captureBusy }()
						result := p.CameraTest{}
						if control == nil {
							result = (agentcamera.Client{Config: cfg}).Test(ctx)
						} else {
							result, _ = control.testCameras(ctx, cfg.ScaleID)
						}
						if err := sendMessage(p.New("camera.test.result", fmt.Sprintf("printer-%d", time.Now().UnixNano()), requestID, result)); err != nil {
							cancelRun(err)
						}
					}(m.RequestID)
				default:
					if err := sendMessage(p.New("camera.test.result", fmt.Sprintf("printer-%d", time.Now().UnixNano()), m.RequestID, p.CameraTest{At: time.Now().UTC()})); err != nil {
						return err
					}
				}
			case "capture.request":
				var request p.CaptureRequest
				if err := json.Unmarshal(m.Payload, &request); err != nil {
					_ = sendMessage(p.New("capture.result", fmt.Sprintf("printer-%d", time.Now().UnixNano()), m.RequestID, p.Capture{Error: "采集参数无效"}))
					continue
				}
				select {
				case captureBusy <- struct{}{}:
					go func(requestID string, request p.CaptureRequest) {
						defer func() { <-captureBusy }()
						result := p.Capture{}
						if control == nil {
							result = captureEvidenceWithPolicy(ctx, cfg, scale, request.RequiresImages())
						} else {
							result = control.captureWithPolicy(ctx, request.RequiresImages())
						}
						if err := sendMessage(p.New("capture.result", fmt.Sprintf("printer-%d", time.Now().UnixNano()), requestID, result)); err != nil {
							cancelRun(err)
						}
					}(m.RequestID, request)
				default:
					result := p.Capture{Success: false, Error: "正在采集，请核实前次请求结果", CapturedAt: time.Now().UTC()}
					if err := sendMessage(p.New("capture.result", fmt.Sprintf("printer-%d", time.Now().UnixNano()), m.RequestID, result)); err != nil {
						return err
					}
				}
			case p.TypeMeasurementBeginRequest:
				var request p.MeasurementBeginRequest
				if err := json.Unmarshal(m.Payload, &request); err != nil {
					if err := sendRequest(p.ProtocolVersion, p.TypeMeasurementBeginResult, m.RequestID, p.MeasurementBeginResult{Accepted: false, Code: p.ErrorInvalidPayload, Message: "测量开始参数无效"}); err != nil {
						return err
					}
					continue
				}
				result := beginMeasurement(control, scale, request)
				if err := sendRequest(p.ProtocolVersion, p.TypeMeasurementBeginResult, m.RequestID, result); err != nil {
					return err
				}
			case p.TypeMeasurementEndRequest:
				var request p.MeasurementEndRequest
				if err := json.Unmarshal(m.Payload, &request); err != nil {
					if err := sendRequest(p.ProtocolVersion, p.TypeMeasurementEndResult, m.RequestID, p.MeasurementEndResult{Verified: false, Code: p.ErrorInvalidPayload, Message: "测量结束参数无效", EndedAt: time.Now().UTC()}); err != nil {
						return err
					}
					continue
				}
				result := endMeasurement(control, scale, request)
				if err := sendRequest(p.ProtocolVersion, p.TypeMeasurementEndResult, m.RequestID, result); err != nil {
					return err
				}
			case p.TypeMeasurementCancelRequest:
				var request p.MeasurementCancelRequest
				if err := json.Unmarshal(m.Payload, &request); err != nil {
					if err := sendRequest(p.ProtocolVersion, p.TypeMeasurementCancelResult, m.RequestID, p.MeasurementCancelResult{Cancelled: false, Code: p.ErrorInvalidPayload, Message: "测量取消参数无效"}); err != nil {
						return err
					}
					continue
				}
				result := cancelMeasurement(control, scale, request)
				if err := sendRequest(p.ProtocolVersion, p.TypeMeasurementCancelResult, m.RequestID, result); err != nil {
					return err
				}
			case p.TypeMeasurementVerifyRequest:
				var request p.MeasurementVerifyRequest
				if err := json.Unmarshal(m.Payload, &request); err != nil {
					if err := sendRequest(p.ProtocolVersion, p.TypeMeasurementVerifyResult, m.RequestID, p.MeasurementVerifyResult{Valid: false, Code: p.ErrorInvalidPayload, Message: "测量核实参数无效", CheckedAt: time.Now().UTC()}); err != nil {
						return err
					}
					continue
				}
				result := verifyMeasurement(control, scale, request)
				if err := sendRequest(p.ProtocolVersion, p.TypeMeasurementVerifyResult, m.RequestID, result); err != nil {
					return err
				}
			case p.TypeAgentConfigApplyRequest:
				var request p.ConfigApplyRequest
				if json.Unmarshal(m.Payload, &request) != nil {
					if err := sendRequest(p.ProtocolVersion, p.TypeAgentConfigApplyResult, m.RequestID, p.ConfigApplyResult{Status: p.ConfigApplyRejected, Code: p.ErrorInvalidPayload, Message: "配置应用参数无效"}); err != nil {
						return err
					}
					continue
				}
				go func(requestID string, request p.ConfigApplyRequest) {
					result := p.ConfigApplyResult{Status: p.ConfigApplyRejected, Code: p.ErrorDeviceUnavailable, Message: "设备维护未启用"}
					if control != nil {
						result = control.maintenance.ApplySettings(ctx, request)
					}
					if err := sendRequest(p.ProtocolVersion, p.TypeAgentConfigApplyResult, requestID, result); err != nil {
						cancelRun(err)
						return
					}
					if result.Status == p.ConfigApplyApplied {
						if err := sendState(); err != nil {
							cancelRun(err)
							return
						}
						if err := sendConfigState(); err != nil {
							cancelRun(err)
							return
						}
						if err := sendConfigBackup(); err != nil {
							cancelRun(err)
						}
					}
				}(m.RequestID, request)
			case p.TypeAgentConfigRestoreRequest:
				var request p.ConfigRestoreRequest
				if json.Unmarshal(m.Payload, &request) != nil {
					if err := sendRequest(p.ProtocolVersion, p.TypeAgentConfigRestoreResult, m.RequestID, p.ConfigApplyResult{Status: p.ConfigApplyRejected, Code: p.ErrorInvalidPayload, Message: "配置恢复参数无效"}); err != nil {
						return err
					}
					continue
				}
				go func(requestID string, request p.ConfigRestoreRequest) {
					result := p.ConfigApplyResult{Status: p.ConfigApplyRejected, Code: p.ErrorDeviceUnavailable, Message: "设备维护未启用"}
					if control != nil {
						result = control.maintenance.RestoreSettings(ctx, request)
					}
					if err := sendRequest(p.ProtocolVersion, p.TypeAgentConfigRestoreResult, requestID, result); err != nil {
						cancelRun(err)
						return
					}
					if result.Status == p.ConfigApplyApplied {
						if err := sendState(); err != nil {
							cancelRun(err)
							return
						}
						if err := sendConfigState(); err != nil {
							cancelRun(err)
							return
						}
						if err := sendConfigBackup(); err != nil {
							cancelRun(err)
						}
					}
				}(m.RequestID, request)
			case p.TypePrinterQueuesRequest:
				result := p.CupsQueuesResult{At: time.Now().UTC(), Code: p.ErrorQueuesUnavailable, Error: "设备维护未启用"}
				if control != nil {
					result = control.maintenance.Queues()
				}
				if err := sendRequest(p.ProtocolVersion, p.TypePrinterQueuesResult, m.RequestID, result); err != nil {
					return err
				}
			case "print.request":
				var req agentprinter.Request
				if err := json.Unmarshal(m.Payload, &req); err != nil {
					slog.Error("invalid print request", "error", err)
					continue
				}
				if m.RequestID != fmt.Sprintf("print-%d", req.AttemptID) {
					slog.Error("print identity mismatch")
					continue
				}
				if control != nil {
					select {
					case managedPrints <- devicePrint{request: req, generation: control.printGeneration()}:
					default:
						slog.Error("print worker queue full", "attempt", req.AttemptID)
					}
					continue
				}
				select {
				case prints <- req:
				default:
					slog.Error("print worker queue full", "attempt", req.AttemptID)
				}
			}
		}
	}
}

// currentDeviceState never turns a stale diagnostic reading into a fresh
// stable value. A negative, anomalous or precision-incompatible reading is
// reported as unavailable with weight zero, because the v1 payload cannot
// carry a negative business weight.
func currentDeviceState(scale *agentscale.Monitor, now time.Time) p.DeviceState {
	if scale == nil {
		return p.DeviceState{Unit: "t", DeviceOK: false, Error: "称重仪表尚未配置"}
	}
	return deviceStateFromSnapshot(scale.Snapshot(now))
}

func deviceStateFromSnapshot(s agentscale.Snapshot) p.DeviceState {
	if !s.DeviceOK {
		reason := s.Fault
		if reason == "" {
			reason = "称重仪表当前不可采集"
		}
		return p.DeviceState{Unit: "t", DeviceOK: false, Stable: false, Error: reason}
	}
	return p.DeviceState{Weight: p.Weight(s.WeightMilliTonnes), Unit: "t", Stable: s.Stable, DeviceOK: true}
}

func measuredState(s agentscale.Snapshot) p.MeasuredState {
	return p.MeasuredState{DeviceState: deviceStateFromSnapshot(s), At: s.LastValidAt.UTC()}
}

func activeMonitor(control *deviceRuntime, fallback *agentscale.Monitor) (*agentscale.Monitor, error) {
	if control == nil {
		if fallback == nil {
			return nil, agentscale.ErrMeasurementUnavailable
		}
		return fallback, nil
	}
	return control.measurementScale()
}

func beginMeasurement(control *deviceRuntime, fallback *agentscale.Monitor, request p.MeasurementBeginRequest) p.MeasurementBeginResult {
	if request.CaptureID == "" || len(request.CaptureID) > 80 || len(request.CapturePolicyRevision) > 80 || request.CameraConfigVersion < 0 {
		return p.MeasurementBeginResult{Accepted: false, Code: p.ErrorInvalidPayload, Message: "测量开始参数无效"}
	}
	monitor, err := activeMonitor(control, fallback)
	if err != nil {
		return p.MeasurementBeginResult{Accepted: false, Code: p.ErrorDeviceUnavailable, Message: err.Error()}
	}
	measurement, err := monitor.BeginMeasurement(time.Now())
	result := p.MeasurementBeginResult{State: measuredState(measurement.Snapshot)}
	if err != nil {
		result.Code, result.Message = measurementErrorCode(err), err.Error()
		return result
	}
	result.Accepted, result.Generation = true, measurement.Generation
	return result
}

func endMeasurement(control *deviceRuntime, fallback *agentscale.Monitor, request p.MeasurementEndRequest) p.MeasurementEndResult {
	now := time.Now()
	result := p.MeasurementEndResult{Generation: request.Generation, EndedAt: now.UTC()}
	monitor, err := activeMonitor(control, fallback)
	if err != nil {
		result.Code, result.Message = p.ErrorDeviceUnavailable, err.Error()
		return result
	}
	measurement, err := monitor.EndMeasurement(request.Generation, now)
	result.State = measuredState(measurement.Snapshot)
	if !measurement.EndedAt.IsZero() {
		result.EndedAt = measurement.EndedAt.UTC()
	}
	if err != nil {
		result.Code, result.Message = measurementErrorCode(err), err.Error()
		return result
	}
	result.Verified = true
	return result
}

func cancelMeasurement(control *deviceRuntime, fallback *agentscale.Monitor, request p.MeasurementCancelRequest) p.MeasurementCancelResult {
	result := p.MeasurementCancelResult{Generation: request.Generation}
	monitor, err := activeMonitor(control, fallback)
	if err != nil {
		result.Code, result.Message = p.ErrorDeviceUnavailable, err.Error()
		return result
	}
	if err := monitor.CancelMeasurement(request.Generation); err != nil {
		result.Code, result.Message = measurementErrorCode(err), err.Error()
		return result
	}
	result.Cancelled = true
	return result
}

func verifyMeasurement(control *deviceRuntime, fallback *agentscale.Monitor, request p.MeasurementVerifyRequest) p.MeasurementVerifyResult {
	now := time.Now()
	result := p.MeasurementVerifyResult{Generation: request.Generation, CheckedAt: now.UTC()}
	monitor, err := activeMonitor(control, fallback)
	if err != nil {
		result.Code, result.Message = p.ErrorDeviceUnavailable, err.Error()
		return result
	}
	measurement, err := monitor.VerifyMeasurement(request.Generation, now)
	result.State = measuredState(measurement.Snapshot)
	if err != nil {
		result.Code, result.Message = measurementErrorCode(err), err.Error()
		return result
	}
	result.Valid = true
	return result
}

func measurementErrorCode(err error) string {
	switch {
	case errors.Is(err, agentscale.ErrMeasurementUnstable):
		return p.ErrorDeviceUnstable
	case errors.Is(err, agentscale.ErrMeasurementStale):
		return p.ErrorDeviceStale
	case errors.Is(err, agentscale.ErrMeasurementFault):
		return p.ErrorDeviceFault
	case errors.Is(err, agentscale.ErrMeasurementWeightChanged):
		return p.ErrorWeightChanged
	case errors.Is(err, agentscale.ErrMeasurementSuperseded):
		return p.ErrorGenerationSuperseded
	case errors.Is(err, agentscale.ErrMeasurementGenerationInvalid):
		return p.ErrorGenerationInvalid
	case errors.Is(err, agentscale.ErrMeasurementUnavailable):
		return p.ErrorDeviceUnavailable
	default:
		return p.ErrorDeviceUnavailable
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// helloHandshake performs the revision 2 keyless admission handshake. A
// rejection (uninitialized site, missing scale, conflict or unsupported
// protocol) is surfaced with its actual reason for logging and retry.
func helloHandshake(conn *websocket.Conn) (p.HelloAck, error) {
	var ack p.HelloAck
	hostname, _ := os.Hostname()
	request := fmt.Sprintf("hello-%d", time.Now().UnixNano())
	message := p.NewVersion(p.ProtocolVersion, p.TypeHello, "hello-"+request, request, p.Hello{
		ProtocolVersion: p.ProtocolVersion,
		AgentVersion:    "agent-printer",
		Hostname:        hostname,
		Capabilities: []string{
			p.CapabilityMeasurementWindow, p.CapabilityEffectiveConfig,
			p.CapabilityCupsQueues, p.CapabilityParameterBackup,
		},
	})
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := websocket.JSON.Send(conn, message); err != nil {
		return ack, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var reply p.Message
		if err := websocket.JSON.Receive(conn, &reply); err != nil {
			return ack, fmt.Errorf("等待 Agent 接入确认失败：%w", err)
		}
		switch reply.Type {
		case p.TypeHelloAck:
			if reply.RequestID != "" && reply.RequestID != request {
				continue
			}
			if err := json.Unmarshal(reply.Payload, &ack); err != nil {
				return ack, errors.New("Agent 接入确认格式无效")
			}
			if ack.ProtocolVersion != p.ProtocolVersion {
				return ack, fmt.Errorf("Agent 协议版本不受支持：%d", ack.ProtocolVersion)
			}
			return ack, nil
		case "error":
			var refusal p.Error
			_ = json.Unmarshal(reply.Payload, &refusal)
			if refusal.Message == "" {
				refusal.Message = "Server 拒绝了设备接入"
			}
			return ack, fmt.Errorf("Agent 接入失败（%s）：%s", refusal.Code, refusal.Message)
		}
	}
}

func captureFailure(scale *agentscale.Monitor, now time.Time) p.Capture {
	s := scale.Snapshot(now)
	reason := "摄像头未接入，无法取得前后抓拍，本次采集失败"
	if s.Enabled && !s.DeviceOK && s.Fault != "" {
		reason += "；仪表状态：" + s.Fault
	}
	return p.Capture{
		Success: false, Error: reason, DeviceState: currentDeviceState(scale, now),
		CapturedAt: now.UTC(), Images: []p.Image{},
	}
}

// captureEvidence keeps camera acquisition off the WSS event loop. Every new
// attempt uses fresh images and rechecks the live instrument throughout capture.
func captureEvidence(parent context.Context, cfg agentprinter.Config, scale *agentscale.Monitor) p.Capture {
	return captureEvidenceWithPolicy(parent, cfg, scale, true)
}
func captureEvidenceWithPolicy(parent context.Context, cfg agentprinter.Config, scale *agentscale.Monitor, required bool) p.Capture {
	if cfg.CameraFrontURL == "" || cfg.CameraRearURL == "" {
		if required {
			return captureFailure(scale, time.Now())
		}
		state := currentDeviceState(scale, time.Now())
		return p.Capture{Success: parent.Err() == nil && state.DeviceOK && state.Stable && state.Weight > 0, DeviceState: state, CapturedAt: time.Now().UTC(), Images: []p.Image{}, ImageStatus: "disabled", ImageError: "未启用抓拍"}
	}
	initial := currentDeviceState(scale, time.Now())
	if !initial.DeviceOK || !initial.Stable {
		return p.Capture{Error: "称重仪表不可用或未稳定", DeviceState: initial, CapturedAt: time.Now().UTC()}
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				state := currentDeviceState(scale, time.Now())
				if !state.DeviceOK || !state.Stable || state.Weight != initial.Weight {
					cancel()
					return
				}
			}
		}
	}()
	images, plate, err := (agentcamera.Client{Config: cfg}).Capture(ctx)
	now := time.Now().UTC()
	final := currentDeviceState(scale, now)
	result := p.Capture{DeviceState: final, CapturedAt: now, Images: []p.Image{}, Plate: plate}
	if parent.Err() != nil || (ctx.Err() != nil && (required || !errors.Is(ctx.Err(), context.DeadlineExceeded))) || !final.DeviceOK || !final.Stable || final.Weight != initial.Weight {
		result.Error = "采集期间仪表失稳、重量变化或采集超时，请重新采集"
		return result
	}
	result.Images = images
	if err != nil {
		result.ImageStatus, result.ImageError = "capture_failed", err.Error()
		if errors.Is(err, agentcamera.ErrUpload) {
			result.ImageStatus = "upload_failed"
		}
		if required {
			result.Error = err.Error()
			return result
		}
	} else {
		result.ImageStatus = "complete"
	}
	result.Success = true
	return result
}

func printWorker(ctx context.Context, cfg agentprinter.Config, state, queue, pausePath string, jobs <-chan agentprinter.Request) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-jobs:
			handlePrint(ctx, cfg, state, queue, pausePath, req)
		}
	}
}

func handlePrint(ctx context.Context, cfg agentprinter.Config, state, queue, pausePath string, req agentprinter.Request) {
	for {
		if _, err := os.Stat(pausePath); os.IsNotExist(err) {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	if queue == "" {
		// A keyless first installation may not have a CUPS queue yet. Do not
		// reserve an attempt or invoke lp with an empty destination; the Server
		// keeps the physical result unknown for the existing verification flow.
		slog.Error("print queue not configured; print not submitted", "attempt", req.AttemptID)
		return
	}
	jobCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if req.Ticket.PrintSheet != nil {
		req.Ticket.PrintSheet.Calibration = cfg.Calibration()
	}
	data, err := agentprinter.Render(jobCtx, req.Ticket.PrintSheet)
	job := ""
	if err == nil {
		job, err = agentprinter.Submit(jobCtx, state, queue, req, data)
	}
	slog.Info("print attempt requires onsite verification", "attempt", req.AttemptID, "cups", job, "error", err)
	// CUPS submission does not prove physical output. Leave the Server attempt
	// unknown for the existing operator verification workflow; never auto-reprint.
}

func writeScaleStatus(ctx context.Context, path string, scale *agentscale.Monitor) {
	write := func() {
		now := time.Now()
		_ = agentscale.WriteStatus(path, scale.Snapshot(now), now)
	}
	write()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			write()
		}
	}
}

func writeAgentStatus(path string, connected bool, digest string) {
	b, _ := json.Marshal(struct {
		At           time.Time `json:"at"`
		Connected    bool      `json:"connected"`
		ConfigDigest string    `json:"configDigest"`
	}{time.Now(), connected, digest})
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, path)
	}
}
