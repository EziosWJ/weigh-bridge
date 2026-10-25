package agentscale

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// maxConsecutiveBad makes a sustained checksum/structure failure visible before
// the normal stale threshold. A single corrupt frame followed by a good one
// does not take the scale out of service, but it does latch the current
// measurement basis invalid because a formal capture must not hide a serial
// error that happened inside the image capture interval.
const maxConsecutiveBad = 3

const (
	// ProtocolName is the only supported instrument profile after ADR 0030.
	ProtocolName = "keli-d12-tf05"
	// StaleThreshold is the fixed 1 s freshness limit for a valid frame.
	StaleThreshold = time.Second
	// DefaultBaud is the documented D12 setting; the site may still choose
	// another supported value when its instrument is configured differently.
	DefaultBaud = 9600
)

// errReadTimeout is returned by the Linux serial reader when no bytes arrived
// within the poll interval. It is not a device failure.
var errReadTimeout = errors.New("serial read timeout")

// Measurement errors are mapped to protocol error codes by the Agent. The
// monitor itself stays protocol-free.
var (
	ErrMeasurementUnavailable       = errors.New("称重仪表未配置或不可用，无法建立测量依据")
	ErrMeasurementFault             = errors.New("称重仪表故障或数据无效，无法建立测量依据")
	ErrMeasurementStale             = errors.New("超过 1 秒未收到有效帧，测量依据已过期")
	ErrMeasurementUnstable          = errors.New("称重仪表未稳定，无法建立测量依据")
	ErrMeasurementWeightChanged     = errors.New("测量期间重量发生变化，请重新采集")
	ErrMeasurementGenerationInvalid = errors.New("测量依据已失效或代次无效，请重新采集")
	ErrMeasurementSuperseded        = errors.New("测量窗口已被新的采集替代，请重新采集")
)

// Config describes one locally attached D12 instrument. The protocol, source
// unit and freshness threshold are fixed; only the local port and baud come
// from the Agent's device settings.
type Config struct {
	Port string
	Baud int
}

// Configured reports whether a serial path exists. An unconfigured Agent keeps
// its maintenance connection and printing ability.
func (c Config) Configured() bool { return c.Port != "" }

func (c Config) Validate() error {
	if c.Port == "" {
		return nil
	}
	if !strings.HasPrefix(c.Port, "/dev/") || strings.Contains(c.Port, "..") {
		return errors.New("serial port must be an absolute /dev path")
	}
	if !supportedBaud(c.Baud) {
		return fmt.Errorf("unsupported serial baud %d", c.Baud)
	}
	return nil
}

// Opener opens a byte stream. Tests substitute a fake; production uses the
// Linux termios implementation.
type Opener func(context.Context) (io.ReadCloser, error)

// SerialOpener returns the production serial opener for a device path.
func SerialOpener(path string, baud int) Opener {
	return func(context.Context) (io.ReadCloser, error) { return openSerial(path, baud) }
}

// Snapshot is the immutable diagnostic view consumed by the Agent and the
// local maintenance file. Freshness is computed at read time, never stored.
type Snapshot struct {
	Enabled           bool      `json:"enabled"`
	Port              string    `json:"port"`
	Baud              int       `json:"baud"`
	Protocol          string    `json:"protocol"`
	Unit              string    `json:"unit"`
	Connected         bool      `json:"connected"`
	Armed             bool      `json:"armed"`
	DeviceOK          bool      `json:"deviceOk"`
	Stable            bool      `json:"stable"`
	Stale             bool      `json:"stale"`
	StaleMS           int       `json:"staleMs"`
	WeightMilliTonnes int64     `json:"weightMilliTonnes"`
	Weight            string    `json:"weight"`
	SourceWeight      string    `json:"sourceWeight"`
	SourceTare        string    `json:"sourceTare"`
	TareIssue         string    `json:"tareIssue,omitempty"`
	RawWeight         int64     `json:"rawWeight"`
	RawTare           int64     `json:"rawTare"`
	DecimalPosition   int       `json:"decimalPosition"`
	WeightType        string    `json:"weightType"`
	InstrumentStable  bool      `json:"instrumentStable"`
	Negative          bool      `json:"negative"`
	Anomaly           bool      `json:"anomaly"`
	UnitFlag          string    `json:"unitFlag"`
	PrintFlag         bool      `json:"printFlag"`
	ExtendedFlag      bool      `json:"extendedFlag"`
	LastValidAt       time.Time `json:"lastValidAt"`
	LastFrameAt       time.Time `json:"lastFrameAt"`
	Fault             string    `json:"fault"`
	ChecksumStatus    string    `json:"checksum"`
	ChecksumFailures  int       `json:"checksumFailures"`
	LastFrameError    string    `json:"lastFrameError,omitempty"`
	FramesValid       uint64    `json:"framesValid"`
	FramesInvalid     uint64    `json:"framesInvalid"`
	DiscardedBytes    uint64    `json:"discardedBytes"`
	RawFrame          string    `json:"rawFrame"`
}

