//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
	"gorm.io/gorm"
)

type fakeSyncV4Cloud struct {
	mu             sync.Mutex
	payloads       []string
	mode           string
	ticketResponse string
	batchDigest    string
	batchTicketNo  string
	batchStatus    string
	lookupResponse string
}

func (f *fakeSyncV4Cloud) handler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "" {
		http.Error(w, "unexpected credential", http.StatusBadRequest)
		return
	}
	switch {
	case r.URL.Path == "/api/v1/weighbridge/sync/v4/changes" && r.Method == http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.payloads = append(f.payloads, string(body))
		mode := f.mode
		f.mu.Unlock()
		var change struct {
			Digest        string `json:"digest"`
			TicketNo      string `json:"ticketNo"`
			Type          string `json:"type"`
			SchemaVersion int    `json:"schemaVersion"`
			DigestVersion int    `json:"digestVersion"`
		}
		if json.Unmarshal(body, &change) != nil || len(change.Digest) != 64 {
			http.Error(w, "invalid change", http.StatusBadRequest)
			return
		}
		if change.SchemaVersion == 5 && change.DigestVersion != 3 {
			http.Error(w, "invalid v5 change", http.StatusBadRequest)
			return
		}
		if change.SchemaVersion == 4 && change.DigestVersion != 2 {
			http.Error(w, "invalid v4 change", http.StatusBadRequest)
			return
		}
		if change.SchemaVersion != 4 && change.SchemaVersion != 5 {
			http.Error(w, "unsupported schema", http.StatusBadRequest)
			return
		}
		switch mode {
		case "unknown":
			fmt.Fprint(w, `{"code":200,"data":null}`)
		case "config":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"code":400,"message":"厂区编码不存在或已停用","data":{"category":"config_or_data","reason":"site_not_found_or_disabled","retryable":false}}`)
		case "predecessor":
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"code":409,"message":"等待前序确认","data":{"category":"predecessor_pending","reason":"original_not_void","retryable":true}}`)
		case "conflict":
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"code":409,"message":"变化冲突","data":{"category":"conflict","reason":"change_conflict","retryable":false}}`)
		default:
			fmt.Fprintf(w, `{"code":200,"message":"success","data":{"digest":%q,"ticketNo":%q,"changeType":%q,"receivedAt":%q,"duplicate":false}}`, change.Digest, change.TicketNo, change.Type, time.Now().UTC().Format(time.RFC3339Nano))
		}
	case r.URL.Path == "/api/v1/weighbridge/sync/v4/changes" && r.Method == http.MethodGet:
		f.mu.Lock()
		response := f.lookupResponse
		f.mu.Unlock()
		if response == "" {
			response = fmt.Sprintf(`{"found":false,"siteCode":%q,"digest":%q,"confirmation":null}`, r.URL.Query().Get("siteCode"), r.URL.Query().Get("digest"))
		}
		fmt.Fprintf(w, `{"code":200,"message":"success","data":%s}`, response)
	case r.URL.Path == "/api/v1/weighbridge/sync/v4/tickets":
		f.mu.Lock()
		response := f.ticketResponse
		payloads := append([]string(nil), f.payloads...)
		f.mu.Unlock()
		if response == "" {
			response = fmt.Sprintf(`{"found":false,"siteCode":%q,"ticketNo":%q}`, r.URL.Query().Get("siteCode"), r.URL.Query().Get("ticketNo"))
		} else {
			var result map[string]any
			if json.Unmarshal([]byte(response), &result) == nil && result["found"] == true {
				for i := len(payloads) - 1; i >= 0; i-- {
					var change struct {
						TicketNo         string          `json:"ticketNo"`
						PreviousSiteCode string          `json:"previousSiteCode"`
						SchemaVersion    int             `json:"schemaVersion"`
						DigestVersion    int             `json:"digestVersion"`
						Ticket           json.RawMessage `json:"ticket"`
						Void             *struct {
							Reason string `json:"reason"`
							Actor  string `json:"actor"`
							At     string `json:"at"`
						} `json:"void"`
					}
					if json.Unmarshal([]byte(payloads[i]), &change) != nil || change.TicketNo != r.URL.Query().Get("ticketNo") || len(change.Ticket) == 0 {
						continue
					}
					result["payloadSchemaVersion"] = change.SchemaVersion
					result["payloadDigestVersion"] = change.DigestVersion
					if result["status"] == "void" && change.Void == nil {
						continue
					}
					result["ticket"] = change.Ticket
					if change.PreviousSiteCode != "" {
						result["previousSiteCode"] = change.PreviousSiteCode
					}
					if change.Void != nil && result["status"] == "void" {
						result["voidReason"], result["voidedBy"], result["voidedAt"] = change.Void.Reason, change.Void.Actor, change.Void.At
					}
					break
				}
				// Real cloud snapshots carry the latest receipt in their history.
				// Keep the simulated conflict receipt complete as well.
				if digest, ok := result["latestDigest"].(string); ok {
					changes, _ := result["changes"].([]any)
					found := false
					for _, item := range changes {
						if receipt, ok := item.(map[string]any); ok && receipt["digest"] == digest {
							result["latestReceivedAt"] = receipt["receivedAt"]
							found = true
						}
					}
					if !found {
						schema := 4
						if value, ok := result["payloadSchemaVersion"].(float64); ok && value > 0 {
							schema = int(value)
						}
						changes = append(changes, map[string]any{"digest": digest, "ticketNo": result["ticketNo"], "changeType": result["latestChangeType"], "digestVersion": result["latestDigestVersion"], "schemaVersion": schema, "receivedAt": result["latestReceivedAt"]})
						result["changes"] = changes
					}
				}
				data, _ := json.Marshal(result)
				response = string(data)
			}
		}
		fmt.Fprintf(w, `{"code":200,"message":"success","data":%s}`, response)
	case r.URL.Path == "/api/v1/weighbridge/sync/v4/reconcile":
		var input struct {
			Items []struct {
				SiteCode string `json:"siteCode"`
				TicketNo string `json:"ticketNo"`
			} `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		f.mu.Lock()
		digest, ticketNo, status := f.batchDigest, f.batchTicketNo, f.batchStatus
		f.mu.Unlock()
		items := make([]string, 0, len(input.Items))
		for _, item := range input.Items {
			if digest != "" && item.TicketNo == ticketNo {
				items = append(items, fmt.Sprintf(`{"siteCode":%q,"ticketNo":%q,"found":true,"status":%q,"latestDigest":%q,"latestDigestVersion":3,"latestChangeType":"void","latestReceivedAt":"%s","error":null}`, item.SiteCode, item.TicketNo, status, digest, time.Now().UTC().Format(time.RFC3339Nano)))
				continue
			}
			items = append(items, fmt.Sprintf(`{"siteCode":%q,"ticketNo":%q,"found":false,"latestDigest":null,"latestDigestVersion":null,"latestChangeType":null,"latestReceivedAt":null,"previousSiteCode":null,"previousTicketNo":null,"reopenedBy":null,"error":null}`, item.SiteCode, item.TicketNo))
		}
		fmt.Fprintf(w, `{"code":200,"message":"success","data":{"items":[%s]}}`, strings.Join(items, ","))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeSyncV4Cloud) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.payloads)
}

