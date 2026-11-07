//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

const manualChecks = `{"plate":true,"business":true,"weight":true,"evidence":true,"fullyOnScale":true}`

type manualReturnFixture struct {
	t         *testing.T
	router    http.Handler
	database  *platformdatabase.Database
	device    *weighingAgent
	admin     string
	session   weighing.SessionResult
	material  weighing.Material
	party     weighing.Party
	requestNo int
}

func TestPostgresManualReturnAndTermination(t *testing.T) {
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	repo := weighing.NewRepository(database.GORM)
	imageDir := t.TempDir()
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, scaleCode string) (agentbridge.Config, error) {
		return repo.AgentConfig(ctx, scaleCode, imageDir)
	})
	deps := testDependencies(t, database, t.TempDir())
	deps.Weighing = weighing.NewService(repo, hub)
	deps.Camera, _, _ = newCaptureService(t, database, t.TempDir())
	router, err := app.Build(testAPIConfig(), database, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	testLocalSetup(t, database, router, admin, 1, "SITE01", "原厂区", "SCALE01", "一号地磅", "integration-agent")
	addCaptureCameras(t, database, "rtsp://camera/manual-return-1")
	deviceConfig, err := repo.AgentConfig(context.Background(), "SCALE01", imageDir)
	if err != nil {
		t.Fatal(err)
	}
	device := startWeighingAgentWithConfig(t, router, hub, deviceConfig)
	material := weighingData[weighing.Material](t, router, "POST", "/api/weighing/materials", `{"name":"补录高粱","spec":"一级"}`, admin)
	party := weighingData[weighing.Party](t, router, "POST", "/api/weighing/parties", `{"name":"补录供收方","nature":"enterprise","roles":["supplier","customer"]}`, admin)
	session := weighingData[weighing.SessionResult](t, router, "POST", "/api/weighing/scales/SCALE01/session/acquire", `{"clientId":"manual-browser"}`, admin)
	f := &manualReturnFixture{t: t, router: router, database: database, device: device, admin: admin, session: session, material: material, party: party}

	t.Run("manual return completes task, issues ticket and releases plate", func(t *testing.T) {
		first := f.createFirstTask("川A12345", "inbound", 30000)
		f.backdateFirst(first)
		before := f.taskDetail(first.TaskID)
		// No operation session, device sample, stability state or image is needed.
		assertEnvelopeCode(t, serveJSON(f.router, "POST", f.sessionPath("release"), encodeJSON(t, map[string]any{"clientId": "manual-browser", "sessionToken": f.session.SessionToken}), f.admin), 200, 200, "success")

		returnedAt := time.Now().UTC().Add(-30 * time.Minute)
		var samplesBefore, samplesAfter int64
		if err := f.database.GORM.Table("wb_sample").Count(&samplesBefore).Error; err != nil {
			t.Fatal(err)
		}
		body := map[string]any{"requestId": "manual-return-0001", "version": first.TaskVersion, "weight": "10", "returnedAt": returnedAt.Format(time.RFC3339), "reason": "车辆已回磅离开，现场已拍下地磅读数，经核对后补录"}
		result := weighingData[weighing.CommandResult](t, f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, body), f.admin)
		if result.Status != "completed" || result.TaskID != first.TaskID || result.RecordID == 0 || result.TicketID == 0 {
			t.Fatalf("manual return result: %+v", result)
		}
		if err := f.database.GORM.Table("wb_sample").Count(&samplesAfter).Error; err != nil {
			t.Fatal(err)
		}
		if samplesAfter != samplesBefore {
			t.Fatalf("manual return fabricated a device sample: %d -> %d", samplesBefore, samplesAfter)
		}
		detail := f.taskDetail(first.TaskID)
		if detail.Status != "completed" || detail.CompletedAt == nil || detail.CurrentSecondRecordID == nil || len(detail.Records) != 2 {
			t.Fatalf("manual return task: %+v", detail)
		}
		manual := detail.Records[1]
		if manual.Source != "manual" || manual.SampleID != nil || manual.Stage != "second" || manual.Kind != "tare" || manual.Weight != 10000 || manual.ActorName != "管理员" || manual.Reason != body["reason"] {
			t.Fatalf("manual record source/details: %+v", manual)
		}
		if delta := manual.CapturedAt.Sub(returnedAt); delta < -time.Second || delta > time.Second {
			t.Fatalf("actual return time not preserved: %v vs %v", manual.CapturedAt, returnedAt)
		}
		if !manual.ConfirmedAt.After(manual.CapturedAt) {
			t.Fatalf("system entry time must be separate from actual return time: %+v", manual)
		}
		if before.Records[0].Source != "collected" || before.Records[0].Weight != 30000 {
			t.Fatalf("first basis was not preserved: %+v", before.Records)
		}

		ticket := weighingData[weighing.TicketView](t, f.router, "GET", "/api/weighing/tickets/"+itoa(result.TicketID), "", f.admin)
		if ticket.OriginalGross != 30000 || ticket.OriginalTare != 10000 || ticket.FinalNet != 20000 || ticket.GrossSource != "collected" || ticket.TareSource != "manual" || len(ticket.Basis) != 2 || ticket.Basis[1].Source != "manual" || ticket.Basis[1].SampleID != nil {
			t.Fatalf("manual ticket snapshot: %+v", ticket)
		}
		if !containsString(ticket.PrintSheet.Marks, "回磅人工补录") {
			t.Fatalf("print marks do not disclose manual return: %+v", ticket.PrintSheet.Marks)
		}

		var payload struct {
			Ticket struct {
				GrossSource string `json:"grossSource"`
				TareSource  string `json:"tareSource"`
			} `json:"ticket"`
		}
		var raw string
		if err := f.database.GORM.Raw("SELECT payload::text FROM wb_ticket_sync_change_v5 WHERE ticket_id=? UNION ALL SELECT payload::text FROM wb_ticket_sync_change_v4 WHERE ticket_id=? LIMIT 1", result.TicketID, result.TicketID).Scan(&raw).Error; err != nil || raw == "" {
			t.Fatalf("load sync payload: %s %v", raw, err)
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Ticket.GrossSource != "collected" || payload.Ticket.TareSource != "manual" {
			t.Fatalf("cloud payload misreports manual return: %s", raw)
		}
		var eventCount int64
		if err := f.database.GORM.Table("wb_business_event").Where("task_id=? AND action='task.manual-return' AND reason=?", first.TaskID, body["reason"]).Count(&eventCount).Error; err != nil || eventCount != 1 {
			t.Fatalf("manual return audit event: count=%d error=%v", eventCount, err)
		}

		replay := weighingData[weighing.CommandResult](t, f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, body), f.admin)
		if replay.TicketID != result.TicketID || replay.RecordID != result.RecordID {
			t.Fatalf("idempotent replay created new facts: %+v %+v", result, replay)
		}
		mismatch := map[string]any{"requestId": "manual-return-0001", "version": first.TaskVersion, "weight": "11", "returnedAt": returnedAt.Format(time.RFC3339), "reason": "相同请求号不同内容"}
		assertEnvelopeCode(t, serveJSON(f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, mismatch), f.admin), 200, 400, weighing.ErrRequestKey.Error())

		var active *weighing.TaskDetail
		active = weighingData[*weighing.TaskDetail](t, f.router, "GET", "/api/weighing/tasks/active?plate=川A12345", "", f.admin)
		if active != nil {
			t.Fatalf("completed manual return still occupies plate: %+v", active)
		}
		// Continue with a fresh operation session for later normal captures.
		f.session = f.acquireSession("manual-browser")
		// The released plate can start a new task, and termination keeps the
		// existing first record without issuing a ticket.
		next := f.createFirstTask("川A12345", "inbound", 28000)
		terminated := weighingData[weighing.CommandResult](t, f.router, "POST", "/api/weighing/tasks/"+itoa(next.TaskID)+"/termination", encodeJSON(t, map[string]any{"requestId": "terminate-new-trip-0001", "version": next.TaskVersion, "reason": "不再回磅"}), f.admin)
		if terminated.Status != "terminated" {
			t.Fatalf("termination result: %+v", terminated)
		}
		terminatedReplay := weighingData[weighing.CommandResult](t, f.router, "POST", "/api/weighing/tasks/"+itoa(next.TaskID)+"/termination", encodeJSON(t, map[string]any{"requestId": "terminate-new-trip-0001", "version": next.TaskVersion, "reason": "不再回磅"}), f.admin)
		if terminatedReplay.Status != "terminated" {
			t.Fatalf("termination replay: %+v", terminatedReplay)
		}
		terminatedDetail := f.taskDetail(next.TaskID)
		if terminatedDetail.Status != "terminated" || len(terminatedDetail.Records) != 1 || terminatedDetail.TerminateReason != "不再回磅" {
			t.Fatalf("termination retained facts: %+v", terminatedDetail)
		}
		var terminatedTickets int64
		if err := f.database.GORM.Table("wb_ticket").Where("task_id=?", next.TaskID).Count(&terminatedTickets).Error; err != nil || terminatedTickets != 0 {
			t.Fatalf("terminated task issued ticket: count=%d error=%v", terminatedTickets, err)
		}
		if active = weighingData[*weighing.TaskDetail](t, f.router, "GET", "/api/weighing/tasks/active?plate=川A12345", "", f.admin); active != nil {
			t.Fatalf("terminated task still occupies plate: %+v", active)
		}
		f.createFirstTask("川A12345", "inbound", 26000)
	})

	t.Run("weight, time, state and version errors do not write facts", func(t *testing.T) {
		first := f.createFirstTask("川B12345", "inbound", 30000)
		f.backdateFirst(first)
		returnedAt := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
		base := func(weight string, at string) map[string]any {
			return map[string]any{"requestId": f.request("invalid"), "version": first.TaskVersion, "weight": weight, "returnedAt": at, "reason": "人工核对后补录"}
		}
		assertEnvelopeCode(t, serveJSON(f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, base("30", returnedAt)), f.admin), 200, 400, weighing.ErrWeight.Error())
		assertEnvelopeCode(t, serveJSON(f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, base("0", returnedAt)), f.admin), 200, 400, weighing.ErrWeight.Error())
		assertEnvelopeCode(t, serveJSON(f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, base("10.0001", returnedAt)), f.admin), 200, 400, weighing.ErrInvalid.Error())
		beforeFirst := f.taskDetail(first.TaskID).Records[0].CapturedAt.Add(-time.Hour).Format(time.RFC3339)
		assertEnvelopeCode(t, serveJSON(f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, base("10", beforeFirst)), f.admin), 200, 400, weighing.ErrManualReturnTime.Error())
		future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
		assertEnvelopeCode(t, serveJSON(f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, base("10", future)), f.admin), 200, 400, weighing.ErrManualReturnTime.Error())
		stale := base("10", returnedAt)
		stale["version"] = first.TaskVersion + 1
		assertEnvelopeCode(t, serveJSON(f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, stale), f.admin), 200, 400, weighing.ErrConflict.Error())
		detail := f.taskDetail(first.TaskID)
		if detail.Status != "awaiting_second" || len(detail.Records) != 1 || detail.CurrentSecondRecordID != nil {
			t.Fatalf("rejected manual returns wrote facts: %+v", detail)
		}
		var tickets int64
		if err := f.database.GORM.Table("wb_ticket").Where("task_id=?", first.TaskID).Count(&tickets).Error; err != nil || tickets != 0 {
			t.Fatalf("rejected manual returns issued tickets: %d %v", tickets, err)
		}
		// Terminating is the only remaining outcome; manual return cannot revive it.
		weighingData[weighing.CommandResult](t, f.router, "POST", "/api/weighing/tasks/"+itoa(first.TaskID)+"/termination", encodeJSON(t, map[string]any{"requestId": f.request("terminate-invalid"), "version": first.TaskVersion, "reason": "错误补录后终止"}), f.admin)
		assertEnvelopeCode(t, serveJSON(f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, base("10", returnedAt)), f.admin), 200, 400, weighing.ErrConflict.Error())
	})

	t.Run("permission and default role grant", func(t *testing.T) {
		created := serveJSON(f.router, "POST", "/api/system/user", `{"username":"manual-operator","nickname":"补录操作员","status":1}`, f.admin)
		assertEnvelopeCode(t, created, 200, 200, "success")
		var userID int64
		if err := f.database.GORM.Table("sys_user").Select("id").Where("username=?", "manual-operator").Scan(&userID).Error; err != nil || userID == 0 {
			t.Fatalf("operator user: %d %v", userID, err)
		}
		if err := f.database.GORM.Exec("INSERT INTO sys_user_role (user_id, role_id) SELECT ?, id FROM sys_role WHERE role_code='WEIGHING_OPERATOR'", userID).Error; err != nil {
			t.Fatal(err)
		}
		weighingData[any](t, f.router, "PUT", "/api/weighing/users/"+itoa(userID)+"/sites", `{"grants":[{"siteCode":"SITE01","canOperate":1}]}`, f.admin)
		operator := loginUser(t, f.router, "manual-operator", "admin123")
		codes := weighingData[map[string]any](t, f.router, "GET", "/api/weighing/capabilities", "", operator)
		permissionCodes, _ := codes["permissionCodes"].([]any)
		foundGrant := false
		for _, code := range permissionCodes {
			if code == "weighing:manual-return" {
				foundGrant = true
			}
		}
		if !foundGrant {
			t.Fatalf("default operator role missing manual-return grant: %+v", permissionCodes)
		}
		first := f.createFirstTask("川C12345", "inbound", 30000)
		f.backdateFirst(first)
		allowe := weighingData[weighing.CommandResult](t, f.router, "POST", f.manualPath(first.TaskID), encodeJSON(t, map[string]any{"requestId": f.request("operator"), "version": first.TaskVersion, "weight": "10", "returnedAt": time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339), "reason": "操作员补录"}), operator)
		if allowe.Status != "completed" || allowe.TicketID == 0 {
			t.Fatalf("operator manual return: %+v", allowe)
		}
		second := f.createFirstTask("川D12345", "inbound", 30000)
		f.backdateFirst(second)
		if err := f.database.GORM.Exec("DELETE FROM sys_role_menu WHERE role_id IN (SELECT id FROM sys_role WHERE role_code='WEIGHING_OPERATOR') AND menu_id IN (SELECT id FROM sys_menu WHERE permission_code='weighing:manual-return')").Error; err != nil {
			t.Fatal(err)
		}
		denied := serveJSON(f.router, "POST", f.manualPath(second.TaskID), encodeJSON(t, map[string]any{"requestId": f.request("denied"), "version": second.TaskVersion, "weight": "10", "returnedAt": time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339), "reason": "无权限补录"}), operator)
		assertEnvelopeCode(t, denied, 403, 403, "无权执行此操作")
		weighingData[weighing.CommandResult](t, f.router, "POST", "/api/weighing/tasks/"+itoa(second.TaskID)+"/termination", encodeJSON(t, map[string]any{"requestId": f.request("cleanup"), "version": second.TaskVersion, "reason": "清理测试任务"}), f.admin)
	})

	t.Run("manual return races safely with normal second confirmation and termination", func(t *testing.T) {
		normal := f.createFirstTask("川E12345", "inbound", 30000)
		f.backdateFirst(normal)
		second := f.captureSecond(normal.TaskID, normal.TaskVersion, 10000)
		returnedAt := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
		confirmBody := map[string]any{"scaleCode": "SCALE01", "clientId": "manual-browser", "sessionToken": f.session.SessionToken, "sampleId": second.ID, "requestId": f.request("race-confirm"), "version": normal.TaskVersion, "checks": json.RawMessage(manualChecks)}
		manualBody := map[string]any{"requestId": f.request("race-manual"), "version": normal.TaskVersion, "weight": "10", "returnedAt": returnedAt, "reason": "并发补录"}
		responses := make(chan *httptest.ResponseRecorder, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			responses <- serveJSON(f.router, "POST", "/api/weighing/tasks/"+itoa(normal.TaskID)+"/second-confirmation", encodeJSON(t, confirmBody), f.admin)
		}()
		go func() {
			defer wg.Done()
			responses <- serveJSON(f.router, "POST", f.manualPath(normal.TaskID), encodeJSON(t, manualBody), f.admin)
		}()
		wg.Wait()
		close(responses)
		successes, rejections := 0, 0
		for response := range responses {
			switch code := envelopeCode(t, response); code {
			case 200:
				successes++
			case 400:
				rejections++
			default:
				t.Fatalf("unexpected race response %d: %s", code, response.Body.String())
			}
		}
		if successes != 1 || rejections != 1 {
			t.Fatalf("race outcome successes=%d rejections=%d", successes, rejections)
		}
		normalDetail := f.taskDetail(normal.TaskID)
		var normalTickets []weighing.Ticket
		if err := f.database.GORM.Table("wb_ticket").Where("task_id=?", normal.TaskID).Find(&normalTickets).Error; err != nil {
			t.Fatal(err)
		}
		if normalDetail.Status != "completed" || len(normalTickets) != 1 {
			t.Fatalf("normal/manual race must complete once with one ticket: %+v tickets=%d", normalDetail, len(normalTickets))
		}
		if normalTickets[0].TareSource != "manual" && (normalTickets[0].GrossSource != "collected" || normalTickets[0].TareSource != "collected") {
			t.Fatalf("race ticket source is neither a clean manual nor a clean collected result: %+v", normalTickets[0])
		}

		terminateTask := f.createFirstTask("川F12345", "inbound", 30000)
		f.backdateFirst(terminateTask)
		responses = make(chan *httptest.ResponseRecorder, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			responses <- serveJSON(f.router, "POST", "/api/weighing/tasks/"+itoa(terminateTask.TaskID)+"/termination", encodeJSON(t, map[string]any{"requestId": f.request("race-terminate-real"), "version": terminateTask.TaskVersion, "reason": "并发终止"}), f.admin)
		}()
		go func() {
			defer wg.Done()
			responses <- serveJSON(f.router, "POST", f.manualPath(terminateTask.TaskID), encodeJSON(t, map[string]any{"requestId": f.request("race-manual-real"), "version": terminateTask.TaskVersion, "weight": "10", "returnedAt": returnedAt, "reason": "并发补录"}), f.admin)
		}()
		wg.Wait()
		close(responses)
		successes, rejections = 0, 0
		for response := range responses {
			switch code := envelopeCode(t, response); code {
			case 200:
				successes++
			case 400:
				rejections++
			default:
				t.Fatalf("unexpected termination race response %d: %s", code, response.Body.String())
			}
		}
		if successes != 1 || rejections != 1 {
			t.Fatalf("termination race outcome successes=%d rejections=%d", successes, rejections)
		}
		raceDetail := f.taskDetail(terminateTask.TaskID)
		var raceTickets int64
		if err := f.database.GORM.Table("wb_ticket").Where("task_id=?", terminateTask.TaskID).Count(&raceTickets).Error; err != nil {
			t.Fatal(err)
		}
		if raceDetail.Status == "completed" {
			if raceTickets != 1 || raceDetail.CurrentSecondRecordID == nil {
				t.Fatalf("completed race must have exactly one ticket: %+v tickets=%d", raceDetail, raceTickets)
			}
		} else if raceDetail.Status == "terminated" {
			if raceTickets != 0 || raceDetail.CurrentSecondRecordID != nil {
				t.Fatalf("terminated race must not issue a ticket: %+v tickets=%d", raceDetail, raceTickets)
			}
		} else {
			t.Fatalf("unexpected race task status: %+v", raceDetail)
		}
	})

	t.Run("normal device collection is unaffected", func(t *testing.T) {
		first := f.createFirstTask("川G12345", "outbound", 12000)
		second := f.captureSecond(first.TaskID, first.TaskVersion, 32000)
		ticketID := f.confirmSecond(first.TaskID, first.TaskVersion, second.ID, f.request("normal-second"))
		ticket := weighingData[weighing.TicketView](t, f.router, "GET", "/api/weighing/tickets/"+itoa(ticketID), "", f.admin)
		if ticket.OriginalGross != 32000 || ticket.OriginalTare != 12000 || ticket.GrossSource != "collected" || ticket.TareSource != "collected" || len(ticket.Basis) != 2 || ticket.Basis[0].Source != "collected" || ticket.Basis[1].Source != "collected" {
			t.Fatalf("normal device flow changed: %+v", ticket)
		}
		if containsString(ticket.PrintSheet.Marks, "回磅人工补录") {
			t.Fatalf("normal ticket wrongly marked manual: %+v", ticket.PrintSheet.Marks)
		}
	})
}

