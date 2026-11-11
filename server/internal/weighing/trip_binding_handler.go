package weighing

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// tripCandidates godoc
// @Summary 按车牌查询云端流转单候选
// @Tags 称重业务
// @Security BearerAuth
// @Param plate query string true "车牌"
// @Success 200 {object} HTTPEnvelope[TripCandidateResult]
// @Router /api/weighing/trip-candidates [get]
func (h *Handler) tripCandidates(c *gin.Context) {
	value, err := h.service.TripCandidates(c.Request.Context(), c.Query("plate"))
	writeResult(c, value, err)
}

// taskTripBasis godoc
// @Summary 查询待回磅任务的首磅流转单快照
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "运输任务 ID"
// @Success 200 {object} HTTPEnvelope[TaskTripBasis]
// @Router /api/weighing/tasks/{id}/trip-basis [get]
func (h *Handler) taskTripBasis(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeResult(c, nil, ErrInvalid)
		return
	}
	value, err := h.service.TaskTripBasis(c.Request.Context(), id)
	writeResult(c, value, err)
}

// taskTripCloudStatus godoc
// @Summary 查询待回磅任务的云端卸货确认状态
// @Tags 称重业务
// @Security BearerAuth
// @Description 仅用于回磅前提醒，不阻断本地称重；云端不可用时在 message 返回原因。
// @Param id path int true "运输任务 ID"
// @Success 200 {object} HTTPEnvelope[TaskTripCloudStatus]
// @Router /api/weighing/tasks/{id}/trip-cloud-status [get]
func (h *Handler) taskTripCloudStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeResult(c, nil, ErrInvalid)
		return
	}
	value, err := h.service.TaskTripCloudStatus(c.Request.Context(), id)
	writeResult(c, value, err)
}
