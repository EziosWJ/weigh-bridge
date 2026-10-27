//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

type measurementAPI struct {
	router   http.Handler
	token    string
	hub      *agentbridge.Hub
	device   *weighingAgent
	frames   *captureFrameControl
	imageDir string
}

func newMeasurementAPI(t *testing.T, db *platformdatabase.Database) *measurementAPI {
	t.Helper()
	deps := testDependencies(t, db, t.TempDir())
	t.Cleanup(deps.Notification.Close)
	repo := weighing.NewRepository(db.GORM)
	imageDir := t.TempDir()
	hub := agentbridge.NewMultiple(nil)
	hub.SetLocalImageDir(imageDir)
	hub.SetResolver(func(ctx context.Context, scaleCode string) (agentbridge.Config, error) {
		return repo.AgentConfig(ctx, scaleCode, imageDir)
	})
	deps.Weighing = weighing.NewService(repo, hub)
	cameraService, frames, _ := newCaptureService(t, db, imageDir)
	deps.Camera = cameraService
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	testLocalSetup(t, db, router, token, 1)
	addCaptureCameras(t, db, "rtsp://camera/measurement-1", "rtsp://camera/measurement-2")
	deviceConfig, err := repo.AgentConfig(context.Background(), "WB01", imageDir)
	if err != nil {
		t.Fatal(err)
	}
	device := startWeighingAgentWithConfig(t, router, hub, deviceConfig)
	return &measurementAPI{router: router, token: token, hub: hub, device: device, frames: frames, imageDir: imageDir}
}

