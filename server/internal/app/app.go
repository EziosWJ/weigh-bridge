// Package app wires infrastructure dependencies into the HTTP application.
package app

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
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
	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
	"github.com/EziosWJ/weighbridge/server/internal/usermgmt"
	"github.com/EziosWJ/weighbridge/server/internal/webui"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// Dependencies holds the named business services the HTTP application assembles.
// Core management services are required; notification routes are enabled when
// the optional Notification service is supplied.
type Dependencies struct {
	Node                         *nodesync.Service
	Auth                         *auth.Service
	RBAC                         *rbac.Service
	Department                   *dept.Service
	User                         *usermgmt.Service
	Dictionary                   *dictionary.Service
	SysConfig                    *sysconfig.Service
	File                         *filemgmt.Service
	Log                          *logmgmt.Service
	Notification                 *notification.Service
	Monitoring                   *monitoring.Service
	Maintenance                  *agentmaintenance.Service
	MaintenanceBusinessValidator agentmaintenance.BusinessConfigValidator
	AuditRecorder                audit.Recorder
	Weighing                     *weighing.Service
}

// Application is the assembled HTTP application and its process logger.
type Application struct {
	Router *gin.Engine
	Logger *slog.Logger
}

// New assembles the HTTP router. Database readiness is supplied by the caller
// so that the application layer does not depend on a concrete database driver.
func New(cfg config.Config, readiness platformhttp.ReadinessChecker, deps Dependencies) (*Application, error) {
	if cfg.Node.Role == "standalone" && deps.Maintenance != nil {
		if deps.MaintenanceBusinessValidator == nil {
			return nil, fmt.Errorf("standalone Agent maintenance requires a local identity validator")
		}
		deps.Maintenance.SetBusinessConfigValidator(deps.MaintenanceBusinessValidator)
	}
	if cfg.Node.Role != "" && deps.Node == nil {
		return nil, fmt.Errorf("node service is required for independent deployment")
	}
	if err := deps.validate(); err != nil {
		return nil, err
	}

	logger, err := newLogger(cfg)
	if err != nil {
		return nil, err
	}

	router := gin.New()
	if err := router.SetTrustedProxies(cfg.HTTP.TrustedProxies); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	router.Use(
		platformhttp.RequestMetadata(),
		platformhttp.RequestLogger(logger),
		platformhttp.Recovery(logger),
		platformhttp.CORS(platformhttp.CORSConfig{
			AllowedOrigins: cfg.CORS.AllowedOrigins,
			AllowedMethods: cfg.CORS.AllowedMethods,
			AllowedHeaders: cfg.CORS.AllowedHeaders,
			ExposedHeaders: cfg.CORS.ExposedHeaders,
			MaxAge:         cfg.CORS.MaxAge,
		}),
	)
	if deps.Node != nil {
		router.Use(deps.Node.Boundary)
		deps.Node.Register(router, deps.Auth)
	}
	router.NoRoute(platformhttp.NotFoundHandler)

	platformhttp.RegisterSystemRoutes(router, platformhttp.SystemRoutes{
		Readiness: readiness,
	})
	authHandler, err := auth.NewHandler(deps.Auth.WithServerMonitoring(cfg.Monitoring.Enabled), deps.Auth)
	if err != nil {
		return nil, fmt.Errorf("create authentication handler: %w", err)
	}
	auth.RegisterRoutes(router, authHandler)
	if deps.Weighing != nil {
		deps.Weighing.SetNode(cfg.Node, cfg.Weighing.ImageDir)
		if devices := deps.Weighing.Agent(); devices != nil && cfg.Node.Role != "central" {
			devices.Register(router)
		}
		business := router.Group("/api/weighing")
		business.Use(auth.BearerMiddleware(deps.Auth))
		weighing.RegisterRoutes(business, weighing.NewHandler(deps.Weighing, deps.RBAC))
	}

	system := router.Group("/api/system")
	system.Use(
		auth.BearerMiddleware(deps.Auth),
		rbac.ProtectManagementWrites(deps.RBAC),
		platformhttp.MultipartProtection(platformhttp.MultipartProtectionConfig{
			Policies: map[string]platformhttp.MultipartPolicy{
				"/api/system/file/upload":       {MaxBodyBytes: filemgmt.MaxSingleBodySize},
				"/api/system/file/upload-batch": {MaxBodyBytes: filemgmt.MaxBatchBodySize},
			},
			MaxConcurrent: 8,
			Logger:        logger,
		}),
	)

	rbacHandler, err := rbac.NewHandler(deps.RBAC)
	if err != nil {
		return nil, fmt.Errorf("create RBAC handler: %w", err)
	}
	rbac.RegisterRoutes(system, rbacHandler)

	deptHandler, err := dept.NewHandler(deps.Department)
	if err != nil {
		return nil, fmt.Errorf("create department handler: %w", err)
	}
	dept.RegisterRoutes(system, deptHandler)

	userHandler, err := usermgmt.NewHandler(deps.User)
	if err != nil {
		return nil, fmt.Errorf("create user handler: %w", err)
	}
	usermgmt.RegisterRoutes(system, userHandler)

	dictionaryHandler, err := dictionary.NewHandler(deps.Dictionary)
	if err != nil {
		return nil, fmt.Errorf("create dictionary handler: %w", err)
	}
	dictionary.RegisterRoutes(system, dictionaryHandler)

	sysconfig.NewHandler(deps.SysConfig).Register(system)

	fileHandler, err := filemgmt.NewHandler(deps.File)
	if err != nil {
		return nil, fmt.Errorf("create file handler: %w", err)
	}
	filemgmt.RegisterRoutes(system, fileHandler)

	logHandler, err := logmgmt.NewHandler(deps.Log)
	if err != nil {
		return nil, fmt.Errorf("create log handler: %w", err)
	}
	logmgmt.RegisterRoutes(system, logHandler)
	if deps.Maintenance != nil {
		deps.Maintenance.RegisterBusinessRoutes(system, deps.Auth, deps.RBAC, deps.AuditRecorder)
	}

	if deps.Notification != nil {
		notificationHandler, err := notification.NewHandler(deps.Notification)
		if err != nil {
			return nil, fmt.Errorf("create notification handler: %w", err)
		}
		notification.RegisterRoutes(system, notificationHandler)
		notification.RegisterAnnouncementRoutes(router.Group("/api/public"), system, notificationHandler)
		notification.RegisterRealtimeRoutes(router.Group("/api/public"), system, notificationHandler, deps.Auth)
	}
	if deps.Monitoring != nil {
		if !cfg.Monitoring.Enabled {
			deps.Monitoring = deps.Monitoring.Disabled()
		}
		versioned := router.Group("/api/v1")
		versioned.Use(auth.BearerMiddleware(deps.Auth))
		monitoring.RegisterRoutes(versioned, monitoring.NewHandler(deps.Monitoring))
	}

	if cfg.Environment == config.EnvironmentDev && cfg.Swagger.Enabled {
		registerSwaggerUI(router)
	}

	if err := webui.Register(router); err != nil {
		return nil, fmt.Errorf("register embedded frontend: %w", err)
	}

	return &Application{Router: router, Logger: logger}, nil
}

