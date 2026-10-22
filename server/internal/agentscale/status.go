package agentscale

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StatusFile is the local read-only handoff between the Agent (which owns the
// serial port) and the maintenance service (which must never open it).
type StatusFile struct {
	Version int       `json:"version"`
	At      time.Time `json:"at"`
	Snapshot
}

// WriteStatus atomically replaces path with the latest snapshot. Permission is
// 0600 because the raw frame and diagnostics are site evidence, not public data.
func WriteStatus(path string, s Snapshot, at time.Time) error {
	data, err := json.Marshal(StatusFile{Version: 1, At: at.UTC(), Snapshot: s})
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".scale-status-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace scale status: %w", err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
