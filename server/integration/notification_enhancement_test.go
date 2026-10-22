//go:build integration

package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/notification"
	"github.com/EziosWJ/weighbridge/server/internal/platform/database"
)

func TestPostgresNotificationSessionExpiry(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	defer deps.Notification.Close()
	tokens, err := auth.NewTokenManager(auth.TokenConfig{SigningKey: "integration-test-signing-key", Issuer: "base-go-api", Audience: "react-admin", TTL: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	deps.Auth, err = auth.NewService(auth.NewRepository(db.GORM), tokens)
	if err != nil {
		t.Fatal(err)
	}
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	events, stop := notificationStream(t, server.URL+"/api/system/notification/events", loginAdmin(t, router))
	defer stop()
	awaitChange(t, events, "ready")
	awaitStreamClosed(t, events)
}

func awaitStreamClosed(t *testing.T, events <-chan string) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("stream did not close")
		}
	}
}

func TestNotificationRealtimePostgresContract(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	defer deps.Notification.Close()
	notificationRealtimeContract(t, db, deps)
}
func notificationRealtimeContract(t *testing.T, db *database.Database, deps app.Dependencies) {
	t.Helper()
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	defer server.Close()
	admin := loginAdmin(t, router)
	events, stop := notificationStream(t, server.URL+"/api/system/notification/events", admin)
	defer stop()
	awaitChange(t, events, "ready")
	time.Sleep(150 * time.Millisecond)
	response := serveJSON(router, "POST", "/api/system/notification", `{"title":"实时消息","content":"实时内容","userIds":[1]}`, admin)
	assertEnvelopeCode(t, response, 200, 200, "success")
	awaitChange(t, events, "notification")
	assertEnvelopeCode(t, serveJSON(router, "PUT", "/api/system/notification/read-all", "", admin), 200, 200, "success")
	awaitChange(t, events, "read")
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/system/user", `{"username":"realtime-user","nickname":"接收用户","status":1}`, admin), 200, 200, "success")
	other := loginUser(t, router, "realtime-user", "admin123")
	otherEvents, stopOther := notificationStream(t, server.URL+"/api/system/notification/events", other)
	defer stopOther()
	awaitChange(t, otherEvents, "ready")
	publicEvents, stopPublic := notificationStream(t, server.URL+"/api/public/announcement/events", "")
	defer stopPublic()
	awaitChange(t, publicEvents, "ready")
	// Audit failure must roll back the role association and its notification.

	if err := db.GORM.Exec(`ALTER TABLE sys_oper_log ADD CONSTRAINT reject_notification_audit CHECK (operation_type <> 'user.roles') NOT VALID`).Error; err != nil {
		t.Fatal(err)
	}
	failed := serveJSON(router, "PUT", "/api/system/user/2/roles", `{"roleIds":[1]}`, admin)
	if failed.Code != 500 {
		t.Fatalf("audit failure should reject role change: %s", failed.Body.String())
	}
	otherPage := serveJSON(router, "GET", "/api/system/notification/page", "", other)
	if pageTotal(t, otherPage.Body.Bytes()) != 0 {
		t.Fatalf("rolled back notification visible: %s", otherPage.Body.String())
	}
	noChange(t, otherEvents)

	if err := db.GORM.Exec("ALTER TABLE sys_oper_log DROP CONSTRAINT reject_notification_audit").Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(router, "PUT", "/api/system/user/2/roles", `{"roleIds":[1]}`, admin), 200, 200, "success")
	awaitChange(t, otherEvents, "notification")
	assertEnvelopeCode(t, serveJSON(router, "PUT", "/api/system/user/2/roles", `{"roleIds":[1]}`, admin), 200, 200, "success")
	noChange(t, otherEvents)
	if total := pageTotal(t, serveJSON(router, "GET", "/api/system/notification/page", "", other).Body.Bytes()); total != 1 {
		t.Fatalf("duplicate role save produced notification: %d", total)
	}
	noChange(t, publicEvents)
	invalid := serveJSON(router, "POST", "/api/system/notification", `{"title":"跳转","content":"内容","userIds":[1],"jumpPath":"//evil.example/path"}`, admin)
	assertEnvelopeCode(t, invalid, 200, 400, "参数错误")
	expiredAt := time.Now().Add(300 * time.Millisecond).UTC().Format(time.RFC3339Nano)
	draft := serveJSON(router, "POST", "/api/system/announcement", fmt.Sprintf(`{"title":"自动到期","content":"维护","scope":"PUBLIC","expiresAt":%q}`, expiredAt), admin)
	assertEnvelopeCode(t, draft, 200, 200, "success")
	var a struct {
		Data notification.Announcement `json:"data"`
	}
	if err := json.Unmarshal(draft.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(router, "PUT", fmt.Sprintf("/api/system/announcement/%d/publish", a.Data.ID), "", admin), 200, 200, "success")
	awaitChange(t, publicEvents, "announcement")
	awaitChange(t, publicEvents, "announcement")
	assertEnvelopeCode(t, serveJSON(router, "GET", fmt.Sprintf("/api/public/announcement/%d", a.Data.ID), "", ""), 200, 404, "公告已失效或不存在")
	// A revoked session cannot receive its next event, even before a heartbeat.
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/auth/logout", "", other), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/system/notification", `{"title":"退出后消息","content":"不能推送","userIds":[2]}`, admin), 200, 200, "success")
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-otherEvents:
			if !ok {
				return
			}
			if strings.Contains(line, `"type":"notification"`) {
				t.Fatal("revoked session received notification")
			}
		case <-timer.C:
			t.Fatal("revoked stream did not close")
		}
	}
}

