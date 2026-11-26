package serialscanner

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	// OnlineWindow is how long a heartbeat keeps the Agent marked online.
	OnlineWindow = 15 * time.Second

	DefaultDevice          = "/dev/ttyUSB0"
	DefaultBaudRate        = 9600
	DefaultDelimiter       = `\r\n`
	DefaultRetryCount      = 2
	DefaultRetryIntervalMS = 500
)

// DefaultConfig mirrors the seeded singleton row. It is used when the seed has
// not run yet, so the page still renders a usable form.
func DefaultConfig() Config {
	return Config{
		Enabled: true, Device: DefaultDevice, BaudRate: DefaultBaudRate,
		DataBits: 8, StopBits: 1, Parity: "none", Delimiter: DefaultDelimiter,
		RetryCount: DefaultRetryCount, RetryIntervalMS: DefaultRetryIntervalMS, Version: 1,
	}
}

// DecodeDelimiter converts the operator-visible escaped delimiter (for example
// \r\n) into the bytes the Agent expects on the wire.
func DecodeDelimiter(value string) ([]byte, error) {
	if value == "" {
		return nil, errors.New("扫码结束符不能为空")
	}
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out = append(out, value[i])
			continue
		}
		i++
		if i >= len(value) {
			return nil, errors.New("扫码结束符转义不完整")
		}
		switch value[i] {
		case 'r':
			out = append(out, '\r')
		case 'n':
			out = append(out, '\n')
		case 't':
			out = append(out, '\t')
		case '\\':
			out = append(out, '\\')
		default:
			return nil, fmt.Errorf("扫码结束符不支持 \\%c 转义", value[i])
		}
	}
	if len(out) == 0 || len(out) > 8 {
		return nil, errors.New("扫码结束符解码后须为 1～8 字节")
	}
	return out, nil
}

// ValidateInput rejects unsafe or unsupported scanner parameters before they
// reach the Agent.
func ValidateInput(in ConfigInput) (ConfigInput, error) {
	in.Device = strings.TrimSpace(in.Device)
	in.Parity = strings.ToLower(strings.TrimSpace(in.Parity))
	in.Delimiter = strings.TrimSpace(in.Delimiter)
	in.TrimPrefix = strings.TrimSpace(in.TrimPrefix)
	in.TrimSuffix = strings.TrimSpace(in.TrimSuffix)

	if in.Device == "" {
		if in.Enabled {
			return in, errors.New("启用扫码枪时必须填写串口设备")
		}
	} else if !strings.HasPrefix(in.Device, "/dev/") || strings.Contains(in.Device, "..") || strings.ContainsAny(in.Device, "\r\n\x00") {
		return in, errors.New("串口设备须为 /dev 下的设备路径，例如 /dev/ttyUSB0")
	}
	switch in.BaudRate {
	case 1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200:
	default:
		return in, errors.New("扫码枪波特率无效")
	}
	if in.DataBits < 5 || in.DataBits > 8 {
		return in, errors.New("数据位须在 5～8 之间")
	}
	if in.StopBits != 1 && in.StopBits != 2 {
		return in, errors.New("停止位须为 1 或 2")
	}
	if in.Parity != "none" && in.Parity != "even" && in.Parity != "odd" {
		return in, errors.New("校验位须为 none、even 或 odd")
	}
	if _, err := DecodeDelimiter(in.Delimiter); err != nil {
		return in, err
	}
	if err := validateAffix("前置字符", in.TrimPrefix); err != nil {
		return in, err
	}
	if err := validateAffix("后置字符", in.TrimSuffix); err != nil {
		return in, err
	}
	if in.RetryCount < 0 || in.RetryCount > 10 {
		return in, errors.New("失败重试次数须在 0～10 之间")
	}
	if in.RetryIntervalMS < 100 || in.RetryIntervalMS > 10000 {
		return in, errors.New("失败重试间隔须在 100～10000 ms 之间")
	}
	return in, nil
}

func validateAffix(label, value string) error {
	if len(value) > 32 {
		return fmt.Errorf("%s不能超过 32 个字符", label)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s不能包含控制字符", label)
		}
	}
	return nil
}

// TrimFrame applies the configured prefix/suffix and strips surrounding
// whitespace from one delimiter-framed scanner frame.
func TrimFrame(value, prefix, suffix string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\r\n\x00")
	if prefix != "" {
		value = strings.TrimPrefix(value, prefix)
	}
	if suffix != "" {
		value = strings.TrimSuffix(value, suffix)
	}
	return strings.TrimSpace(value)
}
