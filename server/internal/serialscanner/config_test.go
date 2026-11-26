package serialscanner

import (
	"bytes"
	"testing"
)

func TestDecodeDelimiter(t *testing.T) {
	cases := map[string][]byte{
		`\r\n`: {'\r', '\n'},
		`\n`:   {'\n'},
		`\r`:   {'\r'},
		`\t`:   {'\t'},
		`;`:    {';'},
		`\\`:   {'\\'},
	}
	for input, want := range cases {
		got, err := DecodeDelimiter(input)
		if err != nil {
			t.Fatalf("DecodeDelimiter(%q): %v", input, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("DecodeDelimiter(%q) = %v, want %v", input, got, want)
		}
	}
	for _, input := range []string{"", `\`, `\x`, `\n\n\n\n\n\n\n\n\n`} {
		if _, err := DecodeDelimiter(input); err == nil {
			t.Fatalf("DecodeDelimiter(%q) accepted an invalid value", input)
		}
	}
}

func TestValidateInput(t *testing.T) {
	valid := ConfigInput{
		Enabled: true, Device: "/dev/ttyUSB0", BaudRate: 9600, DataBits: 8, StopBits: 1,
		Parity: "none", Delimiter: `\r\n`, RetryCount: 2, RetryIntervalMS: 500,
	}
	if _, err := ValidateInput(valid); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	tests := map[string]func(*ConfigInput){
		"relative device":    func(in *ConfigInput) { in.Device = "ttyUSB0" },
		"traversal device":   func(in *ConfigInput) { in.Device = "/dev/../etc/passwd" },
		"bad baud":           func(in *ConfigInput) { in.BaudRate = 12345 },
		"bad data bits":      func(in *ConfigInput) { in.DataBits = 9 },
		"bad stop bits":      func(in *ConfigInput) { in.StopBits = 3 },
		"bad parity":         func(in *ConfigInput) { in.Parity = "mark" },
		"empty delimiter":    func(in *ConfigInput) { in.Delimiter = `` },
		"bad retry count":    func(in *ConfigInput) { in.RetryCount = 11 },
		"bad retry interval": func(in *ConfigInput) { in.RetryIntervalMS = 50 },
	}
	for name, mutate := range tests {
		value := valid
		mutate(&value)
		if _, err := ValidateInput(value); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestTrimFrame(t *testing.T) {
	if got := TrimFrame("\r\nPREfc-handoff:abcSUF\r\n", "PRE", "SUF"); got != "fc-handoff:abc" {
		t.Fatalf("TrimFrame = %q", got)
	}
	if got := TrimFrame("  fc-handoff:abc  ", "", ""); got != "fc-handoff:abc" {
		t.Fatalf("TrimFrame = %q", got)
	}
}
