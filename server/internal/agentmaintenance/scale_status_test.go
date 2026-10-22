package agentmaintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/agentscale"
)

func TestBuildAgentEnvironmentRoundTripsSerialAndPrint(t *testing.T) {
	dir := t.TempDir()
	c := Defaults()
	c.AgentKey = "secret"
	c.SerialEnabled = true
	c.SerialPort = "/dev/ttyS0"
	c.SerialBaud = 9600
	c.SerialProtocol = "keli-d12-tf05"
	c.SerialWeightUnit = "t"
	c.SerialStaleMS = 1000
	if err := BuildAgentEnvironment(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := agentprinter.ReadEnvironment(filepath.Join(dir, "agent.env"), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Fatalf("round trip changed config:\n got %+v\nwant %+v", got, c)
	}
}

func writeScaleSnapshot(t *testing.T, path string, at time.Time, s agentscale.Snapshot) {
	t.Helper()
	if err := agentscale.WriteStatus(path, s, at); err != nil {
		t.Fatal(err)
	}
}

func TestScaleViewNeverPresentsStoppedOrStaleDataAsCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scale-status.json")
	now := time.Now()
	if v := readScaleView(path, now, true); v.Known || v.State != "未知／尚无仪表状态" {
		t.Fatalf("missing file: %+v", v)
	}
	fresh := agentscale.Snapshot{
		Enabled: true, DeviceOK: true, Stable: true, Weight: "1.55", Unit: "t",
		SourceWeight: "1.550", SourceTare: "0.000", WeightType: "gross",
		Port: "/dev/ttyS0", Baud: 9600, Protocol: "keli-d12-tf05",
		LastValidAt: now, StaleMS: 1000, ChecksumStatus: "ok", UnitFlag: "kg",
	}
	writeScaleSnapshot(t, path, now, fresh)
	v := readScaleView(path, now, true)
	if !v.Known || !v.DeviceOK || !v.Stable || v.State != "稳定" || v.Weight != "1.55" || v.Fault != "" {
		t.Fatalf("fresh view: %+v", v)
	}

	stale := fresh
	stale.LastValidAt = now.Add(-2 * time.Second)
	writeScaleSnapshot(t, path, now, stale)
	v = readScaleView(path, now, true)
	if v.DeviceOK || v.Stable || !v.Stale || v.State != "无新鲜数据" {
		t.Fatalf("stale reading: %+v", v)
	}

	writeScaleSnapshot(t, path, now.Add(-10*time.Second), fresh)
	v = readScaleView(path, now, true)
	if v.DeviceOK || v.Stable || v.State != "Agent 状态已过期" {
		t.Fatalf("old file: %+v", v)
	}

	writeScaleSnapshot(t, path, now, fresh)
	v = readScaleView(path, now, false)
	if v.DeviceOK || v.Stable || v.State != "Agent 未运行" {
		t.Fatalf("stopped agent: %+v", v)
	}

	disabled := fresh
	disabled.Enabled, disabled.DeviceOK, disabled.Stable = false, false, false
	writeScaleSnapshot(t, path, now, disabled)
	v = readScaleView(path, now, true)
	if v.State != "未启用" || v.DeviceOK || v.Stable {
		t.Fatalf("disabled: %+v", v)
	}

	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if v = readScaleView(path, now, true); v.Known || v.State != "未知／仪表状态不可读取" {
		t.Fatalf("corrupt file: %+v", v)
	}
}

func TestStatusResponseIncludesScaleView(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, &fakeRunner{outputs: map[string]Result{
		"lpstat -r": {Output: "scheduler is running"},
		"lpstat -p": {Output: "printer Aisino-SK820-raw is idle. enabled"},
		"systemctl --user is-active weighbridge-agent-printer.service": {Output: "active"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	writeScaleSnapshot(t, filepath.Join(dir, "scale-status.json"), now, agentscale.Snapshot{
		Enabled: true, DeviceOK: true, Stable: true, Weight: "1.48", Unit: "t",
		Port: "/dev/ttyS0", Baud: 9600, Protocol: "keli-d12-tf05", LastValidAt: now, StaleMS: 1000,
	})
	w, _ := call(t, s.Handler(), "GET", "/api/status", "", nil)
	var body struct {
		Scale ScaleView `json:"scale"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Scale.State != "稳定" || body.Scale.Weight != "1.48" {
		t.Fatalf("status scale: %s", w.Body)
	}
}
