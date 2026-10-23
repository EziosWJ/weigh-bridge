package agentmaintenance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

func writePrivate(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".maintenance-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func importEnvironment(path string, cfg Config) (Config, error) {
	return agentprinter.ReadEnvironment(path, cfg)
}

// agent.env is authoritative; maintenance.json keeps drafts and rollback data.
// Preserve an edited draft when an administrator updates agent.env.
// Callers serialize this with opMu after startup.
func (s *Service) reloadEnvironment() (bool, error) {
	path := s.environmentPath()
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	c, err := importEnvironment(path, Defaults())
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != c {
		oldActive, oldDraft := s.active, s.draft
		if s.draft == s.active {
			s.draft = c
		}
		s.active = c
		if err := s.persist(); err != nil {
			s.active, s.draft = oldActive, oldDraft
			return false, err
		}
	}
	s.startupError = ""
	return true, nil
}

func (s *Service) runtimeConfigState(c Config) string {
	running := s.runner.Run([]string{"systemctl", "--user", "is-active", "weighbridge-agent-printer.service"})
	if running.Err != nil || strings.TrimSpace(running.Output) != "active" {
		if s.deviceOwned && strings.TrimSpace(running.Output) == "inactive" {
			return "设备执行已停止；Agent 维护连接保留"
		}
		return "未知／打印 Agent 未运行或状态不可读取"
	}
	b, err := os.ReadFile(filepath.Join(s.dir, "maintenance-status.json"))
	var state struct {
		At     time.Time `json:"at"`
		Digest string    `json:"configDigest"`
	}
	if err != nil || json.Unmarshal(b, &state) != nil || state.Digest == "" || time.Since(state.At) > 10*time.Second || time.Until(state.At) > 5*time.Second {
		return "未知／暂无运行配置反馈"
	}
	if state.Digest != agentprinter.ConfigDigest(c) {
		return "运行配置与 agent.env 不一致，请应用配置重启打印 Agent"
	}
	return "运行配置与 agent.env 一致"
}

// The caller has stopped the Agent and checked the queues. Failed activation
// restores the environment and keeps the last confirmed active configuration.
func (s *Service) activateConfig(cfg Config, restore bool) (Result, bool) {
	if err := validate(cfg); err != nil {
		return Result{Err: err}, true
	}
	if cfg.AgentKey == "" || strings.ContainsAny(cfg.AgentKey, "\r\n\x00") {
		return Result{Err: errors.New("Agent 连接密钥未设置或格式无效")}, true
	}
	path := s.environmentPath()
	oldEnv, err := os.ReadFile(path)
	hadEnv := err == nil
	if err != nil && !os.IsNotExist(err) {
		return Result{Err: err}, true
	}
	s.mu.Lock()
	oldActive, oldDraft, oldPrevious := s.active, s.draft, s.previous
	s.previous = oldActive
	err = s.persist()
	s.mu.Unlock()
	if err != nil {
		s.mu.Lock()
		s.previous = oldPrevious
		s.mu.Unlock()
		return Result{Err: err}, true
	}
	result := Result{Err: BuildAgentEnvironmentAt(s.environmentPath(), cfg)}
	started := false
	if result.Err == nil {
		started = true
		result = s.runner.Run([]string{"systemctl", "--user", "start", "weighbridge-agent-printer.service"})
	}
	if result.Err == nil {
		result.Err = s.verifyAgent()
	}
	if result.Err == nil && cfg.CameraMode == "mediamtx" {
		result.Err = s.verifyCameraRuntime(cfg)
	}
	if result.Err == nil {
		s.mu.Lock()
		s.active = cfg
		if restore {
			s.draft = cfg
		}
		result.Err = s.persist()
		s.mu.Unlock()
	}
	if result.Err != nil {
		if started {
			stopped := s.runner.Run([]string{"systemctl", "--user", "stop", "weighbridge-agent-printer.service"})
			if stopped.Err != nil {
				result.Err = fmt.Errorf("%v；停止新配置 Agent 失败：%w", result.Err, stopped.Err)
				return result, false
			}
		}
		safe := true
		if hadEnv {
			err = writePrivate(path, oldEnv)
		} else {
			err = os.Remove(path)
		}
		if err != nil && !os.IsNotExist(err) {
			result.Err = fmt.Errorf("%v；恢复原环境失败：%w", result.Err, err)
			safe = false
		}
		s.mu.Lock()
		s.active, s.draft, s.previous = oldActive, oldDraft, oldPrevious
		err = s.persist()
		s.mu.Unlock()
		if err != nil {
			result.Err = fmt.Errorf("%v；保存回退状态失败：%w", result.Err, err)
			safe = false
		}
		return result, safe
	}
	return result, true
}
