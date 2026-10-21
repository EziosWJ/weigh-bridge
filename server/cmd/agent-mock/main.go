package main

import (
	"context"
	"flag"
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	// The simulator can run with only its Agent environment, independently of
	// Server database/JWT configuration. A valid local config remains convenient.
	agent := config.WeighingConfig{AgentID: "scale-agent-1", ScaleID: 1}
	serverURL := "http://127.0.0.1:8080"
	if cfg, err := config.Load(); err == nil {
		agent = cfg.Weighing
		serverURL = localServerURL(cfg.HTTP.Address)
	}
	if value := os.Getenv("APP_WEIGHING__AGENT_KEY"); value != "" {
		agent.AgentKey = value
	}
	if value := os.Getenv("APP_WEIGHING__AGENT_ID"); value != "" {
		agent.AgentID = value
	}
	if value := os.Getenv("APP_WEIGHING__SCALE_ID"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 {
			slog.Error("invalid APP_WEIGHING__SCALE_ID")
			os.Exit(1)
		}
		agent.ScaleID = parsed
	}
	server := flag.String("server", serverURL, "Server HTTP(S) address (defaults to local API configuration)")
	listen := flag.String("listen", "0.0.0.0:8090", "mock control page listen address")
	id := flag.String("agent-id", agent.AgentID, "Agent identity")
	scale := flag.Int64("scale-id", agent.ScaleID, "scale identity")
	flag.Parse()
	if agent.AgentKey == "" {
		slog.Error("set APP_WEIGHING__AGENT_KEY in both API and mock environments")
		os.Exit(1)
	}
	m := agentmock.New(agentmock.Config{Server: *server, ID: *id, ScaleID: *scale, Key: agent.AgentKey})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go m.Run(ctx)
	srv := &http.Server{Addr: *listen, Handler: m.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("Agent mock control page", "address", "http://"+*listen, "server", *server)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("mock server", "error", err)
		os.Exit(1)
	}
}

// A wildcard listen address identifies the local API but is not a dial target.
func localServerURL(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
