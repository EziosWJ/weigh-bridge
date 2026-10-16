package monitoring

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	ErrForbidden = errors.New("仅 ADMIN 可访问服务器监控")
	ErrInvalid   = errors.New("无效的监控资源或设备")
	ErrDisabled  = errors.New("服务器监控模块未启用")
)

type historyFrame struct {
	at     time.Time
	values map[string]map[string]*float64
	times  map[string]time.Time
}
type warningState struct {
	high, low, last time.Time
	active          bool
	since           time.Time
}

// Service owns the current API instance's bounded, non-persistent history.
// The collector supplies immutable snapshots with original measurement times.
type Service struct {
	enabled    bool
	mu         sync.RWMutex
	authorizer Authorizer
	collector  Collector
	now        func() time.Time
	timeout    time.Duration
	interval   time.Duration
	latest     Snapshot
	frames     []historyFrame
	alerts     map[string]*warningState
}

type Option func(*Service)

func WithTimeout(timeout time.Duration) Option { return func(s *Service) { s.timeout = timeout } }
func WithEnabled(enabled bool) Option          { return func(s *Service) { s.enabled = enabled } }

// Disabled preserves authorization while dropping the collector and history.
func (s *Service) Disabled() *Service {
	return NewService(s.authorizer, nil, s.now, WithEnabled(false))
}

func NewService(authorizer Authorizer, collector Collector, now func() time.Time, options ...Option) *Service {
	if now == nil {
		now = time.Now
	}
	s := &Service{enabled: true, authorizer: authorizer, collector: collector, now: now, timeout: 4 * time.Second, interval: SampleInterval}
	for _, option := range options {
		option(s)
	}
	if !s.enabled {
		s.collector = nil
		return s
	}
	s.alerts = map[string]*warningState{}
	s.latest.Host.Status = StatusCollecting
	s.latest.CPU.Status = StatusCollecting
	s.latest.Memory.Status = StatusCollecting
	s.latest.Filesystems.Status = StatusCollecting
	s.latest.Disks.Status = StatusCollecting
	s.latest.Network.Status = StatusCollecting
	s.latest.GPU.Status = StatusCollecting
	s.latest.Processes.Status = StatusCollecting
	return s
}

func (s *Service) authorize(ctx context.Context, userID int64) error {
	if s.authorizer == nil || userID <= 0 {
		return ErrForbidden
	}
	ok, err := s.authorizer.IsAdmin(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if !s.enabled {
		return ErrDisabled
	}
	return nil
}
func (s *Service) Overview(ctx context.Context, userID int64) (Snapshot, error) {
	if err := s.authorize(ctx, userID); err != nil {
		return Snapshot{}, err
	}
	return s.Snapshot(), nil
}

func regionView[T any](r Region[T], now time.Time) Region[T] {
	r.Stale = r.CollectedAt != nil && now.Sub(*r.CollectedAt) > StaleAfter
	if r.Stale && (r.Status == StatusOK || r.Status == StatusCollecting) {
		r.Status = StatusStale
	}
	return r
}
func (s *Service) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.latest
	now := s.now()
	out.Host = regionView(out.Host, now)
	out.CPU = regionView(out.CPU, now)
	out.Memory = regionView(out.Memory, now)
	out.Filesystems = regionView(out.Filesystems, now)
	out.Disks = regionView(out.Disks, now)
	out.Network = regionView(out.Network, now)
	out.GPU = regionView(out.GPU, now)
	out.Processes = regionView(out.Processes, now)
	out.Warnings = make([]Warning, 0)
	for key, a := range s.alerts {
		if !a.active {
			continue
		}
		resource, device := splitKey(key)
		message := "资源占用达到 90% 并持续 60 秒"
		if resource == "filesystem" {
			message = "文件系统占用达到 90%"
		}
		out.Warnings = append(out.Warnings, Warning{Resource: resource, Device: device, Since: a.since, Message: message})
	}
	sort.Slice(out.Warnings, func(i, j int) bool {
		return out.Warnings[i].Resource+out.Warnings[i].Device < out.Warnings[j].Resource+out.Warnings[j].Device
	})
	return out
}

