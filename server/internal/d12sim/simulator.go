package d12sim

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	Interval       = 200 * time.Millisecond
	settleDuration = 4 * time.Second
)

type Command struct {
	Action string `json:"action"`
	Weight string `json:"weight,omitempty"`
}

type State struct {
	Port           string    `json:"port"`
	Weight         string    `json:"weight"`
	TargetWeight   string    `json:"targetWeight"`
	Dynamic        bool      `json:"dynamic"`
	Ramping        bool      `json:"ramping"`
	Paused         bool      `json:"paused"`
	ContinuousBad  bool      `json:"continuousBad"`
	BadOncePending bool      `json:"badOncePending"`
	ReaderPresent  bool      `json:"readerPresent"`
	Closed         bool      `json:"closed"`
	FramesSent     uint64    `json:"framesSent"`
	BadFramesSent  uint64    `json:"badFramesSent"`
	FramesSkipped  uint64    `json:"framesSkipped"`
	LastFrame      string    `json:"lastFrame"`
	LastFrameBad   bool      `json:"lastFrameBad"`
	LastSentAt     time.Time `json:"lastSentAt"`
	WriteError     string    `json:"writeError"`
	IntervalMS     int64     `json:"intervalMs"`
}

// Simulator owns a single PTY and emits frames independently of HTTP clients.
// Call Run once; Close releases the PTY after Run exits (or on startup errors).
type Simulator struct {
	mu       sync.Mutex
	terminal *terminal
	state    State
	weight   int64
	target   int64
	upAt     time.Time
	pausedAt time.Time
}

func New() (*Simulator, error) {
	pty, err := openTerminal()
	if err != nil {
		return nil, fmt.Errorf("创建虚拟串口失败：%w", err)
	}
	return &Simulator{terminal: pty, state: State{Port: pty.path, Weight: "0", TargetWeight: "0", IntervalMS: Interval.Milliseconds()}}, nil
}

func (s *Simulator) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Closed {
		return nil
	}
	s.state.Closed, s.state.ReaderPresent = true, false
	return s.terminal.close()
}

func (s *Simulator) Run(ctx context.Context) {
	ticker := time.NewTicker(Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.emit(time.Now())
		}
	}
}

func (s *Simulator) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.update(time.Now())
	return s.state
}

func (s *Simulator) Control(cmd Command) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.control(cmd, time.Now())
}

func (s *Simulator) control(cmd Command, now time.Time) error {
	if s.state.Closed {
		return errors.New("模拟器已停止")
	}
	// Validate before changing any state; malformed controls have no effects.
	var weight int64
	if cmd.Action == "set_weight" || cmd.Action == "up" {
		var err error
		weight, err = parseWeight(cmd.Weight)
		if err != nil {
			return err
		}
		if cmd.Action == "up" && weight == 0 {
			return errors.New("上磅目标须大于零；归零请使用下磅")
		}
	} else if cmd.Weight != "" {
		return errors.New("只有设置重量和模拟上磅操作接受 weight")
	}
	s.update(now)
	switch cmd.Action {
	case "set_weight", "down":
		s.weight, s.target = weight, weight
		s.upAt = time.Time{}
		s.state.Dynamic = false
	case "up":
		s.target, s.upAt = weight, now
		s.state.Dynamic = true
		if s.state.Paused {
			s.pausedAt = now
		}
		s.weight = waveWeight(weight, 0)
	case "unstable":
		s.upAt = time.Time{}
		s.state.Dynamic = true
	case "stable":
		if !s.upAt.IsZero() {
			s.weight = s.target
		}
		s.upAt = time.Time{}
		s.state.Dynamic = false
	case "pause":
		if !s.state.Paused {
			s.state.Paused, s.pausedAt = true, now
		}
	case "resume":
		if s.state.Paused {
			if !s.upAt.IsZero() {
				s.upAt = s.upAt.Add(now.Sub(s.pausedAt))
			}
			s.state.Paused = false
		}
	case "bad_once":
		if s.state.Paused || s.state.ContinuousBad || s.state.BadOncePending {
			return errors.New("请先恢复发送及正常校验，并等待已安排的单帧发送")
		}
		s.state.BadOncePending = true
	case "bad_start":
		s.state.ContinuousBad, s.state.BadOncePending = true, false
	case "bad_stop":
		s.state.ContinuousBad, s.state.BadOncePending = false, false
	default:
		return errors.New("未知操作")
	}
	s.update(now)
	return nil
}

func (s *Simulator) update(now time.Time) {
	if !s.upAt.IsZero() && !s.state.Paused {
		elapsed := now.Sub(s.upAt)
		if elapsed >= settleDuration {
			s.weight = s.target
			s.upAt = time.Time{}
			s.state.Dynamic = false
		} else {
			s.weight = waveWeight(s.target, elapsed)
		}
	}
	s.state.Weight, s.state.TargetWeight = formatWeight(s.weight), formatWeight(s.target)
	s.state.Ramping = !s.upAt.IsZero()
}

// A deterministic rising, oscillating signal: same inputs reproduce a run.
func waveWeight(target int64, elapsed time.Duration) int64 {
	step := int64(elapsed / Interval)
	if step < 0 {
		step = 0
	}
	if step > 19 {
		step = 19
	}
	weight := target*(75+step)/100 + (step%3-1)*10
	if weight < 0 {
		return 0
	}
	if weight > maxWeight {
		return maxWeight
	}
	return weight
}

func (s *Simulator) emit(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Closed {
		return
	}
	s.update(now)
	present, err := s.terminal.readerPresent()
	s.state.ReaderPresent = present
	s.state.WriteError = ""
	if err != nil {
		s.state.WriteError = err.Error()
		return
	}
	if s.state.Paused {
		return
	}
	if !present {
		s.state.FramesSkipped++
		return
	}
	bad := s.state.ContinuousBad || s.state.BadOncePending
	frame := encodeFrame(s.weight, s.state.Dynamic, bad)
	if err := s.terminal.write(frame); err != nil {
		s.state.WriteError = err.Error()
		s.state.FramesSkipped++
		return
	}
	s.state.FramesSent++
	if bad {
		s.state.BadFramesSent++
	}
	s.state.BadOncePending = false
	s.state.LastFrame = fmt.Sprintf("% x", frame)
	s.state.LastFrameBad, s.state.LastSentAt = bad, now
}
