//go:build linux

package monitoring

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/blockdevice"
	"golang.org/x/sys/unix"
)

func assertMeasured(t *testing.T, value *float64, want float64) {
	t.Helper()
	if value == nil || math.Abs(*value-want) > 0.000001 {
		t.Fatalf("measured %v, want %g", value, want)
	}
}

func TestLinuxMetricSemantics(t *testing.T) {
	// Guest CPU is included in user time already, not added a second time.
	old := procfs.CPUStat{User: 10, Idle: 30, Guest: 5}
	current := procfs.CPUStat{User: 20, Idle: 60, Guest: 10}
	assertMeasured(t, cpuPercent(old, current), 25)
	if cpuPercent(current, old) != nil || cpuPercent(old, old) != nil {
		t.Fatal("reset or first CPU counters must have no measured rate")
	}
	mem := memoryValues(1000, 250, 200, 150)
	if mem.UsedBytes != 750 || mem.UsagePercent != 75 || mem.SwapUsedBytes != 50 {
		t.Fatalf("memory semantics: %+v", mem)
	}
	assertMeasured(t, mem.SwapUsagePercent, 25)
	if memoryValues(1000, 250, 0, 0).SwapUsagePercent != nil {
		t.Fatal("no swap must not appear as measured 0%")
	}
	total, used, available, percent := filesystemValues(unix.Statfs_t{Bsize: 4096, Blocks: 100, Bfree: 40, Bavail: 20})
	if total != 409600 || used != 245760 || available != 81920 {
		t.Fatal("filesystem block conversion")
	}
	assertMeasured(t, percent, 75)
	assertMeasured(t, counterRate(100, 500, 2*time.Second), 200)
	assertMeasured(t, counterRate(100, 500, 8*time.Second), 50)
	if counterRate(500, 100, time.Second) != nil || counterRate(100, 500, 0) != nil {
		t.Fatal("counter reset must not produce a rate")
	}
}

func TestGPUCSVPreservesUnsupportedFieldsAndCards(t *testing.T) {
	data, partial, err := parseGPUCSV("GPU-a, Card A, 30, 8192, 2048, N/A, 100, 555.1\nGPU-b, Card B, 0, 4096, 0, 35, N/A, 555.1\nmalformed\n")
	if err != nil || !partial || len(data) != 2 {
		t.Fatalf("GPU parse: %+v %v %v", data, partial, err)
	}
	if data[0].TemperatureCelsius != nil || data[1].PowerWatts != nil || data[0].ID != "GPU-a" || *data[0].MemoryTotalBytes != 8192*1048576 {
		t.Fatalf("GPU support semantics: %+v", data)
	}
	assertMeasured(t, data[1].UsagePercent, 0)
}

func TestHostMountParserHandlesWSLSpacesAndKernelEscapes(t *testing.T) {
	mounts, err := parseHostMounts(`1102 1048 0:138 / /Docker/host rw,noatime - 9p C:\134Program\040Files\134Docker rw,aname=drvfs;path=C:\Program Files\Docker;symlinkroot=/mnt/` + "\n")
	if err != nil || len(mounts) != 1 || mounts[0].MountPoint != "/Docker/host" || mounts[0].Source != `C:\Program Files\Docker` {
		t.Fatalf("mount parse: %+v %v", mounts, err)
	}
}

