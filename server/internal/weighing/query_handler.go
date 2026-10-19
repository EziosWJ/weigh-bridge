package weighing

import (
	"mime"

	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerQueryRoutes(router gin.IRouter, h *Handler) {
	read := rbac.RequirePermission(h.permissions, "weighing:read")
	print := rbac.RequirePermission(h.permissions, "weighing:print")
	export := rbac.RequirePermission(h.permissions, "weighing:export")
	router.GET("/originals", read, h.originals)
	for _, kind := range []string{"originals", "tickets"} {
		router.GET("/"+kind+"/summary", read, h.summary(kind))
		router.GET("/"+kind+"/export", export, h.export(kind))
		router.GET("/"+kind+"/summary/export", export, h.export(kind+"-summary"))
	}
	router.GET("/tasks/export", export, h.export("tasks"))
	router.GET("/tasks/:id/events", read, h.taskEvents)
	router.GET("/tickets/:id/preview", print, h.ticketDetail)
	router.GET("/tickets/:id/print-attempts", print, h.printAttempts)
	router.POST("/tickets/:id/print-attempts", print, h.startPrint)
	router.POST("/print-attempts/:id/result", print, h.resolvePrint)
}

// originals godoc
// @Summary 每任务一行的当前原始称重组合
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
// @Router /api/weighing/originals [get]
func (h *Handler) originals(c *gin.Context) {
	q, err := pageQuery(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	v, e := h.service.Originals(c, q)
	writeResult(c, v, e)
}

// summary godoc
// @Summary 仅计当前有效单的原始或最终净重汇总
// @Tags 称重业务
// @Security BearerAuth
// @Param keyword query string false "车牌、任务号或单号"
// @Param from query string false "北京时间起始日期 YYYY-MM-DD"
// @Param to query string false "北京时间结束日期 YYYY-MM-DD（包含该日）"
// @Param direction query string false "inbound/outbound"
// @Param materialId query int false "物料 ID"
// @Param partyId query int false "供收方 ID"
// @Param status query string false "对应列表状态"
// @Success 200 {object} HTTPEnvelope[Summary]
// @Router /api/weighing/originals/summary [get]
// @Router /api/weighing/tickets/summary [get]
func (h *Handler) summary(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		q, err := pageQuery(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		v, e := h.service.Summary(c, kind, q)
		writeResult(c, v, e)
	}
}

// export godoc
// @Summary 导出全部筛选结果的测试 CSV
// @Tags 称重业务
// @Security BearerAuth
// @Param keyword query string false "车牌、任务号或单号"
// @Param from query string false "北京时间起始日期 YYYY-MM-DD"
// @Param to query string false "北京时间结束日期 YYYY-MM-DD（包含该日）"
// @Param direction query string false "inbound/outbound"
// @Param materialId query int false "物料 ID"
// @Param partyId query int false "供收方 ID"
// @Param status query string false "对应列表状态"
// @Produce text/csv
// @Success 200 {file} file
// @Router /api/weighing/tasks/export [get]
// @Router /api/weighing/originals/export [get]
// @Router /api/weighing/tickets/export [get]
// @Router /api/weighing/originals/summary/export [get]
// @Router /api/weighing/tickets/summary/export [get]
func (h *Handler) export(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		q, err := pageQuery(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		data, err := h.service.Export(c, kind, q, requestEvent(c))
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "测试-" + kind + ".csv"}))
		c.Data(200, "text/csv; charset=utf-8", data)
	}
}

// taskEvents godoc
// @Summary 任务业务历史和异常尝试（分页）
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数 1-500，默认 10"
// @Success 200 {object} HTTPEnvelope[Page[BusinessEvent]]
// @Router /api/weighing/tasks/{id}/events [get]
func (h *Handler) taskEvents(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	q, err := pageQuery(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	v, e := h.service.TaskEvents(c, id, q)
	writeResult(c, v, e)
}

// printAttempts godoc
// @Summary 单据打印发起和人工反馈历史
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/weighing/tickets/{id}/print-attempts [get]
func (h *Handler) printAttempts(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	v, e := h.service.PrintAttempts(c, id)
	writeResult(c, v, e)
}

// startPrint godoc
// @Summary 登记打印发起或补打，不推断出纸成功
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body PrintInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[CommandResult]
// @Router /api/weighing/tickets/{id}/print-attempts [post]
func (h *Handler) startPrint(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var in PrintInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	in.AllowReprint, err = h.permissions.HasPermission(c, principal.UserID, "weighing:reprint")
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	v, e := h.service.StartPrint(c, id, in, requestEvent(c))
	writeResult(c, v, e)
}

// resolvePrint godoc
// @Summary 操作员反馈真实打印核实结果
// @Tags 称重业务
// @Security BearerAuth
// @Param id path int true "业务对象 ID"
// @Param request body PrintResultInput true "动作输入，业务重量为吨制字符串"
// @Success 200 {object} HTTPEnvelope[CommandResult]
// @Router /api/weighing/print-attempts/{id}/result [post]
func (h *Handler) resolvePrint(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var in PrintResultInput
	if c.ShouldBindJSON(&in) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	v, e := h.service.ResolvePrint(c, id, in, requestEvent(c))
	writeResult(c, v, e)
}
