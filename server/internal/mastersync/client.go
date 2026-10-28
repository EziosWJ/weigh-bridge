package mastersync

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseSize = 16 << 20

// Fetch reads one complete snapshot. It never includes the response
// body or transport error in its errors, which are suitable for sync logs.
func Fetch(ctx context.Context, baseURL, site string) (Snapshot, error) {
	return fetch(ctx, baseURL, site, &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func fetch(ctx context.Context, baseURL, site string, client *http.Client) (Snapshot, error) {
	var empty Snapshot
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return empty, errors.New("云端地址无效")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/weighbridge/sync/master"
	u.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return empty, errors.New("云端请求无效")
	}
	q := req.URL.Query()
	q.Set("siteCode", site)
	req.URL.RawQuery = q.Encode()
	resp, err := client.Do(req)
	if err != nil {
		return empty, errors.New("连接云端失败或超时")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized {
			return empty, errors.New("云端拒绝厂区编码：编码不存在或已停用")
		}
		return empty, errors.New("云端 HTTP 响应失败")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil || len(body) > maxResponseSize {
		return empty, errors.New("云端响应读取失败或过大")
	}
	return Decode(body, site)
}
