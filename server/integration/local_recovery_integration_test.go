//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// This exercises database recovery through the existing business HTTP seam.
// It verifies business and print state recovery with an explicitly initialized local identity.
// Filesystem images and the Agent printer ledger remain outside this database-only test.
func TestPostgresLocalBusinessRecovery(t *testing.T) {
	source := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, source.dsn)
	t.Cleanup(func() { _ = db.Close() })
	imageDir := t.TempDir()
	var material weighing.Material
	var party weighing.Party
	var savedTicket, postBackupTicket weighing.TicketView
	var savedTask weighing.TaskDetail
	var savedPrints []weighing.PrintAttempt
	var pending weighing.CommandResult
	t.Run("prepare_local_business", func(t *testing.T) {
		router, token, device := recoveryBusinessAPI(t, db, imageDir)
		material = weighingData[weighing.Material](t, router, "POST", "/api/weighing/materials", `{"name":"恢复验证高粱"}`, token)
		party = weighingData[weighing.Party](t, router, "POST", "/api/weighing/parties", `{"name":"恢复验证供方","nature":"enterprise","roles":["supplier"]}`, token)
		savedTicket = recoveryTrip(t, router, token, device, material.ID, party.ID, "川A12345", "saved", true).ticket
		weighingData[weighing.CommandResult](t, router, "POST", fmt.Sprintf("/api/weighing/tickets/%d/print-attempts", savedTicket.ID), `{"requestId":"recovery-print-unknown","intent":"initial"}`, token)
		savedPrints = weighingData[[]weighing.PrintAttempt](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d/print-attempts", savedTicket.ID), "", token)
		if len(savedPrints) != 1 || savedPrints[0].Status != "initiated" || savedPrints[0].ClosedAt != nil {
			t.Fatalf("expected an unresolved print before backup: %+v", savedPrints)
		}
		savedTicket = weighingData[weighing.TicketView](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", savedTicket.ID), "", token)
		savedTask = weighingData[weighing.TaskDetail](t, router, "GET", fmt.Sprintf("/api/weighing/tasks/%d", savedTicket.TaskID), "", token)
		pending = recoveryTrip(t, router, token, device, material.ID, party.ID, "川A12346", "pending", false).first
	})
	if t.Failed() {
		return
	}
	backup := source.backup(t)
	t.Run("business_after_backup", func(t *testing.T) {
		router, token, device := recoveryBusinessAPI(t, db, imageDir)
		postBackupTicket = recoveryTrip(t, router, token, device, material.ID, party.ID, "川A12347", "later", true).ticket
	})
	if t.Failed() {
		return
	}
	restored := startPostgres(t)
	restored.restore(t, backup)
	restoredDB := openTemporaryDatabase(t, restored.dsn)
	t.Cleanup(func() { _ = restoredDB.Close() })
	t.Run("restore_without_automatic_replay", func(t *testing.T) {
		router, token, device := recoveryBusinessAPI(t, restoredDB, imageDir)
		got := weighingData[weighing.TicketView](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", savedTicket.ID), "", token)
		if !reflect.DeepEqual(got.Ticket, savedTicket.Ticket) || got.FinalNet != savedTicket.FinalNet {
			t.Fatal("ticket identity, weights, basis or business snapshot changed across restore")
		}
		task := weighingData[weighing.TaskDetail](t, router, "GET", fmt.Sprintf("/api/weighing/tasks/%d", savedTask.ID), "", token)
		if !reflect.DeepEqual(task, savedTask) {
			t.Fatal("confirmed records, business snapshot or event history changed across restore")
		}
		prints := weighingData[[]weighing.PrintAttempt](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d/print-attempts", savedTicket.ID), "", token)
		if !reflect.DeepEqual(prints, savedPrints) {
			t.Fatalf("unresolved print changed or replayed across restore: %+v", prints)
		}
		awaiting := weighingData[weighing.TaskDetail](t, router, "GET", fmt.Sprintf("/api/weighing/tasks/%d", pending.TaskID), "", token)
		if awaiting.Status != "awaiting_second" || len(awaiting.Records) != 1 {
			t.Fatalf("pending task was automatically completed: %+v", awaiting)
		}
		op := weighingData[weighing.Operation](t, router, "GET", "/api/weighing/operations/saved-second-confirmation", "", token)
		if op.Status != "committed" || op.TicketID == nil || *op.TicketID != savedTicket.ID {
			t.Fatalf("restored request result lost: %+v", op)
		}
		for table, want := range map[string]int64{"wb_transport_task": 2, "wb_ticket": 1, "wb_print_attempt": 1, "wb_weigh_record": 3} {
			var count int64
			if err := restoredDB.GORM.Table(table).Count(&count).Error; err != nil || count != want {
				t.Fatalf("restore replayed %s: count=%d want=%d err=%v", table, count, want, err)
			}
		}
		fresh := recoveryTrip(t, router, token, device, material.ID, party.ID, "川A12347", "restored", true).ticket
		if fresh.ID == postBackupTicket.ID || fresh.TicketNo == postBackupTicket.TicketNo || fresh.TaskID == postBackupTicket.TaskID {
			t.Fatal("restoring an older backup reused a post-backup business identity or ticket number")
		}
		response, err := http.Get(device.control + "/api/state")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var state struct {
			Prints []json.RawMessage `json:"prints"`
		}
		if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
			t.Fatal(err)
		}
		if len(state.Prints) != 0 {
			t.Fatal("restored unresolved print was automatically sent to the Agent")
		}
	})
}

func recoveryBusinessAPI(t *testing.T, db *platformdatabase.Database, imageDir string) (http.Handler, string, *weighingAgent) {
	t.Helper()
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	repo := weighing.NewRepository(db.GORM)
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, id int64) (agentbridge.Config, error) {
		return repo.AgentConfig(ctx, id, imageDir)
	})
	deps.Weighing = weighing.NewService(repo, hub)
	deps.Camera, _, _ = newCaptureService(t, db, t.TempDir())
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	testLocalSetup(t, db, router, token, 1)
	addCaptureCameras(t, db, "rtsp://camera/recovery-1", "rtsp://camera/recovery-2")
	deviceConfig, err := repo.AgentConfig(context.Background(), 1, imageDir)
	if err != nil {
		t.Fatal(err)
	}
	return router, token, startWeighingAgentWithConfig(t, router, hub, deviceConfig)
}