func (a *measurementAPI) encode(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (a *measurementAPI) business(t *testing.T, plate, direction string) map[string]any {
	t.Helper()
	material := weighingData[weighing.Material](t, a.router, http.MethodPost, "/api/weighing/materials", `{"name":"测量物料"}`, a.token)
	party := weighingData[weighing.Party](t, a.router, http.MethodPost, "/api/weighing/parties", `{"name":"测量供收方","nature":"enterprise","roles":["supplier","customer"]}`, a.token)
	return map[string]any{"plate": plate, "direction": direction, "materialId": material.ID, "partyId": party.ID}
}

func captureEvidenceOf(t *testing.T, sample weighing.Sample) struct {
	Status     string    `json:"imageStatus"`
	Source     string    `json:"source"`
	Images     []p.Image `json:"images"`
	Failures   []any     `json:"failures"`
	Generation string    `json:"measurementGeneration"`
} {
	t.Helper()
	var evidence struct {
		Status     string    `json:"imageStatus"`
		Source     string    `json:"source"`
		Images     []p.Image `json:"images"`
		Failures   []any     `json:"failures"`
		Generation string    `json:"measurementGeneration"`
	}
	if err := json.Unmarshal(sample.Evidence, &evidence); err != nil {
		t.Fatal(err)
	}
	return evidence
}

// Successful acquisition freezes evidence independently of later live readings.
func TestPostgresSuccessfulCaptureIsFrozenUntilConfirmation(t *testing.T) {
	db := openTemporaryDatabase(t, startBusinessPostgres(t).dsn)
	t.Cleanup(func() { _ = db.Close() })
	api := newMeasurementAPI(t, db)
	api.device.weight("100")
	api.device.plate("川A54322")
	session := weighingData[weighing.SessionResult](t, api.router, http.MethodPost, "/api/weighing/scales/WB01/session/acquire", `{"clientId":"measurement-browser"}`, api.token)
	business := api.business(t, "川A54321", "inbound")
	sampleBody := map[string]any{"scaleCode": "WB01", "clientId": "measurement-browser", "sessionToken": session.SessionToken, "business": business,
		"plateEvidence": map[string]any{"source": "agent", "scenePlate": "川A54321", "candidate": "川A54322", "at": time.Now().UTC().Format(time.RFC3339Nano)}}

	sample := weighingData[weighing.Sample](t, api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, sampleBody), api.token)
	evidence := captureEvidenceOf(t, sample)
	var plateEvidence struct {
		Source         string `json:"source"`
		Candidate      string `json:"candidate"`
		ConfirmedPlate string `json:"confirmedPlate"`
	}
	if err := json.Unmarshal(sample.PlateEvidence, &plateEvidence); err != nil {
		t.Fatal(err)
	}
	// Recognition is not configured in this environment, so the candidate is the
	// Agent's observation and must keep that source rather than becoming "server".
	if plateEvidence.Source != "agent" || plateEvidence.Candidate != "川A54322" || plateEvidence.ConfirmedPlate != "川A54321" {
		t.Fatalf("plate evidence lost its actual source: %+v", plateEvidence)
	}
	if sample.Source != "server" || sample.MeasurementGeneration == "" || evidence.Generation != sample.MeasurementGeneration || len(evidence.Images) != 2 || evidence.Status != "complete" {
		t.Fatalf("new sample evidence=%+v sample=%+v", evidence, sample)
	}
	// Change the reading after success and leave it changed at confirmation.
	api.device.weight("101")
	api.device.plate("川A54329")
	business["driver"] = "核对时补齐司机"
	checks := map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}
	first := map[string]any{"scaleCode": "WB01", "clientId": "measurement-browser", "sessionToken": session.SessionToken, "business": business, "sampleId": sample.ID, "requestId": "measurement-first-0001", "checks": checks, "reviewedVersions": map[string]any{"material": sample.Business.MaterialVersion, "party": sample.Business.PartyVersion, "carrier": sample.Business.CarrierVersion}}
	confirmed := weighingData[weighing.CommandResult](t, api.router, http.MethodPost, "/api/weighing/tasks/first-confirmation", api.encode(t, first), api.token)
	if confirmed.TaskID == 0 || confirmed.RecordID == 0 {
		t.Fatalf("frozen confirmation failed: %+v", confirmed)
	}

	// Complete the second weighing so a formal ticket exists for title checks.
	api.device.weight("10")
	secondBody := map[string]any{"scaleCode": "WB01", "clientId": "measurement-browser", "sessionToken": session.SessionToken, "business": business, "taskId": confirmed.TaskID}
	secondSample := weighingData[weighing.Sample](t, api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, secondBody), api.token)
	// Failure and disconnection after acquisition must not revoke its snapshot.
	api.device.controlAction(`{"action":"unstable"}`)
	api.device.wait(func(state agentbridge.State) bool { return !state.Stable })
	api.device.controlAction(`{"action":"fault"}`)
	api.device.wait(func(state agentbridge.State) bool { return !state.DeviceOK })
	api.device.controlAction(`{"action":"disconnect"}`)
	api.device.wait(func(state agentbridge.State) bool { return !state.Online })
	secondConfirm := map[string]any{"scaleCode": "WB01", "clientId": "measurement-browser", "sessionToken": session.SessionToken, "business": business, "sampleId": secondSample.ID, "requestId": "measurement-second-0001", "version": confirmed.TaskVersion, "checks": checks}
	completed := weighingData[weighing.CommandResult](t, api.router, http.MethodPost, "/api/weighing/tasks/"+itoa(confirmed.TaskID)+"/second-confirmation", api.encode(t, secondConfirm), api.token)
	if completed.TicketID == 0 {
		t.Fatalf("second confirmation produced no ticket: %+v", completed)
	}

	// New tickets freeze the title used at generation; later edits do not
	// rewrite the stored snapshot or the printed sheet.
	ticketPath := "/api/weighing/tickets/" + itoa(completed.TicketID)
	ticket := weighingData[weighing.TicketView](t, api.router, http.MethodGet, ticketPath, "", api.token)
	if ticket.Business.Driver != "核对时补齐司机" {
		t.Fatalf("reviewed business correction was lost: %+v", ticket.Business)
	}
	if ticket.OriginalGross.String() != "100" || ticket.OriginalTare.String() != "10" {
		t.Fatalf("confirmation replaced frozen weights with live readings: %+v", ticket)
	}
	if ticket.TicketTitle == "" || ticket.PrintSheet == nil || !strings.Contains(ticket.PrintSheet.Title, ticket.TicketTitle) {
		t.Fatalf("ticket title snapshot missing: %+v", ticket)
	}
	var titleRow sysconfig.Config
	if err := db.GORM.Where("config_key=?", sysconfig.TicketTitleKey).Take(&titleRow).Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(api.router, http.MethodPut, "/api/system/config/"+itoa(titleRow.ID), `{"configValue":"修改后的标题"}`, api.token), 200, 200, "success")
	unchanged := weighingData[weighing.TicketView](t, api.router, http.MethodGet, ticketPath, "", api.token)
	if unchanged.TicketTitle != ticket.TicketTitle || unchanged.PrintSheet == nil || unchanged.PrintSheet.Title != ticket.PrintSheet.Title {
		t.Fatalf("title edit rewrote an existing ticket: before=%+v after=%+v", ticket, unchanged)
	}
	// Pre-snapshot tickets receive the upgrade-time baseline once.
	if err := db.GORM.Exec("UPDATE wb_ticket SET ticket_title='' WHERE id=?", ticket.ID).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := weighing.FreezeLegacyTicketTitles(ctx, db.GORM, "升级时票面基准"); err != nil {
		t.Fatal(err)
	}
	frozen := weighingData[weighing.TicketView](t, api.router, http.MethodGet, ticketPath, "", api.token)
	if frozen.TicketTitle != "升级时票面基准" || frozen.PrintSheet == nil || !strings.Contains(frozen.PrintSheet.Title, "升级时票面基准") {
		t.Fatalf("legacy baseline not frozen: %+v", frozen)
	}
}