// Measurement is one atomic begin/end/verify view of the instrument basis.
type Measurement struct {
	Generation string
	Snapshot   Snapshot
	EndedAt    time.Time
}

// MeasurementInvalidation is an unsolicited transition that made a window
// invalid after it was opened.
type MeasurementInvalidation struct {
	Generation string
	Reason     string
	Snapshot   Snapshot
	At         time.Time
}

type measurementWindow struct {
	generation string
	basis      uint64
	weight     int64
	at         time.Time
	ended      bool
	cancelled  bool
	superseded bool
	notified   bool
	endedAt    time.Time
	verifiedAt time.Time
}

type measurementTracker struct {
	runID   string
	basis   uint64
	seq     uint64
	windows map[string]*measurementWindow
}

// Monitor owns one serial reader. It has no WSS dependency; the Agent starts it
// once and reads snapshots across connection retries.
type Monitor struct {
	cfg    Config
	opener Opener
	now    func() time.Time

	mu             sync.RWMutex
	stream         Stream
	connected      bool
	armed          bool
	hasFrame       bool
	lastFrame      Frame
	lastMilli      int64
	lastIssue      string
	tareIssue      string
	lastValid      time.Time
	lastFrameAt    time.Time
	transportFault string
	lastFrameError string
	valid          uint64
	invalid        uint64
	consecutiveBad int
	lastHealth     string
	changes        chan struct{}
	measurements   measurementTracker
}

func New(cfg Config, opener Opener) (*Monitor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Baud == 0 && cfg.Configured() {
		cfg.Baud = DefaultBaud
	}
	if opener == nil && cfg.Configured() {
		opener = SerialOpener(cfg.Port, cfg.Baud)
	}
	return &Monitor{
		cfg:          cfg,
		opener:       opener,
		now:          time.Now,
		changes:      make(chan struct{}, 1),
		measurements: measurementTracker{runID: newMeasurementRunID(), windows: map[string]*measurementWindow{}},
	}, nil
}

func newMeasurementRunID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// HealthChanges signals transitions in availability, stability or fault text.
// Consumers still poll snapshots; the signal is only a prompt to report early.
func (m *Monitor) HealthChanges() <-chan struct{} { return m.changes }

