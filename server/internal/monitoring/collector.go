package monitoring

import (
	"context"
	"encoding/csv"
	"errors"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func topProcesses(items []Process) Processes {
	data := Processes{CPU: append([]Process{}, items...), Memory: append([]Process{}, items...)}
	sort.Slice(data.CPU, func(i, j int) bool {
		a, b := data.CPU[i], data.CPU[j]
		if a.CPUPercent == nil {
			return b.CPUPercent == nil && a.PID < b.PID
		}
		if b.CPUPercent == nil {
			return true
		}
		if *a.CPUPercent == *b.CPUPercent {
			return a.PID < b.PID
		}
		return *a.CPUPercent > *b.CPUPercent
	})
	sort.Slice(data.Memory, func(i, j int) bool {
		if data.Memory[i].MemoryBytes == data.Memory[j].MemoryBytes {
			return data.Memory[i].PID < data.Memory[j].PID
		}
		return data.Memory[i].MemoryBytes > data.Memory[j].MemoryBytes
	})
	data.CPU = data.CPU[:min(10, len(data.CPU))]
	data.Memory = data.Memory[:min(10, len(data.Memory))]
	return data
}

type timedCounter[T any] struct {
	at    time.Time
	value T
}
type processCounter struct {
	started uint64
	seconds float64
}

func collected[T any](data T, now time.Time) Region[T] {
	return Region[T]{Status: StatusOK, CollectedAt: &now, Data: &data}
}
func failed[T any](err error) Region[T] { return Region[T]{Status: StatusError, Message: err.Error()} }

func collectRegion[T any](ctx context.Context, lock *sync.Mutex, collect func(context.Context) Region[T], dest *Region[T], wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		if !lock.TryLock() {
			*dest = failed[T](errors.New("previous collection still running"))
			return
		}
		result := make(chan Region[T], 1)
		go func() { defer lock.Unlock(); result <- collect(ctx) }()
		select {
		case *dest = <-result:
		case <-ctx.Done():
			*dest = failed[T](ctx.Err())
		}
	}()
}

func memoryValues(total, available, swapTotal, swapFree uint64) Memory {
	available = min(total, available)
	swapFree = min(swapTotal, swapFree)
	data := Memory{TotalBytes: total, AvailableBytes: available, UsedBytes: total - available, SwapTotalBytes: swapTotal, SwapUsedBytes: swapTotal - swapFree}
	if total > 0 {
		data.UsagePercent = 100 * float64(data.UsedBytes) / float64(total)
	}
	if swapTotal > 0 {
		value := 100 * float64(data.SwapUsedBytes) / float64(swapTotal)
		data.SwapUsagePercent = &value
	}
	return data
}

func counterRate(previous, current uint64, elapsed time.Duration) *float64 {
	if elapsed <= 0 || current < previous {
		return nil
	}
	value := float64(current-previous) / elapsed.Seconds()
	return &value
}

func gpuNumber(value string) *float64 {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || number < 0 || math.IsNaN(number) || math.IsInf(number, 0) {
		return nil
	}
	return &number
}
func gpuMemory(value string) *uint64 {
	number := gpuNumber(value)
	if number == nil {
		return nil
	}
	bytes := uint64(*number * 1048576)
	return &bytes
}
func parseGPUCSV(output string) ([]GPU, bool, error) {
	reader := csv.NewReader(strings.NewReader(output))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, false, err
	}
	data := []GPU{}
	partial := false
	for _, row := range rows {
		if len(row) != 8 {
			partial = true
			continue
		}
		if strings.TrimSpace(row[0]) == "" || strings.Contains(row[0], "[Not Supported]") {
			partial = true
			continue
		}
		item := GPU{ID: strings.TrimSpace(row[0]), Name: strings.TrimSpace(row[1]), UsagePercent: gpuNumber(row[2]), MemoryTotalBytes: gpuMemory(row[3]), MemoryUsedBytes: gpuMemory(row[4]), TemperatureCelsius: gpuNumber(row[5]), PowerWatts: gpuNumber(row[6])}
		// The final field is driver version, retained as support context.
		item.Message = "driver " + strings.TrimSpace(row[7])
		if item.UsagePercent == nil || item.MemoryTotalBytes == nil || item.MemoryUsedBytes == nil || item.TemperatureCelsius == nil || item.PowerWatts == nil {
			partial = true
			item.Message += "; 部分字段不支持或不可读取"
		}
		data = append(data, item)
	}
	return data, partial, nil
}

// interfaceAddresses distinguishes an unreadable interface from one with no IP.
func interfaceAddresses(iface net.Interface) ([]IPAddress, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	return parseInterfaceAddresses(addrs)
}

func parseInterfaceAddresses(addrs []net.Addr) ([]IPAddress, error) {
	addresses := make([]IPAddress, 0, len(addrs))
	seen := map[string]bool{}
	for _, addr := range addrs {
		ip, network, err := net.ParseCIDR(addr.String())
		if err != nil {
			return nil, err
		}
		prefix, _ := network.Mask.Size()
		family := "IPv6"
		if ip.To4() != nil {
			family = "IPv4"
		}
		address := IPAddress{Family: family, IP: ip.String(), PrefixLength: prefix}
		key := address.IP + "/" + strconv.Itoa(prefix)
		if !seen[key] {
			addresses = append(addresses, address)
			seen[key] = true
		}
	}
	sort.Slice(addresses, func(i, j int) bool {
		if addresses[i].Family != addresses[j].Family {
			return addresses[i].Family < addresses[j].Family
		}
		if addresses[i].IP != addresses[j].IP {
			return addresses[i].IP < addresses[j].IP
		}
		return addresses[i].PrefixLength < addresses[j].PrefixLength
	})
	return addresses, nil
}

// cpuDelta uses Windows user + system (excluding idle) + idle seconds.
func cpuDelta(previous, current, previousIdle, currentIdle float64) *float64 {
	delta, idle := current-previous, currentIdle-previousIdle
	if delta <= 0 || idle < 0 || idle > delta {
		return nil
	}
	value := max(0, min(100, 100*(delta-idle)/delta))
	return &value
}