func (d Dependencies) validate() error {
	switch {
	case d.Auth == nil:
		return fmt.Errorf("auth service is required")
	case d.RBAC == nil:
		return fmt.Errorf("rbac service is required")
	case d.Department == nil:
		return fmt.Errorf("department service is required")
	case d.User == nil:
		return fmt.Errorf("user service is required")
	case d.Dictionary == nil:
		return fmt.Errorf("dictionary service is required")
	case d.SysConfig == nil:
		return fmt.Errorf("sysconfig service is required")
	case d.File == nil:
		return fmt.Errorf("file service is required")
	case d.Log == nil:
		return fmt.Errorf("log service is required")
	}
	return nil
}

// Build constructs a router for callers that do not need the process logger.
func Build(cfg config.Config, readiness platformhttp.ReadinessChecker, deps Dependencies) (*gin.Engine, error) {
	application, err := New(cfg, readiness, deps)
	if err != nil {
		return nil, err
	}
	return application.Router, nil
}

func newLogger(cfg config.Config) (*slog.Logger, error) {
	level := new(slog.LevelVar)
	if err := level.UnmarshalText([]byte(cfg.Log.Level)); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}

	options := &slog.HandlerOptions{
		AddSource: cfg.Log.AddSource,
		Level:     level,
	}
	if cfg.Log.Format == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, options)), nil
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, options)), nil
}
