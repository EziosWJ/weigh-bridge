package agentprinter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCameraEnvironmentAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	text := `WEIGHBRIDGE_SERVER_URL=http://127.0.0.1:18900
WEIGHBRIDGE_AGENT_ID=agent-site01
WEIGHBRIDGE_SCALE_ID=11
WEIGHBRIDGE_PRINT_QUEUE=Aisino-SK820-raw
WEIGHBRIDGE_CAMERA_FRONT_URL="rtsp://camera-user:private@192.0.2.1/front"
WEIGHBRIDGE_CAMERA_REAR_URL="http://192.0.2.2/snapshot"
WEIGHBRIDGE_PLATE_RECOGNITION_URL="http://127.0.0.1:18080/recognize"
WEIGHBRIDGE_CAMERA_TIMEOUT_MS=3000
`
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := ReadEnvironment(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if c.CameraTimeoutMS != 3000 || c.CameraFrontURL != "rtsp://camera-user:private@192.0.2.1/front" || c.PlateRecognitionURL == "" {
		t.Fatal("camera connection configuration lost")
	}
	c.CameraRearURL = ""
	if ValidateConfig(c) == nil {
		t.Fatal("partial camera configuration must be rejected")
	}
	c.CameraRearURL = "file:///etc/passwd"
	if ValidateConfig(c) == nil {
		t.Fatal("unsupported camera scheme accepted")
	}
	c.CameraRearURL = "http://192.0.2.2/snapshot"
	c.PlateRecognitionURL = "rtsp://192.0.2.2/stream"
	if ValidateConfig(c) == nil {
		t.Fatal("recognition must use HTTP")
	}
}
