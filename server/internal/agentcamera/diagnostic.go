package agentcamera

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"sync"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

// Test acquires fresh frames on the Agent and returns bounded thumbnails in the
// correlated diagnostic response. It never uploads evidence or reads a scale.
func (c Client) Test(parent context.Context) p.CameraTest {
	timeout := c.Config.CameraTimeoutMS
	if timeout == 0 {
		timeout = 2000
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	result := p.CameraTest{At: time.Now().UTC(), Views: make([]p.CameraTestView, 2)}
	var wg sync.WaitGroup
	for i, endpoint := range []string{c.Config.CameraFrontURL, c.Config.CameraRearURL} {
		wg.Add(1)
		go func(i int, endpoint string) {
			defer wg.Done()
			v := p.CameraTestView{View: []string{"front", "rear"}[i]}
			data, err := c.snapshot(ctx, endpoint)
			if err == nil {
				var source image.Image
				source, _, err = image.Decode(bytes.NewReader(data))
				if err == nil {
					bounds := source.Bounds()
					w, h := bounds.Dx(), bounds.Dy()
					width, height := 320, 180
					if w*height > h*width {
						height = h * width / w
					} else {
						width = w * height / h
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
							thumb.Set(x, y, source.At(bounds.Min.X+x*w/width, bounds.Min.Y+y*h/height))
						}
					}
					var out bytes.Buffer
					err = jpeg.Encode(&out, thumb, &jpeg.Options{Quality: 55})
					if err == nil && out.Len() <= 40<<10 {
						v.JPEG = out.Bytes()
						v.Success = true
					}
				}
			}
			if !v.Success {
				v.Error = "摄像头测试抓拍失败，请检查连接、权限和超时"
			}
			result.Views[i] = v
		}(i, endpoint)
	}
	wg.Wait()
	result.At = time.Now().UTC()
	return result
}
