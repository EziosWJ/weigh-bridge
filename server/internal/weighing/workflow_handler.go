package weighing

import (
	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerWorkflowRoutes(router gin.IRouter, h *Handler) {
	read := rbac.RequirePermission(h.permissions, "weighing:read")
	capture := rbac.RequireAnyPermission(h.permissions, "weighing:confirm", "weighing:reweigh")
	confirm := rbac.RequirePermission(h.permissions, "weighing:confirm")
	router.GET("/scales/:code/session", read, h.session)
	router.GET("/scales/:code/device", read, h.deviceState)
	router.GET("/scales/:code/camera-preview", read, h.cameraPreview)
	router.GET("/plate-candidates", read, h.plateCandidates)
	router.POST("/plate-confirmations", confirm, h.confirmPlateRecognition)
	if h.service.devices != nil {
		router.GET("/agent-images/:imageId", read, h.service.devices.ServeImage)
	}
	for _, action := range []string{"acquire", "renew", "release", "takeover"} {
		middleware := rbac.RequireAnyPermission(h.permissions, "weighing:confirm", "weighing:reweigh", "weighing:takeover")
		if action == "takeover" {
			middleware = rbac.RequirePermission(h.permissions, "weighing:takeover")
		}
		router.POST("/scales/:code/session/"+action, middleware, h.changeSession(action))
	}
	router.POST("/samples", capture, h.capture)
	router.POST("/samples/:id/invalidate", capture, h.invalidateSample)
	router.POST("/tasks/first-confirmation", confirm, h.confirmFirst)
	router.POST("/tasks/:id/second-confirmation", confirm, h.confirmSecond)
	router.GET("/tasks", read, h.tasks)
	router.GET("/tickets", read, h.tickets)
	router.GET("/tickets/:id", read, h.ticketDetail)
	router.GET("/tasks/active", read, h.activeTask)
	router.GET("/tasks/:id", read, h.taskDetail)
	router.GET("/operations/:requestId", h.operation)
}

// session godoc
// @Summary 地磅操作会话
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Success 200 {object} HTTPEnvelope[ScaleSession]
// @Router /api/weighing/scales/{id}/session [get]
func (h *Handler) session(c *gin.Context) {
	v, e := h.service.Session(c, c.Param("code"))
	writeResult(c, v, e)
}

// changeSession godoc
// @Summary 取得、续租、释放或接管地磅操作权
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body SessionInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[SessionResult]
// @Router /api/weighing/scales/{id}/session/acquire [post]
// @Router /api/weighing/scales/{id}/session/renew [post]
// @Router /api/weighing/scales/{id}/session/release [post]
// @Router /api/weighing/scales/{id}/session/takeover [post]
func (h *Handler) changeSession(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in SessionInput
		if c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		v, e := h.service.ChangeSession(c, c.Param("code"), action, in, requestEvent(c))
		writeResult(c, v, e)
	}
}

// capture godoc
// @Summary 从 Agent 采集重量与对应抓拍
// @Tags 称重业务
// @Security BearerAuth
// @Param request body CaptureInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[Sample]
// @Router /api/weighing/samples [post]
func (h *Handler) capture(c *gin.Context) {
	var in CaptureInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	v, e := h.service.Capture(c, in, requestEvent(c))
	writeResult(c, v, e)
}

// invalidateSample godoc
// @Summary 废弃未确认采样
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body SessionInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/samples/{id}/invalidate [post]
func (h *Handler) invalidateSample(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var in SessionInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	writeResult(c, nil, h.service.InvalidateSample(c, id, in, principal.UserID))
}

// confirmFirst godoc
// @Summary 确认首磅并原子创建运输任务
// @Tags 称重业务
// @Security BearerAuth
// @Param request body ConfirmationInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[CommandResult]
// @Router /api/weighing/tasks/first-confirmation [post]
func (h *Handler) confirmFirst(c *gin.Context) {
	var in ConfirmationInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	v, e := h.service.ConfirmFirst(c, in, requestEvent(c))
	writeResult(c, v, e)
}

