//go:build windows

package monitoring

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows"
)

// WindowsCollector uses native counters. Independent region locks bound slow
// system calls without allowing a timed-out region to overlap its next sample.
type WindowsCollector struct {
	sourceID        string
	locks           [8]sync.Mutex
	cpuBaseline     []cpu.TimesStat
	cpuAt           time.Time
	cpuInfo         []cpu.InfoStat
	physicalCores   int
	diskBaseline    map[string]timedCounter[windowsDiskStat]
	netBaseline     map[string]timedCounter[psnet.IOCountersStat]
	netIdentity     map[string]string
	processBaseline map[int]processCounter
	processTotal    float64
	processAt       time.Time
}

func NewNativeCollector() (Collector, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	return &WindowsCollector{sourceID: fmt.Sprintf("%x", id), diskBaseline: map[string]timedCounter[windowsDiskStat]{}, netBaseline: map[string]timedCounter[psnet.IOCountersStat]{}, netIdentity: map[string]string{}, processBaseline: map[int]processCounter{}}, nil
}

func NewLinuxCollector() (Collector, error) {
	return nil, errors.New("独立 Unix socket 宿主机采集进程仅支持 Linux；Windows 请原生运行 API 并使用 native 数据源")
}

func (c *WindowsCollector) Collect(ctx context.Context) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
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

var windowsWMI = wmi.Client{PtrNil: true}

func (c *WindowsCollector) host(ctx context.Context) Region[Host] {
	name, err := os.Hostname()
	if err != nil {
		return failed[Host](err)
	}
	var systems []struct {
		Caption string
		Version string
	}
	if err := windowsWMI.Query("SELECT Caption, Version FROM Win32_OperatingSystem", &systems); err != nil {
		return failed[Host](err)
	}
	if len(systems) == 0 {
		return failed[Host](errors.New("未返回 Windows 系统信息"))
	}
	if ctx.Err() != nil {
		return failed[Host](ctx.Err())
	}
	return collected(Host{Hostname: name, OS: systems[0].Caption, Kernel: systems[0].Version, UptimeSeconds: windows.DurationSinceBoot().Seconds()}, time.Now())
}

func windowsCPUPercent(old, current cpu.TimesStat) *float64 {
	if current.User < old.User || current.System < old.System {
		return nil
	}
	return cpuDelta(old.User+old.System+old.Idle, current.User+current.System+current.Idle, old.Idle, current.Idle)
}

func (c *WindowsCollector) cpu(ctx context.Context) Region[CPU] {
	times, err := cpu.TimesWithContext(ctx, true)
	if err != nil || len(times) == 0 {
		c.cpuBaseline = nil
		if err == nil {
			err = errors.New("未返回 CPU 计数")
		}
		return failed[CPU](err)
	}
	if len(c.cpuInfo) == 0 {
		c.cpuInfo, _ = cpu.InfoWithContext(ctx)
	}
	if c.physicalCores == 0 {
		c.physicalCores, _ = cpu.CountsWithContext(ctx, false)
	}
	data := CPU{LogicalCores: len(times), PhysicalCores: c.physicalCores, PerCore: make([]*float64, len(times))}
	if len(c.cpuInfo) > 0 {
		data.Model = c.cpuInfo[0].ModelName
	}
	now := time.Now()
	if len(c.cpuBaseline) == len(times) && now.Sub(c.cpuAt) <= StaleAfter {
		var oldTotal, newTotal cpu.TimesStat
		for i, current := range times {
			old := c.cpuBaseline[i]
			data.PerCore[i] = windowsCPUPercent(old, current)
			oldTotal.User += old.User
			oldTotal.System += old.System
			oldTotal.Idle += old.Idle
			newTotal.User += current.User
			newTotal.System += current.System
			newTotal.Idle += current.Idle
		}
		data.UsagePercent = windowsCPUPercent(oldTotal, newTotal)
		for _, value := range data.PerCore {
			if value == nil {
				data.UsagePercent = nil
				break
			}
		}
	}
	if ctx.Err() != nil {
		c.cpuBaseline = nil
		return failed[CPU](ctx.Err())
	}
	c.cpuBaseline, c.cpuAt = times, now
	region := collected(data, now)
	region.Message = "Windows 不提供 Linux 1/5/15 分钟系统负载"
	if data.UsagePercent == nil {
		region.Status = StatusCollecting
		region.Message += "; 建立 CPU 计数基线"
	}
	if data.Model == "" || data.PhysicalCores == 0 {
		region.Partial = true
		region.Message += "; CPU 型号或物理核数不可读取"
	}
	return region
}

