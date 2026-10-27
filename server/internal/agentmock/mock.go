// Package agentmock implements the development-only device simulator.
package agentmock

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"golang.org/x/net/websocket"
)

//go:embed assets/* index.html
var assets embed.FS

type Config struct {
	SiteName        string
	ScaleName       string
	SiteCode        string
	Server, ID, Key string
	OnKick          func()
	ScaleCode       string
}
type Print struct {
	RequestID string `json:"requestId"`
	p.PrintRequest
	Status string `json:"status"`
}

type mockMeasurement struct {
	generation string
	basis      uint64
	weight     p.Weight
	ended      bool
	cancelled  bool
	superseded bool
	notified   bool
}
type Mock struct {
	mu                       sync.Mutex
	writes                   sync.Mutex
	cfg                      Config
	conn                     *websocket.Conn
	enabled                  bool
	online                   bool
	kicked                   bool
	err                      string
	target                   p.Weight
	current                  p.DeviceState
	upAt                     time.Time
	onScale, unstable, fault bool
	plate                    p.Plate
	images                   map[string][]byte
	imageStatus              string
	prints                   []Print
	history                  []p.DeviceState
	http                     *http.Client
	measurementRun           string
	measurementBasis         uint64
	measurementSeq           uint64
	measurementKey           string
	measurements             map[string]*mockMeasurement
}

