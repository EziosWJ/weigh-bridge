//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
	"gorm.io/gorm"
)

type taskSyncRow struct {
	TaskID, Version, Attempts            int64
	SiteCode, TaskNo, Payload, LastError string
	NextAttemptAt                        time.Time
	ConfirmedAt, BlockedAt               *time.Time
}

func newTaskSyncFixture(t *testing.T, cloud http.HandlerFunc) (*gorm.DB, *weighing.Service) {
	t.Helper()
	pg := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, pg.dsn)
	t.Cleanup(func() { _ = db.Close() })
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	remote := httptest.NewServer(cloud)
	t.Cleanup(remote.Close)
	ctx := context.Background()
	event := audit.Event{Metadata: audit.Metadata{ActorID: 1}}
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "W01", ScaleCode: "WB01"}, event); err != nil {
		t.Fatal(err)
	}
	if err := service.ConfigureMasterSync(ctx, true, remote.URL); err != nil {
		t.Fatal(err)
	}
	taskSyncSQL(t, db.GORM, "UPDATE wb_master_sync_state SET bound=true, last_success=now() WHERE id=1")
	taskSyncSQL(t, db.GORM, "INSERT INTO wb_task_sync_bootstrap (id, initialized_at) VALUES (1, now())")
	taskSyncSQL(t, db.GORM, "INSERT INTO wb_material (id,code,name,create_by,update_by) VALUES (101,'M1','粮食',1,1)")
	taskSyncSQL(t, db.GORM, "INSERT INTO wb_party (id,code,name,nature,is_supplier,create_by,update_by) VALUES (101,'P1','供货方','enterprise',1,1,1)")
	return db.GORM, service
}

func taskSyncSQL(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatal(err)
	}
}

func queueTaskSync(t *testing.T, db *gorm.DB, site, no string, id, version int64) {
	t.Helper()
	taskSyncSQL(t, db, `INSERT INTO wb_transport_task
		(id,site_code,task_no,plate,direction,status,scale_code,material_id,party_id,business_snapshot,create_time,update_time,actor_id,actor_name)
		VALUES (?,?,?,?,'inbound','awaiting_second','WB01',101,101,'{}',now(),now(),1,'操作员') ON CONFLICT(id) DO NOTHING`,
		id, site, no, fmt.Sprintf("川A%05d", id))
	taskSyncSQL(t, db, `INSERT INTO wb_task_sync_outbox (task_id,site_code,task_no,version,payload)
		VALUES (?,?,?,?,?)`, id, site, no, version, fmt.Sprintf(`{"taskNo":%q,"version":%d,"status":"awaiting_second","plate":%q,"direction":"inbound","materialCode":"M1","materialName":"粮食","partyCode":"P1","partyName":"供货方","firstWeighedAt":"2026-11-19T01:00:00Z","termination":null,"ticket":null}`,
		no, version, fmt.Sprintf("川A%05d", id)))
}

func readTaskSyncRow(t *testing.T, db *gorm.DB, id, version int64) taskSyncRow {
	t.Helper()
	var row taskSyncRow
	if err := db.Table("wb_task_sync_outbox").Where("task_id=? AND version=?", id, version).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

type taskSyncBatch struct {
	SchemaVersion int
	SiteCode      string
	Items         []struct {
		TaskNo  string `json:"taskNo"`
		Version int64  `json:"version"`
	}
}

func decodeTaskSyncBatch(t *testing.T, w http.ResponseWriter, r *http.Request) taskSyncBatch {
	t.Helper()
	var batch taskSyncBatch
	if r.URL.Path != "/api/v1/weighbridge/sync/tasks" || r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&batch) != nil || batch.SchemaVersion != 1 || batch.Items == nil {
		t.Error("invalid task sync request")
		w.WriteHeader(http.StatusBadRequest)
	}
	return batch
}

func acknowledgeTaskSync(w http.ResponseWriter, batch taskSyncBatch) {
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{
		"schemaVersion": 1, "siteCode": batch.SiteCode, "items": batch.Items, "receivedAt": time.Now(),
	}})
}

type taskSyncCalls struct {
	mu     sync.Mutex
	values []string
}

func (c *taskSyncCalls) add(value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values = append(c.values, value)
}

