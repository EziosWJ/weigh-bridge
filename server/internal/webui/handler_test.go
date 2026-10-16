package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestFrontendRoutes(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":            {Data: []byte("<!doctype html><title>Base Admin</title>")},
		"assets/index-hash.js":  {Data: []byte("console.log('frontend')")},
		"assets/index-hash.css": {Data: []byte("body { color: black; }")},
		"login/svg/logo.svg":    {Data: []byte("<svg></svg>")},
	}
	handler, err := newHandler(assets)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.NoRoute(handler)
	router.GET("/api/example", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 0}) })

	for _, tc := range []struct {
		method, target, contentType, body string
		status                            int
	}{
		{http.MethodGet, "/", "text/html", "Base Admin", 200},
		{http.MethodGet, "/index.html", "text/html", "Base Admin", 200},
		{http.MethodGet, "/login", "text/html", "Base Admin", 200},
		{http.MethodGet, "/system/user?keyword=test", "text/html", "Base Admin", 200},
		{http.MethodGet, "/system/user/", "text/html", "Base Admin", 200},
		{http.MethodHead, "/login", "text/html", "", 200},
		{http.MethodGet, "/assets/index-hash.js", "javascript", "console.log", 200},
		{http.MethodGet, "/assets/index-hash.css", "text/css", "color: black", 200},
		{http.MethodHead, "/assets/index-hash.js", "javascript", "", 200},
		{http.MethodGet, "/login/svg/logo.svg", "image/svg+xml", "<svg>", 200},
		{http.MethodGet, "/api/example", "application/json", `"code":0`, 200},
		{http.MethodGet, "/api", "application/json", `"code":404`, 404},
		{http.MethodGet, "/api/missing", "application/json", `"code":404`, 404},
		{http.MethodGet, "/health/missing", "application/json", `"code":404`, 404},
		{http.MethodHead, "/ready", "application/json", "", 404},
		{http.MethodGet, "/metrics/missing", "application/json", `"code":404`, 404},
		{http.MethodGet, "/swagger/index.html", "application/json", `"code":404`, 404},
		{http.MethodPost, "/login", "application/json", `"code":404`, 404},
		{http.MethodGet, "/assets/missing.js", "application/json", `"code":404`, 404},
		{http.MethodGet, "/assets/missing", "application/json", `"code":404`, 404},
		{http.MethodGet, "/assets/", "application/json", `"code":404`, 404},
		{http.MethodGet, "/login/svg/missing.svg", "application/json", `"code":404`, 404},
		{http.MethodGet, "/missing.png", "application/json", `"code":404`, 404},
		{http.MethodGet, "/../index.html", "application/json", `"code":404`, 404},
		{http.MethodGet, "/assets/%2e%2e/index.html", "application/json", `"code":404`, 404},
	} {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(tc.method, tc.target, nil))
			if response.Code != tc.status || !strings.Contains(response.Header().Get("Content-Type"), tc.contentType) {
				t.Fatalf("status=%d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
			}
			if !strings.Contains(response.Body.String(), tc.body) {
				t.Fatalf("body=%s, want %q", response.Body.String(), tc.body)
			}
			if tc.method == http.MethodHead && tc.status == http.StatusOK && response.Body.Len() != 0 {
				t.Fatalf("HEAD returned a body: %s", response.Body.String())
			}
			if tc.status == http.StatusOK && tc.target != "/api/example" && response.Header().Get("Cache-Control") != "no-cache" {
				t.Fatal("frontend response must revalidate before reuse")
			}
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/assets/index-hash.js", nil)
	request.Header.Set("Range", "bytes=0-6")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || response.Body.String() != "console" {
		t.Fatalf("range response=%d %q", response.Code, response.Body.String())
	}
}

func TestFrontendRequiresIndex(t *testing.T) {
	if _, err := newHandler(fstest.MapFS{}); err == nil {
		t.Fatal("expected missing index error")
	}
}
