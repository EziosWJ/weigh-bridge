package weighing

import (
	"log/slog"
	"net/http"

	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerMobileHandoffRoutes(router gin.IRouter, h *Handler) {
	router.POST("/mobile-handoffs/scan", rbac.RequirePermission(h.permissions, "weighing:confirm"), h.scanMobileHandoff)
	router.GET("/mobile-handoffs", rbac.RequirePermission(h.permissions, "weighing:read"), h.mobileHandoffs)
}

// scanMobileHandoff is the legacy authenticated scanner endpoint. The serial
// Agent does not use it: it calls the loopback Unix socket route and never
// obtains a user token. This route remains for an external network scanner
// that already has a staff session and is not shown in the factory UI.
// @Summary 手机酒糟二维码扫码首磅/回磅
// @Tags 称重业务
// @Security BearerAuth
// @Param request body MobileHandoffScanInput true "二维码内容"
// @Success 200 {object} HTTPEnvelope[MobileHandoffResult]
// @Router /api/weighing/mobile-handoffs/scan [post]
func (h *Handler) scanMobileHandoff(c *gin.Context) {
	var in MobileHandoffScanInput
	if c.ShouldBindJSON(&in) != nil || in.Code == "" {
		writeResult(c, nil, ErrInvalid)
		return
	}
	value, err := h.service.ScanMobileHandoff(c, in.Code, requestEvent(c))
	if err != nil {
		slog.Error("mobile handoff scan failed", "error", err)
		platformhttp.WriteError(c, http.StatusConflict, http.StatusConflict, err.Error(), nil)
		return
	}
	platformhttp.OK(c, value)
}

// mobileHandoffs returns recent mobile QR handoffs for the factory feedback
// page. @Summary 手机扫码交接记录
// @Tags 称重业务
// @Security BearerAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Param keyword query string false "车牌 / 手机账号 / 任务号 / 磅单号"
// @Param action query string false "动作：first/second"
// @Param status query string false "状态：claimed/completed/failed"
// @Param source query string false "来源：serial_agent/operator"
// @Param from query string false "开始日期（北京时间，YYYY-MM-DD）"
// @Param to query string false "结束日期（北京时间，YYYY-MM-DD）"
// @Success 200 {object} HTTPEnvelope[Page[MobileHandoff]]
// @Router /api/weighing/mobile-handoffs [get]
func (h *Handler) mobileHandoffs(c *gin.Context) {
	q, err := pageQuery(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	if q.Action != "" && q.Action != "first" && q.Action != "second" {
		writeResult(c, nil, ErrInvalid)
		return
	}
	if q.Status != "" && q.Status != MobileHandoffClaimed && q.Status != MobileHandoffCompleted && q.Status != MobileHandoffFailed {
		writeResult(c, nil, ErrInvalid)
		return
	}
	if q.Source != "" && q.Source != "serial_agent" && q.Source != "operator" {
		writeResult(c, nil, ErrInvalid)
		return
	}
	value, err := h.service.MobileHandoffs(c, q)
	writeResult(c, value, err)
}
