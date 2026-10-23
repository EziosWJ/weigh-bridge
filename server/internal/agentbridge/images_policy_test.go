package agentbridge

import (
	"bytes"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"image"
	"image/png"
	"testing"
)

func TestOptionalImagesRequireExplicitAbsenceAndValidReferences(t *testing.T) {
	hub := New(Config{ScaleID: 1, ImageDir: t.TempDir()})
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
