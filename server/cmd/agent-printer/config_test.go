package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

func TestManagedPrinterUsesSharedFileWithoutEnvironmentOverrides(t *testing.T) {
	file := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(file, []byte("APP_WEIGHING__AGENT_KEY='file-key'\nWEIGHBRIDGE_SERVER_URL=\"https://server.example\"\nWEIGHBRIDGE_AGENT_ID=onsite\nWEIGHBRIDGE_SCALE_ID=7\nWEIGHBRIDGE_PRINT_QUEUE=onsite-queue\nWEIGHBRIDGE_PRINT_WIDTH_MM=170\n"), 0600); err != nil {
		t.Fatal(err)
	}
	getenv := func(string) string { return "stale-process-environment" }
	c, state, err := printerOptions([]string{"--env-file", file, "--state-dir", "existing-state"}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	want := agentprinter.Defaults()
	want.ServerURL, want.AgentID, want.ScaleID, want.Queue, want.AgentKey, want.PrintWidthMM = "https://server.example", "onsite", 7, "onsite-queue", "file-key", 170
	if c != want || state != "existing-state" {
		t.Fatalf("managed configuration diverged: %+v %s", c, state)
	}
	if _, _, err := printerOptions([]string{"--env-file", file, "--server", "http://different.example"}, getenv); err == nil {
		t.Fatal("mixed config sources accepted")
	}
	if _, _, err := printerOptions([]string{"--env-file", file + "-missing"}, getenv); err == nil {
		t.Fatal("missing config silently defaulted")
	}
}

func TestStandaloneInvalidNumericEnvironmentIsRejected(t *testing.T) {
	for _, key := range []string{"WEIGHBRIDGE_SCALE_ID", "WEIGHBRIDGE_PRINT_WIDTH_MM"} {
		t.Run(key, func(t *testing.T) {
			_, _, err := printerOptions(nil, func(name string) string {
				if name == key {
					return "invalid"
				}
				if name == "APP_WEIGHING__AGENT_KEY" {
					return "test-key"
				}
				return ""
			})
			if err == nil {
				t.Fatal("invalid config silently fell back to defaults")
			}
		})
	}
}

func TestManagedPrinterRejectsIncompleteIdentityInsteadOfUsingDefaults(t *testing.T) {
	file := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(file, []byte("APP_WEIGHING__AGENT_KEY=test-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := printerOptions([]string{"--env-file", file}, func(string) string { return "" }); err == nil {
		t.Fatal("incomplete identity silently defaulted")
	}
}
