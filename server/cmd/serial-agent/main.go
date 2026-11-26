// Command serial-agent continuously reads a serial QR scanner and calls the
// factory's loopback internal scan API. It has no UI, no user login, and no
// database access; the API's Unix socket file permission is its credential.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/serialport"
)

type scannerConfig struct {
	Enabled         bool   `json:"enabled"`
	Device          string `json:"device"`
	BaudRate        int    `json:"baudRate"`
	DataBits        int    `json:"dataBits"`
	StopBits        int    `json:"stopBits"`
	Parity          string `json:"parity"`
	Delimiter       string `json:"delimiter"`
	TrimPrefix      string `json:"trimPrefix"`
	TrimSuffix      string `json:"trimSuffix"`
	RetryCount      int    `json:"retryCount"`
	RetryIntervalMS int    `json:"retryIntervalMs"`
	Version         int64  `json:"version"`
}

type scanResult struct {
	Action   string `json:"action"`
	Status   string `json:"status"`
	TaskNo   string `json:"taskNo"`
	TicketNo string `json:"ticketNo"`
	Net      string `json:"net"`
	Message  string `json:"message"`
}

type heartbeatInput struct {
	AgentID        string     `json:"agentId"`
	PID            int        `json:"pid"`
	Connected      bool       `json:"connected"`
	Device         string     `json:"device"`
	ConfigVersion  int64      `json:"configVersion"`
	Scans          int64      `json:"scans"`
	Failures       int64      `json:"failures"`
	LastScanAt     *time.Time `json:"lastScanAt,omitempty"`
	LastScanResult string     `json:"lastScanResult"`
	LastError      string     `json:"lastError"`
}

type heartbeatResult struct {
	ConfigVersion int64 `json:"configVersion"`
	Enabled       bool  `json:"enabled"`
	Reload        bool  `json:"reload"`
}

type client struct {
	http *http.Client
	base string
}

func newClient(socket string) *client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
	}
	return &client{http: &http.Client{Transport: transport, Timeout: 60 * time.Second}, base: "http://unix"}
}

func (c *client) request(ctx context.Context, method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return 0, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &envelope)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || (envelope.Code != 0 && envelope.Code != http.StatusOK) {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = strings.TrimSpace(string(raw))
		}
		if message == "" {
			message = resp.Status
		}
		return resp.StatusCode, errors.New(message)
	}
	if out != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("响应格式无效：%w", err)
		}
	}
	return resp.StatusCode, nil
}

type runner struct {
	client  *client
	agentID string
	stdin   bool
	logger  *slog.Logger
	reload  chan struct{}

	mu             sync.Mutex
	cfg            scannerConfig
	appliedVersion int64
	connected      bool
	port           *serialport.Port
	reloading      bool
	scans          int64
	failures       int64
	lastScanAt     *time.Time
	lastScanResult string
	lastError      string
}

func main() {
	var socket, agentID string
	var useStdin, debug bool
	flag.StringVar(&socket, "socket", envOr("WEIGHBRIDGE_SERIAL_AGENT_SOCKET", "/tmp/weighbridge-serial-agent.sock"), "厂端内部 Unix socket 路径")
	flag.StringVar(&agentID, "id", envOr("WEIGHBRIDGE_SERIAL_AGENT_ID", hostname()), "Agent 标识")
	flag.BoolVar(&useStdin, "stdin", false, "从标准输入读取二维码，仅用于本地联调")
	flag.BoolVar(&debug, "debug", false, "输出调试日志")
	flag.Parse()

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := &runner{
		client: newClient(socket), agentID: agentID, stdin: useStdin, logger: logger,
		reload: make(chan struct{}, 1),
	}
	logger.Info("serial scanner agent started", "socket", socket, "agentId", agentID, "stdin", useStdin)
	if err := r.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("serial scanner agent stopped", "error", err)
		os.Exit(1)
	}
	logger.Info("serial scanner agent stopped")
}

func (r *runner) run(ctx context.Context) error {
	go r.heartbeatLoop(ctx)
	for {
		if ctx.Err() != nil {
			return nil
		}
		r.consume(ctx)
		if !sleepContext(ctx, 2*time.Second) {
			return nil
		}
	}
}

