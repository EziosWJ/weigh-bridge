//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/notification"
	"github.com/pressly/goose/v3"
)

func TestPostgresNotificationUpgradePreservesHistory(t *testing.T) {
	temporary := startPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	schema := filepath.Join(projectRoot(t), "migrations/schema")
	seed := filepath.Join(projectRoot(t), "migrations/seed")
	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpToContext(ctx, db.SQL, schema, 16); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_seed_db_version")
	if err := goose.UpToContext(ctx, db.SQL, seed, 12); err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec(`INSERT INTO sys_notification (id,title,content,source_type,publish_time,create_time) VALUES (1,'升级前消息','保留正文','MANUAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec(`INSERT INTO sys_notification_recipient (notification_id,user_id,is_read,read_time) VALUES (1,1,1,CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec(`SELECT setval(pg_get_serial_sequence('sys_notification','id'), (SELECT MAX(id) FROM sys_notification), true)`).Error; err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	if err := goose.UpContext(ctx, db.SQL, schema); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_seed_db_version")
	if err := goose.UpContext(ctx, db.SQL, seed); err != nil {
		t.Fatal(err)
	}

	// Version 17 deliberately refuses an in-place rollback. Confirm the
	// refusal leaves the upgrade and notification history intact.
	goose.SetTableName("goose_schema_db_version")
	if err := goose.DownToContext(ctx, db.SQL, schema, 7); err == nil || !strings.Contains(err.Error(), "restoring the previous database backup") {
		t.Fatalf("schema 17 rollback should require restoring the previous database backup, got %v", err)
	}
	var schemaVersion int64
	if err := db.GORM.Table("goose_schema_db_version").Select("MAX(version_id)").Where("is_applied=TRUE").Scan(&schemaVersion).Error; err != nil || schemaVersion != 17 {
		t.Fatalf("schema version after refused rollback=%d err=%v", schemaVersion, err)
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