func (c *taskSyncCalls) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.values...)
}

func runFirstTaskSyncWorkerPass(t *testing.T, db *gorm.DB, service *weighing.Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.RunTaskSyncWorker(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var heartbeat struct{ LastHeartbeatAt *time.Time }
		err := db.WithContext(ctx).Table("wb_task_sync_bootstrap").Where("id=1").Take(&heartbeat).Error
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if heartbeat.LastHeartbeatAt != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runTaskSync(t *testing.T, service *weighing.Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := service.RunTaskSync(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresTaskSyncOldSiteRejectionDoesNotBlockCurrentSite(t *testing.T) {
	const oldNo, currentNo = "RW-FCC01-20261030-0002", "RW-W01-20261119-0001"
	const previousRejection = "task sync rejected: HTTP 401, code 401"
	var calls taskSyncCalls
	db, service := newTaskSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		batch := decodeTaskSyncBatch(t, w, r)
		if len(batch.Items) == 0 {
			calls.add(batch.SiteCode + " heartbeat")
		} else {
			calls.add(batch.Items[0].TaskNo)
		}
		if batch.SiteCode == "FCC01" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"code":401,"message":"厂端身份无效","data":null}`)
			return
		}
		acknowledgeTaskSync(w, batch)
	})
	queueTaskSync(t, db, "FCC01", oldNo, 1, 1)
	queueTaskSync(t, db, "FCC01", oldNo, 1, 2)
	queueTaskSync(t, db, "FCC01", oldNo, 1, 3)
	queueTaskSync(t, db, "W01", currentNo, 2, 1)
	queueTaskSync(t, db, "FCC01", "RW-FCC01-20261030-0003", 3, 1)
	taskSyncSQL(t, db, "UPDATE wb_task_sync_outbox SET confirmed_at=now() WHERE task_id=1 AND version=1")
	taskSyncSQL(t, db, "UPDATE wb_task_sync_outbox SET attempts=7 WHERE task_id=1 AND version=2")
	taskSyncSQL(t, db, "UPDATE wb_task_sync_outbox SET attempts=8,last_error=?,next_attempt_at=now()+interval '10 minutes' WHERE task_id=3", previousRejection)
	confirmed := readTaskSyncRow(t, db, 1, 1)
	old := readTaskSyncRow(t, db, 1, 2)
	prior401 := readTaskSyncRow(t, db, 3, 1)

	runFirstTaskSyncWorkerPass(t, db, service)
	if current := readTaskSyncRow(t, db, 2, 1); current.ConfirmedAt == nil {
		t.Fatalf("W01 did not receive its receipt after the FCC01 rejection; HTTP calls: %v", calls.snapshot())
	}
	if want := []string{oldNo, currentNo, "W01 heartbeat"}; !reflect.DeepEqual(calls.snapshot(), want) {
		t.Fatalf("calls = %v, want %v", calls.snapshot(), want)
	}
	paused := readTaskSyncRow(t, db, 1, 2)
	if paused.BlockedAt == nil || paused.ConfirmedAt != nil || paused.Attempts != 8 || paused.LastError != previousRejection ||
		paused.SiteCode != old.SiteCode || paused.TaskNo != old.TaskNo || paused.Version != old.Version || paused.Payload != old.Payload {
		t.Fatalf("old snapshot or failure not retained: %+v", paused)
	}
	legacyPaused := readTaskSyncRow(t, db, 3, 1)
	if legacyPaused.BlockedAt == nil {
		t.Fatal("historical 401 during backoff was not paused")
	}
	legacyPaused.BlockedAt = nil
	if !reflect.DeepEqual(legacyPaused, prior401) || !reflect.DeepEqual(readTaskSyncRow(t, db, 1, 1), confirmed) {
		t.Fatal("historical rejection or existing receipt was changed")
	}
	if next := readTaskSyncRow(t, db, 1, 3); next.Attempts != 0 || next.ConfirmedAt != nil {
		t.Fatalf("later version bypassed the rejected predecessor: %+v", next)
	}
	taskSyncSQL(t, db, "UPDATE wb_task_sync_outbox SET next_attempt_at=now() WHERE confirmed_at IS NULL")
	taskSyncSQL(t, db, "UPDATE wb_task_sync_bootstrap SET last_heartbeat_at=now()-interval '2 minutes'")
	runTaskSync(t, service)
	if want := []string{oldNo, currentNo, "W01 heartbeat", "W01 heartbeat"}; !reflect.DeepEqual(calls.snapshot(), want) {
		t.Fatalf("later pass retried a paused record or missed heartbeat: %v", calls.snapshot())
	}
	if after := readTaskSyncRow(t, db, 1, 2); after.LastError != paused.LastError || after.Attempts != paused.Attempts || !after.BlockedAt.Equal(*paused.BlockedAt) {
		t.Fatalf("later pass changed the pause: %+v", after)
	}
}

func TestPostgresTaskSyncBackoffAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		reason string
	}{
		{"connection closed", 0, "连接云端失败"},
		{"cloud unavailable", http.StatusServiceUnavailable, "HTTP 503"},
		{"current site rejected", http.StatusUnauthorized, "HTTP 401"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkTaskSyncBackoff(t, tc.status, tc.reason)
		})
	}
}

func checkTaskSyncBackoff(t *testing.T, status int, reason string) {
	t.Helper()
	const oldNo, currentNo, independentNo = "RW-FCC01-20261030-0002", "RW-W01-20261119-0001", "RW-W01-20261119-0002"
	var calls taskSyncCalls
	var disconnect atomic.Bool
	disconnect.Store(true)
	db, service := newTaskSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		batch := decodeTaskSyncBatch(t, w, r)
		if len(batch.Items) == 0 {
			calls.add(batch.SiteCode + " heartbeat")
		} else {
			item := batch.Items[0]
			calls.add(fmt.Sprintf("%s v%d", item.TaskNo, item.Version))
			if disconnect.Load() {
				if status != 0 {
					w.WriteHeader(status)
					fmt.Fprintf(w, `{"code":%d}`, status)
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
				return
			}
		}
		acknowledgeTaskSync(w, batch)
	})
	queueTaskSync(t, db, "FCC01", oldNo, 1, 1)
	taskSyncSQL(t, db, "UPDATE wb_task_sync_outbox SET next_attempt_at=now()+interval '10 minutes' WHERE task_id=1")
	queueTaskSync(t, db, "W01", currentNo, 2, 1)
	queueTaskSync(t, db, "W01", currentNo, 2, 2)
	queueTaskSync(t, db, "W01", independentNo, 3, 1)

	runTaskSync(t, service)
	if want := []string{currentNo + " v1"}; !reflect.DeepEqual(calls.snapshot(), want) {
		t.Fatalf("the old backoff blocked W01 or a transport failure caused more requests: %v", calls.snapshot())
	}
	failed := readTaskSyncRow(t, db, 2, 1)
	if failed.BlockedAt != nil || failed.ConfirmedAt != nil || failed.Attempts != 1 || !failed.NextAttemptAt.After(time.Now()) || !strings.Contains(failed.LastError, reason) {
		t.Fatalf("temporary failure was not scheduled for retry: %+v", failed)
	}
	disconnect.Store(false)
	runTaskSync(t, service)
	if want := []string{currentNo + " v1", independentNo + " v1"}; !reflect.DeepEqual(calls.snapshot(), want) {
		t.Fatalf("backoff blocked an independent task or permitted a premature heartbeat/version: %v", calls.snapshot())
	}
	taskSyncSQL(t, db, "UPDATE wb_task_sync_outbox SET next_attempt_at=now() WHERE task_id=2 AND version=1")
	runTaskSync(t, service)
	runTaskSync(t, service)
	if want := []string{currentNo + " v1", independentNo + " v1", currentNo + " v1", currentNo + " v2", "W01 heartbeat"}; !reflect.DeepEqual(calls.snapshot(), want) {
		t.Fatalf("recovery did not preserve version ordering and heartbeat rules: %v", calls.snapshot())
	}
	recovered := readTaskSyncRow(t, db, 2, 1)
	if recovered.ConfirmedAt == nil || recovered.Payload != failed.Payload || recovered.LastError != "" {
		t.Fatalf("network retry did not confirm the saved snapshot: %+v", recovered)
	}
}
