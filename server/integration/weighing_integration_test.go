//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	"github.com/pressly/goose/v3"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresWeighingFoundation(t *testing.T) {
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()
	checkWeighingFoundation(t, database, testDependencies(t, database, t.TempDir()))
	// The local-only cutover is intentionally not reversible in place. The
	// migration must refuse rollback and leave confirmed history untouched.
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	schema := filepath.Join(projectRoot(t), "migrations/schema")
	// Reapply the widening migration to populated history and verify references.
	var ticketsBefore int64
	if err := database.GORM.Table("wb_ticket").Count(&ticketsBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := goose.DownToContext(context.Background(), database.SQL, schema, 11); err == nil || !strings.Contains(err.Error(), "restoring the previous database backup") {
		t.Fatalf("schema 17 rollback should require restoring the previous database backup, got %v", err)
	}
	var ticketsAfter int64
	if err := database.GORM.Table("wb_ticket").Count(&ticketsAfter).Error; err != nil || ticketsBefore == 0 || ticketsBefore != ticketsAfter {
		t.Fatalf("refused local-only rollback changed ticket history: %d -> %d %v", ticketsBefore, ticketsAfter, err)
	}
}

func TestPostgresWeighingMasterMigrationRollbackRequiresRestore(t *testing.T) {
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()
	if err := database.GORM.Exec("INSERT INTO wb_material_category (name, create_by, update_by) VALUES ('回退分类',1,1)").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.Exec("INSERT INTO wb_material (code,name,category_id,create_by,update_by) SELECT 'ROLLBACK','回退物料',id,1,1 FROM wb_material_category").Error; err != nil {
		t.Fatal(err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	schema := filepath.Join(projectRoot(t), "migrations/schema")
	goose.SetTableName("goose_schema_db_version")
	if err := goose.DownToContext(context.Background(), database.SQL, schema, 9); err == nil || !strings.Contains(err.Error(), "restoring the previous database backup") {
		t.Fatalf("schema 17 rollback should be explicitly unsupported, got %v", err)
	}
	var categories, materials, version int64
	if err := database.GORM.Table("wb_material_category").Count(&categories).Error; err != nil || categories != 1 {
		t.Fatalf("refused rollback changed material categories: count=%d err=%v", categories, err)
	}
	if err := database.GORM.Table("wb_material").Count(&materials).Error; err != nil || materials != 1 {
		t.Fatalf("refused rollback changed materials: count=%d err=%v", materials, err)
	}
	if err := database.GORM.Table("goose_schema_db_version").Select("MAX(version_id)").Where("is_applied=TRUE").Scan(&version).Error; err != nil || version != 17 {
		t.Fatalf("schema version after refused rollback=%d err=%v", version, err)
	}
}

func checkWeighingFoundation(t *testing.T, database *platformdatabase.Database, deps app.Dependencies) {
	t.Helper()
	for _, table := range []string{"wb_material_category", "wb_material", "wb_party", "wb_party_contact", "wb_number_sequence"} {
		var count int64
		if err := database.GORM.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("empty %s: count=%d, error=%v", table, count, err)
		}
	}
	var scales int64
	if err := database.GORM.Table("wb_scale").Count(&scales).Error; err != nil || scales != 0 {
		t.Fatalf("scale identity: count=%d error=%v", scales, err)
	}
	repo := weighing.NewRepository(database.GORM)
	imageDir := t.TempDir()
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, id int64) (agentbridge.Config, error) {
		return repo.AgentConfig(ctx, id, imageDir)
	})
	deps.Weighing = weighing.NewService(repo, hub)
	deps.Camera, _, _ = newCaptureService(t, database, t.TempDir())
	router, err := app.Build(testAPIConfig(), database, deps)
	if err != nil {
		t.Fatal(err)
	}
	adminToken := loginAdmin(t, router)
	ensureLocalSite(t, router, adminToken, 1, "SITE01", "原厂区", "SCALE01", "一号地磅", "integration-agent")
	testLocalSetup(t, database, router, adminToken, 1)
	addCaptureCameras(t, database, "rtsp://camera/foundation-1", "rtsp://camera/foundation-2")
	checkWeighingMasters(t, router, adminToken)
	deviceConfig, err := repo.AgentConfig(context.Background(), 1, imageDir)
	if err != nil {
		t.Fatal(err)
	}
	device := startWeighingAgentWithConfig(t, router, hub, deviceConfig)
	checkWeighingFirstConfirmation(t, router, adminToken, database, device)
	created := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"weighing-operator","nickname":"操作员","status":1}`, adminToken)
	assertEnvelopeCode(t, created, 200, 200, "success")
	var userID int64
	if err := database.GORM.Table("sys_user").Select("id").Where("username=?", "weighing-operator").Scan(&userID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.Exec("INSERT INTO sys_user_role (user_id, role_id) SELECT ?, id FROM sys_role WHERE role_code='WEIGHING_OPERATOR'", userID).Error; err != nil {
		t.Fatal(err)
	}
	rbacRepo := rbac.NewRepository(database.GORM)
	codes, err := rbacRepo.PermissionCodes(context.Background(), userID)
	if err != nil || !slices.Contains(codes, "weighing:confirm") || slices.Contains(codes, "weighing:master:write") || slices.Contains(codes, "system:role") {
		t.Fatalf("operator grants=%v, error=%v", codes, err)
	}
	var tokenResponse struct {
		Data struct {
			TokenValue string `json:"tokenValue"`
		} `json:"data"`
	}
	login := serveJSON(router, http.MethodPost, "/api/auth/login", `{"username":"weighing-operator","password":"admin123"}`, "")
	if err := json.Unmarshal(login.Body.Bytes(), &tokenResponse); err != nil || tokenResponse.Data.TokenValue == "" {
		t.Fatalf("operator login: %s, error=%v", login.Body.String(), err)
	}
	for _, path := range []string{"/api/system/role/1/menus", "/api/system/user/1/roles", "/api/system/user/1/reset-password", "/api/system/menu/1"} {
		response := serveJSON(router, http.MethodPut, path, `{}`, tokenResponse.Data.TokenValue)
		assertEnvelopeCode(t, response, 403, 403, "无权执行此操作")
	}
	self := serveJSON(router, http.MethodPut, "/api/system/user/me/avatar", `{"avatar":"/test-avatar.png"}`, tokenResponse.Data.TokenValue)
	assertEnvelopeCode(t, self, 200, 200, "success")
	denied := serveJSON(router, http.MethodPost, "/api/weighing/materials", `{"name":"越权资料"}`, tokenResponse.Data.TokenValue)
	assertEnvelopeCode(t, denied, 403, 403, "无权执行此操作")
	if err := database.GORM.Exec("DELETE FROM sys_role_menu WHERE role_id IN (SELECT id FROM sys_role WHERE role_code='WEIGHING_OPERATOR') AND menu_id IN (SELECT id FROM sys_menu WHERE permission_code='weighing:confirm')").Error; err != nil {
		t.Fatal(err)
	}
	// Revocation is enforced on the next request, even for an already issued token.
	if err := database.GORM.Exec("DELETE FROM sys_role_menu WHERE role_id IN (SELECT id FROM sys_role WHERE role_code='WEIGHING_OPERATOR') AND menu_id IN (SELECT id FROM sys_menu WHERE permission_code IN ('weighing:print','weighing:reprint','weighing:export'))").Error; err != nil {
		t.Fatal(err)
	}
	var ticketID int64
	if err := database.GORM.Table("wb_ticket").Select("id").Order("id").Limit(1).Scan(&ticketID).Error; err != nil || ticketID == 0 {
		t.Fatalf("find generated ticket for permission check: %d %v", ticketID, err)
	}
	for _, request := range []struct{ method, path, body string }{
		{"POST", "/api/weighing/tasks/first-confirmation", `{}`},
		{"GET", "/api/weighing/tasks/export", ``},
		{"POST", fmt.Sprintf("/api/weighing/tickets/%d/print-attempts", ticketID), `{"requestId":"denied-print-0001","intent":"initial"}`},
	} {
		assertEnvelopeCode(t, serveJSON(router, request.method, request.path, request.body, tokenResponse.Data.TokenValue), 403, 403, "无权执行此操作")
	}
	disabledDeps := deps
	disabledDeps.Weighing = weighing.NewService(weighing.NewRepository(database.GORM), nil)
	disabledRouter, err := app.Build(testAPIConfig(), database, disabledDeps)
	if err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(disabledRouter, "POST", "/api/weighing/samples", `{"scaleId":1,"clientId":"disabled-test"}`, adminToken), 200, 400, weighing.ErrConditions.Error())
	codes, err = rbacRepo.PermissionCodes(context.Background(), userID)
	if err != nil || slices.Contains(codes, "weighing:confirm") {
		t.Fatalf("revoked grant retained: %v %v", codes, err)
	}
	checkBusinessOverview(t, database, router, adminToken, tokenResponse.Data.TokenValue)
}

func checkWeighingFirstConfirmation(t *testing.T, router http.Handler, token string, database *platformdatabase.Database, device *weighingAgent) {
	t.Helper()
	material := weighingData[weighing.Material](t, router, "POST", "/api/weighing/materials", `{"name":"首磅物料"}`, token)
	party := weighingData[weighing.Party](t, router, "POST", "/api/weighing/parties", `{"name":"首磅供收方","nature":"enterprise","roles":["supplier","customer"]}`, token)
	session := weighingData[weighing.SessionResult](t, router, "POST", "/api/weighing/scales/1/session/acquire", `{"clientId":"browser-a"}`, token)
	occupied := serveJSON(router, "POST", "/api/weighing/scales/1/session/acquire", `{"clientId":"browser-b"}`, token)
	assertEnvelopeCode(t, occupied, 200, 400, weighing.ErrOccupied.Error())
	business := map[string]any{"plate": "川A12345", "direction": "inbound", "materialId": material.ID, "partyId": party.ID}
	encode := func(v any) string {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	sample := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(map[string]any{"scaleId": 1, "clientId": "browser-a", "sessionToken": session.SessionToken, "business": business, "source": "device"}), token)
	if sample.Source != "server" || sample.Weight != 30000 || sample.Kind != "gross" {
		t.Fatalf("sample source and weight: %+v", sample)
	}

	var manualEvidence map[string]any
	if err := json.Unmarshal(sample.PlateEvidence, &manualEvidence); err != nil {
		t.Fatal(err)
	}
	if manualEvidence["source"] != "manual" || manualEvidence["candidate"] != "" {
		t.Fatalf("manual plate fabricated recognition: %+v", manualEvidence)
	}
	// Explicitly discarded samples remain unusable even after device recovery.
	revision := device.hub.State(1).Revision
	device.controlAction(`{"action":"unstable"}`)
	device.wait(func(v agentbridge.State) bool { return !v.Stable && v.Revision != revision })
	device.controlAction(`{"action":"stable"}`)
	device.wait(func(v agentbridge.State) bool { return v.Stable })
	assertEnvelopeCode(t, serveJSON(router, "POST", fmt.Sprintf("/api/weighing/samples/%d/invalidate", sample.ID), encode(map[string]any{"clientId": "browser-a", "sessionToken": session.SessionToken}), token), 200, 200, "success")
	staleInput := map[string]any{"scaleId": 1, "clientId": "browser-a", "sessionToken": session.SessionToken, "business": business, "sampleId": sample.ID, "requestId": "unstable-first-0001", "checks": map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}}
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/tasks/first-confirmation", encode(staleInput), token), 200, 400, weighing.ErrConditions.Error())
	device.plate("川A12346")
	sample = weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(map[string]any{"scaleId": 1, "clientId": "browser-a", "sessionToken": session.SessionToken, "business": business, "plateEvidence": map[string]any{"source": "agent", "scenePlate": "川A12345", "candidate": "川A12346"}}), token)
	var recognition map[string]any
	if err := json.Unmarshal(sample.PlateEvidence, &recognition); err != nil {
		t.Fatal(err)
	}
	if recognition["candidate"] != "川A12346" || recognition["confirmedPlate"] != "川A12345" {
		t.Fatalf("candidate/confirmed distinction lost: %+v", recognition)
	}
	checks := map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}
	input := map[string]any{"scaleId": 1, "clientId": "browser-a", "sessionToken": session.SessionToken, "business": business, "sampleId": sample.ID, "requestId": "first-request-0001", "checks": checks}
	// Time alone must not invalidate a sample. Age both legacy expiry and capture
	// timestamps while leaving the active session and device conditions intact.
	if err := database.GORM.Model(&weighing.Sample{}).Where("id=?", sample.ID).Updates(map[string]any{
		"captured_at": time.Now().UTC().Add(-time.Hour),
		"expires_at":  time.Now().UTC().Add(-time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	firstBody := encode(input)
	responses := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		go func() {
			responses <- serveJSON(router, "POST", "/api/weighing/tasks/first-confirmation", firstBody, token)
		}()
	}
	var first weighing.CommandResult
	for i := 0; i < 2; i++ {
		var response *httptest.ResponseRecorder
		select {
		case response = <-responses:
		case <-time.After(15 * time.Second):
			t.Fatal("concurrent first confirmations exceeded bounded wait")
		}
		assertEnvelopeCode(t, response, 200, 200, "success")
		var payload struct {
			Data weighing.CommandResult `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = payload.Data
		} else if first.TaskID != payload.Data.TaskID || first.RecordID != payload.Data.RecordID {
			t.Fatal("concurrent replay duplicated business records")
		}
	}

	task := weighingData[weighing.TaskDetail](t, router, "GET", "/api/weighing/tasks/"+itoa(first.TaskID), "", token)
	if !strings.HasPrefix(task.TaskNo, "RW") || task.Mode != "normal" || len(task.Records) != 1 || task.Records[0].Weight != 30000 {
		t.Fatalf("first confirmation: %+v", task)
	}
	takenOver := weighingData[weighing.SessionResult](t, router, "POST", "/api/weighing/scales/1/session/takeover", `{"clientId":"browser-b","reason":"测试接管"}`, token)
	replay := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/first-confirmation", encode(input), token)
	if replay.TaskID != first.TaskID || replay.RecordID != first.RecordID {
		t.Fatalf("deduplication changed identity: %+v %+v", first, replay)
	}
	input["requestId"] = "expired-request-0002"
	denied := serveJSON(router, "POST", "/api/weighing/tasks/first-confirmation", encode(input), token)
	assertEnvelopeCode(t, denied, 200, 400, weighing.ErrSession.Error())
	// A rejected request also replays as a business rejection, never a generic 500.
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/tasks/first-confirmation", encode(input), token), 200, 400, weighing.ErrSession.Error())
	op := weighingData[weighing.Operation](t, router, "GET", "/api/weighing/operations/first-request-0001", "", token)
	if op.Status != "committed" || op.TaskID == nil || *op.TaskID != first.TaskID {
		t.Fatalf("original result lookup: %+v", op)
	}
	missing := weighingData[weighing.Operation](t, router, "GET", "/api/weighing/operations/not-yet-visible-0001", "", token)
	if missing.Status != "not_found" {
		t.Fatalf("missing result must not imply rejection: %+v", missing)
	}
	input["requestId"] = "first-request-0001"
	input["sampleId"] = sample.ID + 1
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/tasks/first-confirmation", encode(input), token), 200, 400, weighing.ErrRequestKey.Error())
	used := serveJSON(router, "DELETE", "/api/weighing/materials/"+itoa(material.ID), `{"version":1}`, token)
	assertEnvelopeCode(t, used, 200, 400, weighing.ErrReferenced.Error())
	partyUpdate := fmt.Sprintf(`{"name":"首磅供收方已调整","nature":"enterprise","roles":["customer"],"version":%d}`, party.Version)
	partyUpdateResponse := serveJSON(router, "PUT", "/api/weighing/parties/"+itoa(party.ID), partyUpdate, token)
	if partyUpdateResponse.Code != 200 || !strings.Contains(partyUpdateResponse.Body.String(), `"code":200`) {
		t.Fatalf("party update failed: party=%+v input=%s response=%s", party, partyUpdate, partyUpdateResponse.Body.String())
	}
	assertEnvelopeCode(t, partyUpdateResponse, 200, 200, "success")
	var partyUpdateEnvelope struct {
		Data weighing.Party `json:"data"`
	}
	if err := json.Unmarshal(partyUpdateResponse.Body.Bytes(), &partyUpdateEnvelope); err != nil {
		t.Fatal(err)
	}
	updatedParty := partyUpdateEnvelope.Data
	if len(updatedParty.Roles) != 1 || updatedParty.Roles[0] != "customer" {
		t.Fatalf("party roles were not editable for future tasks: %+v", updatedParty)
	}
	firstSnapshot := weighingData[weighing.TaskDetail](t, router, "GET", "/api/weighing/tasks/"+itoa(first.TaskID), "", token)
	if firstSnapshot.Business.PartyName != party.Name || firstSnapshot.Business.PartyVersion != 1 {
		t.Fatalf("editing current party changed an existing task snapshot: %+v", firstSnapshot.Business)
	}
	assertEnvelopeCode(t, serveJSON(router, "DELETE", "/api/weighing/parties/"+itoa(party.ID), fmt.Sprintf(`{"version":%d}`, updatedParty.Version), token), 200, 400, weighing.ErrReferenced.Error())

	// A crossed date keeps the same task and first basis; current sample must still be fresh.
	if err := database.GORM.Exec("UPDATE wb_weigh_record SET confirmed_at=? WHERE id=?", time.Now().UTC().Add(-24*time.Hour), first.RecordID).Error; err != nil {
		t.Fatal(err)
	}
	conditions := map[string]any{"weight": "40", "stable": true, "evidence": true, "cameraPreset": "front"}
	device.weight(conditions["weight"].(string))
	sampleBody := map[string]any{"scaleId": 1, "clientId": "browser-b", "sessionToken": takenOver.SessionToken, "taskId": first.TaskID}
	invalidSecond := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(sampleBody), token)
	secondInput := map[string]any{"scaleId": 1, "clientId": "browser-b", "sessionToken": takenOver.SessionToken, "sampleId": invalidSecond.ID, "requestId": "invalid-original-0001", "version": 1, "checks": checks, "finalWeights": map[string]any{"gross": "50", "tare": "10"}}
	secondPath := "/api/weighing/tasks/" + itoa(first.TaskID) + "/second-confirmation"
	assertEnvelopeCode(t, serveJSON(router, "POST", secondPath, encode(secondInput), token), 200, 400, weighing.ErrWeight.Error())
	unchanged := weighingData[weighing.TaskDetail](t, router, "GET", "/api/weighing/tasks/"+itoa(first.TaskID), "", token)
	if unchanged.Status != "awaiting_second" || len(unchanged.Records) != 1 {
		t.Fatalf("invalid second confirmation wrote facts: %+v", unchanged)
	}
	conditions["weight"] = "10"
	device.weight(conditions["weight"].(string))
	secondSample := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(sampleBody), token)
	secondInput["sampleId"] = secondSample.ID
	secondInput["requestId"] = "second-request-0001"
	secondInput["finalWeights"] = map[string]any{"gross": "31", "tare": "11", "remark": ""}

	{
		if err := database.GORM.Exec(`ALTER TABLE sys_oper_log ADD CONSTRAINT reject_weighing_audit CHECK (operation_type <> 'second.confirm') NOT VALID`).Error; err != nil {
			t.Fatal(err)
		}
		failed := serveJSON(router, "POST", secondPath, encode(secondInput), token)
		if failed.Code != 500 {
			t.Fatalf("audit rollback response: %d %s", failed.Code, failed.Body.String())
		}
		rolledBack := weighingData[weighing.TaskDetail](t, router, "GET", "/api/weighing/tasks/"+itoa(first.TaskID), "", token)
		if rolledBack.Status != "awaiting_second" || len(rolledBack.Records) != 1 {
			t.Fatalf("partial second confirmation survived rollback: %+v", rolledBack)
		}
		if err := database.GORM.Exec("ALTER TABLE sys_oper_log DROP CONSTRAINT reject_weighing_audit").Error; err != nil {
			t.Fatal(err)
		}
	}
	completed := weighingData[weighing.CommandResult](t, router, "POST", secondPath, encode(secondInput), token)
	completedReplay := weighingData[weighing.CommandResult](t, router, "POST", secondPath, encode(secondInput), token)
	if completed.TicketID == 0 || completedReplay.TicketID != completed.TicketID || completed.Status != "completed" {
		t.Fatalf("second result: %+v %+v", completed, completedReplay)
	}
	ticket := weighingData[weighing.TicketView](t, router, "GET", "/api/weighing/tickets/"+itoa(completed.TicketID), "", token)
	if ticket.OriginalGross != 30000 || ticket.OriginalTare != 10000 || ticket.FinalGross != 31000 || ticket.FinalTare != 11000 || ticket.GrossDifference != -1000 || ticket.AdjustmentRemark != "磅精度调整" || ticket.Business.PartyName != party.Name || len(ticket.Basis) != 2 || !strings.HasPrefix(ticket.TicketNo, "GB") {
		t.Fatalf("test ticket weights: %+v", ticket)
	}
	finalTask := weighingData[weighing.TaskDetail](t, router, "GET", "/api/weighing/tasks/"+itoa(first.TaskID), "", token)
	if finalTask.Status != "completed" || len(finalTask.Records) != 2 || finalTask.CurrentSecondRecordID == nil {
		t.Fatalf("completed task: %+v", finalTask)
	}
	for _, event := range finalTask.Events {
		if strings.Contains(string(event.After), "sessionToken") {
			t.Fatal("session token leaked into business history")
		}
	}

	// Start another trip of the same plate before correcting the completed one.
	voidBody := map[string]any{"requestId": "void-old-ticket-0001", "reason": "测试更正"}
	voided := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tickets/"+itoa(ticket.ID)+"/void", encode(voidBody), token)
	futureBusiness := map[string]any{"plate": "川A67890", "direction": "inbound", "materialId": material.ID, "partyId": party.ID}
	futureSampleBody := map[string]any{"scaleId": 1, "clientId": "browser-b", "sessionToken": takenOver.SessionToken, "business": futureBusiness}
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/samples", encode(futureSampleBody), token), 200, 400, weighing.ErrInvalid.Error())
	partyRestore := fmt.Sprintf(`{"name":"首磅供收方已调整","nature":"enterprise","roles":["supplier","customer"],"version":%d}`, updatedParty.Version)
	weighingData[weighing.Party](t, router, "PUT", "/api/weighing/parties/"+itoa(party.ID), partyRestore, token)
	conditions["weight"] = "30"
	device.weight(conditions["weight"].(string))
	nextSample := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(map[string]any{"scaleId": 1, "clientId": "browser-b", "sessionToken": takenOver.SessionToken, "business": business}), token)
	nextInput := map[string]any{"scaleId": 1, "clientId": "browser-b", "sessionToken": takenOver.SessionToken, "business": business, "sampleId": nextSample.ID, "requestId": "new-trip-first-0001", "checks": checks}
	newTrip := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/first-confirmation", encode(nextInput), token)
	oldCaptureBody := map[string]any{"scaleId": 1, "clientId": "browser-b", "sessionToken": takenOver.SessionToken, "taskId": first.TaskID, "targetRecordId": secondSample.ID}
	// targetRecordId is a record id, never a sample id.
	oldCaptureBody["targetRecordId"] = completed.RecordID
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/samples", encode(oldCaptureBody), token), 200, 400, weighing.ErrConflict.Error())
	reopenBody := map[string]any{"requestId": "manual-reopen-0001", "taskVersion": voided.TaskVersion, "reason": "旧依据人工更正", "business": business, "basisValid": true, "finalWeights": map[string]any{"gross": "32", "tare": "11"}}
	reopened := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tickets/"+itoa(ticket.ID)+"/reopen", encode(reopenBody), token)
	if reopened.TicketID == ticket.ID {
		t.Fatal("reopen reused old ticket identity")
	}
	oldTicket := weighingData[weighing.TicketView](t, router, "GET", "/api/weighing/tickets/"+itoa(ticket.ID), "", token)
	if oldTicket.Status != "void" || oldTicket.FinalGross != 31000 || oldTicket.FinalTare != 11000 || oldTicket.OriginalTare != 10000 {
		t.Fatalf("old ticket snapshot rewritten: %+v", oldTicket)
	}
	newTicket := weighingData[weighing.TicketView](t, router, "GET", "/api/weighing/tickets/"+itoa(reopened.TicketID), "", token)
	if newTicket.PreviousTicketID == nil || *newTicket.PreviousTicketID != ticket.ID || newTicket.PreviousFinal == nil || newTicket.PreviousFinal.Gross != 31000 {
		t.Fatalf("reopen chain/prior final weights: %+v", newTicket)
	}
	terminated := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/"+itoa(newTrip.TaskID)+"/termination", encode(map[string]any{"requestId": "terminate-trip-0001", "version": newTrip.TaskVersion, "reason": "取消新趟运输"}), token)
	if terminated.Status != "terminated" {
		t.Fatalf("termination: %+v", terminated)
	}
	voidedAgain := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tickets/"+itoa(newTicket.ID)+"/void", `{"requestId":"void-new-ticket-0002","reason":"实际重称皮重"}`, token)
	conditions["weight"] = "12"
	device.weight(conditions["weight"].(string))
	replacement := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(oldCaptureBody), token)
	device.controlAction(`{"action":"down"}`)
	device.wait(func(state agentbridge.State) bool { return state.Weight == 0 })
	replaceBody := map[string]any{"requestId": "reweigh-second-0001", "version": voidedAgain.TaskVersion, "scaleId": 1, "clientId": "browser-b", "sessionToken": takenOver.SessionToken, "sampleId": replacement.ID, "targetRecordId": completed.RecordID, "checks": checks, "sameLoadConfirmed": true, "otherBasisValid": true, "reason": "空车重称"}
	replaced := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/"+itoa(first.TaskID)+"/reweighs", encode(replaceBody), token)
	repeated := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/"+itoa(first.TaskID)+"/reweighs", encode(replaceBody), token)
	if replaced.RecordID != repeated.RecordID {
		t.Fatal("reweigh replay duplicated record")
	}
	taskAfterReplacement := weighingData[weighing.TaskDetail](t, router, "GET", "/api/weighing/tasks/"+itoa(first.TaskID), "", token)
	if len(taskAfterReplacement.Records) != 3 || *taskAfterReplacement.CurrentFirstRecordID != first.RecordID || *taskAfterReplacement.CurrentSecondRecordID != replaced.RecordID {
		t.Fatalf("single replacement chain: %+v", taskAfterReplacement)
	}
	lastRecord := taskAfterReplacement.Records[len(taskAfterReplacement.Records)-1]
	if lastRecord.Weight.String() != "12" {
		t.Fatalf("reweigh did not use frozen sample: %+v", lastRecord)
	}
	if lastRecord.ReplacesRecordID == nil || *lastRecord.ReplacesRecordID != completed.RecordID || lastRecord.RetainedRecordID == nil || *lastRecord.RetainedRecordID != first.RecordID {
		t.Fatalf("replacement links: %+v", lastRecord)
	}
	badReopen := map[string]any{"requestId": "bad-reopen-0002", "taskVersion": replaced.TaskVersion, "reason": "非法最终值", "business": business, "basisValid": true, "finalWeights": map[string]any{"gross": "30", "tare": "40"}}
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/tickets/"+itoa(newTicket.ID)+"/reopen", encode(badReopen), token), 200, 400, weighing.ErrWeight.Error())
	var activeCount int64
	if err := database.GORM.Table("wb_ticket").Where("task_id=? AND status='valid'", first.TaskID).Count(&activeCount).Error; err != nil || activeCount != 0 {
		t.Fatalf("failed reopen restored a ticket: %d %v", activeCount, err)
	}
	badReopen["requestId"] = "valid-reopen-0003"
	badReopen["finalWeights"] = map[string]any{"gross": "31", "tare": "12"}
	finalReopen := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tickets/"+itoa(newTicket.ID)+"/reopen", encode(badReopen), token)
	if finalReopen.TicketID == 0 {
		t.Fatal("missing reopened ticket")
	}
	frozen := weighingData[weighing.TicketView](t, router, "GET", "/api/weighing/tickets/"+itoa(newTicket.ID), "", token)
	if frozen.Status != "void" || frozen.OriginalTare != 10000 || frozen.FinalGross != 32000 {
		t.Fatalf("replacement mutated previous evidence: %+v", frozen)
	}
	checkWeighingQueriesAndPrint(t, router, token, first.TaskID, finalReopen.TicketID)

}

