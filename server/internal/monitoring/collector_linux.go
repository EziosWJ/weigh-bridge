//go:build linux

package monitoring

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/blockdevice"
	"golang.org/x/sys/unix"
)

// LinuxCollector runs only in the host namespaces. Each region has an independent
// lock, so a slow filesystem or driver cannot overlap itself or block other areas.
type LinuxCollector struct {
	fs              procfs.FS
	blocks          blockdevice.FS
	sourceID        string
	locks           [8]sync.Mutex
	cpuBaseline     *procfs.Stat
	cpuAt           time.Time
	diskBaseline    map[string]timedCounter[blockdevice.Diskstats]
	netBaseline     map[string]timedCounter[procfs.NetDevLine]
	netIdentity     map[string]string
	processBaseline map[int]processCounter
	processTotal    float64
	processAt       time.Time
	mountLocks      map[string]*sync.Mutex
	statfsSlots     chan struct{}
	statfs          func(string, *unix.Statfs_t) error
}

func NewLinuxCollector() (Collector, error) {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return nil, fmt.Errorf("local host monitoring cannot run in a container; configure socket source")
		}
	}
	cgroup, _ := os.ReadFile("/proc/1/cgroup")
	for _, marker := range []string{"docker", "containerd", "kubepods", "lxc"} {
		if strings.Contains(string(cgroup), marker) {
			return nil, fmt.Errorf("container detected; configure socket source for host monitoring")
		}
	}
	fs, err := procfs.NewFS("/proc")
	if err != nil {
		return nil, err
	}
	blocks, err := blockdevice.NewFS("/proc", "/sys")
	if err != nil {
		return nil, err
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return nil, err
	}
	return &LinuxCollector{fs: fs, blocks: blocks, sourceID: fmt.Sprintf("%x", id), diskBaseline: map[string]timedCounter[blockdevice.Diskstats]{}, netBaseline: map[string]timedCounter[procfs.NetDevLine]{}, netIdentity: map[string]string{}, processBaseline: map[int]processCounter{}}, nil
}

func NewNativeCollector() (Collector, error) { return NewLinuxCollector() }

func (c *LinuxCollector) Collect(ctx context.Context) (Snapshot, error) {
	budget := 3500 * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		budget = min(budget, remaining-min(100*time.Millisecond, remaining/10))
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	s := Snapshot{SourceID: c.sourceID, SampledAt: time.Now()}
	var wg sync.WaitGroup
	collectRegion(ctx, &c.locks[0], c.host, &s.Host, &wg)
	collectRegion(ctx, &c.locks[1], c.cpu, &s.CPU, &wg)
	collectRegion(ctx, &c.locks[2], c.memory, &s.Memory, &wg)
	collectRegion(ctx, &c.locks[3], c.filesystems, &s.Filesystems, &wg)
	collectRegion(ctx, &c.locks[4], c.disks, &s.Disks, &wg)
	collectRegion(ctx, &c.locks[5], c.network, &s.Network, &wg)
	collectRegion(ctx, &c.locks[6], c.gpu, &s.GPU, &wg)
	collectRegion(ctx, &c.locks[7], c.processes, &s.Processes, &wg)
	wg.Wait()
	return s, nil
}

func (c *LinuxCollector) host(ctx context.Context) Region[Host] {
	name, err := os.Hostname()
	if err != nil {
		return failed[Host](err)
	}
	var uts unix.Utsname
	if err = unix.Uname(&uts); err != nil {
		return failed[Host](err)
	}
	data := Host{Hostname: name, Kernel: unix.ByteSliceToString(uts.Release[:]), OS: "Linux"}
	if release, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(release), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				data.OS = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
				break
			}
		}
	}
	up, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return failed[Host](err)
	}
	fields := strings.Fields(string(up))
	if len(fields) == 0 {
		return failed[Host](errors.New("missing uptime"))
	}
	data.UptimeSeconds, err = strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return failed[Host](err)
	}
	if ctx.Err() != nil {
		return failed[Host](ctx.Err())
	}
	return collected(data, time.Now())
}

func cpuTotal(s procfs.CPUStat) float64 {
	return s.User + s.Nice + s.System + s.Idle + s.Iowait + s.IRQ + s.SoftIRQ + s.Steal
}
func cpuPercent(old, current procfs.CPUStat) *float64 {
	delta := cpuTotal(current) - cpuTotal(old)
	componentsOld := []float64{old.User, old.Nice, old.System, old.Idle, old.Iowait, old.IRQ, old.SoftIRQ, old.Steal}
	componentsNow := []float64{current.User, current.Nice, current.System, current.Idle, current.Iowait, current.IRQ, current.SoftIRQ, current.Steal}
	if delta <= 0 {
		return nil
	}
	for i := range componentsOld {
		if componentsNow[i] < componentsOld[i] {
			return nil
		}
	}
	value := 100 * (delta - (current.Idle - old.Idle) - (current.Iowait - old.Iowait)) / delta
	value = max(0, min(100, value))
	return &value
}

