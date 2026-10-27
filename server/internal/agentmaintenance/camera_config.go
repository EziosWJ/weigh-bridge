package agentmaintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

func (s *Service) SetOperationGuard(fn func(context.Context) (func(), error)) { s.operationGuard = fn }

// CameraConfig exposes a shared mapping only after the running Agent confirms
// the active environment digest; drafts and unverified external edits stay unavailable.
func (s *Service) CameraConfig() (Config, error) {
	s.mu.Lock()
	c := s.active
	s.mu.Unlock()
	if c.CameraMode == "mediamtx" {
		b, err := os.ReadFile(filepath.Join(s.dir, "maintenance-status.json"))
		var runtime struct {
			At     time.Time `json:"at"`
			Digest string    `json:"configDigest"`
		}
		if err != nil || json.Unmarshal(b, &runtime) != nil || time.Since(runtime.At) > 10*time.Second || time.Until(runtime.At) > 5*time.Second || runtime.Digest != agentprinter.ConfigDigest(c) {
			return c, errors.New("Agent 当前配置未核实，请检查设备状态")
		}
	}
	return c, nil
}
func (s *Service) CameraAvailable(scaleCode string) bool {
	c, err := s.CameraConfig()
	return c.CameraMode == "" || (err == nil && c.ScaleCode == scaleCode && c.CameraEnabled)
}

func (s *Service) verifyCameraRuntime(c Config) error {
	deadline := time.Now().Add(3 * time.Second)
	for {
		b, err := os.ReadFile(filepath.Join(s.dir, "maintenance-status.json"))
		var status struct {
			At     time.Time `json:"at"`
			Digest string    `json:"configDigest"`
		}
		if err == nil && json.Unmarshal(b, &status) == nil && time.Since(status.At) < 10*time.Second && time.Until(status.At) < 5*time.Second && status.Digest == agentprinter.ConfigDigest(c) {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("Agent 未确认采用新的摄像头配置，已停止切换并尝试回退")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
