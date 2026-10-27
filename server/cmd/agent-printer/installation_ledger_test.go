package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/gin-gonic/gin"
)

func TestAgentReusesPrintLedgerAfterScaleCodeChangeAndRestart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	for name, script := range map[string]string{
		"python3": "#!/bin/sh\ncat >/dev/null\nprintf rendered\n",
		"lp":      "#!/bin/sh\ncat >/dev/null\necho called >> '" + calls + "'\necho request-id\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	var mu sync.RWMutex
	scaleCode := "WB01"
	hub := agentbridge.NewMultiple(nil)
	hub.SetLocalResolver(func(context.Context) (agentbridge.Config, error) {
		mu.RLock()
		defer mu.RUnlock()
		return agentbridge.Config{ID: "agent-a", SiteCode: "F001", ScaleCode: scaleCode, InstallationID: "00000000-0000-4000-8000-000000000001", ImageDir: root}, nil
	})
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	scale, writer, stop := startTestScale(t)
	defer stop()
	defer writer.Close()
	ledger, err := agentprinter.InstallationDirectory(root, "00000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger, "attempt-77.json"), []byte("reserved before upgrade"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := agentprinter.Defaults()
	cfg.ServerURL = server.URL
	cfg.AgentID = "agent-a"
	cfg.ScaleCode = "WB01"
	cfg.Queue = "test"
	start := func(current, ledger string) (context.CancelFunc, <-chan error) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- runManaged(ctx, cfg, scale, ledger, filepath.Join(root, "status.json"), filepath.Join(root, "pause"), nil, root)
		}()
		deadline := time.Now().Add(4 * time.Second)
		for (!hub.Connected(current) || !hub.Supports(current, p.CapabilityMeasurementWindow)) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if !hub.Connected(current) || !hub.Supports(current, p.CapabilityMeasurementWindow) {
			cancel()
			t.Fatalf("Agent did not connect to %s", current)
		}
		return cancel, done
	}
	stopRun := func(cancel context.CancelFunc, done <-chan error) {
		t.Helper()
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("Agent did not stop")
		}
	}
	sheet := &agentprinter.Sheet{Template: "weighing-compact-v1", Width: 210, Height: 99, Title: "测试", Rows: [][]agentprinter.Field{{{Label: "单号", Value: "TEST"}}, {{Label: "物料", Value: "高粱"}}, {{Label: "毛重", Value: "30"}}, {{Label: "时间", Value: "2026"}}, {{Label: "签字", Value: "签字"}}}}
	ticket := map[string]any{"printSheet": sheet}
	sendAndWait := func(current string, id int64) {
		t.Helper()
		if err := hub.Print(current, id, ticket); err != nil {
			t.Fatalf("send %d: %v", id, err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(ledger, fmt.Sprintf("attempt-%d.json.cups", id))); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("new print %d was not processed", id)
	}
	cancel, done := start("WB01", root)
	if err := hub.Print("WB01", 77, ticket); err != nil {
		t.Fatal(err)
	}
	sendAndWait("WB01", 78)
	stopRun(cancel, done)
	mu.Lock()
	scaleCode = "WB02"
	mu.Unlock()
	cfg.ScaleCode = "WB02"
	cancel, done = start("WB02", root)
	if err := hub.Print("WB02", 77, ticket); err != nil {
		t.Fatal(err)
	}
	sendAndWait("WB02", 79)
	stopRun(cancel, done)
	output, err := os.ReadFile(calls)
	if err != nil || strings.Count(string(output), "called") != 2 {
		t.Fatalf("unexpected physical print calls=%q err=%v", output, err)
	}
}
