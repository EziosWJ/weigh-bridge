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

	"github.com/EziosWJ/weighbridge/server/internal/d12sim"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8091", "控制页监听地址；局域网可显式设置 0.0.0.0:8091")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, *listen); err != nil {
		slog.Error("D12 模拟器停止", "error", err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, address string) error {
	sim, err := d12sim.New()
	if err != nil {
		return err
	}
	defer sim.Close()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: sim.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); sim.Run(ctx) }()
	defer func() { cancel(); <-done }()
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	fmt.Printf("D12 虚拟串口：%s\n控制页：http://%s\n现场配置：tF=05，9600 8N1，源单位 t，每 200ms 一帧\n", sim.State().Port, listener.Addr())
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
