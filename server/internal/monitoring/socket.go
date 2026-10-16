package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type unixCollector struct{ client *http.Client }

// NewUnixCollector only reads the explicitly configured local source. A failed
// socket request never falls back to collecting the API container's resources.
func NewUnixCollector(path string) Collector {
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	return &unixCollector{client: &http.Client{Transport: transport, Timeout: 4 * time.Second}}
}

func (c *unixCollector) Collect(ctx context.Context) (Snapshot, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/snapshot", nil)
	if err != nil {
		return Snapshot{}, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return Snapshot{}, fmt.Errorf("host collector socket: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("host collector returned HTTP %d", response.StatusCode)
	}
	var snapshot Snapshot
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("invalid host collector snapshot: %w", err)
	}
	// Preserve all measurement times and the source's identity across reads.
	return snapshot, nil
}

// ServeUnixCollector serves only the latest sample, without storing trends or
// persistent history. Filesystem permissions are the local access boundary.
func ServeUnixCollector(ctx context.Context, path string, collector Collector) error {
	if collector == nil {
		return errors.New("host collector is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	if existing, err := os.Lstat(path); err == nil {
		if existing.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("refusing to replace non-socket %s", path)
		}
		probe, probeErr := net.DialTimeout("unix", path, 200*time.Millisecond)
		if probeErr == nil {
			probe.Close()
			return fmt.Errorf("collector socket already active: %s", path)
		}
		if !errors.Is(probeErr, syscall.ECONNREFUSED) {
			return fmt.Errorf("cannot safely replace collector socket: %w", probeErr)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	defer listener.Close()
	if err := os.Chmod(path, 0660); err != nil {
		return err
	}
	created, _ := os.Lstat(path)
	defer func() {
		current, err := os.Lstat(path)
		if err == nil && created != nil && os.SameFile(created, current) {
			_ = os.Remove(path)
		}
	}()
	initial := NewService(nil, nil, nil).Snapshot()
	var mu sync.RWMutex
	latest := initial
	samplerContext, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		ticker := time.NewTicker(SampleInterval)
		defer ticker.Stop()
		for {
			if samplerContext.Err() != nil {
				return
			}
			sampleContext, cancel := context.WithTimeout(samplerContext, 4*time.Second)
			sample, err := collector.Collect(sampleContext)
			cancel()
			mu.Lock()
			if err != nil {
				sample = Snapshot{}
				sample.Host = failedSocketRegion[Host](err)
				sample.CPU = failedSocketRegion[CPU](err)
				sample.Memory = failedSocketRegion[Memory](err)
				sample.Filesystems = failedSocketRegion[[]Filesystem](err)
				sample.Disks = failedSocketRegion[[]Disk](err)
				sample.Network = failedSocketRegion[[]Network](err)
				sample.GPU = failedSocketRegion[[]GPU](err)
				sample.Processes = failedSocketRegion[Processes](err)
			}
			now := time.Now()
			sample.Host, _ = mergeRegion(latest.Host, sample.Host, now)
			sample.CPU, _ = mergeRegion(latest.CPU, sample.CPU, now)
			sample.Memory, _ = mergeRegion(latest.Memory, sample.Memory, now)
			sample.Filesystems, _ = mergeRegion(latest.Filesystems, sample.Filesystems, now)
			sample.Disks, _ = mergeRegion(latest.Disks, sample.Disks, now)
			sample.Network, _ = mergeRegion(latest.Network, sample.Network, now)
			sample.GPU, _ = mergeRegion(latest.GPU, sample.GPU, now)
			sample.Processes, _ = mergeRegion(latest.Processes, sample.Processes, now)
			if sample.SourceID == "" {
				sample.SourceID = latest.SourceID
			}
			if sample.SampledAt.IsZero() {
				sample.SampledAt = latest.SampledAt
			}
			latest = sample
			mu.Unlock()
			select {
			case <-samplerContext.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /snapshot", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		snapshot := latest
		mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 4 * time.Second, WriteTimeout: 4 * time.Second, IdleTimeout: 5 * time.Second}
	closed := make(chan struct{})
	defer close(closed)
	go func() {
		select {
		case <-ctx.Done():
			shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownContext)
		case <-closed:
		}
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func failedSocketRegion[T any](err error) Region[T] {
	return Region[T]{Status: StatusError, Message: err.Error()}
}