// Required capture needs the frozen non-empty set to succeed in full; optional
// capture keeps every image and per-camera failure fact.
func TestPostgresCaptureRequiresEveryFrozenCameraAndNonEmptySet(t *testing.T) {
	db := openTemporaryDatabase(t, startBusinessPostgres(t).dsn)
	t.Cleanup(func() { _ = db.Close() })
	api := newMeasurementAPI(t, db)
	api.device.weight("20")
	session := weighingData[weighing.SessionResult](t, api.router, http.MethodPost, "/api/weighing/scales/WB01/session/acquire", `{"clientId":"capture-policy-browser"}`, api.token)
	business := api.business(t, "川A54322", "outbound")
	sampleBody := map[string]any{"scaleCode": "WB01", "clientId": "capture-policy-browser", "sessionToken": session.SessionToken, "business": business}

	// One frozen camera fails: required capture must reject the whole sample.
	api.frames.failSource("rtsp://camera/measurement-2", true)
	assertEnvelopeCode(t, serveJSON(api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, sampleBody), api.token), 200, 400, weighing.ErrConditions.Error())
	var samples int64
	if err := db.GORM.Table("wb_sample").Count(&samples).Error; err != nil || samples != 0 {
		t.Fatalf("failed required capture wrote a sample: count=%d err=%v", samples, err)
	}

	// All cameras succeed: both images are frozen into the sample.
	api.frames.failSource("rtsp://camera/measurement-2", false)
	sample := weighingData[weighing.Sample](t, api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, sampleBody), api.token)
	evidence := captureEvidenceOf(t, sample)
	if len(evidence.Images) != 2 || len(evidence.Failures) != 0 || evidence.Status != "complete" {
		t.Fatalf("complete required capture evidence=%+v", evidence)
	}

	// Optional capture keeps the successful image plus the failed camera fact.
	var row sysconfig.Config
	if err := db.GORM.Where("config_key=?", sysconfig.CaptureRequiredKey).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(api.router, http.MethodPut, "/api/system/config/"+itoa(row.ID), `{"configValue":"false"}`, api.token), 200, 200, "success")
	api.frames.failSource("rtsp://camera/measurement-2", true)
	optional := weighingData[weighing.Sample](t, api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, sampleBody), api.token)
	optionalEvidence := captureEvidenceOf(t, optional)
	if len(optionalEvidence.Images) != 1 || len(optionalEvidence.Failures) != 1 || optionalEvidence.Status != "complete" {
		t.Fatalf("optional partial capture evidence=%+v", optionalEvidence)
	}

	// An empty required set cannot masquerade as "all zero cameras succeeded".
	disableCaptureCameras(t, db)
	assertEnvelopeCode(t, serveJSON(api.router, http.MethodPut, "/api/system/config/"+itoa(row.ID), `{"configValue":"true"}`, api.token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, sampleBody), api.token), 200, 400, weighing.ErrCaptureCameraRequired.Error())

	// Optional capture with no camera records the explicit no-image state.
	assertEnvelopeCode(t, serveJSON(api.router, http.MethodPut, "/api/system/config/"+itoa(row.ID), `{"configValue":"false"}`, api.token), 200, 200, "success")
	noImage := weighingData[weighing.Sample](t, api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, sampleBody), api.token)
	noImageEvidence := captureEvidenceOf(t, noImage)
	if noImageEvidence.Status != "disabled" || len(noImageEvidence.Images) != 0 || noImageEvidence.Source != "server" {
		t.Fatalf("optional no-camera evidence=%+v", noImageEvidence)
	}
}

