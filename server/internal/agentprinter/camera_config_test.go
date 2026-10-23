package agentprinter

import (
	"strings"
	"testing"
)

func TestMediaCameraMappingAndDisable(t *testing.T) {
	c := Defaults()
	c.AgentKey = "private-agent-key"
	c.SetCameraSettings(CameraSettings{Mode: "mediamtx", Enabled: true, RTSPBase: "rtsp://media:8554", WHEPBase: "https://video.example/cameras", FrontPath: "scale/front", RearPath: "scale/rear"})
	if err := ValidateConfig(c); err != nil {
		t.Fatal(err)
	}
	if c.CameraFrontURL != "rtsp://media:8554/scale/front" || c.CameraWHEPURL(c.CameraRearPath) != "https://video.example/cameras/scale/rear/whep" {
		t.Fatalf("wrong mapping: %+v", c.CameraSettings())
	}
	c.CameraEnabled = false
	c = NormalizeConfig(c)
	if c.CameraFrontURL != "" || c.CameraRearURL != "" || c.CameraFrontPath != "scale/front" {
		t.Fatal("disable must clear execution URLs but preserve mapping")
	}
	c.CameraEnabled = true
	c = NormalizeConfig(c)
	if !strings.Contains(c.CameraFrontURL, "scale/front") {
		t.Fatal("enable lost mapping")
	}
}
func TestMediaCameraRejectsAmbiguousOrCredentialedInputs(t *testing.T) {
	base := Defaults()
	base.SetCameraSettings(CameraSettings{Mode: "mediamtx", Enabled: true, RTSPBase: "rtsp://media:8554", WHEPBase: "http://video:8889", FrontPath: "front", RearPath: "rear"})
	for _, edit := range []func(*Config){func(c *Config) { c.CameraRTSPBase = "rtsp://user:pass@media:8554" }, func(c *Config) { c.CameraWHEPBase = "http://video?token=secret" }, func(c *Config) { c.CameraRearPath = "front" }, func(c *Config) { c.CameraFrontPath = "../front" }, func(c *Config) { c.CameraMode = "other" }} {
		c := base
		edit(&c)
		if ValidateConfig(c) == nil {
			t.Fatal("invalid camera config accepted")
		}
	}
	legacy := Defaults()
	legacy.CameraFrontURL = "http://camera/front.jpg"
	legacy.CameraRearURL = "rtsp://user:pass@camera/rear"
	if got := NormalizeConfig(legacy); got.CameraFrontURL != legacy.CameraFrontURL || got.CameraRearURL != legacy.CameraRearURL {
		t.Fatal("legacy was implicitly migrated")
	}
}

func TestCameraPreviewUsesPlainMediaAddresses(t *testing.T) {
	c := Defaults()
	c.AgentKey = "business-device-secret"
	c.SetCameraSettings(CameraSettings{Mode: "mediamtx", Enabled: true, RTSPBase: "rtsp://media:8554", WHEPBase: "http://media:8889", FrontPath: "front", RearPath: "rear"})
	preview := c.PreviewConfig(nil)
	if !preview.Ready || preview.Front != "http://media:8889/front/whep" || c.CameraFrontURL != "rtsp://media:8554/front" {
		t.Fatal("preview and capture must use direct media paths")
	}
	c.CameraEnabled = false
	if c.PreviewConfig(nil).Ready {
		t.Fatal("disabled mapping became playable")
	}
}
