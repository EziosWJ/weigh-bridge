package agentbridge

import (
	"bytes"
	"context"
	"encoding/json"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"image"
	_ "image/jpeg"
	"time"
)

func (h *Hub) TestCameras(ctx context.Context, scaleCode string) (p.CameraTest, error) {
	if h.children != nil {
		if child := h.forScale(scaleCode); child != nil {
			return child.TestCameras(ctx, scaleCode)
		}
		return p.CameraTest{}, ErrUnavailable
	}
	h.mu.Lock()
	if h.cfg.ScaleCode != scaleCode || h.conn == nil || h.conn.kicking || time.Since(h.conn.seen) > 15*time.Second {
		h.mu.Unlock()
		return p.CameraTest{}, ErrUnavailable
	}
	c := h.conn
	request := ID()
	ch := make(chan p.Message, 1)
	c.pending[request] = ch
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(c.pending, request); h.mu.Unlock() }()
	if err := c.send(p.New("camera.test.request", ID(), request, struct{}{})); err != nil {
		return p.CameraTest{}, err
	}
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	var m p.Message
	select {
	case value, ok := <-ch:
		if !ok {
			return p.CameraTest{}, ErrUnavailable
		}
		m = value
	case <-ctx.Done():
		return p.CameraTest{}, ctx.Err()
	case <-timer.C:
		return p.CameraTest{}, ErrUnavailable
	}
	var result p.CameraTest
	if m.Type != "camera.test.result" || json.Unmarshal(m.Payload, &result) != nil || len(result.Views) != 2 || result.At.IsZero() {
		return result, ErrUnavailable
	}
	h.mu.Lock()
	same := h.conn == c
	h.mu.Unlock()
	if !same {
		return p.CameraTest{}, ErrUnavailable
	}
	seen := map[string]bool{}
	for _, view := range result.Views {
		if seen[view.View] || (view.View != "front" && view.View != "rear") {
			return p.CameraTest{}, ErrUnavailable
		}
		seen[view.View] = true
		if view.Success {
			info, format, err := image.DecodeConfig(bytes.NewReader(view.JPEG))
			if err != nil || format != "jpeg" || len(view.JPEG) > 40<<10 || info.Width > 320 || info.Height > 180 {
				return p.CameraTest{}, ErrUnavailable
			}
		}
	}
	return result, nil
}
