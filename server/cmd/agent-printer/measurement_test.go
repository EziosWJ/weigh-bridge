package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/agentscale"
	"github.com/gin-gonic/gin"
)

func commandMeasurement(t *testing.T, hub *agentbridge.Hub, kind string, payload any) p.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	message, err := hub.Command(ctx, "WB01", kind, payload, 2*time.Second)
	if err != nil {
		t.Fatalf("%s: %v", kind, err)
	}
	return message
}

func TestMeasurementWindowOverKeylessAgentConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := agentbridge.New(agentbridge.Config{ID: "agent-01", SiteCode: "F001", ScaleCode: "WB01", ImageDir: t.TempDir()})
	hub.SetLocalResolver(func(context.Context) (agentbridge.Config, error) {
		return agentbridge.Config{ID: "agent-01", SiteCode: "F001", ScaleCode: "WB01", InstallationID: "00000000-0000-4000-8000-000000000001", ImageDir: t.TempDir()}, nil
	})
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()

	scale, writer, stop := startTestScale(t)
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	done := make(chan error, 1)
	go func() {
		done <- runManaged(ctx, agentprinter.Config{ServerURL: server.URL, Queue: "unused"}, scale, dir, filepath.Join(dir, "status.json"), filepath.Join(dir, "pause"), nil, dir)
	}()
	defer func() {
		cancel()
		_ = writer.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if hub.State("WB01").Online && hub.Supports("WB01", p.CapabilityMeasurementWindow) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("keyless Agent did not complete hello: %+v", hub.State("WB01"))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if info, err := os.Stat(filepath.Join(dir, "installation-00000000-0000-4000-8000-000000000001")); err != nil || !info.IsDir() {
		t.Fatalf("Agent did not bind the server installation: %v", err)
	}
	if _, err := writer.Write(makeD12Frame(0x30, "001000")); err != nil {
		t.Fatal(err)
	}
	waitScale(t, scale, func(s agentscale.Snapshot) bool { return s.DeviceOK && s.Stable && s.WeightMilliTonnes == 1000 })

	beginMessage := commandMeasurement(t, hub, p.TypeMeasurementBeginRequest, p.MeasurementBeginRequest{CaptureID: "capture-1"})
	var begin p.MeasurementBeginResult
	if err := json.Unmarshal(beginMessage.Payload, &begin); err != nil || !begin.Accepted || begin.Generation == "" {
		t.Fatalf("measurement begin: %+v %v", begin, err)
	}

	// Stable 1.000 -> 1.010 -> 1.000 in one serial read batch. The value is
	// back to the original but the window must already be invalid.
	if _, err := writer.Write(append(makeD12Frame(0x38, "001010"), makeD12Frame(0x30, "001000")...)); err != nil {
		t.Fatal(err)
	}
	waitScale(t, scale, func(s agentscale.Snapshot) bool { return s.FramesValid >= 3 && s.Stable && s.WeightMilliTonnes == 1000 })

	endMessage := commandMeasurement(t, hub, p.TypeMeasurementEndRequest, p.MeasurementEndRequest{Generation: begin.Generation})
	var end p.MeasurementEndResult
	if err := json.Unmarshal(endMessage.Payload, &end); err != nil || end.Verified || end.Code == "" {
		t.Fatalf("same-batch change was accepted: %+v %v", end, err)
	}

	secondMessage := commandMeasurement(t, hub, p.TypeMeasurementBeginRequest, p.MeasurementBeginRequest{CaptureID: "capture-2"})
	var second p.MeasurementBeginResult
	if err := json.Unmarshal(secondMessage.Payload, &second); err != nil || !second.Accepted || second.Generation == "" {
		t.Fatalf("fresh stable window rejected: %+v %v", second, err)
	}
	endMessage = commandMeasurement(t, hub, p.TypeMeasurementEndRequest, p.MeasurementEndRequest{Generation: second.Generation})
	if err := json.Unmarshal(endMessage.Payload, &end); err != nil || !end.Verified {
		t.Fatalf("fresh window end: %+v %v", end, err)
	}
	verifyMessage := commandMeasurement(t, hub, p.TypeMeasurementVerifyRequest, p.MeasurementVerifyRequest{Generation: second.Generation})
	var verify p.MeasurementVerifyResult
	if err := json.Unmarshal(verifyMessage.Payload, &verify); err != nil || !verify.Valid {
		t.Fatalf("fresh window confirmation: %+v %v", verify, err)
	}
}
