package agentmaintenance

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

// deviceConfigSchema is the only remotely writable device configuration after
// ADR 0030: serial transport, print queue and calibration. Connection identity,
// Server URL and runtime/execution state are never accepted from this path.
func deviceSettingsFromConfig(c Config) p.DeviceSettings {
	return p.DeviceSettings{
		Serial: p.SerialSettings{Port: c.SerialPort, Baud: c.SerialBaud},
		Print: p.PrintSettings{
			Queue: c.Queue, FormLengthMM: c.FormLengthMM, PrintWidthMM: c.PrintWidthMM,
			PrintScale: c.PrintScale, OffsetXMM: c.OffsetXMM, OffsetYMM: c.OffsetYMM,
		},
	}
}

func applyDeviceSettings(c Config, in p.DeviceSettings) Config {
	c.SerialPort, c.SerialBaud = in.Serial.Port, in.Serial.Baud
	c.Queue = in.Print.Queue
	c.FormLengthMM, c.PrintWidthMM, c.PrintScale = in.Print.FormLengthMM, in.Print.PrintWidthMM, in.Print.PrintScale
	c.OffsetXMM, c.OffsetYMM = in.Print.OffsetXMM, in.Print.OffsetYMM
	return agentprinter.NormalizeConfig(c)
}

var queuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func validateDeviceSettings(in p.DeviceSettings) error {
	if in.Print.FormLengthMM <= 0 || in.Print.PrintWidthMM <= 0 || in.Print.PrintScale <= 0 {
		return errInvalidSettings("打印节长、宽度与缩放必须为有效数值")
	}
	if err := (agentprinter.Calibration{
		WidthMM: in.Print.PrintWidthMM, OffsetXMM: in.Print.OffsetXMM, OffsetYMM: in.Print.OffsetYMM,
		FormLengthMM: in.Print.FormLengthMM, Scale: in.Print.PrintScale,
	}).Validate(); err != nil {
		return err
	}
	if in.Serial.Port != "" {
		if !strings.HasPrefix(in.Serial.Port, "/dev/") || strings.Contains(in.Serial.Port, "..") || strings.ContainsAny(in.Serial.Port, "\r\n\x00") {
			return errInvalidSettings("称重仪表串口路径无效，须为 /dev 下的设备路径")
		}
		switch in.Serial.Baud {
		case 1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200:
		default:
			return errInvalidSettings("称重仪表波特率无效")
		}
	}
	if in.Print.Queue != "" && !queuePattern.MatchString(in.Print.Queue) {
		return errInvalidSettings("打印队列名称无效")
	}
	return nil
}

type settingsError string

func (e settingsError) Error() string { return string(e) }

func errInvalidSettings(message string) error { return settingsError(message) }

func (s *Service) agentRunning() bool {
	r := s.runner.Run([]string{"systemctl", "--user", "is-active", "weighbridge-agent-printer.service"})
	return r.Err == nil && strings.TrimSpace(r.Output) == "active"
}

// EffectiveConfig is the applied-device truth reported to the Server. It is
// derived from the last successfully applied local configuration, not from the
// editable form or an in-flight candidate.
func (s *Service) EffectiveConfig() p.EffectiveConfig {
	s.mu.Lock()
	cfg, version, applied := s.active, s.configVersion, s.appliedAt
	s.mu.Unlock()
	running := s.agentRunning()
	deviceState := "unavailable"
	if running {
		deviceState = "running"
	}
	serialState := "not_configured"
	if cfg.SerialPort != "" {
		serialState = "configured"
		view := readScaleView(s.scaleStatusPath(), time.Now(), running)
		if !running {
			serialState = "unavailable"
		} else if view.Known && !view.DeviceOK {
			serialState = "fault"
			deviceState = "degraded"
		}
	} else if running {
		deviceState = "degraded"
	}
	printerState := "not_configured"
	if cfg.Queue != "" {
		printerState = "unavailable"
		if s.runner.Run([]string{"lpstat", "-p", cfg.Queue}).Err == nil {
			printerState = "ready"
		}
	}
	return p.EffectiveConfig{
		ConfigVersion: version, Digest: deviceSettingsFromConfig(cfg).Digest(),
		Serial: p.SerialSettings{Port: cfg.SerialPort, Baud: cfg.SerialBaud},
		Print: p.PrintSettings{
			Queue: cfg.Queue, FormLengthMM: cfg.FormLengthMM, PrintWidthMM: cfg.PrintWidthMM,
			PrintScale: cfg.PrintScale, OffsetXMM: cfg.OffsetXMM, OffsetYMM: cfg.OffsetYMM,
		},
		AppliedAt: applied, Running: running, DeviceState: deviceState,
		SerialState: serialState, PrinterState: printerState,
	}
}

func (s *Service) scaleStatusPath() string {
	return s.dir + "/scale-status.json"
}

// ConfigBackup reports the last successfully applied device parameters. It is
// the value the Server may offer for an explicit restore; connation and runtime
// state are deliberately excluded.
func (s *Service) ConfigBackup() (p.ConfigBackup, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.appliedAt.IsZero() {
		return p.ConfigBackup{}, false
	}
	settings := deviceSettingsFromConfig(s.active)
	return p.ConfigBackup{ConfigVersion: s.configVersion, Digest: settings.Digest(), Settings: settings, AppliedAt: s.appliedAt}, true
}

