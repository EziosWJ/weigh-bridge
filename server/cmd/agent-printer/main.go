// agent-printer connects the SK-820 host to the existing Agent protocol.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"golang.org/x/net/websocket"
)

func main() {
	cfg, state, err := printerOptions(os.Args[1:], os.Getenv)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		slog.Error("invalid printer configuration", "error", err)
		os.Exit(1)
	}
	digest := agentprinter.ConfigDigest(cfg)
	identityDir, identityErr := agentprinter.IdentityDirectory(state, cfg.ServerURL, cfg.AgentID, cfg.ScaleID)
	if identityErr != nil {
		slog.Error("attempt identity initialization failed", "error", identityErr)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for ctx.Err() == nil {
		statusPath := filepath.Join(filepath.Dir(state), "maintenance-status.json")
		pausePath := filepath.Join(filepath.Dir(state), "maintenance.pause")
		if err := run(ctx, cfg, identityDir, statusPath, pausePath); errors.Is(err, p.ErrKicked) {
			writeAgentStatus(statusPath, false, digest)
			slog.Info("Agent 已被后台踢出，停止运行")
			return
		} else if err != nil {
			writeAgentStatus(statusPath, false, digest)
			slog.Warn("connection ended", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}
func run(parent context.Context, cfg agentprinter.Config, state, statusPath, pausePath string) (resultErr error) {
	ctx, cancelRun := context.WithCancelCause(parent)
	defer cancelRun(nil)
	defer func() {
		if errors.Is(context.Cause(ctx), p.ErrKicked) {
			resultErr = p.ErrKicked
		}
	}()
	server, id, key, scale, queue := cfg.ServerURL, cfg.AgentID, cfg.AgentKey, cfg.ScaleID, cfg.Queue
	digest := agentprinter.ConfigDigest(cfg)
	u, err := url.Parse(server)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return fmt.Errorf("expected HTTP(S) Server URL")
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/api/agent/ws"
	wsConfig, err := websocket.NewConfig(u.String(), server)
	if err != nil {
		return err
	}
	wsConfig.Header.Set("Authorization", "Bearer "+key)
	wsConfig.Header.Set("X-Agent-ID", id)
	wsConfig.Header.Set("X-Scale-ID", strconv.FormatInt(scale, 10))
	dial, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := wsConfig.DialContext(dial)
	cancel()
	if err != nil {
		return err
	}
	defer conn.Close()
	writeAgentStatus(statusPath, true, digest)
	defer writeAgentStatus(statusPath, false, digest)
	conn.MaxPayloadBytes = 128 << 10
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	slog.Info("printer Agent connected", "queue", queue, "scale", scale)
	var writes sync.Mutex
	sendMessage := func(message p.Message) error {
		writes.Lock()
		defer writes.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return websocket.JSON.Send(conn, message)
	}
	send := func(kind string, payload any) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return sendMessage(p.New(kind, fmt.Sprintf("printer-%d", time.Now().UnixNano()), "", payload))
	}
	if err = send("device.state", p.DeviceState{Unit: "t", DeviceOK: false, Error: "仪表和摄像头未接入；仅打印可用"}); err != nil {
		return err
	}
	incoming := make(chan p.Message, 32)
	readErr := make(chan error, 1)
	go func() {
		for {
			_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
			var m p.Message
			if err := websocket.JSON.Receive(conn, &m); err != nil {
				readErr <- err
				return
			}
			if m.Type == "agent.kick" {
				_ = sendMessage(p.New("agent.kicked", fmt.Sprintf("kick-%d", time.Now().UnixNano()), m.RequestID, struct{}{}))
				cancelRun(p.ErrKicked)
				_ = conn.Close()
				return
			}
			select {
			case incoming <- m:
			case <-done:
				return
			}
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return err
		case <-ticker.C:
			writeAgentStatus(statusPath, true, digest)
			if err := send("device.state", p.DeviceState{Unit: "t", DeviceOK: false, Error: "仪表和摄像头未接入；仅打印可用"}); err != nil {
				return err
			}
			if err := send("heartbeat", struct{}{}); err != nil {
				return err
			}
		case m := <-incoming:
			if m.Type != "print.request" {
				continue
			}
			var req agentprinter.Request
			if err := json.Unmarshal(m.Payload, &req); err != nil {
				slog.Error("invalid print request", "error", err)
				continue
			}
			if m.RequestID != fmt.Sprintf("print-%d", req.AttemptID) {
				slog.Error("print identity mismatch")
				continue
			}
			for {
				if _, e := os.Stat(pausePath); os.IsNotExist(e) {
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
				}
			}
			jobCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if req.Ticket.PrintSheet != nil {
				req.Ticket.PrintSheet.Calibration = agentprinter.Calibration{WidthMM: cfg.PrintWidthMM, OffsetXMM: cfg.OffsetXMM, OffsetYMM: cfg.OffsetYMM, FormLengthMM: cfg.FormLengthMM}
			}
			data, err := agentprinter.Render(jobCtx, req.Ticket.PrintSheet)
			job := ""
			if err == nil {
				job, err = agentprinter.Submit(jobCtx, state, queue, req, data)
			}
			cancel()
			slog.Info("print attempt requires onsite verification", "attempt", req.AttemptID, "cups", job, "error", err)
			// CUPS submission does not prove physical output. Leave the Server attempt
			// unknown for the existing operator verification workflow; never auto-reprint.
		}
	}
}

func writeAgentStatus(path string, connected bool, digest string) {
	b, _ := json.Marshal(struct {
		At           time.Time `json:"at"`
		Connected    bool      `json:"connected"`
		ConfigDigest string    `json:"configDigest"`
	}{time.Now(), connected, digest})
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, path)
	}
}
