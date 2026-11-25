//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresPublicLiveWeight(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	repository := weighing.NewRepository(db.GORM)
	imageDir := t.TempDir()
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, scaleCode string) (agentbridge.Config, error) {
		return repository.AgentConfig(ctx, scaleCode, imageDir)
	})
	deps.Weighing = weighing.NewService(repository, hub)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}

	const base = "/api/weighing/scales/WB01"
	const public = base + "/device"
	const detail = base + "/device-detail"
	assertEnvelopeCode(t, serveJSON(router, http.MethodGet, public, "", ""), 404, 404, weighing.ErrNotFound.Error())
	admin := loginAdmin(t, router)
	testLocalSetup(t, db, router, admin, 1)
	assertEnvelopeCode(t, serveJSON(router, http.MethodGet, "/api/weighing/scales/FOREIGN/device", "", ""), 404, 404, weighing.ErrNotFound.Error())
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, detail, "", ""))
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, "/api/weighing/tickets", "", ""))

	read := func(token string) weighing.LiveWeight {
		t.Helper()
		response := serveJSON(router, http.MethodGet, public, "", token)
		assertEnvelopeCode(t, response, http.StatusOK, http.StatusOK, "success")
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control=%q", got)
		}
		var payload weighing.HTTPEnvelope[json.RawMessage]
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		var result weighing.LiveWeight
		if err := json.Unmarshal(payload.Data, &result); err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(payload.Data, &fields); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"scaleCode", "weight", "unit", "online", "deviceOk", "stable", "receivedAt"} {
			if _, ok := fields[field]; !ok {
				t.Fatalf("missing public field %q: %s", field, payload.Data)
			}
			delete(fields, field)
		}
		if len(fields) != 0 {
			t.Fatalf("unexpected public fields: %s", payload.Data)
		}
		return result
	}

	if got := read(""); got.ScaleCode != "WB01" || got.Unit != "t" || got.Online || got.DeviceOK || got.Stable || got.Weight != nil || got.ReceivedAt != nil {
		t.Fatalf("unexpected initial state: %+v", got)
	}
	cfg, err := repository.AgentConfig(context.Background(), "WB01", imageDir)
	if err != nil {
		t.Fatal(err)
	}
	agent := startWeighingAgentWithConfig(t, router, hub, cfg)
	agent.plate("川A12345")
	if got := read("Bearer expired-or-unrelated-token"); !got.Online || !got.DeviceOK || !got.Stable || got.Weight == nil || *got.Weight != "30" || got.ReceivedAt == nil {
		t.Fatalf("unexpected live state: %+v", got)
	}
	if got := weighingData[agentbridge.State](t, router, http.MethodGet, detail, "", admin); got.Plate.Candidate != "川A12345" || got.ConnectionID == "" {
		t.Fatalf("unexpected authorized device details: %+v", got)
	}
	agent.controlAction(`{"action":"unstable"}`)
	agent.wait(func(state agentbridge.State) bool { return state.Online && state.DeviceOK && !state.Stable })
	if got := read(""); !got.Online || !got.DeviceOK || got.Stable || got.Weight == nil {
		t.Fatalf("unexpected dynamic state: %+v", got)
	}
	agent.controlAction(`{"action":"fault"}`)
	agent.wait(func(state agentbridge.State) bool { return state.Online && !state.DeviceOK })
	if got := read(""); !got.Online || got.DeviceOK || got.Stable || got.Weight != nil || got.ReceivedAt == nil {
		t.Fatalf("unexpected fault state: %+v", got)
	}
	agent.controlAction(`{"action":"disconnect"}`)
	agent.wait(func(state agentbridge.State) bool { return !state.Online })
	if got := read(""); got.Online || got.DeviceOK || got.Stable || got.Weight != nil {
		t.Fatalf("unexpected disconnected state: %+v", got)
	}
}
