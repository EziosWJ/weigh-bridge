package agentbridge

import (
	"context"
	"testing"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

func TestConfigReportsAreRecordedAndPublished(t *testing.T) {
	hub := New(Config{ID: "agent", ScaleCode: "WB01"})
	conn := &connection{id: "connection", pending: map[string]chan p.Message{}, prints: map[string]int64{}, capabilities: map[string]bool{}}
	hub.mu.Lock()
	hub.conn = conn
	hub.mu.Unlock()

	reported := make(chan struct{}, 2)
	hub.SetConfigStateHandler(func(_ context.Context, scaleCode string, effective *p.EffectiveConfig, backup *p.ConfigBackup) {
		if scaleCode != "WB01" || (effective == nil && backup == nil) {
			t.Errorf("unexpected report scale=%s effective=%v backup=%v", scaleCode, effective, backup)
		}
		reported <- struct{}{}
	})
	effective := p.EffectiveConfig{ConfigVersion: 3, Digest: "abc", DeviceState: "running", AppliedAt: time.Now().UTC()}
	if err := hub.receive(conn, p.NewVersion(p.ProtocolVersion, p.TypeAgentConfigState, "m1", "", effective)); err != nil {
		t.Fatal(err)
	}
	backup := p.ConfigBackup{ConfigVersion: 3, Digest: "abc", AppliedAt: time.Now().UTC()}
	if err := hub.receive(conn, p.NewVersion(p.ProtocolVersion, p.TypeAgentConfigBackup, "m2", "", backup)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-reported:
		case <-time.After(time.Second):
			t.Fatal("config report callback was not invoked")
		}
	}
	stored, ok := hub.EffectiveConfig("WB01")
	if !ok || stored.ConfigVersion != 3 || stored.Digest != "abc" {
		t.Fatalf("effective config not retained: %+v", stored)
	}
	storedBackup, ok := hub.ConfigBackup("WB01")
	if !ok || storedBackup.ConfigVersion != 3 {
		t.Fatalf("config backup not retained: %+v", storedBackup)
	}
}
