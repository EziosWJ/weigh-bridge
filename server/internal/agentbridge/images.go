package agentbridge

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	ph "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

var imageName = regexp.MustCompile(`^([A-Za-z0-9_.-]{1,40})_([a-f0-9]{32})_(front|rear)\.(png|jpg)$`)

// ParseImageID returns the scale code that owns an Agent image id. Scale codes
// may contain underscores, so the id is parsed from its structure instead of
// splitting on the first underscore; the greedy prefix keeps codes intact.
func ParseImageID(id string) (string, bool) {
	match := imageName.FindStringSubmatch(id)
	if match == nil {
		return "", false
	}
	return match[1], true
}

func (h *Hub) Register(router gin.IRouter) {
	g := router.Group("/api/agent")
	// Keyless mode is the ADR 0030 production path. The server resolves the
	// local site's sole scaleCode; the Agent supplies only its Server address.
	// The legacy keyed path remains available only when no local resolver was
	// installed, so old deployments can be diagnosed during the upgrade.
	if h.local != nil {
		g.GET("/ws", func(c *gin.Context) {
			websocket.Server{Handshake: func(_ *websocket.Config, _ *http.Request) error { return nil }, Handler: websocket.Handler(h.serveLocal)}.ServeHTTP(c.Writer, c.Request)
		})
		g.POST("/images", h.uploadLocal)
		return
	}
	g.Use(func(c *gin.Context) {
		child, resolveErr := h.resolve(c.Request.Context(), c.GetHeader("X-Scale-Code"))
		if resolveErr != nil || child == nil || child.cfg.Key == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+child.cfg.Key)) != 1 || c.GetHeader("X-Agent-ID") != child.cfg.ID {
			ph.AbortError(c, 401, 401, "Agent 身份无效", nil)
			return
		}
		c.Set("agentHub", child)
		c.Next()
	})
	g.GET("/ws", func(c *gin.Context) {
		child := c.MustGet("agentHub").(*Hub)
		websocket.Server{Handshake: func(_ *websocket.Config, _ *http.Request) error { return nil }, Handler: websocket.Handler(child.Serve)}.ServeHTTP(c.Writer, c.Request)
	})
	g.POST("/images", func(c *gin.Context) { c.MustGet("agentHub").(*Hub).upload(c) })
}
func (h *Hub) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 11<<20)
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		ph.WriteError(c, 400, 400, "图片上传失败或超过 10MB", nil)
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		ph.WriteError(c, 400, 400, "缺少图片", nil)
		return
	}
	defer file.Close()
	v, err := h.SaveImage(c.PostForm("view"), file)
	if err != nil {
		ph.WriteError(c, 400, 400, err.Error(), nil)
		return
	}
	ph.OK(c, v)
}
func (h *Hub) SaveImage(view string, reader io.Reader) (p.Image, error) {
	if view != "front" && view != "rear" {
		return p.Image{}, fmt.Errorf("invalid image view")
	}
	data, err := io.ReadAll(io.LimitReader(reader, (10<<20)+1))
	if err != nil {
		return p.Image{}, err
	}
	if len(data) > 10<<20 {
		return p.Image{}, fmt.Errorf("图片超过 10MB")
	}
	info, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || info.Width < 1 || info.Height < 1 || int64(info.Width)*int64(info.Height) > 40000000 {
		return p.Image{}, fmt.Errorf("仅支持有效 PNG/JPEG 图片，最多四千万像素")
	}
	ext := "png"
	if format == "jpeg" {
		ext = "jpg"
	}
	id := fmt.Sprintf("%s_%s_%s.%s", h.cfg.ScaleCode, ID(), view, ext)
	if err := os.MkdirAll(h.cfg.ImageDir, 0700); err != nil {
		return p.Image{}, err
	}
	if err := os.WriteFile(filepath.Join(h.cfg.ImageDir, id), data, 0600); err != nil {
		return p.Image{}, err
	}
	sum := sha256.Sum256(data)
	return p.Image{ID: id, View: view, URL: "/api/weighing/agent-images/" + id, SHA256: hex.EncodeToString(sum[:])}, nil
}

