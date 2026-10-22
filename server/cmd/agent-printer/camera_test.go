package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/agentscale"
	"github.com/gin-gonic/gin"
)

func TestCapturePersistsBothViewsAndRejectsWeightChange(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		t.Fatal(err)
	}
	for _, unstable := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable", true: "weight-changes-during-camera-read"}[unstable], func(t *testing.T) {
			scale, writer, stop := startTestScale(t)
			defer stop()
			if _, err := writer.Write(makeD12Frame(0x30, "001550")); err != nil {
				t.Fatal(err)
			}
			waitScale(t, scale, func(s agentscale.Snapshot) bool { return s.DeviceOK && s.Stable })
			camera := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if unstable {
					_, _ = writer.Write(makeD12Frame(0x38, "001560"))
					select {
					case <-r.Context().Done():
						return
					case <-time.After(300 * time.Millisecond):
					}
				}
				_, _ = w.Write(data.Bytes())
			}))
			defer camera.Close()
			hub := agentbridge.New(agentbridge.Config{ID: "real-camera-test", Key: "key", ScaleID: 1, ImageDir: t.TempDir()})
			router := gin.New()
			hub.Register(router)
			server := httptest.NewServer(router)
			defer server.Close()
			cfg := agentprinter.Defaults()
			cfg.AgentID, cfg.AgentKey, cfg.ServerURL = "real-camera-test", "key", server.URL
			cfg.CameraFrontURL, cfg.CameraRearURL = camera.URL, camera.URL
			result := captureEvidence(context.Background(), cfg, scale)
			if unstable {
				if result.Success || len(result.Images) != 0 || result.Error == "" {
					t.Fatalf("unstable evidence accepted: %+v", result)
				}
				return
			}
			if !result.Success || result.Weight.String() != "1.55" || len(result.Images) != 2 || result.CapturedAt.IsZero() || !result.Plate.Failed {
				t.Fatalf("stable evidence failed: %+v", result)
			}
			for _, ref := range result.Images {
				if _, err := hub.Image(ref.ID); err != nil {
					t.Fatal("evidence not durably uploaded:", err)
				}
			}
		})
	}
}
