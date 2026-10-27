package main

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/agentscale"
	"github.com/gin-gonic/gin"
)

func makeD12Frame(b byte, weight string) []byte {
	raw := make([]byte, 18)
	raw[0] = 0x02
	raw[1], raw[2], raw[3] = 0x2d, b, 0x20
	copy(raw[4:10], weight)
	copy(raw[10:16], "000000")
	raw[16] = 0x0d
	total := 0
	for _, v := range raw[:16] {
		total += int(v)
	}
	raw[17] = byte(total & 0xff)
	return raw
}

func startTestScale(t *testing.T) (*agentscale.Monitor, *io.PipeWriter, context.CancelFunc) {
	t.Helper()
	pr, pw := io.Pipe()
	m, err := agentscale.New(agentscale.Config{Port: "/dev/ttyS0", Baud: 9600}, func(context.Context) (io.ReadCloser, error) { return pr, nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go m.Run(ctx)
	return m, pw, func() { cancel(); _ = pw.Close(); _ = pr.Close() }
}

func waitScale(t *testing.T, m *agentscale.Monitor, cond func(agentscale.Snapshot) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond(m.Snapshot(time.Now())) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("scale state not reached: %+v", m.Snapshot(time.Now()))
}

func TestDeviceStateUsesFreshInstrumentStableBit(t *testing.T) {
	m, pw, stop := startTestScale(t)
	defer stop()
	if _, err := pw.Write(makeD12Frame(0x30, "001550")); err != nil {
		t.Fatal(err)
	}
	waitScale(t, m, func(s agentscale.Snapshot) bool { return s.DeviceOK && s.Stable })
	state := currentDeviceState(m, time.Now())
	if !state.DeviceOK || !state.Stable || state.Unit != "t" || state.Weight.String() != "1.55" || state.Error != "" {
		t.Fatalf("device state: %+v", state)
	}
	if _, err := pw.Write(makeD12Frame(0x38, "001560")); err != nil {
		t.Fatal(err)
	}
	waitScale(t, m, func(s agentscale.Snapshot) bool { return s.DeviceOK && !s.Stable })
	state = currentDeviceState(m, time.Now())
	if !state.DeviceOK || state.Stable || state.Weight.String() != "1.56" {
		t.Fatalf("dynamic device state: %+v", state)
	}
}

func TestUnavailableInstrumentNeverFabricatesWeight(t *testing.T) {
	m, pw, stop := startTestScale(t)
	defer stop()
	if _, err := pw.Write(makeD12Frame(0x32, "001550")); err != nil { // negative bit
		t.Fatal(err)
	}
	waitScale(t, m, func(s agentscale.Snapshot) bool { return !s.DeviceOK && s.Fault != "" })
	state := currentDeviceState(m, time.Now())
	if state.DeviceOK || state.Stable || state.Weight != 0 || state.Error == "" {
		t.Fatalf("negative state fabricated a usable weight: %+v", state)
	}
	result := captureFailure(m, time.Now())
	if result.Success || len(result.Images) != 0 || result.Error == "" || result.Weight != 0 {
		t.Fatalf("capture failure fabricated success: %+v", result)
	}
	if result.CapturedAt.IsZero() {
		t.Fatal("capture failure has no timestamp")
	}
}

func TestDisabledScaleIsPrintOnly(t *testing.T) {
	m, err := agentscale.New(agentscale.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := currentDeviceState(m, time.Now())
	if state.DeviceOK || state.Stable || state.Weight != 0 || state.Error == "" {
		t.Fatalf("disabled state: %+v", state)
	}
	if result := captureFailure(m, time.Now()); result.Success || result.Error == "" {
		t.Fatalf("disabled capture: %+v", result)
	}
}

func TestPrintPendingDoesNotBlockWeightReportingOrKick(t *testing.T) {
	hub := agentbridge.New(agentbridge.Config{ID: "printer", ScaleCode: "WB01", Key: "printer-test-key", ImageDir: t.TempDir()})
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	dir := t.TempDir()
	pr, pw := io.Pipe()
	scale, err := agentscale.New(agentscale.Config{Port: "/dev/ttyS0", Baud: 9600}, func(context.Context) (io.ReadCloser, error) { return pr, nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go scale.Run(ctx)
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, agentprinter.Config{ServerURL: server.URL, AgentID: "printer", ScaleCode: "WB01", AgentKey: "printer-test-key", Queue: "unused"}, scale, dir, filepath.Join(dir, "status.json"), filepath.Join(dir, "pause"))
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !hub.State("WB01").Online && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !hub.State("WB01").Online {
		t.Fatal("agent did not connect")
	}
	if _, err := pw.Write(makeD12Frame(0x30, "001550")); err != nil {
		t.Fatal(err)
	}
	waitState := func(weight string, stable bool) {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			s := hub.State("WB01")
			if s.Online && s.Weight.String() == weight && s.Stable == stable {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("hub state weight=%s stable=%t: %+v", weight, stable, hub.State("WB01"))
	}
	waitState("1.55", true)
	if err := os.WriteFile(filepath.Join(dir, "pause"), []byte("maintenance"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := hub.Print("WB01", 42, map[string]string{"ticketNo": "GB-PENDING"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // let the print worker enter its pause wait
	if _, err := pw.Write(makeD12Frame(0x38, "001560")); err != nil {
		t.Fatal(err)
	}
	waitState("1.56", false)
	if _, err := os.Stat(filepath.Join(dir, "attempt-42.json")); !os.IsNotExist(err) {
		t.Fatal("paused print reserved an attempt")
	}
	state := hub.State("WB01")
	result, err := hub.Kick(ctx, "WB01", state.ConnectionID)
	if err != nil || !result.ExitConfirmed {
		t.Fatalf("kick: %+v %v", result, err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, p.ErrKicked) {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not exit after kick while print pending")
	}
}

func TestCaptureRequestWithoutCameraFailsExplicitly(t *testing.T) {
	hub := agentbridge.New(agentbridge.Config{ID: "printer", ScaleCode: "WB01", Key: "printer-test-key", ImageDir: t.TempDir()})
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	dir := t.TempDir()
	pr, pw := io.Pipe()
	scale, err := agentscale.New(agentscale.Config{Port: "/dev/ttyS0", Baud: 9600}, func(context.Context) (io.ReadCloser, error) { return pr, nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go scale.Run(ctx)
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, agentprinter.Config{ServerURL: server.URL, AgentID: "printer", ScaleCode: "WB01", AgentKey: "printer-test-key", Queue: "unused"}, scale, dir, filepath.Join(dir, "status.json"), filepath.Join(dir, "pause"))
	}()
	defer func() {
		cancel()
		_ = pw.Close()
		_ = pr.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !hub.State("WB01").Online && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := pw.Write(makeD12Frame(0x30, "001550")); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := hub.State("WB01")
		if s.Online && s.DeviceOK && s.Stable && s.Weight.String() == "1.55" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	result, _, err := hub.Capture(ctx, "WB01")
	if !errors.Is(err, agentbridge.ErrUnavailable) {
		t.Fatalf("capture error %v", err)
	}
	if result.Success || result.Error == "" || len(result.Images) != 0 {
		t.Fatalf("capture failure was not explicit: %+v", result)
	}
	if !strings.Contains(result.Error, "摄像头未接入") {
		t.Fatalf("capture reason: %q", result.Error)
	}
}
