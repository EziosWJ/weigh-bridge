package serialscanner

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
	"github.com/gin-gonic/gin"
)

const (
	PermissionView   = "agent:maintenance:view"
	PermissionManage = "agent:maintenance:manage"
)

type internalScanInput struct {
	Code   string `json:"code"`
	Source string `json:"source"`
	Device string `json:"device"`
}

// RegisterBusinessRoutes mounts the maintenance-page API. It keeps using the
// existing business JWT, RBAC, and operation audit.
func (s *Service) RegisterBusinessRoutes(group *gin.RouterGroup, authenticator auth.Authenticator, permissions rbac.PermissionChecker, recorder audit.Recorder) {
	routes := group.Group("/agent-maintenance")
	routes.Use(auth.BearerMiddleware(authenticator))
	routes.GET("/serial-scanner", rbac.RequireAnyPermission(permissions, PermissionView, PermissionManage), s.getSnapshot)
	routes.PUT("/serial-scanner", rbac.RequireAnyPermission(permissions, PermissionManage), func(c *gin.Context) {
		var in ConfigInput
		if err := c.ShouldBindJSON(&in); err != nil {
			platformhttp.WriteError(c, 400, 400, "扫码枪参数格式无效", nil)
			return
		}
		actor, _ := platformhttp.UserIDFromContext(c.Request.Context())
		cfg, err := s.SaveConfig(c.Request.Context(), in, actor)
		if err != nil {
			platformhttp.WriteError(c, 400, 400, err.Error(), nil)
			return
		}
		if recorder != nil {
			meta, _ := platformhttp.RequestMetaFromContext(c.Request.Context())
			_ = recorder.Record(c.Request.Context(), audit.Event{
				Action: "serial-scanner.update", Resource: "agent-maintenance",
				Summary: "扫码枪参数已更新",
				Metadata: audit.Metadata{ActorID: actor, RequestID: meta.RequestID, ClientIP: meta.ClientIP,
					UserAgent: meta.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI()},
			})
		}
		snapshot, err := s.Snapshot(c.Request.Context())
		if err != nil {
			platformhttp.WriteError(c, 500, 500, "扫码枪状态读取失败", nil)
			return
		}
		snapshot.Config = cfg
		c.JSON(http.StatusOK, snapshot)
	})
}

func (s *Service) getSnapshot(c *gin.Context) {
	snapshot, err := s.Snapshot(c.Request.Context())
	if err != nil {
		slog.Error("serial scanner snapshot failed", "error", err)
		platformhttp.WriteError(c, 500, 500, "扫码枪状态读取失败", nil)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

// RegisterInternal mounts the loopback-only API consumed by the serial Agent.
// Authentication is the Unix socket file permission; no user token or account
// is involved.
func (s *Service) RegisterInternal(router gin.IRouter) {
	group := router.Group("/api/internal")
	group.GET("/serial-scanner/config", s.internalConfig)
	group.POST("/serial-scanner/heartbeat", s.internalHeartbeat)
	group.POST("/weighing/mobile-handoffs/scan", s.internalScan)
}

func (s *Service) internalConfig(c *gin.Context) {
	cfg, err := s.Config(c.Request.Context())
	if err != nil {
		platformhttp.WriteError(c, 500, 500, "扫码枪配置读取失败", nil)
		return
	}
	platformhttp.OK(c, cfg)
}

func (s *Service) internalHeartbeat(c *gin.Context) {
	var in HeartbeatInput
	if err := c.ShouldBindJSON(&in); err != nil {
		platformhttp.WriteError(c, 400, 400, "心跳格式无效", nil)
		return
	}
	value, err := s.Heartbeat(c.Request.Context(), in)
	if err != nil {
		slog.Error("serial scanner heartbeat failed", "error", err)
		platformhttp.WriteError(c, 500, 500, "心跳写入失败", nil)
		return
	}
	platformhttp.OK(c, value)
}

func (s *Service) internalScan(c *gin.Context) {
	var in internalScanInput
	if err := c.ShouldBindJSON(&in); err != nil || in.Code == "" {
		platformhttp.WriteError(c, 400, 400, "二维码内容不能为空", nil)
		return
	}
	value, err := s.Scan(c.Request.Context(), in.Code, in.Source, in.Device)
	if err != nil {
		slog.Error("serial scanner scan failed", "error", err)
		writeScanError(c, err)
		return
	}
	platformhttp.OK(c, value)
}

func writeScanError(c *gin.Context, err error) {
	var rejected weighing.RejectionError
	switch {
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.WriteError(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, err.Error(), nil)
	case errors.As(err, &rejected):
		platformhttp.WriteError(c, http.StatusConflict, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, weighing.ErrInvalid), errors.Is(err, weighing.ErrConditions), errors.Is(err, weighing.ErrWeight),
		errors.Is(err, weighing.ErrSession), errors.Is(err, weighing.ErrOccupied),
		errors.Is(err, weighing.ErrCaptureCameraRequired), errors.Is(err, weighing.ErrCapturePolicyChanged),
		errors.Is(err, weighing.ErrCameraConfigChanged), errors.Is(err, weighing.ErrMeasurementCapability):
		platformhttp.WriteError(c, http.StatusConflict, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, weighing.ErrForbidden):
		platformhttp.WriteError(c, http.StatusForbidden, http.StatusForbidden, err.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, http.StatusInternalServerError, "扫码处理失败："+err.Error(), nil)
	}
}
