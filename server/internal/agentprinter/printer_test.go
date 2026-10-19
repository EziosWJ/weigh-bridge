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
