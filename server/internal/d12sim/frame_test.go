package d12sim

import (
	"bytes"
	"errors"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/agentscale"
)

func TestFrameMatchesCapturedSiteBytes(t *testing.T) {
	// Independent ground truth: nonzero stable/dynamic site samples, not a
	// checksum calculated by the simulator or the production parser.
	stable := []byte{0x02, 0x2d, 0x30, 0x20, 0x30, 0x30, 0x31, 0x35, 0x36, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x0d, 0xcb}
	dynamic := append([]byte(nil), stable...)
	dynamic[2], dynamic[17] = 0x38, 0xd3
	for _, tc := range []struct {
		dynamic bool
		want    []byte
	}{{false, stable}, {true, dynamic}} {
		got := encodeFrame(1560, tc.dynamic, false)
		if !bytes.Equal(got, tc.want) {
			t.Fatalf("frame = % x, want % x", got, tc.want)
		}
		bad := encodeFrame(1560, tc.dynamic, true)
		if !bytes.Equal(bad[:17], got[:17]) || bad[17] == got[17] {
			t.Fatalf("bad frame must change only checksum: % x", bad)
		}
		if _, err := agentscale.ParseFrame(bad); !errors.Is(err, agentscale.ErrChecksum) {
			t.Fatalf("bad frame parser error = %v", err)
		}
	}
}

func TestExactWeightRangeAndEncoding(t *testing.T) {
	for _, tc := range []struct {
		input string
		milli int64
		text  string
	}{{"0", 0, "0"}, {"0.001", 1, "0.001"}, {"1.550", 1550, "1.55"}, {"30", 30000, "30"}, {"999.999", 999999, "999.999"}} {
		milli, err := parseWeight(tc.input)
		if err != nil || milli != tc.milli || formatWeight(milli) != tc.text {
			t.Fatalf("%q: milli=%d text=%q err=%v", tc.input, milli, formatWeight(milli), err)
		}
		frame, err := agentscale.ParseFrame(encodeFrame(milli, false, false))
		if err != nil {
			t.Fatal(err)
		}
		got, err := frame.MilliTonnes(agentscale.UnitTonne)
		if err != nil || got != milli || !frame.Stable || frame.RawTare != 0 {
			t.Fatalf("%q round trip: frame=%+v milli=%d err=%v", tc.input, frame, got, err)
		}
	}
	for _, text := range []string{"", "-1", "+1", " 1", "1 ", "1e2", "NaN", "1000", "0.0001", "1.", ".1", "1.2.3", "1.0011", "９"} {
		if _, err := parseWeight(text); err == nil {
			t.Fatalf("invalid weight accepted: %q", text)
		}
	}
}