func noChange(t *testing.T, events <-chan string) {
	t.Helper()
	timer := time.NewTimer(75 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-events:
			if !ok {
				t.Fatal("stream unexpectedly closed")
			}
			if strings.HasPrefix(line, "data:") {
				t.Fatalf("unexpected event: %s", line)
			}
		case <-timer.C:
			return
		}
	}
}

func notificationStream(t *testing.T, url, token string) (<-chan string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", token)
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" {
		cancel()
		response.Body.Close()
		t.Fatalf("stream status %d", response.StatusCode)
	}
	changes := make(chan string, 100)
	go func() {
		defer close(changes)
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			select {
			case changes <- line:
			case <-ctx.Done():
				return
			}
		}
	}()
	return changes, cancel
}
func awaitChange(t *testing.T, events <-chan string, kind string) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-events:
			if !ok {
				t.Fatalf("stream closed before %s", kind)
			}
			if strings.Contains(line, `"type":"`+kind+`"`) {
				return
			}
		case <-timer.C:
			t.Fatalf("no %s event within 2 seconds", kind)
		}
	}
}

func TestNotificationAnnouncementPostgresContract(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	announcementContract(t, router)
}

func announcementContract(t *testing.T, router http.Handler) {
	t.Helper()
	admin := loginAdmin(t, router)
	create := serveJSON(router, "POST", "/api/system/announcement", `{"title":"维护公告","content":"今晚维护","scope":"PUBLIC"}`, admin)
	assertEnvelopeCode(t, create, 200, 200, "success")
	var envelope struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &envelope); err != nil || envelope.Data.ID == 0 {
		t.Fatalf("create: %s", create.Body.String())
	}
	id := envelope.Data.ID
	public := serveJSON(router, "GET", "/api/public/announcement/page", "", "")
	assertEnvelopeCode(t, public, 200, 200, "success")
	if got := pageTotal(t, public.Body.Bytes()); got != 0 {
		t.Fatalf("draft visible: %s", public.Body.String())
	}
	assertEnvelopeCode(t, serveJSON(router, "PUT", fmt.Sprintf("/api/system/announcement/%d/publish", id), "", admin), 200, 200, "success")
	public = serveJSON(router, "GET", "/api/public/announcement/page", "", "")
	if got := pageTotal(t, public.Body.Bytes()); got != 1 {
		t.Fatalf("published missing: %s", public.Body.String())
	}
	if strings.Contains(public.Body.String(), "publisherId") {
		t.Fatalf("public announcement exposed administrator identity: %s", public.Body.String())
	}
	edit := serveJSON(router, "PUT", fmt.Sprintf("/api/system/announcement/%d", id), `{"title":"修改","content":"修改","scope":"PUBLIC"}`, admin)
	assertEnvelopeCode(t, edit, 200, 400, "参数错误")
	assertEnvelopeCode(t, serveJSON(router, "PUT", fmt.Sprintf("/api/system/announcement/%d/withdraw", id), "", admin), 200, 200, "success")
	detail := serveJSON(router, "GET", fmt.Sprintf("/api/public/announcement/%d", id), "", "")
	assertEnvelopeCode(t, detail, 200, 404, "公告已失效或不存在")
	public = serveJSON(router, "GET", "/api/public/announcement/page", "", "")
	if got := pageTotal(t, public.Body.Bytes()); got != 0 {
		t.Fatalf("withdrawn visible: %s", public.Body.String())
	}
	internal := serveJSON(router, "POST", "/api/system/announcement", `{"title":"站内公告","content":"内部内容","scope":"INTERNAL"}`, admin)
	assertEnvelopeCode(t, internal, 200, 200, "success")
	if err := json.Unmarshal(internal.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	internalID := envelope.Data.ID
	assertEnvelopeCode(t, serveJSON(router, "PUT", fmt.Sprintf("/api/system/announcement/%d/publish", internalID), "", admin), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, "GET", fmt.Sprintf("/api/public/announcement/%d", internalID), "", ""), 200, 404, "公告已失效或不存在")
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/system/notification", `{"title":"全体快照","content":"发布时账号","allUsers":true}`, admin), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/system/user", `{"username":"announcement-new-user","nickname":"新用户","status":1}`, admin), 200, 200, "success")
	user := loginUser(t, router, "announcement-new-user", "admin123")
	if total := pageTotal(t, serveJSON(router, "GET", "/api/system/notification/page", "", user).Body.Bytes()); total != 0 {
		t.Fatalf("new user received historical broadcast: %d", total)
	}
	assertEnvelopeCode(t, serveJSON(router, "GET", "/api/system/notification/1", "", user), 200, 404, "通知不存在")
	assertEnvelopeCode(t, serveJSON(router, "PUT", "/api/system/notification/1/read", "", user), 200, 404, "通知不存在")
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/system/notification", `{"title":"无效收件人","content":"应整体失败","userIds":[1,9999]}`, admin), 200, 400, "参数错误")
	if total := pageTotal(t, serveJSON(router, "GET", "/api/system/notification/page", "", admin).Body.Bytes()); total != 1 {
		t.Fatalf("invalid recipient caused partial delivery: %d", total)
	}
	internalPage := serveJSON(router, "GET", "/api/system/announcement/page", "", user)
	if pageTotal(t, internalPage.Body.Bytes()) != 1 {
		t.Fatalf("new user cannot see current internal announcement: %s", internalPage.Body.String())
	}
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/system/announcement", `{"title":"越权","content":"越权","scope":"PUBLIC"}`, user), 403, 403, "无权限")
	adminPage := serveJSON(router, "GET", "/api/system/announcement-admin/page", "", admin)
	if pageTotal(t, adminPage.Body.Bytes()) != 2 {
		t.Fatalf("admin history: %s", adminPage.Body.String())
	}
	// Preserve the existing role-management behavior for disabled accounts.
	assertEnvelopeCode(t, serveJSON(router, "PATCH", "/api/system/user/2/status", `{"status":0}`, admin), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, "PUT", "/api/system/user/2/roles", `{"roleIds":[1]}`, admin), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, "PATCH", "/api/system/user/2/status", `{"status":1}`, admin), 200, 200, "success")
	restored := loginUser(t, router, "announcement-new-user", "admin123")
	if total := pageTotal(t, serveJSON(router, "GET", "/api/system/notification/page", "", restored).Body.Bytes()); total != 1 {
		t.Fatalf("disabled account lost role notification: %d", total)
	}
}

func pageTotal(t *testing.T, body []byte) int {
	t.Helper()
	var e struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatal(err)
	}
	return e.Data.Total
}