func New(cfg Config) *Mock {
	m := &Mock{cfg: cfg, enabled: true, target: 30000, plate: p.Plate{Candidate: "晋A12345", At: time.Now().UTC()}, images: map[string][]byte{}, prints: []Print{}, history: []p.DeviceState{}, http: &http.Client{Timeout: 10 * time.Second}, measurementRun: agentbridge.ID(), measurements: map[string]*mockMeasurement{}}
	m.images["front"], _ = assets.ReadFile("assets/truck-front.png")
	m.images["rear"], _ = assets.ReadFile("assets/truck-rear.png")
	m.current = p.DeviceState{Unit: "t", DeviceOK: true}
	return m
}
func (m *Mock) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, _ := assets.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("GET /api/state", m.stateHTTP)
	mux.HandleFunc("POST /api/control", m.control)
	mux.HandleFunc("POST /api/images", m.uploadHTTP)
	mux.HandleFunc("POST /api/prints", m.printHTTP)
	return mux
}
func (m *Mock) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		m.mu.Lock()
		enabled := m.enabled
		kicked := m.kicked
		m.mu.Unlock()
		if kicked {
			return
		}
		if enabled {
			m.runConnection(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
func (m *Mock) dial(ctx context.Context) (*websocket.Conn, error) {
	base, err := url.Parse(m.cfg.Server)
	if err != nil {
		return nil, err
	}
	if base.Scheme == "https" {
		base.Scheme = "wss"
	} else if base.Scheme == "http" {
		base.Scheme = "ws"
	}
	base.Path = strings.TrimSuffix(base.Path, "/") + "/api/agent/ws"
	origin := strings.Replace(strings.Replace(m.cfg.Server, "wss:", "https:", 1), "ws:", "http:", 1)
	cfg, err := websocket.NewConfig(base.String(), origin)
	if err != nil {
		return nil, err
	}
	cfg.Header = m.headers()
	return cfg.DialContext(ctx)
}

// headers returns only the optional legacy diagnostics. Keyless servers ignore
// them; the v2 hello message is the authoritative capability handshake.
func (m *Mock) headers() http.Header {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := http.Header{}
	if m.cfg.Key != "" {
		h.Set("Authorization", "Bearer "+m.cfg.Key)
	}
	if m.cfg.ID != "" {
		h.Set("X-Agent-ID", m.cfg.ID)
	}
	if m.cfg.ScaleCode != "" {
		h.Set("X-Scale-Code", m.cfg.ScaleCode)
	}
	return h
}
func (m *Mock) send(kind, request string, payload any) error {
	return m.sendVersion(p.LegacyProtocolVersion, kind, request, payload)
}
func (m *Mock) sendVersion(version int, kind, request string, payload any) error {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("Agent 未连接")
	}
	m.writes.Lock()
	defer m.writes.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return websocket.JSON.Send(conn, p.NewVersion(version, kind, agentbridge.ID(), request, payload))
}
func (m *Mock) runConnection(ctx context.Context) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := m.dial(dialCtx)
	cancel()
	if err != nil {
		m.mu.Lock()
		m.err = err.Error()
		m.mu.Unlock()
		return
	}
	conn.MaxPayloadBytes = 1 << 20
	m.mu.Lock()
	m.conn = conn
	m.online = true
	m.err = ""
	// A new Server connection cannot reuse a previous capture basis.
	m.invalidateMeasurementsLocked()
	m.mu.Unlock()
	defer func() {
		_ = conn.Close()
		m.mu.Lock()
		if m.conn == conn {
			m.conn = nil
			m.online = false
		}
		m.invalidateMeasurementsLocked()
		m.mu.Unlock()
	}()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
			var msg p.Message
			if err := websocket.JSON.Receive(conn, &msg); err != nil {
				return
			}
			switch msg.Type {
			case p.TypeHelloAck:
				var ack p.HelloAck
				if json.Unmarshal(msg.Payload, &ack) == nil && ack.ScaleCode != "" {
					m.mu.Lock()
					m.cfg.ScaleCode, m.cfg.SiteCode, m.cfg.ID = ack.ScaleCode, ack.SiteCode, ack.AgentID
					m.mu.Unlock()
				}
			case "agent.kick":
				_ = m.send("agent.kicked", msg.RequestID, struct{}{})
				m.mu.Lock()
				m.kicked = true
				m.enabled = false
				m.mu.Unlock()
				if m.cfg.OnKick != nil {
					m.cfg.OnKick()
				}
				return
			case "capture.request":
				var request p.CaptureRequest
				if json.Unmarshal(msg.Payload, &request) == nil {
					go m.captureWithPolicy(msg.RequestID, request.RequiresImages())
				}
			case p.TypeMeasurementBeginRequest:
				var request p.MeasurementBeginRequest
				if json.Unmarshal(msg.Payload, &request) == nil {
					go m.beginMeasurement(msg.RequestID, request)
				}
			case p.TypeMeasurementEndRequest:
				var request p.MeasurementEndRequest
				if json.Unmarshal(msg.Payload, &request) == nil {
					go m.endMeasurement(msg.RequestID, request)
				}
			case p.TypeMeasurementCancelRequest:
				var request p.MeasurementCancelRequest
				if json.Unmarshal(msg.Payload, &request) == nil {
					go m.cancelMeasurement(msg.RequestID, request)
				}
			case p.TypeMeasurementVerifyRequest:
				var request p.MeasurementVerifyRequest
				if json.Unmarshal(msg.Payload, &request) == nil {
					go m.verifyMeasurement(msg.RequestID, request)
				}
			case "print.request":
				var value p.PrintRequest
				if json.Unmarshal(msg.Payload, &value) == nil {
					m.mu.Lock()
					found := false
					for _, old := range m.prints {
						if old.AttemptID == value.AttemptID {
							found = true
						}
					}
					if !found {
						m.prints = append(m.prints, Print{RequestID: msg.RequestID, PrintRequest: value, Status: "unknown"})
					}
					m.mu.Unlock()
				}
			case "error":
				m.mu.Lock()
				m.err = string(msg.Payload)
				m.mu.Unlock()
			}
		}
	}()
	_ = m.sendVersion(p.ProtocolVersion, p.TypeHello, "", p.Hello{
		ProtocolVersion: p.ProtocolVersion,
		AgentVersion:    "agent-mock",
		Hostname:        "agent-mock",
		Capabilities:    append([]string{}, p.FormalCaptureCapabilities...),
	})
	m.mu.Lock()
	plate := m.plate
	m.mu.Unlock()
	_ = m.send("plate.observed", "", plate)
	m.publishImages()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-keepalive.C:
			if err := m.send("heartbeat", "", struct{}{}); err != nil {
				return
			}
		case <-ticker.C:
			state := m.tick()
			if err := m.send("device.state", "", state); err != nil {
				return
			}
			for _, invalidation := range m.measurementInvalidations() {
				if err := m.sendVersion(p.ProtocolVersion, p.TypeMeasurementInvalidated, "", invalidation); err != nil {
					return
				}
			}
			m.mu.Lock()
			enabled := m.enabled
			m.mu.Unlock()
			if !enabled {
				return
			}
		}
	}
}
func (m *Mock) tick() p.DeviceState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tickLocked()
}

