package agentmaintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDeviceRePairingPreservesPendingParametersAndCurrentIdentity(t *testing.T) {
	dir := t.TempDir()
	environment := filepath.Join(dir, "agent.env")
	old := Defaults()
	old.SiteCode = "LOCAL"
	old.ScaleCode = "SCALE"
	old.AgentKey = "old-key"
	old.Queue = "active-queue"
	draft := old
	draft.SerialPort = "/dev/pending-serial"
	draft.OffsetXMM = 7
	draft.Queue = "pending-queue"
	previous := old
	previous.OffsetXMM = 3
	saved, _ := json.Marshal(map[string]any{"draft": draft, "active": old, "previous": previous})
	if err := os.WriteFile(filepath.Join(dir, "maintenance.json"), saved, 0600); err != nil {
		t.Fatal(err)
	}
	current := old
	current.AgentKey = "new-pairing-key"
	current.ServerURL = "http://new-api.local:8900"
	if err := BuildAgentEnvironmentAt(environment, current); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(environment)
	service, err := OpenDevice(dir, environment, current, &controllableAgentRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if service.draft.SerialPort != draft.SerialPort || service.draft.OffsetXMM != draft.OffsetXMM || service.draft.Queue != draft.Queue {
		t.Fatal("re-pairing discarded pending device parameters")
	}
	if service.previous.OffsetXMM != previous.OffsetXMM {
		t.Fatal("re-pairing discarded rollback parameters")
	}
	for _, cfg := range []Config{service.draft, service.previous} {
		if cfg.AgentKey != current.AgentKey || cfg.ServerURL != current.ServerURL || cfg.ScaleCode != current.ScaleCode {
			t.Fatal("draft or rollback would restore an old connection identity")
		}
	}
	after, _ := os.ReadFile(environment)
	if string(before) != string(after) {
		t.Fatal("opening paired maintenance changed the effective device environment")
	}
	reopened, err := OpenDevice(dir, environment, current, &controllableAgentRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.draft != service.draft || reopened.previous != service.previous {
		t.Fatal("re-paired drafts were not persisted across restart")
	}
}
