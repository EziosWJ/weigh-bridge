// Package agentbridge owns live Agent connections and correlated device commands.
package agentbridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"golang.org/x/net/websocket"
)

var ErrUnavailable = errors.New("Agent 离线、数据过期或设备未稳定，请重新采集")

type Config struct {
	ID       string
	ScaleID  int64
	Key      string
	ImageDir string
}
type State struct {
	p.DeviceState
	AgentID    string    `json:"agentId"`
	ScaleID    int64     `json:"scaleId"`
	Online     bool      `json:"online"`
	Revision   int64     `json:"revision,string"`
	ReceivedAt time.Time `json:"receivedAt"`
	Plate      p.Plate   `json:"plate"`
	Images     []p.Image `json:"images"`
}
type connection struct {
	ws      *websocket.Conn
	writes  sync.Mutex
	seen    time.Time
	pending map[string]chan p.Message
	prints  map[string]int64
}
type Hub struct {
	mu          sync.Mutex
	cfg         Config
	conn        *connection
	state       State
	printResult func(context.Context, int64, p.PrintResult) error
}

func New(cfg Config) *Hub {
	return &Hub{cfg: cfg, state: State{AgentID: cfg.ID, ScaleID: cfg.ScaleID, Revision: time.Now().UnixNano(), Images: []p.Image{}}}
}
func ID() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func (h *Hub) SetPrintHandler(fn func(context.Context, int64, p.PrintResult) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.printResult = fn
}
func (c *connection) send(m p.Message) error {
	c.writes.Lock()
	defer c.writes.Unlock()
	_ = c.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return websocket.JSON.Send(c.ws, m)
}
func (h *Hub) freshLocked() bool {
	return h.conn != nil && time.Since(h.conn.seen) < 15*time.Second && !h.state.ReceivedAt.IsZero() && time.Since(h.state.ReceivedAt) < 3*time.Second
}
func (h *Hub) readyLocked() bool {
	return h.freshLocked() && h.state.Stable && h.state.DeviceOK && h.state.Weight > 0
}
func (h *Hub) State(scale int64) State {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.state
	s.Online = scale == h.cfg.ScaleID && h.freshLocked()
	s.Images = append([]p.Image{}, s.Images...)
	if !s.Online {
		s.Stable = false
	}
	return s
}

