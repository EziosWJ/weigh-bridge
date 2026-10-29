//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/camera"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestPlateEvidenceRejectsForgedEventsAndPreservesCorrection(t *testing.T) {
	db := openTemporaryDatabase(t, startBusinessPostgres(t).dsn)
	t.Cleanup(func() { _ = db.Close() })
	api := newMeasurementAPI(t, db)
	cam := decodeCameraData[camera.Camera](t, serveJSON(api.router, "POST", "/api/weighing/cameras", `{"name":"识别相机","sourceUrl":"rtsp://camera/recognition"}`, api.token))
	assertEnvelopeCode(t, serveJSON(api.router, "PUT", "/api/weighing/cameras/"+itoa(cam.ID)+"/events", `{"serialNumber":"plate-camera","enabled":true,"timeoutSeconds":180}`, api.token), 200, 200, "success")
	body := fmt.Sprintf(`{"operator":"CarLicenseSnapshot","deviceInfo":{"serialNumber":"plate-camera"},"info":{"eventId":1,"time":%q,"carLicenseAttriInfo":{"Number":"川A54322"}}}`, time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano))
	if response := serveJSON(api.router, "POST", "/api/v1/event/pull", body, ""); response.Code != 200 {
		t.Fatalf("event: %d", response.Code)
	}
	var id int64
	if err := db.GORM.Raw("SELECT id FROM wb_camera_license_snapshot WHERE camera_id=?", cam.ID).Scan(&id).Error; err != nil || id == 0 {
		t.Fatalf("event id: %d %v", id, err)
	}
	api.device.weight("100")
	session := weighingData[weighing.SessionResult](t, api.router, http.MethodPost, "/api/weighing/scales/WB01/session/acquire", `{"clientId":"plate-evidence"}`, api.token)
	business := api.business(t, "川A54321", "inbound")
	evidence := map[string]any{"source": "server", "candidate": "川A54322", "sourceEventRecordIds": []int64{id}, "corrected": true}
	sampleBody := map[string]any{"scaleCode": "WB01", "clientId": "plate-evidence", "sessionToken": session.SessionToken, "business": business, "plateEvidence": evidence}
	confirmationBody := map[string]any{"scaleCode": "WB01", "clientId": "plate-evidence", "sessionToken": session.SessionToken, "confirmedPlate": "川A54321", "evidence": evidence}
	evidence["sourceEventRecordIds"] = []int64{id + 9999}
	assertEnvelopeCode(t, serveJSON(api.router, "POST", "/api/weighing/plate-confirmations", api.encode(t, confirmationBody), api.token), 200, 400, weighing.ErrInvalid.Error())
	evidence["sourceEventRecordIds"] = []int64{id}
	evidence["corrected"] = false
	assertEnvelopeCode(t, serveJSON(api.router, "POST", "/api/weighing/plate-confirmations", api.encode(t, confirmationBody), api.token), 200, 400, weighing.ErrInvalid.Error())
	evidence["corrected"] = true
	oldGroupBody := fmt.Sprintf(`{"operator":"CarLicenseSnapshot","deviceInfo":{"serialNumber":"plate-camera"},"info":{"eventId":2,"time":%q,"carLicenseAttriInfo":{"Number":"川A54322"}}}`, time.Now().Add(-21*time.Second).UTC().Format(time.RFC3339Nano))
	if response := serveJSON(api.router, "POST", "/api/v1/event/pull", oldGroupBody, ""); response.Code != 200 {
		t.Fatalf("second event: %d", response.Code)
	}
	var otherID int64
	if err := db.GORM.Raw("SELECT id FROM wb_camera_license_snapshot WHERE camera_id=? AND id<>?", cam.ID, id).Scan(&otherID).Error; err != nil || otherID == 0 {
		t.Fatal("missing second event")
	}
	if err := db.GORM.Exec("UPDATE wb_camera_license_snapshot SET received_at=? WHERE id=?", time.Now().Add(-20*time.Second).UTC(), otherID).Error; err != nil {
		t.Fatal(err)
	}
	evidence["sourceEventRecordIds"] = []int64{id, otherID}
	assertEnvelopeCode(t, serveJSON(api.router, "POST", "/api/weighing/plate-confirmations", api.encode(t, confirmationBody), api.token), 200, 400, weighing.ErrInvalid.Error())
	evidence["sourceEventRecordIds"] = []int64{id}
	if err := db.GORM.Exec("UPDATE wb_camera_license_snapshot SET received_at=? WHERE id=?", time.Now().Add(-time.Minute), id).Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(api.router, "POST", "/api/weighing/plate-confirmations", api.encode(t, confirmationBody), api.token), 200, 400, weighing.ErrInvalid.Error())
	if err := db.GORM.Exec("UPDATE wb_camera_license_snapshot SET received_at=? WHERE id=?", time.Now().UTC(), id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("UPDATE wb_camera_event_binding SET enabled=false WHERE camera_id=?", cam.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(api.router, "POST", "/api/weighing/plate-confirmations", api.encode(t, confirmationBody), api.token), 200, 400, weighing.ErrInvalid.Error())
	if err := db.GORM.Exec("UPDATE wb_camera_event_binding SET enabled=true WHERE camera_id=?", cam.ID).Error; err != nil {
		t.Fatal(err)
	}
	confirmation := weighingData[struct {
		ConfirmationID int64 `json:"confirmationId"`
	}](t, api.router, "POST", "/api/weighing/plate-confirmations", api.encode(t, confirmationBody), api.token)
	if confirmation.ConfirmationID == 0 {
		t.Fatal("missing confirmation")
	}
	evidence["confirmationId"] = confirmation.ConfirmationID + 9999
	assertEnvelopeCode(t, serveJSON(api.router, "POST", "/api/weighing/samples", api.encode(t, sampleBody), api.token), 200, 400, weighing.ErrInvalid.Error())
	evidence["confirmationId"] = confirmation.ConfirmationID
	if err := db.GORM.Exec("UPDATE wb_plate_confirmation SET actor_id=actor_id+1 WHERE id=?", confirmation.ConfirmationID).Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(api.router, "POST", "/api/weighing/samples", api.encode(t, sampleBody), api.token), 200, 400, weighing.ErrInvalid.Error())
	if err := db.GORM.Exec("UPDATE wb_plate_confirmation SET actor_id=actor_id-1 WHERE id=?", confirmation.ConfirmationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("UPDATE wb_camera_event_binding SET enabled=false WHERE camera_id=?", cam.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(api.router, "POST", "/api/weighing/samples", api.encode(t, sampleBody), api.token), 200, 400, weighing.ErrInvalid.Error())
	if err := db.GORM.Exec("UPDATE wb_camera_event_binding SET enabled=true WHERE camera_id=?", cam.ID).Error; err != nil {
		t.Fatal(err)
	}
	sample := weighingData[weighing.Sample](t, api.router, "POST", "/api/weighing/samples", api.encode(t, sampleBody), api.token)
	var saved struct {
		Source, Candidate, ConfirmedPlate string
		Corrected                         bool
		SourceEvents                      []struct {
			EventRecordID, CameraID int64
			CameraName              string
		}
	}
	if err := json.Unmarshal(sample.PlateEvidence, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Source != "server" || saved.Candidate != "川A54322" || saved.ConfirmedPlate != "川A54321" || !saved.Corrected || len(saved.SourceEvents) != 1 || saved.SourceEvents[0].EventRecordID != id || saved.SourceEvents[0].CameraID != cam.ID {
		t.Fatalf("saved evidence: %+v", saved)
	}
	if len(captureEvidenceOf(t, sample).Images) != 2 {
		t.Fatal("formal RTSP evidence changed")
	}
	checks := map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}
	firstBody := map[string]any{"scaleCode": "WB01", "clientId": "plate-evidence", "sessionToken": session.SessionToken, "business": business, "sampleId": sample.ID, "requestId": "plate-first-0001", "checks": checks, "reviewedVersions": map[string]any{"material": sample.Business.MaterialVersion, "party": sample.Business.PartyVersion, "carrier": sample.Business.CarrierVersion}}
	first := weighingData[weighing.CommandResult](t, api.router, "POST", "/api/weighing/tasks/first-confirmation", api.encode(t, firstBody), api.token)
	active := weighingData[weighing.TaskDetail](t, api.router, "GET", "/api/weighing/tasks/active?scale=WB01&plate=川A54321", "", api.token)
	if active.Task.ID != first.TaskID {
		t.Fatal("return weighing task not found")
	}
	secondCamera := decodeCameraData[camera.Camera](t, serveJSON(api.router, "POST", "/api/weighing/cameras", `{"name":"另一方向","sourceUrl":"rtsp://camera/other"}`, api.token))
	assertEnvelopeCode(t, serveJSON(api.router, "PUT", "/api/weighing/cameras/"+itoa(secondCamera.ID)+"/events", `{"serialNumber":"other-camera","enabled":true,"timeoutSeconds":180}`, api.token), 200, 200, "success")
	secondEvent := fmt.Sprintf(`{"operator":"CarLicenseSnapshot","deviceInfo":{"serialNumber":"other-camera"},"info":{"eventId":1,"time":%q,"carLicenseAttriInfo":{"Number":"川A54321"}}}`, time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano))
	if response := serveJSON(api.router, "POST", "/api/v1/event/pull", secondEvent, ""); response.Code != 200 {
		t.Fatalf("second event: %d", response.Code)
	}
	var secondID int64
	if err := db.GORM.Raw("SELECT id FROM wb_camera_license_snapshot WHERE camera_id=?", secondCamera.ID).Scan(&secondID).Error; err != nil || secondID == 0 {
		t.Fatal("missing second event")
	}
	secondEvidence := map[string]any{"source": "server", "candidate": "川A54321", "sourceEventRecordIds": []int64{secondID}, "corrected": false}
	secondConfirmationBody := map[string]any{"scaleCode": "WB01", "clientId": "plate-evidence", "sessionToken": session.SessionToken, "confirmedPlate": "川A54321", "evidence": secondEvidence}
	secondConfirmation := weighingData[struct {
		ConfirmationID int64 `json:"confirmationId"`
	}](t, api.router, "POST", "/api/weighing/plate-confirmations", api.encode(t, secondConfirmationBody), api.token)
	secondEvidence["confirmationId"] = secondConfirmation.ConfirmationID
	api.device.weight("10")
	secondBody := map[string]any{"scaleCode": "WB01", "clientId": "plate-evidence", "sessionToken": session.SessionToken, "business": business, "taskId": first.TaskID, "plateEvidence": secondEvidence}
	secondSample := weighingData[weighing.Sample](t, api.router, "POST", "/api/weighing/samples", api.encode(t, secondBody), api.token)
	secondConfirm := map[string]any{"scaleCode": "WB01", "clientId": "plate-evidence", "sessionToken": session.SessionToken, "sampleId": secondSample.ID, "version": first.TaskVersion, "requestId": "plate-second-0001", "checks": checks}
	second := weighingData[weighing.CommandResult](t, api.router, "POST", "/api/weighing/tasks/"+itoa(first.TaskID)+"/second-confirmation", api.encode(t, secondConfirm), api.token)
	if second.TicketID == 0 {
		t.Fatal("second weighing did not issue ticket")
	}
	var records []struct{ PlateEvidence json.RawMessage }
	if err := db.GORM.Table("wb_weigh_record").Select("plate_evidence").Where("task_id=?", first.TaskID).Order("id").Scan(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || !strings.Contains(string(records[0].PlateEvidence), `"cameraName": "识别相机"`) || !strings.Contains(string(records[1].PlateEvidence), `"cameraName": "另一方向"`) {
		t.Fatalf("first/second sources not retained: %s", records)
	}
}
