package agentbridge

import (
	"bytes"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptionalImagesRequireExplicitAbsenceAndValidReferences(t *testing.T) {
	hub := New(Config{ScaleCode: "WB01", ImageDir: t.TempDir()})
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	front, err := hub.SaveImage("front", bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rear, err := hub.SaveImage("rear", bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, status    string
		images          []p.Image
		required, valid bool
	}{
		{"default requires both", "", nil, true, false},
		{"optional legacy empty rejected", "", nil, false, false},
		{"optional disabled", "disabled", nil, false, true},
		{"mandatory disabled rejected", "disabled", nil, true, false},
		{"capture failed", "capture_failed", nil, false, true},
		{"upload partial", "upload_failed", []p.Image{front}, false, true},
		{"complete pair", "complete", []p.Image{front, rear}, true, true},
		{"mandatory contradictory state", "disabled", []p.Image{front, rear}, true, false},
		{"invalid partial reference", "upload_failed", []p.Image{{ID: "missing"}}, false, false},
		{"duplicate view", "capture_failed", []p.Image{front, front}, false, false},
		{"partial marked complete", "complete", []p.Image{front}, false, false},
		{"disabled with image", "disabled", []p.Image{front}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := hub.resolveCaptureImages(p.Capture{Images: tc.images, ImageStatus: tc.status}, tc.required)
			if (err == nil) != tc.valid {
				t.Fatalf("images=%v err=%v", out, err)
			}
		})
	}
}

func writeLegacyImage(dir, id string, data []byte) error {
	return os.WriteFile(filepath.Join(dir, id), data, 0o600)
}

func TestImageIdentityKeepsUnderscoreScaleCodes(t *testing.T) {
	hub := New(Config{ScaleCode: "WB_01", ImageDir: t.TempDir()})
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	saved, err := hub.SaveImage("front", bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	code, ok := ParseImageID(saved.ID)
	if !ok || code != "WB_01" {
		t.Fatalf("parsed scale code %q ok=%v from %s", code, ok, saved.ID)
	}
	if _, err := hub.Image(saved.ID); err != nil {
		t.Fatalf("underscore scale code image not readable: %v", err)
	}
	// Pre-code Agent images carry the numeric scale id; a single connected
	// scale keeps them readable without rewriting history.
	legacyID := "1_0123456789abcdef0123456789abcdef_front.png"
	if err := writeLegacyImage(hub.cfg.ImageDir, legacyID, data.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Image(legacyID); err != nil {
		t.Fatalf("legacy numeric image not readable: %v", err)
	}
	oldCodeID := "OLD_WB_0123456789abcdef0123456789abcdef_front.png"
	if err := writeLegacyImage(hub.cfg.ImageDir, oldCodeID, data.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.resolveCaptureImages(p.Capture{ImageStatus: "upload_failed", Images: []p.Image{{ID: oldCodeID}}}, false); err == nil {
		t.Fatal("old historical image accepted as evidence from current capture")
	}
}

// Historical image prefixes must remain safe when they differ from the current code.
func TestParseImageIDRejectsUnsafePrefixes(t *testing.T) {
	for _, id := range []string{
		"../WB_0123456789abcdef0123456789abcdef_front.png",
		"/WB_0123456789abcdef0123456789abcdef_front.png",
		strings.Repeat("a", 41) + "_0123456789abcdef0123456789abcdef_front.png",
	} {
		if _, ok := ParseImageID(id); ok {
			t.Errorf("accepted unsafe image id %q", id)
		}
	}
}
