//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/camera"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
)

func TestPlateCandidateAPI(t *testing.T) {
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
	type page struct {
		Items      []camera.PlateCandidate `json:"items"`
		NextCursor string                  `json:"nextCursor"`
	}
	baseline := decodeCameraData[page](t, serveJSON(router, "GET", endpoint, "", token))
	if baseline.NextCursor == "" {
		t.Fatal("missing initial cursor")
	}
	var firstCameraID int64
	for i := 0; i < 2; i++ {
		cam := decodeCameraData[camera.Camera](t, serveJSON(router, "POST", "/api/weighing/cameras", fmt.Sprintf(`{"name":"camera%d","sourceUrl":"rtsp://camera/%d"}`, i, i), token))
		if i == 0 {
			firstCameraID = cam.ID
		}
		binding := fmt.Sprintf(`{"serialNumber":"serial%d","enabled":true,"timeoutSeconds":180}`, i)
		assertEnvelopeCode(t, serveJSON(router, "PUT", "/api/weighing/cameras/"+itoa(cam.ID)+"/events", binding, token), 200, 200, "success")
		body := fmt.Sprintf(`{"operator":"CarLicenseSnapshot","deviceInfo":{"serialNumber":"serial%d"},"info":{"eventId":1,"time":%q,"carLicenseAttriInfo":{"Number":"晋A12345"}}}`, i, time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano))
		if got := serveJSON(router, "POST", "/api/v1/event/pull", body, ""); got.Code != 200 {
			t.Fatalf("event status %d", got.Code)
		}
		if got := serveJSON(router, "POST", "/api/v1/event/pull", body, ""); got.Code != 200 {
			t.Fatalf("retry status %d", got.Code)
		}
	}
	page1 := decodeCameraData[page](t, serveJSON(router, "GET", endpoint+"&after="+url.QueryEscape(baseline.NextCursor), "", token))
	if len(page1.Items) != 1 || page1.Items[0].Plate != "晋A12345" || len(page1.Items[0].SourceEvents) != 2 {
		t.Fatalf("unexpected candidates: %+v", page1.Items)
	}
	page2 := decodeCameraData[page](t, serveJSON(router, "GET", endpoint+"&after="+url.QueryEscape(page1.NextCursor), "", token))
	if len(page2.Items) != 0 {
		t.Fatalf("duplicate candidates: %+v", page2.Items)
	}
	if response := serveJSON(router, "GET", endpoint, "", ""); response.Code == 200 {
		t.Fatal("unauthenticated candidate access")
	}
	var roleID int64
	if err := db.GORM.Table("sys_role").Select("id").Where("role_code='WEIGHING_OPERATOR' AND status=1 AND deleted=0").Scan(&roleID).Error; err != nil || roleID == 0 {
		t.Fatal("missing operator role")
	}
	for _, name := range []string{"candidate-operator", "candidate-no-grant"} {
		response := serveJSON(router, "POST", "/api/system/user", fmt.Sprintf(`{"username":%q,"nickname":%q,"status":1}`, name, name), token)
		if response.Code != 200 {
			t.Fatalf("create operator: %d", response.Code)
		}
		var userID int64
		if err := db.GORM.Table("sys_user").Select("id").Where("username=?", name).Scan(&userID).Error; err != nil || userID == 0 {
			t.Fatal("missing operator")
		}
		assertEnvelopeCode(t, serveJSON(router, "PUT", fmt.Sprintf("/api/system/user/%d/roles", userID), fmt.Sprintf(`{"roleIds":[%d]}`, roleID), token), 200, 200, "success")
		if name == "candidate-operator" {
			assertEnvelopeCode(t, serveJSON(router, "PUT", fmt.Sprintf("/api/weighing/users/%d/sites", userID), fmt.Sprintf(`{"grants":[{"siteCode":%q,"canOperate":1}]}`, status.SiteCode), token), 200, 200, "success")
		}
	}
	operator := loginUser(t, router, "candidate-operator", "admin123")
	allowed := decodeCameraData[page](t, serveJSON(router, "GET", endpoint, "", operator))
	if len(allowed.Items) != 1 {
		t.Fatalf("operator cannot read candidates: %+v", allowed.Items)
	}
	denied := loginUser(t, router, "candidate-no-grant", "admin123")
	if response := serveJSON(router, "GET", endpoint, "", denied); response.Code != 403 {
		t.Fatalf("ungranted access: %d", response.Code)
	}
	if response := serveJSON(router, "GET", "/api/weighing/plate-candidates?scale=OTHER", "", operator); response.Code == 200 {
		t.Fatal("other scale accepted")
	}
	// A transaction holding an allocated but uncommitted event must not allow
	// a reader to advance its watermark past that event.
	tx := db.GORM.Begin()
	defer tx.Rollback()
	if err := camera.LockPlateEvents(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Second)
	if err := tx.Exec(`INSERT INTO wb_camera_license_snapshot (camera_id,serial_number,event_id,device_time,received_at,plate,normalized_plate,attributes,sha256,capture_bytes,background_bytes) VALUES (?,?,?,?,?,?,?,'{}',?,0,0)`, firstCameraID, "serial0", "concurrent", now.Add(-time.Second).Format(time.RFC3339Nano), now, "晋A99999", "晋A99999", "concurrent-hash").Error; err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result <- serveJSON(router, "GET", endpoint+"&after="+url.QueryEscape(page1.NextCursor), "", token)
	}()
	select {
	case <-result:
		t.Fatal("watermark read passed an uncommitted event")
	case <-time.After(150 * time.Millisecond):
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-result:
		got := decodeCameraData[page](t, response)
		if len(got.Items) != 1 || got.Items[0].Plate != "晋A99999" {
			t.Fatalf("missing committed event: %+v", got.Items)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watermark reader did not resume")
	}
	// A later batch of the same plate keeps one candidate slot; the newest
	// group is returned and the older batch is not listed separately.
	older := time.Now().UTC().Add(-21 * time.Second)
	if err := db.GORM.Exec(`INSERT INTO wb_camera_license_snapshot (camera_id,serial_number,event_id,device_time,received_at,plate,normalized_plate,attributes,sha256,capture_bytes,background_bytes) VALUES (?,?,?,?,?,?,?,'{}',?,0,0)`, firstCameraID, "serial0", "older-batch", older.Add(-time.Second).Format(time.RFC3339Nano), older, "晋A12345", "晋A12345", "older-batch-hash").Error; err != nil {
		t.Fatal(err)
	}
	deduped := decodeCameraData[page](t, serveJSON(router, "GET", endpoint, "", token))
	if len(deduped.Items) != 2 {
		t.Fatalf("same plate occupied extra slots: %+v", deduped.Items)
	}
	for _, item := range deduped.Items {
		if item.Plate == "晋A12345" && len(item.SourceEvents) != 2 {
			t.Fatalf("kept the older batch: %+v", item.SourceEvents)
		}
	}
}
