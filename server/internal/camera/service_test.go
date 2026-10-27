package camera

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

type fakeStore struct {
	Store
	mu          sync.Mutex
	settings    settingsRecord
	cameras     []cameraRecord
	streams     []streamRecord
	images      map[string]imageRecord
	saved       []imageRecord
	audits      []audit.Event
	markSynced  []int64
	markErrors  []string
	markRemoved []int64
	videoState  string
}

func (f *fakeStore) ListState(context.Context) (settingsRecord, []cameraRecord, []streamRecord, error) {
	return f.settings, f.cameras, f.streams, nil
}

func (f *fakeStore) Camera(_ context.Context, id int64) (cameraRecord, error) {
	for _, camera := range f.cameras {
		if camera.ID == id {
			return camera, nil
		}
	}
	return cameraRecord{}, ErrNotFound
}

func (f *fakeStore) SaveImage(_ context.Context, record imageRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.images == nil {
		f.images = map[string]imageRecord{}
	}
	f.images[record.ID] = record
	f.saved = append(f.saved, record)
	return nil
}

func (f *fakeStore) Image(_ context.Context, id string) (imageRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.images[id]
	if !ok {
		return imageRecord{}, ErrNotFound
	}
	return record, nil
}

func (f *fakeStore) RecordAudit(_ context.Context, event audit.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits = append(f.audits, event)
	return nil
}

func (f *fakeStore) SyncSnapshot(context.Context) (settingsRecord, []streamRecord, map[int64]cameraRecord, error) {
	cameras := map[int64]cameraRecord{}
	for _, camera := range f.cameras {
		cameras[camera.ID] = camera
	}
	return f.settings, f.streams, cameras, nil
}

func (f *fakeStore) MarkStreamSynced(_ context.Context, _ int64, version int64) error {
	f.markSynced = append(f.markSynced, version)
	return nil
}

func (f *fakeStore) MarkStreamError(_ context.Context, _ int64, _ int64, message string) error {
	f.markErrors = append(f.markErrors, message)
	return nil
}

func (f *fakeStore) MarkStreamRemoved(_ context.Context, _ int64, _ int64) error {
	f.markRemoved = append(f.markRemoved, 1)
	return nil
}

func (f *fakeStore) UpdateVideoSync(_ context.Context, _ int64, state, _ string, _ bool) error {
	f.videoState = state
	return nil
}

type frameFunc func(ctx context.Context, source, username, password string) (Frame, error)

func (f frameFunc) Snapshot(ctx context.Context, source, username, password string) (Frame, error) {
	return f(ctx, source, username, password)
}

type controlCall struct {
	Method  string
	Path    string
	Source  string
	Desired int64
}

type fakeControl struct {
	mu      sync.Mutex
	exists  map[string]bool
	sources map[string]string
	calls   []controlCall
	err     error
}

func (f *fakeControl) record(call controlCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeControl) Get(_ context.Context, _ Endpoint, name string) (string, bool, error) {
	f.record(controlCall{Method: "get", Path: name})
	if f.err != nil {
		return "", false, f.err
	}
	if f.exists == nil {
		return "", false, nil
	}
	return f.sources[name], f.exists[name], nil
}

func (f *fakeControl) Add(_ context.Context, _ Endpoint, name, source string) error {
	f.record(controlCall{Method: "add", Path: name, Source: source})
	if f.exists == nil {
		f.exists = map[string]bool{}
	}
	f.exists[name] = true
	return nil
}

func (f *fakeControl) Patch(_ context.Context, _ Endpoint, name, source string) error {
	f.record(controlCall{Method: "patch", Path: name, Source: source})
	return nil
}

