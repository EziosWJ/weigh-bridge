package agentprinter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sheet() *Sheet {
	return &Sheet{Template: "weighing-compact-v1", Width: 210, Height: 99, Title: "酒厂过磅单", Rows: [][]Field{{{Label: "单号", Value: "WB123"}, {Label: "净重（吨）", Value: "5.25"}}, {{Label: "物料", Value: "豌豆"}}, {{Label: "毛重", Value: "50"}}, {{Label: "时间", Value: "2026-10-19"}}, {{Label: "签字", Value: "___"}}}}
}
func TestPrepareChineseAndNoControlInjection(t *testing.T) {
	s := sheet()
	s.Title += "\x1b@\f"
	data, err := Prepare(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Sheet
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Rows[0][1].Value != "5.25" || strings.ContainsAny(decoded.Title, "\x1b\f") {
		t.Fatalf("unexpected content %s", data)
	}
	if s.Title == decoded.Title {
		t.Fatal("original snapshot was mutated")
	}
}
func TestOversizeRefused(t *testing.T) {
	s := sheet()
	s.Rows[0][0].Value = strings.Repeat("长", 201)
	if _, err := Prepare(s); err == nil {
		t.Fatal("expected overflow rejection")
	}
}
func TestSubmitDoesNotRepeatAcrossCallsOrFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "uncertain"}[failed], func(t *testing.T) {
			dir := t.TempDir()
			bin := t.TempDir()
			marker := filepath.Join(bin, "calls")
			script := "#!/bin/sh\ncat >/dev/null\necho called >> '" + marker + "'\necho request-id-123\n"
			if failed {
				script += "exit 1\n"
			}
			if err := os.WriteFile(filepath.Join(bin, "lp"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			req := Request{AttemptID: 12}
			req.Ticket.PrintSheet = sheet()
			_, err := Submit(context.Background(), dir, "test", req, []byte("rendered table"))
			if (err != nil) != failed {
				t.Fatalf("unexpected first outcome %v", err)
			}
			if _, err := Submit(context.Background(), dir, "test", req, []byte("rendered table")); err == nil {
				t.Fatal("duplicate accepted")
			}
			calls, err := os.ReadFile(marker)
			if err != nil || string(calls) != "called\n" {
				t.Fatalf("calls %q %v", calls, err)
			}
		})
	}
}

func TestIdentityDirectoryBindsLegacyAttemptsAndIsolatesSwitches(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "attempt-7.json")
	if err := os.WriteFile(legacy, []byte("reserved"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := IdentityDirectory(root, "http://server.example/", "agent-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if first != root {
		t.Fatalf("legacy attempts not bound to first identity: %s", first)
	}
	same, err := IdentityDirectory(root, "http://server.example", "agent-a", 1)
	if err != nil || same != root {
		t.Fatalf("equivalent URL split identity: %s %v", same, err)
	}
	other, err := IdentityDirectory(root, "https://server.example", "agent-a", 1)
	if err != nil || other == root {
		t.Fatalf("different server not isolated: %s %v", other, err)
	}
	back, err := IdentityDirectory(root, "http://server.example", "agent-a", 1)
	if err != nil || back != root {
		t.Fatalf("switch back lost original ledger: %s %v", back, err)
	}
	otherScale, err := IdentityDirectory(root, "http://server.example", "agent-a", 2)
	if err != nil || otherScale == root {
		t.Fatalf("scale identity not isolated: %s %v", otherScale, err)
	}
	equivalent, err := IdentityDirectory(root, "http://SERVER.example:80/", "agent-a", 1)
	if err != nil || equivalent != root {
		t.Fatalf("URL spelling bypassed legacy ledger: %s %v", equivalent, err)
	}
	if _, err := IdentityDirectory(root, "http://server.example/?new=1", "agent-a", 1); err == nil {
		t.Fatal("query parameters created a new print identity")
	}
}

func TestPrepareContinuousPaperAndLegacyCompatibility(t *testing.T) {
	s := sheet()
	s.Template, s.Width, s.Height = "weighing-compact-v2", 241, 93
	s.Rows = append([][]Field{{{Label: "单号", Value: "GB-NEW-9a6735f61d3b803e078773473802c348"}}}, s.Rows...)
	if _, err := Prepare(s); err != nil {
		t.Fatal(err)
	}
	s.Height = 297
	if _, err := Prepare(s); err == nil {
		t.Fatal("A4 height accepted for one continuous-paper section")
	}
	if _, err := Prepare(sheet()); err != nil {
		t.Fatalf("legacy sheet no longer accepted: %v", err)
	}
}
