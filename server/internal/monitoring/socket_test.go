package monitoring

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type socketFixtureCollector struct{ sample Snapshot }

func (c socketFixtureCollector) Collect(context.Context) (Snapshot, error) { return c.sample, nil }

func TestUnixSocketPreservesOriginalSampleAndCleansUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sock")
	at := time.Now().Add(-time.Second).UTC()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host := Host{Hostname: "real-host-fixture"}
	interfaces := []Network{{ID: "host-eth0", Addresses: []IPAddress{{"IPv4", "10.0.0.5", 24}, {"IPv6", "fe80::5", 64}}}}
	sample := Snapshot{SourceID: "host-process-1", SampledAt: at, Host: Region[Host]{Status: StatusOK, CollectedAt: &at, Data: &host}, Network: collected(interfaces, at)}
	done := make(chan error, 1)
	go func() { done <- ServeUnixCollector(ctx, path, socketFixtureCollector{sample}) }()
	client := NewUnixCollector(path)
	deadline := time.Now().Add(2 * time.Second)
	var received Snapshot
	for time.Now().Before(deadline) {
		received, _ = client.Collect(ctx)
		if received.Host.Data != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if received.Host.Data == nil || received.Host.Data.Hostname != "real-host-fixture" || !received.Host.CollectedAt.Equal(at) || received.SourceID != sample.SourceID {
		t.Fatalf("socket roundtrip: %+v", received)
	}
	if received.Network.Data == nil || len(*received.Network.Data) != 1 || len((*received.Network.Data)[0].Addresses) != 2 || (*received.Network.Data)[0].Addresses[0].IP != "10.0.0.5" || !received.Network.CollectedAt.Equal(at) {
		t.Fatalf("host socket lost IP information: %+v", received.Network)
	}
	second, err := client.Collect(ctx)
	if err != nil || !second.Host.CollectedAt.Equal(at) || !second.SampledAt.Equal(at) {
		t.Fatal("reading an old sample must not refresh its timestamp")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0660 {
		t.Fatalf("socket permissions: %v %v", info, err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("socket shutdown did not finish")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("socket must be removed on shutdown")
	}
	if _, err := client.Collect(context.Background()); err == nil {
		t.Fatal("disconnected socket must fail without native fallback")
	}
}

func TestUnixSocketRefusesActiveAndNonSocketPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sock")
	ctx := context.Background()
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ServeUnixCollector(ctx, path, socketFixtureCollector{}); err == nil {
		t.Fatal("must preserve non-socket file")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := ServeUnixCollector(ctx, path, socketFixtureCollector{}); err == nil {
		t.Fatal("must preserve active socket")
	}
}
