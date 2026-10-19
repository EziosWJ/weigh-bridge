package agentbridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bridge "github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

func TestAgentProtocolLifecycle(t *testing.T) {
	hub := bridge.New(bridge.Config{ID: "agent-1", ScaleID: 1, Key: "test-key", ImageDir: t.TempDir()})
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL+"/api/agent/ws", nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("unauthenticated connection=%d", response.StatusCode)
	}
	dial := func() *websocket.Conn {
		cfg, _ := websocket.NewConfig(strings.Replace(server.URL, "http:", "ws:", 1)+"/api/agent/ws", server.URL)
		cfg.Header.Set("Authorization", "Bearer test-key")
		cfg.Header.Set("X-Agent-ID", "agent-1")
		cfg.Header.Set("X-Scale-ID", "1")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn, err := cfg.DialContext(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	conn := dial()
	defer conn.Close()
	send := func(kind, request string, value any) {
		t.Helper()
		if err := websocket.JSON.Send(conn, p.New(kind, bridge.ID(), request, value)); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(check func(bridge.State) bool) {
		t.Helper()
		limit := time.Now().Add(2 * time.Second)
		for time.Now().Before(limit) {
			if check(hub.State(1)) {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("state timeout: %+v", hub.State(1))
	}
	images := []p.Image{}
	for _, view := range []string{"front", "rear"} {
		var pixels bytes.Buffer
		if err := png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
			t.Fatal(err)
		}
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		_ = form.WriteField("view", view)
		file, _ := form.CreateFormFile("file", view+".png")
		_, _ = io.Copy(file, &pixels)
		_ = form.Close()
		req, _ := http.NewRequest("POST", server.URL+"/api/agent/images", &body)
		req.Header.Set("Authorization", "Bearer test-key")
		req.Header.Set("X-Agent-ID", "agent-1")
		req.Header.Set("X-Scale-ID", "1")
		req.Header.Set("Content-Type", form.FormDataContentType())
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Code int
			Data p.Image
		}
		err = json.NewDecoder(response.Body).Decode(&result)
		_ = response.Body.Close()
		if err != nil || result.Code != 200 {
			t.Fatalf("image upload: %+v %v", result, err)
		}
		images = append(images, result.Data)
	}
	send("camera.updated", "", p.Camera{Images: images})
	send("device.state", "", p.DeviceState{Weight: 29983, Unit: "t", Stable: false, DeviceOK: true})
	wait(func(s bridge.State) bool { return s.Online && s.Weight == 29983 && !s.Stable })
	if _, _, err := hub.Guard(1, 0); err == nil {
		t.Fatal("unstable device was accepted")
	}
	send("device.state", "", p.DeviceState{Weight: 30000, Unit: "t", Stable: true, DeviceOK: true})
	wait(func(s bridge.State) bool { return s.Stable })
	revision := hub.State(1).Revision
	// Unchanged stable samples leave the same basis valid.
	send("device.state", "", p.DeviceState{Weight: 30000, Unit: "t", Stable: true, DeviceOK: true})
	time.Sleep(10 * time.Millisecond)
	_, release, err := hub.Guard(1, revision)
	if err != nil {
		t.Fatal(err)
	}
	release()
	captured := make(chan error, 1)
	go func() {
		value, rev, err := hub.Capture(context.Background(), 1)
		if err == nil && (value.Weight != 30000 || rev != revision || len(value.Images) != 2 || value.Images[0].SHA256 == "") {
			err = io.ErrUnexpectedEOF
		}
		captured <- err
	}()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var command p.Message
	if err := websocket.JSON.Receive(conn, &command); err != nil || command.Type != "capture.request" {
		t.Fatalf("capture command: %+v %v", command, err)
	}
	result := p.Capture{Success: true, DeviceState: p.DeviceState{Weight: 30000, Unit: "t", Stable: true, DeviceOK: true}, CapturedAt: time.Now(), Images: images}
	send("capture.result", command.RequestID, result)
	if err := <-captured; err != nil {
		t.Fatal(err)
	}
	send("device.state", "", p.DeviceState{Weight: 30000, Unit: "t", Stable: false, DeviceOK: true})
	wait(func(s bridge.State) bool { return !s.Stable })
	send("device.state", "", p.DeviceState{Weight: 30000, Unit: "t", Stable: true, DeviceOK: true})
	wait(func(s bridge.State) bool { return s.Stable })
	if _, release, err := hub.Guard(1, revision); err == nil {
		release()
		t.Fatal("old basis became valid after instability")
	}
	// A delayed reply cannot satisfy a later capture request.
	send("capture.result", command.RequestID, result)
	results := make(chan p.PrintResult, 2)
	hub.SetPrintHandler(func(_ context.Context, _ int64, result p.PrintResult) error { results <- result; return nil })
	if err := hub.Print(1, 42, map[string]string{"ticketNo": "GB0001"}); err != nil {
		t.Fatal(err)
	}
	if err := websocket.JSON.Receive(conn, &command); err != nil || command.Type != "print.request" {
		t.Fatalf("print command: %+v %v", command, err)
	}
	send("print.result", command.RequestID, p.PrintResult{AttemptID: 42, Status: "printed"})
	select {
	case <-results:
	case <-time.After(time.Second):
		t.Fatal("print result missing")
	}
	send("print.result", command.RequestID, p.PrintResult{AttemptID: 42, Status: "printed"})
	select {
	case <-results:
		t.Fatal("duplicate print receipt processed twice")
	case <-time.After(20 * time.Millisecond):
	}
	if err := hub.Print(1, 43, map[string]string{"ticketNo": "GB0002"}); err != nil {
		t.Fatal(err)
	}
	if err := websocket.JSON.Receive(conn, &command); err != nil || command.Type != "print.request" {
		t.Fatalf("failure print command: %+v %v", command, err)
	}
	send("print.result", command.RequestID, p.PrintResult{AttemptID: 43, Status: "failed", Reason: "printer unavailable"})
	select {
	case result := <-results:
		if result.Status != "failed" {
			t.Fatalf("failed print result: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("failure receipt missing")
	}
	replacement := dial()
	defer replacement.Close()
	wait(func(s bridge.State) bool { return !s.Online })
	if _, release, err := hub.Guard(1, 0); err == nil {
		release()
		t.Fatal("replacement connection reused old telemetry")
	}
}
