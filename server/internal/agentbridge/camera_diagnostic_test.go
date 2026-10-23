package agentbridge_test

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bridge "github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

func TestCameraDiagnosticWorksWithoutScaleAndDoesNotBecomeEvidence(t *testing.T) {
	hub := bridge.New(bridge.Config{ID: "agent", ScaleID: 1, Key: "key", ImageDir: t.TempDir()})
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	cfg, _ := websocket.NewConfig(strings.Replace(server.URL, "http:", "ws:", 1)+"/api/agent/ws", server.URL)
	cfg.Header.Set("Authorization", "Bearer key")
	cfg.Header.Set("X-Agent-ID", "agent")
	cfg.Header.Set("X-Scale-ID", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := cfg.DialContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// A heartbeat round trip establishes admission without a stable weight.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err = websocket.JSON.Send(conn, p.New("heartbeat", bridge.ID(), "", struct{}{})); err != nil {
		t.Fatal(err)
	}
	var heartbeat p.Message
	if err = websocket.JSON.Receive(conn, &heartbeat); err != nil || heartbeat.Type != "heartbeat" {
		t.Fatal("heartbeat admission failed", err)
	}
	result := make(chan error, 1)
	go func() {
		diagnostic, e := hub.TestCameras(ctx, 1)
		if e == nil && (len(diagnostic.Views) != 2 || !diagnostic.Views[0].Success) {
			e = bridge.ErrUnavailable
		}
		result <- e
	}()
	var request p.Message
	if err = websocket.JSON.Receive(conn, &request); err != nil || request.Type != "camera.test.request" {
		t.Fatalf("unexpected diagnostic request: %s %v", request.Type, err)
	}
	var pixels bytes.Buffer
	_ = jpeg.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil)
	response := p.CameraTest{At: time.Now().UTC(), Views: []p.CameraTestView{{View: "front", Success: true, JPEG: pixels.Bytes()}, {View: "rear", Success: true, JPEG: pixels.Bytes()}}}
	if err = websocket.JSON.Send(conn, p.New("camera.test.result", bridge.ID(), request.RequestID, response)); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("diagnostic timed out")
	}
	if len(hub.State(1).Images) != 0 {
		t.Fatal("diagnostic thumbnails became formal evidence")
	}
	if _, _, err = hub.Capture(ctx, 1); err == nil {
		t.Fatal("diagnostic bypassed formal instrument conditions")
	}
}
