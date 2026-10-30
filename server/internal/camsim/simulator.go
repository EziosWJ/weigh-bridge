package camsim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultHeartbeatInterval matches the manufacturer example and the
	// strategy value currently sent for field cameras.
	DefaultHeartbeatInterval = 30 * time.Second
	requestTimeout           = 10 * time.Second
	maxResponseBytes         = 64 << 10
)

// Config is the simulator's connection identity. ServerURL is the local
// Server root, not the event endpoint.
type Config struct {
	ServerURL         string
	SerialNumber      string
	HeartbeatInterval time.Duration
	HTTPClient        *http.Client
	Now               func() time.Time
}

// Attempt is one heartbeat or plate report result.
type Attempt struct {
	At          time.Time `json:"at"`
	DurationMS  int64     `json:"durationMs"`
	HTTPStatus  int       `json:"httpStatus,omitempty"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	ACKOperator string    `json:"ackOperator,omitempty"`
}

// Report keeps the latest recognition report so the page can explain a
// failure and retry the same event without inventing a new one.
type Report struct {
	Attempt
	EventID    string `json:"eventId"`
	Plate      string `json:"plate"`
	DeviceTime string `json:"deviceTime"`
	Attempts   int    `json:"attempts"`
	CanRetry   bool   `json:"canRetry"`
}

// State is the control page view. It intentionally excludes request bodies.
type State struct {
	Simulator           bool       `json:"simulator"`
	SerialNumber        string     `json:"serialNumber"`
	ServerURL           string     `json:"serverUrl"`
	EventURL            string     `json:"eventUrl"`
	Plate               string     `json:"plate"`
	EventID             string     `json:"eventId"`
	RecognizedAt        *time.Time `json:"recognizedAt,omitempty"`
	HeartbeatIntervalMS int64      `json:"heartbeatIntervalMs"`
	LastHeartbeat       *Attempt   `json:"lastHeartbeat,omitempty"`
	LastReport          *Report    `json:"lastReport,omitempty"`
}

type Simulator struct {
	serverURL         string
	eventURL          string
	serial            string
	heartbeatInterval time.Duration
	client            *http.Client
	now               func() time.Time

	// actionMu serializes recognition, report and retry actions so a response
	// always describes the request it belongs to.
	actionMu sync.Mutex
	mu       sync.Mutex

	plate         string
	eventID       string
	lastEventID   int64
	recognizedAt  time.Time
	lastHeartbeat *Attempt
	lastReport    *Report
	lastBody      []byte
}

func New(cfg Config) (*Simulator, error) {
	serverURL, eventURL, err := normalizeServerURL(cfg.ServerURL)
	if err != nil {
		return nil, err
	}
	serial := cfg.SerialNumber
	if serial == "" || strings.TrimSpace(serial) != serial || len(serial) > 200 || strings.ContainsAny(serial, "\x00\r\n") {
		return nil, errors.New("设备序列号不能为空、不能包含换行或控制字符，且最长 200 字节")
	}
	interval := cfg.HeartbeatInterval
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Simulator{
		serverURL: serverURL, eventURL: eventURL, serial: serial,
		heartbeatInterval: interval, client: client, now: now,
		plate: newPlate(),
	}, nil
}

func normalizeServerURL(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", fmt.Errorf("Server 地址无效：%w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", errors.New("Server 地址必须以 http:// 或 https:// 开头")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", "", errors.New("Server 地址只填写协议、主机和端口，例如 http://127.0.0.1:8900")
	}
	base := parsed.Scheme + "://" + parsed.Host
	return base, base + "/api/v1/event/pull", nil
}

func (s *Simulator) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := State{
		Simulator: true, SerialNumber: s.serial, ServerURL: s.serverURL, EventURL: s.eventURL,
		Plate: s.plate, EventID: s.eventID, HeartbeatIntervalMS: s.heartbeatInterval.Milliseconds(),
	}
	if !s.recognizedAt.IsZero() {
		at := s.recognizedAt
		state.RecognizedAt = &at
	}
	if s.lastHeartbeat != nil {
		attempt := *s.lastHeartbeat
		state.LastHeartbeat = &attempt
	}
	if s.lastReport != nil {
		report := *s.lastReport
		state.LastReport = &report
	}
	return state
}

// Run sends heartbeats immediately and then at the configured interval until
// the context is cancelled. Report actions remain available while it runs.
func (s *Simulator) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.SendHeartbeat(ctx)
			timer.Reset(s.heartbeatInterval)
		}
	}
}

// Recognize replaces the plate with a new valid plate and reports it as a new
// recognition event.
func (s *Simulator) Recognize(ctx context.Context) State {
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	s.mu.Lock()
	previous := s.plate
	plate := newPlate()
	for i := 0; i < 10 && plate == previous; i++ {
		plate = newPlate()
	}
	s.plate, s.recognizedAt = plate, s.now()
	s.mu.Unlock()
	s.sendSnapshot(ctx)
	return s.State()
}

// ReportCurrent reports the current plate again as a new recognition event.
// The plate stays the same, which exercises same-vehicle candidate merging.
func (s *Simulator) ReportCurrent(ctx context.Context) (State, error) {
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	if s.currentPlate() == "" {
		return s.State(), errors.New("当前没有可上报的车牌")
	}
	s.mu.Lock()
	s.recognizedAt = s.now()
	s.mu.Unlock()
	s.sendSnapshot(ctx)
	return s.State(), nil
}

// Retry resends the last failed event unchanged. Reusing the event ID and
// device time exercises the Server's duplicate handling instead of creating a
// new recognition.
func (s *Simulator) Retry(ctx context.Context) (State, error) {
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	s.mu.Lock()
	body := append([]byte(nil), s.lastBody...)
	report := s.lastReport
	if report != nil {
		copy := *report
		report = &copy
	}
	s.mu.Unlock()
	if len(body) == 0 || report == nil || report.Success {
		return s.State(), errors.New("没有失败的上报可重试；请使用“刷新车牌并上报”或“再次上报当前车牌”")
	}
	s.deliver(ctx, body, report.EventID, report.Plate, report.DeviceTime, report.Attempts)
	return s.State(), nil
}

func (s *Simulator) currentPlate() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.plate
}

// SendHeartbeat posts one heartbeat and records its ACK result.
func (s *Simulator) SendHeartbeat(ctx context.Context) Attempt {
	now := s.now()
	eventID := s.nextEventID(now)
	body, err := json.Marshal(heartbeatRequest{
		Operator:   "heartbeat",
		DeviceInfo: deviceInfo{SerialNumber: s.serial},
		Info: heartbeatInfo{
			EventID: json.Number(eventID),
			Time:    now.Format(time.RFC3339),
		},
	})
	attempt := Attempt{At: now}
	if err != nil {
		attempt.Error = "生成心跳报文失败：" + err.Error()
	} else {
		var data []byte
		attempt, data = s.post(ctx, body, "心跳")
		if attempt.Error == "" {
			operator, err := validateHeartbeatACK(data, eventID)
			if err != nil {
				attempt.Error = err.Error()
			} else {
				attempt.Success, attempt.ACKOperator = true, operator
			}
		}
	}
	s.mu.Lock()
	s.lastHeartbeat = &attempt
	s.mu.Unlock()
	return attempt
}

func (s *Simulator) sendSnapshot(ctx context.Context) {
	now := s.now()
	eventID := s.nextEventID(now)
	s.mu.Lock()
	plate := s.plate
	s.eventID = eventID
	s.mu.Unlock()
	deviceTime := now.Format(time.RFC3339)
	body, err := json.Marshal(snapshotRequest{
		Operator:   "CarLicenseSnapshot",
		DeviceInfo: deviceInfo{SerialNumber: s.serial},
		Info: snapshotInfo{
			EventID: json.Number(eventID),
			Time:    deviceTime,
			License: licenseAttributes{Number: plate, Color: 1},
			Car:     carAttributes{Score: 99},
		},
	})
	if err != nil {
		s.mu.Lock()
		s.lastReport = &Report{Attempt: Attempt{At: now, Error: "生成车牌上报报文失败：" + err.Error()}, EventID: eventID, Plate: plate, DeviceTime: deviceTime, Attempts: 1, CanRetry: true}
		s.mu.Unlock()
		return
	}
	s.deliver(ctx, body, eventID, plate, deviceTime, 0)
}

func (s *Simulator) deliver(ctx context.Context, body []byte, eventID, plate, deviceTime string, previousAttempts int) {
	attempt, data := s.post(ctx, body, "车牌上报")
	if attempt.Error == "" {
		operator, err := validateSnapshotACK(data, eventID)
		if err != nil {
			attempt.Error = err.Error()
		} else {
			attempt.Success, attempt.ACKOperator = true, operator
		}
	}
	s.mu.Lock()
	s.lastReport = &Report{
		Attempt: attempt, EventID: eventID, Plate: plate, DeviceTime: deviceTime,
		Attempts: previousAttempts + 1, CanRetry: !attempt.Success,
	}
	if attempt.Success {
		s.lastBody = nil
	} else {
		s.lastBody = append([]byte(nil), body...)
	}
	s.mu.Unlock()
}

func (s *Simulator) post(ctx context.Context, body []byte, action string) (Attempt, []byte) {
	attempt := Attempt{At: s.now()}
	started := time.Now()
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, s.eventURL, bytes.NewReader(body))
	if err != nil {
		attempt.Error = "创建请求失败：" + err.Error()
		return attempt, nil
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	attempt.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		attempt.Error = connectionFailure(action, err)
		return attempt, nil
	}
	defer response.Body.Close()
	attempt.HTTPStatus = response.StatusCode
	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if readErr != nil {
		attempt.Error = action + "响应读取失败：" + readErr.Error()
		return attempt, nil
	}
	if len(data) > maxResponseBytes {
		attempt.Error = action + "响应超过 64 KiB，无法确认 ACK"
		return attempt, nil
	}
	if response.StatusCode != http.StatusOK {
		attempt.Error = statusFailure(action, response.StatusCode)
		return attempt, nil
	}
	return attempt, data
}

func (s *Simulator) nextEventID(now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := now.UnixNano()
	if value <= s.lastEventID {
		value = s.lastEventID + 1
	}
	s.lastEventID = value
	return strconv.FormatInt(value, 10)
}

type deviceInfo struct {
	SerialNumber string `json:"serialNumber"`
}

type heartbeatInfo struct {
	EventID json.Number `json:"eventId"`
	Time    string      `json:"time"`
}

type heartbeatRequest struct {
	Operator   string        `json:"operator"`
	DeviceInfo deviceInfo    `json:"deviceInfo"`
	Info       heartbeatInfo `json:"info"`
}

type licenseAttributes struct {
	Number string `json:"Number"`
	Color  int    `json:"CarLicenseAttriColor"`
}

type carAttributes struct {
	Score int `json:"Score"`
}

type snapshotInfo struct {
	EventID json.Number       `json:"eventId"`
	Time    string            `json:"time"`
	License licenseAttributes `json:"carLicenseAttriInfo"`
	Car     carAttributes     `json:"carAttriInfo"`
}

type snapshotRequest struct {
	Operator   string       `json:"operator"`
	DeviceInfo deviceInfo   `json:"deviceInfo"`
	Info       snapshotInfo `json:"info"`
}

type ackEnvelope struct {
	Operator string `json:"operator"`
	Info     struct {
		EventIDUpper json.RawMessage `json:"eventID"`
		EventIDLower json.RawMessage `json:"eventId"`
	} `json:"info"`
	Result struct {
		ErrorNo     json.RawMessage `json:"errorNo"`
		Description string          `json:"description"`
	} `json:"result"`
}

func decodeACK(data []byte) (ackEnvelope, error) {
	var ack ackEnvelope
	if len(bytes.TrimSpace(data)) == 0 {
		return ack, errors.New("Server 返回 HTTP 200 但没有响应内容")
	}
	if err := json.Unmarshal(data, &ack); err != nil {
		return ack, errors.New("Server HTTP 200 响应不是有效的 ACK JSON：" + err.Error())
	}
	return ack, nil
}

func validateSnapshotACK(data []byte, eventID string) (string, error) {
	ack, err := decodeACK(data)
	if err != nil {
		return "", err
	}
	if ack.Operator != "CarLicenseSnapshot-Ack" {
		return ack.Operator, fmt.Errorf("Server 响应不是 CarLicenseSnapshot-Ack（收到 %q）", ack.Operator)
	}
	if err := ackResultError(ack); err != nil {
		return ack.Operator, err
	}
	got := ackEventID(ack.Info.EventIDUpper)
	if got == "" {
		got = ackEventID(ack.Info.EventIDLower)
	}
	if got == "" {
		return ack.Operator, errors.New("ACK 未回填事件号")
	}
	if got != eventID {
		return ack.Operator, fmt.Errorf("ACK 事件号与上报不一致（上报 %s，ACK %s）", eventID, got)
	}
	return ack.Operator, nil
}

func validateHeartbeatACK(data []byte, eventID string) (string, error) {
	ack, err := decodeACK(data)
	if err != nil {
		return "", err
	}
	if ack.Operator != "heartbeat-ack" {
		return ack.Operator, fmt.Errorf("Server 响应不是 heartbeat-ack（收到 %q）", ack.Operator)
	}
	if err := ackResultError(ack); err != nil {
		return ack.Operator, err
	}
	got := ackEventID(ack.Info.EventIDLower)
	if got == "" {
		got = ackEventID(ack.Info.EventIDUpper)
	}
	if got == "" {
		return ack.Operator, errors.New("心跳 ACK 未回填事件号")
	}
	if got != eventID {
		return ack.Operator, fmt.Errorf("心跳 ACK 事件号不一致（上报 %s，ACK %s）", eventID, got)
	}
	return ack.Operator, nil
}

func ackResultError(ack ackEnvelope) error {
	if len(bytes.TrimSpace(ack.Result.ErrorNo)) == 0 {
		return errors.New("ACK 缺少 result.errorNo")
	}
	decoder := json.NewDecoder(bytes.NewReader(ack.Result.ErrorNo))
	decoder.UseNumber()
	var value json.Number
	if err := decoder.Decode(&value); err != nil {
		return errors.New("ACK 的 result.errorNo 不是数字")
	}
	number, err := strconv.Atoi(value.String())
	if err != nil {
		return errors.New("ACK 的 result.errorNo 超出范围")
	}
	if number != 0 {
		description := strings.TrimSpace(ack.Result.Description)
		if description == "" {
			description = "厂家未提供描述"
		}
		return fmt.Errorf("厂家 ACK 失败（errorNo=%d）：%s", number, description)
	}
	return nil
}

func ackEventID(raw json.RawMessage) string {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return ""
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return text
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var number json.Number
	if decoder.Decode(&number) == nil {
		return number.String()
	}
	return ""
}

func statusFailure(action string, status int) string {
	switch status {
	case http.StatusBadRequest:
		return action + "被 Server 拒绝（HTTP 400）：事件字段、车牌或设备时间格式无效；请核对两端时钟。"
	case http.StatusUnauthorized:
		return action + "被 Server 拒绝（HTTP 401）：上报入口不接受登录凭据，请检查反向代理或网关。"
	case http.StatusForbidden:
		return action + "被 Server 拒绝（HTTP 403）：设备序列号未在摄像头管理页绑定并启用，或本机 IP 不在允许的上报来源内。"
	case http.StatusNotFound:
		return action + "路径不存在（HTTP 404）：请核对 --server 是否指向本厂 Server 根地址。"
	case http.StatusRequestEntityTooLarge:
		return action + "超出 Server 大小上限（HTTP 413）。"
	case http.StatusServiceUnavailable:
		return "Server 暂不可用（HTTP 503）：数据库或设备存储故障，或并发上报过多。"
	default:
		return fmt.Sprintf("%s失败（HTTP %d）。", action, status)
	}
}

func connectionFailure(action string, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return action + "超时：10 秒内未收到 Server 响应。"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return action + "超时：10 秒内未收到 Server 响应。"
	}
	return action + "无法连接 Server：" + err.Error()
}
