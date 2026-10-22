package agentscale

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// maxConsecutiveBad makes a sustained checksum/structure failure visible before
// the normal stale threshold. A single corrupt frame followed by a good one
// does not take the scale out of service.
const maxConsecutiveBad = 3

// errReadTimeout is returned by the Linux serial reader when no bytes arrived
// within the poll interval. It is not a device failure.
var errReadTimeout = errors.New("serial read timeout")

// Config describes one locally attached D12 instrument.
type Config struct {
	Enabled  bool
	Port     string
	Baud     int
	Protocol string
	Unit     Unit
	Stale    time.Duration
}

func (c Config) Validate() error {
	if c.Protocol != "" && c.Protocol != "keli-d12-tf05" {
		return fmt.Errorf("unsupported scale protocol %q", c.Protocol)
	}
	if !c.Enabled {
		return nil
	}
	if c.Port == "" || !strings.HasPrefix(c.Port, "/dev/") || strings.Contains(c.Port, "..") {
		return errors.New("serial port must be an absolute /dev path")
	}
	if !supportedBaud(c.Baud) {
		return fmt.Errorf("unsupported serial baud %d", c.Baud)
	}
	if c.Unit != UnitTonne && c.Unit != UnitKilogram {
		return fmt.Errorf("unsupported source unit %q", c.Unit)
	}
	if c.Stale < 200*time.Millisecond || c.Stale >= 3*time.Second {
		return errors.New("stale threshold must be between 200 ms and 3000 ms")
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
}

func New(cfg Config, opener Opener) (*Monitor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if opener == nil {
		opener = SerialOpener(cfg.Port, cfg.Baud)
	}
	return &Monitor{cfg: cfg, opener: opener, now: time.Now, changes: make(chan struct{}, 1)}, nil
}

// HealthChanges signals transitions in availability, stability or fault text.
// Consumers still poll snapshots; the signal is only a prompt to report early.
func (m *Monitor) HealthChanges() <-chan struct{} { return m.changes }

// Run reads until ctx is cancelled. Open failures, read failures and device
// disappearance are retried with bounded backoff; cancellation closes the port.
func (m *Monitor) Run(ctx context.Context) {
	if !m.cfg.Enabled {
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
		},
		func(err error) {
			m.lastFrameAt = at
			m.invalid++
			m.consecutiveBad++
			m.lastFrameError = err.Error()
		},
	)
	m.stream.Trim()
	m.updateHealthLocked(at)
}

func (m *Monitor) lastMillis(f Frame) {
	m.lastMilli, m.lastIssue = interpret(f, m.cfg.Unit)
	if _, err := f.TareMilliTonnes(m.cfg.Unit); err != nil {
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
	m.updateHealthLocked(m.now())
	m.mu.Unlock()
}

func (m *Monitor) setTransportFault(reason string) {
	m.mu.Lock()
	m.connected = false
	m.armed = false
	m.transportFault = reason
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
		Enabled:           m.cfg.Enabled,
		Port:              m.cfg.Port,
		Baud:              m.cfg.Baud,
		Protocol:          m.cfg.Protocol,
		Unit:              string(m.cfg.Unit),
		Connected:         m.connected,
		Armed:             m.armed,
		StaleMS:           int(m.cfg.Stale / time.Millisecond),
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
	if !m.cfg.Enabled {
		s.Fault = "称重仪表未启用"
		return s
	}
	stale := m.hasFrame && now.Sub(m.lastValid) > m.cfg.Stale
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
		s.Fault = fmt.Sprintf("超过 %d ms 未收到有效帧", m.cfg.Stale.Milliseconds())
	case m.consecutiveBad >= maxConsecutiveBad:
		s.Fault = fmt.Sprintf("连续 %d 帧校验或结构错误", m.consecutiveBad)
	case m.lastIssue != "":
		s.Fault = m.lastIssue
	}
	return s
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
