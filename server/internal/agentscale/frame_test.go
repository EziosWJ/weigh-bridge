package agentscale

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

func makeFrame(a, b, c byte, weight, tare string) []byte {
	raw := make([]byte, FrameLength)
	raw[0] = stx
	raw[1], raw[2], raw[3] = a, b, c
	copy(raw[4:10], weight)
	copy(raw[10:16], tare)
	raw[16] = cr
	raw[17] = byte(sum(raw[:16]) & 0xff)
	return raw
}

func decodeHex(t *testing.T, text string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(text, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRealSiteCaptureParsesCompletely(t *testing.T) {
	data, err := os.ReadFile("testdata/d12-tf05-20261022.bin")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 15624 {
		t.Fatalf("capture size %d", len(data))
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != "911c2231b35436751d7258ceda2fe0bcb2d9e684101baf06d42337a4b5f4eff5" {
		t.Fatalf("capture digest %s", got)
	}
	var frames []Frame
	var invalid []error
	var stream Stream
	stream.Feed(data, func(f Frame) { frames = append(frames, f) }, func(err error) { invalid = append(invalid, err) })
	if len(frames) != 868 || len(invalid) != 0 {
		t.Fatalf("frames=%d invalid=%d discarded=%d", len(frames), len(invalid), stream.Discarded())
	}
	if stream.Discarded() != 0 {
		t.Fatalf("discarded=%d", stream.Discarded())
	}
	stable, dynamic, nonzero := 0, 0, 0
	seen := map[int64]bool{}
	for _, f := range frames {
		if f.Stable {
			stable++
		} else {
			dynamic++
		}
		if f.RawWeight > 0 {
			nonzero++
		}
		seen[f.RawWeight] = true
		if f.DecimalPosition != 5 || f.WeightType != "gross" || f.UnitFlag != "kg" || f.Anomaly || f.WeightNegative {
			t.Fatalf("unexpected status: %+v", f)
		}
		if _, err := f.MilliTonnes(UnitTonne); err != nil {
			t.Fatalf("tonne conversion: %v", err)
		}
	}
	if stable != 787 || dynamic != 81 || nonzero != 409 || len(seen) != 52 {
		t.Fatalf("stable=%d dynamic=%d nonzero=%d distinct=%d", stable, dynamic, nonzero, len(seen))
	}
	if frames[190].RawWeight != 1560 || !frames[190].Stable || frames[190].WeightSource != "1.560" {
		t.Fatalf("frame 190: %+v", frames[190])
	}
	if frames[186].RawWeight != 1560 || !frames[186].Dynamic || frames[186].WeightSource != "1.560" {
		t.Fatalf("frame 186: %+v", frames[186])
	}
	if frames[171].RawWeight != 10 || frames[171].WeightSource != "0.010" {
		t.Fatalf("first nonzero: %+v", frames[171])
	}
}

func TestByteStreamDoesNotDependOnReadBoundaries(t *testing.T) {
	data, err := os.ReadFile("testdata/d12-tf05-20261022.bin")
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []int{1, 2, 3, 7, 8, 17, 18, 19, 257} {
		var stream Stream
		count := 0
		for i := 0; i < len(data); i += chunk {
			end := i + chunk
			if end > len(data) {
				end = len(data)
			}
			stream.Feed(data[i:end], func(Frame) { count++ }, func(error) { t.Errorf("unexpected invalid frame at chunk %d", chunk) })
		}
		if count != 868 {
			t.Fatalf("chunk %d parsed %d frames", chunk, count)
		}
	}
	// The site capture actually arrived as 8+8+2 byte reads. Exercise that
	// exact acknowledgement boundary over a known frame.
	frame := decodeHex(t, "02 2d 30 20 30 30 31 35 36 30 30 30 30 30 30 30 0d cb")
	var stream Stream
	got := 0
	stream.Feed(frame[:8], func(Frame) { got++ }, func(error) { t.Fatal("partial frame reported invalid") })
	if got != 0 {
		t.Fatal("partial frame emitted early")
	}
	stream.Feed(frame[8:16], func(Frame) { got++ }, func(error) { t.Fatal("partial frame reported invalid") })
	stream.Feed(frame[16:], func(Frame) { got++ }, func(error) { t.Fatal("complete frame rejected") })
	if got != 1 {
		t.Fatalf("split frame emitted %d", got)
	}
}

func TestGarbageAndChecksumControlByteAreHandledByFixedLength(t *testing.T) {
	frame := makeFrame(0x2d, 0x30, 0x20, "000000", "000000")
	// Adjust C so CKS is CR (0x0d), proving the trailing control byte is not a
	// frame separator.
	frame[3] = 0x20 + 0x4e
	frame[17] = byte(sum(frame[:16]) & 0xff)
	if frame[17] != cr {
		t.Fatalf("crafted checksum 0x%02x", frame[17])
	}
	input := append([]byte{0x00, 0xff, 0x02, 0x41}, frame...)
	input = append(input, frame...)
	var stream Stream
	var frames []Frame
	var invalid int
	stream.Feed(input, func(f Frame) { frames = append(frames, f) }, func(error) { invalid++ })
	if len(frames) != 2 || invalid == 0 {
		t.Fatalf("frames=%d invalid=%d discarded=%d", len(frames), invalid, stream.Discarded())
	}
}

func TestAnySingleByteCorruptionIsRejected(t *testing.T) {
	original := decodeHex(t, "02 2d 30 20 30 30 31 35 36 30 30 30 30 30 30 30 0d cb")
	if _, err := ParseFrame(original); err != nil {
		t.Fatalf("original rejected: %v", err)
	}
	for i := 0; i < FrameLength; i++ {
		corrupt := append([]byte(nil), original...)
		corrupt[i] ^= 0x01
		if _, err := ParseFrame(corrupt); err == nil {
			t.Fatalf("byte %d corruption accepted", i)
		}
	}
}

func TestStructureAndChecksumErrorsAreDistinct(t *testing.T) {
	frame := decodeHex(t, "02 2d 30 20 30 30 31 35 36 30 30 30 30 30 30 30 0d cb")
	badCR := append([]byte(nil), frame...)
	badCR[16] = 0x0a
	badCR[17] = byte(sum(badCR[:16]) & 0xff)
	if _, err := ParseFrame(badCR); !errors.Is(err, ErrFrameStructure) {
		t.Fatalf("bad CR error %v", err)
	}
	badDigit := append([]byte(nil), frame...)
	badDigit[4] = 'x'
	badDigit[17] = byte(sum(badDigit[:16]) & 0xff)
	if _, err := ParseFrame(badDigit); !errors.Is(err, ErrFrameStructure) {
		t.Fatalf("bad digit error %v", err)
	}
	badCKS := append([]byte(nil), frame...)
	badCKS[17] ^= 0xff
	if _, err := ParseFrame(badCKS); !errors.Is(err, ErrChecksum) {
		t.Fatalf("bad checksum error %v", err)
	}
	if _, err := ParseFrame(frame[:17]); !errors.Is(err, ErrFrameIncomplete) {
		t.Fatalf("short frame error %v", err)
	}
}

func TestAllDecimalPositionsAndExactConversion(t *testing.T) {
	for d := 0; d <= 7; d++ {
		frame, err := ParseFrame(makeFrame(byte(0x28|d), 0x30, 0x20, "123456", "000000"))
		if err != nil {
			t.Fatal(err)
		}
		if frame.DecimalPosition != d {
			t.Fatalf("decimal %d parsed as %d", d, frame.DecimalPosition)
		}
		exp := 5 - d
		want, wantErr := scaleExact(123456, exp)
		got, err := frame.MilliTonnes(UnitTonne)
		if wantErr != nil {
			if !errors.Is(err, ErrPrecision) {
				t.Fatalf("d=%d expected precision error, got %v", d, err)
			}
		} else if err != nil || got != want {
			t.Fatalf("d=%d got %d/%v want %d", d, got, err, want)
		}
	}
	for _, tc := range []struct {
		raw  string
		d    int
		unit Unit
		want int64
		err  error
	}{
		{"001550", 5, UnitTonne, 1550, nil},
		{"001550", 5, UnitKilogram, 0, ErrPrecision},
		{"001000", 5, UnitKilogram, 1, nil},
		{"123456", 5, UnitTonne, 123456, nil},
		{"000001", 7, UnitTonne, 0, ErrPrecision},
		{"100000", 7, UnitTonne, 1000, nil},
		{"000123", 3, UnitKilogram, 0, ErrPrecision},
		{"001230", 3, UnitKilogram, 123, nil},
	} {
		frame, err := ParseFrame(makeFrame(byte(0x28|tc.d), 0x30, 0x20, tc.raw, "000000"))
		if err != nil {
			t.Fatal(err)
		}
		got, err := frame.MilliTonnes(tc.unit)
		if tc.err != nil {
			if !errors.Is(err, tc.err) {
				t.Fatalf("%s d=%d %s: got %d/%v want %v", tc.raw, tc.d, tc.unit, got, err, tc.err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%s d=%d %s: got %d/%v want %d", tc.raw, tc.d, tc.unit, got, err, tc.want)
		}
	}
	if _, err := ParseFrame(makeFrame(0x2d, 0x30, 0x20, "000001", "000000")); err != nil {
		t.Fatal(err)
	}
	f, _ := ParseFrame(makeFrame(0x2d, 0x30, 0x20, "000001", "000000"))
	if _, err := f.MilliTonnes(Unit("lb")); !errors.Is(err, ErrUnsupportedUnit) {
		t.Fatalf("unsupported unit error %v", err)
	}
}

func TestStatusBitsAndSign(t *testing.T) {
	f, err := ParseFrame(makeFrame(0x2d, 0x3f, 0x38, "001550", "000020"))
	if err != nil {
		t.Fatal(err)
	}
	if f.WeightType != "net" || !f.WeightNegative || !f.Anomaly || !f.Dynamic || f.Stable {
		t.Fatalf("status bits: %+v", f)
	}
	if f.UnitFlag != "kg" || !f.PrintFlag || !f.ExtendedFlag {
		t.Fatalf("flags: %+v", f)
	}
	if f.WeightSource != "-1.550" || f.TareSource != "0.020" {
		t.Fatalf("source texts: %q %q", f.WeightSource, f.TareSource)
	}
	v, err := f.SignedMilliTonnes(UnitTonne)
	if err != nil || v != -1550 {
		t.Fatalf("signed value %d/%v", v, err)
	}
	clean, _ := ParseFrame(makeFrame(0x2d, 0x20, 0x20, "000000", "000000"))
	if clean.WeightType != "gross" || clean.WeightNegative || clean.Anomaly || clean.Dynamic || !clean.Stable || clean.UnitFlag != "none" {
		t.Fatalf("clean status: %+v", clean)
	}
}

func TestFormatMilliTonnesMatchesProtocolText(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{{0, "0"}, {1, "0.001"}, {1000, "1"}, {1550, "1.55"}, {1560, "1.56"}, {123456, "123.456"}, {-1550, "-1.55"}} {
		if got := FormatMilliTonnes(tc.in); got != tc.want {
			t.Fatalf("%d: %s != %s", tc.in, got, tc.want)
		}
	}
}
