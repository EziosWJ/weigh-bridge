package main

import (
	"context"
	"errors"
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

func TestPrinterKickEndsRunNormally(t *testing.T) {
	hub := agentbridge.New(agentbridge.Config{ID: "printer", ScaleID: 1, Key: "printer-test-key", ImageDir: t.TempDir()})
	router := gin.New()
	hub.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		scale, err := agentscale.New(agentscale.Config{}, nil)
		if err != nil {
			done <- err
			return
		}
		done <- run(ctx, agentprinter.Config{ServerURL: server.URL, AgentID: "printer", ScaleID: 1, AgentKey: "printer-test-key", Queue: "unused"}, scale, dir, filepath.Join(dir, "status.json"), filepath.Join(dir, "pause"))
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !hub.State(1).Online && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := hub.State(1)
	if !state.Online {
		t.Fatal("printer did not connect")
	}
	// A paused print may not prevent the reader from processing kick.
	if err := os.WriteFile(filepath.Join(dir, "pause"), []byte("maintenance"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := hub.Print(1, 99, map[string]string{"ticketNo": "GB-KICK"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	result, err := hub.Kick(ctx, 1, state.ConnectionID)
	if err != nil || !result.ExitConfirmed {
		t.Fatalf("kick: %+v %v", result, err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, p.ErrKicked) {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("printer did not exit")
	}
	if hub.State(1).Online {
		t.Fatal("printer stayed online after kick")
	}
}
