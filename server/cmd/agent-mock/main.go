package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
	"github.com/EziosWJ/weighbridge/server/internal/agentmock"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

func main() {
	serverDefault := "http://127.0.0.1:8900"
	idDefault := "scale-agent-1"
	scaleDefault := "WB01"
	if value := os.Getenv("APP_WEIGHING__AGENT_ID"); value != "" {
		idDefault = value
	}
	if value := os.Getenv("APP_WEIGHING__SCALE_CODE"); value != "" {
		scaleDefault = value
	}
	server := flag.String("server", serverDefault, "Server HTTP(S) address (explicit mock mode only)")
	listen := flag.String("listen", "0.0.0.0:8090", "mock control page listen address")
	id := flag.String("agent-id", idDefault, "Agent identity (explicit mock mode only)")
	scale := flag.String("scale-code", scaleDefault, "地磅编码（仅显式 mock 模式诊断用）")
	file := flag.String("config", "", "portable connection JSON file")
	envFile := flag.String("env-file", "", "Agent-owned agent.env connection and device settings")
	flag.Parse()

	identityOverride := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "server" || f.Name == "agent-id" || f.Name == "scale-code" {
			identityOverride = true
		}
	})
	key := os.Getenv("APP_WEIGHING__AGENT_KEY")
	connection, err := resolveConnection(*file, *envFile, *server, *id, *scale, key, identityOverride)
	if err != nil {
		slog.Error("mock connection configuration invalid", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	m := agentmock.New(agentmock.Config{
		Server: connection.ServerURL, ID: connection.AgentID, SiteCode: connection.SiteCode, ScaleCode: connection.ScaleCode,
		Key: connection.AgentKey, SiteName: connection.SiteName, ScaleName: connection.ScaleName, OnKick: stop,
	})
	defer stop()
	go m.Run(ctx)
	srv := &http.Server{Addr: *listen, Handler: m.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("Agent mock control page", "address", "http://"+*listen, "server", connection.ServerURL, "site", connection.SiteName, "scale", connection.ScaleCode)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("mock server", "error", err)
		os.Exit(1)
	}
}

func resolveConnection(configFile, envFile, serverURL, agentID string, scaleCode string, mockKey string, identityOverride bool) (agentconfig.Connection, error) {
	if configFile != "" && envFile != "" {
		return agentconfig.Connection{}, errors.New("--config 与 --env-file 不能同时使用")
	}
	if (configFile != "" || envFile != "") && identityOverride {
		return agentconfig.Connection{}, errors.New("使用 --config 或 --env-file 时，不得同时指定 --server、--agent-id 或 --scale-code")
	}
	if configFile != "" {
		return agentconfig.Read(configFile)
	}
	if envFile != "" {
		if _, err := os.Stat(envFile); err != nil {
			return agentconfig.Connection{}, fmt.Errorf("读取 --env-file: %w", err)
		}
		cfg, err := agentprinter.ReadEnvironment(envFile, agentprinter.Defaults())
		if err != nil {
			return agentconfig.Connection{}, fmt.Errorf("读取 Agent 环境配置失败: %w", err)
		}
		connection := connectionFromAgentConfig(cfg)
		if err := connection.ValidateEndpoint(); err != nil {
			return agentconfig.Connection{}, err
		}
		return connection, nil
	}
	// ADR 0030 keyless mode: only the Server address is required. Explicit
	// flags remain useful for local diagnostics, but no key or provisioned
	// identity is an admission gate. The Server assigns the local scale in
	// agent.hello.ack.
	connection := agentconfig.Connection{ServerURL: serverURL, SiteName: "mock", ScaleCode: scaleCode, AgentID: agentID, AgentKey: mockKey}
	if err := connection.ValidateEndpoint(); err != nil {
		return agentconfig.Connection{}, err
	}
	return connection, nil
}

func connectionFromAgentConfig(cfg agentprinter.Config) agentconfig.Connection {
	return agentconfig.Connection{
		ServerURL: cfg.ServerURL, SiteCode: cfg.SiteCode, SiteName: cfg.SiteName,
		ScaleCode: cfg.ScaleCode, ScaleName: cfg.ScaleName, AgentID: cfg.AgentID, AgentKey: cfg.AgentKey,
	}
}
