package agentmaintenance

import (
	"context"
	"errors"
	"net/http"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

func (s *Service) SetCameraTester(fn func(context.Context, string) (p.CameraTest, error)) {
	s.cameraTester = fn
}
func (s *Service) testCamera(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	if !s.require(w, r) {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	cfg, err := s.CameraConfig()
	if err != nil {
		s.fail(w, err)
		return
	}
	if err = s.validateBusinessConfig(r, cfg); err != nil {
		s.fail(w, err)
		return
	}
	if s.cameraTester == nil || (cfg.CameraMode == "mediamtx" && !cfg.CameraEnabled) {
		s.fail(w, errors.New("摄像头已停用或测试通道未接入"))
		return
	}
	// Drain formal capture/confirmation before issuing this separate diagnostic.
	if s.operationGuard != nil {
		release, e := s.operationGuard(r.Context())
		if e != nil {
			s.fail(w, e)
			return
		}
		defer release()
	}
	result, err := s.cameraTester(r.Context(), cfg.ScaleCode)
	// Only status metadata enters the audit; thumbnails live in this response.
	statuses := map[string]bool{}
	for _, v := range result.Views {
		statuses[v.View] = v.Success
	}
	err = errors.Join(err, s.audit(r, "摄像头测试", "前后摄像头", nil, statuses, err))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.json(w, result)
}
