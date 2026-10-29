package camera

import (
	"testing"
	"time"
)

func TestAggregatePlateCandidates(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := func(id int64, camera int64, plate string, age time.Duration) CandidateEvent {
		at := now.Add(-age)
		return CandidateEvent{ID: id, CameraID: camera, Plate: plate, ReceivedAt: at, RecognizedAt: at}
	}
	a := e(1, 1, "A", 15*time.Second)
	b := e(2, 2, "A", 0)
	c := e(3, 2, "B", 0)
	old := e(4, 1, "C", 60*time.Second)
	late := e(5, 1, "D", 0)
	late.RecognizedAt = now.Add(-61 * time.Second)
	got := AggregatePlateCandidates([]CandidateEvent{c, a, b, a, old, late}, now)
	if len(got) != 2 {
		t.Fatalf("got %d candidates", len(got))
	}
	for _, g := range got {
		if g.Plate == "A" && (len(g.SourceEvents) != 2 || g.ID != 1) {
			t.Fatalf("bad merge: %+v", g)
		}
	}
}

func TestCandidateTimeBoundaries(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []CandidateEvent{
		{ID: 1, Plate: "A", ReceivedAt: now.Add(-30 * time.Second), RecognizedAt: now.Add(-30 * time.Second)},
		{ID: 2, Plate: "A", ReceivedAt: now.Add(-15 * time.Second), RecognizedAt: now.Add(-15 * time.Second)},
		{ID: 3, Plate: "A", ReceivedAt: now.Add(time.Second), RecognizedAt: now},
		{ID: 4, Plate: "B", ReceivedAt: now, RecognizedAt: now.Add(time.Second)},
		{ID: 5, Plate: "C", ReceivedAt: now, RecognizedAt: now.Add(-60 * time.Second)},
		{ID: 6, Plate: "", ReceivedAt: now, RecognizedAt: now},
		{ID: 7, Plate: "A", ReceivedAt: now.Add(time.Millisecond), RecognizedAt: now},
	}
	got := AggregatePlateCandidates(events, now)
	if len(got) != 2 {
		t.Fatalf("unexpected candidates: %+v", got)
	}
	for _, group := range got {
		if group.Plate == "A" && len(group.SourceEvents) != 2 {
			t.Fatalf("15s boundary: %+v", group)
		}
	}
	if len(AggregatePlateCandidates(events[:1], now.Add(30*time.Second))) != 0 {
		t.Fatal("candidate survived its 60s lifetime")
	}
}

func TestAggregatePlateCandidatesDeduplicatesSamePlate(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := func(id int64, camera int64, plate string, age time.Duration) CandidateEvent {
		at := now.Add(-age)
		return CandidateEvent{ID: id, CameraID: camera, Plate: plate, ReceivedAt: at, RecognizedAt: at}
	}
	// The same plate arriving in a later batch (more than 15s apart) keeps only
	// its newest group, so it cannot occupy several display slots.
	got := AggregatePlateCandidates([]CandidateEvent{
		e(1, 1, "A", 40*time.Second),
		e(2, 1, "B", 20*time.Second),
		e(3, 2, "A", 10*time.Second),
	}, now)
	if len(got) != 2 || got[0].Plate != "A" || got[1].Plate != "B" {
		t.Fatalf("duplicate plate candidates: %+v", got)
	}
	if got[0].ID != 3 || len(got[0].SourceEvents) != 1 || got[0].SourceEvents[0].ID != 3 {
		t.Fatalf("kept older group: %+v", got[0])
	}
}
