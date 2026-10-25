package agentbridge

import (
	"context"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

// Command sends one protocol revision 2 request on the current connection and
// waits for the first correlated reply. It deliberately performs no business
// validation: callers must check the required capability, connection revision
// and the reply's generation/version before accepting a result.
//
// A disconnect closes the pending channel, a timeout returns ErrUnavailable,
// and a reply that arrives after the connection changed is rejected. Duplicate
// or late replies cannot satisfy a later request because request IDs are
// never reused.
func (h *Hub) Command(ctx context.Context, scale int64, kind string, payload any, timeout time.Duration) (p.Message, error) {
	if h.children != nil {
		child := h.forScale(scale)
		if child == nil {
			return p.Message{}, ErrUnavailable
		}
		return child.Command(ctx, scale, kind, payload, timeout)
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	h.mu.Lock()
	if scale != h.cfg.ScaleID || h.conn == nil || h.conn.kicking || time.Since(h.conn.seen) > 15*time.Second {
		h.mu.Unlock()
		return p.Message{}, ErrUnavailable
	}
	c := h.conn
	request := ID()
	ch := make(chan p.Message, 1)
	c.pending[request] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(c.pending, request)
		h.mu.Unlock()
	}()
	if err := c.send(p.NewVersion(p.ProtocolVersion, kind, ID(), request, payload)); err != nil {
		// A socket that was closed before the read loop noticed is a
		// disconnect, not a business error: callers must retry the capture.
		return p.Message{}, ErrUnavailable
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var message p.Message
	select {
	case value, ok := <-ch:
		if !ok {
			return p.Message{}, ErrUnavailable
		}
		message = value
	case <-ctx.Done():
		return p.Message{}, ctx.Err()
	case <-timer.C:
		return p.Message{}, ErrUnavailable
	}
	h.mu.Lock()
	same := h.conn == c
	h.mu.Unlock()
	if !same {
		return p.Message{}, ErrUnavailable
	}
	return message, nil
}
