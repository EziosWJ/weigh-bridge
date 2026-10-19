// agent-printer connects the SK-820 host to the existing Agent protocol.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"golang.org/x/net/websocket"
)

func main() {
	server := flag.String("server", "http://127.0.0.1:18900", "Server URL")
	id := flag.String("agent-id", "scale-agent-1", "Agent identity")
	scale := flag.Int64("scale-id", 1, "Scale identity")
	queue := flag.String("queue", "Aisino-SK820-raw", "CUPS queue")
	state := flag.String("state-dir", "./state", "Durable attempt directory")
	flag.Parse()
	key := os.Getenv("APP_WEIGHING__AGENT_KEY")
	if key == "" || *scale < 1 {
		slog.Error("Agent key and valid scale required")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for ctx.Err() == nil {
		if err := run(ctx, *server, *id, key, *scale, *queue, *state); err != nil {
			slog.Warn("connection ended", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}
func run(ctx context.Context, server, id, key string, scale int64, queue, state string) error {
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
	cfg, err := websocket.NewConfig(u.String(), server)
	if err != nil {
		return err
	}
	cfg.Header.Set("Authorization", "Bearer "+key)
	cfg.Header.Set("X-Agent-ID", id)
	cfg.Header.Set("X-Scale-ID", strconv.FormatInt(scale, 10))
	dial, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := cfg.DialContext(dial)
	cancel()
	if err != nil {
		return err
	}
	defer conn.Close()
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
	send := func(kind string, payload any) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return websocket.JSON.Send(conn, p.New(kind, fmt.Sprintf("printer-%d", time.Now().UnixNano()), "", payload))
	}
	if err = send("device.state", p.DeviceState{Unit: "t", DeviceOK: false, Error: "仪表和摄像头未接入；仅打印可用"}); err != nil {
		return err
	}
	incoming := make(chan p.Message)
	readErr := make(chan error, 1)
	go func() {
		for {
			_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
			var m p.Message
			if err := websocket.JSON.Receive(conn, &m); err != nil {
				readErr <- err
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
			jobCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
