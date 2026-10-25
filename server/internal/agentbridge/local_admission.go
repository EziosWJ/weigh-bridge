package agentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	ph "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

// AdmissionError carries a protocol error code that an Agent can display and
// retry against. It deliberately contains no credential or host detail.
type AdmissionError struct {
	Code    string
	Message string
}

func (e *AdmissionError) Error() string { return e.Message }

var (
	// ErrSiteNotInitialized means the local factory has no identity yet. The
	// Agent must not create a site or scale; it retries after Web initialization.
	ErrSiteNotInitialized = &AdmissionError{Code: p.ErrorSiteNotInitialized, Message: "本厂尚未完成初始化，请在网页完成厂区与地磅配置后重试"}
	// ErrScaleNotInitialized means the site identity exists but its sole scale
	// is missing or disabled.
	ErrScaleNotInitialized = &AdmissionError{Code: p.ErrorScaleNotInitialized, Message: "本厂地磅尚未配置或已停用，请检查本厂配置"}
	// ErrCapabilityMissing is returned instead of silently weakening a formal
	// capture when the connected client does not advertise the required check.
	ErrCapabilityMissing = errors.New("Agent 不支持所需的测量有效性检查，请升级设备 Agent")
)

// SetLocalResolver enables keyless admission. The resolver reads the local
// site identity and its sole enabled scale; it must not create business rows.
func (h *Hub) SetLocalResolver(fn func(context.Context) (Config, error)) {
	h.gate.Lock()
	h.local = fn
	h.gate.Unlock()
}

// localChild resolves the local site scale and returns the per-scale hub. The
// caller holds the root gate so site disable cannot race admission.
func (h *Hub) localChild(ctx context.Context) (*Hub, error) {
	if h.local == nil {
		return nil, fmt.Errorf("keyless Agent admission is not configured")
	}
	cfg, err := h.local(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.ScaleID < 1 {
		return nil, ErrScaleNotInitialized
	}
	if h.children == nil {
		if h.cfg.ScaleID != cfg.ScaleID {
			return nil, ErrScaleNotInitialized
		}
		h.mu.Lock()
		h.cfg.ID, h.cfg.SiteID, h.cfg.Key = cfg.ID, cfg.SiteID, ""
		h.mu.Unlock()
		return h, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	child := h.children[cfg.ScaleID]
	if child == nil {
		child = New(cfg)
		child.parent = h
		child.printResult = h.printResult
		child.configState = h.configState
		h.children[cfg.ScaleID] = child
	} else {
		child.mu.Lock()
		child.cfg.ID, child.cfg.SiteID, child.cfg.Key = cfg.ID, cfg.SiteID, ""
		child.mu.Unlock()
	}
	return child, nil
}

// serveLocal performs keyless admission. A rejection is sent as a normal error
// message after the WebSocket upgrade so the Agent can display the actual
// reason (uninitialized site, missing scale, conflict) instead of a bare HTTP
// handshake failure.
func (h *Hub) serveLocal(ws *websocket.Conn) {
	root := h
	if h.parent != nil {
		root = h.parent
	}
	root.gate.RLock()
	released := false
	release := func() {
		if !released {
			released = true
			root.gate.RUnlock()
		}
	}
	child, err := root.localChild(ws.Request().Context())
	if err != nil {
		release()
		code, message := admissionCode(err)
		_ = ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = websocket.JSON.Send(ws, p.NewVersion(p.ProtocolVersion, "error", ID(), "", p.Error{Code: code, Message: message}))
		_ = ws.Close()
		return
	}
	child.serve(ws, release)
}

func admissionCode(err error) (string, string) {
	var admission *AdmissionError
	if errors.As(err, &admission) {
		return admission.Code, admission.Message
	}
	return p.ErrorServerUnavailable, "本厂身份暂时不可读取，请稍后重试"
}

func (h *Hub) receiveHello(c *connection, m p.Message) error {
	var hello p.Hello
	if err := json.Unmarshal(m.Payload, &hello); err != nil {
		return errors.New("invalid agent hello")
	}
	if hello.ProtocolVersion < p.LegacyProtocolVersion || hello.ProtocolVersion > p.ProtocolVersion {
		_ = c.send(p.NewVersion(p.ProtocolVersion, "error", ID(), m.RequestID, p.Error{Code: p.ErrorAgentProtocolUnsupported, Message: "Agent 协议版本不受支持，请升级到与 Server 相同的版本"}))
		return nil
	}
	if len(hello.AgentVersion) > 60 || len(hello.Hostname) > 255 || len(hello.Capabilities) > 32 {
		return errors.New("invalid agent hello metadata")
	}
	capabilities := make([]string, 0, len(hello.Capabilities))
	seen := map[string]bool{}
	for _, capability := range hello.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" || len(capability) > 64 || seen[capability] {
			continue
		}
		seen[capability] = true
		capabilities = append(capabilities, capability)
	}
	c.protocol = hello.ProtocolVersion
	c.capabilities = seen
	c.hello = &hello
	h.state.ProtocolVersion = hello.ProtocolVersion
	h.state.AgentVersion = hello.AgentVersion
	h.state.Hostname = hello.Hostname
	h.state.Capabilities = append([]string{}, capabilities...)
	known := map[string]bool{
		p.CapabilityMeasurementWindow: true,
		p.CapabilityEffectiveConfig:   true,
		p.CapabilityCupsQueues:        true,
		p.CapabilityParameterBackup:   true,
	}
	accepted := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if known[capability] {
			accepted = append(accepted, capability)
		}
	}
	ack := p.HelloAck{
		ProtocolVersion: p.ProtocolVersion,
		SiteID:          h.cfg.SiteID,
		ScaleID:         h.cfg.ScaleID,
		AgentID:         h.cfg.ID,
		Capabilities:    accepted,
		ServerTime:      time.Now().UTC(),
	}
	_ = c.send(p.NewVersion(p.ProtocolVersion, p.TypeHelloAck, ID(), m.RequestID, ack))
	return nil
}

