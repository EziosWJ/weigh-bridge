package agentcamera

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

func TestFreshCameraEvidenceAndRecognition(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var captures, uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/camera":
			if r.Header.Get("Cache-Control") == "" {
				t.Error("missing no-cache request")
			}
			captures.Add(1)
			_, _ = w.Write(imageData.Bytes())
		case "/recognize":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			defer r.MultipartForm.RemoveAll()
			_, _ = w.Write([]byte(`{"candidate":"川A12345"}`))
		case "/api/agent/images":
			if r.Header.Get("Authorization") != "Bearer private-key" || r.Header.Get("X-Agent-ID") != "agent-1" || r.Header.Get("X-Scale-Code") != "WB21" {
				t.Error("Agent identity not forwarded")
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			defer r.MultipartForm.RemoveAll()
			view := r.FormValue("view")
			sum := sha256.Sum256(imageData.Bytes())
			uploads.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": p.Image{ID: "fresh-" + view, View: view, SHA256: hex.EncodeToString(sum[:])}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := agentprinter.Defaults()
	cfg.ServerURL, cfg.AgentID, cfg.AgentKey, cfg.ScaleCode = server.URL, "agent-1", "private-key", "WB21"
	cfg.CameraFrontURL, cfg.CameraRearURL, cfg.PlateRecognitionURL = server.URL+"/camera", server.URL+"/camera", server.URL+"/recognize"
	client := Client{Config: cfg}
	for range 2 {
		images, plate, err := client.Capture(context.Background())
		if err != nil || len(images) != 2 || images[0].View != "front" || images[1].View != "rear" || plate.Failed || plate.Candidate != "川A12345" {
			t.Fatalf("capture = %#v %#v %v", images, plate, err)
		}
	}
	if captures.Load() != 4 || uploads.Load() != 4 {
		t.Fatal("each capture must acquire and persist both new images")
	}
}

func TestCameraFailureNeverReturnsPartialOrCachedEvidence(t *testing.T) {
	for _, mode := range []string{"invalid", "unauthorized", "timeout", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "invalid":
					_, _ = w.Write([]byte("not an image"))
				case "unauthorized":
					http.Error(w, "secret-device-message", http.StatusUnauthorized)
				case "timeout":
					<-r.Context().Done()
				case "redirect":
					http.Redirect(w, r, "http://user:secret@127.0.0.1:1", http.StatusFound)
				}
			}))
			defer server.Close()
			cfg := agentprinter.Defaults()
			cfg.CameraFrontURL, cfg.CameraRearURL, cfg.CameraTimeoutMS = server.URL, server.URL, 200
			start := time.Now()
			images, _, err := (Client{Config: cfg}).Capture(context.Background())
			if err == nil || len(images) != 0 || strings.Contains(err.Error(), "secret") {
				t.Fatalf("failure leaked evidence/credentials: %v %v", images, err)
			}
			if time.Since(start) > time.Second {
				t.Fatal("camera timeout not enforced")
			}
		})
	}
}

func TestMissingCameraConfigurationFails(t *testing.T) {
	if images, _, err := (Client{Config: agentprinter.Defaults()}).Capture(context.Background()); err == nil || len(images) != 0 {
		t.Fatal("missing cameras accepted")
	}
}

// Diagnostic frames never use the image upload or recognition endpoints.
func TestCameraDiagnosticDoesNotUploadEvidence(t *testing.T) {
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 640, 480))); err != nil {
		t.Fatal(err)
	}
	uploads := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			uploads++
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pixels.Bytes())
	}))
	defer source.Close()
	cfg := agentprinter.Defaults()
	cfg.CameraFrontURL = source.URL + "/front"
	cfg.CameraRearURL = source.URL + "/rear"
	cfg.ServerURL = source.URL
	cfg.PlateRecognitionURL = source.URL + "/recognize"
	result := (Client{Config: cfg}).Test(context.Background())
	if len(result.Views) != 2 || uploads != 0 {
		t.Fatal("diagnostic used evidence upload or recognition")
	}
	for _, view := range result.Views {
		info, format, err := image.DecodeConfig(bytes.NewReader(view.JPEG))
		if !view.Success || err != nil || format != "jpeg" || info.Width > 320 || info.Height > 180 {
			t.Fatalf("invalid diagnostic %s", view.View)
		}
	}
}
