package weighing

import (
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/gin-gonic/gin"
)

func (s *Service) SetCameraConfigSource(source func() (agentprinter.Config, error)) {
	s.cameraConfig = source
}

// The route uses the same JWT, weighing:read and site scope as device state.
func (h *Handler) cameraPreview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	if h.service.cameraConfig == nil {
		writeResult(c, agentprinter.CameraPreview{Mode: "legacy"}, nil)
		return
	}
	cfg, runtimeError := h.service.cameraConfig()
	if cfg.ScaleID != id {
		writeResult(c, agentprinter.CameraPreview{Mode: "legacy"}, nil)
		return
	}
	writeResult(c, cfg.PreviewConfig(runtimeError), nil)
}