// Run reads until ctx is cancelled. Open failures, read failures and device
// disappearance are retried with bounded backoff; cancellation closes the port.
func (m *Monitor) Run(ctx context.Context) {
	if !m.cfg.Configured() {
		return
	}
	backoff := 200 * time.Millisecond
	for ctx.Err() == nil {
		conn, err := m.opener(ctx)
		if err != nil {
			m.setTransportFault("串口打开失败：" + err.Error())
			if !sleepContext(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = 200 * time.Millisecond
		m.opened()
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-done:
			}
		}()
		err = m.consume(ctx, conn)
		close(done)
		_ = conn.Close()
		m.closed(err)
		if ctx.Err() != nil {
			return
		}
		if !sleepContext(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

func (m *Monitor) consume(ctx context.Context, conn io.Reader) error {
	buf := make([]byte, 512)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			m.feed(buf[:n], m.now())
		}
		if errors.Is(err, errReadTimeout) {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
}

func (m *Monitor) feed(data []byte, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stream.Feed(data,
		func(f Frame) {
			wasHasFrame, wasArmed := m.hasFrame, m.armed
			oldMilli, oldIssue, oldStable := m.lastMilli, m.lastIssue, m.lastFrame.Stable
			m.hasFrame = true
			m.armed = true
			m.lastFrame = f
			m.lastMillis(f)
			m.lastValid = at
			m.lastFrameAt = at
			m.valid++
			m.consecutiveBad = 0
			m.lastFrameError = ""
			m.transportFault = ""
			// Every parsed frame participates in the basis. A repeated identical
			// stable frame does not change it; any value, stability or fault change
			// does, even when it later returns to the previous value.
			if !wasHasFrame || !wasArmed || m.lastMilli != oldMilli || m.lastIssue != oldIssue || f.Stable != oldStable {
				m.measurements.basis++
			}
		},
		func(err error) {
			m.lastFrameAt = at
			m.invalid++
			m.consecutiveBad++
			m.lastFrameError = err.Error()
			// A malformed frame is a serial error and latches invalidity for any
			// capture that spans it.
			m.measurements.basis++
		},
	)
	m.stream.Trim()
	m.updateHealthLocked(at)
}

func (m *Monitor) lastMillis(f Frame) {
	m.lastMilli, m.lastIssue = interpret(f, UnitTonne)
	if _, err := f.TareMilliTonnes(UnitTonne); err != nil {
		m.tareIssue = err.Error()
	} else {
		m.tareIssue = ""
	}
}

func interpret(f Frame, unit Unit) (int64, string) {
	value, err := f.SignedMilliTonnes(unit)
	var issues []string
	switch {
	case errors.Is(err, ErrUnsupportedUnit):
		issues = append(issues, "仪表源单位不受支持")
	case errors.Is(err, ErrPrecision):
		issues = append(issues, "仪表精度无法无损换算为 0.001 吨")
	case errors.Is(err, ErrWeightOverflow):
		issues = append(issues, "仪表重量超出可表示范围")
	case err != nil:
		issues = append(issues, err.Error())
	}
	if f.WeightType == "net" {
		issues = append(issues, "仪表处于净重模式，首期仅支持毛重测量")
	}
	if f.Anomaly {
		issues = append(issues, "仪表异常位已置位（超载或低于零）")
	}
	if f.WeightNegative {
		issues = append(issues, "仪表显示负重量，不能作为正式采集值")
	}
	return value, strings.Join(issues, "；")
}

func (m *Monitor) opened() {
	m.mu.Lock()
	m.connected = true
	m.armed = false // a fresh frame, not a successful open, restores availability
	m.transportFault = ""
	m.measurements.basis++
	m.updateHealthLocked(m.now())
	m.mu.Unlock()
}

func (m *Monitor) closed(err error) {
	m.mu.Lock()
	m.connected = false
	m.armed = false
	if err != nil {
		m.transportFault = "串口读取中断：" + err.Error()
	} else if m.transportFault == "" {
		m.transportFault = "串口读取已停止"
	}
	m.measurements.basis++
	m.updateHealthLocked(m.now())
	m.mu.Unlock()
}

func (m *Monitor) setTransportFault(reason string) {
	m.mu.Lock()
	m.connected = false
	m.armed = false
	m.transportFault = reason
	m.measurements.basis++
	m.updateHealthLocked(m.now())
	m.mu.Unlock()
}

// Snapshot reports current availability using the caller's clock so stale data
// and clock-skew checks are explicit and testable.
func (m *Monitor) Snapshot(now time.Time) Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshotLocked(now)
}

func (m *Monitor) snapshotLocked(now time.Time) Snapshot {
	s := Snapshot{
		Enabled:           m.cfg.Configured(),
		Port:              m.cfg.Port,
		Baud:              m.cfg.Baud,
		Protocol:          ProtocolName,
		Unit:              string(UnitTonne),
		Connected:         m.connected,
		Armed:             m.armed,
		StaleMS:           int(StaleThreshold / time.Millisecond),
		WeightMilliTonnes: m.lastMilli,
		Weight:            FormatMilliTonnes(m.lastMilli),
		SourceWeight:      m.lastFrame.WeightSource,
		SourceTare:        m.lastFrame.TareSource,
		TareIssue:         m.tareIssue,
		RawWeight:         m.lastFrame.RawWeight,
		RawTare:           m.lastFrame.RawTare,
		DecimalPosition:   m.lastFrame.DecimalPosition,
		WeightType:        m.lastFrame.WeightType,
		InstrumentStable:  m.hasFrame && m.lastFrame.Stable,
		Negative:          m.lastFrame.WeightNegative,
		Anomaly:           m.lastFrame.Anomaly,
		UnitFlag:          m.lastFrame.UnitFlag,
		PrintFlag:         m.lastFrame.PrintFlag,
		ExtendedFlag:      m.lastFrame.ExtendedFlag,
		LastValidAt:       m.lastValid,
		LastFrameAt:       m.lastFrameAt,
		ChecksumStatus:    "ok",
		ChecksumFailures:  m.consecutiveBad,
		LastFrameError:    m.lastFrameError,
		FramesValid:       m.valid,
		FramesInvalid:     m.invalid,
		DiscardedBytes:    m.stream.Discarded(),
	}
	if m.hasFrame {
		s.RawFrame = m.lastFrame.Hex()
	}
	if m.consecutiveBad > 0 {
		s.ChecksumStatus = "failed"
	}
	if !m.cfg.Configured() {
		s.Fault = "称重仪表尚未配置串口路径，仅打印可用"
		return s
	}
	stale := m.hasFrame && now.Sub(m.lastValid) > StaleThreshold
	s.Stale = stale
	healthy := m.connected && m.armed && m.hasFrame && !stale && m.consecutiveBad < maxConsecutiveBad && m.lastIssue == ""
	s.DeviceOK = healthy
	s.Stable = healthy && m.lastFrame.Stable
	switch {
	case m.transportFault != "":
		s.Fault = m.transportFault
	case !m.connected:
		s.Fault = "串口未连接"
	case !m.hasFrame && m.invalid > 0 && m.consecutiveBad >= maxConsecutiveBad:
		s.Fault = fmt.Sprintf("连续 %d 帧校验或结构错误，尚未收到有效帧", m.consecutiveBad)
	case !m.hasFrame:
		s.Fault = "串口已打开，等待有效帧"
	case !m.armed:
		s.Fault = "串口已重新打开，等待新的有效帧"
	case stale:
		s.Fault = "超过 1000 ms 未收到有效帧"
	case m.consecutiveBad >= maxConsecutiveBad:
		s.Fault = fmt.Sprintf("连续 %d 帧校验或结构错误", m.consecutiveBad)
	case m.lastIssue != "":
		s.Fault = m.lastIssue
	}
	return s
}

// BeginMeasurement atomically captures the current basis. Repeated identical
// stable frames leave it valid; any later value, stability, serial or fault
// change makes end/verify fail even if the reading returns to this value.
func (m *Monitor) BeginMeasurement(now time.Time) (Measurement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneMeasurementsLocked(now)
	s := m.snapshotLocked(now)
	if err := m.readinessLocked(s); err != nil {
		return Measurement{Snapshot: s}, err
	}
	for _, w := range m.measurements.windows {
		if !w.cancelled {
			w.superseded = true
		}
	}
	m.measurements.seq++
	gen := fmt.Sprintf("%s-%d-%d", m.measurements.runID, m.measurements.basis, m.measurements.seq)
	w := &measurementWindow{generation: gen, basis: m.measurements.basis, weight: s.WeightMilliTonnes, at: now}
	m.measurements.windows[gen] = w
	return Measurement{Generation: gen, Snapshot: s}, nil
}

// EndMeasurement verifies that the window survived the image capture interval
// without any invalidating transition. The basis is not cleared here: it stays
// valid until the operator confirmation calls VerifyMeasurement.
func (m *Monitor) EndMeasurement(generation string, now time.Time) (Measurement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.snapshotLocked(now)
	w := m.measurements.windows[generation]
	if w == nil {
		return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementGenerationInvalid
	}
	if w.cancelled {
		return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementGenerationInvalid
	}
	if w.superseded {
		return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementSuperseded
	}
	if w.basis != m.measurements.basis {
		if s.WeightMilliTonnes != w.weight {
			return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementWeightChanged
		}
		return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementGenerationInvalid
	}
	if err := m.readinessLocked(s); err != nil {
		return Measurement{Generation: generation, Snapshot: s}, err
	}
	if s.WeightMilliTonnes != w.weight {
		return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementWeightChanged
	}
	w.ended, w.endedAt = true, now
	return Measurement{Generation: generation, Snapshot: s, EndedAt: now}, nil
}

// VerifyMeasurement rechecks the same basis at operator confirmation, after
// the capture window ended. A transient change that restored the same value
// already changed the basis and is rejected here.
func (m *Monitor) VerifyMeasurement(generation string, now time.Time) (Measurement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.snapshotLocked(now)
	w := m.measurements.windows[generation]
	if w == nil || !w.ended || w.cancelled {
		return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementGenerationInvalid
	}
	if w.superseded {
		return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementSuperseded
	}
	if w.basis != m.measurements.basis {
		if s.WeightMilliTonnes != w.weight {
			return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementWeightChanged
		}
		return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementGenerationInvalid
	}
	if err := m.readinessLocked(s); err != nil {
		return Measurement{Generation: generation, Snapshot: s}, err
	}
	if s.WeightMilliTonnes != w.weight {
		return Measurement{Generation: generation, Snapshot: s}, ErrMeasurementWeightChanged
	}
	w.verifiedAt = now
	return Measurement{Generation: generation, Snapshot: s, EndedAt: w.endedAt}, nil
}

// CancelMeasurement releases a window that will never be confirmed. An unknown
// or already superseded generation returns ErrMeasurementGenerationInvalid.
func (m *Monitor) CancelMeasurement(generation string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.measurements.windows[generation]
	if w == nil {
		return ErrMeasurementGenerationInvalid
	}
	if w.superseded {
		return ErrMeasurementSuperseded
	}
	w.cancelled = true
	return nil
}

// InvalidateMeasurements latches every open window invalid, for example when
// the Server connection changes. The basis also advances so future windows
// cannot reuse an old reading.
func (m *Monitor) InvalidateMeasurements(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.measurements.basis++
	for _, w := range m.measurements.windows {
		if !w.cancelled && !w.superseded {
			w.superseded = true
		}
	}
}

// InvalidatedMeasurements reports windows that must be declared invalid to the
// Server. Each window is reported at most once; the correlated end/verify
// reply remains authoritative.
func (m *Monitor) InvalidatedMeasurements(now time.Time) []MeasurementInvalidation {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.snapshotLocked(now)
	var out []MeasurementInvalidation
	for _, w := range m.measurements.windows {
		if w.cancelled || w.superseded || w.notified {
			continue
		}
		reason := ""
		switch {
		case w.basis != m.measurements.basis:
			reason = "测量期间重量或仪表状态发生变化，本次依据已失效"
		case s.Stale:
			reason = ErrMeasurementStale.Error()
		case !s.DeviceOK:
			if s.Fault != "" {
				reason = s.Fault
			} else {
				reason = ErrMeasurementFault.Error()
			}
		case !s.Stable:
			reason = ErrMeasurementUnstable.Error()
		case s.WeightMilliTonnes != w.weight:
			reason = ErrMeasurementWeightChanged.Error()
		}
		if reason == "" {
			continue
		}
		w.notified = true
		out = append(out, MeasurementInvalidation{Generation: w.generation, Reason: reason, Snapshot: s, At: now})
	}
	return out
}

func (m *Monitor) readinessLocked(s Snapshot) error {
	switch {
	case !m.cfg.Configured():
		return ErrMeasurementUnavailable
	case s.Stale:
		return ErrMeasurementStale
	case !s.DeviceOK:
		return ErrMeasurementFault
	case !s.Stable:
		return ErrMeasurementUnstable
	case s.WeightMilliTonnes <= 0:
		return ErrMeasurementUnavailable
	}
	return nil
}

func (m *Monitor) pruneMeasurementsLocked(now time.Time) {
	for gen, w := range m.measurements.windows {
		if now.Sub(w.at) > time.Hour {
			delete(m.measurements.windows, gen)
		}
	}
}

func (m *Monitor) updateHealthLocked(at time.Time) {
	s := m.snapshotLocked(at)
	key := fmt.Sprintf("%t|%t|%t|%t|%d|%s", s.DeviceOK, s.Stable, s.Connected, s.Stale, s.ChecksumFailures, s.Fault)
	if key == m.lastHealth {
		return
	}
	m.lastHealth = key
	select {
	case m.changes <- struct{}{}:
	default:
	}
}

func supportedBaud(baud int) bool {
	switch baud {
	case 1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200:
		return true
	default:
		return false
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 2*time.Second {
		return 2 * time.Second
	}
	return d
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
