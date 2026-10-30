package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/camsim"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8092", "控制页监听地址；局域网可显式设置 0.0.0.0:8092")
	serverURL := flag.String("server", "http://127.0.0.1:8900", "本厂 Server 根地址，例如 http://192.168.1.10:8900")
	serial := flag.String("serial", "camera-sim-01", "摄像头管理页绑定的设备序列号")
	heartbeat := flag.Duration("heartbeat", camsim.DefaultHeartbeatInterval, "心跳间隔，需大于零")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, *listen, camsim.Config{ServerURL: *serverURL, SerialNumber: *serial, HeartbeatInterval: *heartbeat}); err != nil {
		slog.Error("摄像头模拟器停止", "error", err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, address string, config camsim.Config) error {
	sim, err := camsim.New(config)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: sim.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); sim.Run(runCtx) }()
	defer func() { cancel(); <-done }()
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	state := sim.State()
	fmt.Printf("摄像头模拟器：序列号 %s -> %s\n控制页：http://%s\n上报地址：%s\n", state.SerialNumber, state.ServerURL, listener.Addr(), state.EventURL)
	select {
	case err := <-serverDone:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
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
