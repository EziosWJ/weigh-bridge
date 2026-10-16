//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/notification"
	"github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"gorm.io/gorm"
)

func TestSQLiteNotificationBusinessEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	businessEntryContract(t, db, sqliteDependencies(t, db, t.TempDir()))
}
func TestNotificationBusinessEntryPostgres(t *testing.T) {
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	businessEntryContract(t, db, testDependencies(t, db, t.TempDir()))
}
func businessEntryContract(t *testing.T, db *database.Database, deps app.Dependencies) {
	t.Helper()
	deps.Notification.Close()
	writer := notification.NewRepository(db.GORM)
	service, err := notification.NewService(writer)
	if err != nil {
		t.Fatal(err)
	}
	deps.Notification = service
	defer service.Close()
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	token := loginAdmin(t, router)
	events, stop := notificationStream(t, server.URL+"/api/system/notification/events", token)
	defer stop()
	awaitChange(t, events, "ready")
	// Arrange the result of a business transaction through the integration entry;
	// observe delivery and persistence only through the application's HTTP boundary.
	var afterCommit func()
	err = db.GORM.Transaction(func(tx *gorm.DB) error {
		var err error
		afterCommit, err = writer.RecordBusiness(context.Background(), tx, notification.PublishInput{Title: "业务结果", Content: "业务已完成", UserIDs: []int64{1}, JumpPath: "/dashboard"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	afterCommit()
	awaitChange(t, events, "notification")
	response := serveJSON(router, "GET", "/api/system/notification/1", "", token)
	assertEnvelopeCode(t, response, 200, 200, "success")
	var result struct {
		Data notification.Notification `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.SourceType != "BUSINESS" || result.Data.JumpPath != "/dashboard" || result.Data.IsRead != 1 {
		t.Fatalf("business notification: %s", response.Body.String())
	}
	awaitChange(t, events, "read")
	rolledBack := db.GORM.Transaction(func(tx *gorm.DB) error {
		var err error
		afterCommit, err = writer.RecordBusiness(context.Background(), tx, notification.PublishInput{Title: "回滚结果", Content: "不应发送", UserIDs: []int64{1}})
		if err != nil {
			return err
		}
		return errors.New("business failed")
	})
	if rolledBack == nil {
		t.Fatal("fixture business failure did not rollback")
	}
	noChange(t, events)
	if total := pageTotal(t, serveJSON(router, "GET", "/api/system/notification/page", "", token).Body.Bytes()); total != 1 {
		t.Fatalf("business rollback left notification: %d", total)
	}
	service.Close()
	awaitStreamClosed(t, events)
}