func (r *runner) consume(ctx context.Context) {
	r.mu.Lock()
	r.reloading = false
	r.mu.Unlock()
	cfg, err := r.fetchConfig(ctx)
	if err != nil {
		r.setError("读取扫码枪配置失败：" + err.Error())
		return
	}
	r.setConfig(cfg)
	if !cfg.Enabled {
		r.closePort()
		r.setConnected(false)
		r.logger.Info("serial scanner disabled")
		return
	}
	delimiter, err := decodeDelimiter(cfg.Delimiter)
	if err != nil {
		r.setError("扫码结束符无效：" + err.Error())
		return
	}
	var reader io.Reader
	if r.stdin {
		reader = os.Stdin
		r.setConnected(true)
		r.clearError()
	} else {
		port, err := serialport.Open(serialport.Config{
			Device: cfg.Device, BaudRate: cfg.BaudRate, DataBits: cfg.DataBits,
			StopBits: cfg.StopBits, Parity: cfg.Parity,
		})
		if err != nil {
			r.setConnected(false)
			r.setError("打开串口失败：" + err.Error())
			return
		}
		r.setPort(port)
		r.setConnected(true)
		r.clearError()
		r.logger.Info("serial port opened", "device", cfg.Device, "baud", cfg.BaudRate, "delimiter", cfg.Delimiter)
		reader = port
	}
	defer func() {
		r.closePort()
		r.setConnected(false)
	}()

	frames := make(chan string, 16)
	errs := make(chan error, 1)
	go readFrames(ctx, reader, delimiter, frames, errs)

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.reload:
			r.logger.Info("serial scanner configuration changed, reloading")
			return
		case frame := <-frames:
			r.handleFrame(ctx, cfg, frame)
		case err := <-errs:
			if ctx.Err() != nil {
				return
			}
			r.mu.Lock()
			reloading := r.reloading
			r.reloading = false
			r.mu.Unlock()
			if !reloading {
				r.setError("读取串口失败：" + err.Error())
			}
			return
		}
	}
}

func (r *runner) handleFrame(ctx context.Context, cfg scannerConfig, raw string) {
	code := trimFrame(raw, cfg.TrimPrefix, cfg.TrimSuffix)
	if code == "" {
		return
	}
	if !strings.HasPrefix(code, "fc-handoff:") {
		r.recordFailure("忽略非手机二维码内容：" + mask(code))
		return
	}
	attempts := cfg.RetryCount + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	var status int
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && !sleepContext(ctx, time.Duration(cfg.RetryIntervalMS)*time.Millisecond) {
			return
		}
		var result scanResult
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		status, lastErr = r.client.request(attemptCtx, http.MethodPost, "/api/internal/weighing/mobile-handoffs/scan", map[string]string{
			"code": code, "source": "serial_agent", "device": cfg.Device,
		}, &result)
		cancel()
		if lastErr == nil {
			r.recordSuccess(result)
			r.logger.Info("serial scan accepted", "action", result.Action, "status", result.Status, "taskNo", result.TaskNo, "ticketNo", result.TicketNo, "code", mask(code))
			return
		}
		// 4xx responses are business rejections (expired, used, wrong site,
		// conditions not met). Retrying cannot help and may apply an old QR.
		if status >= 400 && status < 500 {
			break
		}
		r.logger.Warn("serial scan attempt failed", "attempt", attempt+1, "status", status, "error", lastErr)
	}
	message := "扫码处理失败"
	if lastErr != nil {
		message += "：" + lastErr.Error()
	}
	r.recordFailure(message)
	r.logger.Error("serial scan rejected", "error", lastErr, "code", mask(code))
}

func (r *runner) fetchConfig(ctx context.Context) (scannerConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var cfg scannerConfig
	_, err := r.client.request(ctx, http.MethodGet, "/api/internal/serial-scanner/config", nil, &cfg)
	return cfg, err
}

func (r *runner) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sendHeartbeat(ctx)
		}
	}
}

func (r *runner) sendHeartbeat(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r.mu.Lock()
	in := heartbeatInput{
		AgentID: r.agentID, PID: os.Getpid(), Connected: r.connected,
		Device: r.cfg.Device, ConfigVersion: r.appliedVersion,
		Scans: r.scans, Failures: r.failures, LastScanAt: r.lastScanAt,
		LastScanResult: r.lastScanResult, LastError: r.lastError,
	}
	r.mu.Unlock()
	var result heartbeatResult
	if _, err := r.client.request(ctx, http.MethodPost, "/api/internal/serial-scanner/heartbeat", in, &result); err != nil {
		r.logger.Warn("serial scanner heartbeat failed", "error", err)
		return
	}
	if result.Reload && !r.stdin {
		r.requestReload()
	}
}