func (c *WindowsCollector) memory(ctx context.Context) Region[Memory] {
	info, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return failed[Memory](err)
	}
	data := memoryValues(info.Total, info.Available, 0, 0)
	data.SwapUnsupported = true
	region := collected(data, time.Now())
	region.Message = "Windows 分页文件与 Linux Swap 口径不同，Swap 指标不支持"
	return region
}

func (c *WindowsCollector) filesystems(ctx context.Context) Region[[]Filesystem] {
	return windowsFilesystems(ctx)
}

func (c *WindowsCollector) disks(ctx context.Context) Region[[]Disk] {
	stats, failures, unsupported, err := windowsDiskCounters(ctx)
	if err != nil {
		c.diskBaseline = map[string]timedCounter[windowsDiskStat]{}
		return failed[[]Disk](err)
	}
	now := time.Now()
	next := map[string]timedCounter[windowsDiskStat]{}
	data := []Disk{}
	for id, stat := range stats {
		item := Disk{ID: id, Message: "建立磁盘计数基线"}
		if old, ok := c.diskBaseline[id]; ok && now.Sub(old.at) <= StaleAfter {
			elapsed := now.Sub(old.at)
			item.ReadBytesPerSecond = counterRate(old.value.readBytes, stat.readBytes, elapsed)
			item.WriteBytesPerSecond = counterRate(old.value.writeBytes, stat.writeBytes, elapsed)
			item.ReadIOPS = counterRate(old.value.readCount, stat.readCount, elapsed)
			item.WriteIOPS = counterRate(old.value.writeCount, stat.writeCount, elapsed)
			if item.ReadBytesPerSecond != nil && item.WriteBytesPerSecond != nil && item.ReadIOPS != nil && item.WriteIOPS != nil {
				item.Message = ""
			} else {
				item.ReadBytesPerSecond = nil
				item.WriteBytesPerSecond = nil
				item.ReadIOPS = nil
				item.WriteIOPS = nil
			}
		}
		data = append(data, item)
		next[id] = timedCounter[windowsDiskStat]{now, stat}
	}
	c.diskBaseline = next
	sort.Slice(data, func(i, j int) bool { return data[i].ID < data[j].ID })
	if len(data) == 0 && unsupported > 0 && failures == 0 {
		return Region[[]Disk]{Status: StatusUnsupported, CollectedAt: &now, Message: "Windows 磁盘性能计数器不支持或未启用"}
	}
	region := partialDevices(data, failures, "磁盘 I/O")
	if unsupported > 0 {
		region.Partial = true
		region.Message += "; 部分磁盘不提供性能计数器"
	}
	return region
}

type adapterState struct {
	speed uint64
	up    bool
}

func windowsAdapters() (map[int]adapterState, error) {
	var size uint32 = 15000
	for attempt := 0; attempt < 3; attempt++ {
		buffer := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if errors.Is(err, windows.ERROR_NO_DATA) {
			return map[int]adapterState{}, nil
		}
		if err != nil {
			return nil, err
		}
		result := map[int]adapterState{}
		for adapter := first; adapter != nil; adapter = adapter.Next {
			state := adapterState{speed: min(adapter.ReceiveLinkSpeed, adapter.TransmitLinkSpeed), up: adapter.OperStatus == windows.IfOperStatusUp}
			result[int(adapter.IfIndex)] = state
			result[int(adapter.Ipv6IfIndex)] = state
		}
		return result, nil
	}
	return nil, errors.New("网卡信息在采集时持续变化")
}

