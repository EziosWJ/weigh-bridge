// Package webui serves the frontend bundled into a release binary.
package webui

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	platformhttp "github.com/EziosWJ/weighbridge/server/internal/platform/http"
)

// Register installs the SPA fallback without taking over API or system routes.
// Ordinary Go builds keep the API-only development behavior.
func Register(router *gin.Engine) error {
	assets := embeddedAssets()
	if assets == nil {
		return nil
	}
	handler, err := newHandler(assets)
	if err != nil {
		return err
	}
	router.NoRoute(handler)
	return nil
}

func newHandler(assets fs.FS) (gin.HandlerFunc, error) {
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read embedded frontend index: %w", err)
	}
	files := http.FileServer(http.FS(assets))
	return func(c *gin.Context) {
		name := strings.TrimSuffix(strings.TrimPrefix(c.Request.URL.Path, "/"), "/")
		if (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) || reservedPath(name) || (name != "" && !fs.ValidPath(name)) {
			platformhttp.NotFoundHandler(c)
			return
		}

		if name != "" && name != "index.html" {
			info, err := fs.Stat(assets, name)
			if err == nil && !info.IsDir() {
				c.Header("Cache-Control", "no-cache")
				files.ServeHTTP(c.Writer, c.Request)
				return
			}
			// Missing resources must not return HTML or directory listings.
			if path.Ext(name) != "" || name == "assets" || strings.HasPrefix(name, "assets/") {
				platformhttp.NotFoundHandler(c)
				return
			}
		}

		c.Header("Cache-Control", "no-cache")
		http.ServeContent(c.Writer, c.Request, "index.html", time.Time{}, bytes.NewReader(index))
	}, nil
}

func reservedPath(name string) bool {
	for _, prefix := range []string{"api", "health", "ready", "metrics", "swagger"} {
		if name == prefix || strings.HasPrefix(name, prefix+"/") {
			return true
		}
	}
	return false
}
