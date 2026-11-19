//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresTicketSyncPausesOldSiteChanges(t *testing.T) {
	pg := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, pg.dsn)
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	repo := weighing.NewRepository(db.GORM)
	service := weighing.NewService(repo, nil)
	deps := testDependencies(t, db, t.TempDir())
	deps.Weighing = service
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	testLocalSetup(t, db, router, admin, 1, "FCC01", "原厂区", "WB01", "地磅", "integration-agent")
	material := weighingData[weighing.Material](t, router, "POST", "/api/weighing/materials", `{"name":"高粱","spec":"一级"}`, admin)
	party := weighingData[weighing.Party](t, router, "POST", "/api/weighing/parties", `{"name":"供收方","nature":"enterprise","roles":["supplier"]}`, admin)
	first := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	issue := func(n int) weighing.TicketView {
		t.Helper()
		body := map[string]any{
			"requestId":   fmt.Sprintf("sync-site-change-%d", n),
			"business":    map[string]any{"plate": fmt.Sprintf("川A%05d", n), "direction": "inbound", "materialId": material.ID, "partyId": party.ID},
			"firstWeight": "30", "secondWeight": "10", "firstWeighedAt": first, "secondWeighedAt": first.Add(time.Hour), "reason": "power_failure",
		}
		result := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/manual-supplements", encodeJSON(t, body), admin)
		return weighingData[weighing.TicketView](t, router, "GET", fmt.Sprintf("/api/weighing/tickets/%d", result.TicketID), "", admin)
	}
	execSQL := func(query string, args ...any) {
		t.Helper()
		if err := db.GORM.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Prepare actual historical outbox rows for tickets created before the old
	// protocols stopped being used for fresh issues.
	legacy := func(ticket weighing.TicketView, status string) string {
		t.Helper()
		payload, err := json.Marshal(map[string]any{"schemaVersion": 3, "changeId": ticket.TicketNo, "ticketNo": ticket.TicketNo, "siteCode": ticket.SiteCode, "type": "issue"})
		if err != nil {
			t.Fatal(err)
		}
		execSQL("DELETE FROM wb_ticket_sync_change_v5 WHERE ticket_id = ?", ticket.ID)
		execSQL("INSERT INTO wb_ticket_sync_change (ticket_id, change_id, ticket_no, change_type, payload, status) VALUES (?, ?, ?, 'issue', ?, ?)", ticket.ID, ticket.TicketNo, ticket.TicketNo, string(payload), status)
		return string(payload)
	}
	oldLegacy := issue(1)
	oldLegacyPayload := legacy(oldLegacy, "failed")
	oldV4 := issue(2)
	execSQL(`INSERT INTO wb_ticket_sync_change_v4 (ticket_id, digest, digest_version, schema_version, site_code, ticket_no, change_type, payload)
		SELECT ticket_id, digest, 2, 4, site_code, ticket_no, change_type, payload FROM wb_ticket_sync_change_v5 WHERE ticket_id = ?`, oldV4.ID)
	execSQL("DELETE FROM wb_ticket_sync_change_v5 WHERE ticket_id = ?", oldV4.ID)
	old := issue(3)
	voided := weighingData[weighing.CommandResult](t, router, "POST", fmt.Sprintf("/api/weighing/tickets/%d/void", old.ID), `{"requestId":"void-before-code-change","reason":"更正测试"}`, admin)
	oldRows := readSyncV4Rows(t, db.GORM)

	config := weighingData[[]weighing.SiteConfiguration](t, router, "GET", "/api/weighing/site-config", "", admin)[0]
	update := weighing.SiteConfigurationInput{Code: "FCC02", Name: config.Name, ScaleCode: config.ScaleCode, ScaleName: config.ScaleName, Enabled: config.Enabled, Version: config.Version}
	weighingData[weighing.SiteConfiguration](t, router, "PUT", "/api/weighing/site-config", encodeJSON(t, update), admin)
	reopened := weighingData[weighing.CommandResult](t, router, "POST", fmt.Sprintf("/api/weighing/tickets/%d/reopen", old.ID), encodeJSON(t, map[string]any{
		"requestId": "reopen-after-code-change", "taskVersion": voided.TaskVersion, "reason": "更正重开", "basisValid": true, "business": old.Business.BusinessInput,
	}), admin)
	current := issue(4)
	rejected := issue(5)
	rejectedPayload := legacy(rejected, "pending")

	var rejectedCalls, unexpectedCalls atomic.Int32
	cloud := &fakeSyncV4Cloud{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/weighbridge/sync/changes" {
			var change struct{ TicketNo string }
			if err := json.NewDecoder(r.Body).Decode(&change); err != nil || change.TicketNo != rejected.TicketNo {
				unexpectedCalls.Add(1)
			} else {
				rejectedCalls.Add(1)
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		cloud.handler(w, r)
	}))
	defer remote.Close()
	if err := service.ConfigureMasterSync(ctx, true, remote.URL); err != nil {
		t.Fatal(err)
	}
	execSQL("UPDATE wb_master_sync_state SET bound=true, last_success=now() WHERE id=1")
	for i := 0; i < 2; i++ {
		for _, run := range []func(context.Context) error{service.RunTicketSync, service.RunSyncV4, service.RunSyncV5} {
			if err := run(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	if unexpectedCalls.Load() != 0 || rejectedCalls.Load() != 1 || cloud.count() != 1 || !strings.Contains(cloud.last(), current.TicketNo) {
		t.Fatalf("unexpected uploads: old legacy=%d, rejected legacy=%d, digest=%d", unexpectedCalls.Load(), rejectedCalls.Load(), cloud.count())
	}

	for ticketID, originalPayload := range map[int64]string{oldLegacy.ID: oldLegacyPayload, rejected.ID: rejectedPayload} {
		var row struct{ Status, Reason, Payload string }
		if err := db.GORM.Table("wb_ticket_sync_change").Where("ticket_id = ?", ticketID).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		if row.Status != "needs_admin" || row.Payload != originalPayload || row.Reason == "" {
			t.Fatalf("legacy change was not retained for review: %+v", row)
		}
	}
	rows := readSyncV4Rows(t, db.GORM)
	for i, before := range oldRows {
		after := rows[i]
		if after.Status != "needs_admin" || after.ReasonCode != "old_site_code" || after.Attempts != before.Attempts || after.SiteCode != "FCC01" || after.Digest != before.Digest || after.Payload != before.Payload {
			t.Fatalf("historical change was changed or sent: %+v", after)
		}
	}
	detail, err := service.SyncTicketDetail(ctx, reopened.TicketID)
	if err != nil || len(detail.Changes) != 1 || detail.Changes[0].Status == "synced" || detail.Changes[0].Attempts != 0 {
		t.Fatalf("reopen must wait for original void confirmation: %+v, %v", detail, err)
	}
	overview, err := service.SyncOverview(ctx)
	if err != nil || overview.NeedsActionTickets != 4 {
		t.Fatalf("paused tickets should be counted for administrators: %+v, %v", overview, err)
	}
	if _, err := service.RetrySyncChange(ctx, oldRows[1].ID, audit.Event{}); err == nil || !strings.Contains(err.Error(), "已暂停上传") {
		t.Fatalf("manual retry should explain the old code: %v", err)
	}
	result, err := service.ReconcileSyncTicket(ctx, old.ID, "manual")
	if err != nil || result.Status != "needs_review" || result.ReasonCode != "old_site_code" {
		t.Fatalf("old-code reconciliation must not schedule an upload: %+v, %v", result, err)
	}
	if after := readSyncV4Rows(t, db.GORM)[1]; after.Status != "needs_admin" {
		t.Fatal("read-only reconciliation requeued a paused change")
	}
}