// Queues enumerates CUPS destinations on the Agent host. The maintenance page
// must never guess Server-host queues.
func (s *Service) Queues() p.CupsQueuesResult {
	now := time.Now().UTC()
	listed := s.runner.Run([]string{"lpstat", "-p"})
	if listed.Err != nil {
		message := strings.TrimSpace(listed.Output)
		if message == "" {
			message = listed.Err.Error()
		}
		return p.CupsQueuesResult{At: now, Code: p.ErrorQueuesUnavailable, Error: message}
	}
	defaultName := ""
	if out := s.runner.Run([]string{"lpstat", "-d"}); out.Err == nil {
		if _, value, ok := strings.Cut(strings.TrimSpace(out.Output), ":"); ok {
			defaultName = strings.TrimSpace(value)
		}
	}
	queues := []p.CupsQueue{}
	for _, line := range strings.Split(listed.Output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "printer" {
			continue
		}
		state := "idle"
		if strings.Contains(line, "disabled") || strings.Contains(line, "paused") {
			state = "disabled"
		}
		queues = append(queues, p.CupsQueue{Name: fields[1], State: state, IsDefault: fields[1] == defaultName})
	}
	return p.CupsQueuesResult{Queues: queues, At: now}
}

// ApplySettings is the single save-and-apply operation used by the Server. It
// validates, persists, reloads the device runtime, verifies it and restores the
// last successful environment on failure. The maintenance connection itself is
// owned by the caller and stays connected throughout.
func (s *Service) ApplySettings(ctx context.Context, req p.ConfigApplyRequest) p.ConfigApplyResult {
	return s.applyDeviceSettings(ctx, req.BaseVersion, req.Settings, 0)
}

// RestoreSettings applies one Server-held backup only after an explicit
// restore request. The full settings travel with the request so a stale backup
// id can never overwrite newer local values without the version check.
func (s *Service) RestoreSettings(ctx context.Context, req p.ConfigRestoreRequest) p.ConfigApplyResult {
	return s.applyDeviceSettings(ctx, req.BaseVersion, req.Backup.Settings, req.BackupID)
}

func (s *Service) applyDeviceSettings(ctx context.Context, baseVersion int64, settings p.DeviceSettings, restoreOf int64) p.ConfigApplyResult {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	// A valid external edit is picked up before choosing the candidate base.
	_, _ = s.reloadEnvironment()
	s.mu.Lock()
	active, version := s.active, s.configVersion
	s.mu.Unlock()
	if baseVersion != 0 && baseVersion != version {
		return p.ConfigApplyResult{Status: p.ConfigApplyRejected, Code: p.ErrorConfigConflict, Message: "配置已在设备上更新，请刷新后重试"}
	}
	if err := validateDeviceSettings(settings); err != nil {
		return p.ConfigApplyResult{Status: p.ConfigApplyRejected, Code: p.ErrorConfigInvalid, Message: err.Error()}
	}
	candidate := applyDeviceSettings(active, settings)
	if err := validate(candidate); err != nil {
		return p.ConfigApplyResult{Status: p.ConfigApplyRejected, Code: p.ErrorConfigInvalid, Message: err.Error()}
	}
	if ctx.Err() != nil {
		return p.ConfigApplyResult{Status: p.ConfigApplyRejected, Code: p.ErrorCancelled, Message: "请求已取消，配置未应用"}
	}
	// The runtime reads agent.env when it starts. Stop the current execution
	// first so the candidate is actually loaded; an already-running process
	// would otherwise keep the old parameters while reporting success.
	wasRunning := s.agentRunning()
	if stopped := s.runner.Run([]string{"systemctl", "--user", "stop", "weighbridge-agent-printer.service"}); stopped.Err != nil {
		return p.ConfigApplyResult{Status: p.ConfigApplyRejected, Code: p.ErrorDeviceBusy, Message: "设备正在执行，暂不能应用配置：" + stopped.Err.Error()}
	}
	result, safe := s.activateConfig(candidate, true)
	s.mu.Lock()
	currentVersion, appliedAt := s.configVersion, s.appliedAt
	s.mu.Unlock()
	if result.Err != nil {
		if wasRunning {
			resumed := s.runner.Run([]string{"systemctl", "--user", "start", "weighbridge-agent-printer.service"})
			if resumed.Err == nil {
				resumed.Err = s.verifyAgent()
			}
			if resumed.Err != nil {
				result.Err = errors.Join(result.Err, resumed.Err)
				safe = false
			}
		}
		status, code := p.ConfigApplyRolledBack, p.ErrorRollbackFailed
		if !safe {
			status, code = p.ConfigApplyRollbackFailed, p.ErrorRollbackFailed
		}
		effective := s.EffectiveConfig()
		return p.ConfigApplyResult{Status: status, Code: code, Message: result.Err.Error(), ConfigVersion: currentVersion, Digest: deviceSettingsFromConfig(candidate).Digest(), Effective: &effective}
	}
	// Applying parameters is not an execution request. Restore a previously
	// stopped device so maintenance cannot silently start weighing or printing.
	if !wasRunning {
		_ = s.runner.Run([]string{"systemctl", "--user", "stop", "weighbridge-agent-printer.service"})
	}
	effective := s.EffectiveConfig()
	_ = restoreOf
	return p.ConfigApplyResult{
		Status: p.ConfigApplyApplied, ConfigVersion: currentVersion,
		Digest: settings.Digest(), AppliedAt: &appliedAt, Effective: &effective,
	}
}
