package agentscale

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func stableWeightFrame(weight string) []byte {
	return makeFrame(0x2d, 0x30, 0x20, weight, "000000")
}

func dynamicWeightFrame(weight string) []byte {
	return makeFrame(0x2d, 0x38, 0x20, weight, "000000")
}

func startMeasurementMonitor(t *testing.T) (*Monitor, *testClock, *pipeConn, context.CancelFunc) {
	t.Helper()
	conn := newPipeConn()
	opener := &scriptedOpener{conns: []io.ReadCloser{conn}}
	m, clock := newTestMonitor(t, testConfig(), opener.Open)
	ctx, cancel := context.WithCancel(context.Background())
	go m.Run(ctx)
	waitFor(t, "serial open", func() bool { return m.Snapshot(clock.Now()).Connected })
	return m, clock, conn, cancel
}

func feedAndWait(t *testing.T, m *Monitor, clock *testClock, conn *pipeConn, frames []byte, valid uint64) {
	t.Helper()
	conn.ch <- frames
	waitFor(t, "frames processed", func() bool {
		s := m.Snapshot(clock.Now())
		return s.FramesValid >= valid && s.DeviceOK
	})
}

func TestMeasurementRejectsSameBatchValueChangeAndRestore(t *testing.T) {
	m, clock, conn, cancel := startMeasurementMonitor(t)
	defer cancel()
	feedAndWait(t, m, clock, conn, stableWeightFrame("001000"), 1)
	first, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	// 1.000 -> 1.010 -> 1.000 in the same serial read batch.
	feedAndWait(t, m, clock, conn, append(dynamicWeightFrame("001010"), stableWeightFrame("001000")...), 3)
	if s := m.Snapshot(clock.Now()); s.WeightMilliTonnes != 1000 || !s.Stable {
		t.Fatalf("reading did not return to the original value: %+v", s)
	}
	if _, err := m.EndMeasurement(first.Generation, clock.Now()); !errors.Is(err, ErrMeasurementGenerationInvalid) && !errors.Is(err, ErrMeasurementWeightChanged) {
		t.Fatalf("same-batch change and restore must fail the window: %v", err)
	}
	if _, err := m.VerifyMeasurement(first.Generation, clock.Now()); err == nil {
		t.Fatal("invalid window became valid at confirmation")
	}
	// A fresh window on the restored reading is valid.
	second, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatalf("fresh window on stable reading rejected: %v", err)
	}
	if _, err := m.EndMeasurement(second.Generation, clock.Now()); err != nil {
		t.Fatalf("fresh window end: %v", err)
	}
	if _, err := m.VerifyMeasurement(second.Generation, clock.Now()); err != nil {
		t.Fatalf("fresh window verify: %v", err)
	}
}

func TestMeasurementRepeatedStableFramesStayValid(t *testing.T) {
	m, clock, conn, cancel := startMeasurementMonitor(t)
	defer cancel()
	feedAndWait(t, m, clock, conn, stableWeightFrame("001000"), 1)
	window, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Repeated identical valid stable frames must not advance the basis.
	feedAndWait(t, m, clock, conn, append(stableWeightFrame("001000"), stableWeightFrame("001000")...), 3)
	if _, err := m.EndMeasurement(window.Generation, clock.Now()); err != nil {
		t.Fatalf("identical stable frames invalidated the window: %v", err)
	}
	if _, err := m.VerifyMeasurement(window.Generation, clock.Now()); err != nil {
		t.Fatalf("identical stable frames invalidated confirmation: %v", err)
	}
}

func TestMeasurementEndAndVerifyLatchInstability(t *testing.T) {
	m, clock, conn, cancel := startMeasurementMonitor(t)
	defer cancel()
	feedAndWait(t, m, clock, conn, stableWeightFrame("001000"), 1)
	window, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	feedAndWait(t, m, clock, conn, append(dynamicWeightFrame("001010"), stableWeightFrame("001000")...), 3)
	if _, err := m.EndMeasurement(window.Generation, clock.Now()); err == nil {
		t.Fatal("instability during the window was accepted")
	}
}

