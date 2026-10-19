package weighing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExactTonneInputAndFormatting(t *testing.T) {
	for _, test := range []struct{ input, want string }{{`"30.125"`, `"30.125"`}, {`"9.000"`, `"9"`}, {`"0.001"`, `"0.001"`}, {`"5.250"`, `"5.25"`}} {
		var value Tonnes
		if err := json.Unmarshal([]byte(test.input), &value); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(value)
		if err != nil || string(data) != test.want {
			t.Fatalf("%s -> %s (%v)", test.input, data, err)
		}
	}
	for _, input := range []string{`30.125`, `"NaN"`, `"-1"`, `"1.0001"`, `"1e3"`, `"01"`, `"9223372036854775.808"`, `"1."`} {
		var value Tonnes
		if err := json.Unmarshal([]byte(input), &value); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	if Tonnes(-1001).String() != "-1.001" {
		t.Fatal("signed adjustment formatting")
	}
}

func TestRejectedHistoryRedactsNestedSessionTokens(t *testing.T) {
	payload := struct {
		ID    int64
		Input map[string]any
	}{9223372036854775807, map[string]any{"sessionToken": "secret-session", "sampleId": int64(7)}}
	saved := string(redactedPayload(payload))
	if strings.Contains(saved, "secret-session") || strings.Contains(saved, "sessionToken") || !strings.Contains(saved, "9223372036854775807") || !strings.Contains(saved, "sampleId") {
		t.Fatalf("rejected snapshot=%s", saved)
	}
}
