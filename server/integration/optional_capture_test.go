//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPostgresOptionalCaptureBusinessLoop(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	repository := weighing.NewRepository(db.GORM)
	imageDir := t.TempDir()
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, id int64) (agentbridge.Config, error) {
		return repository.AgentConfig(ctx, id, imageDir)
	})
	deps.Weighing = weighing.NewService(repository, hub)
	cameraService, frames, _ := newCaptureService(t, db, t.TempDir())
	deps.Camera = cameraService
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	testLocalSetup(t, db, router, token, 1)
	addCaptureCameras(t, db, "rtsp://camera/optional-1", "rtsp://camera/optional-2")
	deviceConfig, err := repository.AgentConfig(context.Background(), 1, imageDir)
	if err != nil {
		t.Fatal(err)
	}
	device := startWeighingAgentWithConfig(t, router, hub, deviceConfig)
	encode := func(v any) string {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	var row sysconfig.Config
	if err := db.GORM.Where("config_key=?", sysconfig.CaptureRequiredKey).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.ConfigValue != "true" || row.IsBuiltin != 1 {
		t.Fatalf("default=%+v", row)
	}
	configPath := "/api/system/config/" + itoa(row.ID)
	assertEnvelopeCode(t, serveJSON(router, "POST", "/api/system/user", `{"username":"optional-operator","nickname":"无图操作员","status":1}`, token), 200, 200, "success")
	if err := db.GORM.Exec("INSERT INTO sys_user_role (user_id,role_id) SELECT u.id,r.id FROM sys_user u,sys_role r WHERE u.username=? AND r.role_code='WEIGHING_OPERATOR'", "optional-operator").Error; err != nil {
		t.Fatal(err)
	}
	operator := loginUser(t, router, "optional-operator", "admin123")
	assertEnvelopeCode(t, serveJSON(router, "PUT", configPath, `{"configValue":"false"}`, operator), 403, 403, "无权执行此操作")

	toggle := func(required bool) {
		t.Helper()
		assertEnvelopeCode(t, serveJSON(router, "PUT", configPath, fmt.Sprintf(`{"configValue":"%t"}`, required), token), 200, 200, "success")
	}
	captureState := func() sysconfig.CapturePolicy {
		return weighingData[sysconfig.CapturePolicy](t, router, "GET", "/api/weighing/scales/1/device", "", token)
	}
	if !captureState().Required {
		t.Fatal("default capture policy is optional")
	}
	assertEnvelopeCode(t, serveJSON(router, "PATCH", configPath+"/status", `{"status":0}`, token), 200, 400, sysconfig.ErrBuiltin.Error())
	material := weighingData[weighing.Material](t, router, "POST", "/api/weighing/materials", `{"name":"无图物料"}`, token)
	party := weighingData[weighing.Party](t, router, "POST", "/api/weighing/parties", `{"name":"无图供收方","nature":"enterprise","roles":["supplier","customer"]}`, token)
	checks := map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}
	for index, direction := range []string{"inbound", "outbound"} {
		frames.failAll(true)
		firstWeight, secondWeight := "30", "10"
		if direction == "outbound" {
			firstWeight, secondWeight = "10", "30"
		}
		device.weight(firstWeight)
		device.wait(func(s agentbridge.State) bool { return s.DeviceOK && s.Stable && s.Weight.String() == firstWeight })
		session := weighingData[weighing.SessionResult](t, router, "POST", "/api/weighing/scales/1/session/acquire", `{"clientId":"optional-browser"}`, token)
		business := map[string]any{"plate": fmt.Sprintf("川A1234%d", index), "direction": direction, "materialId": material.ID, "partyId": party.ID}
		sampleBody := map[string]any{"scaleId": 1, "clientId": "optional-browser", "sessionToken": session.SessionToken, "business": business}
		toggle(true)
		assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/samples", encode(sampleBody), token), 200, 400, weighing.ErrConditions.Error())
		toggle(false)
		if captureState().Required {
			t.Fatal("toggle did not apply immediately")
		}
		sample := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(sampleBody), token)
		assertCaptureEvidence(t, sample.Evidence, false, "capture_failed", 0)
		firstBody := map[string]any{"scaleId": 1, "clientId": "optional-browser", "sessionToken": session.SessionToken, "business": business, "sampleId": sample.ID, "requestId": fmt.Sprintf("optional-stale-%d", index), "checks": checks}
		// Policy edits apply to later acquisitions, not this frozen no-image sample.
		toggle(true)
		first := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/first-confirmation", encode(firstBody), token)
		if first.TaskID == 0 {
			t.Fatal("no first record")
		}
		toggle(false)
		// A mixed-policy trip reuses its confirmed no-image first basis.
		if direction == "outbound" {
			frames.failAll(false)
			toggle(true)
		}
		device.weight(secondWeight)
		sampleBody["taskId"] = first.TaskID
		secondSample := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(sampleBody), token)
		status, count := "capture_failed", 0
		if direction == "outbound" {
			status, count = "complete", 2
		}
		assertCaptureEvidence(t, secondSample.Evidence, direction == "outbound", status, count)
		secondPath := "/api/weighing/tasks/" + itoa(first.TaskID) + "/second-confirmation"
		second := weighingData[weighing.CommandResult](t, router, "POST", secondPath, encode(map[string]any{"scaleId": 1, "clientId": "optional-browser", "sessionToken": session.SessionToken, "sampleId": secondSample.ID, "requestId": fmt.Sprintf("optional-second-%d", index), "version": first.TaskVersion, "checks": checks}), token)
		ticketPath := "/api/weighing/tickets/" + itoa(second.TicketID)
		ticket := weighingData[weighing.TicketView](t, router, "GET", ticketPath, "", token)
		if ticket.Mode != "normal" || ticket.FinalNet != 20000 || len(ticket.Basis) != 2 {
			t.Fatalf("formal ticket=%+v", ticket)
		}
		assertCaptureEvidence(t, ticket.Basis[0].Evidence, false, "capture_failed", 0)
		toggle(true)
		replay := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/first-confirmation", encode(firstBody), token)
		if replay.RecordID != first.RecordID {
			t.Fatal("policy change broke committed replay")
		}
		print := weighingData[weighing.CommandResult](t, router, "POST", ticketPath+"/print-attempts", fmt.Sprintf(`{"requestId":"optional-print-%d","intent":"initial"}`, index), token)
		weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/print-attempts/"+itoa(print.PrintAttemptID)+"/result", fmt.Sprintf(`{"requestId":"optional-printed-%d","status":"printed","reason":"软件测试核实"}`, index), token)
		reprint := weighingData[weighing.CommandResult](t, router, "POST", ticketPath+"/print-attempts", fmt.Sprintf(`{"requestId":"optional-reprint-%d","intent":"reprint","reason":"软件测试补打"}`, index), token)
		if reprint.PrintAttemptID == print.PrintAttemptID {
			t.Fatal("missing reprint")
		}
		weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/print-attempts/"+itoa(reprint.PrintAttemptID)+"/result", fmt.Sprintf(`{"requestId":"optional-reprinted-%d","status":"printed","reason":"软件测试核实"}`, index), token)
		void := weighingData[weighing.CommandResult](t, router, "POST", ticketPath+"/void", fmt.Sprintf(`{"requestId":"optional-void-%d","reason":"软件测试重称"}`, index), token)
		toggle(false)
		frames.failAll(true)
		sampleBody["targetRecordId"] = second.RecordID
		replacement := weighingData[weighing.Sample](t, router, "POST", "/api/weighing/samples", encode(sampleBody), token)
		assertCaptureEvidence(t, replacement.Evidence, false, "capture_failed", 0)
		reweigh := weighingData[weighing.CommandResult](t, router, "POST", "/api/weighing/tasks/"+itoa(first.TaskID)+"/reweighs", encode(map[string]any{"requestId": fmt.Sprintf("optional-reweigh-%d", index), "version": void.TaskVersion, "scaleId": 1, "clientId": "optional-browser", "sessionToken": session.SessionToken, "sampleId": replacement.ID, "targetRecordId": second.RecordID, "checks": checks, "sameLoadConfirmed": true, "otherBasisValid": true, "reason": "软件测试单磅重称"}), token)
		if reweigh.RecordID == second.RecordID {
			t.Fatal("missing replacement")
		}
		// Camera exceptions must never relax instrument conditions.
		for _, action := range []string{"unstable", "fault"} {
			device.controlAction(fmt.Sprintf(`{"action":%q}`, action))
			device.wait(func(s agentbridge.State) bool { return !s.Stable || !s.DeviceOK })
			assertEnvelopeCode(t, serveJSON(router, "POST", "/api/weighing/samples", encode(sampleBody), token), 200, 400, weighing.ErrConditions.Error())
			device.controlAction(`{"action":"recover"}`)
			device.weight(secondWeight)
			device.wait(func(s agentbridge.State) bool { return s.DeviceOK && s.Stable && s.Weight.String() == secondWeight })
		}
		weighingData[weighing.SessionResult](t, router, "POST", "/api/weighing/scales/1/session/release", encode(map[string]any{"clientId": "optional-browser", "sessionToken": session.SessionToken}), token)
	}
	// Corrupted/disabled/missing config never silently enables no-image weighing.
	for _, updates := range []map[string]any{{"config_value": "invalid"}, {"config_value": "false", "status": 0}, {"config_value": "false", "status": 1, "value_type": "TEXT"}, {"deleted": 1}} {
		if err := db.GORM.Model(&sysconfig.Config{}).Where("id=?", row.ID).Updates(updates).Error; err != nil {
			t.Fatal(err)
		}
		policy, err := deps.Weighing.CapturePolicy(context.Background())
		if err != nil || !policy.Required {
			t.Fatalf("unsafe default=%+v %v", policy, err)
		}
	}
}

func assertCaptureEvidence(t *testing.T, data weighing.JSON, required bool, status string, images int) {
	t.Helper()
	var saved struct {
		Required bool   `json:"captureRequired"`
		Revision string `json:"capturePolicyRevision"`
		Status   string `json:"imageStatus"`
		Images   []any  `json:"images"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Required != required || saved.Revision == "" || saved.Status != status || len(saved.Images) != images {
		t.Fatalf("evidence=%s", data)
	}
}
