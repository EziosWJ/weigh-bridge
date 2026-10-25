package agentmock_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/gin-gonic/gin"
)

func mockControl(t *testing.T, server *httptest.Server, body any) {
	t.Helper()
	data, _ := json.Marshal(body)
	resp, err := http.Post(server.URL+"/api/control", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("control %v: %d", body, resp.StatusCode)
	}
}

func waitMockState(t *testing.T, hub *agentbridge.Hub, weight p.Weight) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state := hub.State(1)
		if state.Online && state.Stable && state.Weight == weight {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("mock state weight=%s: %+v", weight, hub.State(1))
}

func TestMockMeasurementLifecycleAndValueChangeLatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := agentbridge.New(agentbridge.Config{ID: "mock-01", SiteID: 1, ScaleID: 1, ImageDir: t.TempDir()})
	hub.SetLocalResolver(func(context.Context) (agentbridge.Config, error) {
		return agentbridge.Config{ID: "mock-01", SiteID: 1, ScaleID: 1, ImageDir: t.TempDir()}, nil
	})
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mock := agentmock.New(agentmock.Config{Server: server.URL})
	go mock.Run(ctx)
	mockServer := httptest.NewServer(mock.Handler())
	defer mockServer.Close()

	deadline := time.Now().Add(3 * time.Second)
	for !(hub.State(1).Online && hub.Supports(1, p.CapabilityMeasurementWindow)) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !hub.Supports(1, p.CapabilityMeasurementWindow) {
		t.Fatalf("mock did not advertise measurement capability: %+v", hub.State(1))
	}

	mockControl(t, mockServer, map[string]any{"action": "up", "weight": "1.000"})
	mockControl(t, mockServer, map[string]any{"action": "stable"})
	waitMockState(t, hub, 1000)

	command := func(kind string, payload any) p.Message {
		t.Helper()
		ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		message, err := hub.Command(ctx, 1, kind, payload, 2*time.Second)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		return message
	}
	message := command(p.TypeMeasurementBeginRequest, p.MeasurementBeginRequest{CaptureID: "mock-1"})
	var begin p.MeasurementBeginResult
	if err := json.Unmarshal(message.Payload, &begin); err != nil || !begin.Accepted || begin.Generation == "" {
		t.Fatalf("mock begin: %+v %v", begin, err)
	}

	// 1.000 -> 1.010 -> 1.000 with a tick on the intermediate value.
	mockControl(t, mockServer, map[string]any{"action": "up", "weight": "1.010"})
	mockControl(t, mockServer, map[string]any{"action": "stable"})
	waitMockState(t, hub, 1010)
	mockControl(t, mockServer, map[string]any{"action": "up", "weight": "1.000"})
	mockControl(t, mockServer, map[string]any{"action": "stable"})
	waitMockState(t, hub, 1000)

	message = command(p.TypeMeasurementEndRequest, p.MeasurementEndRequest{Generation: begin.Generation})
	var end p.MeasurementEndResult
	if err := json.Unmarshal(message.Payload, &end); err != nil || end.Verified || end.Code == "" {
		t.Fatalf("mock same-value restore was accepted: %+v %v", end, err)
	}

	message = command(p.TypeMeasurementBeginRequest, p.MeasurementBeginRequest{CaptureID: "mock-2"})
	if err := json.Unmarshal(message.Payload, &begin); err != nil || !begin.Accepted {
		t.Fatalf("mock fresh begin: %+v %v", begin, err)
	}
	message = command(p.TypeMeasurementEndRequest, p.MeasurementEndRequest{Generation: begin.Generation})
	if err := json.Unmarshal(message.Payload, &end); err != nil || !end.Verified {
		t.Fatalf("mock fresh end: %+v %v", end, err)
	}
	message = command(p.TypeMeasurementVerifyRequest, p.MeasurementVerifyRequest{Generation: begin.Generation})
	var verify p.MeasurementVerifyResult
	if err := json.Unmarshal(message.Payload, &verify); err != nil || !verify.Valid {
		t.Fatalf("mock verify: %+v %v", verify, err)
	}
	message = command(p.TypeMeasurementCancelRequest, p.MeasurementCancelRequest{Generation: begin.Generation, Reason: "test"})
	var cancelled p.MeasurementCancelResult
	if err := json.Unmarshal(message.Payload, &cancelled); err != nil || !cancelled.Cancelled {
		t.Fatalf("mock cancel: %+v %v", cancelled, err)
	}
}
