package agentprinter

import (
	"math"
	"testing"
)

func TestCalibrationGeometryAndLimits(t *testing.T) {
	c := Defaults()
	c.FormLengthMM, c.PrintScale, c.OffsetYMM = 93.13, 1.02, 1
	if err := ValidateConfig(c); err != nil {
		t.Fatal(err)
	}
	g := c.Calibration().Geometry()
	if math.Abs(g.EffectiveFormMM-93.13) > 25.4/720 || g.FeedMM > g.EffectiveFormMM || math.Abs(g.HeightMM-86.36) > .001 {
		t.Fatalf("unexpected physical dimensions: %+v", g)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.PrintScale = 1.3 },
		func(c *Config) { c.PrintScale = .49 },
		func(c *Config) { c.FormLengthMM = 85 },
		func(c *Config) { c.FormLengthMM = math.NaN() },
		func(c *Config) { c.PrintScale = math.Inf(1) },
	} {
		bad := c
		mutate(&bad)
		if ValidateConfig(bad) == nil {
			t.Fatalf("invalid calibration accepted: %+v", bad.Calibration())
		}
	}
	c.PrintScale = 0 // old JSON files omit scale
	if c.Calibration().Scale != 1 || ValidateConfig(c) != nil {
		t.Fatal("legacy scale changed")
	}
}

func TestCalibrationSheetUsesEffectiveParameters(t *testing.T) {
	c := Defaults()
	c.FormLengthMM, c.PrintScale = 93.13, 1.02
	s := CalibrationSheet(c)
	if !s.CalibrationPattern || s.Calibration != c.Calibration() || len(s.Rows) != 5 {
		t.Fatal("sample diverged from business calibration")
	}
	if s.Rows[3][0].Value != "93.13 / 93.133 mm" {
		t.Fatalf("sample lost form precision: %s", s.Rows[3][0].Value)
	}
	if _, err := Prepare(s); err != nil {
		t.Fatal(err)
	}
}