func (c *LinuxCollector) cpu(ctx context.Context) Region[CPU] {
	stat, err := c.fs.Stat()
	if err != nil {
		c.cpuBaseline = nil
		return failed[CPU](err)
	}
	info, err := c.fs.CPUInfo()
	if err != nil {
		c.cpuBaseline = nil
		return failed[CPU](err)
	}
	load, err := c.fs.LoadAvg()
	if err != nil {
		c.cpuBaseline = nil
		return failed[CPU](err)
	}
	data := CPU{LogicalCores: len(stat.CPU), Load1: &load.Load1, Load5: &load.Load5, Load15: &load.Load15}
	physical := map[string]bool{}
	for _, i := range info {
		if data.Model == "" {
			data.Model = i.ModelName
		}
		if i.PhysicalID != "" && i.CoreID != "" {
			physical[i.PhysicalID+":"+i.CoreID] = true
		}
	}
	data.PhysicalCores = len(physical)
	ids := make([]int, 0, len(stat.CPU))
	for id := range stat.CPU {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	now := time.Now()
	if c.cpuBaseline != nil && now.Sub(c.cpuAt) <= StaleAfter {
		data.UsagePercent = cpuPercent(c.cpuBaseline.CPUTotal, stat.CPUTotal)
		if len(c.cpuBaseline.CPU) != len(stat.CPU) {
			data.UsagePercent = nil
		}
	}
	for _, id := range ids {
		var value *float64
		if c.cpuBaseline != nil && now.Sub(c.cpuAt) <= StaleAfter {
			if old, exists := c.cpuBaseline.CPU[int64(id)]; exists {
				value = cpuPercent(old, stat.CPU[int64(id)])
			}
		}
		data.PerCore = append(data.PerCore, value)
	}
	if ctx.Err() != nil {
		c.cpuBaseline = nil
		return failed[CPU](ctx.Err())
	}
	c.cpuBaseline = &stat
	c.cpuAt = now
	region := collected(data, now)
	if data.UsagePercent == nil {
		region.Status = StatusCollecting
		region.Message = "建立 CPU 计数基线"
	}
	return region
}

func (c *LinuxCollector) memory(ctx context.Context) Region[Memory] {
	mem, err := c.fs.Meminfo()
	if err != nil {
		return failed[Memory](err)
	}
	// procfs exposes both original KiB values and byte-converted values.
	if mem.MemTotalBytes == nil || mem.MemAvailableBytes == nil || mem.SwapTotalBytes == nil || mem.SwapFreeBytes == nil {
		now := time.Now()
		return Region[Memory]{Status: StatusUnsupported, CollectedAt: &now, Message: "内核未提供 MemAvailable/Swap 字段"}
	}
	if ctx.Err() != nil {
		return failed[Memory](ctx.Err())
	}
	region := collected(memoryValues(*mem.MemTotalBytes, *mem.MemAvailableBytes, *mem.SwapTotalBytes, *mem.SwapFreeBytes), time.Now())
	if *mem.SwapTotalBytes == 0 {
		region.Message = "未配置 Swap"
	}
	return region
}

func filesystemValues(stat unix.Statfs_t) (total, used, available uint64, percent *float64) {
	unit := uint64(stat.Bsize)
	total = stat.Blocks * unit
	used = (stat.Blocks - min(stat.Blocks, stat.Bfree)) * unit
	available = stat.Bavail * unit
	if used+available > 0 {
		value := 100 * float64(used) / float64(used+available)
		percent = &value
	}
	return
}

func (c *LinuxCollector) filesystems(ctx context.Context) Region[[]Filesystem] {
	contents, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return failed[[]Filesystem](err)
	}
	mounts, err := parseHostMounts(string(contents))
	if err != nil {
		return failed[[]Filesystem](err)
	}
	return c.collectFilesystems(ctx, mounts)
}

