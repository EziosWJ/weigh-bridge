package agentprinter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEnv(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOldEnvironmentKeepsPrintingAndAddsDisabledSerialDefaults(t *testing.T) {
	path := writeEnv(t, "APP_WEIGHING__AGENT_KEY=key\nWEIGHBRIDGE_SERVER_URL=http://server.example\nWEIGHBRIDGE_AGENT_ID=scale-agent-1\nWEIGHBRIDGE_SCALE_ID=1\nWEIGHBRIDGE_PRINT_QUEUE=Aisino-SK820-raw\nWEIGHBRIDGE_PRINT_WIDTH_MM=175\n")
	c, err := ReadEnvironment(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if c.SerialEnabled {
		t.Fatal("old environment enabled a serial device implicitly")
	}
	if c.SerialPort != "/dev/ttyS0" || c.SerialBaud != 9600 || c.SerialProtocol != "keli-d12-tf05" || c.SerialWeightUnit != "t" || c.SerialStaleMS != 1000 {
		t.Fatalf("serial defaults: %+v", c)
	}
	if c.Queue != "Aisino-SK820-raw" || c.PrintWidthMM != 175 || c.AgentKey != "key" {
		t.Fatalf("print settings changed: %+v", c)
	}
}

func TestSerialEnvironmentRoundTripAndDigest(t *testing.T) {
	path := writeEnv(t, `APP_WEIGHING__AGENT_KEY="key"
WEIGHBRIDGE_SERVER_URL="https://server.example"
WEIGHBRIDGE_AGENT_ID="onsite"
WEIGHBRIDGE_SCALE_ID=7
WEIGHBRIDGE_PRINT_QUEUE="onsite-queue"
WEIGHBRIDGE_SERIAL_ENABLED=true
WEIGHBRIDGE_SERIAL_PORT="/dev/ttyS0"
WEIGHBRIDGE_SERIAL_BAUD=9600
WEIGHBRIDGE_SERIAL_PROTOCOL="keli-d12-tf05"
WEIGHBRIDGE_SERIAL_WEIGHT_UNIT="t"
WEIGHBRIDGE_SERIAL_STALE_MS=1000
`)
	c, err := ReadEnvironment(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if !c.SerialEnabled || c.SerialPort != "/dev/ttyS0" || c.SerialBaud != 9600 || c.SerialProtocol != "keli-d12-tf05" || c.SerialWeightUnit != "t" || c.SerialStaleMS != 1000 {
		t.Fatalf("serial config: %+v", c)
	}
	if ConfigDigest(c) == ConfigDigest(Defaults()) {
		t.Fatal("serial settings do not affect the running config digest")
	}
}

func TestSerialConfigurationValidation(t *testing.T) {
	bad := []func(*Config){
		func(c *Config) { c.SerialPort = "ttyS0" },
		func(c *Config) { c.SerialPort = "/dev/../etc/passwd" },
		func(c *Config) { c.SerialBaud = 1234 },
		func(c *Config) { c.SerialProtocol = "toledo" },
		func(c *Config) { c.SerialWeightUnit = "lb" },
		func(c *Config) { c.SerialStaleMS = 199 },
		func(c *Config) { c.SerialStaleMS = 3000 },
	}
	for i, mutate := range bad {
		c := Defaults()
		c.SerialEnabled = true
		mutate(&c)
		if err := ValidateConfig(c); err == nil {
			t.Fatalf("invalid serial config %d accepted: %+v", i, c)
		}
	}
	c := Defaults()
	c.SerialEnabled = true
	if err := ValidateConfig(c); err != nil {
		t.Fatalf("valid default serial config rejected: %v", err)
	}
}

func TestSerialEnvironmentRejectsInvalidValues(t *testing.T) {
	base := "APP_WEIGHING__AGENT_KEY=key\nWEIGHBRIDGE_SERVER_URL=http://server.example\nWEIGHBRIDGE_AGENT_ID=a\nWEIGHBRIDGE_SCALE_ID=1\nWEIGHBRIDGE_PRINT_QUEUE=q\n"
	for _, line := range []string{
		"WEIGHBRIDGE_SERIAL_ENABLED=maybe",
		"WEIGHBRIDGE_SERIAL_BAUD=abc",
		"WEIGHBRIDGE_SERIAL_STALE_MS=abc",
	} {
		path := writeEnv(t, base+line+"\n")
		if _, err := ReadEnvironment(path, Defaults()); err == nil || !strings.Contains(err.Error(), "无效") {
			t.Fatalf("%s accepted: %v", line, err)
		}
	}
}

func TestNormalizeConfigKeepsLegacyPrintingDisabledSerial(t *testing.T) {
	legacy := Config{ServerURL: "http://server.example", AgentID: "agent", ScaleID: 1, AgentKey: "key", Queue: "q", PrintWidthMM: 175, FormLengthMM: 99}
	n := NormalizeConfig(legacy)
	if n.SerialEnabled || n.SerialPort != "/dev/ttyS0" || n.SerialBaud != 9600 || n.SerialProtocol != "keli-d12-tf05" || n.SerialWeightUnit != "t" || n.SerialStaleMS != 1000 {
		t.Fatalf("normalized legacy config: %+v", n)
	}
	if n.Queue != legacy.Queue || n.PrintWidthMM != legacy.PrintWidthMM || n.AgentKey != legacy.AgentKey {
		t.Fatalf("normalization changed print settings: %+v", n)
	}
	if err := ValidateConfig(n); err != nil {
		t.Fatal(err)
	}
}
