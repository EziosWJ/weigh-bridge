package agentmaintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

func TestMediaMappingRoundTripAndVerifiedRuntime(t *testing.T) {
	dir := t.TempDir()
	c := Defaults()
	c.AgentKey = "private-agent-key"
	c.SetCameraSettings(agentprinter.CameraSettings{Mode: "mediamtx", Enabled: true, RTSPBase: "rtsp://media:8554", WHEPBase: "https://video", FrontPath: "front", RearPath: "rear"})
	if err := BuildAgentEnvironment(dir, c); err != nil {
		t.Fatal(err)
	}
	actual, err := agentprinter.ReadEnvironment(filepath.Join(dir, "agent.env"), Defaults())
	if err != nil || actual != c {
		t.Fatalf("roundtrip: %v %+v", err, actual.CameraSettings())
	}
	s := &Service{dir: dir, active: c}
	if _, err = s.CameraConfig(); err == nil {
		t.Fatal("unverified runtime accepted")
	}
	data, _ := json.Marshal(map[string]any{"at": time.Now(), "configDigest": agentprinter.ConfigDigest(c)})
	if err = os.WriteFile(filepath.Join(dir, "maintenance-status.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CameraConfig(); err != nil || got.CameraSettings() != c.CameraSettings() {
		t.Fatal("verified config unavailable", err)
	}
	c.CameraEnabled = false
	s.active = agentprinter.NormalizeConfig(c)
	if s.CameraAvailable(c.ScaleCode) {
		t.Fatal("disabled camera accepted formal capture")
	}
}

type cameraActivationRunner struct {
	dir   string
	match bool
}

func (r cameraActivationRunner) Run(args []string) Result {
	if len(args) > 3 && args[0] == "systemctl" && args[2] == "start" {
		cfg, err := agentprinter.ReadEnvironment(filepath.Join(r.dir, "agent.env"), Defaults())
		if err != nil {
			return Result{Err: err}
		}
		digest := "wrong"
		if r.match {
			digest = agentprinter.ConfigDigest(cfg)
		}
		b, _ := json.Marshal(map[string]any{"at": time.Now(), "configDigest": digest})
		if err = os.WriteFile(filepath.Join(r.dir, "maintenance-status.json"), b, 0600); err != nil {
			return Result{Err: err}
		}
	}
	return Result{Output: "active"}
}
func TestMediaActivationRequiresRuntimeMatchAndRollsBack(t *testing.T) {
	for _, match := range []bool{true, false} {
		t.Run(map[bool]string{true: "verified", false: "mismatch"}[match], func(t *testing.T) {
			dir := t.TempDir()
			old := Defaults()
			old.AgentKey = "local-test-key"
			if err := BuildAgentEnvironment(dir, old); err != nil {
				t.Fatal(err)
			}
			candidate := old
			candidate.SetCameraSettings(agentprinter.CameraSettings{Mode: "mediamtx", Enabled: true, RTSPBase: "rtsp://media:8554", WHEPBase: "http://media:8889", FrontPath: "front", RearPath: "rear"})
			s := &Service{dir: dir, runner: cameraActivationRunner{dir, match}, active: old, draft: candidate}
			result, safe := s.activateConfig(candidate, false)
			actual, err := agentprinter.ReadEnvironment(filepath.Join(dir, "agent.env"), Defaults())
			if err != nil {
				t.Fatal(err)
			}
			if match {
				if result.Err != nil || s.active != candidate || actual != candidate {
					t.Fatal("verified activation failed", result.Err)
				}
			} else if result.Err == nil || !safe || s.active != old || actual != old {
				t.Fatal("mismatch promoted unverified config or failed rollback")
			}
		})
	}
}
