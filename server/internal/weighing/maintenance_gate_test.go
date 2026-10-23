package weighing

import (
	"context"
	"testing"
	"time"
)

func TestMaintenanceDrainsWorkAndRejectsNewCommands(t *testing.T) {
	s := NewService(nil, nil)
	finish, err := s.maintenance.enter()
	if err != nil {
		t.Fatal(err)
	}
	applied := make(chan func(), 1)
	go func() {
		release, e := s.BeginMaintenance(context.Background())
		if e != nil {
			t.Error(e)
			return
		}
		applied <- release
	}()
	deadline := time.Now().Add(time.Second)
	for {
		s.maintenance.mu.Lock()
		paused := s.maintenance.paused
		s.maintenance.mu.Unlock()
		if paused {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("maintenance did not pause")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err = s.maintenance.enter(); err == nil {
		t.Fatal("new work admitted during maintenance")
	}
	select {
	case <-applied:
		t.Fatal("maintenance passed admitted work")
	default:
	}
	finish()
	select {
	case release := <-applied:
		release()
	case <-time.After(time.Second):
		t.Fatal("did not drain")
	}
	finish, err = s.maintenance.enter()
	if err != nil {
		t.Fatal("work did not resume")
	}
	finish()
}
func TestCancelledMaintenanceDoesNotLeaveScalePaused(t *testing.T) {
	s := NewService(nil, nil)
	finish, _ := s.maintenance.enter()
	defer finish()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.BeginMaintenance(ctx); err == nil {
		t.Fatal("cancelled drain succeeded")
	}
	release, err := s.maintenance.enter()
	if err != nil {
		t.Fatal("cancelled maintenance kept gate closed")
	}
	release()
}
