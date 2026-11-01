package weighing

import (
	"strconv"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func registerSyncV4Routes(router gin.IRouter, h *Handler) {
	tickets := rbac.RequirePermission(h.permissions, "weighing:sync:tickets")
	logs := rbac.RequirePermission(h.permissions, "weighing:sync:logs")
	manage := rbac.RequirePermission(h.permissions, "weighing:sync:manage")

	router.GET("/sync/overview", tickets, func(c *gin.Context) {
		value, err := h.service.SyncOverview(c)
		writeResult(c, value, err)
	})
	router.GET("/sync/tickets", tickets, h.syncTickets)
	router.GET("/sync/tickets/:id", tickets, func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		value, err := h.service.SyncTicketDetail(c, id)
		writeResult(c, value, err)
	})
	router.POST("/sync/tickets/:id/reconcile", manage, func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		value, err := h.service.ReconcileSyncTicket(c, id, "manual")
		writeResult(c, value, err)
	})
	router.POST("/sync/reconcile", manage, func(c *gin.Context) {
		var input struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if c.ShouldBindJSON(&input) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		value, err := h.service.ReconcileSyncRange(c, input.From, input.To, "manual")
		writeResult(c, value, err)
	})
	router.POST("/sync/changes/:id/retry", manage, func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		value, err := h.service.RetrySyncChange(c, id, requestEvent(c))
		writeResult(c, value, err)
	})
	router.POST("/sync/reviews/:id/conclusion", manage, func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		var input struct {
			Conclusion string `json:"conclusion"`
		}
		if c.ShouldBindJSON(&input) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		writeResult(c, nil, h.service.RecordSyncReviewConclusion(c, id, input.Conclusion, requestEvent(c)))
	})
	router.POST("/sync/reviews/:id/recheck", manage, func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		value, err := h.service.RecheckSyncReview(c, id, requestEvent(c))
		writeResult(c, value, err)
	})
	router.GET("/sync/confirmations", manage, func(c *gin.Context) {
		value, err := h.service.LookupSyncConfirmation(c, c.Query("siteCode"), c.Query("digest"))
		writeResult(c, value, err)
	})
	router.GET("/sync/logs", logs, func(c *gin.Context) {
		rows, err := h.service.masterSyncLogs(c)
		writeResult(c, rows, err)
	})
	router.DELETE("/sync/logs", logs, func(c *gin.Context) {
		err := h.service.clearMasterSyncLogs(c, requestEvent(c))
		writeResult(c, nil, err)
	})
}

func (h *Handler) syncTickets(c *gin.Context) {
	q := SyncTicketQuery{Page: 1, PageSize: 20, Keyword: strings.TrimSpace(c.Query("keyword")), SyncStatus: c.Query("syncStatus"), ReconcileStatus: c.Query("reconcileStatus"), From: c.Query("from"), To: c.Query("to")}
	var err error
	if value := c.Query("page"); value != "" {
		q.Page, err = strconv.Atoi(value)
		if err != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
	}
	if value := c.Query("pageSize"); value != "" {
		q.PageSize, err = strconv.Atoi(value)
		if err != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
	}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 500 {
		writeResult(c, nil, ErrInvalid)
		return
	}
	if value := c.Query("protocol"); value != "" {
		q.Protocol, err = strconv.Atoi(value)
		if err != nil || (q.Protocol != 0 && q.Protocol != 3 && q.Protocol != 4) {
			writeResult(c, nil, ErrInvalid)
			return
		}
	}
	for _, value := range []string{q.SyncStatus, q.ReconcileStatus} {
		if value != "" && len(value) > 32 {
			writeResult(c, nil, ErrInvalid)
			return
		}
	}
	value, err := h.service.SyncTickets(c, q)
	writeResult(c, value, err)
}
