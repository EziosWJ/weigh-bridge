package monitoring

import (
	"errors"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/gin-gonic/gin"
	"net/http"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }
func RegisterRoutes(router gin.IRouter, h *Handler) {
	group := router.Group("/monitoring")
	group.GET("/overview", h.overview)
	group.GET("/history", h.history)
}
func principalID(c *gin.Context) (int64, bool) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok || p.UserID <= 0 {
		platformhttp.WriteError(c, http.StatusUnauthorized, platformhttp.CodeUnauthorized, "未登录", nil)
		return 0, false
	}
	return p.UserID, true
}

// overview godoc
// @Summary Linux / Windows 宿主机资源概览（仅 ADMIN；需启用监控）
// @Tags 服务器监控
// @Security BearerAuth
// @Success 200 {object} OverviewEnvelope
// @Failure 401 {object} OverviewEnvelope
// @Failure 403 {object} OverviewEnvelope
// @Failure 503 {object} OverviewEnvelope
// @Router /api/v1/monitoring/overview [get]
func (h *Handler) overview(c *gin.Context) {
	id, ok := principalID(c)
	if !ok {
		return
	}
	value, err := h.service.Overview(c.Request.Context(), id)
	write(c, value, err)
}

// history godoc
// @Summary 当前 API 实例最近 30 分钟资源历史（仅 ADMIN）
// @Tags 服务器监控
// @Security BearerAuth
// @Param resource query string true "资源" Enums(cpu,memory,filesystem,disk,network,gpu)
// @Param device query string false "设备 ID；CPU 逻辑核索引，空值表示总体"
// @Success 200 {object} HistoryEnvelope
// @Failure 400 {object} HistoryEnvelope
// @Failure 401 {object} HistoryEnvelope
// @Failure 403 {object} HistoryEnvelope
// @Failure 503 {object} HistoryEnvelope
// @Router /api/v1/monitoring/history [get]
func (h *Handler) history(c *gin.Context) {
	id, ok := principalID(c)
	if !ok {
		return
	}
	value, err := h.service.History(c.Request.Context(), id, c.Query("resource"), c.Query("device"))
	write(c, value, err)
}

type OverviewEnvelope struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    Snapshot `json:"data"`
}
type HistoryEnvelope struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Data    History `json:"data"`
}

func write(c *gin.Context, value any, err error) {
	switch {
	case err == nil:
		platformhttp.OK(c, value)
	case errors.Is(err, ErrDisabled):
		platformhttp.WriteError(c, http.StatusServiceUnavailable, platformhttp.CodeServiceUnavailable, err.Error(), gin.H{"reason": "monitoring_disabled"})
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.TemporaryUnavailable(c)
	case errors.Is(err, ErrForbidden):
		platformhttp.WriteError(c, http.StatusForbidden, platformhttp.CodeForbidden, err.Error(), nil)
	case errors.Is(err, ErrInvalid):
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, err.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
