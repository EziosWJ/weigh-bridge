package agentprinter

// CameraPreview contains only public MediaMTX playback addresses. Reading the
// mapping uses the business API; streaming goes directly to MediaMTX.
type CameraPreview struct {
	Mode     string `json:"mode"`
	Enabled  bool   `json:"enabled"`
	Ready    bool   `json:"ready"`
	Front    string `json:"front,omitempty"`
	Rear     string `json:"rear,omitempty"`
	Revision string `json:"revision,omitempty"`
	Message  string `json:"message,omitempty"`
}

func (c Config) PreviewConfig(runtimeError error) CameraPreview {
	if c.CameraMode == "" {
		return CameraPreview{Mode: "legacy"}
	}
	result := CameraPreview{Mode: c.CameraMode, Enabled: c.CameraEnabled}
	if !c.CameraEnabled {
		result.Message = "摄像头已停用"
		return result
	}
	if runtimeError != nil {
		result.Message = runtimeError.Error()
		return result
	}
	result.Ready = true
	result.Front, result.Rear = c.CameraWHEPURL(c.CameraFrontPath), c.CameraWHEPURL(c.CameraRearPath)
	result.Revision = ConfigDigest(c)
	return result
}