func (f *fakeSyncV4Cloud) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.payloads) == 0 {
		return ""
	}
	return f.payloads[len(f.payloads)-1]
}

type syncV4Row struct {
	ID            int64
	TicketID      int64
	Digest        string
	DigestVersion int
	SchemaVersion int
	SiteCode      string
	TicketNo      string
	ChangeType    string
	Payload       string
	Status        string
	Category      string
	ReasonCode    string
	Reason        string
	Attempts      int
	SyncedAt      *time.Time
	ReceivedAt    *time.Time
}

// readSyncV4Rows lists every digest-protocol outbox row across v4 and v5.
// Fresh tickets enqueue as v5; v4 rows come from pre-existing protocol state.
// Rows keep insertion order by table then id, which matches change order
// because one ticket never spans two digest protocols.

// bumpSyncAttempt touches the outbox row in whichever protocol table holds it.
func bumpSyncAttempt(db *gorm.DB, sql string, args ...any) {
	db.Exec(strings.Replace(sql, "wb_ticket_sync_change_v4", "wb_ticket_sync_change_v5", 1), args...)
	db.Exec(sql, args...)
}
func readSyncV4Rows(t *testing.T, db *gorm.DB) []syncV4Row {
	t.Helper()
	rows := []syncV4Row{}
	if err := db.Table("wb_ticket_sync_change_v4").Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	v5rows := []syncV4Row{}
	if err := db.Table("wb_ticket_sync_change_v5").Order("id").Find(&v5rows).Error; err != nil {
		t.Fatal(err)
	}
	return append(rows, v5rows...)
}

