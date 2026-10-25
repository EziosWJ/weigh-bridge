package camera

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var errRemotePathNotFound = errors.New("MediaMTX path not found")

// Endpoint is the Server-reachable MediaMTX Control API configuration.
type Endpoint struct {
	URL      string
	Username string
	Password string
}

// Control manages only explicitly named paths owned by this system. It never
// lists or mutates unrelated streams in a shared MediaMTX instance.
type Control interface {
	Get(ctx context.Context, endpoint Endpoint, name string) (source string, exists bool, err error)
	Add(ctx context.Context, endpoint Endpoint, name, source string) error
	Patch(ctx context.Context, endpoint Endpoint, name, source string) error
	Delete(ctx context.Context, endpoint Endpoint, name string) error
}

// HTTPControl implements the MediaMTX v3 Control API used by the current
// deployment baseline. Error messages intentionally exclude the endpoint URL
// and response body, which can contain source credentials.
type HTTPControl struct {
	HTTP *http.Client
}

func (c HTTPControl) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func controlURL(endpoint Endpoint, action, name string) (string, error) {
	base := strings.TrimSpace(endpoint.URL)
	if base == "" {
		return "", errors.New("MediaMTX management endpoint is not configured")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("MediaMTX management endpoint is invalid")
	}
	return strings.TrimRight(base, "/") + "/v3/config/paths/" + action + "/" + url.PathEscape(name), nil
}

func (c HTTPControl) request(ctx context.Context, endpoint Endpoint, method, action, name string, body any) (*http.Response, error) {
	target, err := controlURL(endpoint, action, name)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if endpoint.Username != "" {
		request.SetBasicAuth(endpoint.Username, endpoint.Password)
	}
	return c.client().Do(request)
}

func (c HTTPControl) Get(ctx context.Context, endpoint Endpoint, name string) (string, bool, error) {
	response, err := c.request(ctx, endpoint, http.MethodGet, "get", name, nil)
	if err != nil {
		return "", false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	if response.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("MediaMTX 管理接口返回 %d", response.StatusCode)
	}
	var path struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&path); err != nil {
		return "", false, errors.New("MediaMTX 管理接口响应无效")
	}
	return path.Source, true, nil
}

func (c HTTPControl) Add(ctx context.Context, endpoint Endpoint, name, source string) error {
	response, err := c.request(ctx, endpoint, http.MethodPost, "add", name, map[string]any{
		"source":         source,
		"sourceOnDemand": false,
	})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusConflict {
		return ErrConflict
	}
	return fmt.Errorf("MediaMTX 管理接口返回 %d", response.StatusCode)
}

func (c HTTPControl) Patch(ctx context.Context, endpoint Endpoint, name, source string) error {
	response, err := c.request(ctx, endpoint, http.MethodPatch, "patch", name, map[string]any{"source": source})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if response.StatusCode == http.StatusNotFound {
		return errRemotePathNotFound
	}
	return fmt.Errorf("MediaMTX 管理接口返回 %d", response.StatusCode)
}

func (c HTTPControl) Delete(ctx context.Context, endpoint Endpoint, name string) error {
	response, err := c.request(ctx, endpoint, http.MethodDelete, "delete", name, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("MediaMTX 管理接口返回 %d", response.StatusCode)
}

func managedPath(publicID string) string { return "wb-" + publicID }

func playbackURL(whepBase, path string) string {
	if strings.TrimSpace(whepBase) == "" || strings.TrimSpace(path) == "" {
		return ""
	}
	return strings.TrimRight(whepBase, "/") + "/" + path + "/whep"
}

// sourceWithCredentials is used only for the MediaMTX management request so
// the remote service can pull the RTSP source. It is never returned to a
// browser, logged or written to image metadata.
func sourceWithCredentials(source, username, password string) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("camera source is invalid")
	}
	if username != "" {
		parsed.User = url.UserPassword(username, password)
	}
	return parsed.String(), nil
}
