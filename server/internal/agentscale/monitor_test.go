package agentscale

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type pipeConn struct {
	ch     chan []byte
	closed chan struct{}
	once   sync.Once
}

func newPipeConn() *pipeConn {
	return &pipeConn{ch: make(chan []byte, 16), closed: make(chan struct{})}
}

func (p *pipeConn) Read(b []byte) (int, error) {
	select {
	case data, ok := <-p.ch:
		if !ok {
			return 0, io.EOF
		}
		return copy(b, data), nil
	case <-p.closed:
		return 0, io.EOF
	}
}
func (p *pipeConn) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}

type scriptedOpener struct {
	mu    sync.Mutex
	conns []io.ReadCloser
	calls int
}

func (o *scriptedOpener) Open(context.Context) (io.ReadCloser, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls++
	if len(o.conns) == 0 {
		return nil, errors.New("no serial device")
	}
	c := o.conns[0]
	o.conns = o.conns[1:]
	return c, nil
}
func (o *scriptedOpener) Calls() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

func testConfig() Config {
	return Config{Port: "/dev/ttyS0", Baud: 9600}
}

func newTestMonitor(t *testing.T, cfg Config, opener Opener) (*Monitor, *testClock) {
	t.Helper()
	m, err := New(cfg, opener)
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{t: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)}
	m.now = clock.Now
	return m, clock
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

var stableFrame = makeFrame(0x2d, 0x30, 0x20, "001550", "000000")
var dynamicFrame = makeFrame(0x2d, 0x38, 0x20, "001560", "000000")

func TestFreshnessComesFromValidFramesNotOpenOrSnapshot(t *testing.T) {
	conn := newPipeConn()
	opener := &scriptedOpener{conns: []io.ReadCloser{conn}}
	m, clock := newTestMonitor(t, testConfig(), opener.Open)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	waitFor(t, "port open", func() bool { return m.Snapshot(clock.Now()).Connected })
	if s := m.Snapshot(clock.Now()); s.DeviceOK || !strings.Contains(s.Fault, "等待有效帧") {
		t.Fatalf("open without data must not be usable: %+v", s)
	}
	conn.ch <- stableFrame
	waitFor(t, "first stable frame", func() bool { s := m.Snapshot(clock.Now()); return s.DeviceOK && s.Stable })
	s := m.Snapshot(clock.Now())
	if s.WeightMilliTonnes != 1550 || s.Weight != "1.55" || s.SourceWeight != "1.550" || s.ChecksumStatus != "ok" {
		t.Fatalf("snapshot: %+v", s)
	}
	clock.Advance(1100 * time.Millisecond)
	stale := m.Snapshot(clock.Now())
	if !stale.Stale || stale.DeviceOK || stale.Stable || !strings.Contains(stale.Fault, "1000 ms") {
		t.Fatalf("stale snapshot: %+v", stale)
	}
	// Repeated snapshots cannot refresh the old reading.
	clock.Advance(100 * time.Millisecond)
	again := m.Snapshot(clock.Now())
	if again.DeviceOK || again.LastValidAt != s.LastValidAt {
		t.Fatalf("old reading revived: %+v", again)
	}
	conn.ch <- dynamicFrame
	waitFor(t, "recovery", func() bool {
		s := m.Snapshot(clock.Now())
		return s.DeviceOK && !s.Stable && s.WeightMilliTonnes == 1560
	})
}

func TestSustainedChecksumFailureFaultsAndRecovers(t *testing.T) {
	conn := newPipeConn()
	opener := &scriptedOpener{conns: []io.ReadCloser{conn}}
	m, clock := newTestMonitor(t, testConfig(), opener.Open)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	waitFor(t, "port open", func() bool { return m.Snapshot(clock.Now()).Connected })
	conn.ch <- stableFrame
	waitFor(t, "valid", func() bool { return m.Snapshot(clock.Now()).DeviceOK })
	corrupt := append([]byte(nil), stableFrame...)
	corrupt[5] ^= 0x01
	for i := 0; i < maxConsecutiveBad; i++ {
		conn.ch <- corrupt
	}
	waitFor(t, "checksum fault", func() bool {
		s := m.Snapshot(clock.Now())
		return !s.DeviceOK && strings.Contains(s.Fault, "连续 3 帧校验")
	})
	conn.ch <- stableFrame
	waitFor(t, "recovery after checksum burst", func() bool {
		s := m.Snapshot(clock.Now())
		return s.DeviceOK && s.Stable && s.ChecksumFailures == 0 && s.FramesInvalid >= 3
	})
}