func (c *LinuxCollector) collectFilesystems(ctx context.Context, mounts []*procfs.MountInfo) Region[[]Filesystem] {
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline.Add(-50*time.Millisecond))
		defer cancel()
	}
	if c.mountLocks == nil {
		c.mountLocks = map[string]*sync.Mutex{}
		c.statfsSlots = make(chan struct{}, 8)
	}
	type mountResult struct {
		mount *procfs.MountInfo
		stat  unix.Statfs_t
		err   error
	}
	results := make(chan mountResult, len(mounts))
	pending := map[string]bool{}
	data := []Filesystem{}
	failedMounts := []string{}
	for _, mount := range mounts {
		switch mount.FSType {
		case "proc", "sysfs", "devpts", "cgroup", "cgroup2", "securityfs", "debugfs", "tracefs", "pstore", "configfs", "mqueue", "hugetlbfs", "fusectl", "autofs", "binfmt_misc", "nsfs":
			continue
		}
		if pending[mount.MountPoint] {
			continue
		}
		pending[mount.MountPoint] = true
		lock := c.mountLocks[mount.MountPoint]
		if lock == nil {
			lock = &sync.Mutex{}
			c.mountLocks[mount.MountPoint] = lock
		}
		go func(mount *procfs.MountInfo, lock *sync.Mutex) {
			result := mountResult{mount: mount}
			if !lock.TryLock() {
				result.err = errors.New("previous Statfs still running")
				results <- result
				return
			}
			defer lock.Unlock()
			select {
			case c.statfsSlots <- struct{}{}:
				defer func() { <-c.statfsSlots }()
			case <-ctx.Done():
				result.err = ctx.Err()
				results <- result
				return
			}
			if ctx.Err() != nil {
				result.err = ctx.Err()
				results <- result
				return
			}
			statfs := c.statfs
			if statfs == nil {
				statfs = unix.Statfs
			}
			result.err = statfs(mount.MountPoint, &result.stat)
			results <- result
		}(mount, lock)
	}
	for len(pending) > 0 {
		select {
		case result := <-results:
			mount := result.mount
			delete(pending, mount.MountPoint)
			if result.err != nil {
				failedMounts = append(failedMounts, mount.MountPoint)
				continue
			}
			total, used, available, percent := filesystemValues(result.stat)
			if total > 0 {
				data = append(data, Filesystem{ID: mount.MajorMinorVer + ":" + mount.MountPoint, Device: mount.Source, Mountpoint: mount.MountPoint, Type: mount.FSType, TotalBytes: total, UsedBytes: used, AvailableBytes: available, UsagePercent: percent})
			}
		case <-ctx.Done():
			for path := range pending {
				failedMounts = append(failedMounts, path)
			}
			clear(pending)
		}
	}
	sort.Slice(data, func(i, j int) bool { return data[i].Mountpoint < data[j].Mountpoint })
	sort.Strings(failedMounts)
	active := map[string]bool{}
	for _, mount := range mounts {
		active[mount.MountPoint] = true
	}
	for path, lock := range c.mountLocks {
		if !active[path] && lock.TryLock() {
			lock.Unlock()
			delete(c.mountLocks, path)
		}
	}
	region := collected(data, time.Now())
	if len(data) == 0 {
		region.Status = StatusNoDevice
	}
	if len(failedMounts) > 0 {
		region.Partial = true
		region.Message = "无法读取挂载点: " + strings.Join(failedMounts, ", ")
		if len(data) == 0 {
			region.Status = StatusError
			region.CollectedAt = nil
		}
	}
	return region
}

// Only parse the fixed fields that monitoring needs. WSL's 9p options can
// contain spaces, so locating the separator by its offset from the end fails.
func parseHostMounts(contents string) ([]*procfs.MountInfo, error) {
	data := []*procfs.MountInfo{}
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for _, line := range strings.Split(contents, "\n") {
		if line == "" {
			continue
		}
		left, right, found := strings.Cut(line, " - ")
		fields := strings.Fields(left)
		details := strings.Fields(right)
		if !found || len(fields) < 6 || len(details) < 2 {
			return nil, errors.New("invalid host mountinfo record")
		}
		data = append(data, &procfs.MountInfo{MajorMinorVer: fields[2], MountPoint: unescape.Replace(fields[4]), FSType: details[0], Source: unescape.Replace(details[1])})
	}
	return data, nil
}

