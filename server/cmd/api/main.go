// Command api starts the Go REST API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/EziosWJ/weighbridge/server/docs"
	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/dept"
	"github.com/EziosWJ/weighbridge/server/internal/dictionary"
	"github.com/EziosWJ/weighbridge/server/internal/filemgmt"
	"github.com/EziosWJ/weighbridge/server/internal/logmgmt"
	"github.com/EziosWJ/weighbridge/server/internal/monitoring"
	"github.com/EziosWJ/weighbridge/server/internal/nodesync"
	"github.com/EziosWJ/weighbridge/server/internal/notification"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
	"github.com/EziosWJ/weighbridge/server/internal/usermgmt"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

const defaultUserPassword = "admin123"

// @title Weighbridge Server API
// @version 0.1.0
// @description System APIs and server-backed testing weighbridge workflows; formal device integration is not implemented.
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}

	database, err := platformdatabase.Open(context.Background(), cfg.Database)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := database.Close(); err != nil {
			applicationLogger := slog.Default()
			applicationLogger.Error("close database", "error", err)
		}
	}()

	nodeService, err := nodesync.New(database.GORM, cfg.Node, cfg.Weighing.ImageDir)
	if err != nil {
		slog.Error("bind deployment identity; run migrations first", "error", err)
		os.Exit(1)
	}
	authService, err := newAuthService(database, cfg.JWT, cfg.Auth.LoginGuard)
	if err != nil {
		slog.Error("build authentication service", "error", err)
		os.Exit(1)
	}

	rbacService, err := rbac.NewService(rbac.NewRepository(database.GORM))
	if err != nil {
		slog.Error("build RBAC service", "error", err)
		os.Exit(1)
	}
	deptService, err := dept.NewService(dept.NewRepository(database.GORM))
	if err != nil {
		slog.Error("build department service", "error", err)
		os.Exit(1)
	}
	notificationRepository := notification.NewRepository(database.GORM)
	userService, err := usermgmt.NewService(usermgmt.NewRepository(database.GORM, notificationRepository), defaultUserPassword)
	if err != nil {
		slog.Error("build user service", "error", err)
		os.Exit(1)
	}
	dictionaryService, err := dictionary.NewService(dictionary.NewRepository(database.GORM))
	if err != nil {
		slog.Error("build dictionary service", "error", err)
		os.Exit(1)
	}
	configService := sysconfig.NewService(sysconfig.NewRepository(database.GORM))
	fileStorage, err := filemgmt.NewLocalStorage(cfg.File.StorageRoot)
	if err != nil {
		slog.Error("build file storage", "error", err)
		os.Exit(1)
	}
	fileService, err := filemgmt.NewService(filemgmt.NewRepository(database.GORM), fileStorage)
	if err != nil {
		slog.Error("build file service", "error", err)
		os.Exit(1)
	}
	logService, err := logmgmt.NewService(logmgmt.NewRepository(database.GORM), configService)
	if err != nil {
		slog.Error("build log service", "error", err)
		os.Exit(1)
	}
	notificationService, err := notification.NewService(notificationRepository)
	if err != nil {
		slog.Error("build notification service", "error", err)
		os.Exit(1)
	}

	var collector monitoring.Collector
	if cfg.Monitoring.Enabled && cfg.Monitoring.Source == "unix" {
		collector = monitoring.NewUnixCollector(cfg.Monitoring.SocketPath)
	} else if cfg.Monitoring.Enabled {
		collector, err = monitoring.NewNativeCollector()
		if err != nil {
			slog.Error("build host resource collector", "error", err)
			os.Exit(1)
		}
	}
	monitoringService := monitoring.NewService(monitoring.NewRepository(database.GORM), collector, nil, monitoring.WithTimeout(cfg.Monitoring.Timeout), monitoring.WithEnabled(cfg.Monitoring.Enabled))

	weighRepo := weighing.NewRepository(database.GORM, weighing.RuntimeOptions{SessionTTL: cfg.Weighing.SessionTTL, TicketTitle: cfg.Weighing.TicketTitle})
	initial := []agentbridge.Config{}
	for _, v := range cfg.Weighing.Agents {
		initial = append(initial, agentbridge.Config{ID: v.ID, ScaleID: v.ScaleID, Key: v.Key})
	}
	if len(initial) == 0 && cfg.Weighing.AgentKey != "" {
		initial = append(initial, agentbridge.Config{ID: cfg.Weighing.AgentID, ScaleID: cfg.Weighing.ScaleID, Key: cfg.Weighing.AgentKey})
	}
	if cfg.Node.Role == "standalone" {
		for _, binding := range initial {
			site, e := weighRepo.ResourceSite(context.Background(), "scales", binding.ScaleID)
			if e != nil || site != cfg.Node.SiteID {
				slog.Error("configured Agent is outside standalone site", "agent", binding.ID)
				os.Exit(1)
			}
		}
	}
	if err := weighRepo.ImportAgentBindings(context.Background(), initial); err != nil {
		slog.Error("import Agent bindings; run migrations first", "error", err)
		os.Exit(1)
	}
	managed, err := weighRepo.AgentConfigs(context.Background(), cfg.Weighing.ImageDir)
	if err != nil {
		slog.Error("load Agent bindings", "error", err)
		os.Exit(1)
	}
	if cfg.Node.Role == "standalone" {
		local := managed[:0]
		for _, binding := range managed {
			site, e := weighRepo.ResourceSite(context.Background(), "scales", binding.ScaleID)
			if e == nil && site == cfg.Node.SiteID {
				local = append(local, binding)
			}
		}
		managed = local
	}
	agents := agentbridge.NewMultiple(managed)
	agents.SetResolver(func(ctx context.Context, scale int64) (agentbridge.Config, error) {
		if cfg.Node.Role == "central" {
			return agentbridge.Config{}, fmt.Errorf("central has no local Agent")
		}
		if cfg.Node.Role == "site" || cfg.Node.Role == "standalone" {
			site, e := weighRepo.ResourceSite(ctx, "scales", scale)
			if e != nil || site != cfg.Node.SiteID {
				return agentbridge.Config{}, fmt.Errorf("Agent outside local site")
			}
		}
		return weighRepo.AgentConfig(ctx, scale, cfg.Weighing.ImageDir)
	})
	weighService := weighing.NewService(weighRepo, agents)
	var remoteMaintenance *agentmaintenance.Remote
	if cfg.Node.Role != "central" && cfg.Maintenance.IsEnabled(cfg.Node.Role == "standalone") {
		remoteMaintenance = agentmaintenance.NewRemote(agents, func(ctx context.Context) (agentprinter.Config, error) {
			scaleID := cfg.Weighing.ScaleID
			if len(cfg.Weighing.Agents) > 0 {
				scaleID = cfg.Weighing.Agents[0].ScaleID
			}
			return weighRepo.BoundMaintenanceConfig(ctx, cfg.Node.SiteID, scaleID, "http://127.0.0.1"+apiPort(cfg.HTTP.Address))
		}, weighService.BeginMaintenance)
	}
	application, err := app.New(*cfg, database, app.Dependencies{
		Node:              nodeService,
		Auth:              authService,
		RBAC:              rbacService,
		Department:        deptService,
		User:              userService,
		Dictionary:        dictionaryService,
		SysConfig:         configService,
		File:              fileService,
		Log:               logService,
		Notification:      notificationService,
		Monitoring:        monitoringService,
		RemoteMaintenance: remoteMaintenance,
		AuditRecorder:     audit.NewRecorder(database.GORM),
		Weighing:          weighService,
	})
	if err != nil {
		slog.Error("build application", "error", err)
		os.Exit(1)
	}
	monitoringContext, stopMonitoring := context.WithCancel(context.Background())
	monitoringDone := make(chan struct{})
	if cfg.Monitoring.Enabled {
		go func() {
			defer close(monitoringDone)
			monitoringService.Run(monitoringContext)
		}()
	} else {
		close(monitoringDone)
	}
	defer stopMonitoring()
	syncDone := make(chan struct{})
	if cfg.Node.Role == "site" {
		go func() { defer close(syncDone); nodeService.Run(monitoringContext) }()
	} else {
		close(syncDone)
	}

	server := &http.Server{
		Addr:              cfg.HTTP.Address,
		Handler:           application.Router,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	go func() {
		application.Logger.Info("HTTP server started", "address", cfg.HTTP.Address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			application.Logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	notificationService.Close()
	stopMonitoring()
	<-monitoringDone
	<-syncDone

	shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		application.Logger.Error("HTTP server shutdown failed", "error", err)
		os.Exit(1)
	}
	application.Logger.Info("HTTP server stopped")
}

func newAuthService(database *platformdatabase.Database, jwtConfig config.JWTConfig, guardConfig config.LoginGuardConfig) (*auth.Service, error) {
	tokens, err := auth.NewTokenManager(auth.TokenConfig{
		SigningKey: jwtConfig.Secret,
		Issuer:     jwtConfig.Issuer,
		Audience:   jwtConfig.Audience,
		TTL:        jwtConfig.TTL,
	})
	if err != nil {
		return nil, err
	}
	loginGuard, err := auth.NewLoginGuard(auth.LoginGuardConfig{
		IPWindow:            guardConfig.IPWindow,
		IPMaxAttempts:       guardConfig.IPMaxAttempts,
		UsernameWindow:      guardConfig.UsernameWindow,
		UsernameMaxAttempts: guardConfig.UsernameMaxAttempts,
		BackoffInitial:      guardConfig.BackoffInitial,
		BackoffMax:          guardConfig.BackoffMax,
		LockDuration:        guardConfig.LockDuration,
		MaxEntries:          guardConfig.MaxEntries,
	})
	if err != nil {
		return nil, fmt.Errorf("build login guard: %w", err)
	}
	return auth.NewService(auth.NewRepository(database.GORM), tokens, loginGuard)
}

func configuredAgents(cfg config.WeighingConfig) *agentbridge.Hub {
	if len(cfg.Agents) == 0 {
		return agentbridge.New(agentbridge.Config{ID: cfg.AgentID, ScaleID: cfg.ScaleID, Key: cfg.AgentKey, ImageDir: cfg.ImageDir})
	}
	bindings := make([]agentbridge.Config, 0, len(cfg.Agents))
	for _, v := range cfg.Agents {
		bindings = append(bindings, agentbridge.Config{ID: v.ID, ScaleID: v.ScaleID, Key: v.Key, ImageDir: cfg.ImageDir})
	}
	return agentbridge.NewMultiple(bindings)
}
