// Package agentscale reads the Keli D12 tF=05 continuous serial output.
//
// The frame layout and checksum below were verified against the site capture
// on 2026-10-22. Only that captured firmware/profile is claimed: the checksum
// is the observed sum(frame[0:16]) & 0xff, not the Toledostyle complement
// algorithm. The display unit is configured explicitly because the site's
// status bit B4 says "kg" while the instrument actually displays tonnes.
package agentscale

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	// FrameLength is the fixed tF=05 frame size in bytes.
	FrameLength = 18
	stx         = 0x02
	cr          = 0x0d
)

// Unit is the engineering unit in which the instrument's numeric value is
// interpreted. It is site configuration, never inferred from the frame alone.
type Unit string

const (
	UnitTonne    Unit = "t"
	UnitKilogram Unit = "kg"
)

var (
	ErrFrameIncomplete = errors.New("incomplete frame")
	ErrFrameStructure  = errors.New("invalid frame structure")
	ErrChecksum        = errors.New("checksum mismatch")
	ErrPrecision       = errors.New("weight precision is not representable in 0.001 t")
	ErrUnsupportedUnit = errors.New("unsupported source unit")
	ErrWeightOverflow  = errors.New("weight is too large")
)

// Frame is one structurally valid, checksum-verified D12 tF=05 frame.
// The raw bytes and every status bit are retained for diagnostics.
type Frame struct {
	Raw             [FrameLength]byte
	StatusA         byte
	StatusB         byte
	StatusC         byte
	DecimalPosition int
	ScaleFactor     int
	RawWeight       int64
	RawTare         int64
	WeightNegative  bool
	WeightType      string // "gross" or "net"
	Anomaly         bool   // overload or below zero
	Dynamic         bool   // instrument says the value is changing
	Stable          bool   // instrument non-dynamic bit
	UnitFlag        string // documented meaning of B bit 4: "kg" or "none"
	PrintFlag       bool
	ExtendedFlag    bool
	WeightSource    string // signed source-unit decimal, e.g. "-1.550"
	TareSource      string // source-unit decimal
}

// ParseFrame validates the fixed tF=05 structure, checksum and numeric fields.
// It does not apply a configured unit or business availability rules.
func ParseFrame(raw []byte) (Frame, error) {
	var f Frame
	if len(raw) < FrameLength {
		return f, ErrFrameIncomplete
	}
	raw = raw[:FrameLength]
	copy(f.Raw[:], raw)
	if raw[0] != stx || raw[16] != cr {
		return f, fmt.Errorf("%w: missing STX or CR", ErrFrameStructure)
	}
	if byte(sum(raw[:16])&0xff) != raw[17] {
		return f, fmt.Errorf("%w: got 0x%02x, want 0x%02x", ErrChecksum, raw[17], byte(sum(raw[:16])&0xff))
	}
	f.StatusA, f.StatusB, f.StatusC = raw[1], raw[2], raw[3]
	weight, err := asciiNumber(raw[4:10])
	if err != nil {
		return f, fmt.Errorf("%w: display weight: %v", ErrFrameStructure, err)
	}
	tare, err := asciiNumber(raw[10:16])
	if err != nil {
		return f, fmt.Errorf("%w: tare: %v", ErrFrameStructure, err)
	}
	f.DecimalPosition = int(f.StatusA & 0x07)
	f.ScaleFactor = 1 << ((f.StatusA >> 3) & 0x03)
	f.RawWeight, f.RawTare = weight, tare
	f.WeightType = "gross"
	if f.StatusB&0x01 != 0 {
		f.WeightType = "net"
	}
	f.WeightNegative = f.StatusB&0x02 != 0
	f.Anomaly = f.StatusB&0x04 != 0
	f.Dynamic = f.StatusB&0x08 != 0
	f.Stable = !f.Dynamic
	f.UnitFlag = "none"
	if f.StatusB&0x10 != 0 {
		f.UnitFlag = "kg"
	}
	f.PrintFlag = f.StatusC&0x08 != 0
	f.ExtendedFlag = f.StatusC&0x10 != 0
	f.WeightSource = scaledText(weight, f.DecimalPosition)
	if f.WeightNegative {
		f.WeightSource = "-" + f.WeightSource
	}
	f.TareSource = scaledText(tare, f.DecimalPosition)
	return f, nil
}

