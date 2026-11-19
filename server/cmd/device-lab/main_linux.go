// device-lab runs the local D12 and plate simulators and owns their Agent.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/camsim"
	"github.com/EziosWJ/weighbridge/server/internal/d12sim"
)

//go:embed index.html
var page []byte

func main() {
	listen := flag.String("listen", "127.0.0.1:8093", "控制页回环监听地址")
	server := flag.String("server", "http://127.0.0.1:8900", "开发 Server 根地址")
	serial := flag.String("serial", "camera-sim-01", "测试库绑定的摄像头序列号")
	heartbeat := flag.Duration("heartbeat", camsim.DefaultHeartbeatInterval, "摄像头心跳间隔")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	binary, err := os.Executable()
	if err == nil {
		err = serve(ctx, *listen, filepath.Join(filepath.Dir(binary), "agent-printer"), camsim.Config{
			ServerURL: *server, SerialNumber: *serial, HeartbeatInterval: *heartbeat,
		})
	}
	if err != nil {
		slog.Error("设备联调台停止", "error", err)
		os.Exit(1)
	}
}

func serve(parent context.Context, address, agentBinary string, config camsim.Config) error {
	camera, err := camsim.New(config)
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("控制页地址须使用回环 IP，例如 127.0.0.1:8093")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("监听控制页失败：%w", err)
	}
	defer listener.Close()
	scale, err := d12sim.New()
	if err != nil {
		return err
	}
	defer scale.Close()

	dir, err := os.MkdirTemp("", "weighbridge-device-lab-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	agent, err := startAgent(ctx, agentBinary, dir, camera.State().ServerURL, scale.State().Port)
	if err != nil {
		return err
	}
	var simulators sync.WaitGroup
	simulators.Go(func() { scale.Run(ctx) })
	simulators.Go(func() { camera.Run(ctx) })
	defer func() {
		cancel()
		<-agent.done
		simulators.Wait()
	}()

	server := &http.Server{
		Handler: controlPage(scale, camera, agent), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second,
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	fmt.Printf("设备联调台 http://%s\n", listener.Addr())
	select {
	case err := <-serverDone:
		return err
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		err := server.Shutdown(shutdown)
		if err != nil {
			_ = server.Close()
		}
		<-serverDone
		return err
	}
}

func controlPage(scale *d12sim.Simulator, camera *camsim.Simulator, agent *agentProcess) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("GET /api/agent", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(agent.status())
	})
	scaleControl := http.StripPrefix("/d12", scale.Handler())
	mux.Handle("GET /d12/api/state", scaleControl)
	mux.Handle("POST /d12/api/control", scaleControl)
	cameraControl := http.StripPrefix("/camera", camera.Handler())
	mux.Handle("GET /camera/api/state", cameraControl)
	mux.Handle("POST /camera/api/control", cameraControl)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

type agentProcess struct {
	dir     string
	done    chan struct{}
	exitErr error // Read only after done closes.
}

func startAgent(ctx context.Context, binary, dir, server, port string) (*agentProcess, error) {
	path := filepath.Join(dir, "agent.env")
	config := fmt.Sprintf("WEIGHBRIDGE_SERVER_URL=%q\nWEIGHBRIDGE_SERIAL_PORT=%q\nWEIGHBRIDGE_SERIAL_BAUD=9600\nWEIGHBRIDGE_PRINT_QUEUE=\n", server, port)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		return nil, fmt.Errorf("写入联调 Agent 配置失败：%w", err)
	}

	cmd := exec.CommandContext(ctx, binary, "--env-file", path, "--state-dir", filepath.Join(dir, "state"))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动联调 Agent 失败：%w", err)
	}
	agent := &agentProcess{dir: dir, done: make(chan struct{})}
	go func() {
		agent.exitErr = cmd.Wait()
		close(agent.done)
	}()
	return agent, nil
}

type agentStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

func (a *agentProcess) status() agentStatus {
	select {
	case <-a.done:
		message := "Agent 已退出，请检查启动终端后重启联调台"
		if a.exitErr != nil {
			message += "（" + a.exitErr.Error() + "）"
		}
		return agentStatus{"stopped", message}
	default:
	}

	var snapshot struct {
		At        time.Time `json:"at"`
		Connected bool      `json:"connected"`
		ErrorCode string    `json:"errorCode"`
	}
	data, err := os.ReadFile(filepath.Join(a.dir, "maintenance-status.json"))
	if err == nil && json.Unmarshal(data, &snapshot) == nil && time.Since(snapshot.At) < 3*time.Second {
		if snapshot.Connected {
			return agentStatus{"connected", "Agent 已连接 Server"}
		}
	}
	// The last refusal remains relevant while the Agent retries its handshake.
	if snapshot.ErrorCode == p.ErrorAgentConflict {
		return agentStatus{"conflict", "该地磅已有 Agent 在线。停止原 Agent 后，本次 Agent 会自动重试。"}
	}
	return agentStatus{"waiting", "等待 Agent 连接 Server，请核对地址和本厂身份"}
}
