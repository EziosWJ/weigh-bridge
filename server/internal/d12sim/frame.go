// Package d12sim simulates the captured site D12 tF=05 profile over a Linux PTY.
// It has no Agent identity, business connection, or camera/printing behavior.
package d12sim

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const maxWeight = 999999 // 999.999 t, six digits with three decimal places

func parseWeight(text string) (int64, error) {
	invalid := errors.New("重量须为 0～999.999 吨的十进制数，最多三位小数")
	parts := strings.Split(text, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts[0]) > 3 {
		return 0, invalid
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) == 0 || len(fraction) > 3 {
			return 0, invalid
		}
	}
	for _, c := range parts[0] + fraction {
		if c < '0' || c > '9' {
			return 0, invalid
		}
	}
	value, err := strconv.ParseInt(parts[0]+fraction+strings.Repeat("0", 3-len(fraction)), 10, 64)
	if err != nil || value > maxWeight {
		return 0, invalid
	}
	return value, nil
}

func formatWeight(value int64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%d.%03d", value/1000, value%1000), "0"), ".")
}

// encodeFrame independently constructs the site-verified profile. A=0x2d
// encodes three decimals; B4 remains set as captured, while the source unit
// must explicitly be configured as tonnes in the Agent. Tare is always zero.
func encodeFrame(weight int64, dynamic, badChecksum bool) []byte {
	frame := []byte(fmt.Sprintf("\x02\x2d\x30\x20%06d000000\r\x00", weight))
	if dynamic {
		frame[2] |= 0x08
	}
	for _, b := range frame[:16] {
		frame[17] += b
	}
	if badChecksum {
		frame[17] ^= 0x01
	}
	return frame
}
