package agentmaintenance

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Stop the producer before re-reading the approved CUPS snapshot. A pause file
// alone cannot stop an attempt which has already entered Render/Submit.
func (s *Service) runRecovery(w http.ResponseWriter, r *http.Request, action, token, disposition string, selected []string) (result Result, handled bool) {
	s.mu.Lock()
	_, validPreview := s.previews[token]
	s.mu.Unlock()
	if !validPreview {
		return Result{Err: errors.New("请先查看当前作业并确认")}, true
	}
	previewResponse := &bufferedPreview{headers: make(http.Header)}
	state := s.runner.Run([]string{"systemctl", "--user", "is-active", "weighbridge-agent-printer.service"})
	wasRunning := strings.TrimSpace(state.Output) == "active"
	stopped := s.runner.Run([]string{"systemctl", "--user", "stop", "weighbridge-agent-printer.service"})
	if stopped.Err != nil {
		return stopped, true
	}
	_ = os.Remove(filepath.Join(s.dir, "maintenance-status.json"))
	defer func() {
		if wasRunning {
			resumed := s.runner.Run([]string{"systemctl", "--user", "start", "weighbridge-agent-printer.service"})
			if resumed.Err == nil {
				resumed.Err = s.verifyAgent()
			}
			if resumed.Err != nil {
				result.Err = errors.Join(result.Err, fmt.Errorf("恢复原打印 Agent 失败：%w", resumed.Err))
				handled = true
			}
		}
		if !handled {
			previewResponse.flush(w)
		}
	}()
	if !s.requirePreview(previewResponse, r, token, disposition, selected) {
		return Result{}, false
	}
	if action == "restart-agent" {
		wasRunning = false
		result = s.runner.Run([]string{"systemctl", "--user", "start", "weighbridge-agent-printer.service"})
		if result.Err == nil {
			result.Err = s.verifyAgent()
		}
	} else if action == "stop-agent" {
		wasRunning = false
		result = Result{Output: "stopped"}
		result.Err = s.verifyAgentStopped()
	} else if action == "restart-cups" {
		result = s.runner.Run([]string{"systemctl", "restart", "cups.service"})
		if result.Err == nil {
			result.Err = s.verifyCUPS()
		}
	} else {
		s.mu.Lock()
		queue := s.active.Queue
		s.mu.Unlock()
		result = s.runner.Run([]string{"cupsenable", queue})
		if result.Err == nil {
			result.Err = s.verifyQueue(queue)
		}
	}
	return result, true
}

// Delay a changed/invalid-preview response until the original producer is
// restored, so a failed resume cannot be hidden behind a successful preview.
type bufferedPreview struct {
	headers http.Header
	status  int
	body    bytes.Buffer
}

func (b *bufferedPreview) Header() http.Header { return b.headers }
func (b *bufferedPreview) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *bufferedPreview) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(data)
}
func (b *bufferedPreview) flush(w http.ResponseWriter) {
	for k, values := range b.headers {
		w.Header()[k] = values
	}
	if b.status == 0 {
		b.status = http.StatusOK
	}
	w.WriteHeader(b.status)
	_, _ = w.Write(b.body.Bytes())
}

func (s *Service) cancelJob(r *http.Request, id string) Result {
	result := s.runner.Run([]string{"cancel", id})
	result.Err = errors.Join(result.Err, s.audit(r, "取消打印作业", id, nil, result.Output, result.Err))
	return result
}

func (s *Service) readJobs() Result {
	result := s.runner.Run([]string{"lpstat", "-W", "not-completed", "-o"})
	if result.Err == nil {
		for _, line := range strings.Split(result.Output, "\n") {
			if line = strings.TrimSpace(line); line != "" && !jobRE.MatchString(line) {
				result.Err = errors.New("无法解析完整 CUPS 作业列表，作业状态未知；请核查后再恢复")
				break
			}
		}
	}
	return result
}
