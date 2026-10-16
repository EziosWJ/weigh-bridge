//go:build windows

package monitoring

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

var windowsSmoke = flag.Bool("windows-smoke", false, "run read-only Windows native collector verification")

func TestWindowsCPUCounterNormalization(t *testing.T) {
	old := cpu.TimesStat{User: 20, System: 10, Idle: 70, Irq: 5}
	current := cpu.TimesStat{User: 40, System: 20, Idle: 140, Irq: 10}
	if value := windowsCPUPercent(old, current); value == nil || *value != 30 {
		t.Fatalf("CPU normalized incorrectly: %v", value)
	}
	current.System = 9
	if windowsCPUPercent(old, current) != nil {
		t.Fatal("component reset created false CPU usage")
	}
}

func TestWindowsNativeSmoke(t *testing.T) {
	if !*windowsSmoke {
		t.Skip("pass -windows-smoke to verify on a real Windows host")
	}
	collector, err := NewNativeCollector()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	first, err := collector.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range []struct{ name, status, message string }{
		{"host", first.Host.Status, first.Host.Message}, {"cpu", first.CPU.Status, first.CPU.Message}, {"memory", first.Memory.Status, first.Memory.Message}, {"filesystems", first.Filesystems.Status, first.Filesystems.Message}, {"network", first.Network.Status, first.Network.Message}, {"processes", first.Processes.Status, first.Processes.Message},
	} {
		if region.status != StatusOK && region.status != StatusCollecting {
			t.Errorf("%s: %s %s", region.name, region.status, region.message)
		}
	}
	if first.CPU.Data != nil && (first.CPU.Data.Load1 != nil || first.CPU.Data.LogicalCores <= 0) {
		t.Fatal("invalid Windows CPU/load representation")
	}
	if first.Memory.Data != nil && (!first.Memory.Data.SwapUnsupported || first.Memory.Data.TotalBytes == 0) {
		t.Fatal("invalid Windows memory representation")
	}
	if first.Network.Data != nil {
		for _, item := range *first.Network.Data {
			if item.AddressError != "" {
				t.Errorf("address collection: %s", item.AddressError)
			}
		}
	}
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(time.Second):
	}
	second, err := collector.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.CPU.Data == nil || second.CPU.Data.UsagePercent == nil {
		t.Fatalf("second CPU sample: %s %s", second.CPU.Status, second.CPU.Message)
	}
	if second.Disks.Status == StatusError {
		t.Errorf("disk I/O: %s", second.Disks.Message)
	}
	if second.Disks.Status == StatusOK && second.Disks.Data != nil {
		for _, item := range *second.Disks.Data {
			if item.ReadBytesPerSecond == nil || item.WriteBytesPerSecond == nil || item.ReadIOPS == nil || item.WriteIOPS == nil {
				t.Errorf("disk %s has no measured second sample", item.ID)
			}
		}
	}
	if second.Processes.Data != nil {
		measured := false
		for _, item := range second.Processes.Data.CPU {
			measured = measured || item.CPUPercent != nil
		}
		if !measured {
			t.Error("no measured process CPU after second sample")
		}
	}
	t.Logf("Windows native verification: host/CPU/memory/storage/network/processes; disk I/O=%s, GPU=%s", second.Disks.Status, second.GPU.Status)
}