func (c *LinuxCollector) disks(ctx context.Context) Region[[]Disk] {
	stats, err := c.blocks.ProcDiskstats()
	if err != nil {
		c.diskBaseline = map[string]timedCounter[blockdevice.Diskstats]{}
		return failed[[]Disk](err)
	}
	now := time.Now()
	next := map[string]timedCounter[blockdevice.Diskstats]{}
	data := []Disk{}
	for _, stat := range stats {
		if strings.HasPrefix(stat.DeviceName, "loop") || strings.HasPrefix(stat.DeviceName, "ram") {
			continue
		}
		id := stat.DeviceName
		item := Disk{ID: id, Message: "建立块设备计数基线"}
		if old, exists := c.diskBaseline[id]; exists && now.Sub(old.at) <= StaleAfter && old.value.MajorNumber == stat.MajorNumber && old.value.MinorNumber == stat.MinorNumber {
			elapsed := now.Sub(old.at)
			item.ReadBytesPerSecond = counterRate(old.value.ReadSectors*512, stat.ReadSectors*512, elapsed)
			item.WriteBytesPerSecond = counterRate(old.value.WriteSectors*512, stat.WriteSectors*512, elapsed)
			item.ReadIOPS = counterRate(old.value.ReadIOs, stat.ReadIOs, elapsed)
			item.WriteIOPS = counterRate(old.value.WriteIOs, stat.WriteIOs, elapsed)
			if item.ReadBytesPerSecond == nil || item.WriteBytesPerSecond == nil || item.ReadIOPS == nil || item.WriteIOPS == nil {
				item.ReadBytesPerSecond = nil
				item.WriteBytesPerSecond = nil
				item.ReadIOPS = nil
				item.WriteIOPS = nil
			} else {
				item.Message = ""
			}
		}
		data = append(data, item)
		next[id] = timedCounter[blockdevice.Diskstats]{now, stat}
	}
	if ctx.Err() != nil {
		c.diskBaseline = map[string]timedCounter[blockdevice.Diskstats]{}
		return failed[[]Disk](ctx.Err())
	}
	c.diskBaseline = next
	region := collected(data, now)
	if len(data) == 0 {
		region.Status = StatusNoDevice
	}
	return region
}

func readSys(path string) string {
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(string(data))
}
func (c *LinuxCollector) network(ctx context.Context) Region[[]Network] {
	stats, err := c.fs.NetDev()
	if err != nil {
		c.netBaseline = map[string]timedCounter[procfs.NetDevLine]{}
		return failed[[]Network](err)
	}
	now := time.Now()
	next := map[string]timedCounter[procfs.NetDevLine]{}
	identities := map[string]string{}
	data := []Network{}
	for id, stat := range stats {
		root := filepath.Join("/sys/class/net", id)
		identity := readSys(filepath.Join(root, "ifindex")) + ":" + readSys(filepath.Join(root, "address"))
		identities[id] = identity
		flags, _ := strconv.ParseUint(readSys(filepath.Join(root, "flags")), 0, 32)
		item := Network{ID: id, Loopback: id == "lo" || flags&unix.IFF_LOOPBACK != 0, Virtual: strings.HasPrefix(id, "veth"), State: readSys(filepath.Join(root, "operstate")), ReceiveBytes: stat.RxBytes, TransmitBytes: stat.TxBytes, ReceiveErrors: stat.RxErrors, TransmitErrors: stat.TxErrors, ReceiveDropped: stat.RxDropped, TransmitDropped: stat.TxDropped, Message: "建立网卡计数基线"}
		iface, addressErr := net.InterfaceByName(id)
		if addressErr == nil {
			item.Addresses, addressErr = interfaceAddresses(*iface)
		}
		if addressErr != nil {
			item.AddressError = "IP 地址读取失败：" + addressErr.Error()
		}
		if speed, err := strconv.ParseFloat(readSys(filepath.Join(root, "speed")), 64); err == nil && speed > 0 {
			speed *= 1e6
			item.SpeedBitsPerSecond = &speed
		}
		if old, exists := c.netBaseline[id]; exists && identities[id] == c.netIdentity[id] && now.Sub(old.at) <= StaleAfter {
			elapsed := now.Sub(old.at)
			item.ReceiveBytesPerSecond = counterRate(old.value.RxBytes, stat.RxBytes, elapsed)
			item.TransmitBytesPerSecond = counterRate(old.value.TxBytes, stat.TxBytes, elapsed)
			if item.ReceiveBytesPerSecond == nil || item.TransmitBytesPerSecond == nil {
				item.ReceiveBytesPerSecond = nil
				item.TransmitBytesPerSecond = nil
			} else {
				item.Message = ""
				if item.SpeedBitsPerSecond != nil {
					rx := *item.ReceiveBytesPerSecond * 8 / *item.SpeedBitsPerSecond * 100
					tx := *item.TransmitBytesPerSecond * 8 / *item.SpeedBitsPerSecond * 100
					item.ReceiveUsagePercent = &rx
					item.TransmitUsagePercent = &tx
				}
			}
		}
		if item.SpeedBitsPerSecond == nil {
			if item.Message != "" {
				item.Message += "; "
			}
			item.Message += "链路速率不支持或不可读取"
		}
		data = append(data, item)
		next[id] = timedCounter[procfs.NetDevLine]{now, stat}
	}
	sort.Slice(data, func(i, j int) bool { return data[i].ID < data[j].ID })
	if ctx.Err() != nil {
		c.netBaseline = map[string]timedCounter[procfs.NetDevLine]{}
		return failed[[]Network](ctx.Err())
	}
	c.netBaseline = next
	c.netIdentity = identities
	region := collected(data, now)
	for _, item := range data {
		if item.AddressError != "" {
			region.Partial = true
		}
	}
	if len(data) == 0 {
		region.Status = StatusNoDevice
	}
	return region
}

