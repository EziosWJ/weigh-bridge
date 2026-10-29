package weighing

import (
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/camera"
)

func TestCandidatePaginationRetainsOmittedEvents(t *testing.T) {
	groups := make([]camera.PlateCandidate, 7)
	for i := range groups {
		id := int64(7 - i)
		groups[i] = camera.PlateCandidate{ID: id, SourceEvents: []camera.CandidateEvent{{ID: id}}}
	}
	first, cursor := paginateCandidates(groups, 0, 7, false)
	if len(first) != 5 || cursor != 5 {
		t.Fatalf("first page: %d cursor %d", len(first), cursor)
	}
	second, cursor := paginateCandidates(groups, cursor, 7, false)
	if len(second) != 2 || second[0].ID != 6 || second[1].ID != 7 || cursor != 7 {
		t.Fatalf("second page: %+v cursor %d", second, cursor)
	}
	initial, cursor := paginateCandidates(groups, 0, 7, true)
	if len(initial) != 5 || initial[0].ID != 7 || cursor != 7 {
		t.Fatalf("initial page: %+v cursor %d", initial, cursor)
	}
	if got, err := parsePlateCursor(plateCursor(7)); err != nil || got != 7 {
		t.Fatalf("cursor: %d %v", got, err)
	}
	if _, err := parsePlateCursor("invalid!"); err == nil {
		t.Fatal("invalid cursor accepted")
	}
}