func TestReconnectRequiresANewFrameAndDoesNotResurrectOldStableValue(t *testing.T) {
	first, second := newPipeConn(), newPipeConn()
	opener := &scriptedOpener{conns: []io.ReadCloser{first, second}}
	m, clock := newTestMonitor(t, testConfig(), opener.Open)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	waitFor(t, "first open", func() bool { return opener.Calls() == 1 && m.Snapshot(clock.Now()).Connected })
	first.ch <- stableFrame
	waitFor(t, "first stable", func() bool { return m.Snapshot(clock.Now()).DeviceOK })
	_ = first.Close() // device disappeared / read error
	waitFor(t, "second open", func() bool { return opener.Calls() == 2 })
	s := m.Snapshot(clock.Now())
	if s.DeviceOK || s.Stable || !strings.Contains(s.Fault, "等待新的有效帧") {
		t.Fatalf("old stable value survived reconnect: %+v", s)
	}
	second.ch <- dynamicFrame
	waitFor(t, "new frame", func() bool {
		s := m.Snapshot(clock.Now())
		return s.DeviceOK && !s.Stable && s.WeightMilliTonnes == 1560
	})
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop on cancellation")
	}
}

func TestCancellationClosesPortAndStops(t *testing.T) {
	conn := newPipeConn()
	opener := &scriptedOpener{conns: []io.ReadCloser{conn}}
	m, clock := newTestMonitor(t, testConfig(), opener.Open)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	waitFor(t, "open", func() bool { return m.Snapshot(clock.Now()).Connected })
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor ignored cancellation")
	}
	select {
	case <-conn.closed:
	default:
		t.Fatal("port was not closed")
	}
	if m.Snapshot(clock.Now()).Connected {
		t.Fatal("snapshot still connected after cancellation")
	}
}

func TestUnconfiguredMonitorIsExplicitlyUnavailable(t *testing.T) {
	m, clock := newTestMonitor(t, Config{}, nil)
	s := m.Snapshot(clock.Now())
	if s.Enabled || s.DeviceOK || s.Stable || !strings.Contains(s.Fault, "未配置") {
		t.Fatalf("unconfigured snapshot: %+v", s)
	}
	if err := (Config{Port: "ttyS0", Baud: 9600}).Validate(); err == nil {
		t.Fatal("relative device path accepted")
	}
	for _, cfg := range []Config{
		{Port: "/dev/ttyS0", Baud: 1234},
		{Port: "/dev/../etc/passwd", Baud: 9600},
	} {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid config accepted: %+v", cfg)
		}
	}
}

func TestInterpretationMarksNegativeNetAnomalyAndPrecisionUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame Frame
		unit  Unit
		match string
	}{
		{"negative", mustParse(t, makeFrame(0x2d, 0x32, 0x20, "001550", "000000")), UnitTonne, "负重量"},
		{"net", mustParse(t, makeFrame(0x2d, 0x31, 0x20, "001550", "000000")), UnitTonne, "净重模式"},
		{"anomaly", mustParse(t, makeFrame(0x2d, 0x34, 0x20, "001550", "000000")), UnitTonne, "异常位"},
		{"precision", mustParse(t, makeFrame(0x2d, 0x30, 0x20, "001550", "000000")), UnitKilogram, "精度"},
	} {
		_, issue := interpret(tc.frame, tc.unit)
		if !strings.Contains(issue, tc.match) {
			t.Fatalf("%s: issue %q", tc.name, issue)
		}
	}
}

func mustParse(t *testing.T, raw []byte) Frame {
	t.Helper()
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestStreamTrimBoundsRetainedGarbage(t *testing.T) {
	var s Stream
	s.buf = make([]byte, maxStreamBuffer+100)
	s.Trim()
	if len(s.buf) != FrameLength-1 || s.Discarded() != uint64(maxStreamBuffer+100-(FrameLength-1)) {
		t.Fatalf("buffered=%d discarded=%d", len(s.buf), s.Discarded())
	}
}
