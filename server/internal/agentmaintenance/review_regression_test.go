package agentmaintenance

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type scriptedRunner func([]string) Result

func (f scriptedRunner) Run(args []string) Result { return f(args) }

func TestSavedDraftLeavesOperationalQueueUnchanged(t *testing.T) {
	dir := t.TempDir()
	f := &fakeRunner{outputs: map[string]Result{
		"lpstat -r": {Output: "scheduler is running"},
		"systemctl --user is-active weighbridge-agent-printer.service": {Output: "active"},
		"lpstat -p": {Output: "printer Aisino-SK820-raw is idle. enabled\nprinter other is idle. enabled"},
	}}
	s, err := Open(dir, f)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	draft := Defaults()
	draft.Queue = "other"
	b, _ := json.Marshal(draft)
	w, _ := call(t, h, http.MethodPut, "/api/config", string(b), cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w, _ = call(t, h, http.MethodGet, "/api/status", "", nil)
	var status struct {
		Queue string `json:"queue"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Queue != Defaults().Queue {
		t.Fatalf("draft affected operational queue before apply: %q", status.Queue)
	}
}

func TestFailedActivationRestoresEnvironmentAndLastActiveConfig(t *testing.T) {
	dir := t.TempDir()
	old := Defaults()
	old.AgentKey = "existing-agent-key"
	if err := BuildAgentEnvironment(dir, old); err != nil {
		t.Fatal(err)
	}
	oldEnv, _ := os.ReadFile(filepath.Join(dir, "agent.env"))
	starts := 0
	s, err := Open(dir, scriptedRunner(func(args []string) Result {
		switch strings.Join(args, " ") {
		case "lpstat -p Aisino-SK820-raw":
			return Result{Output: "printer Aisino-SK820-raw is idle. enabled"}
		case "systemctl --user is-active weighbridge-agent-printer.service":
			return Result{Output: "active"}
		case "systemctl --user start weighbridge-agent-printer.service":
			starts++
			if starts == 1 {
				return Result{Err: errors.New("fixture failed start")}
			}
		}
		return Result{}
	}))
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	draft := old
	draft.PrintWidthMM = 170
	b, _ := json.Marshal(draft)
	w, _ := call(t, h, http.MethodPut, "/api/config", string(b), cookie)
	if w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w, _ = call(t, h, http.MethodPost, "/api/action", `{"action":"apply"}`, cookie)
	if w.Code < 400 {
		t.Fatalf("failed start reported success: %d %s", w.Code, w.Body)
	}
	currentEnv, _ := os.ReadFile(filepath.Join(dir, "agent.env"))
	if string(oldEnv) != string(currentEnv) {
		t.Fatal("failed activation left new environment installed")
	}
	if starts != 2 {
		t.Fatalf("original running Agent was not resumed: starts=%d", starts)
	}
	w, _ = call(t, h, http.MethodGet, "/api/config", "", cookie)
	var state struct {
		Active Config `json:"active"`
		Draft  Config `json:"config"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Active.PrintWidthMM != 175 || state.Draft.PrintWidthMM != 170 {
		t.Fatalf("failed activation confused active/draft: %+v", state)
	}
}

func TestInvalidPreviewDoesNotStopProducer(t *testing.T) {
	dir := t.TempDir()
	var calls []string
	s, err := Open(dir, scriptedRunner(func(args []string) Result { calls = append(calls, strings.Join(args, " ")); return Result{} }))
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	w, _ := call(t, h, http.MethodPost, "/api/action", `{"action":"restart-cups","previewToken":"invalid","disposition":"keep"}`, cookie)
	if w.Code < 400 || len(calls) != 0 {
		t.Fatalf("invalid preview affected producer: %d %v", w.Code, calls)
	}
}

func TestQueueChangeDoesNotHideProducerResumeFailure(t *testing.T) {
	dir := t.TempDir()
	jobs := "queue-11 operator 1024 bytes Tue 20 Oct 2026"
	s, err := Open(dir, scriptedRunner(func(args []string) Result {
		switch strings.Join(args, " ") {
		case "lpstat -W not-completed -o":
			return Result{Output: jobs}
		case "systemctl --user is-active weighbridge-agent-printer.service":
			return Result{Output: "active"}
		case "systemctl --user start weighbridge-agent-printer.service":
			return Result{Err: errors.New("resume fixture failure")}
		}
		return Result{}
	}))
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	w, _ := call(t, h, http.MethodGet, "/api/jobs", "", cookie)
	var preview struct {
		Token string `json:"previewToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	jobs += "\nqueue-12 operator 1024 bytes Tue 20 Oct 2026"
	b, _ := json.Marshal(map[string]any{"action": "restart-cups", "previewToken": preview.Token, "disposition": "keep"})
	w, _ = call(t, h, http.MethodPost, "/api/action", string(b), cookie)
	if w.Code < 400 || !strings.Contains(w.Body.String(), "resume fixture failure") {
		t.Fatalf("preview swallowed failed resume: %d %s", w.Code, w.Body)
	}
}

func TestCalibrationEndpointReleasesStateLock(t *testing.T) {
	// Rendering fails immediately without Python, allowing this lock regression
	// to run on Go-only CI hosts without installing fonts or printer tools.
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	s, err := Open(dir, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	done := make(chan int, 1)
	go func() { w, _ := call(t, h, http.MethodPost, "/api/test-print", "{}", cookie); done <- w.Code }()
	select {
	case code := <-done:
		if code != 400 {
			t.Fatalf("unexpected render rejection: %d", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("calibration endpoint deadlocked")
	}
}

func TestApplyQueueChangeReportsFailedOriginalAgentResume(t *testing.T) {
	dir := t.TempDir()
	jobs := ""
	s, err := Open(dir, scriptedRunner(func(args []string) Result {
		switch strings.Join(args, " ") {
		case "lpstat -p Aisino-SK820-raw":
			return Result{Output: "printer Aisino-SK820-raw is idle. enabled"}
		case "lpstat -W not-completed -o":
			return Result{Output: jobs}
		case "systemctl --user is-active weighbridge-agent-printer.service":
			return Result{Output: "active"}
		case "systemctl --user stop weighbridge-agent-printer.service":
			jobs = "Aisino-SK820-raw-12 operator 1024 bytes Tue 20 Oct 2026"
			return Result{}
		case "systemctl --user start weighbridge-agent-printer.service":
			return Result{Err: errors.New("apply resume fixture failure")}
		}
		return Result{}
	}))
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	cfg := Defaults()
	cfg.AgentKey = "regression-fixture-key"
	b, _ := json.Marshal(cfg)
	w, _ := call(t, h, http.MethodPut, "/api/config", string(b), cookie)
	if w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w, _ = call(t, h, http.MethodPost, "/api/action", `{"action":"apply"}`, cookie)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "apply resume fixture failure") {
		t.Fatalf("apply preview hid failed producer resume: %d %s", w.Code, w.Body)
	}
}

func TestApplyCannotKeepPendingJobs(t *testing.T) {
	dir := t.TempDir()
	var calls []string
	s, err := Open(dir, scriptedRunner(func(args []string) Result {
		cmd := strings.Join(args, " ")
		calls = append(calls, cmd)
		if cmd == "lpstat -W not-completed -o" {
			return Result{Output: "Aisino-SK820-raw-12 operator 1024 bytes Mon 20 Oct 2026 10:00:00"}
		}
		if strings.HasPrefix(cmd, "lpstat -p ") {
			return Result{Output: "printer Aisino-SK820-raw is idle. enabled"}
		}
		if strings.Contains(cmd, "is-active") {
			return Result{Output: "active"}
		}
		return Result{}
	}))
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	cfg := Defaults()
	cfg.AgentKey = "test-maintenance-agent-key"
	configBody, _ := json.Marshal(cfg)
	w, _ := call(t, h, http.MethodPut, "/api/config", string(configBody), cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("save valid draft: %d %s", w.Code, w.Body)
	}
	w, _ = call(t, h, http.MethodGet, "/api/jobs", "", cookie)
	var preview struct {
		Token string `json:"previewToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"action": "apply", "previewToken": preview.Token, "disposition": "keep"})
	w, _ = call(t, h, http.MethodPost, "/api/action", string(b), cookie)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "必须完成") {
		t.Fatalf("applied while keeping pending work: %d %s; commands=%v", w.Code, w.Body, calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.env")); !os.IsNotExist(err) {
		t.Fatalf("blocked apply changed runtime environment: %v", err)
	}
}

func TestCancellationRecordIncludesSelectedJob(t *testing.T) {
	dir := t.TempDir()
	job := "Aisino-SK820-raw-12"
	queued := true
	s, err := Open(dir, scriptedRunner(func(args []string) Result {
		if len(args) == 3 && args[0] == "lpstat" && args[1] == "-p" {
			return Result{Output: "printer Aisino-SK820-raw is idle. enabled"}
		}
		switch strings.Join(args, " ") {
		case "lpstat -W not-completed -o":
			if queued {
				return Result{Output: job + " operator 1024 bytes Mon 20 Oct 2026 10:00:00"}
			}
		case "cancel " + job:
			queued = false
		}
		return Result{}
	}))
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	b, _ := json.Marshal(map[string]any{"action": "cancel-jobs", "jobIds": []string{job}})
	w, _ := call(t, h, http.MethodPost, "/api/action", string(b), cookie)
	if w.Code != 200 {
		t.Fatalf("cancel: %d %s", w.Code, w.Body)
	}
	w, _ = call(t, h, http.MethodGet, "/api/audit", "", cookie)
	if !strings.Contains(w.Body.String(), job) {
		t.Fatalf("query cannot identify canceled job: %s", w.Body)
	}
}

func TestOlderMaintenanceRecordsRemainQueryable(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "maintenance.jsonl"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for i := 0; i < 502; i++ {
		row := Audit{At: time.Date(2026, 10, 20, 10, 0, i, 0, time.UTC), Action: "recent", Result: "成功"}
		if i == 0 {
			row.At = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
			row.Action = "historical"
		}
		if err := enc.Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookie := login(t, h, dir)
	w, _ := call(t, h, http.MethodGet, "/api/audit?from=2026-01-01&to=2026-01-01", "", cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "historical") {
		t.Fatalf("older matching record disappeared: %d %s", w.Code, w.Body)
	}
}
