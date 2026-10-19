package weighing

import (
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerCorrectionRoutes(router gin.IRouter, h *Handler) {
	router.POST("/tasks/:id/corrections", rbac.RequirePermission(h.permissions, "weighing:correct"), h.correctTask)
	router.POST("/tasks/:id/termination", rbac.RequirePermission(h.permissions, "weighing:terminate"), h.terminateTask)
	router.POST("/tasks/:id/reweighs", rbac.RequirePermission(h.permissions, "weighing:reweigh"), h.reweigh)
	router.POST("/tickets/:id/void", rbac.RequirePermission(h.permissions, "weighing:void"), h.voidTicket)
	router.POST("/tickets/:id/reopen", rbac.RequirePermission(h.permissions, "weighing:reopen"), h.reopenTicket)
}

// correctTask godoc
// @Summary 未完成任务业务资料纠错
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body CorrectionInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[CommandResult]
// @Router /api/weighing/tasks/{id}/corrections [post]
func (h *Handler) correctTask(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var in CorrectionInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	v, e := h.service.CorrectTask(c, id, in, requestEvent(c))
	writeResult(c, v, e)
}

// terminateTask godoc
// @Summary 终止任务且不可恢复
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body TaskActionInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[CommandResult]
// @Router /api/weighing/tasks/{id}/termination [post]
func (h *Handler) terminateTask(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var in TaskActionInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	v, e := h.service.TerminateTask(c, id, in, requestEvent(c))
	writeResult(c, v, e)
}

// voidTicket godoc
// @Summary 作废测试单并保留原内容
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body VoidInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[CommandResult]
// @Router /api/weighing/tickets/{id}/void [post]
func (h *Handler) voidTicket(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var in VoidInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	v, e := h.service.VoidTicket(c, id, in, requestEvent(c))
	writeResult(c, v, e)
}

// reweigh godoc
// @Summary 指定一磅重称并保留替代链
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body ReweighInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[CommandResult]
// @Router /api/weighing/tasks/{id}/reweighs [post]
func (h *Handler) reweigh(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var in ReweighInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	v, e := h.service.Reweigh(c, id, in, requestEvent(c))
	writeResult(c, v, e)
}

// reopenTicket godoc
// @Summary 从末版作废单关联更正重开
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body ReopenInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[CommandResult]
// @Router /api/weighing/tickets/{id}/reopen [post]
func (h *Handler) reopenTicket(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var in ReopenInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	in.AllowAdjust, err = h.permissions.HasPermission(c, principal.UserID, "weighing:adjust")
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	in.AllowCorrect, err = h.permissions.HasPermission(c, principal.UserID, "weighing:correct")
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	v, e := h.service.ReopenTicket(c, id, in, requestEvent(c))
	writeResult(c, v, e)
}
