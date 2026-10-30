package agentmaintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

// A legacy or hand-written agent.env may still carry site/scale display
// metadata that BuildAgentEnvironmentAt does not write back. The running Agent
// reports the digest of the configuration it read; maintenance compares that
// digest with the candidate before accepting an apply, so display metadata must
// not make an otherwise identical configuration roll back.
func TestAgentEnvironmentRoundTripKeepsConfigDigest(t *testing.T) {
	c := Defaults()
	c.SiteCode, c.SiteName = "NEW", "新厂"
	c.ScaleCode, c.ScaleName = "001", "新厂地磅"
	c.ServerURL = "http://127.0.0.1:8900"
	c.AgentID, c.AgentKey = "001", "pairing-key"
	c.Queue = "Aisino-SK820-raw"
	c.SerialPort, c.SerialBaud = "/dev/pts/18", 9600
	c.CameraFrontURL = "rtsp://192.168.1.10:8554/front"
	c.CameraRearURL = "rtsp://192.168.1.10:8554/rear"
	c.CameraTimeoutMS = 6000
	c.CameraMode = "mediamtx"
	c.CameraEnabled = true
	c.CameraRTSPBase = "rtsp://192.168.1.10:8554"
	c.CameraWHEPBase = "http://192.168.1.10:8889"
	c.CameraFrontPath, c.CameraRearPath = "front", "rear"
	c = agentprinter.NormalizeConfig(c)

	path := filepath.Join(t.TempDir(), "agent.env")
	if err := BuildAgentEnvironmentAt(path, c); err != nil {
		t.Fatal(err)
	}
	reloaded, err := agentprinter.ReadEnvironment(path, agentprinter.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if agentprinter.ConfigDigest(reloaded) != agentprinter.ConfigDigest(c) {
		t.Fatalf("agent.env 往返后配置摘要变化：写入 %s，读回 %s", agentprinter.ConfigDigest(c), agentprinter.ConfigDigest(reloaded))
	}
	if reloaded.SerialPort != c.SerialPort || reloaded.SerialBaud != c.SerialBaud || reloaded.CameraSettings() != c.CameraSettings() || reloaded.Queue != c.Queue {
		t.Fatalf("agent.env 往返丢失设备参数：%+v", reloaded)
	}
}

// A hand-written or legacy agent.env can still hold the site/scale display
// lines that agent-provision now deprecates. Saving and applying a device
// parameter must still be accepted: the reloaded Agent reports no display
// metadata, and that alone must not look like an unloaded configuration.
func TestApplySerialPortWithLegacyDisplayMetadata(t *testing.T) {
	dir := t.TempDir()
	environment := filepath.Join(dir, "agent.env")
	legacy := `WEIGHBRIDGE_SERVER_URL="http://127.0.0.1:8900"
WEIGHBRIDGE_AGENT_ID="001"
WEIGHBRIDGE_SITE_ID=1
WEIGHBRIDGE_SITE_CODE="NEW"
WEIGHBRIDGE_SITE_NAME="新厂"
WEIGHBRIDGE_SCALE_CODE="001"
WEIGHBRIDGE_SCALE_NAME="新厂地磅"
WEIGHBRIDGE_PRINT_QUEUE="Aisino-SK820-raw"
WEIGHBRIDGE_SERIAL_PORT="/dev/pts/4"
WEIGHBRIDGE_SERIAL_BAUD=9600
WEIGHBRIDGE_CAMERA_CONFIG="{\"mode\":\"mediamtx\",\"enabled\":true,\"rtspBase\":\"rtsp://192.168.1.10:8554\",\"whepBase\":\"http://192.168.1.10:8889\",\"frontPath\":\"front\",\"rearPath\":\"rear\"}"
`
	if err := os.WriteFile(environment, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	initial, err := agentprinter.ReadEnvironment(environment, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenDevice(dir, environment, initial, cameraActivationRunner{dir: dir, match: true})
	if err != nil {
		t.Fatal(err)
	}
	result := s.ApplySettings(context.Background(), p.ConfigApplyRequest{Settings: p.DeviceSettings{
		Serial: p.SerialSettings{Port: "/dev/pts/18", Baud: 9600},
		Print: p.PrintSettings{
			Queue: initial.Queue, FormLengthMM: initial.FormLengthMM, PrintWidthMM: initial.PrintWidthMM,
			PrintScale: initial.PrintScale, OffsetXMM: initial.OffsetXMM, OffsetYMM: initial.OffsetYMM,
		},
	}})
	if result.Status != p.ConfigApplyApplied {
		t.Fatalf("应用串口配置被回退：%s %s", result.Status, result.Message)
	}
	reloaded, err := agentprinter.ReadEnvironment(environment, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SerialPort != "/dev/pts/18" {
		t.Fatalf("agent.env 未写入新串口：%q", reloaded.SerialPort)
	}
}
