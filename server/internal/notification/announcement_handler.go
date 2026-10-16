package notification

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterAnnouncementRoutes(public, system gin.IRouter, h *Handler) {
	public.GET("/announcement/page", h.publicAnnouncements)
	public.GET("/announcement/:id", h.publicAnnouncement)
	system.GET("/announcement/page", h.internalAnnouncements)
	system.GET("/announcement/:id", h.internalAnnouncement)
	system.GET("/announcement-admin/page", h.adminAnnouncements)
	system.POST("/announcement", h.saveAnnouncement)
	system.PUT("/announcement/:id", h.updateAnnouncement)
	system.PUT("/announcement/:id/publish", h.publishAnnouncement)
	system.PUT("/announcement/:id/withdraw", h.withdrawAnnouncement)
}

// publicAnnouncements godoc
// @Summary 有效公开公告分页（无需登录）
// @Tags 公告
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} ApiEnvelope
// @Router /api/public/announcement/page [get]
func (h *Handler) publicAnnouncements(c *gin.Context) { h.announcementPage(c, ScopePublic, false) }

// internalAnnouncements godoc
// @Summary 有效站内公告分页
// @Tags 公告
// @Security BearerAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/announcement/page [get]
func (h *Handler) internalAnnouncements(c *gin.Context) { h.announcementPage(c, ScopeInternal, false) }

// adminAnnouncements godoc
// @Summary ADMIN 公告管理历史分页
// @Tags 公告
// @Security BearerAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/announcement-admin/page [get]
func (h *Handler) adminAnnouncements(c *gin.Context) { h.announcementPage(c, "", true) }
func (h *Handler) announcementPage(c *gin.Context, scope string, admin bool) {
	actor := int64(0)
	if admin {
		var ok bool
		actor, ok = principalID(c)
		if !ok {
			return
		}
	}
	p, z := pageParams(c)
	v, e := h.service.AnnouncementPage(c.Request.Context(), actor, PageQuery{p, z}, scope, admin)
	write(c, v, e)
}

// publicAnnouncement godoc
// @Summary 有效公开公告详情
// @Tags 公告
// @Param id path int true "公告 ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/public/announcement/{id} [get]
func (h *Handler) publicAnnouncement(c *gin.Context) { h.announcementDetail(c, ScopePublic) }

// internalAnnouncement godoc
// @Summary 有效站内公告详情
// @Tags 公告
// @Security BearerAuth
// @Param id path int true "公告 ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/announcement/{id} [get]
func (h *Handler) internalAnnouncement(c *gin.Context) { h.announcementDetail(c, ScopeInternal) }
func (h *Handler) announcementDetail(c *gin.Context, scope string) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	v, e := h.service.AnnouncementFind(c.Request.Context(), id, scope)
	write(c, v, e)
}

// saveAnnouncement godoc
// @Summary ADMIN 保存公告草稿
// @Tags 公告
// @Security BearerAuth
// @Param body body AnnouncementInput true "公告草稿"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/announcement [post]
func (h *Handler) saveAnnouncement(c *gin.Context) {
	actor, ok := principalID(c)
	if !ok {
		return
	}
	var id int64
	if c.Request.Method == http.MethodPut {
		id, ok = pathID(c)
		if !ok {
			return
		}
	}
	var in AnnouncementInput
	if c.ShouldBindJSON(&in) != nil {
		writeError(c, ErrInvalid)
		return
	}
	v, e := h.service.SaveAnnouncement(c.Request.Context(), actor, id, in)
	write(c, v, e)
}

// updateAnnouncement godoc
// @Summary ADMIN 修改公告草稿（已发布内容不可修改）
// @Tags 公告
// @Security BearerAuth
// @Param id path int true "公告 ID"
// @Param body body AnnouncementInput true "公告草稿"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/announcement/{id} [put]
func (h *Handler) updateAnnouncement(c *gin.Context) { h.saveAnnouncement(c) }

// publishAnnouncement godoc
// @Summary ADMIN 立即发布公告
// @Tags 公告
// @Security BearerAuth
// @Param id path int true "公告 ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/announcement/{id}/publish [put]
func (h *Handler) publishAnnouncement(c *gin.Context) { h.transitionAnnouncement(c, Published) }

// withdrawAnnouncement godoc
// @Summary ADMIN 撤下公告
// @Tags 公告
// @Security BearerAuth
// @Param id path int true "公告 ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/announcement/{id}/withdraw [put]
func (h *Handler) withdrawAnnouncement(c *gin.Context) { h.transitionAnnouncement(c, Withdrawn) }
func (h *Handler) transitionAnnouncement(c *gin.Context, status string) {
	actor, ok := principalID(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	writeMutation(c, h.service.TransitionAnnouncement(c.Request.Context(), actor, id, status))
}
