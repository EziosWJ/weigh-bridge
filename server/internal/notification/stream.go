package notification

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/gin-gonic/gin"
)

func RegisterRealtimeRoutes(public, system gin.IRouter, h *Handler, authenticator auth.Authenticator) {
	h.authenticator = authenticator
	public.GET("/announcement/events", h.publicEvents)
	system.GET("/notification/events", h.privateEvents)
}

// publicEvents godoc
// @Summary 公开公告变化 SSE（无需登录）
// @Tags 公告
// @Produce text/event-stream
// @Success 200 {string} string "SSE ready/announcement/heartbeat"
// @Router /api/public/announcement/events [get]
func (h *Handler) publicEvents(c *gin.Context) { h.stream(c, 0, ScopePublic, nil) }

// privateEvents godoc
// @Summary 当前用户公告、通知及已读变化 SSE
// @Tags 站内通知
// @Security BearerAuth
// @Produce text/event-stream
// @Success 200 {string} string "SSE ready/announcement/notification/read/heartbeat"
// @Router /api/system/notification/events [get]
func (h *Handler) privateEvents(c *gin.Context) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		return
	}
	h.stream(c, p.UserID, ScopeInternal, h.authenticator)
}

func (h *Handler) stream(c *gin.Context, userID int64, scope string, authenticator auth.Authenticator) {
	ctx := c.Request.Context()
	subscription, unsubscribe := h.service.hub.subscribe(userID)
	defer unsubscribe()
	controller := http.NewResponseController(c.Writer)
	// Disable the server's request-wide deadline only for this streaming route.
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		writeError(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-store")
	c.Header("X-Accel-Buffering", "no")
	send := func(change Change) bool {
		if authenticator != nil {
			if _, err := authenticator.Authenticate(ctx, c.GetHeader("Authorization")); err != nil {
				return false
			}
		}
		data, _ := json.Marshal(change)
		if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return false
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
			return false
		}
		if err := controller.Flush(); err != nil {
			return false
		}
		return controller.SetWriteDeadline(time.Time{}) == nil
	}
	expiry := time.NewTimer(time.Hour)
	defer expiry.Stop()
	resetExpiry := func() bool {
		if !expiry.Stop() {
			select {
			case <-expiry.C:
			default:
			}
		}
		next, err := h.service.announcements.NextExpiry(ctx, scope)
		if err != nil {
			return false
		}
		if next != nil {
			expiry.Reset(time.Until(*next))
		}
		return true
	}
	if !resetExpiry() || !send(Change{Type: "ready"}) {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	var tokenExpiry <-chan time.Time
	if p, ok := auth.PrincipalFromContext(ctx); ok {
		timer := time.NewTimer(time.Until(p.ExpiresAt))
		defer timer.Stop()
		tokenExpiry = timer.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tokenExpiry:
			return
		case change, ok := <-subscription.changes:
			if !ok || !send(change) {
				return
			}
			if change.Type == "announcement" && !resetExpiry() {
				return
			}
		case <-expiry.C:
			if !send(Change{Type: "announcement"}) || !resetExpiry() {
				return
			}
		case <-heartbeat.C:
			if !send(Change{Type: "heartbeat"}) {
				return
			}
		}
	}
}
