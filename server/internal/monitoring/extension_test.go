package monitoring

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDisabledMonitoringHTTPAndSampling(t *testing.T) {
	called := false
	authorization := &testAuthorization{admin: true}
	svc := NewService(authorization, collectorFunc(func(context.Context) (Snapshot, error) { called = true; return Snapshot{}, nil }), nil, WithEnabled(false))
	svc.Run(context.Background())
	svc.Ingest(controlledSample(time.Now(), 99, 99))
	if called || !svc.Snapshot().SampledAt.IsZero() {
		t.Fatal("disabled monitoring collected or retained a sample")
	}
	router := testRouter(svc)
	for _, path := range []string{"/api/v1/monitoring/overview", "/api/v1/monitoring/history?resource=cpu"} {
		result := request(t, router, path, nil)
		if result.Code != http.StatusServiceUnavailable || !strings.Contains(result.Body.String(), `"reason":"monitoring_disabled"`) {
			t.Fatalf("disabled response: %d %s", result.Code, result.Body.String())
		}
		authorization.admin = false
		if result := request(t, router, path, nil); result.Code != http.StatusForbidden {
			t.Fatalf("disabled module bypassed authorization: %d", result.Code)
		}
		authorization.admin = true
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("missing session: %d", response.Code)
		}
	}
}

func TestInterfaceAddressesKeepHostIPAndPrefix(t *testing.T) {
	addresses, err := parseInterfaceAddresses([]net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.1.10"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(8, 32)},
		&net.IPNet{IP: net.ParseIP("fe80::1234"), Mask: net.CIDRMask(64, 128)},
		&net.IPNet{IP: net.ParseIP("192.168.1.10"), Mask: net.CIDRMask(24, 32)},
	})
	want := []IPAddress{{"IPv4", "10.0.0.5", 8}, {"IPv4", "192.168.1.10", 24}, {"IPv6", "fe80::1234", 64}}
	if err != nil || !reflect.DeepEqual(addresses, want) {
		t.Fatalf("addresses=%+v error=%v", addresses, err)
	}
	empty, err := parseInterfaceAddresses(nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("unassigned addresses=%v error=%v", empty, err)
	}
	if _, err := parseInterfaceAddresses([]net.Addr{&net.IPAddr{IP: net.ParseIP("192.168.1.10")}}); err == nil {
		t.Fatal("unreadable prefix treated as unassigned")
	}
}

func TestNetworkAddressesThroughHTTP(t *testing.T) {
	now := time.Now()
	svc := NewService(&testAuthorization{true}, nil, func() time.Time { return now })
	items := []Network{{ID: "Ethernet", Addresses: []IPAddress{{"IPv4", "192.168.1.10", 24}, {"IPv6", "fe80::1", 64}}}, {ID: "unassigned", Addresses: []IPAddress{}}, {ID: "denied", AddressError: "权限不足"}}
	sample := controlledSample(now, 40, 50)
	sample.Network = collected(items, now)
	svc.Ingest(sample)
	got := overview(t, testRouter(svc))
	if !reflect.DeepEqual(*got.Network.Data, items) {
		t.Fatalf("network=%+v", *got.Network.Data)
	}
	now = now.Add(5 * time.Second)
	items[0].Addresses = []IPAddress{{"IPv4", "10.0.0.7", 16}}
	sample.Network = collected(items, now)
	svc.Ingest(sample)
	got = overview(t, testRouter(svc))
	if (*got.Network.Data)[0].Addresses[0].IP != "10.0.0.7" {
		t.Fatal("IP change was not delivered")
	}
	now = now.Add(5 * time.Second)
	svc.Ingest(Snapshot{Network: failed[[]Network](context.DeadlineExceeded)})
	got = overview(t, testRouter(svc))
	if got.Network.Status != StatusError || (*got.Network.Data)[0].Addresses[0].IP != "10.0.0.7" {
		t.Fatal("failure did not retain last valid addresses with error status")
	}
}

func TestWindowsMetricSemantics(t *testing.T) {
	value := cpuDelta(100, 200, 60, 110)
	if value == nil || *value != 50 {
		t.Fatalf("Windows CPU=%v", value)
	}
	for _, values := range [][4]float64{{100, 90, 60, 70}, {100, 100, 60, 60}, {100, 200, 60, 50}, {100, 200, 60, 170}} {
		if cpuDelta(values[0], values[1], values[2], values[3]) != nil {
			t.Fatal("invalid/reset CPU counters produced usage")
		}
	}
	data := memoryValues(1000, 300, 0, 0)
	data.SwapUnsupported = true
	if data.UsedBytes != 700 || data.UsagePercent != 70 || data.SwapUsagePercent != nil {
		t.Fatalf("memory=%+v", data)
	}
	encoded, _ := json.Marshal(CPU{})
	if !strings.Contains(string(encoded), `"load1":null`) {
		t.Fatal("unsupported Windows load was serialized as zero")
	}
	items := topProcesses([]Process{{PID: 1, CPUPercent: nil, MemoryBytes: 900}, {PID: 2, CPUPercent: ptr(50), MemoryBytes: 100}, {PID: 3, CPUPercent: ptr(20), MemoryBytes: 200}})
	if items.CPU[0].PID != 2 || items.Memory[0].PID != 1 {
		t.Fatalf("process ranking=%+v", items)
	}
}
