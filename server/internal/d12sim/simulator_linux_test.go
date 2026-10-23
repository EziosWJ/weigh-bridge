//go:build linux

package d12sim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentscale"
	"golang.org/x/sys/unix"
)

func testSimulator(t *testing.T) *Simulator {
	t.Helper()
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func TestControlledWavePauseAndIndependentFaults(t *testing.T) {
	s := testSimulator(t)
	now := time.Now()
	command := func(action, weight string) {
		t.Helper()
		if err := s.control(Command{Action: action, Weight: weight}, now); err != nil {
			t.Fatal(err)
		}
	}
	command("up", "30")
	if !s.state.Dynamic || !s.state.Ramping || s.state.Weight == "30" {
		t.Fatalf("up state: %+v", s.state)
	}
	now = now.Add(2 * time.Second)
	command("pause", "")
	paused := s.state.Weight
	now = now.Add(10 * time.Second)
	s.update(now)
	if s.state.Weight != paused || !s.state.Dynamic {
		t.Fatalf("paused ramp progressed: %+v", s.state)
	}
	command("resume", "")
	now = now.Add(2 * time.Second)
	s.update(now)
	if s.state.Weight != "30" || s.state.Dynamic || s.state.Ramping {
		t.Fatalf("ramp did not settle: %+v", s.state)
	}
	command("unstable", "")
	if s.state.Weight != "30" || !s.state.Dynamic {
		t.Fatalf("dynamic bit should preserve weight: %+v", s.state)
	}
	command("pause", "")
	command("bad_start", "")
	command("set_weight", "12.345")
	if !s.state.Paused || !s.state.ContinuousBad || s.state.Weight != "12.345" || s.state.Dynamic {
		t.Fatalf("weight operation cleared unrelated fault: %+v", s.state)
	}
	command("resume", "")
	if err := s.control(Command{Action: "bad_once"}, now); err == nil {
		t.Fatal("single bad frame accepted during continuous corruption")
	}
	command("bad_stop", "")
	command("bad_once", "")
	command("down", "")
	if !s.state.BadOncePending || s.state.Weight != "0" || s.state.Dynamic {
		t.Fatalf("down state: %+v", s.state)
	}
	before := s.state
	if err := s.control(Command{Action: "set_weight", Weight: "1000"}, now); err == nil || s.state != before {
		t.Fatal("invalid weight changed state")
	}
}

func TestHTTPControlsAndValidation(t *testing.T) {
	s := testSimulator(t)
	h := s.Handler()
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"action":"set_weight","weight":"1.56"}`, 200},
		{`{"action":"set_weight","weight":1.56}`, 400},
		{`{"action":"set_weight","weight":"0.0001"}`, 400},
		{`{"action":"up","weight":"0"}`, 400},
		{`{"action":"unknown"}`, 400},
		{`{"action":"down","unexpected":true}`, 400},
		{`{"action":"down"} {"action":"pause"}`, 400},
		{`{"action":"down","weight":"1"}`, 400},
		{`{"action":"down","padding":"` + strings.Repeat("x", 5000) + `"}`, 400},
	} {
		before := s.State()
		request := httptest.NewRequest(http.MethodPost, "/api/control", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != tc.code {
			t.Fatalf("%s: status=%d body=%s", tc.body, response.Code, response.Body.String())
		}
		if tc.code != 200 && s.State() != before {
			t.Fatalf("invalid request mutated state: %s", tc.body)
		}
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	var state State
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || state.Weight != "1.56" || !strings.HasPrefix(state.Port, "/dev/pts/") {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{http.MethodGet, "/", 200}, {http.MethodGet, "/missing", 404},
		{http.MethodGet, "/api/control", 405}, {http.MethodPost, "/api/control", 415},
	} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.code {
			t.Fatalf("%s %s: %d", tc.method, tc.path, response.Code)
		}
	}
}

func TestUnreadPTYDoesNotBlockControlsAndDropsDisconnectedFrames(t *testing.T) {
	s := testSimulator(t)
	fd, err := unix.Open(s.State().Port, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
	}()
	// Fill the kernel buffer with a reader which never consumes. emit must
	// remain nonblocking so HTTP controls and shutdown remain usable.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 4000 {
			s.emit(time.Now())
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("unread PTY blocked sender")
	}
	if got := s.State(); got.FramesSent == 0 || got.FramesSkipped == 0 {
		t.Fatalf("backpressure was not exercised: %+v", got)
	}
	if err := s.Control(Command{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	_ = unix.Close(fd)
	fd = -1
	s.emit(time.Now())
	if s.State().ReaderPresent {
		t.Fatal("closed reader remains present")
	}
	// On reopen, old unread measurements must have been flushed.
	fd, err = unix.Open(s.State().Port, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	if n, err := unix.Read(fd, buf); n > 0 || err != unix.EAGAIN {
		t.Fatalf("old frames survived reader disconnect: n=%d err=%v", n, err)
	}
}

func TestPTYAgainstProductionSerialMonitor(t *testing.T) {
	s := testSimulator(t)
	ctx, cancel := context.WithCancel(context.Background())
	simDone := make(chan struct{})
	go func() { defer close(simDone); s.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-simDone })
	// No reader: emitted data must not queue up and later become "fresh".
	waitFor(t, "skipped frames without reader", func() bool { return s.State().FramesSkipped >= 2 })
	if s.State().FramesSent != 0 {
		t.Fatal("frames queued without a reader")
	}
	control := func(action, weight string) {
		t.Helper()
		if err := s.Control(Command{Action: action, Weight: weight}); err != nil {
			t.Fatal(err)
		}
	}
	control("pause", "")
	m, err := agentscale.New(agentscale.Config{Enabled: true, Port: s.State().Port, Baud: 9600, Protocol: "keli-d12-tf05", Unit: agentscale.UnitTonne, Stale: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	monitorDone := make(chan struct{})
	go func() { defer close(monitorDone); m.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-monitorDone })
	waitFor(t, "production serial opened", func() bool { return m.Snapshot(time.Now()).Connected })
	// The real monitor has opened, but nothing was sent since it opened.
	time.Sleep(250 * time.Millisecond)
	if got := m.Snapshot(time.Now()); got.DeviceOK || got.Stable || got.FramesValid != 0 {
		t.Fatalf("queued frame revived on open: %+v", got)
	}
	control("resume", "")
	waitFor(t, "stable zero", func() bool {
		got := m.Snapshot(time.Now())
		return got.DeviceOK && got.Stable && got.WeightMilliTonnes == 0
	})
	control("set_weight", "1.56")
	waitFor(t, "nonzero weight", func() bool {
		got := m.Snapshot(time.Now())
		return got.DeviceOK && got.Stable && got.WeightMilliTonnes == 1560
	})
	control("unstable", "")
	waitFor(t, "dynamic same weight", func() bool {
		got := m.Snapshot(time.Now())
		return got.DeviceOK && !got.Stable && got.WeightMilliTonnes == 1560
	})
	control("stable", "")
	waitFor(t, "stable again", func() bool { return m.Snapshot(time.Now()).Stable })
	control("bad_once", "")
	waitFor(t, "one invalid frame", func() bool { return m.Snapshot(time.Now()).FramesInvalid >= 1 })
	if got := m.Snapshot(time.Now()); !got.DeviceOK {
		t.Fatalf("single bad frame faulted: %+v", got)
	}
	waitFor(t, "single-frame recovery", func() bool { return m.Snapshot(time.Now()).ChecksumFailures == 0 })
	control("bad_start", "")
	waitFor(t, "sustained checksum fault", func() bool {
		got := m.Snapshot(time.Now())
		return !got.DeviceOK && !got.Stable && got.ChecksumFailures >= 3
	})
	control("bad_stop", "")
	waitFor(t, "checksum recovery", func() bool { return m.Snapshot(time.Now()).Stable })
	control("pause", "")
	waitFor(t, "stale while paused", func() bool {
		got := m.Snapshot(time.Now())
		return got.Stale && !got.DeviceOK && !got.Stable
	})
	control("resume", "")
	waitFor(t, "resume preserved weight", func() bool {
		got := m.Snapshot(time.Now())
		return got.DeviceOK && got.Stable && got.WeightMilliTonnes == 1560
	})
	control("down", "")
	waitFor(t, "down to zero", func() bool {
		got := m.Snapshot(time.Now())
		return got.DeviceOK && got.Stable && got.WeightMilliTonnes == 0
	})
	cancel()
	select {
	case <-monitorDone:
	case <-time.After(2 * time.Second):
		t.Fatal("production monitor did not stop")
	}
	select {
	case <-simDone:
	case <-time.After(time.Second):
		t.Fatal("simulator did not stop")
	}
	path := s.State().Port
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("PTY survives close: %v", err)
	}
	restarted := testSimulator(t)
	if got := restarted.State(); got.Weight != "0" || got.Dynamic || got.Paused || got.ContinuousBad || got.FramesSent != 0 {
		t.Fatalf("restart restored old state: %+v", got)
	}
}

func waitFor(t *testing.T, name string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", name)
}
