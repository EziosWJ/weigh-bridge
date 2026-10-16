//go:build windows

package monitoring

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"sort"
	"strings"
	"unsafe"
)

// Windows MULTI_SZ strings contain one NUL per path and a final extra NUL.
func windowsStrings(buffer []uint16) []string {
	result := []string{}
	start := 0
	for i, char := range buffer {
		if char == 0 {
			if i == start {
				break
			}
			result = append(result, windows.UTF16ToString(buffer[start:i]))
			start = i + 1
		}
	}
	return result
}

func windowsFilesystems(ctx context.Context) Region[[]Filesystem] {
	name := make([]uint16, 1024)
	handle, err := windows.FindFirstVolume(&name[0], uint32(len(name)))
	if err != nil {
		return failed[[]Filesystem](err)
	}
	defer windows.FindVolumeClose(handle)
	data := []Filesystem{}
	failures := 0
	for {
		if ctx.Err() != nil {
			return failed[[]Filesystem](ctx.Err())
		}
		volume := windows.UTF16ToString(name)
		paths := make([]uint16, 1024)
		var size uint32
		err := windows.GetVolumePathNamesForVolumeName(&name[0], &paths[0], uint32(len(paths)), &size)
		if errors.Is(err, windows.ERROR_MORE_DATA) {
			paths = make([]uint16, size)
			err = windows.GetVolumePathNamesForVolumeName(&name[0], &paths[0], size, &size)
		}
		mounts := windowsStrings(paths)
		if err != nil {
			failures++
		} else {
			if len(mounts) == 0 {
				mounts = []string{volume}
			}
			for _, mount := range mounts {
				if ctx.Err() != nil {
					return failed[[]Filesystem](ctx.Err())
				}
				path, err := windows.UTF16PtrFromString(mount)
				if err != nil {
					failures++
					continue
				}
				var available, total, free uint64
				if err := windows.GetDiskFreeSpaceEx(path, &available, &total, &free); err != nil {
					failures++
					continue
				}
				used := total - min(total, free)
				var percent *float64
				if used+available > 0 {
					value := 100 * float64(used) / float64(used+available)
					percent = &value
				}
				kind := make([]uint16, 256)
				message := ""
				if err := windows.GetVolumeInformation(path, nil, 0, nil, nil, nil, &kind[0], uint32(len(kind))); err != nil {
					message = "文件系统类型读取失败：" + err.Error()
				}
				if free > total {
					used = total - min(total, available)
					value := 100 * float64(used) / float64(max(1, total))
					percent = &value
					message += "; 当前账号配额容量，非整卷容量"
				}
				data = append(data, Filesystem{ID: volume + mount, Device: volume, Mountpoint: mount, Type: windows.UTF16ToString(kind), TotalBytes: total, UsedBytes: used, AvailableBytes: available, UsagePercent: percent, Message: message})
			}
		}
		err = windows.FindNextVolume(handle, &name[0], uint32(len(name)))
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			break
		}
		if err != nil {
			failures++
			break
		}
	}
	sort.Slice(data, func(i, j int) bool { return data[i].Mountpoint < data[j].Mountpoint })
	return partialDevices(data, failures, "卷容量")
}

type windowsDiskStat struct{ readBytes, writeBytes, readCount, writeCount uint64 }

// DISK_PERFORMANCE layout follows the Win32 ABI (including trailing alignment).
type windowsDiskPerformance struct {
	BytesRead           int64
	BytesWritten        int64
	ReadTime            int64
	WriteTime           int64
	IdleTime            int64
	ReadCount           uint32
	WriteCount          uint32
	QueueDepth          uint32
	SplitCount          uint32
	QueryTime           int64
	StorageDeviceNumber uint32
	StorageManagerName  [8]uint16
	Padding             uint32
}

func windowsDiskCounters(ctx context.Context) (map[string]windowsDiskStat, int, int, error) {
	var names []string
	for size := 4096; ; size *= 2 {
		buffer := make([]uint16, size)
		count, err := windows.QueryDosDevice(nil, &buffer[0], uint32(len(buffer)))
		if errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) && size < 1<<20 {
			continue
		}
		if err != nil {
			return nil, 0, 0, err
		}
		names = windowsStrings(buffer[:count])
		break
	}
	result := map[string]windowsDiskStat{}
	failures, unsupported := 0, 0
	for _, name := range names {
		if !strings.HasPrefix(name, "PhysicalDrive") {
			continue
		}
		if ctx.Err() != nil {
			return nil, failures, unsupported, ctx.Err()
		}
		counters, err := readWindowsDisk(name)
		if errors.Is(err, windows.ERROR_INVALID_FUNCTION) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) {
			unsupported++
			continue
		}
		if err != nil {
			failures++
			continue
		}
		result[name] = counters
	}
	return result, failures, unsupported, nil
}

func readWindowsDisk(name string) (windowsDiskStat, error) {
	path, err := windows.UTF16PtrFromString(`\\.\` + name)
	if err != nil {
		return windowsDiskStat{}, err
	}
	handle, err := windows.CreateFile(path, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return windowsDiskStat{}, err
	}
	defer windows.CloseHandle(handle)
	var data windowsDiskPerformance
	var returned uint32
	const ioctlDiskPerformance = 0x70020
	if err := windows.DeviceIoControl(handle, ioctlDiskPerformance, nil, 0, (*byte)(unsafe.Pointer(&data)), uint32(unsafe.Sizeof(data)), &returned, nil); err != nil {
		return windowsDiskStat{}, err
	}
	return windowsDiskStat{uint64(data.BytesRead), uint64(data.BytesWritten), uint64(data.ReadCount), uint64(data.WriteCount)}, nil
}
