package agentmaintenance

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls   [][]string
	outputs map[string]Result
}

func (f *fakeRunner) Run(a []string) Result {
	f.calls = append(f.calls, append([]string(nil), a...))
	if len(a) == 3 && a[0] == "lpstat" && a[1] == "-p" {
		return f.outputs["lpstat -p"]
	}
	return f.outputs[strings.Join(a, " ")]
}
func call(t *testing.T, h http.Handler, method, path, body string, cookie *http.Cookie) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "192.168.1.20:2345"
	r.Header.Set("X-Maintenance-Session", "forged-client-value")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w, r
}
func login(t *testing.T, h http.Handler, dir string) *http.Cookie {
	t.Helper()
	pw, e := os.ReadFile(filepath.Join(dir, "bootstrap-password.txt"))
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(map[string]string{"password": strings.TrimSpace(string(pw))})
	w, _ := call(t, h, "POST", "/api/login", string(body), nil)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	return w.Result().Cookies()[0]
}

func TestReadOnlyAndPerRequestMaintenanceAuthorization(t *testing.T) {
	dir := t.TempDir()
	f := &fakeRunner{outputs: map[string]Result{"lpstat -r": {Output: "scheduler is running"}, "systemctl --user is-active weighbridge-agent-printer.service": {Output: "inactive"}, "lpstat -p": {Output: "printer Test-Queue is idle. enabled"}}}
	s, e := Open(dir, f)
	if e != nil {
		t.Fatal(e)
	}
	h := s.Handler()
	w, _ := call(t, h, "GET", "/api/status", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "未知") {
		t.Fatalf("readonly status unavailable: %d %s", w.Code, w.Body)
	}
	w, _ = call(t, h, "POST", "/api/action", `{"action":"restart-agent"}`, nil)
	if w.Code != 401 {
		t.Fatalf("unauthenticated action accepted: %d", w.Code)
	}
	cookie := login(t, h, dir)
	w, _ = call(t, h, "GET", "/api/config", "", cookie)
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-value") {
		t.Fatalf("secret leaked or config unavailable: %s", w.Body)
	}
}

func TestConfigValidationFixedCommandsAndAuditPersistence(t *testing.T) {
	dir := t.TempDir()
	f := &fakeRunner{outputs: map[string]Result{"systemctl --user restart weighbridge-agent-printer.service": {Output: "restarted"}, "lpstat -W not-completed -o": {}, "lpstat -r": {}, "systemctl --user is-active weighbridge-agent-printer.service": {Output: "active"}, "lpstat -p": {Output: "printer safe is idle. enabled"}}}
	s, e := Open(dir, f)
	if e != nil {
		t.Fatal(e)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	c := Defaults()
	c.AgentKey = "do-not-log-this-secret"
	c.Queue = "-X;id"
	b, _ := json.Marshal(c)
	w, _ := call(t, h, "PUT", "/api/config", string(b), cookie)
	if w.Code != 400 {
		t.Fatalf("option-like queue accepted: %d", w.Code)
	}
	c.Queue = "safe"
	c.ServerURL = "http://server.example"
	b, _ = json.Marshal(c)
	w, _ = call(t, h, "PUT", "/api/config", string(b), cookie)
	if w.Code != 200 {
		t.Fatalf("save config: %d %s", w.Code, w.Body)
	}
	status, _ := call(t, h, "GET", "/api/status", "", cookie)
	if !strings.Contains(status.Body.String(), `"queue":"Aisino-SK820-raw"`) {
		t.Fatalf("draft changed active queue before apply: %s", status.Body)
	}
	w, _ = call(t, h, "POST", "/api/action", `{"action":"apply"}`, cookie)
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	env, e := os.ReadFile(filepath.Join(dir, "agent.env"))
	if e != nil || !bytes.Contains(env, []byte("WEIGHBRIDGE_PRINT_WIDTH_MM=175")) {
		t.Fatalf("effective config missing calibration: %s %v", env, e)
	}
	audit, e := os.ReadFile(filepath.Join(dir, "maintenance.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(audit), c.AgentKey) || strings.Contains(string(audit), "forged-client-value") {
		t.Fatalf("sensitive or forged session audit data: %s", audit)
	}
	var row Audit
	if e = json.Unmarshal(bytes.Split(audit, []byte("\n"))[1], &row); e != nil {
		t.Fatal(e)
	}
	if row.Session == "" {
		t.Fatal("authenticated session fingerprint missing")
	}
	if _, e = Open(dir, f); e != nil {
		t.Fatalf("audit/config not restart durable: %v", e)
	}
}

func TestPreviewTokenDetectsQueueChangesAndCUPSUnknownRecoveryIsExplicit(t *testing.T) {
	dir := t.TempDir()
	f := &fakeRunner{outputs: map[string]Result{"lpstat -W not-completed -o": {Output: "queue-12 operator 1024 bytes Mon 20 Oct 2026 10:00:00"}, "lpstat -r": {Output: "scheduler is running"}}}
	s, _ := Open(dir, f)
	h := s.Handler()
	cookie := login(t, h, dir)
	w, _ := call(t, h, "GET", "/api/jobs", "", cookie)
	var view struct {
		Token string `json:"previewToken"`
		Jobs  []Job  `json:"jobs"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &view)
	if len(view.Jobs) != 1 || strings.Contains(view.Jobs[0].Title, "operator") {
		t.Fatalf("lpstat output misparsed as title: %+v", view.Jobs)
	}
	f.outputs["lpstat -W not-completed -o"] = Result{Output: "queue-12 operator 1024 bytes Mon 20 Oct 2026 10:00:00\nqueue-13 operator 512 bytes Mon 20 Oct 2026 10:01:00"}
	body, _ := json.Marshal(map[string]any{"action": "restart-cups", "previewToken": view.Token, "disposition": "keep"})
	w, _ = call(t, h, "POST", "/api/action", string(body), cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "needsDisposition") {
		t.Fatalf("queue change not re-previewed: %d %s", w.Code, w.Body)
	}
	f.outputs["lpstat -W not-completed -o"] = Result{Err: os.ErrNotExist}
	w, _ = call(t, h, "GET", "/api/jobs", "", cookie)
	_ = json.Unmarshal(w.Body.Bytes(), &view)
	body, _ = json.Marshal(map[string]any{"action": "restart-cups", "previewToken": view.Token, "disposition": "unknown-confirmed"})
	w, _ = call(t, h, "POST", "/api/action", string(body), cookie)
	if w.Code != 200 {
		t.Fatalf("explicit unknown recovery refused: %d %s", w.Code, w.Body)
	}
}

func TestParseJobsAndRunnerRejectsArbitraryExecutable(t *testing.T) {
	jobs := ParseJobs("queue-9 user 1024 bytes Mon 20 Oct 2026 10:00:00\n-evil-2 user 1 bytes now\nnot a job")
	if len(jobs) != 1 || jobs[0].ID != "queue-9" || !strings.Contains(jobs[0].State, "未知") {
		t.Fatalf("unexpected CUPS parser: %+v", jobs)
	}
	r := ExecRunner{}.Run([]string{"sh", "-c", "touch /tmp/not-allowed"})
	if r.Err == nil {
		t.Fatal("arbitrary command accepted")
	}
}

func TestPortLoopbackOnlyRecognizesLiteralLocalAddresses(t *testing.T) {
	if !PortIsLoopback("127.0.0.1:80") || !PortIsLoopback("[::1]:80") || PortIsLoopback(":80") || PortIsLoopback("0.0.0.0:80") {
		t.Fatal("listen binding classification failed")
	}
}