func (f *manualReturnFixture) request(prefix string) string {
	f.requestNo++
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), f.requestNo)
}

func (f *manualReturnFixture) acquireSession(clientID string) weighing.SessionResult {
	f.t.Helper()
	return weighingData[weighing.SessionResult](f.t, f.router, "POST", f.sessionPath("acquire"), encodeJSON(f.t, map[string]any{"clientId": clientID}), f.admin)
}

func (f *manualReturnFixture) sessionPath(action string) string {
	return "/api/weighing/scales/SCALE01/session/" + action
}

func (f *manualReturnFixture) manualPath(id int64) string {
	return "/api/weighing/tasks/" + itoa(id) + "/manual-returns"
}

func (f *manualReturnFixture) createFirstTask(plate, direction string, weight int) weighing.CommandResult {
	f.t.Helper()
	f.device.weight(fmt.Sprintf("%d", weight/1000))
	business := map[string]any{"plate": plate, "direction": direction, "materialId": f.material.ID, "partyId": f.party.ID}
	sample := weighingData[weighing.Sample](f.t, f.router, "POST", "/api/weighing/samples", encodeJSON(f.t, map[string]any{"scaleCode": "SCALE01", "clientId": "manual-browser", "sessionToken": f.session.SessionToken, "business": business}), f.admin)
	body := map[string]any{"scaleCode": "SCALE01", "clientId": "manual-browser", "sessionToken": f.session.SessionToken, "business": business, "sampleId": sample.ID, "requestId": f.request("first"), "checks": json.RawMessage(manualChecks)}
	return weighingData[weighing.CommandResult](f.t, f.router, "POST", "/api/weighing/tasks/first-confirmation", encodeJSON(f.t, body), f.admin)
}

