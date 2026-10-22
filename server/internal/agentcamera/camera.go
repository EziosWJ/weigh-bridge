// Package agentcamera acquires fresh camera evidence for a single capture.
// It never reuses a previously captured image after a camera failure.
package agentcamera

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

const maxImage = 10 << 20

// Client uses the existing authenticated Agent image upload contract. URLs may
// contain device credentials; errors deliberately omit URLs and device bodies.
type Client struct {
	Config agentprinter.Config
	HTTP   *http.Client
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

// Capture gets both views concurrently. The caller must check the scale again
// before confirming that the images correspond to the same stable weight.
func (c Client) Capture(ctx context.Context) ([]p.Image, p.Plate, error) {
	if c.Config.CameraFrontURL == "" || c.Config.CameraRearURL == "" {
		return nil, p.Plate{}, errors.New("摄像头未接入，须配置前后抓拍地址")
	}
	timeout := c.Config.CameraTimeoutMS
	if timeout == 0 {
		timeout = 2000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	var data [2][]byte
	var errs [2]error
	var wg sync.WaitGroup
	for i, endpoint := range []string{c.Config.CameraFrontURL, c.Config.CameraRearURL} {
		wg.Add(1)
		go func(i int, endpoint string) { defer wg.Done(); data[i], errs[i] = c.snapshot(ctx, endpoint) }(i, endpoint)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, p.Plate{}, fmt.Errorf("%s摄像头抓拍失败", []string{"前", "后"}[i])
		}
	}
	images := make([]p.Image, 0, 2)
	for i, view := range []string{"front", "rear"} {
		v, err := c.upload(ctx, view, data[i])
		if err != nil {
			return nil, p.Plate{}, err
		}
		images = append(images, v)
	}
	plate := p.Plate{Failed: true, At: time.Now().UTC()}
	if c.Config.PlateRecognitionURL != "" {
		// A failed recognition leaves the candidate unavailable. Valid image
		// evidence can still be reviewed with a manually checked plate.
		if candidate, err := c.recognize(ctx, data[0]); err == nil && strings.TrimSpace(candidate) != "" {
			plate.Candidate, plate.Failed = strings.TrimSpace(candidate), false
		}
	}
	return images, plate, nil
}

func (c Client) snapshot(ctx context.Context, endpoint string) ([]byte, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.New("摄像头地址无效")
	}
	var data []byte
	if u.Scheme == "rtsp" {
		// Read one newly decoded frame; no shell and no persistent video cache.
		cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-rtsp_transport", "tcp", "-i", endpoint, "-frames:v", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1")
		out := &limitedBuffer{limit: maxImage}
		cmd.Stdout = out
		if cmd.Run() != nil {
			return nil, errors.New("RTSP 抓拍失败")
		}
		data = out.Bytes()
	} else if u.Scheme == "http" || u.Scheme == "https" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, errors.New("摄像头地址无效")
		}
		req.Header.Set("Cache-Control", "no-cache, no-store")
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return nil, errors.New("摄像头请求失败")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New("摄像头拒绝抓拍")
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
		if err != nil {
			return nil, errors.New("摄像头图片读取失败")
		}
	} else {
		return nil, errors.New("摄像头协议无效")
	}
	info, format, err := image.DecodeConfig(bytes.NewReader(data))
	if len(data) > maxImage || err != nil || (format != "jpeg" && format != "png") || info.Width < 1 || info.Height < 1 || int64(info.Width)*int64(info.Height) > 40000000 {
		return nil, errors.New("摄像头未返回有效 PNG/JPEG 图片")
	}
	return data, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("图片超过 10MB")
	}
	return b.Buffer.Write(p)
}

func imageBody(view string, data []byte) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("view", view); err != nil {
		return nil, "", err
	}
	part, err := w.CreateFormFile("file", view+".jpg")
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &body, w.FormDataContentType(), nil
}

func (c Client) upload(ctx context.Context, view string, data []byte) (p.Image, error) {
	body, contentType, err := imageBody(view, data)
	if err != nil {
		return p.Image{}, errors.New("图片请求构建失败")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.Config.ServerURL, "/")+"/api/agent/images", body)
	if err != nil {
		return p.Image{}, errors.New("本地图片地址无效")
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+c.Config.AgentKey)
	req.Header.Set("X-Agent-ID", c.Config.AgentID)
	req.Header.Set("X-Scale-ID", strconv.FormatInt(c.Config.ScaleID, 10))
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return p.Image{}, errors.New("抓拍上传失败")
	}
	defer resp.Body.Close()
	var envelope struct {
		Code int     `json:"code"`
		Data p.Image `json:"data"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope) != nil || envelope.Code != 200 {
		return p.Image{}, errors.New("本地服务未确认抓拍保存")
	}
	sum := sha256.Sum256(data)
	if envelope.Data.ID == "" || envelope.Data.View != view || envelope.Data.SHA256 != hex.EncodeToString(sum[:]) {
		return p.Image{}, errors.New("抓拍保存校验失败")
	}
	return envelope.Data, nil
}

// Recognition adapter: POST multipart file; response {"candidate":"川A12345"}.
// Device-specific or HyperLPR adapters can implement this local HTTP contract.
func (c Client) recognize(ctx context.Context, data []byte) (string, error) {
	body, contentType, err := imageBody("front", data)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Config.PlateRecognitionURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Candidate string `json:"candidate"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result) != nil || len([]rune(result.Candidate)) > 16 {
		return "", errors.New("识别结果无效")
	}
	return result.Candidate, nil
}
