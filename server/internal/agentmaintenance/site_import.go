package agentmaintenance

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
)

func (s *Service) importConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	if !s.require(w, r) {
		return
	}
	var in struct {
		Connection agentconfig.Connection `json:"connection"`
		Confirm    bool                   `json:"confirm"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil {
		s.fail(w, errors.New("厂区配置文件格式无效"))
		return
	}
	if err := in.Connection.Validate(); err != nil {
		s.fail(w, err)
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	_, _ = s.reloadEnvironment()
	s.mu.Lock()
	before := s.draft
	s.mu.Unlock()
	cfg := before
	c := in.Connection
	cfg.ServerURL = c.ServerURL
	cfg.SiteCode = c.SiteCode
	cfg.SiteName = c.SiteName
	cfg.ScaleCode = c.ScaleCode
	cfg.ScaleCode = c.ScaleCode
	cfg.ScaleName = c.ScaleName
	cfg.AgentID = c.AgentID
	cfg.AgentKey = c.AgentKey
	if err := validate(cfg); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.validateBusinessConfig(r, cfg); err != nil {
		s.fail(w, err)
		return
	}
	if in.Confirm {
		s.mu.Lock()
		s.draft = cfg
		err := s.persist()
		if err != nil {
			s.draft = before
		}
		s.mu.Unlock()
		err = errors.Join(err, s.audit(r, "厂区配置导入", cfg.Queue, before, cfg, err))
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	hasAgentKey := cfg.AgentKey != ""
	hasCameraFrontURL := cfg.CameraFrontURL != ""
	hasCameraRearURL := cfg.CameraRearURL != ""
	hasPlateRecognitionURL := cfg.PlateRecognitionURL != ""
	cfg.AgentKey = ""
	cfg.CameraFrontURL, cfg.CameraRearURL, cfg.PlateRecognitionURL = "", "", ""
	s.json(w, map[string]any{"config": cfg, "saved": in.Confirm, "hasAgentKey": hasAgentKey,
		"hasCameraFrontUrl": hasCameraFrontURL, "hasCameraRearUrl": hasCameraRearURL,
		"hasPlateRecognitionUrl": hasPlateRecognitionURL})
}
