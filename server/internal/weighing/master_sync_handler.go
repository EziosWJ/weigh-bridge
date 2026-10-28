package weighing

import (
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerMasterSyncRoutes(router gin.IRouter, h *Handler) {
	read := rbac.RequirePermission(h.permissions, "weighing:master:read")
	router.GET("/master-sync/status", read, func(c *gin.Context) {
		status, err := h.service.MasterSyncStatus(c)
		if err == nil {
			p, _ := auth.PrincipalFromContext(c.Request.Context())
			status.IsAdmin, err = h.service.masterSyncAdmin(c, p.UserID)
		}
		writeResult(c, status, err)
	})
	router.POST("/master-sync", read, func(c *gin.Context) {
		status, err := h.service.MasterSyncStatus(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		if !status.Bound {
			p, _ := auth.PrincipalFromContext(c.Request.Context())
			admin, err := h.service.masterSyncAdmin(c, p.UserID)
			if err != nil {
				writeResult(c, nil, err)
				return
			}
			if !admin {
				writeResult(c, nil, ErrForbidden)
				return
			}
		}
		err = h.service.RunMasterSync(c, "manual")
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		status, err = h.service.MasterSyncStatus(c)
		if err == nil {
			p, _ := auth.PrincipalFromContext(c.Request.Context())
			status.IsAdmin, err = h.service.masterSyncAdmin(c, p.UserID)
		}
		writeResult(c, status, err)
	})
	admin := func(c *gin.Context) bool {
		p, _ := auth.PrincipalFromContext(c.Request.Context())
		ok, err := h.service.masterSyncAdmin(c, p.UserID)
		if err != nil {
			writeResult(c, nil, err)
			return false
		}
		if !ok {
			writeResult(c, nil, ErrForbidden)
			return false
		}
		return true
	}
	router.GET("/master-sync/logs", func(c *gin.Context) {
		if !admin(c) {
			return
		}
		rows, err := h.service.masterSyncLogs(c)
		writeResult(c, rows, err)
	})
	router.DELETE("/master-sync/logs", func(c *gin.Context) {
		if !admin(c) {
			return
		}
		err := h.service.clearMasterSyncLogs(c, requestEvent(c))
		writeResult(c, nil, err)
	})
}