func (f *manualReturnFixture) captureSecond(taskID, version int64, weight int) weighing.Sample {
	f.t.Helper()
	f.device.weight(fmt.Sprintf("%d", weight/1000))
	return weighingData[weighing.Sample](f.t, f.router, "POST", "/api/weighing/samples", encodeJSON(f.t, map[string]any{"scaleCode": "SCALE01", "clientId": "manual-browser", "sessionToken": f.session.SessionToken, "taskId": taskID}), f.admin)
}

func (f *manualReturnFixture) confirmSecond(taskID, version, sampleID int64, requestID string) int64 {
	f.t.Helper()
	body := map[string]any{"scaleCode": "SCALE01", "clientId": "manual-browser", "sessionToken": f.session.SessionToken, "sampleId": sampleID, "requestId": requestID, "version": version, "checks": json.RawMessage(manualChecks)}
	result := weighingData[weighing.CommandResult](f.t, f.router, "POST", "/api/weighing/tasks/"+itoa(taskID)+"/second-confirmation", encodeJSON(f.t, body), f.admin)
	if result.TicketID == 0 {
		f.t.Fatalf("second confirmation did not issue ticket: %+v", result)
	}
	return result.TicketID
}

func (f *manualReturnFixture) backdateFirst(result weighing.CommandResult) {
	f.t.Helper()
	at := time.Now().UTC().Add(-time.Hour)
	if err := f.database.GORM.Exec("UPDATE wb_weigh_record SET captured_at=?, confirmed_at=? WHERE id=?", at, at, result.RecordID).Error; err != nil {
		f.t.Fatal(err)
	}
}

func (f *manualReturnFixture) taskDetail(id int64) weighing.TaskDetail {
	f.t.Helper()
	return weighingData[weighing.TaskDetail](f.t, f.router, "GET", "/api/weighing/tasks/"+itoa(id), "", f.admin)
}

func encodeJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
