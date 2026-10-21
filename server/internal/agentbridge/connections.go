package agentbridge

import (
	"context"
	"errors"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

// Each child owns its own mutex, state and requests; no cross-scale device lock.
func NewMultiple(configs []Config) *Hub {
	h := &Hub{children: map[int64]*Hub{}}
	for _, cfg := range configs {
		h.children[cfg.ScaleID] = New(cfg)
	}
	return h
}
func (h *Hub) forScale(scale int64) *Hub {
	if h.children != nil {
		return h.children[scale]
	}
	if scale == h.cfg.ScaleID {
		return h
	}
	return nil
}

var ErrConnectionChanged = errors.New("Agent 连接已变化，请刷新后重试")

type KickResult struct {
	Disconnected  bool `json:"disconnected"`
	ExitConfirmed bool `json:"exitConfirmed"`
}

func (h *Hub) Kick(ctx context.Context, scale int64, connectionID string) (KickResult, error) {
	if h.children != nil {
		if child := h.children[scale]; child != nil {
			return child.Kick(ctx, scale, connectionID)
		}
		return KickResult{}, ErrUnavailable
	}
	h.mu.Lock()
	c := h.conn
	if c == nil || connectionID == "" || c.id != connectionID || c.kicking {
		h.mu.Unlock()
		return KickResult{}, ErrConnectionChanged
	}
	c.kicking = true
	c.kickID = ID()
	ack := make(chan struct{})
	c.kickAck = ack
	h.state.Revision++
	h.state.Online = false
	h.state.Stable = false
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = map[string]chan p.Message{}
	h.mu.Unlock()
	confirmed := false
	if err := c.send(p.New("agent.kick", ID(), c.kickID, map[string]string{"reason": "管理员踢出"})); err == nil {
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ack:
			confirmed = true
		case <-ctx.Done():
		case <-timer.C:
		}
		timer.Stop()
	}
	h.disconnect(c)
	return KickResult{Disconnected: true, ExitConfirmed: confirmed}, nil
}
