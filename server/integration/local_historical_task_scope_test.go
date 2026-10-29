//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
)

func TestPostgresHistoricalTaskScopeUsesLocalAccess(t *testing.T) {
	postgres := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, postgres.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	ensureLocalSite(t, router, admin, 1, "CURRENT", "本厂", "WB01", "地磅", "local-agent")
	if err := db.GORM.Exec(`INSERT INTO wb_material (code,name,create_by,update_by) VALUES ('TEST-M','测试物料',1,1); INSERT INTO wb_party (code,name,nature,is_supplier,create_by,update_by) VALUES ('TEST-P','测试主体','enterprise',1,1,1)`).Error; err != nil {
		t.Fatal(err)
	}
	var taskID int64
	err = db.GORM.Raw(`INSERT INTO wb_transport_task
  (task_no,mode,plate,direction,status,site_code,scale_code,material_id,party_id,business_snapshot,version,create_time,update_time,actor_id,actor_name)
  SELECT 'RW-OLD-20261027-0001','normal','川A12345','inbound','awaiting_second','OLD','OLD_SCALE',m.id,p.id,'{}'::jsonb,1,now(),now(),1,'管理员'
  FROM wb_material m CROSS JOIN wb_party p LIMIT 1 RETURNING id`).Scan(&taskID).Error
	if err != nil || taskID == 0 {
		t.Fatalf("insert historic task id=%d err=%v", taskID, err)
	}
	for _, path := range []string{"/api/weighing/tasks", "/api/weighing/tasks?siteCode=CURRENT", fmt.Sprintf("/api/weighing/tasks/%d", taskID), "/api/weighing/tasks/active?plate=川A12345&scale=WB01"} {
		response := serveJSON(router, http.MethodGet, path, "", admin)
		if response.Code != http.StatusOK || envelopeCode(t, response) != http.StatusOK || !strings.Contains(response.Body.String(), "RW-OLD-20261027-0001") {
			t.Errorf("historic task unavailable for %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	created := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"no-local-access","nickname":"未授权操作员","status":1}`, admin)
	assertEnvelopeCode(t, created, http.StatusOK, http.StatusOK, "success")
	var userID int64
	if err := db.GORM.Table("sys_user").Select("id").Where("username=?", "no-local-access").Scan(&userID).Error; err != nil || userID == 0 {
		t.Fatalf("user id=%d err=%v", userID, err)
	}
	if err := db.GORM.Exec("INSERT INTO sys_user_role(user_id,role_id) SELECT ?,id FROM sys_role WHERE role_code='WEIGHING_OPERATOR'", userID).Error; err != nil {
		t.Fatal(err)
	}
	unassigned := loginUser(t, router, "no-local-access", "admin123")
	denied := serveJSON(router, http.MethodGet, fmt.Sprintf("/api/weighing/tasks/%d", taskID), "", unassigned)
	assertEnvelopeCode(t, denied, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
	list := serveJSON(router, http.MethodGet, "/api/weighing/tasks", "", unassigned)
	if strings.Contains(list.Body.String(), "RW-OLD-20261027-0001") {
		t.Fatalf("unassigned user saw historic task: %s", list.Body.String())
	}

	// A historical-code pending task still blocks disabling the local site.
	disable := serveJSON(router, http.MethodPut, "/api/weighing/site-config", `{"code":"CURRENT","name":"本厂","scaleCode":"WB01","scaleName":"地磅","enabled":0,"version":1}`, admin)
	assertEnvelopeCode(t, disable, http.StatusOK, http.StatusBadRequest, "不能停用：1 笔未完成任务")

	// The database itself rejects duplicate active plates regardless of snapshot code.
	err = db.GORM.Exec(`INSERT INTO wb_transport_task (task_no,mode,plate,direction,status,site_code,scale_code,material_id,party_id,business_snapshot,version,create_time,update_time,actor_id,actor_name)
 SELECT 'RW-CURRENT-20261027-0001','normal',plate,direction,status,'CURRENT','WB01',material_id,party_id,business_snapshot,1,now(),now(),1,'管理员' FROM wb_transport_task WHERE id=?`, taskID).Error
	if err == nil {
		t.Fatal("duplicate active plate accepted across historical codes")
	}
}
