package agentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

var ErrMaintenanceUnsupported = errors.New("Agent 尚未提供远程维护能力，请升级设备 Agent")

type MaintenanceState = p.MaintenanceState

func (h *Hub) Maintenance(ctx context.Context, scaleCode string, in p.MaintenanceRequest) (p.MaintenanceResponse, error) {
	if h.children != nil {
		if child := h.forScale(scaleCode); child != nil {
			return child.Maintenance(ctx, scaleCode, in)
		}
		return p.MaintenanceResponse{}, ErrUnavailable
	}
	h.mu.Lock()
	if scaleCode != h.cfg.ScaleCode || h.conn == nil || h.conn.kicking || time.Since(h.conn.seen) > 15*time.Second {
		h.mu.Unlock()
		return p.MaintenanceResponse{}, ErrUnavailable
	}
	if h.maintenance == nil {
		h.mu.Unlock()
		return p.MaintenanceResponse{}, ErrMaintenanceUnsupported
	}
	c := h.conn
	request := ID()
	ch := make(chan p.Message, 1)
	c.pending[request] = ch
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(c.pending, request); h.mu.Unlock() }()
	if err := c.send(p.New("maintenance.request", ID(), request, in)); err != nil {
		return p.MaintenanceResponse{}, err
	}
	timer := time.NewTimer(40 * time.Second)
	defer timer.Stop()
	var m p.Message
	select {
	case value, ok := <-ch:
		if !ok {
			return p.MaintenanceResponse{}, ErrUnavailable
		}
		m = value
	case <-ctx.Done():
		return p.MaintenanceResponse{}, ctx.Err()
	case <-timer.C:
		return p.MaintenanceResponse{}, errors.New("Agent 维护结果未确认，请重新读取配置和状态核实")
	}
	var result p.MaintenanceResponse
	if m.Type != "maintenance.result" || json.Unmarshal(m.Payload, &result) != nil || result.Status < 200 || result.Status > 599 {
		return result, ErrUnavailable
	}
	return result, nil
}
func (h *Hub) MaintenanceState(scaleCode string) (MaintenanceState, error) {
	if h.children != nil {
		if child := h.forScale(scaleCode); child != nil {
			return child.MaintenanceState(scaleCode)
		}
		return MaintenanceState{}, ErrUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn == nil || h.conn.kicking || time.Since(h.conn.seen) > 15*time.Second {
		return MaintenanceState{}, ErrUnavailable
	}
	if h.maintenance == nil {
		return MaintenanceState{}, ErrMaintenanceUnsupported
	}
	return *h.maintenance, nil
}
func (h *Hub) InvalidateSamples(scaleCode string) {
	if h.children != nil {
		if child := h.forScale(scaleCode); child != nil {
			child.InvalidateSamples(scaleCode)
		}
		return
	}
	h.mu.Lock()
	h.state.Revision++
	h.mu.Unlock()
}
