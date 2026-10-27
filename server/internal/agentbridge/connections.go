package agentbridge

import (
	"context"
	"errors"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

// Each child owns its own mutex, state and requests; no cross-scaleCode device lock.
func NewMultiple(configs []Config) *Hub {
	h := &Hub{children: map[string]*Hub{}}
	for _, cfg := range configs {
		child := New(cfg)
		child.parent = h
		h.children[cfg.ScaleCode] = child
	}
	return h
}
func (h *Hub) forScale(scaleCode string) *Hub {
	if h.children != nil {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.children[scaleCode]
	}
	if scaleCode == h.cfg.ScaleCode {
		return h
	}
	return nil
}

var ErrConnectionChanged = errors.New("Agent 连接已变化，请刷新后重试")

type KickResult struct {
	Disconnected  bool `json:"disconnected"`
	ExitConfirmed bool `json:"exitConfirmed"`
}

func (h *Hub) Kick(ctx context.Context, scaleCode string, connectionID string) (KickResult, error) {
	if h.children != nil {
		if child := h.forScale(scaleCode); child != nil {
			return child.Kick(ctx, scaleCode, connectionID)
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

// SetResolver enables database-managed bindings without replacing live connections.
func (h *Hub) SetResolver(fn func(context.Context, string) (Config, error)) { h.resolver = fn }
func (h *Hub) resolve(ctx context.Context, scaleCode string) (*Hub, error) {
	if h.resolver == nil {
		return h.forScale(scaleCode), nil
	}
	cfg, err := h.resolver(ctx, scaleCode)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	child := h.children[scaleCode]
	if child == nil {
		child = New(cfg)
		child.parent = h
		child.printResult = h.printResult
		h.children[scaleCode] = child
	}
	if child.cfg.ID != cfg.ID || child.cfg.Key != cfg.Key {
		return nil, ErrUnavailable
	}
	return child, nil
}
func (h *Hub) WithConnectionsPaused(fn func() error) error {
	h.gate.Lock()
	defer h.gate.Unlock()
	return fn()
}

// DisconnectForIdentityChange closes the existing connection without sending a
// kick instruction. The caller must hold WithConnectionsPaused: the Agent may
// reconnect normally after the new local configuration commits. In-flight
// replies remain unknown and are never replayed by this operation.
func (h *Hub) DisconnectForIdentityChange(scaleCode string) {
	child := h.forScale(scaleCode)
	if child == nil {
		return
	}
	child.mu.Lock()
	conn := child.conn
	child.mu.Unlock()
	if conn != nil {
		child.disconnect(conn)
	}
}

func (h *Hub) Connected(scaleCode string) bool {
	child := h.forScale(scaleCode)
	if child == nil {
		return false
	}
	child.mu.Lock()
	defer child.mu.Unlock()
	return child.conn != nil
}
