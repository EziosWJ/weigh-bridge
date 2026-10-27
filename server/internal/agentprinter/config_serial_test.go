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

func TestOldEnvironmentKeepsPrintingWithoutSerialConfiguration(t *testing.T) {
	path := writeEnv(t, "WEIGHBRIDGE_SERVER_URL=http://server.example\nWEIGHBRIDGE_PRINT_QUEUE=Aisino-SK820-raw\nWEIGHBRIDGE_PRINT_WIDTH_MM=175\nWEIGHBRIDGE_SERIAL_ENABLED=true\nWEIGHBRIDGE_SERIAL_PROTOCOL=keli-d12-tf05\nWEIGHBRIDGE_SERIAL_WEIGHT_UNIT=kg\nWEIGHBRIDGE_SERIAL_STALE_MS=300\n")
	c, err := ReadEnvironment(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if c.SerialPort != "" || c.SerialBaud != 9600 {
		t.Fatalf("old serial switches must not configure a device: %+v", c)
	}
	if c.Queue != "Aisino-SK820-raw" || c.PrintWidthMM != 175 {
		t.Fatalf("print settings changed: %+v", c)
	}
}

func TestSerialEnvironmentRoundTripAndDigest(t *testing.T) {
	path := writeEnv(t, `WEIGHBRIDGE_SERVER_URL="https://server.example"
WEIGHBRIDGE_AGENT_ID="onsite"
WEIGHBRIDGE_SCALE_ID=7
WEIGHBRIDGE_PRINT_QUEUE="onsite-queue"
WEIGHBRIDGE_SERIAL_PORT="/dev/ttyS0"
WEIGHBRIDGE_SERIAL_BAUD=19200
WEIGHBRIDGE_SERIAL_PROTOCOL="keli-d12-tf05"
WEIGHBRIDGE_SERIAL_WEIGHT_UNIT="t"
WEIGHBRIDGE_SERIAL_STALE_MS=1000
`)
	c, err := ReadEnvironment(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if c.SerialPort != "/dev/ttyS0" || c.SerialBaud != 19200 {
		t.Fatalf("serial config: %+v", c)
	}
	if ConfigDigest(c) == ConfigDigest(Defaults()) {
		t.Fatal("serial port settings do not affect the running config digest")
	}
	// Retired protocol/unit/timeout keys must not change the digest either.
	plain, err := ReadEnvironment(writeEnv(t, "WEIGHBRIDGE_SERVER_URL=https://server.example\nWEIGHBRIDGE_AGENT_ID=onsite\nWEIGHBRIDGE_SCALE_ID=7\nWEIGHBRIDGE_PRINT_QUEUE=onsite-queue\nWEIGHBRIDGE_SERIAL_PORT=/dev/ttyS0\nWEIGHBRIDGE_SERIAL_BAUD=19200\n"), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if ConfigDigest(plain) != ConfigDigest(c) {
		t.Fatal("retired serial fields still influence the config digest")
	}
}

func TestSerialConfigurationValidation(t *testing.T) {
	bad := []func(*Config){
		func(c *Config) { c.SerialPort = "ttyS0" },
		func(c *Config) { c.SerialPort = "/dev/../etc/passwd" },
		func(c *Config) { c.SerialBaud = 1234 },
		func(c *Config) { c.AgentID = "bad id" },
		func(c *Config) { c.Queue = "bad queue" },
	}
	for i, mutate := range bad {
		c := Defaults()
		c.SerialPort = "/dev/ttyS0"
		mutate(&c)
		if err := ValidateConfig(c); err == nil {
			t.Fatalf("invalid serial config %d accepted: %+v", i, c)
		}
	}
	for _, c := range []Config{Defaults(), func() Config { v := Defaults(); v.SerialPort = "/dev/ttyS0"; v.SerialBaud = 9600; return v }()} {
		if err := ValidateConfig(c); err != nil {
			t.Fatalf("valid serial config rejected: %+v %v", c, err)
		}
	}
}

func TestSerialEnvironmentRejectsInvalidValues(t *testing.T) {
	base := "WEIGHBRIDGE_SERVER_URL=http://server.example\nWEIGHBRIDGE_PRINT_QUEUE=q\n"
	for _, line := range []string{
		"WEIGHBRIDGE_SERIAL_BAUD=abc",
	} {
		path := writeEnv(t, base+line+"\n")
		if _, err := ReadEnvironment(path, Defaults()); err == nil || !strings.Contains(err.Error(), "无效") {
			t.Fatalf("%s accepted: %v", line, err)
		}
	}
}

func TestNormalizeConfigKeepsLegacyPrintingWithoutASerialPort(t *testing.T) {
	legacy := Config{ServerURL: "http://server.example", AgentID: "agent", ScaleCode: "WB01", AgentKey: "key", Queue: "q", PrintWidthMM: 175, FormLengthMM: 99}
	n := NormalizeConfig(legacy)
	if n.SerialPort != "" || n.SerialBaud != 9600 {
		t.Fatalf("normalized legacy config: %+v", n)
	}
	if n.Queue != legacy.Queue || n.PrintWidthMM != legacy.PrintWidthMM || n.AgentKey != legacy.AgentKey {
		t.Fatalf("normalization changed print settings: %+v", n)
	}
	if err := ValidateConfig(n); err != nil {
		t.Fatal(err)
	}
}
