package camera

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestSyntheticSnapshotFixture(t *testing.T) {
	body, err := os.ReadFile("../../../docs/device/fixtures/car-license-snapshot.synthetic.json")
	if err != nil {
		t.Fatal(err)
	}
	var req snapshotRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if req.Info.License.Number != "晋A12345" || string(req.Info.EventID) != "66" {
		t.Fatalf("unexpected fixture: %+v", req)
	}
}

func TestSnapshotExpiryBoundary(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		age     time.Duration
		expired bool
	}{
		{5*time.Minute - time.Nanosecond, false}, {5 * time.Minute, false}, {5*time.Minute + time.Nanosecond, true}, {-time.Minute, false},
	} {
		if got := snapshotExpired(now.Add(-tc.age), now, snapshotMaxAge); got != tc.expired {
			t.Fatalf("age %s: %v", tc.age, got)
		}
	}
}