// taskDetail godoc
// @Summary 运输任务、称重依据与历史详情
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Success 200 {object} HTTPEnvelope[TaskDetail]
// @Router /api/weighing/tasks/{id} [get]
func (h *Handler) taskDetail(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	v, e := h.service.FindTask(c, id)
	writeResult(c, v, e)
}

// activeTask godoc
// @Summary 按车牌查询未结束运输任务
// @Tags 称重业务
// @Security BearerAuth
// @Param plate query string true "人工核对车牌"
// @Success 200 {object} HTTPEnvelope[TaskDetail]
// @Router /api/weighing/tasks/active [get]
func (h *Handler) activeTask(c *gin.Context) {
	v, e := h.service.ActiveTask(c, c.Query("plate"))
	writeResult(c, v, e)
}

// operation godoc
// @Summary 查询当前账号原请求的提交结果
// @Tags 称重业务
// @Security BearerAuth
// @Param requestId path string true "原请求编号（仅当前账号）"
// @Success 200 {object} HTTPEnvelope[Operation]
// @Router /api/weighing/operations/{requestId} [get]
func (h *Handler) operation(c *gin.Context) {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	v, e := h.service.Operation(c, principal.UserID, c.Param("requestId"))
	writeResult(c, v, e)
}

// confirmSecond godoc
// @Summary 确认二磅并原子生成过磅单
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body ConfirmationInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[CommandResult]
// @Router /api/weighing/tasks/{id}/second-confirmation [post]
func (h *Handler) confirmSecond(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var in ConfirmationInput
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
	v, e := h.service.ConfirmSecond(c, id, in, requestEvent(c))
	writeResult(c, v, e)
}

// tasks godoc
// @Summary 运输任务分页筛选
// @Tags 称重业务
// @Security BearerAuth
// @Param keyword query string false "车牌、任务号或单号"
// @Param from query string false "北京时间起始日期 YYYY-MM-DD"
// @Param to query string false "北京时间结束日期 YYYY-MM-DD（包含该日）"
// @Param direction query string false "inbound/outbound"
// @Param materialId query int false "物料 ID"
// @Param partyId query int false "供收方 ID"
// @Param status query string false "对应列表状态"
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数 1-500，默认 10"
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/tasks [get]
func (h *Handler) tasks(c *gin.Context) {
	q, err := pageQuery(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	v, e := h.service.Tasks(c, q)
	writeResult(c, v, e)
}

// tickets godoc
// @Summary 各版测试单据分页筛选
// @Tags 称重业务
// @Security BearerAuth
// @Param keyword query string false "车牌、任务号或单号"
// @Param from query string false "北京时间起始日期 YYYY-MM-DD"
// @Param to query string false "北京时间结束日期 YYYY-MM-DD（包含该日）"
// @Param direction query string false "inbound/outbound"
// @Param materialId query int false "物料 ID"
// @Param partyId query int false "供收方 ID"
// @Param status query string false "对应列表状态"
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数 1-500，默认 10"
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/tickets [get]
func (h *Handler) tickets(c *gin.Context) {
	q, err := pageQuery(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	v, e := h.service.Tickets(c, q)
	writeResult(c, v, e)
}

// ticketDetail godoc
// @Summary 当版测试单据不可变快照
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Success 200 {object} HTTPEnvelope[TicketView]
// @Router /api/weighing/tickets/{id} [get]
// @Router /api/weighing/tickets/{id}/preview [get]
func (h *Handler) ticketDetail(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	v, e := h.service.FindTicket(c, id)
	writeResult(c, v, e)
}

func (h *Handler) deviceState(c *gin.Context) {
	id := c.Param("code")
	if h.service.devices == nil {
		writeResult(c, nil, ErrConditions)
		return
	}
	policy, err := h.service.CapturePolicy(c.Request.Context())
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	reason := h.service.captureBlockedReasonWithPolicy(id, policy.Required)
	writeResult(c, struct {
		agentbridge.State
		CaptureRequired       bool   `json:"captureRequired"`
		CapturePolicyRevision string `json:"capturePolicyRevision"`
		CaptureBlockedReason  string `json:"captureBlockedReason,omitempty"`
	}{h.service.devices.State(id), policy.Required, policy.Revision, reason}, nil)
}
