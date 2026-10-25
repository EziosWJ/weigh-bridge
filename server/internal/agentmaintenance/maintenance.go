// Package agentmaintenance serves the offline, local printer maintenance UI.
package agentmaintenance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"golang.org/x/crypto/bcrypt"
)

type Config = agentprinter.Config

func Defaults() Config { return agentprinter.Defaults() }

type Result struct {
	Output string
	Err    error
}
type Runner interface{ Run(args []string) Result }

// BusinessConfigValidator binds business maintenance operations to the local
// site's immutable device identity. The legacy standalone maintenance handler
// intentionally does not use this callback.
type BusinessConfigValidator func(context.Context, Config) error

// ExecRunner permits only commands selected by the service; request data never supplies executable names.
type ExecRunner struct{ Timeout time.Duration }

func (r ExecRunner) Run(args []string) Result {
	if len(args) == 0 {
		return Result{Err: errors.New("missing fixed command")}
	}
	allowed := map[string]bool{"lpstat": true, "lpoptions": true, "cancel": true, "cupsenable": true, "systemctl": true, "lp": true, "journalctl": true}
	if !allowed[args[0]] {
		return Result{Err: errors.New("command is not allowed")}
	}
	for _, a := range args {
		if strings.ContainsAny(a, "\x00\n\r") {
			return Result{Err: errors.New("invalid argument")}
		}
	}
	t := r.Timeout
	if t == 0 {
		t = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	b, e := c.CombinedOutput()
	return Result{Output: string(b), Err: e}
}

type Audit struct {
	At      time.Time `json:"at"`
	ActorID int64     `json:"actorId,omitempty"`
	Source  string    `json:"source"`
	Session string    `json:"session"`
	Action  string    `json:"action"`
	Target  string    `json:"target,omitempty"`
	Before  any       `json:"before,omitempty"`
	After   any       `json:"after,omitempty"`
	Result  string    `json:"result"`
}
type Job struct {
	ID    string `json:"id"`
	Queue string `json:"queue"`
	User  string `json:"user,omitempty"`
	Title string `json:"title,omitempty"`
	State string `json:"state"`
}
type Service struct {
	dir                     string
	runner                  Runner
	mu                      sync.Mutex
	opMu                    sync.Mutex
	sessions                map[string]time.Time
	draft                   Config
	active                  Config
	previous                Config
	previews                map[string][]Job
	unknownPreviews         map[string]bool
	password                string
	page                    []byte
	startupError            string
	businessConfigValidator BusinessConfigValidator
	operationGuard          func(context.Context) (func(), error)
	cameraTester            func(context.Context, int64) (p.CameraTest, error)
	environment             string
	deviceOwned             bool
	configVersion           int64
	appliedAt               time.Time
}

func (s *Service) SetBusinessConfigValidator(v BusinessConfigValidator) {
	s.mu.Lock()
	s.businessConfigValidator = v
	s.mu.Unlock()
}

func (s *Service) validateBusinessConfig(r *http.Request, cfg Config) error {
	if !isBusinessRequest(r) {
		return nil
	}
	s.mu.Lock()
	v := s.businessConfigValidator
	s.mu.Unlock()
	if v == nil {
		return errors.New("本站维护身份校验未配置，无法执行此操作")
	}
	return v(r.Context(), cfg)
}

func isBusinessRequest(r *http.Request) bool {
	return r != nil && r.Context().Value(businessRequestKey{}) == true
}

func Open(dir string, runner Runner) (*Service, error) {
	return open(dir, runner, nil, false)
}

// OpenWithDraft prepares a new maintenance directory without applying device
// configuration. Existing drafts and agent.env remain authoritative.
func OpenWithDraft(dir string, runner Runner, draft Config) (*Service, error) {
	if err := validate(draft); err != nil {
		return nil, err
	}
	return open(dir, runner, &draft, false)
}

func open(dir string, runner Runner, initialDraft *Config, deviceOwned bool) (*Service, error) {
	if runner == nil {
		runner = ExecRunner{}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Service{dir: dir, runner: runner, deviceOwned: deviceOwned, sessions: map[string]time.Time{}, previews: map[string][]Job{}, unknownPreviews: map[string]bool{}}
	if b, e := os.ReadFile(filepath.Join(dir, "maintenance.json")); e == nil {
		var v struct {
			Draft    Config `json:"draft"`
			Active   Config `json:"active"`
			Config   Config `json:"config"`
			Saved    Config `json:"saved"`
			Previous      Config    `json:"previous"`
			Password      string    `json:"passwordHash"`
			ConfigVersion int64     `json:"configVersion"`
			AppliedAt     time.Time `json:"appliedAt"`
		}
		if json.Unmarshal(b, &v) != nil {
			return nil, errors.New("invalid maintenance config")
		}
		s.draft = agentprinter.NormalizeConfig(v.Draft)
		s.active = agentprinter.NormalizeConfig(v.Active)
		if s.draft.AgentID == "" {
			s.draft = agentprinter.NormalizeConfig(v.Config)
		}
		if s.active.AgentID == "" {
			s.active = agentprinter.NormalizeConfig(v.Saved)
		}
		s.previous = agentprinter.NormalizeConfig(v.Previous)
		s.password = v.Password
		s.configVersion = v.ConfigVersion
		s.appliedAt = v.AppliedAt
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if s.draft.AgentID == "" {
		s.draft = Defaults()
		imported, err := importEnvironment(filepath.Join(dir, "agent.env"), s.draft)
		if err != nil {
			s.startupError = "既有 Agent 配置导入失败，请登录检查并修正配置：" + err.Error()
		} else {
			s.draft = imported
		}
		s.active = s.draft
		if _, err := os.Stat(filepath.Join(dir, "agent.env")); os.IsNotExist(err) && initialDraft != nil {
			s.draft = *initialDraft
		}
	}
	s.draft = agentprinter.NormalizeConfig(s.draft)
	s.active = agentprinter.NormalizeConfig(s.active)
	s.previous = agentprinter.NormalizeConfig(s.previous)
	if s.password == "" && !deviceOwned {
		pw := randomToken(18)
		s.password = hashPassword(pw)
		if err := os.WriteFile(filepath.Join(dir, "bootstrap-password.txt"), []byte(pw+"\n"), 0600); err != nil {
			return nil, err
		}
		if err := s.persist(); err != nil {
			return nil, err
		}
	}
	if _, err := s.reloadEnvironment(); err != nil {
		s.startupError = "既有 Agent 配置导入失败，请登录检查并修正配置：" + err.Error()
	}
	// Existing installations already have an applied configuration but no
	// version yet. Record it once so the Server can store the first recovery
	// copy without inventing a newer version on every reconnect.
	s.mu.Lock()
	if s.appliedAt.IsZero() {
		s.appliedAt = time.Now().UTC()
		if s.configVersion < 1 {
			s.configVersion = 1
		}
		if err := s.persist(); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	s.mu.Unlock()
	s.page = []byte(pageHTML)
	return s, nil
}
func (s *Service) ResetPassword() error {
	pw := randomToken(24)
	s.mu.Lock()
	s.password = hashPassword(pw)
	s.sessions = map[string]time.Time{}
	err := s.persist()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, "bootstrap-password.txt")
	if err = os.WriteFile(path, []byte(pw+"\n"), 0600); err == nil {
		err = os.Chmod(path, 0600)
	}
	return err
}
func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
func hashPassword(p string) string {
	h, _ := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(h)
}
func (s *Service) persist() error {
	b, e := json.MarshalIndent(struct {
		Draft         Config    `json:"draft"`
		Active        Config    `json:"active"`
		Password      string    `json:"passwordHash"`
		Previous      Config    `json:"previous"`
		ConfigVersion int64     `json:"configVersion"`
		AppliedAt     time.Time `json:"appliedAt"`
	}{s.draft, s.active, s.password, s.previous, s.configVersion, s.appliedAt}, "", "  ")
	if e != nil {
		return e
	}
	return writePrivate(filepath.Join(s.dir, "maintenance.json"), b)
}
func (s *Service) audit(r *http.Request, action, target string, before, after any, err error) error {
	status := "成功"
	if err != nil {
		status = err.Error()
	}
	source, _, _ := net.SplitHostPort(r.RemoteAddr)
	if source == "" {
		source = r.RemoteAddr
	}
	session := ""
	if cookie, e := r.Cookie("maintenance_session"); e == nil {
		token := sha256.Sum256([]byte(cookie.Value))
		session = hex.EncodeToString(token[:8])
	}
	actorID, _ := platformhttp.UserIDFromContext(r.Context())
	row := Audit{At: time.Now().UTC(), ActorID: actorID, Source: source, Session: session, Action: action, Target: target, Before: redact(before), After: redact(after), Result: s.redactLog(status)}
	if text, ok := row.After.(string); ok {
		row.After = s.redactLog(text)
	}
	b, _ := json.Marshal(row)
	f, e := os.OpenFile(filepath.Join(s.dir, "maintenance.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return fmt.Errorf("维护记录保存失败：%w", e)
	}
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(append(b, '\n'))
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return fmt.Errorf("操作已执行，但维护记录保存失败：%w", e)
	}
	return nil
}
func redact(v any) any {
	if text, ok := v.(string); ok {
		return redactText(text)
	}
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"agentKey", "cameraFrontUrl", "cameraRearUrl", "plateRecognitionUrl", "password", "passwordHash"} {
		if x, ok := m[k]; ok && x != "" {
			m[k] = "••••••"
		}
	}
	return m
}

var sensitive = regexp.MustCompile(`(?i)(agent[_ -]?key|password|authorization)([=: ]+)((?:Bearer\s+)?[^\s,"']+)`)

func redactText(s string) string { return sensitive.ReplaceAllString(s, "$1$2[已脱敏]") }
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.home)
	mux.HandleFunc("/api/status", s.status)
	mux.HandleFunc("/api/login", s.login)
	mux.HandleFunc("/api/logout", s.logout)
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		s.json(w, map[string]bool{"authenticated": s.authenticated(r)})
	})
	mux.HandleFunc("GET /assets/maintenance.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(pageJS))
	})
	mux.HandleFunc("GET /assets/maintenance.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write([]byte(pageCSS))
	})
	mux.HandleFunc("/api/jobs", s.jobs)
	mux.HandleFunc("/api/config", s.configAPI)
	mux.HandleFunc("/api/config/import", s.importConnection)
	mux.HandleFunc("/api/action", s.action)
	mux.HandleFunc("/api/test-print", s.testPrint)
	mux.HandleFunc("/api/test-camera", s.testCamera)
	mux.HandleFunc("/api/logs", s.logs)
	mux.HandleFunc("/api/audit", s.auditAPI)
	return security(mux)
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			if origin != "" {
				u, e := url.Parse(origin)
				scheme := "http"
				if r.TLS != nil {
					scheme = "https"
				}
				if e != nil || !strings.EqualFold(u.Host, r.Host) || u.Scheme != scheme {
					http.Error(w, "origin denied", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Service) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.page)
}
func (s *Service) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Service) fail(w http.ResponseWriter, e error) {
	http.Error(w, s.redactLog(e.Error()), http.StatusBadRequest)
}
func (s *Service) authenticated(r *http.Request) bool {
	c, e := r.Cookie("maintenance_session")
	if e != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.sessions[c.Value]
	return ok && time.Now().Before(expiry)
}
func (s *Service) require(w http.ResponseWriter, r *http.Request) bool {
	if r.Context().Value(businessRequestKey{}) == true {
		return true
	}
	if !s.authenticated(r) {
		http.Error(w, "需要本地维护密码", http.StatusUnauthorized)
		return false
	}
	return true
}
func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&in) != nil {
		http.Error(w, "invalid", 400)
		return
	}
	s.mu.Lock()
	passwordHash := s.password
	s.mu.Unlock()
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(in.Password)) != nil {
		http.Error(w, "密码不正确", 401)
		return
	}
	token := randomToken(32)
	_ = os.Remove(filepath.Join(s.dir, "bootstrap-password.txt"))
	s.mu.Lock()
	s.sessions[token] = time.Now().Add(12 * time.Hour)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "maintenance_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: 43200})
	s.json(w, map[string]bool{"authenticated": true})
}
func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("maintenance_session"); e == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "maintenance_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	s.json(w, map[string]bool{"ok": true})
}
func (s *Service) status(w http.ResponseWriter, r *http.Request) {
	s.opMu.Lock()
	_, envErr := s.reloadEnvironment()
	s.opMu.Unlock()
	cups := s.runner.Run([]string{"lpstat", "-r"})
	agent := s.runner.Run([]string{"systemctl", "--user", "is-active", "weighbridge-agent-printer.service"})
	queues := s.runner.Run([]string{"lpstat", "-p"})
	var q []string
	for _, l := range strings.Split(queues.Output, "\n") {
		f := strings.Fields(l)
		if len(f) > 1 && f[0] == "printer" {
			q = append(q, f[1])
		}
	}
	s.mu.Lock()
	cfg := s.active
	startupError := s.startupError
	s.mu.Unlock()
	cloud := cloudStatus(filepath.Join(s.dir, "maintenance-status.json"))
	jobs := s.readJobs()
	summary := ParseJobs(jobs.Output)
	for i := range summary {
		summary[i].User = ""
		summary[i].Title = ""
	}
	agentState, cupsState := "停止／不可读取", "停止／不可读取"
	if agent.Err == nil && strings.TrimSpace(agent.Output) == "active" {
		agentState = "运行中"
	}
	if s.deviceOwned && strings.TrimSpace(agent.Output) == "inactive" {
		agentState = "已停止"
	}
	if cups.Err == nil && strings.TrimSpace(cups.Output) == "scheduler is running" {
		cupsState = "运行中"
	}
	last := lastError(agent, cups)
	if startupError != "" {
		last = startupError
	}
	if envErr != nil {
		last = "无法读取 Agent 配置：" + envErr.Error()
	}
	if _, err := os.Stat(filepath.Join(s.dir, "maintenance.pause")); err == nil {
		agentState = "维护暂停"
		last = "上次配置应用尚未结束；请登录核查并应用配置或恢复上一份配置。打印请求不会自动重提。"
	}
	if jobs.Err != nil {
		summary = []Job{}
	}
	queue := "未知／不可读取"
	if queues.Err == nil {
		queue = queueState(queues.Output, cfg.Queue)
	}
	queueDetails := []map[string]string{}
	if queues.Err == nil {
		for _, name := range q {
			queueDetails = append(queueDetails, map[string]string{"name": name, "state": queueState(queues.Output, name)})
		}
	}
	now := time.Now()
	scaleView := readScaleView(filepath.Join(s.dir, "scale-status.json"), now, agentState == "运行中")
	cameraConfigured := cfg.CameraFrontURL != "" && cfg.CameraRearURL != ""
	cameraState := "未配置"
	if cameraConfigured && agentState == "运行中" {
		cameraState = "抓拍已配置；连接状态在称重采集时确认"
	} else if cameraConfigured {
		cameraState = "已配置；Agent 未运行"
	}
	if cfg.CameraMode == "mediamtx" && !cfg.CameraEnabled {
		cameraState = "已停用（连接配置保留）"
	}
	camera := map[string]any{"frontConfigured": cfg.CameraFrontURL != "", "rearConfigured": cfg.CameraRearURL != "", "recognitionConfigured": cfg.PlateRecognitionURL != "", "captureConfigured": cameraConfigured, "state": cameraState}
	if cfg.CameraMode == "mediamtx" {
		camera["enabled"], camera["mode"] = cfg.CameraEnabled, cfg.CameraMode
		camera["frontConfigured"], camera["rearConfigured"] = cfg.CameraFrontPath != "", cfg.CameraRearPath != ""
	}
	s.json(w, map[string]any{"checkedAt": now.Format(time.RFC3339), "maintenance": "运行中", "agent": agentState, "cups": cupsState, "cloud": cloud, "server": cfg.ServerURL, "queue": cfg.Queue, "queueState": queue, "queues": nonNilStrings(q), "queueDetails": queueDetails, "jobs": nonNilJobs(summary), "jobsKnown": jobs.Err == nil, "lastError": s.redactLog(last), "agentKnown": strings.TrimSpace(agent.Output) != "", "cupsKnown": strings.TrimSpace(cups.Output) != "", "queueKnown": queues.Err == nil, "scale": scaleView, "camera": camera})
}
func cloudStatus(path string) string {
	b, e := os.ReadFile(path)
	if e != nil {
		return "未知／尚无 Agent 状态"
	}
	var v struct {
		Connected bool      `json:"connected"`
		At        time.Time `json:"at"`
	}
	if json.Unmarshal(b, &v) != nil || time.Since(v.At) > 10*time.Second || time.Until(v.At) > 5*time.Second {
		return "未知／Agent 状态已过期"
	}
	if v.Connected {
		return "已连接（Agent 心跳）"
	}
	return "断开"
}
func queueState(out, name string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "printer "+name+" ") {
			if strings.Contains(l, "disabled") || strings.Contains(l, "paused") {
				return "暂停"
			}
			return "已启用"
		}
	}
	return "未知／不可读取"
}
func lastError(a, b Result) string {
	if a.Err != nil {
		return "打印 Agent：" + strings.TrimSpace(a.Output)
	}
	if b.Err != nil {
		return "CUPS：" + strings.TrimSpace(b.Output)
	}
	return "暂无可读取错误"
}
func (s *Service) jobs(w http.ResponseWriter, r *http.Request) {
	if !s.require(w, r) {
		return
	}
	res := s.readJobs()
	jobs := ParseJobs(res.Output)
	token := randomToken(24)
	s.mu.Lock()
	s.previews[token] = jobs
	s.unknownPreviews[token] = res.Err != nil
	s.mu.Unlock()
	s.json(w, map[string]any{"jobs": jobs, "previewToken": token, "error": errorString(res.Err), "unknown": res.Err != nil, "queuesOutput": s.runner.Run([]string{"lpstat", "-p"}).Output})
}

