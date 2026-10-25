package weighing

import (
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerSiteConfigurationRoutes(router gin.IRouter, h *Handler) {
	manage := rbac.RequirePermission(h.permissions, "weighing:site:manage")
	view := rbac.RequireAnyPermission(h.permissions, "weighing:site:manage", "weighing:site:export")
	router.GET("/site-config", view, func(c *gin.Context) { v, e := h.service.SiteConfigurations(c); writeResult(c, v, e) })
	router.GET("/agent-settings", view, func(c *gin.Context) { v, e := h.service.AgentSettings(c); writeResult(c, v, e) })
	router.PUT("/agent-settings", manage, func(c *gin.Context) {
		var in AgentSetting
		if c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		writeResult(c, nil, h.service.SaveAgentSettings(c, in.ServerURL, requestEvent(c)))
	})
	save := func(c *gin.Context) {
		id := int64(0)
		var err error
		if c.Request.Method == "PUT" {
			id, err = pathID(c)
			if err != nil {
				writeResult(c, nil, err)
				return
			}
		}
		var in SiteConfigurationInput
		if c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		out, err := h.service.SaveSiteConfiguration(c, id, in, requestEvent(c))
		writeResult(c, out, err)
	}
	router.POST("/site-config/initialize", manage, func(c *gin.Context) {
		var in LocalSiteInput
		if c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		principal, _ := auth.PrincipalFromContext(c.Request.Context())
		v, e := h.service.InitializeLocalSite(c, principal.UserID, in, requestEvent(c))
		writeResult(c, v, e)
	})
	router.PUT("/site-config/:id", manage, save)
	router.GET("/site-config/:id/connection", rbac.RequirePermission(h.permissions, "weighing:site:export"), func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		v, e := h.service.ExportAgentConnection(c, id, c.Query("serverUrl"), requestEvent(c))
		writeResult(c, v, e)
	})
}
