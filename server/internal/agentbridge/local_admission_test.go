package agentbridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func keylessServer(t *testing.T, resolver func(context.Context) (bridge.Config, error)) (*bridge.Hub, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	hub := bridge.New(bridge.Config{ID: "agent-01", SiteID: 1, ScaleID: 1, ImageDir: dir})
	hub.SetLocalResolver(resolver)
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return hub, server
}

func dialKeyless(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	cfg, err := websocket.NewConfig(strings.Replace(server.URL, "http:", "ws:", 1)+"/api/agent/ws", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := cfg.DialContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func receiveMessage(t *testing.T, conn *websocket.Conn) p.Message {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var message p.Message
	if err := websocket.JSON.Receive(conn, &message); err != nil {
		t.Fatal(err)
	}
	return message
}

func TestKeylessAdmissionHelloCapabilityAndConflict(t *testing.T) {
	hub, server := keylessServer(t, func(context.Context) (bridge.Config, error) {
		return bridge.Config{ID: "agent-01", SiteID: 1, ScaleID: 1}, nil
	})
	conn := dialKeyless(t, server)
	hello := p.Hello{ProtocolVersion: p.ProtocolVersion, AgentVersion: "test", Hostname: "host", Capabilities: append([]string{}, p.FormalCaptureCapabilities...)}
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeHello, bridge.ID(), "hello-request", hello)); err != nil {
		t.Fatal(err)
	}
	ackMessage := receiveMessage(t, conn)
	if ackMessage.Type != p.TypeHelloAck || ackMessage.RequestID != "hello-request" {
		t.Fatalf("unexpected hello ack: %+v", ackMessage)
	}
	var ack p.HelloAck
	if err := json.Unmarshal(ackMessage.Payload, &ack); err != nil {
		t.Fatal(err)
	}
	if ack.ProtocolVersion != p.ProtocolVersion || ack.SiteID != 1 || ack.ScaleID != 1 || ack.AgentID != "agent-01" {
		t.Fatalf("hello ack identity mismatch: %+v", ack)
	}
	if !hub.Supports(1, p.CapabilityMeasurementWindow) {
		t.Fatal("advertised measurement capability was not recorded")
	}
	if err := hub.RequireCapability(1, "missing.capability"); !errors.Is(err, bridge.ErrCapabilityMissing) {
		t.Fatalf("missing capability error=%v", err)
	}
	if err := hub.RequireCapability(1, p.CapabilityMeasurementWindow); err != nil {
		t.Fatalf("known capability rejected: %v", err)
	}

	incumbent := hub.State(1)
	replacement := dialKeyless(t, server)
	conflict := receiveMessage(t, replacement)
	if conflict.Type != "error" || !bytes.Contains(conflict.Payload, []byte(p.ErrorAgentConflict)) {
		t.Fatalf("duplicate connection: %+v", conflict)
	}
	current := hub.State(1)
	if current.ConnectionID != incumbent.ConnectionID || current.ConnectionID == "" {
		t.Fatal("conflicting connection displaced the incumbent")
	}

	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("view", "front")
	file, _ := form.CreateFormFile("file", "front.png")
	_, _ = io.Copy(file, &pixels)
	_ = form.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/agent/images", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Code int
		Data p.Image
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || envelope.Code != 200 || envelope.Data.ID == "" || envelope.Data.SHA256 == "" {
		t.Fatalf("keyless image upload: %+v %v", envelope, err)
	}
}

func TestKeylessAdmissionReportsUninitializedSite(t *testing.T) {
	_, server := keylessServer(t, func(context.Context) (bridge.Config, error) {
		return bridge.Config{}, bridge.ErrSiteNotInitialized
	})
	conn := dialKeyless(t, server)
	message := receiveMessage(t, conn)
	if message.Type != "error" || !bytes.Contains(message.Payload, []byte(p.ErrorSiteNotInitialized)) {
		t.Fatalf("uninitialized admission: %+v", message)
	}
}

func TestHelloRejectsUnsupportedProtocolVersion(t *testing.T) {
	hub, server := keylessServer(t, func(context.Context) (bridge.Config, error) {
		return bridge.Config{ID: "agent-01", SiteID: 1, ScaleID: 1}, nil
	})
	_ = hub
	conn := dialKeyless(t, server)
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeHello, bridge.ID(), "hello-request", p.Hello{ProtocolVersion: 99})); err != nil {
		t.Fatal(err)
	}
	message := receiveMessage(t, conn)
	if message.Type != "error" || !bytes.Contains(message.Payload, []byte(p.ErrorAgentProtocolUnsupported)) {
		t.Fatalf("unsupported protocol: %+v", message)
	}
}

