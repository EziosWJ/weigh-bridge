//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresTemporaryPartyRegistrationPermissionsAndRollback(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	service := weighing.NewService(weighing.NewRepository(db.GORM), nil)
	deps.Weighing = service
	ctx := context.Background()
	if _, err := service.InitializeLocalSite(ctx, 1, weighing.LocalSiteInput{SiteCode: "F001", ScaleCode: "WB01"}, audit.Event{}); err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":200,"message":"ok","data":{"schemaVersion":2,"siteCode":"F001","categories":[],"materials":[],"parties":[],"temporaryPartyResults":[]}}`)
	}))
	t.Cleanup(remote.Close)
	if err := service.ConfigureMasterSync(ctx, true, remote.URL); err != nil {
		t.Fatal(err)
	}
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	const path = "/api/weighing/temporary-parties"
	const original = `{"name":"  临时供货方  ","nature":"enterprise","roles":["supplier","carrier"]}`
	beforeBinding := serveJSON(router, http.MethodPost, path, original, admin)
	assertEnvelopeCode(t, beforeBinding, 200, 400, "完成首次总部资料同步后才能登记或更正临时往来主体")
	if err := service.RunMasterSync(ctx, "manual"); err != nil {
		t.Fatal(err)
	}
	remote.Close()

	const username = "temporary-party-operator"
	createdUser := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"temporary-party-operator","nickname":"操作员","status":1}`, admin)
	assertEnvelopeCode(t, createdUser, 200, 200, "success")
	var userID int64
	if err := db.GORM.Table("sys_user").Select("id").Where("username = ?", username).Scan(&userID).Error; err != nil {
		t.Fatal(err)
	}
	operator := loginUser(t, router, username, "admin123")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, path, original, operator), 403, 403, "无权执行此操作")
	if err := db.GORM.Exec("INSERT INTO sys_user_role (user_id, role_id) SELECT ?, id FROM sys_role WHERE role_code='WEIGHING_OPERATOR'", userID).Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, path, original, operator), 403, 403, weighing.ErrForbidden.Error())
	if err := db.GORM.Exec("INSERT INTO wb_local_user_access (user_id,can_operate) VALUES (?,1)", userID).Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, path, `{"name":"临时供货方","nature":"enterprise","roles":[]}`, operator), 200, 400, weighing.ErrInvalid.Error())

	created := serveJSON(router, http.MethodPost, path, original, operator)
	assertEnvelopeCode(t, created, 200, 200, "success")
	party := decodeCameraData[weighing.Party](t, created)
	if party.ID <= 0 || !regexp.MustCompile(`^T-P-[A-Za-z0-9-]{1,36}$`).MatchString(party.Code) ||
		party.Name != "临时供货方" || party.Enabled != 1 || party.TemporaryState != "pending" ||
		party.Version != 1 || !slices.Equal(party.Roles, []string{"supplier", "carrier"}) {
		t.Fatalf("registered party = %+v", party)
	}

	correctionPath := fmt.Sprintf("%s/%d", path, party.ID)
	const correction = `{"name":"更正后的供货方","nature":"individual","roles":["supplier"],"version":1}`
	corrected := serveJSON(router, http.MethodPut, correctionPath, correction, operator)
	assertEnvelopeCode(t, corrected, 200, 200, "success")
	revised := decodeCameraData[weighing.Party](t, corrected)
	if revised.ID != party.ID || revised.Code != party.Code || revised.Version != 2 || revised.Name != "更正后的供货方" ||
		revised.Nature != "individual" || !slices.Equal(revised.Roles, []string{"supplier"}) {
		t.Fatalf("corrected party = %+v", revised)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, correctionPath, correction, operator), 200, 400, weighing.ErrConflict.Error())

	// Reject the next outbox insert and check that neither registration nor
	// correction can leave a usable row without its matching upload.
	if err := db.GORM.Exec("ALTER TABLE wb_temporary_party_outbox ADD CHECK (version <> 3 AND payload NOT LIKE '%模拟失败%')").Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, path, `{"name":"模拟失败","nature":"enterprise","roles":["supplier"]}`, operator), 500, 500, "业务处理失败")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, correctionPath, `{"name":"覆盖失败","nature":"enterprise","roles":["carrier"],"version":2}`, operator), 500, 500, "业务处理失败")
	var result []weighing.Party
	if err := db.GORM.Where("temporary_state <> ''").Find(&result).Error; err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Name != revised.Name || result[0].Version != revised.Version {
		t.Fatalf("party rows after rollback = %+v", result)
	}
	type queued struct {
		Version int64
		Payload string
	}
	var queue []queued
	if err := db.GORM.Table("wb_temporary_party_outbox").Order("version").Find(&queue).Error; err != nil {
		t.Fatal(err)
	}
	if len(queue) != 2 {
		t.Fatalf("queued registrations = %+v", queue)
	}
	for i, name := range []string{party.Name, revised.Name} {
		var payload struct {
			SiteCode      string `json:"siteCode"`
			TemporaryCode string `json:"temporaryCode"`
			Name          string `json:"name"`
			Version       int64  `json:"version"`
		}
		if err := json.Unmarshal([]byte(queue[i].Payload), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.SiteCode != "F001" || payload.TemporaryCode != party.Code ||
			payload.Version != int64(i+1) || queue[i].Version != payload.Version || payload.Name != name {
			t.Fatalf("queued registration = %+v", payload)
		}
	}
}
