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
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/camera"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/dept"
	"github.com/EziosWJ/weighbridge/server/internal/dictionary"
	"github.com/EziosWJ/weighbridge/server/internal/filemgmt"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"github.com/EziosWJ/weighbridge/server/internal/logmgmt"
	"github.com/EziosWJ/weighbridge/server/internal/monitoring"
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

	if len(cfg.Weighing.Agents) > 0 || cfg.Weighing.AgentKey != "" {
		slog.Error("Agent bindings must be configured in 本厂配置; remove weighing.agents and weighing.agent_key from the business API configuration")
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

	if cfg.Environment == config.EnvironmentProd {
		if err := localsite.CheckAdminBootstrap(context.Background(), database.GORM); err != nil {
			slog.Error("prepare local administrator", "error", err)
			os.Exit(1)
		}
	}
	// ADR 0030: capture the previous YAML ticket title once, before any page or
	// ticket can read or edit the sys_config value.
	legacyTitle, err := sysconfig.InstallLegacyTicketTitle(context.Background(), database.GORM, cfg.Weighing.TicketTitle)
	if err != nil {
		slog.Error("install ticket title configuration", "error", err)
		os.Exit(1)
	}
	// Freeze the upgrade-time display baseline for tickets created before
	// snapshots existed. Later title edits never rewrite those tickets.
	if err := weighing.FreezeLegacyTicketTitles(context.Background(), database.GORM, legacyTitle); err != nil {
		slog.Error("freeze legacy ticket titles", "error", err)
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

	weighRepo := weighing.NewRepository(database.GORM, weighing.RuntimeOptions{SessionTTL: cfg.Weighing.SessionTTL})
	managed, err := weighRepo.AgentConfigs(context.Background(), cfg.Weighing.ImageDir)
	if err != nil {
		slog.Error("load local Agent binding", "error", err)
		os.Exit(1)
	}
	agents := agentbridge.NewMultiple(managed)
	agents.SetResolver(func(ctx context.Context, scale int64) (agentbridge.Config, error) {
		status, err := localsite.ReadStatus(ctx, database.GORM)
		if err != nil || !status.Configured {
			return agentbridge.Config{}, fmt.Errorf("local factory is not configured")
		}
		site, e := weighRepo.ResourceSite(ctx, "scales", scale)
		if e != nil || site != status.SiteID {
			return agentbridge.Config{}, fmt.Errorf("Agent outside local factory")
		}
		return weighRepo.AgentConfig(ctx, scale, cfg.Weighing.ImageDir)
	})
	// ADR 0030: a keyless Agent only knows the Server address. The local
	// factory and its sole scale are resolved here, never created by a device.
	agents.SetLocalResolver(func(ctx context.Context) (agentbridge.Config, error) {
		status, err := localsite.ReadStatus(ctx, database.GORM)
		if err != nil {
			return agentbridge.Config{}, err
		}
		if !status.Configured {
			return agentbridge.Config{}, agentbridge.ErrSiteNotInitialized
		}
		return weighRepo.LocalAgentConfig(ctx, cfg.Weighing.ImageDir)
	})
	// Runtime reports update only the last-known effective state. Restore points
	// are created explicitly through the configuration-backup API.
	maintenanceStore := agentmaintenance.NewRepository(database.GORM)
	agents.SetConfigStateHandler(func(ctx context.Context, scale int64, effective *p.EffectiveConfig, _ *p.ConfigBackup) {
		if effective != nil {
			if err := maintenanceStore.SaveEffectiveConfig(ctx, scale, *effective); err != nil {
				slog.Warn("persist effective Agent configuration", "scale", scale, "error", err)
			}
		}
	})
	weighService := weighing.NewService(weighRepo, agents)
	cameraService := camera.NewServiceWithDefaults(camera.NewRepository(database.GORM), cfg.Weighing.ImageDir)
	weighService.SetCamera(cameraService)
	var remoteMaintenance *agentmaintenance.Remote
	if cfg.Maintenance.IsEnabled() {
		remoteMaintenance = agentmaintenance.NewRemote(agents, func(ctx context.Context) (agentprinter.Config, error) {
			status, err := weighService.LocalStatus(ctx)
			if err != nil {
				return agentprinter.Config{}, err
			}
			if !status.Configured {
				return agentprinter.Config{}, fmt.Errorf("请先完成本厂配置")
			}
			return weighRepo.BoundMaintenanceConfig(ctx, status.SiteID, 0, "http://127.0.0.1"+apiPort(cfg.HTTP.Address))
		}, weighService.BeginMaintenance)
		remoteMaintenance.SetStore(maintenanceStore)
	}
	application, err := app.New(*cfg, database, app.Dependencies{
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
		Camera:            cameraService,
	})
	if err != nil {
		slog.Error("build application", "error", err)
		os.Exit(1)
	}
	monitoringContext, stopMonitoring := context.WithCancel(context.Background())
	cameraService.Start(monitoringContext)
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
