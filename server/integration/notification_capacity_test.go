//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/platform/database"
)

func TestNotificationCapacityPostgres(t *testing.T) {
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	defer deps.Notification.Close()
	notificationCapacity(t, db, deps)
}
func notificationCapacity(t *testing.T, db *database.Database, deps app.Dependencies) {
	t.Helper()
	// Arrange accounts directly; all assertions exercise HTTP and browser behavior.
	var password string
	if err := db.GORM.Table("sys_user").Where("id=1").Pluck("password", &password).Error; err != nil {
		t.Fatal(err)
	}
	users := make([]map[string]any, 999)
	for i := range users {
		users[i] = map[string]any{"username": fmt.Sprintf("capacity-%04d", i), "nickname": "容量测试用户", "password": password, "status": 1}
	}
	if err := db.GORM.Table("sys_user").CreateInBatches(users, 100).Error; err != nil {
		t.Fatal(err)
	}
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	token := loginAdmin(t, router)
	for _, path := range []string{"/health", "/ready"} {
		if result := serveJSON(router, "GET", path, "", ""); result.Code != 200 {
			t.Fatalf("capacity service %s not ready: %s", path, result.Body.String())
		}
	}
	arrivals := make(chan time.Time, 98)
	// The browser adds one private and one public stream, for 100 total.
	for i := 0; i < 98; i++ {
		changes, cancel := notificationStream(t, server.URL+"/api/system/notification/events", token)
		defer cancel()
		awaitChange(t, changes, "ready")
		go func() {
			for line := range changes {
				if strings.Contains(line, `"type":"notification"`) {
					arrivals <- time.Now()
					return
				}
			}
		}()
	}
	payload, _ := json.Marshal(map[string]string{"apiURL": server.URL, "token": token})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", filepath.Join(projectRoot(t), "../web/tests/notification-capacity.mjs"))
	command.Stdin = bytes.NewReader(payload)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser capacity: %v\n%s", err, output)
	}
	var report map[string]any
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("browser report: %v\n%s", err, output)
	}
	successMs := int64(report["successTimeUnixMs"].(float64))
	maxProtocolMs := int64(0)
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for i := 0; i < 98; i++ {
		select {
		case arrived := <-arrivals:
			delay := arrived.UnixMilli() - successMs
			if delay > maxProtocolMs {
				maxProtocolMs = delay
			}
		case <-timeout.C:
			t.Fatal("capacity streams did not receive notification")
		}
	}
	if maxProtocolMs > 2000 {
		t.Fatalf("protocol update %dms", maxProtocolMs)
	}
	history := serveJSON(router, "GET", "/api/system/notification-admin/page", "", token)
	var page struct {
		Data struct {
			Records []struct {
				RecipientCount int64 `json:"recipientCount"`
			} `json:"records"`
		} `json:"data"`
	}
	if err := json.Unmarshal(history.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data.Records) != 1 || page.Data.Records[0].RecipientCount != 1000 {
		t.Fatalf("capacity recipients: %s", history.Body.String())
	}
	report["database"] = db.GORM.Dialector.Name()
	report["connections"] = 100
	report["enabledAccounts"] = 1000
	report["maxProtocolUpdateMs"] = maxProtocolMs
	report["goVersion"] = runtime.Version()
	report["os"] = runtime.GOOS
	report["arch"] = runtime.GOARCH
	report["cpuCount"] = runtime.NumCPU()
	report["maxOpenDBConnections"] = db.SQL.Stats().MaxOpenConnections
	report["measuredAt"] = time.Now().UTC()
	data, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(filepath.Join(os.TempDir(), "notification-capacity-"+db.GORM.Dialector.Name()+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("capacity report:\n%s", data)
}
