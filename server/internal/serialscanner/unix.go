package serialscanner

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// UnixServer serves the internal Agent API on a local Unix socket. The socket
// file is owner-only; that file permission is the service credential.
type UnixServer struct {
	server   *http.Server
	listener net.Listener
	path     string
}

// ListenUnix creates the socket and starts serving in the background.
func ListenUnix(path string, handler http.Handler) (*UnixServer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second}
	socket := &UnixServer{server: server, listener: listener, path: path}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serial scanner socket stopped", "path", path, "error", err)
		}
	}()
	return socket, nil
}

func (u *UnixServer) Close() error {
	if u == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := u.server.Shutdown(ctx)
	if closeErr := u.listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) && err == nil {
		err = closeErr
	}
	_ = os.Remove(u.path)
	return err
}
