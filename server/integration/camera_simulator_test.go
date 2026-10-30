//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/camera"
	"github.com/EziosWJ/weighbridge/server/internal/camsim"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
)

// TestCameraSimulatorPlateCandidateChain drives the standalone simulator
// against the real API router and isolated PostgreSQL. It verifies that a
// report is rejected until the serial is bound, that retry reuses the same
// event, and that the workbench candidate sees the simulator's plate.
func TestCameraSimulatorPlateCandidateChain(t *testing.T) {
	t.Setenv("WEIGHBRIDGE_CAMERA_EVENT_ALLOWED_IPS", "")
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	deps.Camera = camera.NewService(camera.NewRepository(db.GORM), t.TempDir(), nil, nil, nil)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	testLocalSetup(t, db, router, token, 1)
	status := weighingData[localsite.Status](t, router, "GET", "/api/weighing/local-status", "", token)
	endpoint := "/api/weighing/plate-candidates?scale=" + url.QueryEscape(status.ScaleCode)
	type candidatePage struct {
		Items      []camera.PlateCandidate `json:"items"`
		NextCursor string                  `json:"nextCursor"`
	}
	baseline := decodeCameraData[candidatePage](t, serveJSON(router, "GET", endpoint, "", token))
	if baseline.NextCursor == "" {
		t.Fatal("missing initial candidate cursor")
	}

	simulatedCamera := decodeCameraData[camera.Camera](t, serveJSON(router, "POST", "/api/weighing/cameras", `{"name":"模拟摄像头","sourceUrl":"rtsp://camera/simulator"}`, token))
	apiServer := httptest.NewServer(router)
	defer apiServer.Close()
	sim, err := camsim.New(camsim.Config{ServerURL: apiServer.URL, SerialNumber: "simulator-serial", HeartbeatInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	failed := sim.Recognize(ctx)
	if failed.LastReport == nil || failed.LastReport.Success || failed.LastReport.HTTPStatus != 403 || !strings.Contains(failed.LastReport.Error, "绑定") {
		t.Fatalf("unbound serial was not explained: %+v", failed.LastReport)
	}
	failedEventID := failed.EventID
	if failedEventID == "" || !strings.Contains(failed.LastReport.Error, "HTTP 403") {
		t.Fatalf("missing failed event identity: %+v", failed.LastReport)
	}

	binding := fmt.Sprintf(`{"serialNumber":"simulator-serial","enabled":true,"timeoutSeconds":180}`)
	assertEnvelopeCode(t, serveJSON(router, "PUT", fmt.Sprintf("/api/weighing/cameras/%d/events", simulatedCamera.ID), binding, token), 200, 200, "success")

	retried, err := sim.Retry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if retried.LastReport == nil || !retried.LastReport.Success || retried.LastReport.Attempts != 2 {
		t.Fatalf("retry did not succeed: %+v", retried.LastReport)
	}
	if retried.EventID != failedEventID || retried.Plate == "" {
		t.Fatalf("retry changed the event: %+v", retried)
	}

	reported, err := sim.ReportCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reported.LastReport == nil || !reported.LastReport.Success || reported.EventID == retried.EventID || reported.Plate != retried.Plate {
		t.Fatalf("same-plate re-report: %+v", reported)
	}
	if heartbeat := sim.SendHeartbeat(ctx); !heartbeat.Success {
		t.Fatalf("heartbeat failed: %+v", heartbeat)
	}

	page := decodeCameraData[candidatePage](t, serveJSON(router, "GET", endpoint+"&after="+url.QueryEscape(baseline.NextCursor), "", token))
	if len(page.Items) != 1 || page.Items[0].Plate != retried.Plate || len(page.Items[0].SourceEvents) != 2 {
		t.Fatalf("workbench candidate mismatch: %+v", page.Items)
	}
	if page.Items[0].SourceEvents[0].CameraID != simulatedCamera.ID || page.Items[0].SourceEvents[1].CameraID != simulatedCamera.ID {
		t.Fatalf("candidate source camera mismatch: %+v", page.Items[0].SourceEvents)
	}
	if response := serveJSON(router, "GET", endpoint, "", ""); response.Code == 200 {
		t.Fatal("candidate endpoint exposed without login")
	}
}