func (r *runner) requestReload() {
	r.mu.Lock()
	r.reloading = true
	port := r.port
	r.mu.Unlock()
	if port != nil {
		_ = port.Close()
	}
	select {
	case r.reload <- struct{}{}:
	default:
	}
}

func (r *runner) setConfig(cfg scannerConfig) {
	r.mu.Lock()
	r.cfg = cfg
	r.appliedVersion = cfg.Version
	r.mu.Unlock()
}

func (r *runner) setPort(port *serialport.Port) {
	r.mu.Lock()
	r.port = port
	r.mu.Unlock()
}

func (r *runner) closePort() {
	r.mu.Lock()
	port := r.port
	r.port = nil
	r.mu.Unlock()
	if port != nil {
		_ = port.Close()
	}
}

func (r *runner) setConnected(value bool) {
	r.mu.Lock()
	r.connected = value
	r.mu.Unlock()
}

func (r *runner) setError(message string) {
	r.mu.Lock()
	changed := r.lastError != message
	r.lastError = message
	r.mu.Unlock()
	if changed {
		r.logger.Error("serial scanner error", "error", message)
	}
}

func (r *runner) clearError() {
	r.mu.Lock()
	r.lastError = ""
	r.mu.Unlock()
}

func (r *runner) recordSuccess(result scanResult) {
	now := time.Now().UTC()
	label := result.ActionLabel() + "已处理"
	if result.TaskNo != "" {
		label += "，任务 " + result.TaskNo
	}
	if result.TicketNo != "" {
		label += "，磅单 " + result.TicketNo
	}
	if result.Message != "" {
		label = result.Message
	}
	r.mu.Lock()
	r.scans++
	r.lastScanAt = &now
	r.lastScanResult = label
	r.lastError = ""
	r.mu.Unlock()
}

func (r *runner) recordFailure(message string) {
	now := time.Now().UTC()
	r.mu.Lock()
	r.failures++
	r.lastScanAt = &now
	r.lastScanResult = "失败"
	r.lastError = message
	r.mu.Unlock()
}

func (s scanResult) ActionLabel() string {
	if s.Action == "first" {
		return "首磅"
	}
	if s.Action == "second" {
		return "回磅"
	}
	return "扫码"
}

func readFrames(ctx context.Context, reader io.Reader, delimiter []byte, frames chan<- string, errs chan<- error) {
	buffer := make([]byte, 1024)
	accumulator := make([]byte, 0, 1024)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			accumulator = append(accumulator, buffer[:n]...)
			for {
				index := bytes.Index(accumulator, delimiter)
				if index < 0 {
					break
				}
				frame := string(accumulator[:index])
				accumulator = accumulator[index+len(delimiter):]
				select {
				case frames <- frame:
				case <-ctx.Done():
					return
				}
			}
			if len(accumulator) > 8192 {
				accumulator = accumulator[:0]
				select {
				case errs <- errors.New("串口数据过长，已丢弃未结束帧"):
				case <-ctx.Done():
				}
				return
			}
		}
		if err != nil {
			select {
			case errs <- err:
			case <-ctx.Done():
			}
			return
		}
	}
}

func decodeDelimiter(value string) ([]byte, error) {
	if value == "" {
		return nil, errors.New("结束符不能为空")
	}
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out = append(out, value[i])
			continue
		}
		i++
		if i >= len(value) {
			return nil, errors.New("结束符转义不完整")
		}
		switch value[i] {
		case 'r':
			out = append(out, '\r')
		case 'n':
			out = append(out, '\n')
		case 't':
			out = append(out, '\t')
		case '\\':
			out = append(out, '\\')
		default:
			return nil, fmt.Errorf("不支持的转义 \\%c", value[i])
		}
	}
	if len(out) == 0 || len(out) > 8 {
		return nil, errors.New("结束符解码后须为 1～8 字节")
	}
	return out, nil
}

func trimFrame(value, prefix, suffix string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\r\n\x00")
	value = strings.TrimPrefix(value, prefix)
	value = strings.TrimSuffix(value, suffix)
	return strings.TrimSpace(value)
}

func mask(code string) string {
	code = strings.TrimSpace(code)
	if len(code) <= 20 {
		return code
	}
	return code[:20] + "…"
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "serial-agent"
	}
	return name
}