func (c *WindowsCollector) network(ctx context.Context) Region[[]Network] {
	stats, err := psnet.IOCountersWithContext(ctx, true)
	if err != nil {
		c.netBaseline = map[string]timedCounter[psnet.IOCountersStat]{}
		return failed[[]Network](err)
	}
	adapters, adapterErr := windowsAdapters()
	now := time.Now()
	next := map[string]timedCounter[psnet.IOCountersStat]{}
	identities := map[string]string{}
	data := []Network{}
	for _, stat := range stats {
		item := Network{ID: stat.Name, State: "unknown", ReceiveBytes: stat.BytesRecv, TransmitBytes: stat.BytesSent, ReceiveErrors: stat.Errin, TransmitErrors: stat.Errout, ReceiveDropped: stat.Dropin, TransmitDropped: stat.Dropout, Message: "建立网卡计数基线"}
		iface, addressErr := net.InterfaceByName(stat.Name)
		if addressErr == nil {
			identities[stat.Name] = fmt.Sprintf("%d:%s", iface.Index, iface.HardwareAddr)
			item.Loopback = iface.Flags&net.FlagLoopback != 0
			item.Virtual = strings.HasPrefix(stat.Name, "vEthernet")
			item.Addresses, addressErr = interfaceAddresses(*iface)
			if adapter, ok := adapters[iface.Index]; ok {
				item.State = "down"
				if adapter.up {
					item.State = "up"
				}
				if adapter.speed > 0 {
					speed := float64(adapter.speed)
					item.SpeedBitsPerSecond = &speed
				}
			}
		}
		if addressErr != nil {
			item.AddressError = "IP 地址读取失败：" + addressErr.Error()
		}
		if old, ok := c.netBaseline[stat.Name]; ok && identities[stat.Name] != "" && identities[stat.Name] == c.netIdentity[stat.Name] && now.Sub(old.at) <= StaleAfter {
			elapsed := now.Sub(old.at)
			item.ReceiveBytesPerSecond = counterRate(old.value.BytesRecv, stat.BytesRecv, elapsed)
			item.TransmitBytesPerSecond = counterRate(old.value.BytesSent, stat.BytesSent, elapsed)
			if item.ReceiveBytesPerSecond != nil && item.TransmitBytesPerSecond != nil {
				item.Message = ""
				if item.SpeedBitsPerSecond != nil {
					rx := *item.ReceiveBytesPerSecond * 8 / *item.SpeedBitsPerSecond * 100
					tx := *item.TransmitBytesPerSecond * 8 / *item.SpeedBitsPerSecond * 100
					item.ReceiveUsagePercent = &rx
					item.TransmitUsagePercent = &tx
				}
			} else {
				item.ReceiveBytesPerSecond = nil
				item.TransmitBytesPerSecond = nil
			}
		}
		if item.SpeedBitsPerSecond == nil {
			item.Message += "; 链路速率不支持或不可读取"
		}
		data = append(data, item)
		next[stat.Name] = timedCounter[psnet.IOCountersStat]{now, stat}
	}
	if ctx.Err() != nil {
		c.netBaseline = map[string]timedCounter[psnet.IOCountersStat]{}
		return failed[[]Network](ctx.Err())
	}
	c.netBaseline, c.netIdentity = next, identities
	sort.Slice(data, func(i, j int) bool { return data[i].ID < data[j].ID })
	region := partialDevices(data, 0, "网卡")
	for _, item := range data {
		if item.AddressError != "" {
			region.Partial = true
		}
	}
	if adapterErr != nil {
		region.Partial = true
		region.Message = "链路状态与速率读取失败：" + adapterErr.Error()
	}
	return region
}

