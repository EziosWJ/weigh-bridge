//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/camera"
)

type cameraIntegrationFrame struct{ data []byte }

func (f cameraIntegrationFrame) Snapshot(context.Context, string, string, string) (camera.Frame, error) {
	return camera.Frame{Data: f.data, Width: 4, Height: 2, Format: "jpeg"}, nil
}

type cameraIntegrationRecognizer struct{ candidate string }

func (r cameraIntegrationRecognizer) Recognize(context.Context, string, []byte) (string, error) {
	return r.candidate, nil
}

type cameraIntegrationControl struct {
	mu      sync.Mutex
	exists  map[string]bool
	sources map[string]string
	adds    int
	deletes int
}

func (c *cameraIntegrationControl) Get(_ context.Context, _ camera.Endpoint, name string) (string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sources[name], c.exists[name], nil
}

func (c *cameraIntegrationControl) Add(_ context.Context, _ camera.Endpoint, name, source string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exists == nil {
		c.exists = map[string]bool{}
		c.sources = map[string]string{}
	}
	c.exists[name], c.sources[name] = true, source
	c.adds++
	return nil
}

func (c *cameraIntegrationControl) Patch(_ context.Context, _ camera.Endpoint, name, source string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sources[name] = source
	return nil
}

func (c *cameraIntegrationControl) Delete(_ context.Context, _ camera.Endpoint, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.exists, name)
	delete(c.sources, name)
	c.deletes++
	return nil
}