func TestMeasurementCommandCorrelatesRequestAndConnection(t *testing.T) {
	hub, server := keylessServer(t, func(context.Context) (bridge.Config, error) {
		return bridge.Config{ID: "agent-01", SiteID: 1, ScaleID: 1}, nil
	})
	conn := dialKeyless(t, server)
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeHello, bridge.ID(), "hello-request", p.Hello{ProtocolVersion: p.ProtocolVersion, Capabilities: append([]string{}, p.FormalCaptureCapabilities...)})); err != nil {
		t.Fatal(err)
	}
	if ack := receiveMessage(t, conn); ack.Type != p.TypeHelloAck {
		t.Fatalf("hello ack missing: %+v", ack)
	}
	result := make(chan p.Message, 1)
	commandErr := make(chan error, 1)
	go func() {
		message, err := hub.Command(context.Background(), 1, p.TypeMeasurementBeginRequest, p.MeasurementBeginRequest{CaptureID: "capture-1"}, 2*time.Second)
		if err != nil {
			commandErr <- err
			return
		}
		result <- message
	}()
	command := receiveMessage(t, conn)
	if command.Type != p.TypeMeasurementBeginRequest || command.RequestID == "" || command.Version != p.ProtocolVersion {
		t.Fatalf("measurement begin command: %+v", command)
	}
	reply := p.MeasurementBeginResult{Accepted: true, Generation: "gen-1", State: p.MeasuredState{DeviceState: p.DeviceState{Weight: 30000, Unit: "t", Stable: true, DeviceOK: true}, At: time.Now().UTC()}}
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeMeasurementBeginResult, bridge.ID(), command.RequestID, reply)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-commandErr:
		t.Fatalf("command failed: %v", err)
	case message := <-result:
		if message.Type != p.TypeMeasurementBeginResult || message.RequestID != command.RequestID {
			t.Fatalf("correlated result: %+v", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("measurement command did not complete")
	}
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeMeasurementInvalidated, bridge.ID(), "", p.MeasurementInvalidated{Generation: "gen-1", Reason: "weight_changed", State: p.MeasuredState{At: time.Now().UTC()}, At: time.Now().UTC()})); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if invalidated, ok := hub.LastMeasurementInvalidation(1); ok && invalidated.Generation == "gen-1" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("measurement invalidation was not recorded")
}

func TestEffectiveConfigAndBackupAreRecorded(t *testing.T) {
	hub, server := keylessServer(t, func(context.Context) (bridge.Config, error) {
		return bridge.Config{ID: "agent-01", SiteID: 1, ScaleID: 1}, nil
	})
	conn := dialKeyless(t, server)
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeHello, bridge.ID(), "hello-request", p.Hello{ProtocolVersion: p.ProtocolVersion, Capabilities: append([]string{}, p.FormalCaptureCapabilities...)})); err != nil {
		t.Fatal(err)
	}
	_ = receiveMessage(t, conn)
	settings := p.DeviceSettings{
		Serial: p.SerialSettings{Port: "/dev/ttyS0", Baud: 9600},
		Print:  p.PrintSettings{Queue: "Aisino-SK820-raw", FormLengthMM: 99, PrintWidthMM: 175, PrintScale: 1},
	}
	effective := p.EffectiveConfig{ConfigVersion: 3, Digest: "abc123", Serial: settings.Serial, Print: settings.Print, AppliedAt: time.Now().UTC(), Running: true, DeviceState: "running"}
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeAgentConfigState, bridge.ID(), "", effective)); err != nil {
		t.Fatal(err)
	}
	backup := p.ConfigBackup{ConfigVersion: 3, Digest: "abc123", Settings: settings, AppliedAt: time.Now().UTC()}
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeAgentConfigBackup, bridge.ID(), "", backup)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gotEffective, okEffective := hub.EffectiveConfig(1)
		gotBackup, okBackup := hub.ConfigBackup(1)
		if okEffective && okBackup {
			if gotEffective.ConfigVersion != 3 || gotEffective.Print.Queue != "Aisino-SK820-raw" || gotBackup.Digest != "abc123" || gotBackup.Settings.Serial.Baud != 9600 {
				t.Fatalf("recorded config mismatch: effective=%+v backup=%+v", gotEffective, gotBackup)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("effective config or backup report was not recorded")
}
