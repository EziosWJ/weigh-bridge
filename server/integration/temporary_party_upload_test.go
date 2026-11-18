//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresTemporaryPartyUploadRetriesAcrossRestart(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	ctx := context.Background()
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	deps.Weighing = service
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F001", ScaleCode: "WB01"}, audit.Event{}); err != nil {
		t.Fatal(err)
	}

	var requests []string
	var mu sync.Mutex
	var cloudVersion int64
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/weighbridge/sync/master":
			fmt.Fprint(w, `{"code":200,"message":"ok","data":{"schemaVersion":2,"siteCode":"F001","categories":[],"materials":[],"parties":[],"temporaryPartyResults":[]}}`)
		case "/api/v1/weighbridge/sync/temporary-parties":
			var payload struct {
				SiteCode      string `json:"siteCode"`
				TemporaryCode string `json:"temporaryCode"`
				Version       int64  `json:"version"`
			}
			body, err := io.ReadAll(r.Body)
			if err != nil || json.Unmarshal(body, &payload) != nil || payload.SiteCode != "F001" || payload.TemporaryCode == "" {
				t.Errorf("invalid registration: %s %v", body, err)
				http.Error(w, "invalid test registration", http.StatusBadRequest)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			requests = append(requests, string(body))
			switch {
			case len(requests) == 1:
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, `{"code":503,"data":null}`)
			case payload.Version == 3:
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"code":409,"message":"含主体名称的云端错误","data":{"kind":"finalized"}}`)
			default:
				cloudVersion = max(cloudVersion, payload.Version)
				fmt.Fprintf(w, `{"code":200,"data":{"schemaVersion":1,"siteCode":%q,"temporaryCode":%q,"version":%d,"status":"pending"}}`, payload.SiteCode, payload.TemporaryCode, cloudVersion)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer cloud.Close()

	if err := service.ConfigureMasterSync(ctx, true, cloud.URL); err != nil {
		t.Fatal(err)
	}
	if err := service.RunMasterSync(ctx, "manual"); err != nil {
		t.Fatal(err)
	}
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	const path = "/api/weighing/temporary-parties"
	created := serveJSON(router, http.MethodPost, path, `{"name":"临时供货方","nature":"enterprise","roles":["supplier"]}`, admin)
	assertEnvelopeCode(t, created, 200, 200, "success")
	party := decodeCameraData[weighing.Party](t, created)
	// A manual pull does not dispatch or acknowledge registrations.
	if err := service.RunMasterSync(ctx, "manual"); err != nil {
		t.Fatal(err)
	}
	type attempt struct {
		Attempts      int
		Payload       string
		LastErrorKind string
		NextAttemptAt *time.Time
		ConfirmedAt   *time.Time
		BlockedAt     *time.Time
	}
	outbox := func(version int) attempt {
		t.Helper()
		var result attempt
		if err := db.GORM.Table("wb_temporary_party_outbox").Where("party_id = ? AND version = ?", party.ID, version).Take(&result).Error; err != nil {
			t.Fatal(err)
		}
		return result
	}
	if row := outbox(1); row.Attempts != 0 || row.ConfirmedAt != nil {
		t.Fatalf("manual pull dispatched registration: %+v", row)
	}

	if err := service.RunTemporaryPartySync(ctx); err != nil {
		t.Fatal(err)
	}
	first := outbox(1)
	if first.Attempts != 1 || first.LastErrorKind != "unknown_response" || first.ConfirmedAt != nil ||
		first.NextAttemptAt == nil || !first.NextAttemptAt.After(time.Now()) {
		t.Fatalf("failed attempt was not retained for retry: %+v", first)
	}
	if err := db.GORM.Exec("UPDATE wb_temporary_party_outbox SET next_attempt_at = CURRENT_TIMESTAMP + INTERVAL '1 hour' WHERE party_id = ? AND version = 1", party.ID).Error; err != nil {
		t.Fatal(err)
	}
	editPath := fmt.Sprintf("%s/%d", path, party.ID)
	corrected := serveJSON(router, http.MethodPut, editPath, `{"name":"更正后的供货方","nature":"individual","roles":["supplier"],"version":1}`, admin)
	assertEnvelopeCode(t, corrected, 200, 200, "success")

	restarted := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	if err := restarted.ConfigureMasterSync(ctx, true, cloud.URL); err != nil {
		t.Fatal(err)
	}
	if err := restarted.RunTemporaryPartySync(ctx); err != nil {
		t.Fatal(err)
	}
	if row := outbox(2); row.ConfirmedAt == nil || row.Attempts != 1 {
		t.Fatalf("newer version was blocked by backoff: %+v", row)
	}
	// Advance the due time through the database to keep the test independent
	// of a timer. Cloud returns its current version for the older retry.
	if err := db.GORM.Exec("UPDATE wb_temporary_party_outbox SET next_attempt_at = CURRENT_TIMESTAMP WHERE party_id = ? AND version = 1", party.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := restarted.RunTemporaryPartySync(ctx); err != nil {
		t.Fatal(err)
	}
	if row := outbox(1); row.Attempts != 2 || row.ConfirmedAt == nil || row.NextAttemptAt != nil || row.LastErrorKind != "" {
		t.Fatalf("original version not confirmed after restart: %+v", row)
	}
	mu.Lock()
	got := append([]string(nil), requests...)
	mu.Unlock()
	if len(got) != 3 || got[0] != first.Payload || got[2] != first.Payload || got[1] == first.Payload {
		t.Fatalf("unexpected registration payloads: %q", got)
	}

	corrected = serveJSON(router, http.MethodPut, editPath, `{"name":"再更正","nature":"individual","roles":["supplier"],"version":2}`, admin)
	assertEnvelopeCode(t, corrected, 200, 200, "success")
	for range 2 {
		if err := restarted.RunTemporaryPartySync(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if row := outbox(3); row.Attempts != 1 || row.ConfirmedAt != nil || row.BlockedAt == nil || row.LastErrorKind != "finalized" {
		t.Fatalf("finalized rejection was not stopped: %+v", row)
	}
	var log weighing.MasterSyncLog
	if err := db.GORM.Table("wb_master_sync_log").Where("sync_type = 'temp_party' AND temporary_code = ? AND category = 'conflict'", party.Code).Take(&log).Error; err != nil {
		t.Fatal(err)
	}
	if log.Success || !strings.Contains(log.Reason, "第 3 版") || !strings.Contains(log.Reason, "请同步资料") ||
		strings.Contains(log.Reason, "含主体名称的云端错误") {
		t.Fatalf("non-actionable or sensitive diagnostic: %+v", log)
	}
}
