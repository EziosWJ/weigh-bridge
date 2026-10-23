//go:build mediamtx_integration

package agentprinter

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// This opt-in contract runs the pinned real media server, FFmpeg publishers,
// shipped WHEP reader and Chromium. It never uses a business database.
func TestRealMediaMTXPreviewAndCapture(t *testing.T) {
	binary := os.Getenv("MEDIAMTX_BIN")
	if binary == "" {
		t.Fatal("set MEDIAMTX_BIN to a MediaMTX v1.21.2 executable")
	}
	version, err := exec.Command(binary, "--version").Output()
	if err != nil || strings.TrimSpace(string(version)) != "v1.21.2" {
		t.Fatal("this contract requires MediaMTX v1.21.2")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	freeTCP := func() string {
		l, e := net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		address := l.Addr().String()
		_ = l.Close()
		return address
	}
	rtsp, whep := freeTCP(), freeTCP()
	udpListener, e := net.ListenPacket("udp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	udp := udpListener.LocalAddr().String()
	_ = udpListener.Close()
	cfg := Defaults()
	cfg.AgentKey = "isolated-agent-secret"
	cfg.SetCameraSettings(CameraSettings{Mode: "mediamtx", Enabled: true, RTSPBase: "rtsp://" + rtsp, WHEPBase: "http://" + whep, FrontPath: "front", RearPath: "rear"})
	router := gin.New()
	router.GET("/preview-config", func(c *gin.Context) { c.JSON(200, map[string]any{"data": cfg.PreviewConfig(nil)}) })
	router.GET("/fixture", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(200, `<video id="video" autoplay muted playsinline></video><script type="module">
import Reader from '/reader.js';
window.start=async(view)=>{const r=await fetch('/preview-config');const body=await r.json();window.reader?.close();video.srcObject=null;window.activeView='';const cfg=body.data;window.reader=new Reader({url:cfg[view],onError:e=>{window.mediaError=e},onTrack:e=>{video.srcObject=e.streams[0];window.startFrames=video.getVideoPlaybackQuality().totalVideoFrames;window.activeView=view;video.play();}});};
</script>`)
	})
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	router.GET("/reader.js", func(c *gin.Context) { c.File(filepath.Join(root, "web/src/lib/vendor/mediamtx/reader.js")) })
	server := httptest.NewServer(router)
	defer server.Close()
	dir := t.TempDir()
	config := fmt.Sprintf("logLevel: info\nrtspTransports: [tcp]\nrtspAddress: %s\nrtmp: false\nhls: false\nsrt: false\nplayback: false\nmoq: false\nwebrtcAddress: %s\nwebrtcLocalUDPAddress: %s\nwebrtcIPsFromInterfaces: false\nwebrtcAdditionalHosts: [127.0.0.1]\nwebrtcICEServers2: []\nwebrtcAllowOrigins: ['*']\npaths:\n  front: {}\n  rear: {}\n", rtsp, whep, udp)
	path := filepath.Join(dir, "mediamtx.yml")
	if err = os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "media.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	start := func(command *exec.Cmd) {
		command.Stdout = logFile
		command.Stderr = logFile
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if command.Process != nil {
				_ = command.Process.Kill()
				_ = command.Wait()
			}
		})
	}
	mediaCommand := exec.CommandContext(ctx, binary, path)
	mediaCommand.Dir = dir
	start(mediaCommand)
	for attempts := 0; ; attempts++ {
		conn, err := net.DialTimeout("tcp", whep, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if attempts > 60 {
			t.Fatal("media server failed to start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	for i, view := range []string{"front", "rear"} {
		start(exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-re", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10", "-an", "-c:v", "libx264", "-threads", "1", "-preset", "ultrafast", "-tune", "zerolatency", "-pix_fmt", "yuv420p", "-profile:v", "baseline", "-g", strconv.Itoa(10+i), "-f", "rtsp", "-rtsp_transport", "tcp", "rtsp://"+rtsp+"/"+view))
	}
	time.Sleep(1200 * time.Millisecond)
	browser := exec.CommandContext(ctx, "node", "tests/camera-mediamtx-contract.mjs", server.URL)
	browser.Dir = filepath.Join(root, "web")
	output, err := browser.CombinedOutput()
	if err != nil {
		logs, _ := os.ReadFile(logPath)
		t.Fatalf("browser contract: %v\n%s\nmedia log: %s", err, output, logs)
	}
	t.Log(strings.TrimSpace(string(output)))
	// Agent capture uses the same generated path as browser preview.
	frame := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-rtsp_transport", "tcp", "-i", cfg.CameraFrontURL, "-frames:v", "1", "-f", "null", "-")
	if output, err := frame.CombinedOutput(); err != nil {
		t.Fatalf("Agent RTSP capture: %v %s", err, output)
	}
}
