//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresManualSupplementIssueAndReplay(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer func() { _ = db.Close() }()
	deps := testDependencies(t, db, t.TempDir())
	deps.Weighing = weighing.NewService(weighing.NewRepository(db.GORM), nil)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	testLocalSetup(t, db, router, admin, 1, "SITE01", "原厂区", "SCALE01", "一号地磅", "integration-agent")
	material := weighingData[weighing.Material](t, router, "POST", "/api/weighing/materials", `{"name":"高粱","spec":"一级"}`, admin)
	party := weighingData[weighing.Party](t, router, "POST", "/api/weighing/parties", `{"name":"供收方","nature":"enterprise","roles":["supplier","customer"]}`, admin)
	for _, direction := range []string{"inbound", "outbound"} {
		t.Run(direction, func(t *testing.T) {
			first, second := "30", "10"
			if direction == "outbound" {
				first, second = "10", "30"
			}
			actualFirst := time.Now().UTC().Add(-26 * time.Hour).Truncate(time.Second)
			actualSecond := actualFirst.Add(time.Hour)
			body := map[string]any{"requestId": "manual-supplement-" + direction, "business": map[string]any{"plate": map[string]string{"inbound": "川A12345", "outbound": "川B12345"}[direction], "direction": direction, "materialId": material.ID, "partyId": party.ID}, "firstWeight": first, "secondWeight": second, "firstWeighedAt": actualFirst, "secondWeighedAt": actualSecond, "reason": "power_failure"}
			result := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/manual-supplements", encodeJSON(t, body), admin)
			replay := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/manual-supplements", encodeJSON(t, body), admin)
			if result.TicketID == 0 || result.TicketID != replay.TicketID {
				t.Fatalf("idempotency: %+v %+v", result, replay)
			}
			ticket := weighingData[weighing.TicketView](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", result.TicketID), "", admin)
			if ticket.EntryType != "manual_supplement" || ticket.GrossSource != "manual" || ticket.TareSource != "manual" || len(ticket.Basis) != 2 || !ticket.Basis[0].CapturedAt.Equal(actualFirst) || !ticket.Basis[1].CapturedAt.Equal(actualSecond) {
				t.Fatalf("ticket: %+v", ticket)
			}
			var raw string
			if err := db.GORM.Raw("SELECT payload::text FROM wb_ticket_sync_change_v5 WHERE ticket_id=?", result.TicketID).Scan(&raw).Error; err != nil {
				t.Fatal(err)
			}
			var payload struct {
				SchemaVersion int `json:"schemaVersion"`
				DigestVersion int `json:"digestVersion"`
				Ticket        struct {
					EntryType string    `json:"entryType"`
					First     time.Time `json:"firstWeighedAt"`
					Second    time.Time `json:"secondWeighedAt"`
				} `json:"ticket"`
			}
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.SchemaVersion != 6 || payload.DigestVersion != 4 || payload.Ticket.EntryType != "manual_supplement" || !payload.Ticket.First.Equal(actualFirst) || !payload.Ticket.Second.Equal(actualSecond) {
				t.Fatalf("payload: %s", raw)
			}
			var count int64
			if err := db.GORM.Table("wb_business_event").Where("ticket_id=? AND action=? AND reason=?", result.TicketID, "ticket.manual-supplement", "power_failure").Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("audit %d %v", count, err)
			}
			void := weighingData[weighing.CommandResult](t, router, "POST", fmt.Sprintf("/api/weighing/tickets/%d/void", result.TicketID), encodeJSON(t, map[string]any{"requestId": "manual-void-" + direction, "reason": "更正"}), admin)
			if void.TicketID != result.TicketID {
				t.Fatalf("void %+v", void)
			}
			reopen := weighingData[weighing.CommandResult](t, router, "POST", fmt.Sprintf("/api/weighing/tickets/%d/reopen", result.TicketID), encodeJSON(t, map[string]any{"requestId": "manual-reopen-" + direction, "taskVersion": void.TaskVersion, "reason": "更正重开", "basisValid": true, "business": ticket.Business.BusinessInput}), admin)
			if reopen.TicketID == 0 || reopen.TicketID == result.TicketID {
				t.Fatalf("reopen %+v", reopen)
			}
			var reopenedPayload string
			if err := db.GORM.Raw("SELECT payload::text FROM wb_ticket_sync_change_v5 WHERE ticket_id=?", reopen.TicketID).Scan(&reopenedPayload).Error; err != nil {
				t.Fatal(err)
			}
			var reopened struct {
				SchemaVersion int    `json:"schemaVersion"`
				Type          string `json:"type"`
				Ticket        struct {
					EntryType string `json:"entryType"`
				} `json:"ticket"`
			}
			if err := json.Unmarshal([]byte(reopenedPayload), &reopened); err != nil {
				t.Fatal(err)
			}
			if reopened.SchemaVersion != 6 || reopened.Type != "reopen" || reopened.Ticket.EntryType != "manual_supplement" {
				t.Fatalf("reopen payload %s", reopenedPayload)
			}
			var versions []int
			if err := db.GORM.Raw("SELECT schema_version FROM wb_ticket_sync_change_v5 WHERE ticket_id=? ORDER BY id", result.TicketID).Scan(&versions).Error; err != nil || fmt.Sprint(versions) != "[6 6]" {
				t.Fatalf("versions %v %v", versions, err)
			}
		})
	}
}
