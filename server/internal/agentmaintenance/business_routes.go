package agentmaintenance

import (
	"context"
	"net/http"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

const (
	PermissionView   = "agent:maintenance:view"
	PermissionManage = "agent:maintenance:manage"
)

type businessRequestKey struct{}

// RegisterBusinessRoutes mounts the maintenance API under the supplied system
// group. The group normally has /api/system as its prefix. This adapter keeps
// the existing maintenance behavior while using the business JWT, RBAC, and
// operation audit recorder. The independent Handler remains available during
// the transition.
func (s *Service) RegisterBusinessRoutes(group *gin.RouterGroup, authenticator auth.Authenticator, permissions rbac.PermissionChecker, recorder audit.Recorder) {
	routes := group.Group("/agent-maintenance")
	routes.Use(auth.BearerMiddleware(authenticator))
	handler := s.Handler()
	bind := func(method, path, legacyPath, permission string, action string) {
		routes.Handle(method, path, rbac.RequireAnyPermission(permissions, permission, PermissionManage), func(c *gin.Context) {
			request := c.Request.Clone(context.WithValue(c.Request.Context(), businessRequestKey{}, true))
			request.URL.Path = "/api/" + legacyPath
			if request.URL.RawPath != "" {
				request.URL.RawPath = ""
			}
			request.RequestURI = request.URL.RequestURI()
			writer := &captureResponseWriter{ResponseWriter: c.Writer}
			handler.ServeHTTP(writer, request)
			if recorder != nil && method != http.MethodGet && method != http.MethodHead && writer.status() < http.StatusBadRequest {
				actor, _ := platformhttp.UserIDFromContext(request.Context())
				meta, _ := platformhttp.RequestMetaFromContext(request.Context())
				_ = recorder.Record(request.Context(), audit.Event{
					Action: action, Resource: "agent-maintenance",
					Summary: "设备维护操作" + action,
					Metadata: audit.Metadata{ActorID: actor, RequestID: meta.RequestID, ClientIP: meta.ClientIP,
						UserAgent: meta.UserAgent, RequestMethod: method, RequestURL: c.FullPath()},
				})
			}
		})
	}
	routes.GET("/camera-preview", rbac.RequireAnyPermission(permissions, PermissionView, PermissionManage), func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		cfg, runtimeError := s.CameraConfig()
		request := c.Request.WithContext(context.WithValue(c.Request.Context(), businessRequestKey{}, true))
		if err := s.validateBusinessConfig(request, cfg); err != nil {
			platformhttp.WriteError(c, 400, 400, err.Error(), nil)
			return
		}
		platformhttp.OK(c, cfg.PreviewConfig(runtimeError))
	})
	view := PermissionView
	manage := PermissionManage
	bind(http.MethodGet, "/status", "status", view, "maintenance.status")
	bind(http.MethodGet, "/jobs", "jobs", view, "maintenance.jobs")
	bind(http.MethodGet, "/config", "config", view, "maintenance.config.read")
	bind(http.MethodPut, "/config", "config", manage, "maintenance.config.save")
	bind(http.MethodPost, "/config/import", "config/import", manage, "maintenance.config.import")
	bind(http.MethodPost, "/action", "action", manage, "maintenance.action")
	bind(http.MethodPost, "/test-print", "test-print", manage, "maintenance.test-print")
	bind(http.MethodPost, "/test-camera", "test-camera", manage, "maintenance.test-camera")
	bind(http.MethodGet, "/logs", "logs", view, "maintenance.logs.read")
	bind(http.MethodGet, "/audit", "audit", view, "maintenance.audit.read")
}

type captureResponseWriter struct {
	http.ResponseWriter
	code int
}

func (w *captureResponseWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *captureResponseWriter) Write(data []byte) (int, error) {
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *captureResponseWriter) status() int {
	if w.code == 0 {
		return http.StatusOK
	}
	return w.code
}

var _ http.ResponseWriter = (*captureResponseWriter)(nil)