// mergeRegion never refreshes a retained value with a failed or repeated read.
func mergeRegion[T any](old, incoming Region[T], now time.Time) (Region[T], bool) {
	if incoming.Status == "" {
		incoming.Status = StatusError
		incoming.Message = "采集区域未返回数据"
	}
	valid := incoming.Status == StatusOK || incoming.Status == StatusCollecting || incoming.Status == StatusNoDevice || incoming.Status == StatusUnsupported
	newer := valid && incoming.CollectedAt != nil && !incoming.CollectedAt.After(now) && (old.CollectedAt == nil || incoming.CollectedAt.After(*old.CollectedAt))
	fresh := newer && now.Sub(*incoming.CollectedAt) <= StaleAfter
	if !newer {
		incoming.Data = old.Data
		incoming.CollectedAt = old.CollectedAt
		if valid && incoming.Status != StatusUnsupported && incoming.Status != StatusNoDevice && old.CollectedAt != nil {
			incoming.Status = old.Status
			incoming.Partial = old.Partial
			incoming.Message = old.Message
		}
		if incoming.Status == StatusNoDevice {
			incoming.Data = nil
		}
	}
	incoming.Stale = false
	return incoming, fresh
}

func (s *Service) Ingest(in Snapshot) {
	if !s.enabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	// A restarted collector cannot continue threshold durations across generations.
	if in.SourceID != "" && s.latest.SourceID != "" && in.SourceID != s.latest.SourceID {
		for _, a := range s.alerts {
			a.high = time.Time{}
			a.low = time.Time{}
			a.last = time.Time{}
		}
	}
	if in.SourceID != "" {
		s.latest.SourceID = in.SourceID
	}
	if in.SampledAt.After(s.latest.SampledAt) && !in.SampledAt.After(now) {
		s.latest.SampledAt = in.SampledAt
	}
	var cpu, memory, fs, disk, network, gpu bool
	s.latest.Host, _ = mergeRegion(s.latest.Host, in.Host, now)
	s.latest.CPU, cpu = mergeRegion(s.latest.CPU, in.CPU, now)
	s.latest.Memory, memory = mergeRegion(s.latest.Memory, in.Memory, now)
	s.latest.Filesystems, fs = mergeRegion(s.latest.Filesystems, in.Filesystems, now)
	s.latest.Disks, disk = mergeRegion(s.latest.Disks, in.Disks, now)
	s.latest.Network, network = mergeRegion(s.latest.Network, in.Network, now)
	s.latest.GPU, gpu = mergeRegion(s.latest.GPU, in.GPU, now)
	s.latest.Processes, _ = mergeRegion(s.latest.Processes, in.Processes, now)
	frame := historyFrame{at: now, values: map[string]map[string]*float64{}, times: map[string]time.Time{
		"cpu":        measurementTime(in.CPU.CollectedAt, now),
		"memory":     measurementTime(in.Memory.CollectedAt, now),
		"filesystem": measurementTime(in.Filesystems.CollectedAt, now),
		"disk":       measurementTime(in.Disks.CollectedAt, now),
		"network":    measurementTime(in.Network.CollectedAt, now),
		"gpu":        measurementTime(in.GPU.CollectedAt, now),
	}}
	var cpuValue, memoryValue *float64
	if cpu && in.CPU.Status == StatusOK && in.CPU.Data != nil {
		cpuValue = in.CPU.Data.UsagePercent
		frame.values["cpu/"] = map[string]*float64{"usagePercent": cpuValue}
		for i, v := range in.CPU.Data.PerCore {
			frame.values["cpu/"+strconv.Itoa(i)] = map[string]*float64{"usagePercent": v}
		}
	}
	if memory && in.Memory.Status == StatusOK && in.Memory.Data != nil {
		memoryValue = ptr(in.Memory.Data.UsagePercent)
		frame.values["memory/"] = map[string]*float64{"usagePercent": memoryValue}
	}
	s.updateWarning("cpu/", cpuValue, measurementTime(in.CPU.CollectedAt, now), false)
	s.updateWarning("memory/", memoryValue, measurementTime(in.Memory.CollectedAt, now), false)
	if in.Filesystems.Status == StatusNoDevice {
		for key := range s.alerts {
			resource, _ := splitKey(key)
			if resource == "filesystem" {
				delete(s.alerts, key)
			}
		}
	}
	if fs && in.Filesystems.Status == StatusOK && in.Filesystems.Data != nil {
		present := map[string]bool{}
		for _, v := range *in.Filesystems.Data {
			key := "filesystem/" + v.ID
			present[key] = true
			frame.values[key] = map[string]*float64{"usagePercent": v.UsagePercent}
			s.updateWarning(key, v.UsagePercent, measurementTime(in.Filesystems.CollectedAt, now), true)
		}
		for key := range s.alerts {
			r, _ := splitKey(key)
			if r == "filesystem" && !present[key] && !in.Filesystems.Partial {
				delete(s.alerts, key)
			}
		}
	}
	if disk && in.Disks.Status == StatusOK && in.Disks.Data != nil {
		for _, v := range *in.Disks.Data {
			frame.values["disk/"+v.ID] = map[string]*float64{"readBytesPerSecond": v.ReadBytesPerSecond, "writeBytesPerSecond": v.WriteBytesPerSecond, "readIops": v.ReadIOPS, "writeIops": v.WriteIOPS}
		}
	}
	if network && in.Network.Status == StatusOK && in.Network.Data != nil {
		for _, v := range *in.Network.Data {
			frame.values["network/"+v.ID] = map[string]*float64{"receiveBytesPerSecond": v.ReceiveBytesPerSecond, "transmitBytesPerSecond": v.TransmitBytesPerSecond}
		}
	}
	if gpu && in.GPU.Status == StatusOK && in.GPU.Data != nil {
		for _, v := range *in.GPU.Data {
			var memoryUsage *float64
			if v.MemoryTotalBytes != nil && *v.MemoryTotalBytes > 0 && v.MemoryUsedBytes != nil {
				memoryUsage = ptr(float64(*v.MemoryUsedBytes) * 100 / float64(*v.MemoryTotalBytes))
			}
			frame.values["gpu/"+v.ID] = map[string]*float64{"usagePercent": v.UsagePercent, "memoryUsagePercent": memoryUsage, "temperatureCelsius": v.TemperatureCelsius, "powerWatts": v.PowerWatts}
		}
	}
	// The scheduler produces at most one frame every five seconds. Bound also
	// protects the input boundary from accidental faster producers.
	if len(s.frames) > 0 && !now.After(s.frames[len(s.frames)-1].at) {
		return
	}
	s.frames = append(s.frames, frame)
	cutoff := now.Add(-HistoryWindow)
	first := 0
	for first < len(s.frames) && s.frames[first].at.Before(cutoff) {
		first++
	}
	if len(s.frames)-first > 361 {
		first = len(s.frames) - 361
	}
	if first > 0 {
		s.frames = append([]historyFrame(nil), s.frames[first:]...)
	}
}
func ptr(v float64) *float64 { return &v }
func measurementTime(t *time.Time, fallback time.Time) time.Time {
	if t != nil {
		return *t
	}
	return fallback
}
func splitKey(key string) (string, string) {
	for i, c := range key {
		if c == '/' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}

func (s *Service) updateWarning(key string, value *float64, at time.Time, immediate bool) {
	a := s.alerts[key]
	if a == nil {
		a = &warningState{}
		s.alerts[key] = a
	}
	if value == nil {
		a.high = time.Time{}
		a.low = time.Time{}
		a.last = time.Time{}
		return
	}
	if !a.last.IsZero() && at.Sub(a.last) > StaleAfter {
		a.high = time.Time{}
		a.low = time.Time{}
	}
	a.last = at
	if *value >= 90 {
		a.low = time.Time{}
		if a.high.IsZero() {
			a.high = at
		}
		if !a.active && (immediate || at.Sub(a.high) >= 60*time.Second) {
			a.active = true
			a.since = at
		}
	} else if *value < 85 {
		a.high = time.Time{}
		if a.low.IsZero() {
			a.low = at
		}
		if a.active && (immediate || at.Sub(a.low) >= 30*time.Second) {
			a.active = false
		}
	} else {
		a.high = time.Time{}
		a.low = time.Time{}
	}
}

func metricNames(resource string) []string {
	switch resource {
	case "cpu", "memory", "filesystem":
		return []string{"usagePercent"}
	case "disk":
		return []string{"readBytesPerSecond", "writeBytesPerSecond", "readIops", "writeIops"}
	case "network":
		return []string{"receiveBytesPerSecond", "transmitBytesPerSecond"}
	case "gpu":
		return []string{"usagePercent", "memoryUsagePercent", "temperatureCelsius", "powerWatts"}
	}
	return nil
}
func (s *Service) History(ctx context.Context, userID int64, resource, device string) (History, error) {
	if err := s.authorize(ctx, userID); err != nil {
		return History{}, err
	}
	names := metricNames(resource)
	if len(names) == 0 || len(device) > 256 || (resource == "memory" && device != "") {
		return History{}, ErrInvalid
	}
	if resource == "cpu" && device != "" {
		n, e := strconv.Atoi(device)
		if e != nil || n < 0 {
			return History{}, ErrInvalid
		}
	}
	out := History{Resource: resource, Device: device, WindowSeconds: 1800, Points: []HistoryPoint{}}
	s.mu.RLock()
	defer s.mu.RUnlock()
	cutoff := s.now().Add(-HistoryWindow)
	for _, f := range s.frames {
		if f.at.Before(cutoff) {
			continue
		}
		values := make(map[string]*float64, len(names))
		for _, name := range names {
			values[name] = nil
		}
		at := f.at
		if _, exists := f.values[resource+"/"+device]; exists {
			at = f.times[resource]
		}
		if at.Before(cutoff) {
			continue
		}
		for name, v := range f.values[resource+"/"+device] {
			values[name] = v
		}
		out.Points = append(out.Points, HistoryPoint{CollectedAt: at, Values: values})
	}
	// Socket delivery may lag behind an already recorded observation gap.
	sort.SliceStable(out.Points, func(i, j int) bool {
		return out.Points[i].CollectedAt.Before(out.Points[j].CollectedAt)
	})
	points := out.Points[:0]
	for _, point := range out.Points {
		if len(points) > 0 && points[len(points)-1].CollectedAt.Equal(point.CollectedAt) {
			for name, value := range point.Values {
				if value != nil {
					points[len(points)-1].Values[name] = value
				}
			}
		} else {
			points = append(points, point)
		}
	}
	out.Points = points
	return out, nil
}

// Run performs immediate then periodic sampling. A timed-out task remains the
// only in-flight task until it exits; ignored cancellation cannot cause overlap.
func (s *Service) Run(ctx context.Context) {
	if !s.enabled || s.collector == nil {
		return
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	type result struct {
		sample  Snapshot
		err     error
		expired bool
	}
	var pending chan result
	var cancel context.CancelFunc
	var timer *time.Timer
	start := func() {
		var collectContext context.Context
		collectContext, cancel = context.WithTimeout(ctx, s.timeout)
		pending = make(chan result, 1)
		ch := pending
		go func() {
			sample, err := s.collector.Collect(collectContext)
			ch <- result{sample: sample, err: err, expired: collectContext.Err() != nil}
		}()
		timer = time.NewTimer(s.timeout)
	}
	defer func() {
		if cancel != nil {
			cancel()
		}
		if timer != nil {
			timer.Stop()
		}
	}()
	start()
	for {
		var deadline <-chan time.Time
		if timer != nil {
			deadline = timer.C
		}
		select {
		case <-ctx.Done():
			return
		case r := <-pending:
			cancel()
			if timer == nil {
				pending = nil
				continue
			}
			timer.Stop()
			timer = nil
			pending = nil
			if r.expired {
				s.fail(context.DeadlineExceeded)
			} else if r.err != nil {
				s.fail(r.err)
			} else {
				s.Ingest(r.sample)
			}
		case <-deadline:
			cancel()
			timer = nil
			s.fail(context.DeadlineExceeded)
		case <-ticker.C:
			if pending == nil {
				start()
			} else if timer == nil {
				s.fail(context.DeadlineExceeded)
			}
		}
	}
}
func (s *Service) fail(err error) {
	message := fmt.Sprintf("采集失败: %v", err)
	in := Snapshot{}
	in.Host = Region[Host]{Status: StatusError, Message: message}
	in.CPU = Region[CPU]{Status: StatusError, Message: message}
	in.Memory = Region[Memory]{Status: StatusError, Message: message}
	in.Filesystems = Region[[]Filesystem]{Status: StatusError, Message: message}
	in.Disks = Region[[]Disk]{Status: StatusError, Message: message}
	in.Network = Region[[]Network]{Status: StatusError, Message: message}
	in.GPU = Region[[]GPU]{Status: StatusError, Message: message}
	in.Processes = Region[Processes]{Status: StatusError, Message: message}
	s.Ingest(in)
}
