package weighing

import (
	"time"

	"github.com/gin-gonic/gin"
)

type LiveWeight struct {
	ScaleCode  string     `json:"scaleCode"`
	Weight     *string    `json:"weight" extensions:"x-nullable"`
	Unit       string     `json:"unit"`
	Online     bool       `json:"online"`
	DeviceOK   bool       `json:"deviceOk"`
	Stable     bool       `json:"stable"`
	ReceivedAt *time.Time `json:"receivedAt" extensions:"x-nullable"`
}

func RegisterPublicRoutes(router gin.IRouter, h *Handler) {
	router.GET("/scales/:code/device", h.liveWeight)
}

// liveWeight godoc
// @Summary 厂内匿名读取地磅实时重量
// @Description 无需令牌。返回最新设备状态，离线、过期或仪表故障时重量为 null。
// @Tags 称重设备
// @Param code path string true "当前本厂地磅编码"
// @Success 200 {object} HTTPEnvelope[LiveWeight]
// @Header 200 {string} Cache-Control "no-store"
// @Failure 404 {object} HTTPEnvelope[any]
// @Router /api/weighing/scales/{code}/device [get]
func (h *Handler) liveWeight(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	status, err := h.service.LocalStatus(c.Request.Context())
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	if !status.Configured || c.Param("code") != status.ScaleCode {
		writeResult(c, nil, ErrNotFound)
		return
	}

	result := LiveWeight{ScaleCode: status.ScaleCode, Unit: "t"}
	if devices := h.service.devices; devices != nil {
		state := devices.State(status.ScaleCode)
		if !state.ReceivedAt.IsZero() {
			result.ReceivedAt = &state.ReceivedAt
		}
		result.Online = state.Online
		result.DeviceOK = state.Online && state.DeviceOK
		if result.DeviceOK {
			weight := state.Weight.String()
			result.Weight = &weight
			result.Stable = state.Stable
		}
	}
	writeResult(c, result, nil)
}