var jobRE = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.-]*)-([0-9]+)\s+([^\s]+)\s+(.+)$`)

func ParseJobs(out string) []Job {
	jobs := []Job{}
	for _, line := range strings.Split(out, "\n") {
		m := jobRE.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) == 5 {
			jobs = append(jobs, Job{ID: m[1] + "-" + m[2], Queue: m[1], User: m[3], Title: "作业提交信息：" + m[4], State: "CUPS 未结束；纸面结果未知"})
		}
	}
	return jobs
}
func nonNilJobs(v []Job) []Job {
	if v == nil {
		return []Job{}
	}
	return v
}
func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
func errorString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
func (s *Service) configAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		if !s.require(w, r) {
			return
		}
		s.opMu.Lock()
		defer s.opMu.Unlock()
		environmentKnown, envErr := s.reloadEnvironment()
		s.mu.Lock()
		c := s.draft
		active := s.active
		hasPrevious := s.previous.AgentKey != ""
		hasKey := s.draft.AgentKey != ""
		hasActiveKey := s.active.AgentKey != ""
		keyChanged := s.draft.AgentKey != s.active.AgentKey
		saved := s.draft == s.active
		s.mu.Unlock()
		c.AgentKey = ""
		active.AgentKey = ""
		runtimeState := s.runtimeConfigState(s.activeConfig())
		if !environmentKnown {
			runtimeState = "未知／尚未载入有效的 agent.env"
		}
		hasCameraFrontURL := c.CameraFrontURL != ""
		hasCameraRearURL := c.CameraRearURL != ""
		hasPlateRecognitionURL := c.PlateRecognitionURL != ""
		cameraConfigChanged := c.CameraFrontURL != active.CameraFrontURL || c.CameraRearURL != active.CameraRearURL || c.PlateRecognitionURL != active.PlateRecognitionURL
		active.CameraFrontURL, active.CameraRearURL, active.PlateRecognitionURL = "", "", ""
		c.CameraFrontURL, c.CameraRearURL, c.PlateRecognitionURL = "", "", ""
		s.json(w, map[string]any{"config": c, "active": active, "hasPrevious": hasPrevious, "hasAgentKey": hasKey, "hasActiveAgentKey": hasActiveKey, "keyChanged": keyChanged, "hasCameraFrontUrl": hasCameraFrontURL, "hasCameraRearUrl": hasCameraRearURL, "hasPlateRecognitionUrl": hasPlateRecognitionURL, "cameraConfigChanged": cameraConfigChanged, "saved": saved, "environmentKnown": environmentKnown, "environmentError": errorString(envErr), "runtimeState": runtimeState})
		return
	}
	if r.Method != "PUT" {
		http.Error(w, "method", 405)
		return
	}
	if !s.require(w, r) {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	var c Config
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&c) != nil {
		http.Error(w, "invalid", 400)
		return
	}
	c = agentprinter.NormalizeConfig(c)
	s.mu.Lock()
	before := s.draft
	// Older clients omit the shared fields. They must not silently downgrade
	// an already migrated camera or re-enable its retired direct URLs.
	if before.CameraMode == "mediamtx" && c.CameraMode == "" {
		c.SetCameraSettings(before.CameraSettings())
	}
	if c.AgentKey == "••••••" || c.AgentKey == "" {
		c.AgentKey = before.AgentKey
	}
	if c.CameraFrontURL == "" || c.CameraFrontURL == "••••••" {
		c.CameraFrontURL = before.CameraFrontURL
	}
	if c.CameraRearURL == "" || c.CameraRearURL == "••••••" {
		c.CameraRearURL = before.CameraRearURL
	}
	if c.PlateRecognitionURL == "" || c.PlateRecognitionURL == "••••••" {
		c.PlateRecognitionURL = before.PlateRecognitionURL
	}
	c = agentprinter.NormalizeConfig(c)
	s.mu.Unlock()
	if e := validate(c); e != nil {
		s.fail(w, e)
		return
	}
	if e := s.validateBusinessConfig(r, c); e != nil {
		s.fail(w, e)
		return
	}
	s.mu.Lock()
	s.draft = c
	e := s.persist()
	if e != nil {
		s.draft = before
	}
	s.mu.Unlock()
	e = errors.Join(e, s.audit(r, "配置保存", c.Queue, before, c, e))
	if e != nil {
		s.fail(w, e)
		return
	}
	s.json(w, map[string]bool{"saved": true})
}
func validate(c Config) error { return agentprinter.ValidateConfig(c) }

func (s *Service) activeConfig() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *Service) action(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	if !s.require(w, r) {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	// A valid external edit is picked up before choosing operational queues.
	// Invalid files may still be repaired by applying a saved draft.
	_, _ = s.reloadEnvironment()
	var in struct {
		Action        string   `json:"action"`
		JobIDs        []string `json:"jobIds"`
		Disposition   string   `json:"disposition"`
		PreviewToken  string   `json:"previewToken"`
		PreviewAction string   `json:"previewAction"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil {
		http.Error(w, "invalid", 400)
		return
	}
	var result Result
	if in.Action == "apply" || in.Action == "restore" {
		if s.operationGuard != nil {
			release, err := s.operationGuard(r.Context())
			if err != nil {
				s.fail(w, err)
				return
			}
			defer release()
		}
		s.mu.Lock()
		before, cfg := s.active, s.draft
		if in.Action == "restore" {
			cfg = s.previous
		}
		s.mu.Unlock()
		if cfg.ServerURL == "" {
			s.fail(w, errors.New("尚无可恢复的上一份配置"))
			return
		}
		if cfg.CameraMode == "mediamtx" && s.operationGuard == nil && !s.deviceOwned {
			s.fail(w, errors.New("共享摄像头配置须通过统一业务维护入口应用，以核实在途称重请求"))
			return
		}
		if err := validate(cfg); err != nil {
			s.fail(w, err)
			return
		}
		if err := s.validateBusinessConfig(r, cfg); err != nil {
			s.fail(w, err)
			return
		}
		skipPrintChecks := s.deviceOwned && samePrinter(before, cfg)
		var current []Job
		if !skipPrintChecks {
			queue := s.runner.Run([]string{"lpstat", "-p", cfg.Queue})
			if queue.Err != nil || queueState(queue.Output, cfg.Queue) == "未知／不可读取" {
				s.fail(w, errors.New("所选 CUPS 队列不存在或不可读取，请先检查打印服务与已安装队列"))
				return
			}
			jobs := s.readJobs()
			if jobs.Err != nil {
				s.fail(w, fmt.Errorf("无法检查队列作业，配置未应用：%w", jobs.Err))
				return
			}
			current = jobsForQueues(ParseJobs(jobs.Output), before.Queue, cfg.Queue)
			if len(current) > 0 {
				if !s.previewMatches(in.PreviewToken, current) {
					s.offerPreview(w, current)
					return
				}
				if in.Disposition != "cancel" {
					http.Error(w, "配置应用前必须完成或明确取消未结束作业；保留作业时暂不应用", http.StatusConflict)
					return
				}
			}
		}
		priorState := s.runner.Run([]string{"systemctl", "--user", "is-active", "weighbridge-agent-printer.service"})
		wasRunning := strings.TrimSpace(priorState.Output) == "active"
		wasStopped := false
		if e := s.runner.Run([]string{"systemctl", "--user", "stop", "weighbridge-agent-printer.service"}); e.Err != nil {
			s.fail(w, e.Err)
			return
		} else {
			wasStopped = true
			_ = os.Remove(filepath.Join(s.dir, "maintenance-status.json"))
		}
		client := w
		response := &bufferedPreview{headers: make(http.Header)}
		w = response
		defer func() {
			if wasStopped && wasRunning {
				resumed := s.runner.Run([]string{"systemctl", "--user", "start", "weighbridge-agent-printer.service"})
				if resumed.Err == nil {
					resumed.Err = s.verifyAgent()
				}
				if resumed.Err != nil {
					err := fmt.Errorf("配置操作未完成，恢复原打印 Agent 失败：%w", resumed.Err)
					_ = s.audit(r, "恢复原打印 Agent", "weighbridge-agent-printer.service", nil, resumed.Output, err)
					http.Error(client, s.redactLog(err.Error()), http.StatusServiceUnavailable)
					return
				}
			}
			response.flush(client)
		}()
		if !skipPrintChecks {
			rechecked := s.readJobs()
			if rechecked.Err != nil {
				s.fail(w, fmt.Errorf("Agent 已暂停，但无法复核 CUPS 作业；未更改配置：%w", rechecked.Err))
				return
			}
			currentAfterStop := jobsForQueues(ParseJobs(rechecked.Output), before.Queue, cfg.Queue)
			if !sameJobs(current, currentAfterStop) {
				s.offerPreview(w, currentAfterStop)
				return
			}
			current = currentAfterStop
			if in.Disposition == "cancel" {
				if len(in.JobIDs) == 0 || !subset(in.JobIDs, current) {
					s.fail(w, errors.New("请选择预览中仍存在的作业"))
					return
				}
				for _, id := range in.JobIDs {
					canceled := s.cancelJob(r, id)
					if canceled.Err != nil {
						s.fail(w, canceled.Err)
						return
					}
				}
				check := s.readJobs()
				if check.Err != nil {
					s.fail(w, check.Err)
					return
				}
				if remaining := jobsForQueues(ParseJobs(check.Output), before.Queue, cfg.Queue); len(remaining) != 0 {
					http.Error(w, "仍有未结束作业，配置未应用；请先处理剩余作业", http.StatusConflict)
					return
				}
			}
		}
		pausePath := filepath.Join(s.dir, "maintenance.pause")
		_, pauseErr := os.Stat(pausePath)
		hadPause := pauseErr == nil
		if e := writePrivate(pausePath, []byte("paused\n")); e != nil {
			s.fail(w, e)
			return
		}
		activationSucceeded := false
		keepPaused := false
		defer func() {
			if (!hadPause && !keepPaused) || activationSucceeded {
				_ = os.Remove(pausePath)
			}
		}()
		var safeToResume bool
		result, safeToResume = s.activateConfig(cfg, in.Action == "restore")
		if !safeToResume {
			keepPaused = true
			wasStopped = false
		}
		e := result.Err
		if e == nil {
			activationSucceeded = true
			wasStopped = false
			s.mu.Lock()
			s.startupError = ""
			s.mu.Unlock()
		}
		actionName := "应用配置"
		if in.Action == "restore" {
			actionName = "恢复上一份配置"
		}
		e = errors.Join(e, s.audit(r, actionName, cfg.Queue, before, cfg, e))
		if e != nil {
			s.fail(w, e)
			return
		}
		s.json(w, map[string]any{"ok": true, "output": s.redactLog(result.Output)})
		return
	}
	if in.Action == "start-agent" || in.Action == "restart-agent" {
		if err := s.validateBusinessConfig(r, s.activeConfig()); err != nil {
			s.fail(w, err)
			return
		}
	}
	switch in.Action {
	case "start-agent":
		result = s.runner.Run([]string{"systemctl", "--user", "start", "weighbridge-agent-printer.service"})
	case "stop-agent":
		var handled bool
		result, handled = s.runRecovery(w, r, in.Action, in.PreviewToken, in.Disposition, in.JobIDs)
		if !handled {
			return
		}
	case "restart-agent":
		var handled bool
		result, handled = s.runRecovery(w, r, in.Action, in.PreviewToken, in.Disposition, in.JobIDs)
		if !handled {
			return
		}
	case "restart-cups":
		var handled bool
		result, handled = s.runRecovery(w, r, in.Action, in.PreviewToken, in.Disposition, in.JobIDs)
		if !handled {
			return
		}
	case "enable-queue":
		var handled bool
		result, handled = s.runRecovery(w, r, in.Action, in.PreviewToken, in.Disposition, in.JobIDs)
		if !handled {
			return
		}
	case "cancel-jobs":
		if len(in.JobIDs) == 0 {
			s.fail(w, errors.New("请选择要取消的作业"))
			return
		}
		current := s.readJobs()
		if current.Err != nil {
			s.fail(w, current.Err)
			return
		}
		if !subset(in.JobIDs, ParseJobs(current.Output)) {
			s.fail(w, errors.New("选中的作业已不存在，请刷新后核对"))
			return
		}
		for _, id := range in.JobIDs {
			if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*-[0-9]+$`).MatchString(id) {
				s.fail(w, errors.New("无效作业编号"))
				return
			}
			result = s.cancelJob(r, id)
			if result.Err != nil {
				break
			}
		}
	default:
		s.fail(w, errors.New("不支持的维护动作"))
		return
	}
	if result.Err == nil {
		switch in.Action {
		case "start-agent", "restart-agent":
			result.Err = s.verifyAgent()
		case "stop-agent":
			result.Err = s.verifyAgentStopped()
		case "restart-cups":
			result.Err = s.verifyCUPS()
		case "enable-queue":
			s.mu.Lock()
			queue := s.active.Queue
			s.mu.Unlock()
			result.Err = s.verifyQueue(queue)
		case "cancel-jobs":
			check := s.readJobs()
			if check.Err != nil {
				result.Err = check.Err
			} else {
				for _, job := range ParseJobs(check.Output) {
					if contains(in.JobIDs, job.ID) {
						result.Err = fmt.Errorf("作业仍在队列：%s", job.ID)
					}
				}
			}
		}
	}
	target := "weighbridge-agent-printer.service"
	if in.Action == "restart-cups" {
		target = "cups.service"
	}
	if in.Action == "enable-queue" {
		s.mu.Lock()
		target = s.active.Queue
		s.mu.Unlock()
	}
	if in.Action == "cancel-jobs" {
		target = strings.Join(in.JobIDs, ", ")
	}
	result.Err = errors.Join(result.Err, s.audit(r, in.Action, target, nil, result.Output, result.Err))
	if result.Err != nil {
		s.fail(w, result.Err)
		return
	}
	s.json(w, map[string]any{"ok": true, "output": s.redactLog(result.Output)})
}
func (s *Service) verifyAgent() error {
	r := s.runner.Run([]string{"systemctl", "--user", "is-active", "weighbridge-agent-printer.service"})
	if r.Err != nil || strings.TrimSpace(r.Output) != "active" {
		return fmt.Errorf("打印 Agent 启动后未报告 active：%s", redactText(strings.TrimSpace(r.Output)))
	}
	return nil
}
func (s *Service) verifyAgentStopped() error {
	r := s.runner.Run([]string{"systemctl", "--user", "is-active", "weighbridge-agent-printer.service"})
	if strings.TrimSpace(r.Output) != "inactive" {
		return fmt.Errorf("打印 Agent 停止后未确认 inactive：%s", redactText(strings.TrimSpace(r.Output)))
	}
	return nil
}
func (s *Service) verifyCUPS() error {
	r := s.runner.Run([]string{"lpstat", "-r"})
	if r.Err != nil || strings.TrimSpace(r.Output) != "scheduler is running" {
		return fmt.Errorf("CUPS 重启后未确认 scheduler running：%s", redactText(strings.TrimSpace(r.Output)))
	}
	return nil
}
func (s *Service) verifyQueue(name string) error {
	r := s.runner.Run([]string{"lpstat", "-p", name})
	if r.Err != nil || queueState(r.Output, name) != "已启用" {
		return fmt.Errorf("队列 %s 未确认已启用：%s", name, redactText(strings.TrimSpace(r.Output)))
	}
	return nil
}
func contains(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
func subset(ids []string, jobs []Job) bool {
	for _, id := range ids {
		found := false
		for _, j := range jobs {
			if id == j.ID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func sameJobs(a, b []Job) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		found := false
		for _, y := range b {
			if x.ID == y.ID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func jobsForQueues(jobs []Job, queues ...string) []Job {
	result := []Job{}
	for _, job := range jobs {
		if contains(queues, job.Queue) {
			result = append(result, job)
		}
	}
	return result
}
func (s *Service) offerPreview(w http.ResponseWriter, jobs []Job) {
	token := randomToken(24)
	s.mu.Lock()
	s.previews[token] = jobs
	s.unknownPreviews[token] = false
	s.mu.Unlock()
	s.json(w, map[string]any{"needsDisposition": true, "jobs": jobs, "previewToken": token})
}
func (s *Service) previewMatches(token string, current []Job) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.previews[token]
	delete(s.previews, token)
	delete(s.unknownPreviews, token)
	return ok && sameJobs(old, current)
}
func (s *Service) requirePreview(w http.ResponseWriter, req *http.Request, token, disposition string, selected []string) bool {
	s.mu.Lock()
	original, exists := s.previews[token]
	wasUnknown := s.unknownPreviews[token]
	delete(s.previews, token)
	delete(s.unknownPreviews, token)
	s.mu.Unlock()
	if !exists {
		http.Error(w, "请先查看当前作业并确认", 409)
		return false
	}
	r := s.readJobs()
	if r.Err != nil {
		if disposition == "unknown-confirmed" && wasUnknown {
			return true
		}
		http.Error(w, "无法读取队列；请明确确认状态未知后再恢复", 409)
		return false
	}
	current := ParseJobs(r.Output)
	if wasUnknown || !sameJobs(original, current) {
		s.offerPreview(w, current)
		return false
	}
	if disposition != "keep" && disposition != "cancel" {
		http.Error(w, "请明确选择保留或取消作业", http.StatusConflict)
		return false
	}
	if len(current) > 0 && disposition == "cancel" {
		if len(selected) == 0 || !subset(selected, current) {
			http.Error(w, "请明确选择要取消的作业", 400)
			return false
		}
		for _, id := range selected {
			x := s.cancelJob(req, id)
			if x.Err != nil {
				http.Error(w, x.Err.Error(), 502)
				return false
			}
		}
		check := s.readJobs()
		if check.Err != nil {
			http.Error(w, check.Err.Error(), 503)
			return false
		}
		for _, j := range ParseJobs(check.Output) {
			if contains(selected, j.ID) {
				http.Error(w, "作业仍未取消："+j.ID, 409)
				return false
			}
		}
	} else if len(current) > 0 && disposition != "keep" {
		s.offerPreview(w, current)
		return false
	}
	return true
}
func (s *Service) testPrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	if !s.require(w, r) {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	q, c := s.active.Queue, s.active
	s.mu.Unlock()
	sample := agentprinter.CalibrationSheet(c)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	rendered, renderErr := agentprinter.Render(ctx, sample)
	if renderErr != nil {
		s.audit(r, "校准测试打印", q, nil, "样张渲染失败", renderErr)
		s.fail(w, renderErr)
		return
	}
	tmp, writeErr := os.CreateTemp(s.dir, ".calibration-*.prn")
	if writeErr != nil {
		s.fail(w, writeErr)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	_, writeErr = tmp.Write(rendered)
	closeErr := tmp.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		s.fail(w, writeErr)
		return
	}
	result := s.runner.Run([]string{"lp", "-d", q, "-o", "raw", "-n", "1", "-t", "weighbridge-CALIBRATION-TEST", tmpPath})
	result.Err = errors.Join(result.Err, s.audit(r, "校准测试打印", q, nil, "仅提交固定测试样张；纸面需现场核实："+result.Output, result.Err))
	if result.Err != nil {
		s.fail(w, result.Err)
		return
	}
	s.json(w, map[string]any{"submitted": true, "paperVerified": false, "message": "已提交一份校准测试样张；CUPS 接收不代表纸面已核实。当前参数仅在 Agent 应用后用于业务单据渲染。", "widthMm": c.PrintWidthMM, "printScale": c.Calibration().Scale, "output": c.Calibration().Geometry(), "offsetXmm": c.OffsetXMM, "offsetYmm": c.OffsetYMM, "formLengthMm": c.FormLengthMM})
}
func (s *Service) logs(w http.ResponseWriter, r *http.Request) {
	if !s.require(w, r) {
		return
	}
	x := s.runner.Run([]string{"journalctl", "--user", "-u", "weighbridge-agent-printer.service", "-n", "200", "--no-pager"})
	if x.Err != nil {
		http.Error(w, x.Err.Error(), 503)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(s.redactLog(x.Output)))
}
func (s *Service) redactLog(v string) string {
	s.mu.Lock()
	activeKey := s.active.AgentKey
	draftKey := s.draft.AgentKey
	previousKey := s.previous.AgentKey
	cameraURLs := []string{s.active.CameraFrontURL, s.active.CameraRearURL, s.active.PlateRecognitionURL, s.draft.CameraFrontURL, s.draft.CameraRearURL, s.draft.PlateRecognitionURL, s.previous.CameraFrontURL, s.previous.CameraRearURL, s.previous.PlateRecognitionURL}
	s.mu.Unlock()
	if activeKey != "" {
		v = strings.ReplaceAll(v, activeKey, "[连接密钥已脱敏]")
	}
	if draftKey != "" {
		v = strings.ReplaceAll(v, draftKey, "[连接密钥已脱敏]")
	}
	if previousKey != "" {
		v = strings.ReplaceAll(v, previousKey, "[连接密钥已脱敏]")
	}
	for _, value := range cameraURLs {
		if value != "" {
			v = strings.ReplaceAll(v, value, "[设备地址已脱敏]")
		}
	}
	return redactText(v)
}
func (s *Service) auditAPI(w http.ResponseWriter, r *http.Request) {
	if !s.require(w, r) {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	rows, err := readAudits(filepath.Join(s.dir, "maintenance.jsonl"))
	if err != nil {
		http.Error(w, "维护记录读取失败", http.StatusInternalServerError)
		return
	}
	typ := r.URL.Query().Get("action")
	result := r.URL.Query().Get("result")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	out := rows[:0]
	for _, v := range rows {
		if typ != "" && !strings.Contains(v.Action, typ) {
			continue
		}
		if result != "" && !strings.Contains(v.Result, result) {
			continue
		}
		if from != "" && v.At.Local().Format("2006-01-02") < from {
			continue
		}
		if to != "" && v.At.Local().Format("2006-01-02") > to {
			continue
		}
		out = append(out, v)
	}
	limit, offset := 100, 0
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 500 {
			s.fail(w, errors.New("查询条数须在 1～500 之间"))
			return
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			s.fail(w, errors.New("查询起点无效"))
			return
		}
	}
	total := len(out)
	start := min(offset, total)
	end := start + min(limit, total-start)
	s.json(w, map[string]any{"records": out[start:end], "total": total, "offset": offset, "limit": limit})
}
func readAudits(path string) ([]Audit, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		if os.IsNotExist(e) {
			return []Audit{}, nil
		}
		return nil, e
	}
	rows := []Audit{}
	for _, line := range strings.Split(string(b), "\n") {
		var a Audit
		if json.Unmarshal([]byte(line), &a) == nil {
			rows = append(rows, a)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].At.After(rows[j].At) })
	return rows, nil
}

//go:embed assets/maintenance.html
var pageHTML string

//go:embed assets/maintenance.js
var pageJS string

//go:embed assets/maintenance.css
var pageCSS string

// BuildAgentEnvironment writes the Agent's private environment without exposing secrets in API responses.
func BuildAgentEnvironment(dir string, c Config) error {
	return BuildAgentEnvironmentAt(filepath.Join(dir, "agent.env"), c)
}

func BuildAgentEnvironmentAt(path string, c Config) error {
	c = agentprinter.NormalizeConfig(c)
	if err := validate(c); err != nil {
		return err
	}
	// ADR 0030: the key is no longer required. A legacy key is retained only
	// so an in-place upgrade does not lose the existing local file before the
	// old keyed deployment is retired.
	data := fmt.Sprintf("WEIGHBRIDGE_SERVER_URL=%s\nWEIGHBRIDGE_AGENT_ID=%s\nWEIGHBRIDGE_SCALE_ID=%d\nWEIGHBRIDGE_PRINT_QUEUE=%s\nWEIGHBRIDGE_PRINT_WIDTH_MM=%d\nWEIGHBRIDGE_PRINT_OFFSET_X_MM=%d\nWEIGHBRIDGE_PRINT_OFFSET_Y_MM=%d\nWEIGHBRIDGE_FORM_LENGTH_MM=%g\nWEIGHBRIDGE_PRINT_SCALE=%g\n", strconv.Quote(c.ServerURL), strconv.Quote(c.AgentID), c.ScaleID, strconv.Quote(c.Queue), c.PrintWidthMM, c.OffsetXMM, c.OffsetYMM, c.FormLengthMM, c.Calibration().Scale)
	if c.AgentKey != "" {
		if strings.ContainsAny(c.AgentKey, "\r\n\x00") {
			return errors.New("旧连接密钥格式无效")
		}
		data = fmt.Sprintf("APP_WEIGHING__AGENT_KEY=%s\n", strconv.Quote(c.AgentKey)) + data
	}
	// D12 tF=05, 8N1, tonne parsing and the 1 s freshness limit are fixed;
	// only the Agent host's serial port and baud are persisted.
	data += fmt.Sprintf("WEIGHBRIDGE_SERIAL_PORT=%s\nWEIGHBRIDGE_SERIAL_BAUD=%d\n", strconv.Quote(c.SerialPort), c.SerialBaud)
	data += fmt.Sprintf("WEIGHBRIDGE_CAMERA_FRONT_URL=%s\nWEIGHBRIDGE_CAMERA_REAR_URL=%s\nWEIGHBRIDGE_PLATE_RECOGNITION_URL=%s\nWEIGHBRIDGE_CAMERA_TIMEOUT_MS=%d\n", strconv.Quote(c.CameraFrontURL), strconv.Quote(c.CameraRearURL), strconv.Quote(c.PlateRecognitionURL), c.CameraTimeoutMS)
	media, _ := json.Marshal(c.CameraSettings())
	data += "WEIGHBRIDGE_CAMERA_CONFIG=" + strconv.Quote(string(media)) + "\n"
	return writePrivate(path, []byte(data))
}
func PortIsLoopback(addr string) bool {
	host, _, e := net.SplitHostPort(addr)
	if e != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func PortIsPrivate(addr string) bool {
	host, port, e := net.SplitHostPort(addr)
	if e != nil {
		return false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return false
	}
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func ValidateListener(addr, cert, key string) error {
	if !PortIsPrivate(addr) {
		return errors.New("维护服务只允许通配、回环或私网 IP 监听，并使用有效端口")
	}
	if (cert == "") != (key == "") {
		return errors.New("TLS 证书和私钥必须同时提供")
	}
	return nil
}