func (c *WindowsCollector) processes(ctx context.Context) Region[Processes] {
	// WMI queries honor the account's existing permissions; never enable debug privileges.
	var procs []struct {
		ProcessId      uint32
		Name           string
		CreationDate   *time.Time
		KernelModeTime *uint64
		UserModeTime   *uint64
		WorkingSetSize *uint64
	}
	if err := windowsWMI.Query("SELECT ProcessId, Name, CreationDate, KernelModeTime, UserModeTime, WorkingSetSize FROM Win32_Process", &procs); err != nil {
		c.processBaseline = map[int]processCounter{}
		return failed[Processes](err)
	}
	times, err := cpu.TimesWithContext(ctx, false)
	if err != nil || len(times) == 0 {
		c.processBaseline = map[int]processCounter{}
		if err == nil {
			err = errors.New("未返回整机 CPU 计数")
		}
		return failed[Processes](err)
	}
	now := time.Now()
	total := times[0].User + times[0].System + times[0].Idle
	next := map[int]processCounter{}
	items := []Process{}
	failures := 0
	for _, proc := range procs {
		if ctx.Err() != nil {
			c.processBaseline = map[int]processCounter{}
			return failed[Processes](ctx.Err())
		}
		if proc.ProcessId == 0 {
			continue
		}
		if proc.WorkingSetSize == nil {
			failures++
			continue
		}
		item := Process{PID: int(proc.ProcessId), Name: proc.Name, MemoryBytes: *proc.WorkingSetSize}
		if proc.CreationDate != nil && proc.KernelModeTime != nil && proc.UserModeTime != nil {
			started := uint64(proc.CreationDate.UnixNano())
			seconds := float64(*proc.KernelModeTime+*proc.UserModeTime) / 1e7
			if old, ok := c.processBaseline[item.PID]; ok && old.started == started && seconds >= old.seconds && total > c.processTotal && now.Sub(c.processAt) <= StaleAfter {
				value := max(0, min(100, 100*(seconds-old.seconds)/(total-c.processTotal)))
				item.CPUPercent = &value
			}
			next[item.PID] = processCounter{started, seconds}
		} else {
			failures++
		}
		items = append(items, item)
	}
	c.processBaseline, c.processTotal, c.processAt = next, total, now
	region := collected(topProcesses(items), now)
	if failures > 0 {
		region.Partial = true
		region.Message = fmt.Sprintf("覆盖不完整：%d 个进程字段读取失败或权限不足", failures)
	}
	if len(items) == 0 && failures > 0 {
		return failed[Processes](errors.New(region.Message))
	}
	return region
}

func (c *WindowsCollector) gpu(ctx context.Context) Region[[]GPU] {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		var controllers []struct {
			Name        string
			PNPDeviceID string
		}
		if err := windowsWMI.Query("SELECT Name, PNPDeviceID FROM Win32_VideoController", &controllers); err != nil {
			return failed[[]GPU](err)
		}
		if len(controllers) == 0 {
			return partialDevices([]GPU{}, 0, "GPU")
		}
		message := "仅正式支持 NVIDIA GPU"
		for _, controller := range controllers {
			if strings.Contains(strings.ToUpper(controller.PNPDeviceID), "VEN_10DE") || strings.Contains(strings.ToUpper(controller.Name), "NVIDIA") {
				message = "NVIDIA 驱动工具 nvidia-smi 不可用"
				break
			}
		}
		now := time.Now()
		return Region[[]GPU]{Status: StatusUnsupported, CollectedAt: &now, Message: message}
	}
	output, err := exec.CommandContext(ctx, path, "--query-gpu=uuid,name,utilization.gpu,memory.total,memory.used,temperature.gpu,power.draw,driver_version", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return failed[[]GPU](fmt.Errorf("nvidia-smi: %w", err))
	}
	data, partial, err := parseGPUCSV(string(output))
	if err != nil {
		return failed[[]GPU](err)
	}
	region := partialDevices(data, 0, "GPU")
	if partial {
		region.Partial = true
		region.Message = "部分 GPU 或字段采集失败/不支持"
		if len(data) == 0 {
			return failed[[]GPU](errors.New(region.Message))
		}
	}
	return region
}

func partialDevices[T any](items []T, failures int, resource string) Region[[]T] {
	if len(items) == 0 && failures > 0 {
		return failed[[]T](fmt.Errorf("%s 读取失败或权限不足", resource))
	}
	region := collected(items, time.Now())
	if len(items) == 0 {
		region.Status = StatusNoDevice
	}
	if failures > 0 {
		region.Partial = true
		region.Message = fmt.Sprintf("%s 覆盖不完整：%d 项读取失败或权限不足", resource, failures)
	}
	return region
}
