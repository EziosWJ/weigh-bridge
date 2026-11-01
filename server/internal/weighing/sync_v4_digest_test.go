package weighing

import (
	"encoding/json"
	"os"
	"testing"
)

func TestDigestV2Vectors(t *testing.T) {
	data, err := os.ReadFile("testdata/digest_v2_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		RulesVersion int `json:"rulesVersion"`
		Vectors      []struct {
			Name           string          `json:"name"`
			Change         json.RawMessage `json:"change"`
			ExpectedDigest string          `json:"expectedDigest"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if file.RulesVersion != SyncDigestVersion2 {
		t.Fatalf("rulesVersion=%d", file.RulesVersion)
	}
	if len(file.Vectors) != 12 {
		t.Fatalf("vectors=%d", len(file.Vectors))
	}
	got := map[string]string{}
	for _, vector := range file.Vectors {
		var change syncChangeV4
		if err := json.Unmarshal(vector.Change, &change); err != nil {
			t.Fatalf("%s: %v", vector.Name, err)
		}
		digest, err := computeChangeDigestV4(change)
		if err != nil {
			t.Fatalf("%s: %v", vector.Name, err)
		}
		if digest != vector.ExpectedDigest {
			t.Errorf("%s: digest=%s want=%s", vector.Name, digest, vector.ExpectedDigest)
		}
		got[vector.Name] = digest
	}
	for _, pair := range [][2]string{{"issue-basic", "issue-normalized"}, {"issue-adjusted", "issue-adjusted-truncated"}, {"void-basic", "void-normalized"}} {
		if got[pair[0]] != got[pair[1]] {
			t.Errorf("%s and %s must normalize identically", pair[0], pair[1])
		}
	}
	if got["issue-basic"] == got["issue-empty-adjusted-by"] {
		t.Error("empty adjustedBy must differ from null")
	}
}

func TestCanonicalDigestWeight(t *testing.T) {
	for _, test := range []struct {
		value  string
		signed bool
		want   string
		ok     bool
	}{
		{"30", false, "30.000", true},
		{"30.0", false, "30.000", true},
		{"30.000", false, "30.000", true},
		{"030", false, "30.000", true},
		{"-0.5", true, "-0.500", true},
		{"0", false, "0.000", true},
		{"-0", true, "0.000", true},
		{"-1", false, "", false},
		{"1.", false, "", false},
		{"1.0000", false, "", false},
		{"", false, "", false},
		{" 1", false, "", false},
		{"+1", true, "", false},
		{"1e3", false, "", false},
	} {
		got, err := canonicalDigestWeight(test.value, test.signed)
		if (err == nil) != test.ok || (test.ok && got != test.want) {
			t.Errorf("weight %q signed=%v -> %q, %v", test.value, test.signed, got, err)
		}
	}
}
