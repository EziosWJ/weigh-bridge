//go:build embedweb

package app

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildServesEmbeddedFrontend(t *testing.T) {
	router, err := Build(testConfig("prod", false), readyProbe{}, newFakeStores().deps())
	if err != nil {
		t.Fatal(err)
	}
	// Requests continue to work with an empty working directory: the release
	// must never rely on web/dist or other files on disk at runtime.
	t.Chdir(t.TempDir())
	for _, target := range []string{"/", "/login", "/system/user", "/system/role"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<div id="root">`) {
			t.Fatalf("GET %s: status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
}

func TestBuildServesAllBundledFiles(t *testing.T) {
	router, err := Build(testConfig("prod", false), readyProbe{}, newFakeStores().deps())
	if err != nil {
		t.Fatal(err)
	}
	assets := os.DirFS("../webui/dist")
	err = fs.WalkDir(assets, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		want, err := fs.ReadFile(assets, name)
		if err != nil {
			return err
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+filepath.ToSlash(name), nil))
		if response.Code != http.StatusOK || response.Body.String() != string(want) {
			t.Errorf("GET /%s: status=%d; bundled content differs", name, response.Code)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
