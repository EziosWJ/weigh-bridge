package agentmaintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	ph "github.com/EziosWJ/weighbridge/server/internal/platform/http"
)

// OpenDevice keeps maintenance state on the Agent host and uses its actual
// environment file, even when --state-dir and --env-file are in different dirs.
func OpenDevice(dir, environment string, initial Config, runner Runner) (*Service, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if environment == "" {
		environment = filepath.Join(dir, "agent.env")
	}
	if _, err := os.Stat(environment); os.IsNotExist(err) {
		if err = BuildAgentEnvironmentAt(environment, initial); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err := validate(initial); err != nil {
		return nil, err
	}
	s, err := open(dir, runner, &initial, true)
	if err != nil {
		return nil, err
	}
	s.environment = environment
	s.deviceOwned = true
	if _, err = s.reloadEnvironment(); err != nil {
		return nil, err
	}
	// The operator may have explicitly re-paired agent.env while preserving
	// maintenance drafts. Keep their device parameters, but carry only the current
	// connection identity into drafts/rollback so they cannot restore an old key.
	s.mu.Lock()
	oldDraft, oldPrevious := s.draft, s.previous
	bindConnection := func(c *Config) {
		c.ServerURL, c.AgentID, c.AgentKey, c.ScaleCode = s.active.ServerURL, s.active.AgentID, s.active.AgentKey, s.active.ScaleCode
	}
	bindConnection(&s.draft)
	if s.previous.AgentID != "" {
		bindConnection(&s.previous)
	}
	if s.draft != oldDraft || s.previous != oldPrevious {
		if err := s.persist(); err != nil {
			s.draft, s.previous = oldDraft, oldPrevious
			s.mu.Unlock()
			return nil, err
		}
	}
	s.mu.Unlock()
	s.SetBusinessConfigValidator(func(_ context.Context, c Config) error {
		// Keyless Agents know only the Server address. Site, scale and Agent
		// codes are assigned by the Server at agent.hello.ack and are diagnostic
		// only (ADR 0030/0033), so importing the Server-provided configuration
		// must not be rejected for carrying them; repointing the host at a
		// different Server (or replacing a stored key) stays forbidden.
		changed := func(known, candidate string) bool { return known != "" && candidate != known }
		if changed(initial.ServerURL, c.ServerURL) || changed(initial.AgentKey, c.AgentKey) {
			return errors.New("设备维护不能修改当前 Agent 的连接地址或身份密钥")
		}
		return nil
	})
	// The Agent runtime serializes capture/print execution with device reload.
	return s, nil
}
func (s *Service) environmentPath() string {
	if s.environment != "" {
		return s.environment
	}
	return filepath.Join(s.dir, "agent.env")
}

var deviceRoutes = map[string]bool{
	"GET /status": true, "GET /jobs": true, "GET /config": true, "PUT /config": true,
	"POST /config/import": true, "POST /action": true, "POST /test-print": true,
	"POST /test-camera": true, "GET /logs": true, "GET /audit": true, "GET /camera-config": true,
}

func (s *Service) DeviceRequest(ctx context.Context, in p.MaintenanceRequest) p.MaintenanceResponse {
	if !deviceRoutes[in.Method+" "+in.Path] || in.ActorID < 1 {
		return p.MaintenanceResponse{Status: 400, Text: "不支持的设备维护请求"}
	}
	ctx = context.WithValue(ctx, businessRequestKey{}, true)
	ctx = ph.ContextWithUserID(ctx, in.ActorID)
	if in.Path == "/camera-config" {
		c, err := s.CameraConfig()
		c.AgentKey = ""
		c.CameraFrontURL, c.CameraRearURL, c.PlateRecognitionURL = "", "", ""
		data, _ := json.Marshal(map[string]any{"config": c, "ready": err == nil})
		return p.MaintenanceResponse{Status: 200, JSON: data}
	}
	req, _ := http.NewRequestWithContext(ctx, in.Method, "http://agent.local/api"+in.Path, bytes.NewReader(in.Body))
	req.URL.RawQuery = in.Query
	if in.ClientIP != "" {
		req.RemoteAddr = net.JoinHostPort(in.ClientIP, "0")
	}
	response := &bufferedPreview{headers: make(http.Header)}
	s.Handler().ServeHTTP(response, req)
	body := response.body.Bytes()
	status := response.status
	if status == 0 {
		status = 200
	}
	if len(body) > 120<<10 {
		return p.MaintenanceResponse{Status: 413, Text: "维护结果超过消息大小，请缩小日志或记录范围"}
	}
	if strings.Contains(response.Header().Get("Content-Type"), "json") && json.Valid(body) {
		return p.MaintenanceResponse{Status: status, JSON: json.RawMessage(body)}
	}
	if len(body) > 48<<10 {
		body = body[len(body)-(48<<10):]
	}
	return p.MaintenanceResponse{Status: status, Text: string(body)}
}
func samePrinter(a, b Config) bool {
	return a.Queue == b.Queue && a.PrintWidthMM == b.PrintWidthMM && a.PrintScale == b.PrintScale && a.FormLengthMM == b.FormLengthMM && a.OffsetXMM == b.OffsetXMM && a.OffsetYMM == b.OffsetYMM
}
