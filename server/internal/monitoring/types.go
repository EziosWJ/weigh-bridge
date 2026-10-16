// Package monitoring provides single-host resource monitoring.
package monitoring

import (
	"context"
	"time"
)

const (
	StatusOK          = "ok"
	StatusCollecting  = "collecting"
	StatusNoDevice    = "no_device"
	StatusUnsupported = "unsupported"
	StatusError       = "error"
	StatusStale       = "stale"
	SampleInterval    = 5 * time.Second
	StaleAfter        = 15 * time.Second
	HistoryWindow     = 30 * time.Minute
)

// Region preserves the time of the last valid sample, including on failure.
// Nullable values mean unsupported/unavailable; zero is a measured value.
type Region[T any] struct {
	Status      string     `json:"status"`
	CollectedAt *time.Time `json:"collectedAt"`
	Message     string     `json:"message"`
	Partial     bool       `json:"partial"`
	Stale       bool       `json:"stale"`
	Data        *T         `json:"data"`
}

type Host struct {
	Hostname      string  `json:"hostname"`
	OS            string  `json:"os"`
	Kernel        string  `json:"kernel"`
	UptimeSeconds float64 `json:"uptimeSeconds"`
}
type CPU struct {
	Model         string     `json:"model"`
	LogicalCores  int        `json:"logicalCores"`
	PhysicalCores int        `json:"physicalCores"`
	UsagePercent  *float64   `json:"usagePercent"`
	PerCore       []*float64 `json:"perCore"`
	Load1         *float64   `json:"load1"`
	Load5         *float64   `json:"load5"`
	Load15        *float64   `json:"load15"`
}
type Memory struct {
	SwapUnsupported  bool     `json:"swapUnsupported,omitempty"`
	TotalBytes       uint64   `json:"totalBytes"`
	UsedBytes        uint64   `json:"usedBytes"`
	AvailableBytes   uint64   `json:"availableBytes"`
	UsagePercent     float64  `json:"usagePercent"`
	SwapTotalBytes   uint64   `json:"swapTotalBytes"`
	SwapUsedBytes    uint64   `json:"swapUsedBytes"`
	SwapUsagePercent *float64 `json:"swapUsagePercent"`
}
type Filesystem struct {
	ID             string   `json:"id"`
	Device         string   `json:"device"`
	Mountpoint     string   `json:"mountpoint"`
	Type           string   `json:"type"`
	TotalBytes     uint64   `json:"totalBytes"`
	UsedBytes      uint64   `json:"usedBytes"`
	AvailableBytes uint64   `json:"availableBytes"`
	UsagePercent   *float64 `json:"usagePercent"`
	Message        string   `json:"message"`
}
type Disk struct {
	ID                  string   `json:"id"`
	ReadBytesPerSecond  *float64 `json:"readBytesPerSecond"`
	WriteBytesPerSecond *float64 `json:"writeBytesPerSecond"`
	ReadIOPS            *float64 `json:"readIops"`
	WriteIOPS           *float64 `json:"writeIops"`
	Message             string   `json:"message"`
}
type IPAddress struct {
	Family       string `json:"family"`
	IP           string `json:"ip"`
	PrefixLength int    `json:"prefixLength"`
}

type Network struct {
	Addresses              []IPAddress `json:"addresses"`
	AddressError           string      `json:"addressError,omitempty"`
	ID                     string      `json:"id"`
	Loopback               bool        `json:"loopback"`
	Virtual                bool        `json:"virtual"`
	State                  string      `json:"state"`
	SpeedBitsPerSecond     *float64    `json:"speedBitsPerSecond"`
	ReceiveBytes           uint64      `json:"receiveBytes"`
	TransmitBytes          uint64      `json:"transmitBytes"`
	ReceiveBytesPerSecond  *float64    `json:"receiveBytesPerSecond"`
	TransmitBytesPerSecond *float64    `json:"transmitBytesPerSecond"`
	ReceiveUsagePercent    *float64    `json:"receiveUsagePercent"`
	TransmitUsagePercent   *float64    `json:"transmitUsagePercent"`
	ReceiveErrors          uint64      `json:"receiveErrors"`
	TransmitErrors         uint64      `json:"transmitErrors"`
	ReceiveDropped         uint64      `json:"receiveDropped"`
	TransmitDropped        uint64      `json:"transmitDropped"`
	Message                string      `json:"message"`
}
type GPU struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	UsagePercent       *float64 `json:"usagePercent"`
	MemoryTotalBytes   *uint64  `json:"memoryTotalBytes"`
	MemoryUsedBytes    *uint64  `json:"memoryUsedBytes"`
	TemperatureCelsius *float64 `json:"temperatureCelsius"`
	PowerWatts         *float64 `json:"powerWatts"`
	Message            string   `json:"message"`
}
type Process struct {
	PID         int      `json:"pid"`
	Name        string   `json:"name"`
	CPUPercent  *float64 `json:"cpuPercent"`
	MemoryBytes uint64   `json:"memoryBytes"`
}
type Processes struct {
	CPU    []Process `json:"cpu"`
	Memory []Process `json:"memory"`
}
type Warning struct {
	Resource string    `json:"resource"`
	Device   string    `json:"device"`
	Since    time.Time `json:"since"`
	Message  string    `json:"message"`
}
type Snapshot struct {
	SourceID    string               `json:"sourceId"`
	SampledAt   time.Time            `json:"sampledAt"`
	Host        Region[Host]         `json:"host"`
	CPU         Region[CPU]          `json:"cpu"`
	Memory      Region[Memory]       `json:"memory"`
	Filesystems Region[[]Filesystem] `json:"filesystems"`
	Disks       Region[[]Disk]       `json:"disks"`
	Network     Region[[]Network]    `json:"network"`
	GPU         Region[[]GPU]        `json:"gpu"`
	Processes   Region[Processes]    `json:"processes"`
	Warnings    []Warning            `json:"warnings"`
}
type HistoryPoint struct {
	CollectedAt time.Time           `json:"collectedAt"`
	Values      map[string]*float64 `json:"values"`
}
type History struct {
	Resource      string         `json:"resource"`
	Device        string         `json:"device"`
	WindowSeconds int            `json:"windowSeconds"`
	Points        []HistoryPoint `json:"points"`
}
type Collector interface {
	Collect(context.Context) (Snapshot, error)
}
type Authorizer interface {
	IsAdmin(context.Context, int64) (bool, error)
}