func (c *LinuxCollector) processes(ctx context.Context) Region[Processes] {
	stat, err := c.fs.Stat()
	if err != nil {
		c.processBaseline = map[int]processCounter{}
		return failed[Processes](err)
	}
	procs, err := c.fs.AllProcs()
	if err != nil {
		c.processBaseline = map[int]processCounter{}
		return failed[Processes](err)
	}
	now := time.Now()
	total := cpuTotal(stat.CPUTotal)
	next := map[int]processCounter{}
	items := []Process{}
	failures := 0
	for _, proc := range procs {
		if ctx.Err() != nil {
			c.processBaseline = map[int]processCounter{}
			return failed[Processes](ctx.Err())
		}
		ps, err := proc.Stat()
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, unix.ESRCH) {
				failures++
			}
			continue
		}
		item := Process{PID: ps.PID, Name: ps.Comm, MemoryBytes: uint64(max(0, ps.ResidentMemory()))}
		if old, exists := c.processBaseline[ps.PID]; exists && old.started == ps.Starttime && now.Sub(c.processAt) <= StaleAfter && total > c.processTotal && ps.CPUTime() >= old.seconds {
			value := 100 * (ps.CPUTime() - old.seconds) / (total - c.processTotal)
			value = max(0, min(100, value))
			item.CPUPercent = &value
		}
		items = append(items, item)
		next[ps.PID] = processCounter{ps.Starttime, ps.CPUTime()}
	}
	if ctx.Err() != nil {
		c.processBaseline = map[int]processCounter{}
		return failed[Processes](ctx.Err())
	}
	c.processBaseline = next
	c.processTotal = total
	c.processAt = now
	data := topProcesses(items)
	region := collected(data, now)
	if failures > 0 {
		region.Partial = true
		region.Message = fmt.Sprintf("覆盖不完整：%d 个进程读取失败", failures)
	}
	return region
}

func (c *LinuxCollector) gpu(ctx context.Context) Region[[]GPU] {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		entries, scanErr := os.ReadDir("/sys/bus/pci/devices")
		if scanErr != nil {
			return failed[[]GPU](scanErr)
		}
		hasDisplay, nvidia := false, false
		for _, entry := range entries {
			root := filepath.Join("/sys/bus/pci/devices", entry.Name())
			if strings.HasPrefix(readSys(filepath.Join(root, "class")), "0x03") {
				hasDisplay = true
				if readSys(filepath.Join(root, "vendor")) == "0x10de" {
					nvidia = true
				}
			}
		}
		if nvidia {
			now := time.Now()
			return Region[[]GPU]{Status: StatusUnsupported, CollectedAt: &now, Message: "NVIDIA 驱动工具 nvidia-smi 不可用"}
		}
		if hasDisplay {
			now := time.Now()
			return Region[[]GPU]{Status: StatusUnsupported, CollectedAt: &now, Message: "仅正式支持 NVIDIA GPU"}
		}
		empty := []GPU{}
		region := collected(empty, time.Now())
		region.Status = StatusNoDevice
		return region
	}
	output, err := exec.CommandContext(ctx, path, "--query-gpu=uuid,name,utilization.gpu,memory.total,memory.used,temperature.gpu,power.draw,driver_version", "--format=csv,noheader,nounits").Output()
	if err != nil {
		if ctx.Err() != nil {
			return failed[[]GPU](ctx.Err())
		}
		return failed[[]GPU](fmt.Errorf("nvidia-smi: %w", err))
	}
	data, partial, err := parseGPUCSV(string(output))
	if err != nil {
		return failed[[]GPU](err)
	}
	region := collected(data, time.Now())
	region.Partial = partial
	if partial {
		region.Message = "部分 GPU 或字段采集失败/不支持"
	}
	if len(data) == 0 {
		if partial {
			region.Status = StatusError
			region.CollectedAt = nil
		} else {
			region.Status = StatusNoDevice
		}
	}
	return region
}
