//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/camera"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
)

// captureFrameControl is a deterministic Server-side frame source. Tests use it
// to exercise parallel success, per-camera failure and temporary unavailability
// without a real RTSP endpoint or ffmpeg.
type captureFrameControl struct {
	mu      sync.Mutex
	data    []byte
	failing map[string]bool
	allFail bool
}

func (c *captureFrameControl) Snapshot(_ context.Context, sourceURL, _, _ string) (camera.Frame, error) {
	c.mu.Lock()
	all, failed := c.allFail, c.failing[sourceURL]
	data := c.data
	c.mu.Unlock()
	if all || failed {
		return camera.Frame{}, errors.New("camera unavailable")
	}
	return camera.Frame{Data: data, Width: 4, Height: 2, Format: "jpeg"}, nil
}

func (c *captureFrameControl) failAll(fail bool) {
	c.mu.Lock()
	c.allFail = fail
	c.mu.Unlock()
}

func (c *captureFrameControl) failSource(source string, fail bool) {
	c.mu.Lock()
	if c.failing == nil {
		c.failing = map[string]bool{}
	}
	c.failing[source] = fail
	c.mu.Unlock()
}

type captureRecognizer struct {
	mu        sync.Mutex
	candidate string
	err       error
}

func (r *captureRecognizer) Recognize(context.Context, string, []byte) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.candidate, r.err
}

func (r *captureRecognizer) set(candidate string, err error) {
	r.mu.Lock()
	r.candidate, r.err = candidate, err
	r.mu.Unlock()
}

func testCaptureJPEG(t *testing.T) []byte {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			source.Set(x, y, color.RGBA{R: 30, G: 90, B: 150, A: 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, source, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// newCaptureService builds the Server camera module used by formal capture.
func newCaptureService(t *testing.T, db *platformdatabase.Database, imageDir string) (*camera.Service, *captureFrameControl, *captureRecognizer) {
	t.Helper()
	frames := &captureFrameControl{data: testCaptureJPEG(t)}
	recognizer := &captureRecognizer{}
	return camera.NewService(camera.NewRepository(db.GORM), imageDir, frames, recognizer, nil), frames, recognizer
}

// addCaptureCameras inserts enabled capture cameras directly so the frozen
// plan sees them without depending on the camera management HTTP routes.
func addCaptureCameras(t *testing.T, db *platformdatabase.Database, sources ...string) {
	t.Helper()
	for index, source := range sources {
		name := fmt.Sprintf("测试抓拍相机%d", index+1)
		publicID := fmt.Sprintf("cam_test_%d_%d", index+1, len(sources))
		if err := db.GORM.Exec("INSERT INTO wb_camera (public_id,name,source_url,capture_enabled,enabled) VALUES (?,?,?,1,1) ON CONFLICT (public_id) DO NOTHING", publicID, name, source).Error; err != nil {
			t.Fatalf("insert capture camera: %v", err)
		}
	}
}

func disableCaptureCameras(t *testing.T, db *platformdatabase.Database) {
	t.Helper()
	if err := db.GORM.Exec("UPDATE wb_camera SET capture_enabled=0 WHERE deleted=0").Error; err != nil {
		t.Fatalf("disable capture cameras: %v", err)
	}
}
