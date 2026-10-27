package weighing

import (
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerSiteConfigurationRoutes(router gin.IRouter, h *Handler) {
	// Identity and serial-watermark entries live on the system configuration
	// page, which is entered with system:config. Existing site-management
	// grants keep working, so either permission is sufficient here.
	manage := rbac.RequireAnyPermission(h.permissions, "system:config", "weighing:site:manage")
	view := rbac.RequireAnyPermission(h.permissions, "system:config", "weighing:site:manage", "weighing:site:export")
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
		var in SiteConfigurationInput
		if c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		out, err := h.service.SaveSiteConfiguration(c, in, requestEvent(c))
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
	router.GET("/ticket-sequences", view, func(c *gin.Context) {
		v, e := h.service.ListTicketSequences(c)
		writeResult(c, v, e)
	})
	router.PUT("/ticket-sequences", manage, func(c *gin.Context) {
		var in struct {
			BusinessDate string `json:"businessDate"`
			LastIssued   int64  `json:"lastIssued"`
		}
		if c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		out, e := h.service.SetTicketWatermark(c, in.BusinessDate, in.LastIssued, requestEvent(c))
		writeResult(c, out, e)
	})
	router.PUT("/site-config", manage, save)
	router.GET("/site-config/connection", rbac.RequirePermission(h.permissions, "weighing:site:export"), func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		v, e := h.service.ExportAgentConnection(c, c.Query("serverUrl"), requestEvent(c))
		writeResult(c, v, e)
	})
}
