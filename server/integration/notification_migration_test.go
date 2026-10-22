//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/notification"
	"github.com/pressly/goose/v3"
)

func TestPostgresNotificationUpgradePreservesHistory(t *testing.T) {
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	directory := filepath.Join(projectRoot(t), "migrations/schema")
	if err := goose.DownToContext(context.Background(), db.SQL, directory, 7); err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec(`INSERT INTO sys_notification (id,title,content,source_type,publish_time,create_time) VALUES (1,'升级前消息','保留正文','MANUAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec(`INSERT INTO sys_notification_recipient (notification_id,user_id,is_read,read_time) VALUES (1,1,1,CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(context.Background(), db.SQL, directory); err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("SELECT setval(pg_get_serial_sequence('sys_notification', 'id'), 1)").Error; err != nil {
		t.Fatal(err)
	}
	deps := testDependencies(t, db, t.TempDir())
	defer deps.Notification.Close()
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	response := serveJSON(router, "GET", "/api/system/notification/1", "", token)
	assertEnvelopeCode(t, response, 200, 200, "success")
	var result struct {
		Data notification.Notification `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Title != "升级前消息" || result.Data.Content != "保留正文" || result.Data.IsRead != 1 || result.Data.JumpPath != "" {
		t.Fatalf("upgrade changed notification: %s", response.Body.String())
	}
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/system/notification", `{"title":"升级后消息","content":"新增正文","userIds":[1]}`, token), 200, 200, "success")
	page := serveJSON(router, "GET", "/api/system/notification/page", "", token)
	if pageTotal(t, page.Body.Bytes()) != 2 {
		t.Fatalf("post-upgrade insert failed: %s", page.Body.String())
	}
}