func (m *Mock) tickLocked() p.DeviceState {
	weight := p.Weight(0)
	stable := false
	if m.onScale {
		weight = m.target
		elapsed := time.Since(m.upAt)
		stable = elapsed >= 4*time.Second && !m.unstable
		if !stable {
			delta := int64(1+(elapsed.Milliseconds()/500)%7) * 17
			if int64(weight) > delta {
				weight -= p.Weight(delta)
			}
		}
	}
	m.current = p.DeviceState{Weight: weight, Unit: "t", Stable: stable, DeviceOK: !m.fault}
	if m.fault {
		m.current.Error = "模拟仪表故障"
	}
	key := fmt.Sprintf("%d|%t|%t|%s", m.current.Weight, m.current.Stable, m.current.DeviceOK, m.current.Error)
	if m.measurementKey != "" && key != m.measurementKey {
		m.measurementBasis++
	}
	m.measurementKey = key
	m.history = append(m.history, m.current)
	if len(m.history) > 40 {
		m.history = m.history[len(m.history)-40:]
	}
	return m.current
}
func (m *Mock) uploadImages() ([]p.Image, error) {
	m.mu.Lock()
	front := append([]byte{}, m.images["front"]...)
	rear := append([]byte{}, m.images["rear"]...)
	m.mu.Unlock()
	out := []p.Image{}
	for i, view := range []string{"front", "rear"} {
		data := front
		if i == 1 {
			data = rear
		}
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		_ = form.WriteField("view", view)
		part, _ := form.CreateFormFile("file", view+".png")
		_, _ = part.Write(data)
		_ = form.Close()
		req, err := http.NewRequest("POST", strings.TrimSuffix(m.cfg.Server, "/")+"/api/agent/images", &body)
		if err != nil {
			return nil, err
		}
		req.Header = m.headers()
		req.Header.Set("Content-Type", form.FormDataContentType())
		resp, err := m.http.Do(req)
		if err != nil {
			return nil, err
		}
		var envelope struct {
			Code    int     `json:"code"`
			Message string  `json:"message"`
			Data    p.Image `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&envelope)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if envelope.Code != 200 {
			return nil, fmt.Errorf("%s", envelope.Message)
		}
		out = append(out, envelope.Data)
	}
	return out, nil
}
func (m *Mock) publishImages() {
	images, err := m.uploadImages()
	if err != nil {
		m.mu.Lock()
		m.err = err.Error()
		m.mu.Unlock()
		return
	}
	_ = m.send("camera.updated", "", p.Camera{Images: images})
}
func (m *Mock) capture(request string) { m.captureWithPolicy(request, true) }
func (m *Mock) captureWithPolicy(request string, required bool) {
	m.mu.Lock()
	state := m.current
	plate := m.plate
	imageStatus := m.imageStatus
	m.mu.Unlock()
	result := p.Capture{DeviceState: state, CapturedAt: time.Now().UTC(), Plate: plate}
	if !state.Stable || !state.DeviceOK || state.Weight <= 0 {
		result.Error = "设备未稳定或故障"
	} else if imageStatus != "" {
		result.ImageStatus, result.ImageError = imageStatus, "模拟摄像头未就绪"
		result.Success = !required
		if required {
			result.Error = result.ImageError
		}
	} else {
		images, err := m.uploadImages()
		if err != nil {
			result.Error = err.Error()
			result.ImageStatus, result.ImageError = "upload_failed", result.Error
			if !required {
				result.Success = true
				result.Error = ""
			}
		} else {
			result.ImageStatus = "complete"
			result.Images = images
			result.Success = true
		}
	}
	_ = m.send("capture.result", request, result)
}
func (m *Mock) invalidateMeasurementsLocked() {
	m.measurementBasis++
	for _, window := range m.measurements {
		if !window.cancelled && !window.superseded {
			window.superseded = true
		}
	}
}

func (m *Mock) measurementStateLocked() p.MeasuredState {
	return p.MeasuredState{DeviceState: m.current, At: time.Now().UTC()}
}

func (m *Mock) beginMeasurement(requestID string, request p.MeasurementBeginRequest) {
	if request.CaptureID == "" || len(request.CaptureID) > 80 || len(request.CapturePolicyRevision) > 80 || request.CameraConfigVersion < 0 {
		_ = m.sendVersion(p.ProtocolVersion, p.TypeMeasurementBeginResult, requestID, p.MeasurementBeginResult{Accepted: false, Code: p.ErrorInvalidPayload, Message: "测量开始参数无效"})
		return
	}
	m.mu.Lock()
	state := m.tickLocked()
	if !state.DeviceOK {
		m.mu.Unlock()
		_ = m.sendVersion(p.ProtocolVersion, p.TypeMeasurementBeginResult, requestID, p.MeasurementBeginResult{Accepted: false, Code: p.ErrorDeviceFault, Message: "设备故障"})
		return
	}
	if !state.Stable {
		m.mu.Unlock()
		_ = m.sendVersion(p.ProtocolVersion, p.TypeMeasurementBeginResult, requestID, p.MeasurementBeginResult{Accepted: false, Code: p.ErrorDeviceUnstable, Message: "设备未稳定"})
		return
	}
	if state.Weight <= 0 {
		m.mu.Unlock()
		_ = m.sendVersion(p.ProtocolVersion, p.TypeMeasurementBeginResult, requestID, p.MeasurementBeginResult{Accepted: false, Code: p.ErrorDeviceUnavailable, Message: "当前重量为零"})
		return
	}
	for _, window := range m.measurements {
		if !window.cancelled {
			window.superseded = true
		}
	}
	m.measurementSeq++
	generation := fmt.Sprintf("%s-%d-%d", m.measurementRun, m.measurementBasis, m.measurementSeq)
	m.measurements[generation] = &mockMeasurement{generation: generation, basis: m.measurementBasis, weight: state.Weight}
	m.mu.Unlock()
	_ = m.sendVersion(p.ProtocolVersion, p.TypeMeasurementBeginResult, requestID, p.MeasurementBeginResult{Accepted: true, Generation: generation, State: p.MeasuredState{DeviceState: state, At: time.Now().UTC()}})
}

func (m *Mock) endMeasurement(requestID string, request p.MeasurementEndRequest) {
	m.mu.Lock()
	state := m.tickLocked()
	window := m.measurements[request.Generation]
	result := p.MeasurementEndResult{Generation: request.Generation, State: m.measurementStateLocked(), EndedAt: time.Now().UTC()}
	switch {
	case window == nil || window.cancelled:
		result.Code = p.ErrorGenerationInvalid
		result.Message = "测量代次无效"
	case window.superseded:
		result.Code = p.ErrorGenerationSuperseded
		result.Message = "测量窗口已被替代"
	case window.basis != m.measurementBasis:
		if state.Weight != window.weight {
			result.Code = p.ErrorWeightChanged
		} else {
			result.Code = p.ErrorGenerationInvalid
		}
		result.Message = "测量期间仪表状态发生变化"
	case !state.DeviceOK:
		result.Code = p.ErrorDeviceFault
		result.Message = "设备故障"
	case !state.Stable:
		result.Code = p.ErrorDeviceUnstable
		result.Message = "设备未稳定"
	case state.Weight != window.weight:
		result.Code = p.ErrorWeightChanged
		result.Message = "重量发生变化"
	default:
		window.ended = true
		result.Verified = true
	}
	m.mu.Unlock()
	_ = m.sendVersion(p.ProtocolVersion, p.TypeMeasurementEndResult, requestID, result)
}

func (m *Mock) verifyMeasurement(requestID string, request p.MeasurementVerifyRequest) {
	m.mu.Lock()
	state := m.tickLocked()
	window := m.measurements[request.Generation]
	result := p.MeasurementVerifyResult{Generation: request.Generation, State: m.measurementStateLocked(), CheckedAt: time.Now().UTC()}
	switch {
	case window == nil || !window.ended || window.cancelled:
		result.Code = p.ErrorGenerationInvalid
		result.Message = "测量代次无效或尚未结束"
	case window.superseded:
		result.Code = p.ErrorGenerationSuperseded
		result.Message = "测量窗口已被替代"
	case window.basis != m.measurementBasis:
		if state.Weight != window.weight {
			result.Code = p.ErrorWeightChanged
		} else {
			result.Code = p.ErrorGenerationInvalid
		}
		result.Message = "确认前仪表状态已变化"
	case !state.DeviceOK:
		result.Code = p.ErrorDeviceFault
		result.Message = "设备故障"
	case !state.Stable:
		result.Code = p.ErrorDeviceUnstable
		result.Message = "设备未稳定"
	case state.Weight != window.weight:
		result.Code = p.ErrorWeightChanged
		result.Message = "重量发生变化"
	default:
		result.Valid = true
	}
	m.mu.Unlock()
	_ = m.sendVersion(p.ProtocolVersion, p.TypeMeasurementVerifyResult, requestID, result)
}

func (m *Mock) cancelMeasurement(requestID string, request p.MeasurementCancelRequest) {
	m.mu.Lock()
	window := m.measurements[request.Generation]
	result := p.MeasurementCancelResult{Generation: request.Generation}
	switch {
	case window == nil:
		result.Code = p.ErrorGenerationInvalid
		result.Message = "测量代次无效"
	case window.superseded:
		result.Code = p.ErrorGenerationSuperseded
		result.Message = "测量窗口已被替代"
	default:
		window.cancelled = true
		result.Cancelled = true
	}
	m.mu.Unlock()
	_ = m.sendVersion(p.ProtocolVersion, p.TypeMeasurementCancelResult, requestID, result)
}

func (m *Mock) measurementInvalidations() []p.MeasurementInvalidated {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.current
	var out []p.MeasurementInvalidated
	for _, window := range m.measurements {
		if window.cancelled || window.superseded || window.notified {
			continue
		}
		reason := ""
		switch {
		case window.basis != m.measurementBasis:
			reason = "测量期间重量或仪表状态发生变化，本次依据已失效"
		case !state.DeviceOK:
			reason = "模拟仪表故障"
		case !state.Stable:
			reason = "仪表未稳定"
		case state.Weight != window.weight:
			reason = "重量发生变化"
		}
		if reason == "" {
			continue
		}
		window.notified = true
		out = append(out, p.MeasurementInvalidated{Generation: window.generation, Reason: reason, State: p.MeasuredState{DeviceState: state, At: time.Now().UTC()}, At: time.Now().UTC()})
	}
	return out
}

func (m *Mock) stateHTTP(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"online": m.online, "error": m.err, "agentId": m.cfg.ID, "scaleCode": m.cfg.ScaleCode, "siteName": m.cfg.SiteName, "scaleName": m.cfg.ScaleName, "state": m.current, "plate": m.plate, "prints": m.prints, "history": m.history})
}
func (m *Mock) control(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action    string   `json:"action"`
		Weight    p.Weight `json:"weight"`
		Candidate string   `json:"candidate"`
		Failed    bool     `json:"failed"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil {
		http.Error(w, "无效输入", 400)
		return
	}
	m.mu.Lock()
	switch in.Action {
	case "up":
		if in.Weight <= 0 {
			m.mu.Unlock()
			http.Error(w, "目标重量须大于零", 400)
			return
		}
		m.target = in.Weight
		m.onScale = true
		m.upAt = time.Now()
		m.unstable = false
	case "camera-disabled":
		m.imageStatus = "disabled"
	case "camera-failure":
		m.imageStatus = "capture_failed"
	case "upload-failure":
		m.imageStatus = "upload_failed"
	case "camera-ready":
		m.imageStatus = ""
	case "down":
		m.onScale = false
	case "unstable":
		m.unstable = true
	case "stable":
		m.unstable = false
		m.upAt = time.Now().Add(-5 * time.Second)
	case "fault":
		m.fault = true
	case "recover":
		m.fault = false
	case "disconnect":
		m.enabled = false
		m.invalidateMeasurementsLocked()
		if m.conn != nil {
			_ = m.conn.Close()
		}
	case "connect":
		m.enabled = true
	case "plate":
		m.plate = p.Plate{Candidate: in.Candidate, Failed: in.Failed, At: time.Now().UTC()}
	default:
		m.mu.Unlock()
		http.Error(w, "未知动作", 400)
		return
	}
	plate := m.plate
	m.mu.Unlock()
	if in.Action == "plate" {
		_ = m.send("plate.observed", "", plate)
	}
	w.WriteHeader(204)
}
func (m *Mock) uploadHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "上传失败", 400)
		return
	}
	defer r.MultipartForm.RemoveAll()
	view := r.FormValue("view")
	if view != "front" && view != "rear" {
		http.Error(w, "无效视角", 400)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "缺少图片", 400)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil || len(data) > 10<<20 {
		http.Error(w, "图片过大", 400)
		return
	}
	m.mu.Lock()
	m.images[view] = data
	m.mu.Unlock()
	images, err := m.uploadImages()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err = m.send("camera.updated", "", p.Camera{Images: images}); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}
func (m *Mock) printHTTP(w http.ResponseWriter, r *http.Request) {
	var in p.PrintResult
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil || (in.Status != "printed" && in.Status != "failed") {
		http.Error(w, "无效打印结果", 400)
		return
	}
	m.mu.Lock()
	var request string
	for _, v := range m.prints {
		if v.AttemptID == in.AttemptID {
			request = v.RequestID
		}
	}
	m.mu.Unlock()
	if request == "" {
		http.Error(w, "打印任务不存在", 404)
		return
	}
	if err := m.send("print.result", request, in); err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	m.mu.Lock()
	for i := range m.prints {
		if m.prints[i].AttemptID == in.AttemptID {
			m.prints[i].Status = in.Status
		}
	}
	m.mu.Unlock()
	w.WriteHeader(204)
}