func TestFilesystemsKeepReadableMountsWhenOneTaskBlocks(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{})
	finished := make(chan struct{})
	defer func() { close(gate); <-finished }()
	c := &LinuxCollector{statfs: func(path string, stat *unix.Statfs_t) error {
		if path == "/slow" {
			select {
			case <-started:
				t.Error("overlapping slow mount collection")
			default:
				close(started)
			}
			<-gate
			close(finished)
		}
		*stat = unix.Statfs_t{Bsize: 4096, Blocks: 100, Bfree: 40, Bavail: 20}
		return nil
	}}
	mounts := []*procfs.MountInfo{{MountPoint: "/fast", FSType: "ext4"}, {MountPoint: "/slow", FSType: "ext4"}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	first := c.collectFilesystems(ctx, mounts)
	if first.Data == nil || len(*first.Data) != 1 || (*first.Data)[0].Mountpoint != "/fast" || !first.Partial || first.Status != StatusOK {
		t.Fatalf("partial filesystem collection: %+v", first)
	}
	secondCtx, secondCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer secondCancel()
	second := c.collectFilesystems(secondCtx, mounts)
	if second.Data == nil || len(*second.Data) != 1 || !second.Partial {
		t.Fatalf("slow task must not overlap next sample: %+v", second)
	}
}

func TestRegionTimeoutDoesNotOverlapOrBlockOtherRegions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	gate := make(chan struct{})
	started := make(chan struct{})
	finished := make(chan struct{})
	var slowLock, fastLock sync.Mutex
	var slow, fast Region[int]
	var wg sync.WaitGroup
	collectRegion(ctx, &slowLock, func(context.Context) Region[int] {
		close(started)
		<-gate
		close(finished)
		return collected(1, time.Now())
	}, &slow, &wg)
	collectRegion(context.Background(), &fastLock, func(context.Context) Region[int] { return collected(2, time.Now()) }, &fast, &wg)
	<-started
	cancel()
	wg.Wait()
	if slow.Status != StatusError || fast.Data == nil || *fast.Data != 2 {
		t.Fatalf("partial timeout: %+v %+v", slow, fast)
	}
	var repeated Region[int]
	var next sync.WaitGroup
	collectRegion(context.Background(), &slowLock, func(context.Context) Region[int] { t.Error("overlapping collection"); return Region[int]{} }, &repeated, &next)
	next.Wait()
	if repeated.Status != StatusError {
		t.Fatal("in-flight task should report error")
	}
	close(gate)
	<-finished
}

func TestDiskFirstSampleResetAndInterruption(t *testing.T) {
	root := t.TempDir()
	sys := t.TempDir()
	diskfile := filepath.Join(root, "diskstats")
	write := func(read uint64) {
		t.Helper()
		text := fmtDiskstats(read)
		if err := os.WriteFile(diskfile, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(100)
	fs, err := blockdevice.NewFS(root, sys)
	if err != nil {
		t.Fatal(err)
	}
	c := &LinuxCollector{blocks: fs, diskBaseline: map[string]timedCounter[blockdevice.Diskstats]{}}
	first := c.disks(context.Background())
	if first.Data == nil || len(*first.Data) != 1 || (*first.Data)[0].ReadBytesPerSecond != nil {
		t.Fatalf("first disk sample: %+v", first)
	}
	old := c.diskBaseline["sda"]
	old.at = time.Now().Add(-2 * time.Second)
	c.diskBaseline["sda"] = old
	write(104)
	second := c.disks(context.Background())
	if (*second.Data)[0].ReadBytesPerSecond == nil || math.Abs(*(*second.Data)[0].ReadBytesPerSecond-1024) > 1 {
		t.Fatal("sector rate must use 512 byte units and actual interval")
	}
	write(2)
	reset := c.disks(context.Background())
	if (*reset.Data)[0].ReadBytesPerSecond != nil {
		t.Fatal("reset must establish new baseline")
	}
	old = c.diskBaseline["sda"]
	old.at = time.Now().Add(-16 * time.Second)
	c.diskBaseline["sda"] = old
	write(10)
	interrupted := c.disks(context.Background())
	if (*interrupted.Data)[0].ReadBytesPerSecond != nil {
		t.Fatal("interruption must establish new baseline")
	}
}

func fmtDiskstats(read uint64) string {
	return "8 0 sda 10 0 " + strconv.FormatUint(read, 10) + " 0 20 0 300 0 0 0 0\n"
}