func weighingData[T any](t *testing.T, router http.Handler, method, path, body, token string) T {
	t.Helper()
	response := serveJSON(router, method, path, body, token)
	if response.Code != http.StatusOK || envelopeCode(t, response) != http.StatusOK {
		t.Fatalf("%s %s: status=%d body=%s, want success", method, path, response.Code, response.Body.String())
	}
	var result struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Data
}

func checkWeighingMasters(t *testing.T, router http.Handler, token string) {
	t.Helper()
	category := weighingData[weighing.Category](t, router, http.MethodPost, "/api/weighing/material-categories", `{"name":"粮食","enabled":true}`, token)
	material := weighingData[weighing.Material](t, router, http.MethodPost, "/api/weighing/materials", `{"name":"高粱","spec":"一级","categoryId":`+itoa(category.ID)+`}`, token)
	if material.Code == "" || material.Version != 1 {
		t.Fatalf("created material: %+v", material)
	}
	weighingData[weighing.Material](t, router, http.MethodPost, "/api/weighing/materials", `{"name":"高粱","spec":"二级"}`, token)
	referenced := serveJSON(router, http.MethodDelete, "/api/weighing/material-categories/"+itoa(category.ID), `{"version":1}`, token)
	assertEnvelopeCode(t, referenced, 200, 400, weighing.ErrReferenced.Error())
	updated := weighingData[weighing.Material](t, router, http.MethodPut, "/api/weighing/materials/"+itoa(material.ID), `{"name":"高粱新名","spec":"一级","version":1}`, token)
	if updated.Version != 2 || updated.Code != material.Code {
		t.Fatalf("material update: %+v", updated)
	}
	stale := serveJSON(router, http.MethodPut, "/api/weighing/materials/"+itoa(material.ID), `{"name":"覆盖旧版本","version":1}`, token)
	assertEnvelopeCode(t, stale, 200, 400, weighing.ErrConflict.Error())
	party := weighingData[weighing.Party](t, router, http.MethodPost, "/api/weighing/parties", `{"name":"张三","nature":"individual","roles":["supplier","carrier"],"contacts":[{"name":"联络人","isDefault":true}]}`, token)
	if len(party.Roles) != 2 || len(party.Contacts) != 1 || party.Contacts[0].IsDefault != 1 {
		t.Fatalf("party roles/contacts: %+v", party)
	}
	weighingData[weighing.Party](t, router, http.MethodPost, "/api/weighing/parties", `{"name":"张三","nature":"individual","roles":["customer"]}`, token)
	invalid := serveJSON(router, http.MethodPost, "/api/weighing/parties", `{"name":"无默认联系人","nature":"enterprise","roles":["supplier"],"contacts":[{"name":"未指定默认"}]}`, token)
	assertEnvelopeCode(t, invalid, 200, 400, weighing.ErrInvalid.Error())
	foreignContact := serveJSON(router, http.MethodPost, "/api/weighing/parties", `{"name":"盗用联系人","nature":"enterprise","roles":["supplier"],"contacts":[{"id":`+itoa(party.Contacts[0].ID)+`,"name":"联络人","isDefault":true}]}`, token)
	assertEnvelopeCode(t, foreignContact, 200, 400, weighing.ErrInvalid.Error())
	page := weighingData[weighing.Page[weighing.Party]](t, router, http.MethodGet, "/api/weighing/parties?role=carrier&page=1&pageSize=10", "", token)
	if page.Total != 1 || len(page.Records) != 1 || page.Records[0].ID != party.ID {
		t.Fatalf("carrier filter: %+v", page)
	}
}

