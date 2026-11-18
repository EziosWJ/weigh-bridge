package weighing

import (
	"strconv"

	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerTemporaryPartyRoutes(router gin.IRouter, h *Handler) {
	write := rbac.RequirePermission(h.permissions, "weighing:temporary-party:write")
	router.POST("/temporary-parties", write, h.saveTemporaryParty)
	router.PUT("/temporary-parties/:partyId", write, h.saveTemporaryParty)
}

func (h *Handler) saveTemporaryParty(c *gin.Context) {
	var id int64
	if value := c.Param("partyId"); value != "" {
		var err error
		id, err = strconv.ParseInt(value, 10, 64)
		if err != nil || id < 1 {
			writeResult(c, nil, ErrInvalid)
			return
		}
	}
	var input TemporaryPartyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	party, err := h.service.SaveTemporaryParty(c.Request.Context(), id, input, requestEvent(c))
	writeResult(c, party, err)
}
