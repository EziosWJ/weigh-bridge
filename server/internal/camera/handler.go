package camera

import (
	"errors"
	"io"
	"strconv"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service     *Service
	permissions *rbac.Service
}

func NewHandler(service *Service, permissions *rbac.Service) *Handler {
	return &Handler{service: service, permissions: permissions}
}

// RegisterRoutes mounts the camera module under the existing business API
// prefix. The module has its own maintenance boundary and does not depend on
// the Agent connection or the weighing site-scope middleware.
func RegisterRoutes(router gin.IRouter, h *Handler) {
	view := rbac.RequirePermission(h.permissions, "weighing:camera:view")
	manage := rbac.RequirePermission(h.permissions, "weighing:camera:manage")
	router.GET("/cameras", view, h.list)
	router.POST("/cameras", manage, h.create)
	router.PUT("/cameras/:id", manage, h.update)
	router.DELETE("/cameras/:id", manage, h.delete)
	router.GET("/cameras/settings", view, h.settings)
	router.PUT("/cameras/settings", manage, h.saveSettings)
	router.GET("/cameras/video", view, h.video)
	router.PUT("/cameras/video", manage, h.saveVideo)
	router.POST("/cameras/sync", manage, h.sync)
	router.POST("/cameras/:id/test", manage, h.test)
	router.GET("/cameras/:id/preview", view, h.preview)
	router.GET("/images/:id", rbac.RequirePermission(h.permissions, "weighing:read"), h.image)
}

func (h *Handler) list(c *gin.Context) {
	value, err := h.service.List(c)
	writeResult(c, value, err)
}

func (h *Handler) create(c *gin.Context) {
	var input CameraInput
	if c.ShouldBindJSON(&input) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	value, err := h.service.Create(c, input, requestEvent(c))
	writeResult(c, value, err)
}

func (h *Handler) update(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var input CameraInput
	if c.ShouldBindJSON(&input) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	value, err := h.service.Update(c, id, input, requestEvent(c))
	writeResult(c, value, err)
}

func (h *Handler) delete(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	var input struct {
		Version int64 `json:"version"`
	}
	if c.ShouldBindJSON(&input) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	writeResult(c, nil, h.service.Delete(c, id, input.Version, requestEvent(c)))
}

func (h *Handler) settings(c *gin.Context) {
	value, err := h.service.Settings(c)
	writeResult(c, value, err)
}

func (h *Handler) saveSettings(c *gin.Context) {
	var input CaptureSettingsInput
	if c.ShouldBindJSON(&input) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	value, err := h.service.SaveCaptureSettings(c, input, requestEvent(c))
	writeResult(c, value, err)
}

func (h *Handler) video(c *gin.Context) {
	value, err := h.service.Video(c)
	writeResult(c, value, err)
}

func (h *Handler) saveVideo(c *gin.Context) {
	var input VideoSettingsInput
	if c.ShouldBindJSON(&input) != nil {
		writeResult(c, nil, ErrInvalid)
		return
	}
	value, err := h.service.SaveVideoSettings(c, input, requestEvent(c))
	writeResult(c, value, err)
}

func (h *Handler) sync(c *gin.Context) {
	var input SyncRequest
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		writeResult(c, nil, ErrInvalid)
		return
	}
	value, err := h.service.Sync(c, input)
	writeResult(c, value, err)
}

func (h *Handler) test(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	value, err := h.service.Test(c, id, requestEvent(c))
	writeResult(c, value, err)
}

func (h *Handler) preview(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	value, err := h.service.Preview(c, id)
	writeResult(c, value, err)
}

func (h *Handler) image(c *gin.Context) {
	path, err := h.service.ImagePath(c, c.Param("id"))
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400, immutable")
	c.File(path)
}

func pathID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, ErrInvalid
	}
	return id, nil
}

func requestEvent(c *gin.Context) audit.Event {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	meta, _ := platformhttp.RequestMetaFromContext(c.Request.Context())
	return audit.Event{Metadata: audit.Metadata{
		ActorID: principal.UserID, RequestID: meta.RequestID, ClientIP: meta.ClientIP,
		UserAgent: meta.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI(),
	}}
}

func writeResult(c *gin.Context, value any, err error) {
	if err == nil {
		platformhttp.OK(c, value)
		return
	}
	switch {
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.TemporaryUnavailable(c)
	case errors.Is(err, ErrNotFound):
		platformhttp.WriteError(c, 404, 404, err.Error(), nil)
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrConflict):
		platformhttp.WriteError(c, 200, 400, err.Error(), nil)
	default:
		platformhttp.WriteError(c, 500, 500, "业务处理失败", nil)
	}
}
