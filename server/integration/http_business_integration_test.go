//go:build integration

package integration

import (
	"bytes"

	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"

	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
)

func TestPostgresSharedHTTPBusinessContract(t *testing.T) {
	directory := t.TempDir()
	temporary := startBusinessPostgres(t)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	dependencies := testDependencies(t, database, filepath.Join(directory, "uploads"))
	router, err := app.Build(testAPIConfig(), database, dependencies)
	if err != nil {
		t.Fatalf("build Postgres API: %v", err)
	}

	failedLogin := serveJSON(router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"wrong"}`, "")
	assertEnvelopeCode(t, failedLogin, http.StatusOK, 400, "用户名或密码错误")
	adminToken := loginAdmin(t, router)
	me := serveJSON(router, http.MethodGet, "/api/auth/me", "", adminToken)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"roleCode":"ADMIN"`) {
		t.Fatalf("Postgres current user = %d %s", me.Code, me.Body.String())
	}
	menus := serveJSON(router, http.MethodGet, "/api/auth/menus", "", adminToken)
	if menus.Code != http.StatusOK || !strings.Contains(menus.Body.String(), `"menuName":"系统管理"`) {
		t.Fatalf("Postgres current menus = %d %s", menus.Code, menus.Body.String())
	}
	createdRole := serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"Postgres运营","roleCode":"POSTGRES-OPS","status":1,"sortOrder":2}`, adminToken)
	assertEnvelopeCode(t, createdRole, http.StatusOK, 200, "success")
	roleMenus := serveJSON(router, http.MethodPut, "/api/system/role/2/menus", `{"menuIds":[1,2]}`, adminToken)
	assertEnvelopeCode(t, roleMenus, http.StatusOK, 200, "success")
	roleDetail := serveJSON(router, http.MethodGet, "/api/system/role/2", "", adminToken)
	if roleDetail.Code != http.StatusOK || !strings.Contains(roleDetail.Body.String(), `"menuIds":[1,2]`) {
		t.Fatalf("Postgres role detail = %d %s", roleDetail.Code, roleDetail.Body.String())
	}

	createdDept := serveJSON(router, http.MethodPost, "/api/system/dept", `{"parentId":1,"deptName":"Postgres研发部","deptCode":"POSTGRES-RND","status":1}`, adminToken)
	assertEnvelopeCode(t, createdDept, http.StatusOK, 200, "success")
	createdUser := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"postgres-user","nickname":"Postgres用户","deptId":2,"status":1}`, adminToken)
	assertEnvelopeCode(t, createdUser, http.StatusOK, 200, "success")
	assignedRoles := serveJSON(router, http.MethodPut, "/api/system/user/2/roles", `{"roleIds":[2]}`, adminToken)
	assertEnvelopeCode(t, assignedRoles, http.StatusOK, 200, "success")
	userToken := loginUser(t, router, "postgres-user", "admin123")

	items := serveJSON(router, http.MethodGet, "/api/system/dict/USER_STATUS/items", "", adminToken)
	if items.Code != http.StatusOK || !strings.Contains(items.Body.String(), `"value":"1"`) {
		t.Fatalf("Postgres seed dict items = %d %s", items.Code, items.Body.String())
	}
	createdType := serveJSON(router, http.MethodPost, "/api/system/dict-type", `{"dictName":"Postgres环境","dictCode":"POSTGRES_ENV"}`, adminToken)
	assertEnvelopeCode(t, createdType, http.StatusOK, 200, "success")
	createdConfig := serveJSON(router, http.MethodPost, "/api/system/config", `{"configName":"Postgres演示","configKey":"postgres.demo","configValue":"true"}`, adminToken)
	assertEnvelopeCode(t, createdConfig, http.StatusOK, 200, "success")
	configPage := serveJSON(router, http.MethodGet, "/api/system/config/page?page=1&pageSize=20&configKey=postgres.demo", "", adminToken)
	if configPage.Code != http.StatusOK || !strings.Contains(configPage.Body.String(), `"configKey":"postgres.demo"`) {
		t.Fatalf("Postgres config page = %d %s", configPage.Code, configPage.Body.String())
	}
	key := serveJSON(router, http.MethodGet, "/api/system/config/key/system.log-clear-enabled", "", adminToken)
	assertEnvelopeCode(t, key, http.StatusOK, 200, "success")
	disabledConfig := serveJSON(router, http.MethodPatch, "/api/system/config/1/status", `{"status":0}`, adminToken)
	assertEnvelopeCode(t, disabledConfig, http.StatusOK, 200, "success")
	missingKey := serveJSON(router, http.MethodGet, "/api/system/config/key/system.log-clear-enabled", "", adminToken)
	assertEnvelopeCode(t, missingKey, http.StatusOK, 404, "数据不存在")

	loginPage := serveJSON(router, http.MethodGet, "/api/system/login-log/page?page=1&pageSize=20", "", adminToken)
	if loginPage.Code != http.StatusOK || !strings.Contains(loginPage.Body.String(), `"loginStatus":"FAIL"`) {
		t.Fatalf("Postgres login logs = %d %s", loginPage.Code, loginPage.Body.String())
	}
	operPage := serveJSON(router, http.MethodGet, "/api/system/oper-log/page?page=1&pageSize=20&operatorName=admin", "", adminToken)
	if operPage.Code != http.StatusOK || !strings.Contains(operPage.Body.String(), `"operatorName":"admin"`) {
		t.Fatalf("Postgres operation logs = %d %s", operPage.Code, operPage.Body.String())
	}

	upload := serveMultipartFile(router, "/api/system/file/upload", adminToken, "postgres.txt", "Postgres file content")
	assertEnvelopeCode(t, upload, http.StatusOK, 200, "success")
	var uploadPayload struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploadPayload); err != nil || uploadPayload.Data.ID <= 0 {
		t.Fatalf("decode Postgres upload = %s, error=%v", upload.Body.String(), err)
	}
	download := servePlain(t, router, http.MethodGet, "/api/system/file/"+itoa(uploadPayload.Data.ID)+"/download", adminToken)
	if download.Code != http.StatusOK || download.Body.String() != "Postgres file content" {
		t.Fatalf("Postgres file download = %d %q", download.Code, download.Body.String())
	}

	publish := serveJSON(router, http.MethodPost, "/api/system/notification", `{"title":"Postgres通知","content":"通知内容","userIds":[2]}`, adminToken)
	assertEnvelopeCode(t, publish, http.StatusOK, 200, "success")
	page := serveJSON(router, http.MethodGet, "/api/system/notification/page?page=1&pageSize=20", "", userToken)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `"title":"Postgres通知"`) || !strings.Contains(page.Body.String(), `"isRead":0`) {
		t.Fatalf("Postgres notification page = %d %s", page.Code, page.Body.String())
	}
	unread := serveJSON(router, http.MethodGet, "/api/system/notification/unread-count", "", userToken)
	if unread.Code != http.StatusOK || !strings.Contains(unread.Body.String(), `"data":2`) {
		t.Fatalf("Postgres unread count = %d %s", unread.Code, unread.Body.String())
	}
	detail := serveJSON(router, http.MethodGet, "/api/system/notification/2", "", userToken)
	assertEnvelopeCode(t, detail, http.StatusOK, 200, "success")
	unreadAfterDetail := serveJSON(router, http.MethodGet, "/api/system/notification/unread-count", "", userToken)
	if unreadAfterDetail.Code != http.StatusOK || !strings.Contains(unreadAfterDetail.Body.String(), `"data":1`) {
		t.Fatalf("Postgres unread count after detail = %d %s", unreadAfterDetail.Code, unreadAfterDetail.Body.String())
	}

	var auditCount int64
	if err := database.GORM.Table("sys_oper_log").Where("operator_id = 1 AND operation_status = 'SUCCESS'").Count(&auditCount).Error; err != nil {
		t.Fatalf("count Postgres operation audits: %v", err)
	}
	if auditCount < 5 {
		t.Fatalf("Postgres successful operation audit count = %d, want at least 5", auditCount)
	}
}

func serveMultipartFile(router http.Handler, path, token, filename, content string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		panic(err)
	}
	_, _ = io.WriteString(part, content)
	_ = writer.WriteField("businessModule", "postgres")
	if err := writer.Close(); err != nil {
		panic(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func itoa(value int64) string {
	return fmt.Sprintf("%d", value)
}