func cameraIntegrationJPEG(t *testing.T) []byte {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			source.Set(x, y, color.RGBA{R: 40, G: 100, B: 160, A: 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, source, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func decodeCameraData[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
		Data T   `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode camera response: %v", err)
	}
	if payload.Code != 200 {
		t.Fatalf("camera response code=%d body=%s", payload.Code, response.Body.Bytes())
	}
	return payload.Data
}

func TestPostgresCameraModuleCRUDVersioningCaptureAndSync(t *testing.T) {
	temporary := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	imageDir := t.TempDir()
	deps := testDependencies(t, db, t.TempDir())
	control := &cameraIntegrationControl{}
	recognizer := cameraIntegrationRecognizer{candidate: "川A12345"}
	frameData := cameraIntegrationJPEG(t)
	cameraService := camera.NewService(camera.NewRepository(db.GORM), imageDir, cameraIntegrationFrame{data: frameData}, recognizer, control)
	deps.Camera = cameraService
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	testLocalSetup(t, db, router, token, 1)

	createdResponse := serveJSON(router, http.MethodPost, "/api/weighing/cameras",
		`{"name":"入口","sourceUrl":"rtsp://camera/1","username":"operator","password":"camera-secret","captureEnabled":true}`, token)
	created := decodeCameraData[camera.Camera](t, createdResponse)
	if created.ID < 1 || created.Version != 1 || !created.HasPassword || created.Stream.State != camera.StreamPending {
		t.Fatalf("created=%+v", created)
	}
	if strings.Contains(createdResponse.Body.String(), "camera-secret") || strings.Contains(createdResponse.Body.String(), `"password":`) {
		t.Fatalf("secret leaked in camera response: %s", createdResponse.Body.String())
	}

	listed := decodeCameraData[camera.List](t, serveJSON(router, http.MethodGet, "/api/weighing/cameras", "", token))
	if len(listed.Cameras) != 1 || listed.ConfigVersion != 2 || listed.CaptureTimeoutMS != 6000 {
		t.Fatalf("listed=%+v", listed)
	}

	kept := decodeCameraData[camera.Camera](t, serveJSON(router, http.MethodPut, "/api/weighing/cameras/"+itoa(created.ID),
		`{"name":"入口","sourceUrl":"rtsp://camera/1","username":"operator","version":1}`, token))
	if kept.Version != 2 || !kept.HasPassword {
		t.Fatalf("blank password did not keep the secret: %+v", kept)
	}
	listed = decodeCameraData[camera.List](t, serveJSON(router, http.MethodGet, "/api/weighing/cameras", "", token))
	if listed.ConfigVersion != 2 {
		t.Fatalf("non-content edit bumped config version: %+v", listed)
	}

	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, "/api/weighing/cameras/"+itoa(created.ID),
		`{"name":"入口","sourceUrl":"rtsp://camera/1","version":1}`, token), 200, 400, camera.ErrConflict.Error())

	cleared := decodeCameraData[camera.Camera](t, serveJSON(router, http.MethodPut, "/api/weighing/cameras/"+itoa(created.ID),
		`{"name":"入口","sourceUrl":"rtsp://camera/1","username":"operator","passwordAction":"clear","version":2}`, token))
	if cleared.Version != 3 || cleared.HasPassword {
		t.Fatalf("clear did not erase the secret: %+v", cleared)
	}

	settings := decodeCameraData[camera.CaptureSettings](t, serveJSON(router, http.MethodPut, "/api/weighing/cameras/settings",
		`{"configVersion":3,"captureTimeoutMs":7000,"recognitionCameraId":1,"recognitionUrl":"https://recognizer.local/api"}`, token))
	if settings.ConfigVersion != 4 || settings.CaptureTimeoutMS != 7000 || settings.RecognitionCameraID == nil || *settings.RecognitionCameraID != 1 {
		t.Fatalf("settings=%+v", settings)
	}

	video := decodeCameraData[camera.VideoSettings](t, serveJSON(router, http.MethodPut, "/api/weighing/cameras/video",
		`{"apiUrl":"http://media.example:9997","apiUsername":"media","apiPassword":"media-secret","whepBaseUrl":"https://media.example:8889","version":1}`, token))
	if video.Version != 2 || !video.HasAPIPassword || video.SyncState != camera.VideoSyncPending {
		t.Fatalf("video=%+v", video)
	}

	synced := decodeCameraData[camera.SyncResult](t, serveJSON(router, http.MethodPost, "/api/weighing/cameras/sync",
		`{"configVersion":4,"videoVersion":2}`, token))
	if synced.State != camera.VideoSyncSynced || len(synced.Streams) != 1 || control.adds != 1 {
		t.Fatalf("sync=%+v adds=%d", synced, control.adds)
	}
	if synced.Streams[0].PlaybackURL != "https://media.example:8889/wb-"+created.PublicID+"/whep" {
		t.Fatalf("playback url=%q", synced.Streams[0].PlaybackURL)
	}

	tested := decodeCameraData[camera.TestResult](t, serveJSON(router, http.MethodPost, "/api/weighing/cameras/"+itoa(created.ID)+"/test", "", token))
	if !tested.Success || len(tested.JPEG) == 0 {
		t.Fatalf("test result=%+v", tested)
	}
	var imageCount int64
	if err := db.GORM.Table("wb_weighing_image").Count(&imageCount).Error; err != nil {
		t.Fatal(err)
	}
	if imageCount != 0 {
		t.Fatalf("diagnostic test created %d formal image rows", imageCount)
	}

	plan, err := cameraService.Plan(context.Background(), "WB01", "camera-integration", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Cameras) != 1 || plan.ConfigVersion != 4 || plan.TimeoutMS != 7000 {
		t.Fatalf("plan=%+v", plan)
	}
	outcome, err := cameraService.Capture(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Success || len(outcome.Images) != 1 || len(outcome.Failures) != 0 {
		t.Fatalf("outcome=%+v", outcome)
	}
	imageID := outcome.Images[0].ID
	if outcome.Images[0].Source != "server" || outcome.Images[0].CameraName != "入口" {
		t.Fatalf("image metadata=%+v", outcome.Images[0])
	}
	path, err := cameraService.ImagePath(context.Background(), imageID)
	if err != nil || !strings.HasSuffix(path, imageID) {
		t.Fatalf("image path=%q err=%v", path, err)
	}
	candidate, err := cameraService.RecognizeOutcome(context.Background(), plan, outcome)
	if err != nil || candidate != "川A12345" {
		t.Fatalf("candidate=%q err=%v", candidate, err)
	}
	if err := db.GORM.Table("wb_weighing_image").Count(&imageCount).Error; err != nil {
		t.Fatal(err)
	}
	if imageCount != 1 {
		t.Fatalf("formal capture image rows=%d", imageCount)
	}

	assertEnvelopeCode(t, serveJSON(router, http.MethodDelete, "/api/weighing/cameras/"+itoa(created.ID), `{"version":3}`, token), 200, 200, "success")
	listed = decodeCameraData[camera.List](t, serveJSON(router, http.MethodGet, "/api/weighing/cameras", "", token))
	if len(listed.Cameras) != 0 {
		t.Fatalf("deleted camera still listed: %+v", listed)
	}
	removal := decodeCameraData[camera.SyncResult](t, serveJSON(router, http.MethodPost, "/api/weighing/cameras/sync", `{}`, token))
	if removal.State != camera.VideoSyncSynced || control.deletes != 1 {
		t.Fatalf("removal sync=%+v deletes=%d", removal, control.deletes)
	}
	var streamState string
	if err := db.GORM.Table("wb_camera_stream").Select("state").Where("camera_id=?", created.ID).Scan(&streamState).Error; err != nil {
		t.Fatal(err)
	}
	if streamState != camera.StreamRemoved {
		t.Fatalf("stream state=%q", streamState)
	}
	// Historical image evidence survives camera deletion.
	if _, err := cameraService.ImagePath(context.Background(), imageID); err != nil {
		t.Fatalf("historical image unavailable after delete: %v", err)
	}
}
