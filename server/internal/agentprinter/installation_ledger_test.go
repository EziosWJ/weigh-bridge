package agentprinter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const installationA = "00000000-0000-4000-8000-000000000001"
const installationB = "00000000-0000-4000-8000-000000000002"

func TestInstallationLedgerPreservesAttemptsAndIsolatesInstances(t *testing.T) {
	root := t.TempDir()
	first, err := InstallationDirectory(root, installationA)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	script := "#!/bin/sh\ncat >/dev/null\necho called >> '" + calls + "'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "lp"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	req := Request{AttemptID: 12}
	if _, err := Submit(context.Background(), first, "test", req, []byte("first")); err == nil {
		t.Fatal("expected unknown result")
	}
	// Restart, server address changes and business code edits all resolve the
	// same installation ID to the same durable physical-print reservation.
	same, err := InstallationDirectory(root, installationA)
	if err != nil || same != first {
		t.Fatalf("installation ledger changed: %q %v", same, err)
	}
	if _, err := Submit(context.Background(), same, "test", req, []byte("duplicate")); err == nil || !strings.Contains(err.Error(), "already reserved") {
		t.Fatalf("old attempt replayed: %v", err)
	}
	other, err := InstallationDirectory(root, installationB)
	if err != nil || other == first {
		t.Fatalf("different installation reused ledger=%q err=%v", other, err)
	}
	if _, err := Submit(context.Background(), other, "test", req, []byte("other")); err == nil || strings.Contains(err.Error(), "already reserved") {
		t.Fatalf("different installation incorrectly shared attempt: %v", err)
	}
	data, err := os.ReadFile(calls)
	if err != nil || string(data) != "called\ncalled\n" {
		t.Fatalf("unexpected physical submits=%q err=%v", data, err)
	}
}

func TestInstallationLedgerConcurrentBindingAndInvalidPaths(t *testing.T) {
	root := t.TempDir()
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for range 8 {
		wg.Go(func() {
			dir, err := InstallationDirectory(root, installationA)
			if err != nil {
				results <- "error:" + err.Error()
			} else {
				results <- dir
			}
		})
	}
	wg.Wait()
	close(results)
	expected := filepath.Join(root, "installation-"+installationA)
	for dir := range results {
		if dir != expected {
			t.Fatalf("inconsistent concurrent binding %q", dir)
		}
	}
	if _, err := InstallationDirectory(root, "../invalid"); err == nil {
		t.Fatal("invalid identifier accepted")
	}
	link := filepath.Join(root, "installation-"+installationB)
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallationDirectory(root, installationB); err == nil {
		t.Fatal("symlink ledger directory accepted")
	}
}

func TestInstallationLedgerLeavesPreviousFilesUntouched(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "attempt-9.json")
	if err := os.WriteFile(old, []byte("previous system"), 0600); err != nil {
		t.Fatal(err)
	}
	dir, err := InstallationDirectory(root, installationA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "attempt-9.json")); !os.IsNotExist(err) {
		t.Fatalf("old attempt imported into fresh installation: %v", err)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "previous system" {
		t.Fatalf("old file changed: %q %v", data, err)
	}
}