func sum(b []byte) int {
	total := 0
	for _, v := range b {
		total += int(v)
	}
	return total
}

func asciiNumber(b []byte) (int64, error) {
	var n int64
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, errors.New("expected six ASCII digits")
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// scaledText renders raw * 10^(2-decimalPosition) exactly in the source unit.
// Leading zeros in the integer part are removed; the instrument's six-digit
// field width is not part of the value.
func scaledText(raw int64, decimalPosition int) string {
	digits := strconv.FormatInt(raw, 10)
	for len(digits) < 6 {
		digits = "0" + digits
	}
	exp := 2 - decimalPosition
	if exp >= 0 {
		return trimInteger(digits + strings.Repeat("0", exp))
	}
	cut := -exp
	var whole, fraction string
	if len(digits) <= cut {
		whole, fraction = "0", strings.Repeat("0", cut-len(digits))+digits
	} else {
		whole, fraction = digits[:len(digits)-cut], digits[len(digits)-cut:]
	}
	return trimInteger(whole) + "." + fraction
}

func trimInteger(v string) string {
	v = strings.TrimLeft(v, "0")
	if v == "" {
		return "0"
	}
	return v
}

// MilliTonnes converts the displayed weight to 0.001 t using exact integer
// arithmetic. It returns ErrPrecision instead of rounding or truncating when
// the instrument resolution cannot be represented losslessly.
func (f Frame) MilliTonnes(unit Unit) (int64, error) {
	exp, err := milliTonneExponent(f.DecimalPosition, unit)
	if err != nil {
		return 0, err
	}
	return scaleExact(f.RawWeight, exp)
}

// SignedMilliTonnes applies the frame's sign bit to the displayed weight.
func (f Frame) SignedMilliTonnes(unit Unit) (int64, error) {
	v, err := f.MilliTonnes(unit)
	if err != nil {
		return 0, err
	}
	if f.WeightNegative {
		v = -v
	}
	return v, nil
}

// TareMilliTonnes converts the tare field. The frame has no separate tare sign.
func (f Frame) TareMilliTonnes(unit Unit) (int64, error) {
	exp, err := milliTonneExponent(f.DecimalPosition, unit)
	if err != nil {
		return 0, err
	}
	return scaleExact(f.RawTare, exp)
}

func milliTonneExponent(decimalPosition int, unit Unit) (int, error) {
	switch unit {
	case UnitTonne:
		return 5 - decimalPosition, nil
	case UnitKilogram:
		return 2 - decimalPosition, nil
	default:
		return 0, ErrUnsupportedUnit
	}
}

func scaleExact(raw int64, exp int) (int64, error) {
	if raw < 0 {
		return 0, ErrPrecision
	}
	if exp >= 0 {
		if exp > 18 {
			return 0, ErrWeightOverflow
		}
		factor := pow10(exp)
		if raw > math.MaxInt64/factor {
			return 0, ErrWeightOverflow
		}
		return raw * factor, nil
	}
	divisor := pow10(-exp)
	if raw%divisor != 0 {
		return 0, ErrPrecision
	}
	return raw / divisor, nil
}

func pow10(n int) int64 {
	v := int64(1)
	for i := 0; i < n; i++ {
		v *= 10
	}
	return v
}

// FormatMilliTonnes renders 0.001 t as the protocol's trimmed tonne string.
func FormatMilliTonnes(v int64) string {
	n := v
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	for len(digits) < 4 {
		digits = "0" + digits
	}
	whole, fraction := digits[:len(digits)-3], strings.TrimRight(digits[len(digits)-3:], "0")
	if fraction != "" {
		whole += "." + fraction
	}
	return sign + whole
}

// Hex returns the raw frame as space-separated lowercase hex bytes.
func (f Frame) Hex() string {
	parts := make([]string, len(f.Raw))
	for i, b := range f.Raw {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(parts, " ")
}