func (h *Hub) capabilityLocked(scale int64, capability string) bool {
	if h.conn == nil || h.conn.kicking {
		return false
	}
	return h.conn.capabilities[capability]
}

// Supports reports whether the current connection advertised a capability.
func (h *Hub) Supports(scale int64, capability string) bool {
	if h.children != nil {
		child := h.forScale(scale)
		if child == nil {
			return false
		}
		return child.Supports(scale, capability)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.capabilityLocked(scale, capability)
}

// RequireCapability is the only sanctioned gate for a formal capture. Callers
// must return its error rather than falling back to weaker validation.
func (h *Hub) RequireCapability(scale int64, capability string) error {
	if h.Supports(scale, capability) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrCapabilityMissing, capability)
}

// Capabilities returns the current connection's advertised capability names.
func (h *Hub) Capabilities(scale int64) []string {
	if h.children != nil {
		child := h.forScale(scale)
		if child == nil {
			return nil
		}
		return child.Capabilities(scale)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn == nil {
		return nil
	}
	out := make([]string, 0, len(h.conn.capabilities))
	for capability := range h.conn.capabilities {
		out = append(out, capability)
	}
	return out
}

func (h *Hub) EffectiveConfig(scale int64) (p.EffectiveConfig, bool) {
	if h.children != nil {
		child := h.forScale(scale)
		if child == nil {
			return p.EffectiveConfig{}, false
		}
		return child.EffectiveConfig(scale)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn == nil || h.effective == nil {
		return p.EffectiveConfig{}, false
	}
	return *h.effective, true
}

func (h *Hub) ConfigBackup(scale int64) (p.ConfigBackup, bool) {
	if h.children != nil {
		child := h.forScale(scale)
		if child == nil {
			return p.ConfigBackup{}, false
		}
		return child.ConfigBackup(scale)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn == nil || h.backup == nil {
		return p.ConfigBackup{}, false
	}
	return *h.backup, true
}

func (h *Hub) LastMeasurementInvalidation(scale int64) (p.MeasurementInvalidated, bool) {
	if h.children != nil {
		child := h.forScale(scale)
		if child == nil {
			return p.MeasurementInvalidated{}, false
		}
		return child.LastMeasurementInvalidation(scale)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn == nil || h.invalidated == nil {
		return p.MeasurementInvalidated{}, false
	}
	return *h.invalidated, true
}

func (h *Hub) uploadLocal(c *gin.Context) {
	child, err := h.localChild(c.Request.Context())
	if err != nil {
		code, message := admissionCode(err)
		ph.AbortError(c, http.StatusConflict, http.StatusConflict, message, map[string]string{"code": code})
		return
	}
	child.mu.Lock()
	online := child.conn != nil && !child.conn.kicking
	child.mu.Unlock()
	if !online {
		ph.AbortError(c, http.StatusConflict, http.StatusConflict, "Agent 尚未连接，无法上传图片", map[string]string{"code": p.ErrorDeviceUnavailable})
		return
	}
	c.Set("agentHub", child)
	child.upload(c)
}
