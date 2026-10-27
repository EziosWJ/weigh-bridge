package agentmaintenance

import (
	"context"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
)

func TestRemoteDoesNotInventDeviceConfigurationWhileAgentOffline(t *testing.T) {
	hub := agentbridge.New(agentbridge.Config{ID: "agent", ScaleCode: "WB01", Key: "key"})
	remote := NewRemote(hub, func(context.Context) (Config, error) { c := Defaults(); c.AgentID = "agent"; return c, nil }, nil)
	if _, err := remote.CameraConfig(); err == nil {
		t.Fatal("offline device returned an invented active config")
	}
	if remote.CameraAvailable("WB01") {
		t.Fatal("offline Agent permitted capture")
	}
}