func TestMeasurementVerificationRejectsPostWindowChangeAndRestore(t *testing.T) {
	m, clock, conn, cancel := startMeasurementMonitor(t)
	defer cancel()
	feedAndWait(t, m, clock, conn, stableWeightFrame("001000"), 1)
	window, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.EndMeasurement(window.Generation, clock.Now()); err != nil {
		t.Fatalf("end: %v", err)
	}
	// After the capture window ended, a transient change returns to the same
	// value before operator confirmation.
	feedAndWait(t, m, clock, conn, append(dynamicWeightFrame("001010"), stableWeightFrame("001000")...), 3)
	if _, err := m.VerifyMeasurement(window.Generation, clock.Now()); err == nil {
		t.Fatal("confirmation accepted a basis that changed after capture")
	}
}

func TestMeasurementRejectsStaleDisconnectAndReload(t *testing.T) {
	m, clock, conn, cancel := startMeasurementMonitor(t)
	defer cancel()
	feedAndWait(t, m, clock, conn, stableWeightFrame("001000"), 1)
	staleWindow, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(1500 * time.Millisecond)
	if _, err := m.EndMeasurement(staleWindow.Generation, clock.Now()); !errors.Is(err, ErrMeasurementStale) {
		t.Fatalf("stale basis accepted: %v", err)
	}

	feedAndWait(t, m, clock, conn, stableWeightFrame("001000"), 2)
	disconnectWindow, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close() // serial device disappears and reopens
	time.Sleep(50 * time.Millisecond)
	if _, err := m.EndMeasurement(disconnectWindow.Generation, clock.Now()); err == nil {
		t.Fatal("disconnect did not invalidate the window")
	}

	reloaded, err := New(testConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reloaded.EndMeasurement(disconnectWindow.Generation, time.Now()); !errors.Is(err, ErrMeasurementGenerationInvalid) {
		t.Fatalf("old generation survived a device reload: %v", err)
	}
}

func TestMeasurementCancelReleasesTheWindow(t *testing.T) {
	m, clock, conn, cancel := startMeasurementMonitor(t)
	defer cancel()
	feedAndWait(t, m, clock, conn, stableWeightFrame("001000"), 1)
	window, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CancelMeasurement(window.Generation); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EndMeasurement(window.Generation, clock.Now()); err == nil {
		t.Fatal("cancelled window was still verified")
	}
	if _, err := m.VerifyMeasurement(window.Generation, clock.Now()); err == nil {
		t.Fatal("cancelled window was still confirmable")
	}
}

func TestMeasurementInvalidationNotificationIsLatchedOnce(t *testing.T) {
	m, clock, conn, cancel := startMeasurementMonitor(t)
	defer cancel()
	feedAndWait(t, m, clock, conn, stableWeightFrame("001000"), 1)
	window, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if notifications := m.InvalidatedMeasurements(clock.Now()); len(notifications) != 0 {
		t.Fatalf("stable window was invalidated early: %+v", notifications)
	}
	feedAndWait(t, m, clock, conn, append(dynamicWeightFrame("001010"), stableWeightFrame("001000")...), 3)
	notifications := m.InvalidatedMeasurements(clock.Now())
	if len(notifications) != 1 || notifications[0].Generation != window.Generation || notifications[0].Reason == "" || notifications[0].At.IsZero() {
		t.Fatalf("missing invalidation notification: %+v", notifications)
	}
	if again := m.InvalidatedMeasurements(clock.Now()); len(again) != 0 {
		t.Fatalf("invalidation was reported twice: %+v", again)
	}
}

func TestNewMeasurementSupersedesAnEndedButUnconfirmedWindow(t *testing.T) {
	m, clock, conn, cancel := startMeasurementMonitor(t)
	defer cancel()
	feedAndWait(t, m, clock, conn, stableWeightFrame("001000"), 1)
	first, err := m.BeginMeasurement(clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.EndMeasurement(first.Generation, clock.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.BeginMeasurement(clock.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyMeasurement(first.Generation, clock.Now()); err == nil {
		t.Fatal("a new capture did not supersede the unconfirmed window")
	}
}
