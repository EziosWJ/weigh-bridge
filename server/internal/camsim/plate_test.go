package camsim

import (
	"strings"
	"testing"
	"unicode"

	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func TestNewPlateMatchesBusinessRule(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		plate := newPlate()
		if !weighing.IsBusinessPlate(plate) {
			t.Fatalf("generated plate rejected by business rule: %q", plate)
		}
		if plate != strings.ToUpper(plate) || strings.ContainsAny(plate, " ·•\t\r\n") {
			t.Fatalf("generated plate is not canonical: %q", plate)
		}
		if len([]rune(plate)) != 7 && len([]rune(plate)) != 8 {
			t.Fatalf("unexpected plate length: %q", plate)
		}
		for _, r := range plate {
			if unicode.IsLetter(r) && (r == 'I' || r == 'O') {
				t.Fatalf("plate contains I/O: %q", plate)
			}
		}
		seen[plate] = true
	}
	if len(seen) < 1900 {
		t.Fatalf("generator repeated too often: %d unique", len(seen))
	}
}

func TestBusinessPlateExamples(t *testing.T) {
	for _, plate := range []string{"晋A12345", "川A54322", "晋AD12345", "晋A12345D"} {
		if !weighing.IsBusinessPlate(plate) {
			t.Fatalf("expected valid: %q", plate)
		}
	}
	for _, plate := range []string{"", "A12345", "晋I12345", "晋A1234", "晋A12345X"} {
		if weighing.IsBusinessPlate(plate) {
			t.Fatalf("expected invalid: %q", plate)
		}
	}
}
