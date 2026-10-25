package agentmaintenance

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/gin-gonic/gin"
)

type backupTestStore struct {
	records         []BackupRecord
	effectiveWrites int
	creates         int
	deletes         int
}

func (s *backupTestStore) SaveEffectiveConfig(context.Context, int64, p.EffectiveConfig) error {
	s.effectiveWrites++
	return nil
}
func (s *backupTestStore) CreateConfigBackup(context.Context, int64, p.ConfigBackup, string, string, int64) (int64, error) {
	s.creates++
	return 1, nil
}
func (s *backupTestStore) ListConfigBackups(context.Context, int64, int, int) ([]BackupRecord, int64, error) {
	return s.records, int64(len(s.records)), nil
}
func (s *backupTestStore) ConfigBackupByID(context.Context, int64, int64) (BackupRecord, error) {
	return BackupRecord{}, ErrNoBackup
}
func (s *backupTestStore) DeleteConfigBackup(_ context.Context, scale, id int64) error {
	if scale != 1 || id != 11 {
		return ErrNoBackup
	}
	s.deletes++
	return nil
}
func (s *backupTestStore) AgentRuntime(context.Context, int64) (RuntimeRecord, bool, error) {
	return RuntimeRecord{EffectiveConfigVersion: 3, EffectiveSettings: p.DeviceSettings{Serial: p.SerialSettings{Port: "/dev/ttyUSB0", Baud: 9600}}}, true, nil
}

func TestBackupRoutesAreLocalPermissionedAndReadableOffline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &backupTestStore{records: []BackupRecord{{ID: 11, Name: "校准完成", Source: "manual"}}}
	remote := NewRemote(agentbridge.New(agentbridge.Config{ID: "agent", ScaleID: 1}), func(context.Context) (Config, error) { c := Defaults(); c.ScaleID = 1; return c, nil }, nil)
	remote.SetStore(store)
	permissions := businessPermissions{PermissionView: true}
	recorder := &businessAudit{}
	router := gin.New()
	remote.RegisterBusinessRoutes(router.Group("/api/system"), businessAuthenticator{}, permissions, recorder)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/system/agent-maintenance"+path, strings.NewReader(body))
		req.Header.Set("Authorization", token)
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}
	if got := request("GET", "/backups", "", ""); got.Code != 401 {
		t.Fatalf("unauthenticated=%d", got.Code)
	}
	got := request("GET", "/backups", "", "Bearer valid")
	var result struct {
		Records   []backupView       `json:"records"`
		Live      bool               `json:"live"`
		Effective *p.EffectiveConfig `json:"effective"`
	}
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &result) != nil || result.Live || len(result.Records) != 1 || result.Effective == nil {
		t.Fatalf("offline list=%d %s", got.Code, got.Body)
	}
	if got := request("POST", "/backups", `{"name":"备份"}`, "Bearer valid"); got.Code != 403 {
		t.Fatalf("view create=%d", got.Code)
	}
	if got := request("DELETE", "/backups/11", "", "Bearer valid"); got.Code != 403 {
		t.Fatalf("view delete=%d", got.Code)
	}
	permissions[PermissionManage] = true
	if got := request("POST", "/backups", `{"name":"备份","baseVersion":3}`, "Bearer valid"); got.Code != 409 {
		t.Fatalf("offline create=%d %s", got.Code, got.Body)
	}
	if got := request("POST", "/backups", `{"name":"  "}`, "Bearer valid"); got.Code != 400 {
		t.Fatalf("empty name=%d", got.Code)
	}
	if store.creates != 0 {
		t.Fatal("offline creation stored a backup")
	}
	if got := request("GET", "/backups?limit=101", "", "Bearer valid"); got.Code != 400 {
		t.Fatalf("unbounded list=%d", got.Code)
	}
	if got := request("DELETE", "/backups/11", "", "Bearer valid"); got.Code != 204 || store.deletes != 1 {
		t.Fatalf("delete=%d", got.Code)
	}
	if len(recorder.events) != 1 || recorder.events[0].Action != "maintenance.backup.delete" {
		t.Fatalf("audit=%+v", recorder.events)
	}
	if got := request("POST", "/restore", `{"backupId":12,"baseVersion":3}`, "Bearer valid"); got.Code != 400 {
		t.Fatalf("deleted/missing restore=%d", got.Code)
	}
}

func TestManualBackupRequiresMatchingConfirmedSnapshot(t *testing.T) {
	settings := p.DeviceSettings{Serial: p.SerialSettings{Port: "/dev/ttyUSB0", Baud: 9600}}
	backup := p.ConfigBackup{ConfigVersion: 3, Digest: settings.Digest(), Settings: settings, AppliedAt: time.Now()}
	effective := p.EffectiveConfig{ConfigVersion: 3, Digest: backup.Digest, Serial: settings.Serial, Print: settings.Print}
	if err := validateBackupSnapshot(effective, backup, 3, backup.Digest); err != nil {
		t.Fatal(err)
	}
	if validateBackupSnapshot(effective, backup, 2, backup.Digest) == nil {
		t.Fatal("stale version accepted")
	}
	if validateBackupSnapshot(effective, backup, 3, "changed") == nil {
		t.Fatal("stale digest accepted")
	}
	backup.Settings.Serial.Port = "/dev/ttyUSB9"
	if validateBackupSnapshot(effective, backup, 3, backup.Digest) == nil {
		t.Fatal("unconfirmed values accepted")
	}
}
func TestApplyAndRestorePersistOnlyEffectiveState(t *testing.T) {
	store := &backupTestStore{}
	remote := &Remote{store: store}
	remote.persistApplied(context.Background(), 1, p.ConfigApplyResult{Status: p.ConfigApplyApplied, Effective: &p.EffectiveConfig{ConfigVersion: 4}})
	remote.persistApplied(context.Background(), 1, p.ConfigApplyResult{Status: p.ConfigApplyRejected})
	if store.effectiveWrites != 1 || store.creates != 0 {
		t.Fatalf("writes=%d backups=%d", store.effectiveWrites, store.creates)
	}
}
