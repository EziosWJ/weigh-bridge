// The Server owns frame acquisition and recognition. This file is the
// Server-side extraction of the reusable Agent camera behaviour; it does not
// use the old Agent credential upload path.
package camera

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const maxImageBytes = 10 << 20
const maxImagePixels = 40000000

var (
	errFrameUnavailable = errors.New("camera unavailable")
	errFrameTimeout     = errors.New("camera timeout")
	errInvalidFrame     = errors.New("camera returned an invalid image")
)

// Frame is one freshly decoded still image. Format is "jpeg" or "png".
type Frame struct {
	Data   []byte
	Width  int
	Height int
	Format string
}

// FrameGrabber acquires a new frame. Implementations must not reuse a cached
// still image: every call has to read the camera again.
type FrameGrabber interface {
	Snapshot(ctx context.Context, sourceURL, username, password string) (Frame, error)
}

// HTTPGrabber reads RTSP through one ffmpeg process or an HTTP endpoint
// directly. Device credentials are supplied separately from the source URL.
type HTTPGrabber struct {
	HTTP       *http.Client
	FFmpegPath string
	MaxBytes   int
}

func (g HTTPGrabber) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func (g HTTPGrabber) Snapshot(ctx context.Context, sourceURL, username, password string) (Frame, error) {
	endpoint, err := credentialURL(sourceURL, username, password)
	if err != nil {
		return Frame{}, errFrameUnavailable
	}
	parsed, _ := url.Parse(endpoint)
	var data []byte
	switch parsed.Scheme {
	case "rtsp":
		ffmpeg := g.FFmpegPath
		if ffmpeg == "" {
			ffmpeg = "ffmpeg"
		}
		command := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-loglevel", "error",
			"-rtsp_transport", "tcp", "-i", endpoint,
			"-frames:v", "1", "-an", "-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1")
		limit := g.MaxBytes
		if limit <= 0 {
			limit = maxImageBytes
		}
		out := &limitedBuffer{limit: limit}
		command.Stdout = out
		if runErr := command.Run(); runErr != nil {
			if out.overflow {
				return Frame{}, errInvalidFrame
			}
			return Frame{}, contextFrameError(ctx)
		}
		if out.overflow {
			return Frame{}, errInvalidFrame
		}
		data = out.Bytes()
	case "http", "https":
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return Frame{}, errFrameUnavailable
		}
		request.Header.Set("Cache-Control", "no-cache, no-store")
		if username != "" {
			request.SetBasicAuth(username, password)
		}
		response, err := g.client().Do(request)
		if err != nil {
			return Frame{}, contextFrameError(ctx)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return Frame{}, errFrameUnavailable
		}
		limit := g.MaxBytes
		if limit <= 0 {
			limit = maxImageBytes
		}
		data, err = io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
		if err != nil {
			return Frame{}, contextFrameError(ctx)
		}
	default:
		return Frame{}, errFrameUnavailable
	}
	limit := g.MaxBytes
	if limit <= 0 {
		limit = maxImageBytes
	}
	if len(data) > limit || len(data) == 0 {
		return Frame{}, errInvalidFrame
	}
	info, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || info.Width < 1 || info.Height < 1 ||
		int64(info.Width)*int64(info.Height) > maxImagePixels {
		return Frame{}, errInvalidFrame
	}
	return Frame{Data: data, Width: info.Width, Height: info.Height, Format: format}, nil
}

func contextFrameError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errFrameTimeout
	}
	return errFrameUnavailable
}

func credentialURL(sourceURL, username, password string) (string, error) {
	if strings.ContainsAny(sourceURL, "\r\n\x00") || strings.TrimSpace(sourceURL) == "" {
		return "", errFrameUnavailable
	}
	parsed, err := url.Parse(sourceURL)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		(parsed.Scheme != "rtsp" && parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errFrameUnavailable
	}
	if username != "" {
		parsed.User = url.UserPassword(username, password)
	}
	return parsed.String(), nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		b.overflow = true
		return 0, errors.New("image exceeds limit")
	}
	return b.Buffer.Write(p)
}

// Recognizer posts one already captured image to the optional external plate
// recognition endpoint. It never acquires another frame.
type Recognizer interface {
	Recognize(ctx context.Context, endpoint string, data []byte) (string, error)
}

type HTTPRecognizer struct {
	HTTP *http.Client
}

func (r HTTPRecognizer) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func (r HTTPRecognizer) Recognize(ctx context.Context, endpoint string, data []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "camera.jpg")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := r.client().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("recognition unavailable")
	}
	var result struct {
		Candidate string `json:"candidate"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result); err != nil {
		return "", err
	}
	candidate := strings.TrimSpace(result.Candidate)
	if len([]rune(candidate)) > 16 || strings.ContainsAny(candidate, "\r\n\x00") {
		return "", errors.New("invalid recognition result")
	}
	return candidate, nil
}

func thumbnailJPEG(data []byte) ([]byte, int, int, error) {
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	bounds := source.Bounds()
	width, height := 320, 180
	if bounds.Dx()*height > bounds.Dy()*width {
		height = bounds.Dy() * width / bounds.Dx()
	} else {
		width = bounds.Dx() * height / bounds.Dy()
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	thumb := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			thumb.Set(x, y, source.At(bounds.Min.X+x*bounds.Dx()/width, bounds.Min.Y+y*bounds.Dy()/height))
		}
	}
	for _, quality := range []int{55, 40, 25} {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, thumb, &jpeg.Options{Quality: quality}); err != nil {
			return nil, 0, 0, err
		}
		if out.Len() <= 40<<10 {
			return out.Bytes(), bounds.Dx(), bounds.Dy(), nil
		}
	}
	return nil, 0, 0, errors.New("thumbnail too large")
}

func imageDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func imageExtension(format string) string {
	if format == "png" {
		return "png"
	}
	return "jpg"
}

func recognitionTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 5*time.Second)
}
