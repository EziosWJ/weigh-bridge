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
	scale, err := agentscale.New(agentscale.Config{
		Enabled: cfg.SerialEnabled, Port: cfg.SerialPort, Baud: cfg.SerialBaud,
		Protocol: cfg.SerialProtocol, Unit: agentscale.Unit(cfg.SerialWeightUnit),
		Stale: time.Duration(cfg.SerialStaleMS) * time.Millisecond,
	}, nil)
	if err != nil {
		slog.Error("invalid weighing scale configuration", "error", err)
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
	scaleStatusPath := filepath.Join(filepath.Dir(state), "scale-status.json")
	go scale.Run(ctx)
	go writeScaleStatus(ctx, scaleStatusPath, scale)
	for ctx.Err() == nil {
		statusPath := filepath.Join(filepath.Dir(state), "maintenance-status.json")
		pausePath := filepath.Join(filepath.Dir(state), "maintenance.pause")
		if err := run(ctx, cfg, scale, identityDir, statusPath, pausePath); errors.Is(err, p.ErrKicked) {
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

func run(parent context.Context, cfg agentprinter.Config, scale *agentscale.Monitor, state, statusPath, pausePath string) (resultErr error) {
	ctx, cancelRun := context.WithCancelCause(parent)
	defer cancelRun(nil)
	defer func() {
		if errors.Is(context.Cause(ctx), p.ErrKicked) {
			resultErr = p.ErrKicked
		}
	}()
	server, id, key, scaleID, queue := cfg.ServerURL, cfg.AgentID, cfg.AgentKey, cfg.ScaleID, cfg.Queue
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
	wsConfig.Header.Set("Authorization", "Bearer "+key)
	wsConfig.Header.Set("X-Agent-ID", id)
	wsConfig.Header.Set("X-Scale-ID", strconv.FormatInt(scaleID, 10))
	dial, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := wsConfig.DialContext(dial)
	cancel()
	if err != nil {
		return err
	}
	defer conn.Close()
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
	slog.Info("printer Agent connected", "queue", queue, "scale", scaleID, "serial", cfg.SerialEnabled)
	var writes sync.Mutex
	sendMessage := func(message p.Message) error {
		writes.Lock()
		defer writes.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return websocket.JSON.Send(conn, message)
	}
	send := func(kind string, payload any) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return sendMessage(p.New(kind, fmt.Sprintf("printer-%d", time.Now().UnixNano()), "", payload))
	}
	sendState := func() error { return send("device.state", currentDeviceState(scale, time.Now())) }
	if err = sendState(); err != nil {
		return err
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
	go printWorker(ctx, cfg, state, queue, pausePath, prints)

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
		case <-stateTicker.C:
			writeAgentStatus(statusPath, true, digest)
			if err := sendState(); err != nil {
				return err
			}
		case <-heartbeatTicker.C:
			if err := send("heartbeat", struct{}{}); err != nil {
				return err
			}
		case m := <-incoming:
			switch m.Type {
			case "capture.request":
				if err := sendMessage(p.New("capture.result", fmt.Sprintf("printer-%d", time.Now().UnixNano()), m.RequestID, captureFailure(scale, time.Now()))); err != nil {
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
	s := scale.Snapshot(now)
	if !s.Enabled {
		return p.DeviceState{Unit: "t", DeviceOK: false, Error: "称重仪表未启用；仅打印可用"}
	}
	if !s.DeviceOK {
		reason := s.Fault
		if reason == "" {
			reason = "称重仪表当前不可采集"
		}
		return p.DeviceState{Unit: "t", DeviceOK: false, Stable: false, Error: reason}
	}
	return p.DeviceState{Weight: p.Weight(s.WeightMilliTonnes), Unit: "t", Stable: s.Stable, DeviceOK: true}
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
	jobCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if req.Ticket.PrintSheet != nil {
		req.Ticket.PrintSheet.Calibration = agentprinter.Calibration{WidthMM: cfg.PrintWidthMM, OffsetXMM: cfg.OffsetXMM, OffsetYMM: cfg.OffsetYMM, FormLengthMM: cfg.FormLengthMM}
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
