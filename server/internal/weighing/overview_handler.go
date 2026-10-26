package weighing

import "github.com/gin-gonic/gin"

// overview godoc
// @Summary 本厂今日有效单、净重、待回磅与最近有效单据
// @Tags 称重业务
// @Security BearerAuth
// @Success 200 {object} HTTPEnvelope[Overview]
// @Router /api/weighing/overview [get]
func (h *Handler) overview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	v, err := h.service.Overview(c)
	writeResult(c, v, err)
}