// Exercises the actual business transactions constructed by the workflow test.
func checkTicketSyncOutbox(t *testing.T, db *gorm.DB, oldID, newID int64) {
	t.Helper()
	rows := readSyncV4Rows(t, db)
	if len(rows) != 3 || rows[0].ChangeType != "issue" || rows[1].ChangeType != "void" || rows[2].ChangeType != "reopen" || rows[0].TicketID != oldID || rows[2].TicketID != newID {
		t.Fatalf("outbox sequence: %+v", rows)
	}
	if rows[0].DigestVersion != 3 || rows[0].SchemaVersion != 5 || len(rows[0].Digest) != 64 || rows[0].Status != "pending" {
		t.Fatalf("outbox identity: %+v", rows[0])
	}
	var issue, reopen map[string]any
	if json.Unmarshal([]byte(rows[0].Payload), &issue) != nil || json.Unmarshal([]byte(rows[2].Payload), &reopen) != nil {
		t.Fatal("invalid outbox payload")
	}
	if issue["digest"] != rows[0].Digest {
		t.Fatal("payload digest differs from stored identity")
	}
	ticket := issue["ticket"].(map[string]any)
	if issue["siteCode"] != "SITE01" || ticket["partyName"] != "首磅供收方" || ticket["originalGross"] != "30" || ticket["finalGross"] != "30" || ticket["finalTare"] != "10" || ticket["finalNet"] != "18.5" || ticket["netAdjustment"] != "1.5" {
		t.Fatalf("snapshot mapping: %s", rows[0].Payload)
	}
	// v5 carries the detail list; the legacy single-item fields are no
	// longer the authority for the whole ticket.
	items, _ := ticket["netAdjustments"].([]any)
	if len(items) != 1 {
		t.Fatalf("netAdjustments detail: %s", rows[0].Payload)
	}
	item, _ := items[0].(map[string]any)
	if item["mode"] != "amount" || item["amount"] != "1.5" || item["category"] != "WATER" || item["categoryName"] != "水分大" {
		t.Fatalf("netAdjustments item: %s", rows[0].Payload)
	}
	if reopen["siteCode"] != "F001" || reopen["previousSiteCode"] != issue["siteCode"] || reopen["ticket"].(map[string]any)["previousTicketNo"] != issue["ticketNo"] {
		t.Fatalf("reopen association: %s", rows[2].Payload)
	}

	cloud := &fakeSyncV4Cloud{mode: "unknown"}
	remote := httptest.NewServer(http.HandlerFunc(cloud.handler))
	defer remote.Close()
	ctx := context.Background()
	service := weighing.NewService(weighing.NewRepository(db), nil)
	if err := service.ConfigureMasterSync(ctx, true, remote.URL); err != nil {
		t.Fatal(err)
	}
	// The digest upload path only starts after the first successful master
	// sync; this test focuses on the change protocol and marks that state
	// directly instead of exercising the separate master endpoint.
	if err := db.Exec("UPDATE wb_master_sync_state SET bound=true, last_success=now() WHERE id=1").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV4(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV5(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := service.Tickets(ctx, weighing.PageQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, ticket := range page.Records {
		if ticket.ID == oldID && ticket.SyncStatus != "pending" {
			t.Fatalf("latest void should remain pending: %+v", ticket)
		}
	}
	rows = readSyncV4Rows(t, db)
	if cloud.count() != 1 || rows[0].Status != "temporary_failure" || rows[1].Status != "pending" || rows[2].Status != "pending" {
		t.Fatalf("failure order: %+v payloads=%d", rows, cloud.count())
	}
	firstPayload := rows[0].Payload

	cloud.mu.Lock()
	cloud.mode = "config"
	cloud.mu.Unlock()
	// A config/data error is not retried automatically; an administrator may
	// retry it explicitly after fixing the configuration.
	bumpSyncAttempt(db, "UPDATE wb_ticket_sync_change_v4 SET next_attempt_at=now() WHERE id=?", rows[0].ID)
	if err := service.RunSyncV4(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV5(ctx); err != nil {
		t.Fatal(err)
	}
	rows = readSyncV4Rows(t, db)
	if rows[0].Status != "needs_admin" || !strings.Contains(rows[0].Reason, "厂区编码") {
		t.Fatalf("config rejection: %+v", rows[0])
	}
	if _, err := service.RetrySyncChange(ctx, rows[0].ID, audit.Event{}); err != nil {
		t.Fatalf("manual retry: %v", err)
	}

	// An unavailable endpoint leaves the original payload and digest intact.
	offline := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	offlineURL := offline.URL
	offline.Close()
	if err := service.ConfigureMasterSync(ctx, true, offlineURL); err != nil {
		t.Fatal(err)
	}
	bumpSyncAttempt(db, "UPDATE wb_ticket_sync_change_v4 SET next_attempt_at=now() WHERE ticket_id IN (?,?)", oldID, newID)
	if err := service.RunSyncV4(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV5(ctx); err != nil {
		t.Fatal(err)
	}
	rows = readSyncV4Rows(t, db)
	if rows[0].Status != "temporary_failure" || rows[0].Payload != firstPayload || rows[0].Digest != readSyncV4Rows(t, db)[0].Digest {
		t.Fatal("offline attempt changed the queued change")
	}
	if err := db.Exec("DELETE FROM wb_master_sync_log").Error; err != nil {
		t.Fatal(err)
	}
	rows = readSyncV4Rows(t, db)
	if len(rows) != 3 || rows[1].Status != "pending" {
		t.Fatal("clearing logs changed the outbox")
	}

	// Retry every change with the original payload/digest until confirmed.
	cloud.mu.Lock()
	cloud.mode = ""
	cloud.mu.Unlock()
	if err := service.ConfigureMasterSync(ctx, true, remote.URL); err != nil {
		t.Fatal(err)
	}
	bumpSyncAttempt(db, "UPDATE wb_ticket_sync_change_v4 SET status='pending', next_attempt_at=now(), attempts=0 WHERE status <> 'synced'")
	for i := 0; i < 5; i++ {
		if err := service.RunSyncV4(ctx); err != nil {
			t.Fatal(err)
		}
		if err := service.RunSyncV5(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rows = readSyncV4Rows(t, db)
	for _, row := range rows {
		if row.Status != "synced" || row.SyncedAt == nil || row.ReceivedAt == nil {
			t.Fatalf("confirmed status: %+v", row)
		}
	}
	if payloads := cloud.count(); payloads != 5 {
		t.Fatalf("expected retries to reuse stored payloads, got %d", payloads)
	}
	page, err = service.Tickets(ctx, weighing.PageQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, ticket := range page.Records {
		if (ticket.ID == oldID || ticket.ID == newID) && (ticket.SyncStatus != "synced" || ticket.SyncProtocol != 5) {
			t.Fatalf("confirmed status: %+v", ticket)
		}
	}

	// Switching sync off preserves the queue and stops upload and automatic
	// reconciliation. Re-enabling alone does not resume before a successful
	// master sync.
	if err := service.ConfigureMasterSync(ctx, false, remote.URL); err != nil {
		t.Fatal(err)
	}
	if err := (func() error {
		bumpSyncAttempt(db, "UPDATE wb_ticket_sync_change_v4 SET status='pending', synced_at=NULL, received_at=NULL, next_attempt_at=now() WHERE id=?", rows[0].ID)
		return nil
	})(); err != nil {
		t.Fatal(err)
	}
	beforeSwitch := cloud.count()
	if err := service.RunSyncV4(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV5(ctx); err != nil {
		t.Fatal(err)
	}
	if cloud.count() != beforeSwitch {
		t.Fatal("disabled sync uploaded a queued change")
	}
	if _, err := service.ReconcileSyncRange(ctx, "", "", "automatic"); err == nil {
		t.Fatal("disabled sync ran reconciliation")
	}
	if err := service.ConfigureMasterSync(ctx, true, remote.URL); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV4(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV5(ctx); err != nil {
		t.Fatal(err)
	}
	if cloud.count() != beforeSwitch {
		t.Fatal("upload resumed before the first master sync")
	}
	if err := db.Exec("UPDATE wb_master_sync_state SET bound=true, last_success=now() WHERE id=1").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV4(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV5(ctx); err != nil {
		t.Fatal(err)
	}
	rows = readSyncV4Rows(t, db)
	if rows[0].Status != "synced" {
		t.Fatalf("queued change did not resume after re-enable: %+v", rows[0])
	}

	checkSyncV4Reconciliation(t, db, service, cloud, remote.URL, oldID)
	if os.Getenv("WB_SYNC_V4_TEST_URL") == "" {
		checkReopenedVoid(t, db, remote.URL, newID, false)
	}
	checkLiveSyncV4(t, db, service, oldID, newID)
}

// checkLiveSyncV4 runs the same outbox against an isolated real cloud service
// when WB_SYNC_V4_TEST_URL is supplied. It is skipped in ordinary test runs.
func checkLiveSyncV4(t *testing.T, db *gorm.DB, service *weighing.Service, oldID, newID int64) {
	t.Helper()
	live := os.Getenv("WB_SYNC_V4_TEST_URL")
	if live == "" {
		return
	}
	ctx := context.Background()
	if err := service.ConfigureMasterSync(ctx, true, live); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE wb_master_sync_state SET bound=true, last_success=now() WHERE id=1").Error; err != nil {
		t.Fatal(err)
	}
	bumpSyncAttempt(db, "UPDATE wb_ticket_sync_change_v4 SET status='pending', category='', reason_code='', reason='', attempts=0, next_attempt_at=now(), synced_at=NULL, received_at=NULL")
	if err := db.Exec("SELECT 1").Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := service.RunSyncV4(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rows := readSyncV4Rows(t, db)
	for _, row := range rows {
		if row.Status != "synced" || row.SyncedAt == nil || row.ReceivedAt == nil {
			t.Fatalf("live cloud did not confirm change: %+v", row)
		}
	}
	for _, ticketID := range []int64{oldID, newID} {
		result, err := service.ReconcileSyncTicket(ctx, ticketID, "manual")
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "consistent" {
			t.Fatalf("live cloud reconcile ticket %d: %+v", ticketID, result)
		}
	}
	// A repeated upload of the same saved change must return the original
	// confirmation rather than create a second cloud change.
	if err := (func() error {
		bumpSyncAttempt(db, "UPDATE wb_ticket_sync_change_v4 SET status='pending', synced_at=NULL, received_at=NULL, next_attempt_at=now() WHERE id=?", rows[0].ID)
		return nil
	})(); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV4(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.RunSyncV5(ctx); err != nil {
		t.Fatal(err)
	}
	rows = readSyncV4Rows(t, db)
	if rows[0].Status != "synced" {
		t.Fatalf("duplicate upload was not confirmed: %+v", rows[0])
	}
	checkReopenedVoid(t, db, live, newID, true)
}

// Exercise a reopened ticket's void without changing the surrounding workflow.
func checkReopenedVoid(t *testing.T, db *gorm.DB, address string, ticketID int64, live bool) {
	t.Helper()
	rollback := errors.New("rollback test transaction")
	err := db.Transaction(func(tx *gorm.DB) error {
		ctx := context.Background()
		testService := weighing.NewService(weighing.NewRepository(tx), nil)
		if err := testService.ConfigureMasterSync(ctx, true, address); err != nil {
			return err
		}
		if err := tx.Exec("UPDATE wb_master_sync_state SET bound=true, last_success=now() WHERE id=1").Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE wb_ticket_sync_change_v5 SET status='synced', synced_at=now(), received_at=now()").Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE wb_ticket_sync_change_v4 SET status='synced', synced_at=now(), received_at=now()").Error; err != nil {
			return err
		}
		if _, err := testService.VoidTicket(ctx, ticketID, weighing.VoidInput{RequestID: "sync-reopened-void-test", Reason: "核验重开单作废"}, audit.Event{Metadata: audit.Metadata{ActorID: 1}}); err != nil {
			return err
		}
		var row syncV4Row
		if err := tx.Table("wb_ticket_sync_change_v5").Where("ticket_id=? AND change_type='void'", ticketID).Take(&row).Error; err != nil {
			return err
		}
		var payload struct {
			PreviousSiteCode string `json:"previousSiteCode"`
			Ticket           struct {
				PreviousTicketNo *string `json:"previousTicketNo"`
			} `json:"ticket"`
			Void *struct {
				Reason string `json:"reason"`
			} `json:"void"`
		}
		if err := json.Unmarshal([]byte(row.Payload), &payload); err != nil {
			return err
		}
		if payload.PreviousSiteCode != "SITE01" || payload.Ticket.PreviousTicketNo == nil || payload.Void == nil || payload.Void.Reason != "核验重开单作废" {
			return fmt.Errorf("reopened void missing prior association or void info: %s", row.Payload)
		}
		// PostgreSQL now() is fixed at the start of this outer test transaction.
		if err := tx.Exec("UPDATE wb_ticket_sync_change_v5 SET next_attempt_at=now() WHERE id=?", row.ID).Error; err != nil {
			return err
		}
		if err := testService.RunSyncV4(ctx); err != nil {
			return err
		}
		if err := testService.RunSyncV5(ctx); err != nil {
			return err
		}
		if err := tx.Table("wb_ticket_sync_change_v5").Where("id=?", row.ID).Take(&row).Error; err != nil {
			return err
		}
		if row.Status != "synced" {
			return fmt.Errorf("reopened void not confirmed: status=%s reason=%s", row.Status, row.Reason)
		}
		if live {
			result, err := testService.ReconcileSyncTicket(ctx, ticketID, "manual")
			if err != nil {
				return err
			}
			if result.Status != "consistent" {
				return fmt.Errorf("reopened void live reconciliation: %+v", result)
			}
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("reopened void transaction: %v", err)
	}
}

func checkSyncV4Reconciliation(t *testing.T, db *gorm.DB, service *weighing.Service, cloud *fakeSyncV4Cloud, cloudURL string, oldID int64) {
	t.Helper()
	ctx := context.Background()
	oldRows, err := service.SyncTicketDetail(ctx, oldID)
	if err != nil {
		t.Fatal(err)
	}
	if oldRows.Ticket.TicketNo == "" || len(oldRows.Changes) != 2 {
		t.Fatalf("detail: %+v", oldRows)
	}
	latest := oldRows.Changes[len(oldRows.Changes)-1]
	cloud.mu.Lock()
	cloud.ticketResponse = fmt.Sprintf(`{"found":true,"siteCode":%q,"ticketNo":%q,"status":"void","latestDigest":%q,"latestDigestVersion":3,"latestChangeType":"void","latestReceivedAt":"%s","changes":[{"digest":%q,"ticketNo":%q,"changeType":"issue","digestVersion":3,"schemaVersion":5,"receivedAt":"%s"},{"digest":%q,"ticketNo":%q,"changeType":"void","digestVersion":3,"schemaVersion":5,"receivedAt":"%s"}]}`,
		latest.SiteCode, latest.TicketNo, latest.Digest, time.Now().UTC().Format(time.RFC3339Nano),
		oldRows.Changes[0].Digest, oldRows.Changes[0].TicketNo, time.Now().UTC().Format(time.RFC3339Nano),
		latest.Digest, latest.TicketNo, time.Now().UTC().Format(time.RFC3339Nano))
	cloud.mu.Unlock()
	result, err := service.ReconcileSyncTicket(ctx, oldID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "consistent" {
		detail, _ := service.SyncTicketDetail(ctx, oldID)
		t.Fatalf("consistent reconcile: %+v detail=%+v", result, detail.Reconcile)
	}
	detail, err := service.SyncTicketDetail(ctx, oldID)
	if err != nil || detail.Reconcile == nil || detail.Reconcile.Status != "consistent" {
		t.Fatalf("stored reconcile: %+v %v", detail.Reconcile, err)
	}
	checkCloudConfirmationWithoutTicket(t, db, cloud, cloudURL, oldID)

	// The cloud lost the ticket: the factory replays the saved original
	// changes in order instead of inventing new content.
	cloud.mu.Lock()
	cloud.ticketResponse = fmt.Sprintf(`{"found":false,"siteCode":%q,"ticketNo":%q}`, latest.SiteCode, latest.TicketNo)
	cloud.mu.Unlock()
	result, err = service.ReconcileSyncTicket(ctx, oldID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending" || result.ReasonCode != "ticket_missing" {
		t.Fatalf("missing ticket: %+v", result)
	}
	resetRows := readSyncV4Rows(t, db)
	for _, row := range resetRows[:2] {
		if row.Status != "pending" || row.SyncedAt != nil {
			t.Fatalf("resend reset: %+v", row)
		}
	}

	// A cloud digest the factory does not know pauses the ticket and keeps a
	// separate review record.
	cloud.mu.Lock()
	cloud.ticketResponse = fmt.Sprintf(`{"found":true,"siteCode":%q,"ticketNo":%q,"status":"void","latestDigest":"%s","latestDigestVersion":3,"latestChangeType":"void","latestReceivedAt":"%s","changes":[]}`, latest.SiteCode, latest.TicketNo, strings.Repeat("0", 64), time.Now().UTC().Format(time.RFC3339Nano))
	cloud.mu.Unlock()
	result, err = service.ReconcileSyncTicket(ctx, oldID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "conflict" || result.ReasonCode != "cloud_unknown_change" {
		t.Fatalf("conflict reconcile: %+v", result)
	}
	detail, err = service.SyncTicketDetail(ctx, oldID)
	if err != nil {
		t.Fatal(err)
	}
	var openReview int64
	for _, review := range detail.Reviews {
		if review.Status == "open" && review.Blocking {
			openReview = review.ID
		}
	}
	if openReview == 0 {
		t.Fatal("conflict must create a blocking review")
	}
	if _, err := service.RecheckSyncReview(ctx, openReview, audit.Event{}); err == nil {
		t.Fatal("recheck must require a recorded conclusion")
	}
	cloud.mu.Lock()
	cloud.ticketResponse = fmt.Sprintf(`{"found":false,"siteCode":%q,"ticketNo":%q}`, latest.SiteCode, latest.TicketNo)
	cloud.mu.Unlock()
	if result, err := service.ReconcileSyncTicket(ctx, oldID, "automatic"); err != nil || result.Status != "conflict" {
		t.Fatalf("missing ticket must not release a conflict: %+v %v", result, err)
	}
	blocked, err := service.SyncTicketDetail(ctx, oldID)
	if err != nil {
		t.Fatal(err)
	}
	for _, review := range blocked.Reviews {
		if review.ID == openReview && (review.Status != "open" || !review.Blocking) {
			t.Fatalf("automatic check released conflict: %+v", review)
		}
	}
	if err := service.RecordSyncReviewConclusion(ctx, openReview, "已核对差异，等待云端处理", audit.Event{Metadata: audit.Metadata{ActorID: 1}}); err != nil {
		t.Fatal(err)
	}
	// A repeated automatic check must keep the administrator's conclusion.
	if _, err := service.ReconcileSyncTicket(ctx, oldID, "manual"); err != nil {
		t.Fatal(err)
	}
	detail, err = service.SyncTicketDetail(ctx, oldID)
	if err != nil {
		t.Fatal(err)
	}
	var keptConclusion bool
	for _, review := range detail.Reviews {
		if review.ID == openReview && review.Conclusion == "已核对差异，等待云端处理" {
			keptConclusion = true
		}
	}
	if !keptConclusion {
		t.Fatal("repeated conflict erased the recorded conclusion")
	}

	// Recheck succeeds only after the cloud difference is gone.
	cloud.mu.Lock()
	cloud.ticketResponse = fmt.Sprintf(`{"found":true,"siteCode":%q,"ticketNo":%q,"status":"void","latestDigest":%q,"latestDigestVersion":3,"latestChangeType":"void","latestReceivedAt":"%s","changes":[{"digest":%q,"ticketNo":%q,"changeType":"issue","digestVersion":3,"schemaVersion":5,"receivedAt":"%s"},{"digest":%q,"ticketNo":%q,"changeType":"void","digestVersion":3,"schemaVersion":5,"receivedAt":"%s"}]}`,
		latest.SiteCode, latest.TicketNo, latest.Digest, time.Now().UTC().Format(time.RFC3339Nano),
		oldRows.Changes[0].Digest, oldRows.Changes[0].TicketNo, time.Now().UTC().Format(time.RFC3339Nano),
		latest.Digest, latest.TicketNo, time.Now().UTC().Format(time.RFC3339Nano))
	cloud.mu.Unlock()
	if _, err := service.RecheckSyncReview(ctx, openReview, audit.Event{}); err != nil {
		t.Fatal(err)
	}
	detail, err = service.SyncTicketDetail(ctx, oldID)
	if err != nil {
		t.Fatal(err)
	}
	for _, review := range detail.Reviews {
		if review.ID == openReview && (review.Status != "resolved" || review.Blocking) {
			t.Fatalf("recheck did not resolve: %+v", review)
		}
	}
	// A later recurrence must create a new review, preserving the old conclusion.
	cloud.mu.Lock()
	cloud.ticketResponse = fmt.Sprintf(`{"found":true,"siteCode":%q,"ticketNo":%q,"status":"void","latestDigest":"%s","latestDigestVersion":3,"latestChangeType":"void","latestReceivedAt":"%s","changes":[]}`, latest.SiteCode, latest.TicketNo, strings.Repeat("0", 64), time.Now().UTC().Format(time.RFC3339Nano))
	cloud.mu.Unlock()
	if result, err := service.ReconcileSyncTicket(ctx, oldID, "manual"); err != nil || result.Status != "conflict" {
		t.Fatalf("recurrent conflict: %+v %v", result, err)
	}
	detail, err = service.SyncTicketDetail(ctx, oldID)
	if err != nil {
		t.Fatal(err)
	}
	var recurring int64
	for _, review := range detail.Reviews {
		if review.ID == openReview && (review.Status != "resolved" || review.Conclusion != "已核对差异，等待云端处理") {
			t.Fatalf("old conclusion lost: %+v", review)
		}
		if review.ID != openReview && review.Status == "open" && review.Blocking {
			recurring = review.ID
		}
	}
	if recurring == 0 {
		t.Fatal("recurring conflict must create a new review")
	}
	if err := service.RecordSyncReviewConclusion(ctx, recurring, "再次核对", audit.Event{Metadata: audit.Metadata{ActorID: 1}}); err != nil {
		t.Fatal(err)
	}
	cloud.mu.Lock()
	cloud.ticketResponse = fmt.Sprintf(`{"found":true,"siteCode":%q,"ticketNo":%q,"status":"void","latestDigest":%q,"latestDigestVersion":3,"latestChangeType":"void","latestReceivedAt":"%s","changes":[{"digest":%q,"ticketNo":%q,"changeType":"issue","digestVersion":3,"schemaVersion":5,"receivedAt":"%s"},{"digest":%q,"ticketNo":%q,"changeType":"void","digestVersion":3,"schemaVersion":5,"receivedAt":"%s"}]}`,
		latest.SiteCode, latest.TicketNo, latest.Digest, time.Now().UTC().Format(time.RFC3339Nano), oldRows.Changes[0].Digest, oldRows.Changes[0].TicketNo, time.Now().UTC().Format(time.RFC3339Nano), latest.Digest, latest.TicketNo, time.Now().UTC().Format(time.RFC3339Nano))
	cloud.mu.Unlock()
	if _, err := service.RecheckSyncReview(ctx, recurring, audit.Event{}); err != nil {
		t.Fatal(err)
	}

	// A failed query must never be stored as consistent.
	cloud.mu.Lock()
	cloud.ticketResponse = ""
	cloud.mu.Unlock()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	brokenURL := broken.URL
	broken.Close()
	if err := service.ConfigureMasterSync(ctx, true, brokenURL); err != nil {
		t.Fatal(err)
	}
	result, err = service.ReconcileSyncTicket(ctx, oldID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "unknown" {
		t.Fatalf("query failure must be unknown: %+v", result)
	}
	detail, err = service.SyncTicketDetail(ctx, oldID)
	if err != nil || detail.Reconcile == nil || detail.Reconcile.CloudFound != nil {
		t.Fatalf("unknown result must not claim missing: %+v %v", detail.Reconcile, err)
	}
	if err := service.ConfigureMasterSync(ctx, true, cloudURL); err != nil {
		t.Fatal(err)
	}

	// Batch reconciliation uses the same decision rules and never reports a
	// result it could not obtain.
	cloud.mu.Lock()
	cloud.batchDigest = latest.Digest
	cloud.batchTicketNo = latest.TicketNo
	cloud.batchStatus = "void"
	cloud.mu.Unlock()
	summary, err := service.ReconcileSyncRange(ctx, "", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Checked != 2 || summary.Consistent == 0 {
		t.Fatalf("batch summary: %+v", summary)
	}

	// Read-only confirmation by the original site code and digest supports a
	// lost acknowledgement after a site-code change.
	cloud.mu.Lock()
	cloud.lookupResponse = fmt.Sprintf(`{"found":true,"siteCode":%q,"digest":%q,"confirmation":{"digest":%q,"ticketNo":%q,"changeType":"void","digestVersion":3,"schemaVersion":5,"receivedAt":"%s"}}`, latest.SiteCode, latest.Digest, latest.Digest, latest.TicketNo, time.Now().UTC().Format(time.RFC3339Nano))
	cloud.mu.Unlock()
	lookup, err := service.LookupSyncConfirmation(ctx, latest.SiteCode, latest.Digest)
	if err != nil || !lookup.Found || lookup.Confirmation == nil || lookup.Confirmation.Digest != latest.Digest {
		t.Fatalf("history confirmation: %+v %v", lookup, err)
	}
	cloud.mu.Lock()
	cloud.lookupResponse = fmt.Sprintf(`{"found":false,"siteCode":%q,"digest":%q,"confirmation":null}`, latest.SiteCode, strings.Repeat("0", 64))
	cloud.mu.Unlock()
	lookup, err = service.LookupSyncConfirmation(ctx, latest.SiteCode, strings.Repeat("0", 64))
	if err != nil || lookup.Found {
		t.Fatalf("missing history confirmation: %+v %v", lookup, err)
	}
}

func checkCloudConfirmationWithoutTicket(t *testing.T, db *gorm.DB, cloud *fakeSyncV4Cloud, address string, ticketID int64) {
	t.Helper()
	var issue syncV4Row
	if err := db.Table("wb_ticket_sync_change_v5").Where("ticket_id=? AND change_type='issue'", ticketID).Take(&issue).Error; err != nil {
		t.Fatal(err)
	}
	cloud.mu.Lock()
	previousTicket, previousLookup := cloud.ticketResponse, cloud.lookupResponse
	cloud.ticketResponse = fmt.Sprintf(`{"found":false,"siteCode":%q,"ticketNo":%q}`, issue.SiteCode, issue.TicketNo)
	cloud.lookupResponse = fmt.Sprintf(`{"found":true,"siteCode":%q,"digest":%q,"confirmation":{"digest":%q,"ticketNo":%q,"changeType":"issue","digestVersion":3,"schemaVersion":5,"receivedAt":%q}}`, issue.SiteCode, issue.Digest, issue.Digest, issue.TicketNo, time.Now().UTC().Format(time.RFC3339Nano))
	cloud.mu.Unlock()
	defer func() {
		cloud.mu.Lock()
		cloud.ticketResponse, cloud.lookupResponse = previousTicket, previousLookup
		cloud.mu.Unlock()
	}()
	rollback := errors.New("rollback test transaction")
	err := db.Transaction(func(tx *gorm.DB) error {
		ctx := context.Background()
		service := weighing.NewService(weighing.NewRepository(tx), nil)
		if err := service.ConfigureMasterSync(ctx, true, address); err != nil {
			return err
		}
		result, err := service.ReconcileSyncTicket(ctx, ticketID, "manual")
		if err != nil {
			return err
		}
		if result.Status != "needs_review" || result.ReasonCode != "cloud_confirmation_without_ticket" {
			return fmt.Errorf("receipt without ticket: %+v", result)
		}
		var pending int64
		if err := tx.Table("wb_ticket_sync_change_v4").Where("ticket_id=? AND status <> 'synced'", ticketID).Count(&pending).Error; err != nil {
			return err
		}
		if pending != 0 {
			return fmt.Errorf("receipt without ticket requeued %d changes", pending)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("confirmation without ticket: %v", err)
	}
}