// New Server images and historical Agent front/rear files both remain readable
// after the capture ownership change; history keeps its original view and URL.
func TestPostgresLegacyAndServerImagesRemainReadable(t *testing.T) {
	db := openTemporaryDatabase(t, startBusinessPostgres(t).dsn)
	t.Cleanup(func() { _ = db.Close() })
	api := newMeasurementAPI(t, db)
	api.device.weight("12")
	session := weighingData[weighing.SessionResult](t, api.router, http.MethodPost, "/api/weighing/scales/WB01/session/acquire", `{"clientId":"image-history-browser"}`, api.token)
	business := api.business(t, "川A54323", "inbound")
	sample := weighingData[weighing.Sample](t, api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, map[string]any{"scaleCode": "WB01", "clientId": "image-history-browser", "sessionToken": session.SessionToken, "business": business}), api.token)
	evidence := captureEvidenceOf(t, sample)
	if len(evidence.Images) != 2 {
		t.Fatalf("expected two Server images, got %+v", evidence.Images)
	}
	for _, image := range evidence.Images {
		response := serveJSON(api.router, http.MethodGet, image.URL, "", api.token)
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("server image %s unreadable: status=%d bytes=%d", image.URL, response.Code, response.Body.Len())
		}
	}

	legacy := fmt.Sprintf("%d_%s_front.png", 1, strings.Repeat("a", 32))
	if err := os.WriteFile(filepath.Join(api.imageDir, legacy), testCaptureJPEG(t), 0o600); err != nil {
		t.Fatal(err)
	}
	response := serveJSON(api.router, http.MethodGet, "/api/weighing/agent-images/"+legacy, "", api.token)
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("legacy image unreadable: status=%d bytes=%d", response.Code, response.Body.Len())
	}
	if !strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("legacy image cache header lost: %q", response.Header().Get("Cache-Control"))
	}
	oldCode := "OLD_WB_" + strings.Repeat("b", 32) + "_rear.png"
	if err := os.WriteFile(filepath.Join(api.imageDir, oldCode), testCaptureJPEG(t), 0o600); err != nil {
		t.Fatal(err)
	}
	// History remains readable even when the local site is stopped.
	if err := db.GORM.Exec("UPDATE wb_site SET enabled=0").Error; err != nil {
		t.Fatal(err)
	}
	oldResponse := serveJSON(api.router, http.MethodGet, "/api/weighing/agent-images/"+oldCode, "", api.token)
	if oldResponse.Code != http.StatusOK || oldResponse.Body.Len() == 0 {
		t.Fatalf("old code image unreadable: %d %s", oldResponse.Code, oldResponse.Body.String())
	}
	if err := db.GORM.Exec("DELETE FROM wb_local_user_access WHERE user_id=1").Error; err != nil {
		t.Fatal(err)
	}
	denied := serveJSON(api.router, http.MethodGet, "/api/weighing/agent-images/"+oldCode, "", api.token)
	assertEnvelopeCode(t, denied, http.StatusForbidden, http.StatusForbidden, "无权执行此操作")
}

func TestPostgresRecaptureInvalidatesPreviousCodeSnapshot(t *testing.T) {
	db := openTemporaryDatabase(t, startBusinessPostgres(t).dsn)
	t.Cleanup(func() { _ = db.Close() })
	api := newMeasurementAPI(t, db)
	api.device.weight("12")
	session := weighingData[weighing.SessionResult](t, api.router, http.MethodPost, "/api/weighing/scales/WB01/session/acquire", `{"clientId":"recapture-old-code"}`, api.token)
	business := api.business(t, "川A54324", "inbound")
	body := map[string]any{"scaleCode": "WB01", "clientId": "recapture-old-code", "sessionToken": session.SessionToken, "business": business}
	first := weighingData[weighing.Sample](t, api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, body), api.token)
	if err := db.GORM.Exec("UPDATE wb_sample SET scale_code='OLD_SCALE' WHERE id=?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	second := weighingData[weighing.Sample](t, api.router, http.MethodPost, "/api/weighing/samples", api.encode(t, body), api.token)
	var old weighing.Sample
	if err := db.GORM.Where("id=?", first.ID).Take(&old).Error; err != nil || old.InvalidatedAt == nil {
		t.Fatalf("old capture survived explicit recapture: %+v err=%v", old, err)
	}
	if second.InvalidatedAt != nil {
		t.Fatalf("new capture invalidated: %+v", second)
	}
	confirm := map[string]any{"scaleCode": "WB01", "clientId": "recapture-old-code", "sessionToken": session.SessionToken, "business": business, "sampleId": first.ID, "requestId": "old-code-recapture-0001", "checks": map[string]bool{"plate": true, "business": true, "weight": true, "evidence": true, "fullyOnScale": true}}
	response := serveJSON(api.router, http.MethodPost, "/api/weighing/tasks/first-confirmation", api.encode(t, confirm), api.token)
	assertEnvelopeCode(t, response, http.StatusOK, http.StatusBadRequest, weighing.ErrConditions.Error())
}