func (f *fakeControl) Delete(_ context.Context, _ Endpoint, name string) error {
	f.record(controlCall{Method: "delete", Path: name})
	return nil
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			source.Set(x, y, color.RGBA{R: 80, G: 120, B: 200, A: 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, source, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestCaptureUsesOneDeadlineForAllCameras(t *testing.T) {
	data := testJPEG(t)
	store := &fakeStore{}
	grabber := frameFunc(func(ctx context.Context, _, _, _ string) (Frame, error) {
		select {
		case <-time.After(250 * time.Millisecond):
			return Frame{Data: data, Width: 4, Height: 2, Format: "jpeg"}, nil
		case <-ctx.Done():
			return Frame{}, errFrameTimeout
		}
	})
	service := NewService(store, t.TempDir(), grabber, nil, &fakeControl{})
	plan := CapturePlan{ScaleCode: "WB01", ConfigVersion: 7, TimeoutMS: 400, Required: true, Cameras: []CaptureCamera{
		{ID: 1, PublicID: "cam_a", Name: "入口", SourceURL: "rtsp://a", Version: 1},
		{ID: 2, PublicID: "cam_b", Name: "出口", SourceURL: "rtsp://b", Version: 1},
	}}
	started := time.Now()
	outcome, err := service.Capture(context.Background(), plan)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Success || len(outcome.Images) != 2 || len(outcome.Failures) != 0 {
		t.Fatalf("outcome=%+v", outcome)
	}
	if elapsed > 390*time.Millisecond {
		t.Fatalf("capture elapsed %v; cameras were not bounded by one deadline", elapsed)
	}
	if outcome.ConfigVersion != 7 {
		t.Fatalf("config version=%d", outcome.ConfigVersion)
	}
	if len(store.saved) != 2 || store.saved[0].Source != "server" || store.saved[0].SHA256 == "" {
		t.Fatalf("saved=%+v", store.saved)
	}
	for _, image := range outcome.Images {
		if !strings.HasPrefix(image.URL, "/api/weighing/images/") || image.CameraName == "" || image.CapturedAt == nil {
			t.Fatalf("image=%+v", image)
		}
	}
}

func TestCaptureRequiredRejectsEmptyCameraSet(t *testing.T) {
	service := NewService(&fakeStore{}, t.TempDir(), nil, nil, nil)
	outcome, err := service.Capture(context.Background(), CapturePlan{ScaleCode: "WB01", Required: true})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Success || len(outcome.Failures) != 1 || outcome.Failures[0].Code != CodeCaptureRequired {
		t.Fatalf("outcome=%+v", outcome)
	}
}

func TestOptionalCaptureAllowsEmptyCameraSet(t *testing.T) {
	service := NewService(&fakeStore{}, t.TempDir(), nil, nil, nil)
	outcome, err := service.Capture(context.Background(), CapturePlan{ScaleCode: "WB01", Required: false})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Success || len(outcome.Failures) != 0 || len(outcome.Images) != 0 {
		t.Fatalf("outcome=%+v", outcome)
	}
}

func TestCapturePreservesPerCameraFailures(t *testing.T) {
	data := testJPEG(t)
	store := &fakeStore{}
	grabber := frameFunc(func(ctx context.Context, source, _, _ string) (Frame, error) {
		if source == "rtsp://bad" {
			<-ctx.Done()
			return Frame{}, errFrameTimeout
		}
		return Frame{Data: data, Width: 4, Height: 2, Format: "jpeg"}, nil
	})
	service := NewService(store, t.TempDir(), grabber, nil, &fakeControl{})
	plan := CapturePlan{ScaleCode: "WB01", TimeoutMS: 100, Required: true, Cameras: []CaptureCamera{
		{ID: 1, PublicID: "cam_ok", Name: "可用", SourceURL: "rtsp://ok"},
		{ID: 2, PublicID: "cam_bad", Name: "故障", SourceURL: "rtsp://bad"},
	}}
	outcome, err := service.Capture(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Success || len(outcome.Images) != 1 || len(outcome.Failures) != 1 {
		t.Fatalf("outcome=%+v", outcome)
	}
	failure := outcome.Failures[0]
	if failure.Code != CodeTimeout || failure.CameraName != "故障" || failure.CameraID != 2 {
		t.Fatalf("failure=%+v", failure)
	}
	// A failed acquisition is auditable and never receives a fake image URL.
	if len(store.saved) != 2 || store.saved[1].Status != "failed" || store.saved[1].FileName != "" {
		t.Fatalf("saved=%+v", store.saved)
	}
}

func TestOptionalCaptureKeepsFailureDetailsWithoutBlocking(t *testing.T) {
	service := NewService(&fakeStore{}, t.TempDir(), frameFunc(func(context.Context, string, string, string) (Frame, error) {
		return Frame{}, errFrameUnavailable
	}), nil, &fakeControl{})
	outcome, err := service.Capture(context.Background(), CapturePlan{ScaleCode: "WB01", TimeoutMS: 100, Cameras: []CaptureCamera{
		{ID: 1, PublicID: "cam_a", Name: "入口", SourceURL: "rtsp://camera/1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Success || len(outcome.Images) != 0 || len(outcome.Failures) != 1 {
		t.Fatalf("optional capture outcome=%+v", outcome)
	}
	if outcome.Failures[0].Code != CodeUnavailable {
		t.Fatalf("failure=%+v", outcome.Failures[0])
	}
}

func TestSecretSemanticsKeepSetAndClear(t *testing.T) {
	cases := []struct {
		name     string
		current  string
		provided string
		action   string
		want     string
		changed  bool
		wantErr  bool
	}{
		{"blank keeps", "old", "", "", "old", false, false},
		{"blank keeps with explicit action", "old", "", SecretKeep, "old", false, false},
		{"blank create stays empty", "", "", SecretKeep, "", false, false},
		{"value sets on keep", "old", "new", SecretKeep, "new", true, false},
		{"set requires value", "old", "", SecretSet, "", false, true},
		{"clear erases", "old", "", SecretClear, "", true, false},
		{"clear rejects value", "old", "new", SecretClear, "", false, true},
		{"unknown action", "old", "", "reset", "", false, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			value, changed, err := resolveSecret(test.current, test.provided, test.action)
			if (err != nil) != test.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, test.wantErr)
			}
			if value != test.want || changed != test.changed {
				t.Fatalf("value=%q changed=%v, want %q %v", value, changed, test.want, test.changed)
			}
		})
	}
}

func TestNormalizeCameraInputTracksOnlyContentChanges(t *testing.T) {
	current := &cameraRecord{
		ID: 1, PublicID: "cam_a", Name: "入口", SourceURL: "rtsp://camera/1",
		Username: "operator", Password: "secret", CaptureEnabled: true, Enabled: true, Version: 3,
	}
	same := CameraInput{Name: "新名称", SourceURL: "rtsp://camera/1", Version: 3}
	record, contentChanged, streamChanged, err := normalizeCameraInput(current, same)
	if err != nil {
		t.Fatal(err)
	}
	if contentChanged || streamChanged || record.Password != "secret" {
		t.Fatalf("rename changed content=%v stream=%v record=%+v", contentChanged, streamChanged, record)
	}
	clear := CameraInput{Name: "入口", SourceURL: "rtsp://camera/1", PasswordAction: SecretClear, Version: 3}
	record, contentChanged, streamChanged, err = normalizeCameraInput(current, clear)
	if err != nil {
		t.Fatal(err)
	}
	if !contentChanged || !streamChanged || record.Password != "" {
		t.Fatalf("clear content=%v stream=%v password=%q", contentChanged, streamChanged, record.Password)
	}
	captureOnly := CameraInput{Name: "入口", SourceURL: "rtsp://camera/1", CaptureEnabled: boolPointer(false), Version: 3}
	_, contentChanged, streamChanged, err = normalizeCameraInput(current, captureOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !contentChanged || streamChanged {
		t.Fatalf("capture flag content=%v stream=%v", contentChanged, streamChanged)
	}
}

func TestTestCaptureNeverRegistersImageOrSample(t *testing.T) {
	data := testJPEG(t)
	store := &fakeStore{cameras: []cameraRecord{{
		ID: 1, PublicID: "cam_a", Name: "入口", SourceURL: "rtsp://camera/1", Enabled: true, CaptureEnabled: true,
	}}}
	service := NewService(store, t.TempDir(), frameFunc(func(context.Context, string, string, string) (Frame, error) {
		return Frame{Data: data, Width: 4, Height: 2, Format: "jpeg"}, nil
	}), nil, &fakeControl{})
	result, err := service.Test(context.Background(), 1, audit.Event{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || len(result.JPEG) == 0 || result.Width != 4 || result.Height != 2 {
		t.Fatalf("result=%+v", result)
	}
	if len(store.saved) != 0 {
		t.Fatalf("diagnostic test persisted evidence: %+v", store.saved)
	}
	if len(store.audits) != 1 || store.audits[0].Action != "camera.test" {
		t.Fatalf("audits=%+v", store.audits)
	}
}

func TestSyncRejectsLateVersionAndSyncsOnlyCurrentPath(t *testing.T) {
	store := &fakeStore{
		settings: settingsRecord{
			ConfigVersion: 5, VideoVersion: 9, CaptureTimeoutMS: 6000,
			MediaMTXAPIURL: "http://media.example:9997", MediaMTXAPIUsername: "operator", MediaMTXAPIPassword: "secret",
			MediaMTXWHEPBaseURL: "https://media.example:8889",
		},
		cameras: []cameraRecord{{
			ID: 1, PublicID: "cam_a", Name: "入口", SourceURL: "rtsp://camera/1",
			Username: "camera-user", Password: "camera-secret", Enabled: true, CaptureEnabled: true, Version: 4,
		}},
		streams: []streamRecord{{CameraID: 1, MediaPath: "wb-cam_a", DesiredVersion: 4, State: StreamPending, UpdatedAt: time.Now()}},
	}
	control := &fakeControl{}
	service := NewService(store, t.TempDir(), nil, nil, control)
	if _, err := service.Sync(context.Background(), SyncRequest{ConfigVersion: 4}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale config version error=%v", err)
	}
	if _, err := service.Sync(context.Background(), SyncRequest{VideoVersion: 8}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale video version error=%v", err)
	}
	result, err := service.Sync(context.Background(), SyncRequest{ConfigVersion: 5, VideoVersion: 9})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != VideoSyncSynced || len(result.Streams) != 1 {
		t.Fatalf("result=%+v", result)
	}
	if len(control.calls) != 2 || control.calls[0].Method != "get" || control.calls[1].Method != "add" {
		t.Fatalf("control calls=%+v", control.calls)
	}
	if !strings.Contains(control.calls[1].Source, "camera-user") || !strings.Contains(control.calls[1].Source, "camera-secret") {
		t.Fatalf("managed path must carry source credentials for MediaMTX: %q", control.calls[1].Source)
	}
	if len(store.markSynced) != 1 || store.markSynced[0] != 4 || store.videoState != VideoSyncSynced {
		t.Fatalf("marks=%+v video=%s", store.markSynced, store.videoState)
	}
	if result.Streams[0].PlaybackURL != "https://media.example:8889/wb-cam_a/whep" {
		t.Fatalf("playback=%q", result.Streams[0].PlaybackURL)
	}
}

func TestSyncRemovesDisabledCameraPath(t *testing.T) {
	store := &fakeStore{
		settings: settingsRecord{ConfigVersion: 2, VideoVersion: 3, MediaMTXAPIURL: "http://media.example:9997", MediaMTXWHEPBaseURL: "https://media.example:8889"},
		cameras:  []cameraRecord{{ID: 1, PublicID: "cam_a", Name: "入口", SourceURL: "rtsp://camera/1", Enabled: false, Version: 4}},
		streams:  []streamRecord{{CameraID: 1, MediaPath: "wb-cam_a", DesiredVersion: 4, State: StreamRemoving, UpdatedAt: time.Now()}},
	}
	control := &fakeControl{exists: map[string]bool{"wb-cam_a": true}}
	service := NewService(store, t.TempDir(), nil, nil, control)
	result, err := service.Sync(context.Background(), SyncRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != VideoSyncSynced || len(store.markRemoved) != 1 {
		t.Fatalf("result=%+v removed=%v", result, store.markRemoved)
	}
	if len(control.calls) != 2 || control.calls[1].Method != "delete" {
		t.Fatalf("calls=%+v", control.calls)
	}
}

func boolPointer(value bool) *bool { return &value }
