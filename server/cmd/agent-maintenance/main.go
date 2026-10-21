package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:18901", "maintenance listen address; restrict access to the trusted maintenance LAN")
	data := flag.String("data-dir", "./maintenance", "private maintenance data directory")
	cert := flag.String("tls-cert", "", "optional TLS certificate; provide together with --tls-key")
	key := flag.String("tls-key", "", "TLS private key")
	resetPassword := flag.Bool("reset-password", false, "rotate local maintenance password and exit")
	flag.Parse()
	if err := agentmaintenance.ValidateListener(*listen, *cert, *key); err != nil {
		slog.Error("invalid maintenance listener", "error", err)
		os.Exit(2)
	}
	svc, err := agentmaintenance.Open(*data, agentmaintenance.ExecRunner{Timeout: 8 * time.Second})
	if err != nil {
		slog.Error("maintenance init failed", "error", err)
		os.Exit(1)
	}
	if *resetPassword {
		if err = svc.ResetPassword(); err != nil {
			slog.Error("password reset failed", "error", err)
			os.Exit(1)
		}
		slog.Info("new one-time password written", "file", *data+"/bootstrap-password.txt", "mode", "0600")
		return
	}
	if _, err = os.Stat(*data + "/bootstrap-password.txt"); err == nil {
		slog.Warn("first-use maintenance password created", "file", *data+"/bootstrap-password.txt", "mode", "0600")
	}
	srv := &http.Server{Addr: *listen, Handler: svc.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	slog.Info("local printer maintenance ready", "listen", *listen, "data", *data)
	if *cert != "" {
		err = srv.ListenAndServeTLS(*cert, *key)
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		slog.Error("maintenance server stopped", "error", err)
		os.Exit(1)
	}
}