// Guard serializes a business commit with device changes. The caller must release it.
func (h *Hub) Guard(scale, revision int64) (State, func(), error) {
	h.mu.Lock()
	if scale != h.cfg.ScaleID || !h.readyLocked() || (revision != 0 && h.state.Revision != revision) {
		h.mu.Unlock()
		return State{}, nil, ErrUnavailable
	}
	s := h.state
	s.Images = append([]p.Image{}, s.Images...)
	return s, h.mu.Unlock, nil
}
func (h *Hub) disconnect(c *connection) {
	h.mu.Lock()
	if h.conn == c {
		h.conn = nil
		h.state.Revision++
		h.state.Online = false
		for _, ch := range c.pending {
			close(ch)
		}
		c.pending = map[string]chan p.Message{}
	}
	h.mu.Unlock()
	_ = c.ws.Close()
}
func (h *Hub) Serve(ws *websocket.Conn) {
	c := &connection{ws: ws, seen: time.Now(), pending: map[string]chan p.Message{}, prints: map[string]int64{}}
	h.mu.Lock()
	old := h.conn
	h.conn = c
	h.state = State{AgentID: h.cfg.ID, ScaleID: h.cfg.ScaleID, Revision: h.state.Revision + 1, Images: []p.Image{}}
	if old != nil {
		for _, ch := range old.pending {
			close(ch)
		}
		old.pending = map[string]chan p.Message{}
	}
	h.mu.Unlock()
	if old != nil {
		_ = old.ws.Close()
	}
	defer h.disconnect(c)
	ws.MaxPayloadBytes = 128 << 10
	for {
		_ = ws.SetReadDeadline(time.Now().Add(15 * time.Second))
		var m p.Message
		if err := websocket.JSON.Receive(ws, &m); err != nil {
			return
		}
		if m.Version != 1 || m.MessageID == "" || len(m.MessageID) > 128 || m.OccurredAt.IsZero() {
			_ = c.send(p.New("error", ID(), m.RequestID, map[string]string{"code": "invalid_message", "message": "无效协议版本、编号或时间"}))
			continue
		}
		if err := h.receive(c, m); err != nil {
			_ = c.send(p.New("error", ID(), m.RequestID, map[string]string{"code": "invalid_payload", "message": err.Error()}))
		}
	}
}
func (h *Hub) receive(c *connection, m p.Message) error {
	h.mu.Lock()
	if h.conn != c {
		h.mu.Unlock()
		return ErrUnavailable
	}
	defer h.mu.Unlock()
	switch m.Type {
	case "device.state":
		var v p.DeviceState
		if err := json.Unmarshal(m.Payload, &v); err != nil {
			return err
		}
		if v.Unit != "t" || len(v.Error) > 2000 {
			return errors.New("invalid device state")
		}
		if !h.freshLocked() || v.Weight != h.state.Weight || v.Stable != h.state.Stable || v.DeviceOK != h.state.DeviceOK {
			h.state.Revision++
		}
		h.state.DeviceState = v
		h.state.ReceivedAt = time.Now().UTC()
		h.state.Online = true
	case "plate.observed":
		var v p.Plate
		if err := json.Unmarshal(m.Payload, &v); err != nil {
			return err
		}
		if len(v.Candidate) > 40 {
			return errors.New("invalid plate")
		}
		if v.Candidate != h.state.Plate.Candidate || v.Failed != h.state.Plate.Failed {
			h.state.Revision++
		}
		h.state.Plate = v
	case "camera.updated":
		var v p.Camera
		if err := json.Unmarshal(m.Payload, &v); err != nil {
			return err
		}
		images, err := h.resolveImages(v.Images)
		if err != nil {
			return err
		}
		h.state.Images = images
	case "capture.result":
		if ch, ok := c.pending[m.RequestID]; ok {
			ch <- m
			delete(c.pending, m.RequestID)
		}
	case "print.result":
		var result p.PrintResult
		if err := json.Unmarshal(m.Payload, &result); err != nil {
			return err
		}
		id, ok := c.prints[m.RequestID]
		if !ok || id != result.AttemptID {
			return nil
		}
		if result.Status != "printed" && result.Status != "failed" {
			return errors.New("invalid print result")
		}
		if len(result.Reason) > 2000 {
			return errors.New("print reason too long")
		}
		if h.printResult == nil {
			return errors.New("print handler unavailable")
		}
		if err := h.printResult(context.Background(), h.cfg.ScaleID, result); err != nil {
			return err
		}
		delete(c.prints, m.RequestID)
	case "heartbeat":
		_ = c.send(p.New("heartbeat", ID(), "", struct{}{}))
	default:
		return fmt.Errorf("unsupported message %q", m.Type)
	}
	c.seen = time.Now()
	return nil
}
func (h *Hub) Capture(ctx context.Context, scale int64) (p.Capture, int64, error) {
	h.mu.Lock()
	if scale != h.cfg.ScaleID || !h.readyLocked() {
		h.mu.Unlock()
		return p.Capture{}, 0, ErrUnavailable
	}
	c := h.conn
	revision := h.state.Revision
	request := ID()
	ch := make(chan p.Message, 1)
	c.pending[request] = ch
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(c.pending, request); h.mu.Unlock() }()
	if err := c.send(p.New("capture.request", ID(), request, struct{}{})); err != nil {
		return p.Capture{}, 0, err
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	var m p.Message
	select {
	case value, ok := <-ch:
		if !ok {
			return p.Capture{}, 0, ErrUnavailable
		}
		m = value
	case <-ctx.Done():
		return p.Capture{}, 0, ctx.Err()
	case <-timer.C:
		return p.Capture{}, 0, ErrUnavailable
	}
	var result p.Capture
	if err := json.Unmarshal(m.Payload, &result); err != nil {
		return result, 0, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn != c || !h.readyLocked() || h.state.Revision != revision || !result.Success || !result.Stable || !result.DeviceOK || result.Unit != "t" || result.Weight != h.state.Weight || result.CapturedAt.IsZero() {
		return result, 0, ErrUnavailable
	}
	images, err := h.resolveImages(result.Images)
	if err != nil {
		return result, 0, err
	}
	result.Images = images
	result.Plate = h.state.Plate
	return result, revision, nil
}
func (h *Hub) Print(scale int64, id int64, ticket any) error {
	h.mu.Lock()
	if scale != h.cfg.ScaleID || h.conn == nil || time.Since(h.conn.seen) > 15*time.Second {
		h.mu.Unlock()
		return ErrUnavailable
	}
	c := h.conn
	request := fmt.Sprintf("print-%d", id)
	if _, exists := c.prints[request]; exists {
		h.mu.Unlock()
		return nil
	}
	c.prints[request] = id
	h.mu.Unlock()
	return c.send(p.New("print.request", ID(), request, p.PrintRequest{AttemptID: id, Ticket: ticket}))
}
