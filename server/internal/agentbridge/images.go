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
	"strconv"
	"strings"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	ph "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

var imageName = regexp.MustCompile(`^[0-9]+_[a-f0-9]{32}_(front|rear)\.(png|jpg)$`)

func (h *Hub) Register(router gin.IRouter) {
	g := router.Group("/api/agent")
	g.Use(func(c *gin.Context) {
		scale, _ := strconv.ParseInt(c.GetHeader("X-Scale-ID"), 10, 64)
		child := h.forScale(scale)
		if child == nil || child.cfg.Key == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+child.cfg.Key)) != 1 || c.GetHeader("X-Agent-ID") != child.cfg.ID {
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
	id := fmt.Sprintf("%d_%s_%s.%s", h.cfg.ScaleID, ID(), view, ext)
	if err := os.MkdirAll(h.cfg.ImageDir, 0700); err != nil {
		return p.Image{}, err
	}
	if err := os.WriteFile(filepath.Join(h.cfg.ImageDir, id), data, 0600); err != nil {
		return p.Image{}, err
	}
	sum := sha256.Sum256(data)
	return p.Image{ID: id, View: view, URL: "/api/weighing/agent-images/" + id, SHA256: hex.EncodeToString(sum[:])}, nil
}
func (h *Hub) Image(id string) (p.Image, error) {
	if h.children != nil {
		scale, _ := strconv.ParseInt(strings.Split(id, "_")[0], 10, 64)
		if child := h.children[scale]; child != nil {
			return child.Image(id)
		}
		return p.Image{}, fmt.Errorf("invalid image reference")
	}
	if !imageName.MatchString(id) || !strings.HasPrefix(id, strconv.FormatInt(h.cfg.ScaleID, 10)+"_") {
		return p.Image{}, fmt.Errorf("invalid image reference")
	}
	data, err := os.ReadFile(filepath.Join(h.cfg.ImageDir, id))
	if err != nil {
		return p.Image{}, err
	}
	view := "front"
	if strings.Contains(id, "_rear.") {
		view = "rear"
	}
	sum := sha256.Sum256(data)
	return p.Image{ID: id, View: view, URL: "/api/weighing/agent-images/" + id, SHA256: hex.EncodeToString(sum[:])}, nil
}
func (h *Hub) ServeImage(c *gin.Context) {
	id := c.Param("imageId")
	if _, err := h.Image(id); err != nil {
		ph.WriteError(c, 404, 404, "图片不存在", nil)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400, immutable")
	scale, _ := strconv.ParseInt(strings.Split(id, "_")[0], 10, 64)
	child := h.forScale(scale)
	if child == nil {
		ph.WriteError(c, 404, 404, "图片不存在", nil)
		return
	}
	c.File(filepath.Join(child.cfg.ImageDir, id))
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
