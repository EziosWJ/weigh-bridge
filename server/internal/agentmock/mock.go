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
	"strconv"
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
	Server, ID, Key string
	ScaleID         int64
}
type Print struct {
	RequestID string `json:"requestId"`
	p.PrintRequest
	Status string `json:"status"`
}
type Mock struct {
	mu                       sync.Mutex
	writes                   sync.Mutex
	cfg                      Config
	conn                     *websocket.Conn
	enabled                  bool
	online                   bool
	err                      string
	target                   p.Weight
	current                  p.DeviceState
	upAt                     time.Time
	onScale, unstable, fault bool
	plate                    p.Plate
	images                   map[string][]byte
	prints                   []Print
	history                  []p.DeviceState
	http                     *http.Client
}

func New(cfg Config) *Mock {
	m := &Mock{cfg: cfg, enabled: true, target: 30000, plate: p.Plate{Candidate: "晋A12345", At: time.Now().UTC()}, images: map[string][]byte{}, prints: []Print{}, history: []p.DeviceState{}, http: &http.Client{Timeout: 10 * time.Second}}
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
		m.mu.Unlock()
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
func (m *Mock) headers() http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+m.cfg.Key)
	h.Set("X-Agent-ID", m.cfg.ID)
	h.Set("X-Scale-ID", strconv.FormatInt(m.cfg.ScaleID, 10))
	return h
}
func (m *Mock) send(kind, request string, payload any) error {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("Agent 未连接")
	}
	m.writes.Lock()
	defer m.writes.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return websocket.JSON.Send(conn, p.New(kind, agentbridge.ID(), request, payload))
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
	m.mu.Unlock()
	defer func() {
		_ = conn.Close()
		m.mu.Lock()
		if m.conn == conn {
			m.conn = nil
			m.online = false
		}
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
			case "capture.request":
				m.capture(msg.RequestID)
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
func (m *Mock) capture(request string) {
	m.mu.Lock()
	state := m.current
	plate := m.plate
	m.mu.Unlock()
	result := p.Capture{DeviceState: state, CapturedAt: time.Now().UTC(), Plate: plate}
	if !state.Stable || !state.DeviceOK || state.Weight <= 0 {
		result.Error = "设备未稳定或故障"
	} else {
		images, err := m.uploadImages()
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Images = images
			result.Success = true
		}
	}
	_ = m.send("capture.result", request, result)
}
func (m *Mock) stateHTTP(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"online": m.online, "error": m.err, "agentId": m.cfg.ID, "scaleId": m.cfg.ScaleID, "state": m.current, "plate": m.plate, "prints": m.prints, "history": m.history})
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
