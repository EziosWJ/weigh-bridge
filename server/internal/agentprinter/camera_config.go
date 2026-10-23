package agentprinter

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// CameraSettings contains no credentials. Empty Mode preserves legacy URLs.
type CameraSettings struct {
	Mode      string `json:"mode"`
	Enabled   bool   `json:"enabled"`
	RTSPBase  string `json:"rtspBase"`
	WHEPBase  string `json:"whepBase"`
	FrontPath string `json:"frontPath"`
	RearPath  string `json:"rearPath"`
}

func (c Config) CameraSettings() CameraSettings {
	return CameraSettings{c.CameraMode, c.CameraEnabled, c.CameraRTSPBase, c.CameraWHEPBase, c.CameraFrontPath, c.CameraRearPath}
}
func (c *Config) SetCameraSettings(v CameraSettings) {
	c.CameraMode, c.CameraEnabled, c.CameraRTSPBase, c.CameraWHEPBase, c.CameraFrontPath, c.CameraRearPath = v.Mode, v.Enabled, v.RTSPBase, v.WHEPBase, v.FrontPath, v.RearPath
	c.ResolveCameraURLs()
}
func (c *Config) ResolveCameraURLs() {
	if c.CameraMode != "mediamtx" {
		return
	}
	c.CameraFrontURL, c.CameraRearURL = "", ""
	if !c.CameraEnabled {
		return
	}
	c.CameraFrontURL = c.CameraRTSPURL(c.CameraFrontPath)
	c.CameraRearURL = c.CameraRTSPURL(c.CameraRearPath)
}
func (c Config) CameraRTSPURL(path string) string {
	return strings.TrimRight(c.CameraRTSPBase, "/") + "/" + path
}
func (c Config) CameraWHEPURL(path string) string {
	return strings.TrimRight(c.CameraWHEPBase, "/") + "/" + path + "/whep"
}
func validateMediaCamera(c Config) error {
	if c.CameraMode == "" {
		return nil
	}
	if c.CameraMode != "mediamtx" {
		return errors.New("摄像头配置模式无效")
	}
	// An empty disabled configuration is useful during installation.
	if !c.CameraEnabled && c.CameraRTSPBase == "" && c.CameraWHEPBase == "" && c.CameraFrontPath == "" && c.CameraRearPath == "" {
		return nil
	}
	for _, v := range []struct {
		raw  string
		rtsp bool
	}{{c.CameraRTSPBase, true}, {c.CameraWHEPBase, false}} {
		u, err := url.Parse(v.raw)
		if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(v.raw, "\r\n\x00") || (v.rtsp && (u.Scheme != "rtsp" || strings.Trim(u.Path, "/") != "")) || (!v.rtsp && u.Scheme != "http" && u.Scheme != "https") || strings.Contains(u.Path, "..") {
			return errors.New("MediaMTX 服务入口无效：须填写无凭据、无查询参数的 RTSP / HTTP(S) 入口")
		}
	}
	valid := regexp.MustCompile(`^[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*$`)
	if len(c.CameraFrontPath) > 128 || len(c.CameraRearPath) > 128 || !valid.MatchString(c.CameraFrontPath) || !valid.MatchString(c.CameraRearPath) || c.CameraFrontPath == c.CameraRearPath {
		return errors.New("前后流名称须不同，仅支持字母、数字、下划线、短横线及路径分隔符")
	}
	return nil
}
