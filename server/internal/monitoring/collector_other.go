//go:build !linux && !windows

package monitoring

import (
	"context"
	"time"
)

type unsupportedCollector struct{}

func NewLinuxCollector() (Collector, error) {
	return unsupportedCollector{}, nil
}

func NewNativeCollector() (Collector, error) { return unsupportedCollector{}, nil }

func unsupportedRegion[T any](at time.Time) Region[T] {
	return Region[T]{Status: StatusUnsupported, CollectedAt: &at, Message: "正式硬件采集仅支持 Linux / Windows Server 宿主机"}
}

func (unsupportedCollector) Collect(context.Context) (Snapshot, error) {
	now := time.Now()
	return Snapshot{SourceID: "unsupported-platform", SampledAt: now, Host: unsupportedRegion[Host](now), CPU: unsupportedRegion[CPU](now), Memory: unsupportedRegion[Memory](now), Filesystems: unsupportedRegion[[]Filesystem](now), Disks: unsupportedRegion[[]Disk](now), Network: unsupportedRegion[[]Network](now), GPU: unsupportedRegion[[]GPU](now), Processes: unsupportedRegion[Processes](now)}, nil
}
