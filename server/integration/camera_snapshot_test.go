//go:build integration

package integration

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/camera"
)

func TestPostgresCameraLicenseSnapshots(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	dir := t.TempDir()
	deps := testDependencies(t, db, t.TempDir())
	deps.Camera = camera.NewService(camera.NewRepository(db.GORM), dir, nil, nil, nil)
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	testLocalSetup(t, db, router, token, 1)
	first := decodeCameraData[camera.Camera](t, serveJSON(router, "POST", "/api/weighing/cameras", `{"name":"one","sourceUrl":"rtsp://camera/one"}`, token))
	second := decodeCameraData[camera.Camera](t, serveJSON(router, "POST", "/api/weighing/cameras", `{"name":"two","sourceUrl":"rtsp://camera/two"}`, token))
	endpoint := func(id int64) string { return "/api/weighing/cameras/" + itoa(id) + "/events" }
	for i, cam := range []camera.Camera{first, second} {
		assertEnvelopeCode(t, serveJSON(router, "PUT", endpoint(cam.ID), fmt.Sprintf(`{"serialNumber":"serial%d","enabled":true,"timeoutSeconds":180}`, i), token), 200, 200, "success")
	}
	img := cameraIntegrationJPEG(t)
	encoded := base64.StdEncoding.EncodeToString(img)
	makeBody := func(serial, plate string) string {
		return fmt.Sprintf(`{"operator":"CarLicenseSnapshot","deviceInfo":{"serialNumber":%q},"info":{"eventId":90071992547409931234,"time":"2026-10-29T14:19:44+08:00","carLicenseAttriInfo":{"Number":%q,"CarLicenseAttriColor":1},"carAttriInfo":{"Score":99},"CaptureImage":{"width":4,"height":2,"pictureLength":%d,"base64PicLength":%d,"picture":%q},"BackgroundImage":{"picture":%q}}}`, serial, plate, len(img), len(encoded), encoded, encoded)
	}
	freshTime := time.Now().UTC().Format(time.RFC3339Nano)
	fresh := func(body string) string { return strings.ReplaceAll(body, "2026-10-29T14:19:44+08:00", freshTime) }
	postBody := makeBody
	makeBody = func(serial, plate string) string { return fresh(postBody(serial, plate)) }
	body := makeBody("serial0", "晋 A·12345")
	post := func(s string) int { return serveJSON(router, "POST", "/api/v1/event/pull", s, "").Code }
	response := serveJSON(router, "POST", "/api/v1/event/pull", body, "")
	if response.Code != 200 || response.Body.String() != `{"operator":"CarLicenseSnapshot-Ack","info":{"eventID":90071992547409931234},"result":{"errorNo":0,"description":"ok"}}` {
		t.Fatalf("ack %d %s", response.Code, response.Body.String())
	}
	var wg sync.WaitGroup
	codes := make(chan int, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- post(body) }()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatalf("duplicate status %d", code)
		}
	}
	if post(makeBody("serial0", "晋A54321")) != 200 || post(makeBody("serial1", "晋A12345")) != 200 {
		t.Fatal("distinct events rejected")
	}
	if post(makeBody("missing", "晋A12345")) != 403 {
		t.Fatal("unbound camera accepted")
	}
	if post(strings.Replace(body, "晋 A·12345", "", 1)) != 200 {
		t.Fatal("unrecognized plate event rejected")
	}
	// Missing or invalid images do not prevent plate reception.
	noImage := `{"operator":"CarLicenseSnapshot","deviceInfo":{"serialNumber":"serial0"},"info":{"eventId":2,"time":"2026-10-29T14:19:44+08:00","carLicenseAttriInfo":{"Number":"晋A88888"},"CaptureImage":{"picture":"invalid"}}}`
	if post(fresh(noImage)) != 200 {
		t.Fatal("plate without images rejected")
	}
	old := strings.Replace(makeBody("serial0", "晋A33333"), freshTime, time.Now().Add(-6*time.Minute).UTC().Format(time.RFC3339Nano), 1)
	oldResponse := serveJSON(router, "POST", "/api/v1/event/pull", old, "")
	if oldResponse.Code != 200 || !strings.Contains(oldResponse.Body.String(), `"eventID":90071992547409931234`) {
		t.Fatalf("expired ACK: %d %s", oldResponse.Code, oldResponse.Body.String())
	}
	if post(old) != 200 {
		t.Fatal("expired repeat not acknowledged")
	}
	badTime := strings.Replace(makeBody("serial0", "晋A44444"), freshTime, "invalid-time", 1)
	if post(badTime) != 400 {
		t.Fatal("invalid device time accepted")
	}
	if post(strings.Replace(old, "serial0", "missing", 1)) != 403 {
		t.Fatal("unbound old event acknowledged")
	}
	retry := makeBody("serial0", "晋A22222")
	if err := db.GORM.Exec(`CREATE FUNCTION snapshot_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'failure'; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec(`CREATE TRIGGER snapshot_failure BEFORE INSERT ON wb_camera_license_snapshot FOR EACH ROW EXECUTE FUNCTION snapshot_failure()`).Error; err != nil {
		t.Fatal(err)
	}
	if post(retry) != 503 {
		t.Fatal("database failure acknowledged")
	}
	if err := db.GORM.Exec(`DROP TRIGGER snapshot_failure ON wb_camera_license_snapshot`).Error; err != nil {
		t.Fatal(err)
	}
	if post(retry) != 200 {
		t.Fatal("retry failed")
	}
	var n int64
	if err := db.GORM.Table("wb_camera_license_snapshot").Count(&n).Error; err != nil || n != 6 {
		t.Fatalf("count %d %v", n, err)
	}
	var config struct{ ID int64 }
	if err := db.GORM.Table("sys_config").Select("id").Where("config_key=? AND deleted=0", "weighing.camera-event-max-age-minutes").Take(&config).Error; err != nil {
		t.Fatal(err)
	}
	configPath := "/api/system/config/" + itoa(config.ID)
	assertEnvelopeCode(t, serveJSON(router, "PUT", configPath, `{"configValue":"10"}`, token), 200, 200, "success")

	if post(old) != 200 {
		t.Fatal("updated threshold rejected recent event")
	}
	if err := db.GORM.Table("wb_camera_license_snapshot").Count(&n).Error; err != nil || n != 7 {
		t.Fatalf("updated threshold did not persist event: %d %v", n, err)
	}
	assertEnvelopeCode(t, serveJSON(router, "PUT", configPath, `{"configValue":"1"}`, token), 200, 200, "success")

	expired := strings.Replace(makeBody("serial0", "晋A77777"), freshTime, time.Now().Add(-2*time.Minute).UTC().Format(time.RFC3339Nano), 1)
	if post(expired) != 200 {
		t.Fatal("updated threshold did not ack expired event")
	}
	if err := db.GORM.Table("wb_camera_license_snapshot").Count(&n).Error; err != nil || n != 7 {
		t.Fatalf("expired event persisted: %d %v", n, err)
	}
	type result struct {
		Snapshots []camera.LicenseSnapshot `json:"snapshots"`
	}
	got := decodeCameraData[result](t, serveJSON(router, "GET", endpoint(first.ID), "", token))
	if len(got.Snapshots) != 6 {
		t.Fatalf("snapshots %v", got.Snapshots)
	}
	var row camera.LicenseSnapshot
	for _, item := range got.Snapshots {
		if item.Plate == "晋 A·12345" {
			row = item
		}
	}
	if row.ID == 0 || row.NormalizedPlate != "晋A12345" {
		t.Fatalf("plate %+v", row)
	}
	if serveJSON(router, "GET", endpoint(first.ID), "", "").Code != 401 {
		t.Fatal("anonymous records access")
	}
	if err := db.GORM.Exec("UPDATE sys_menu SET status=0 WHERE permission_code='weighing:camera:manage'").Error; err != nil {
		t.Fatal(err)
	}
	if serveJSON(router, "GET", endpoint(first.ID), "", token).Code != 403 {
		t.Fatal("records allowed without manage permission")
	}
}
