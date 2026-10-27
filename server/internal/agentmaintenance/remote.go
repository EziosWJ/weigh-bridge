package agentmaintenance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	ph "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

// Remote owns no device files or executors. Resolve selects the local site's
// bound Agent from business data, never from an arbitrary browser address.
type Remote struct {
	hub     *agentbridge.Hub
	resolve func(context.Context) (Config, error)
	guard   func(context.Context) (func(), error)
	store   ConfigStore
}

func NewRemote(hub *agentbridge.Hub, resolve func(context.Context) (Config, error), guard func(context.Context) (func(), error)) *Remote {
	return &Remote{hub: hub, resolve: resolve, guard: guard}
}

// SetStore attaches the PostgreSQL persistence used for effective configuration
// and manual backups. Without a store the page can still apply, but read-only
// restore points and offline display are unavailable.
func (r *Remote) SetStore(store ConfigStore) { r.store = store }
func (r *Remote) CameraConfig() (Config, error) {
	cfg, err := r.resolve(context.Background())
	if err != nil {
		return cfg, err
	}
	state, err := r.hub.MaintenanceState(cfg.ScaleCode)
	if errors.Is(err, agentbridge.ErrMaintenanceUnsupported) {
		return Config{ScaleCode: cfg.ScaleCode}, nil
	}
	if err != nil {
		return cfg, err
	}
	if !state.Running {
		return state.Config, errors.New("设备执行已停止")
	}
	return state.Config, nil
}
func (r *Remote) CameraAvailable(scaleCode string) bool {
	cfg, err := r.CameraConfig()
	if err != nil || cfg.ScaleCode != scaleCode {
		return false
	}
	return cfg.CameraMode == "" || cfg.CameraEnabled
}
func (r *Remote) RegisterBusinessRoutes(group *gin.RouterGroup, a auth.Authenticator, permissions rbac.PermissionChecker, recorder audit.Recorder) {
	routes := group.Group("/agent-maintenance", auth.BearerMiddleware(a))
	for _, entry := range []struct{ method, path string }{
		{"GET", "/status"}, {"GET", "/jobs"}, {"GET", "/config"}, {"PUT", "/config"}, {"POST", "/config/import"}, {"POST", "/action"}, {"POST", "/test-print"}, {"GET", "/logs"}, {"GET", "/audit"},
	} {
		permission := PermissionView
		if entry.method != "GET" {
			permission = PermissionManage
		}
		routes.Handle(entry.method, entry.path, rbac.RequireAnyPermission(permissions, permission, PermissionManage), r.forward(recorder))
	}
	routes.GET("/backups", rbac.RequireAnyPermission(permissions, PermissionView, PermissionManage), r.listBackups)
	routes.POST("/backups", rbac.RequireAnyPermission(permissions, PermissionManage), r.createBackup(recorder))
	routes.DELETE("/backups/:id", rbac.RequireAnyPermission(permissions, PermissionManage), r.deleteBackup(recorder))
	routes.POST("/apply", rbac.RequireAnyPermission(permissions, PermissionManage), r.applyConfig(recorder))
	routes.POST("/restore", rbac.RequireAnyPermission(permissions, PermissionManage), r.restoreConfig(recorder))
	routes.GET("/queues", rbac.RequireAnyPermission(permissions, PermissionView, PermissionManage), r.queues)
	routes.GET("/camera-preview", rbac.RequireAnyPermission(permissions, PermissionView, PermissionManage), func(c *gin.Context) {
		cfg, err := r.CameraConfig()
		c.Header("Cache-Control", "no-store")
		if err != nil {
			ph.WriteError(c, 503, 503, "Agent 离线或设备执行已停止", nil)
			return
		}
		ph.OK(c, cfg.PreviewConfig(nil))
	})
}
func (r *Remote) forward(recorder audit.Recorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, err := r.resolve(c.Request.Context())
		if err != nil {
			ph.WriteError(c, 503, 503, "无法定位本厂设备 Agent", nil)
			return
		}
		path := c.FullPath()[len("/api/system/agent-maintenance"):]
		body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 16384))
		if err != nil {
			ph.WriteError(c, 400, 400, "维护请求过大", nil)
			return
		}
		if c.Request.Method == "PUT" && path == "/config" {
			var candidate Config
			if json.Unmarshal(body, &candidate) != nil || candidate.ScaleCode != cfg.ScaleCode || candidate.AgentID != cfg.AgentID || (candidate.AgentKey != "" && candidate.AgentKey != cfg.AgentKey) {
				ph.WriteError(c, 400, 400, "不能通过设备参数维护更换 Agent 身份或归属", nil)
				return
			}
		}
		changing := path == "/action" || path == "/test-camera"
		if changing && r.guard != nil {
			release, e := r.guard(c.Request.Context())
			if e != nil {
				ph.WriteError(c, 409, 409, "称重请求尚未结束，请核实后再维护", nil)
				return
			}
			defer release()
			r.hub.InvalidateSamples(cfg.ScaleCode)
		}
		actor, _ := ph.UserIDFromContext(c.Request.Context())
		requestMeta, _ := ph.RequestMetaFromContext(c.Request.Context())
		commandContext := c.Request.Context()
		if changing {
			// Browser navigation cannot release the weighing guard while the
			// Agent is still applying a configuration that was already sent.
			var cancel context.CancelFunc
			commandContext, cancel = context.WithTimeout(context.WithoutCancel(commandContext), 40*time.Second)
			defer cancel()
		}
		result, err := r.hub.Maintenance(commandContext, cfg.ScaleCode, p.MaintenanceRequest{Method: c.Request.Method, Path: path, Query: c.Request.URL.RawQuery, Body: body, ActorID: actor, ClientIP: requestMeta.ClientIP})
		if err != nil {
			if c.Request.Method == "GET" && path == "/status" {
				c.JSON(200, map[string]any{"maintenance": "运行中", "agent": "离线", "agentKnown": true, "agentProcessOnline": r.hub.Connected(cfg.ScaleCode), "maintenanceAvailable": false, "remote": true, "lastError": err.Error()})
				return
			}
			ph.WriteError(c, 503, 503, err.Error(), nil)
			return
		}
		c.Header("Cache-Control", "no-store")
		if c.Request.Method == "GET" && path == "/config" && result.Status < 400 && len(result.JSON) > 0 {
			result.JSON = r.decorateConfig(c.Request.Context(), cfg.ScaleCode, result.JSON)
		}
		if path == "/status" && len(result.JSON) > 0 {
			var state map[string]any
			if json.Unmarshal(result.JSON, &state) == nil {
				state["remote"], state["agentProcessOnline"] = true, true
				result.JSON, _ = json.Marshal(state)
			}
		}
		if c.Request.Method != "GET" && result.Status < 400 && recorder != nil {
			meta, _ := ph.RequestMetaFromContext(c.Request.Context())
			_ = recorder.Record(c.Request.Context(), audit.Event{Action: "maintenance" + path, Resource: "agent-maintenance", Summary: "现场 Agent 维护操作", Metadata: audit.Metadata{ActorID: actor, RequestID: meta.RequestID, ClientIP: meta.ClientIP, UserAgent: meta.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.FullPath()}})
		}
		if len(result.JSON) > 0 {
			c.Data(result.Status, "application/json; charset=utf-8", result.JSON)
		} else {
			c.Data(result.Status, "text/plain; charset=utf-8", []byte(result.Text))
		}
	}
}