func checkWeighingQueriesAndPrint(t *testing.T, router http.Handler, token string, taskID, ticketID int64) {
	t.Helper()
	encode := func(v any) string {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	today := time.Now().UTC().Add(8 * time.Hour).Format("2006-01-02")
	yesterday := time.Now().UTC().Add(-16 * time.Hour).Format("2006-01-02")
	todayRows := weighingData[weighing.Page[weighing.OriginalView]](t, router, "GET", "/api/weighing/originals?from="+today+"&to="+today, "", token)
	if todayRows.Total != 1 || todayRows.Records[0].OriginalNet != 18000 {
		t.Fatalf("current second-confirmation date/original net: %+v", todayRows)
	}
	yesterdayRows := weighingData[weighing.Page[weighing.OriginalView]](t, router, "GET", "/api/weighing/originals?from="+yesterday+"&to="+yesterday, "", token)
	if yesterdayRows.Total != 0 {
		t.Fatalf("first date incorrectly selected original row: %+v", yesterdayRows)
	}
	raw := weighingData[weighing.Summary](t, router, "GET", "/api/weighing/originals/summary", "", token)
	final := weighingData[weighing.Summary](t, router, "GET", "/api/weighing/tickets/summary", "", token)
	if raw.TaskCount != 1 || raw.Net != 18000 || final.TaskCount != 1 || final.Net != 19000 {
		t.Fatalf("raw/final summaries or duplicate reopened versions: %+v %+v", raw, final)
	}
	versions := weighingData[weighing.Page[weighing.Ticket]](t, router, "GET", "/api/weighing/tickets?taskId="+itoa(taskID)+"&pageSize=1", "", token)
	if versions.Total != 3 || len(versions.Records) != 1 {
		t.Fatalf("ticket page: %+v", versions)
	}
	exported := serveJSON(router, "GET", "/api/weighing/tickets/export?taskId="+itoa(taskID)+"&pageSize=1", "", token)
	if exported.Code != 200 || !strings.Contains(exported.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("CSV contract: %d %s", exported.Code, exported.Body.String())
	}
	csvRows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(exported.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || len(csvRows) != 4 {
		t.Fatalf("export must include all versions beyond first page: rows=%d error=%v", len(csvRows), err)
	}
	events := weighingData[weighing.Page[weighing.BusinessEvent]](t, router, "GET", "/api/weighing/tasks/"+itoa(taskID)+"/events?page=1&pageSize=1", "", token)
	nextEvents := weighingData[weighing.Page[weighing.BusinessEvent]](t, router, "GET", "/api/weighing/tasks/"+itoa(taskID)+"/events?page=2&pageSize=1", "", token)
	if events.Total < 2 || len(events.Records) != 1 || len(nextEvents.Records) != 1 || events.Records[0].ID == nextEvents.Records[0].ID {
		t.Fatalf("event pagination: %+v %+v", events, nextEvents)
	}
	path := "/api/weighing/tickets/" + itoa(ticketID)
	printRequest := map[string]any{"requestId": "print-initial-0001", "intent": "initial"}
	firstPrint := weighingData[weighing.CommandResult](t, router, "POST", path+"/print-attempts", encode(printRequest), token)
	duplicate := weighingData[weighing.CommandResult](t, router, "POST", path+"/print-attempts", encode(printRequest), token)
	if firstPrint.PrintAttemptID == 0 || duplicate.PrintAttemptID != firstPrint.PrintAttemptID {
		t.Fatalf("print deduplication: %+v %+v", firstPrint, duplicate)
	}
	pending := weighingData[weighing.TicketView](t, router, "GET", path, "", token)
	if pending.PrintStatus != "unknown" || pending.ReprintCount != 0 {
		t.Fatalf("browser initiation cannot imply success: %+v", pending)
	}
	reprint := weighingData[weighing.CommandResult](t, router, "POST", path+"/print-attempts", `{"requestId":"print-reprint-0002","intent":"reprint","reason":"仍无法排除出纸，人工确认补打"}`, token)
	attempts := weighingData[[]weighing.PrintAttempt](t, router, "GET", path+"/print-attempts", "", token)
	if len(attempts) != 2 || attempts[0].Status != "unknown" || attempts[0].ClosedAt == nil || attempts[0].ResolutionSource != "operator" {
		t.Fatalf("unknown result was forged as printed: %+v", attempts)
	}
	weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/print-attempts/"+itoa(reprint.PrintAttemptID)+"/result", `{"requestId":"print-failed-0002","status":"failed","reason":"确认未出纸"}`, token)
	retry := weighingData[weighing.CommandResult](t, router, "POST", path+"/print-attempts", `{"requestId":"print-retry-0003","intent":"retry"}`, token)
	weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/print-attempts/"+itoa(retry.PrintAttemptID)+"/result", `{"requestId":"print-printed-0003","status":"printed","reason":"操作员确认已出纸"}`, token)
	printed := weighingData[weighing.TicketView](t, router, "GET", path, "", token)
	if printed.PrintStatus != "printed" || printed.ReprintCount != 1 || printed.FinalGross != 31000 || printed.FinalTare != 12000 {
		t.Fatalf("printing changed weights or retry counted as reprint: %+v", printed)
	}
	task := weighingData[weighing.TaskDetail](t, router, "GET", "/api/weighing/tasks/"+itoa(taskID), "", token)
	if task.Status != "completed" {
		t.Fatalf("printing changed task: %+v", task)
	}
}

// Device input travels through the actual public Agent protocol. Business checks
// below keep using the existing HTTP seam and databases for both dialects.
type weighingAgent struct {
	t       *testing.T
	control string
	hub     *agentbridge.Hub
}

func startWeighingAgent(t *testing.T, router http.Handler, hub *agentbridge.Hub) *weighingAgent {
	return startWeighingAgentWithConfig(t, router, hub, agentbridge.Config{ID: "integration-agent", ScaleID: 1, Key: "integration-agent-key"})
}

func startWeighingAgentWithConfig(t *testing.T, router http.Handler, hub *agentbridge.Hub, device agentbridge.Config) *weighingAgent {
	t.Helper()
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	mock := agentmock.New(agentmock.Config{Server: server.URL, ID: device.ID, ScaleID: device.ScaleID, Key: device.Key})
	control := httptest.NewServer(mock.Handler())
	t.Cleanup(control.Close)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); mock.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(12 * time.Second):
			t.Error("Agent did not stop")
		}
	})
	a := &weighingAgent{t, control.URL, hub}
	a.weight("30")
	return a
}
func (a *weighingAgent) controlAction(body string) {
	a.t.Helper()
	response, err := http.Post(a.control+"/api/control", "application/json", bytes.NewBufferString(body))
	if err != nil {
		a.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != 204 {
		a.t.Fatalf("mock control: %d %s", response.StatusCode, data)
	}
}
func (a *weighingAgent) wait(ready func(agentbridge.State) bool) {
	a.t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if ready(a.hub.State(1)) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	a.t.Fatalf("Agent state timeout: %+v", a.hub.State(1))
}
func (a *weighingAgent) weight(weight string) {
	a.t.Helper()
	a.controlAction(fmt.Sprintf(`{"action":"up","weight":%q}`, weight))
	a.controlAction(`{"action":"stable"}`)
	a.wait(func(v agentbridge.State) bool {
		return v.Online && v.Stable && v.Weight.String() == weight
	})
}
func (a *weighingAgent) plate(candidate string) {
	a.t.Helper()
	a.controlAction(fmt.Sprintf(`{"action":"plate","candidate":%q}`, candidate))
	a.wait(func(v agentbridge.State) bool { return v.Plate.Candidate == candidate })
}