func recoveryTrip(t *testing.T, router http.Handler, token string, device *weighingAgent, materialID, partyID int64, plate, key string, complete bool) (result struct {
	first  weighing.CommandResult
	ticket weighing.TicketView
}) {
	t.Helper()
	encode := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	session := weighingData[weighing.SessionResult](t, router, "POST", "/api/weighing/scales/1/session/takeover", `{"clientId":"recovery-browser","reason":"恢复验证"}`, token)
	input := map[string]any{"scaleId": 1, "clientId": "recovery-browser", "sessionToken": session.SessionToken, "business": map[string]any{"plate": plate, "direction": "inbound", "materialId": materialID, "partyId": partyID}, "checks": map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}}
	device.weight("30")
	sample := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(input), token)
	input["sampleId"] = sample.ID
	input["requestId"] = key + "-first-confirmation"
	result.first = weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/first-confirmation", encode(input), token)
	if !complete {
		return
	}
	device.weight("10")
	delete(input, "business")
	input["taskId"] = result.first.TaskID
	sample = weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(input), token)
	input["sampleId"] = sample.ID
	input["requestId"] = key + "-second-confirmation"
	input["version"] = 1
	second := weighingData[weighing.CommandResult](t, router, "POST", fmt.Sprintf("/api/weighing/tasks/%d/second-confirmation", result.first.TaskID), encode(input), token)
	result.ticket = weighingData[weighing.TicketView](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", second.TicketID), "", token)
	if result.ticket.OriginalGross != 30000 || result.ticket.OriginalTare != 10000 || result.ticket.FinalNet != 20000 || len(result.ticket.Basis) != 2 {
		t.Fatalf("invalid local ticket: %+v", result.ticket)
	}
	return
}