// legacyImagePrefix reports a pre-ADR-0033 file prefix, which was the numeric
// scale id. A local database owns exactly one site and scale, so those files
// keep resolving to the sole connected scale without rewriting history.
func legacyImagePrefix(prefix string) bool {
	if prefix == "" {
		return false
	}
	for _, r := range prefix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// imageHub resolves the scale that owns an image id. Ambiguous legacy numeric
// prefixes only resolve when exactly one scale is connected.
func (h *Hub) imageHub(id string) *Hub {
	prefix, ok := ParseImageID(id)
	if !ok {
		return nil
	}
	if h.children != nil {
		h.mu.Lock()
		defer h.mu.Unlock()
		if child := h.children[prefix]; child != nil {
			return child
		}
		if legacyImagePrefix(prefix) && len(h.children) == 1 {
			for _, child := range h.children {
				return child
			}
		}
		return nil
	}
	if prefix == h.cfg.ScaleCode || legacyImagePrefix(prefix) {
		return h
	}
	return nil
}

func (h *Hub) Image(id string) (p.Image, error) {
	owner := h.imageHub(id)
	if owner == nil {
		return p.Image{}, fmt.Errorf("invalid image reference")
	}
	if _, ok := ParseImageID(id); !ok {
		return p.Image{}, fmt.Errorf("invalid image reference")
	}
	data, err := os.ReadFile(filepath.Join(owner.cfg.ImageDir, id))
	if err != nil {
		return p.Image{}, err
	}
	view := "front"
	if strings.Contains(id, "_rear.") {
		view = "rear"
	}
	sum := sha256.Sum256(data)
	// Historical Agent images keep their original file identity and view name.
	// Source and status are explicit so new pages never label them as
	// Server-acquired camera evidence.
	return p.Image{ID: id, View: view, URL: "/api/weighing/agent-images/" + id, SHA256: hex.EncodeToString(sum[:]), Source: "agent", Status: "captured"}, nil
}

// SetLocalImageDir configures the single local history directory independently
// of the current device connection and mutable scale code. Capture validation
// continues to use Image and the connected device's exact identity.
func (h *Hub) SetLocalImageDir(dir string) {
	h.gate.Lock()
	defer h.gate.Unlock()
	h.localImageDir = dir
}

func (h *Hub) ServeImage(c *gin.Context) {
	id := c.Param("imageId")
	if _, ok := ParseImageID(id); !ok {
		ph.WriteError(c, 404, 404, "图片不存在", nil)
		return
	}
	h.gate.RLock()
	dir := h.localImageDir
	h.gate.RUnlock()
	if dir != "" {
		path := filepath.Join(dir, id)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			ph.WriteError(c, 404, 404, "图片不存在", nil)
			return
		}
		c.Header("Cache-Control", "private, max-age=86400, immutable")
		c.File(path)
		return
	}
	if _, err := h.Image(id); err != nil {
		ph.WriteError(c, 404, 404, "图片不存在", nil)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400, immutable")
	owner := h.imageHub(id)
	if owner == nil {
		ph.WriteError(c, 404, 404, "图片不存在", nil)
		return
	}
	c.File(filepath.Join(owner.cfg.ImageDir, id))
}
func (h *Hub) resolveImages(input []p.Image) ([]p.Image, error) {
	if len(input) != 2 {
		return nil, fmt.Errorf("需要前后视角图片")
	}
	out := make([]p.Image, 0, 2)
	views := map[string]bool{}
	for _, v := range input {
		stored, err := h.Image(v.ID)
		if err != nil || views[stored.View] {
			return nil, fmt.Errorf("图片引用或视角无效")
		}
		views[stored.View] = true
		out = append(out, stored)
	}
	if !views["front"] || !views["rear"] {
		return nil, fmt.Errorf("需要前后视角图片")
	}
	return out, nil
}

// Optional capture still validates every supplied reference; only absence is permitted.
func (h *Hub) resolveCaptureImages(result p.Capture, required bool) ([]p.Image, error) {
	if required {
		if result.ImageStatus != "" && result.ImageStatus != "complete" {
			return nil, fmt.Errorf("强制抓拍要求完整图片状态")
		}
		return h.resolveImages(result.Images)
	}
	switch result.ImageStatus {
	case "complete", "":
		return h.resolveImages(result.Images)
	case "disabled":
		if len(result.Images) != 0 {
			return nil, fmt.Errorf("停用抓拍不能提供图片")
		}
	case "capture_failed", "upload_failed":
	default:
		return nil, fmt.Errorf("图片状态无效")
	}
	if len(result.Images) > 2 {
		return nil, fmt.Errorf("图片数量无效")
	}
	out := make([]p.Image, 0, len(result.Images))
	views := map[string]bool{}
	for _, v := range result.Images {
		stored, err := h.Image(v.ID)
		if err != nil || views[stored.View] {
			return nil, fmt.Errorf("图片引用或视角无效")
		}
		views[stored.View] = true
		out = append(out, stored)
	}
	return out, nil
}
