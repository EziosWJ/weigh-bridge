package weighing

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestBusinessDateUsesBeijingMidnight(t *testing.T) {
	before := time.Date(2026, 10, 27, 15, 59, 59, 0, time.UTC)
	after := time.Date(2026, 10, 27, 16, 0, 0, 0, time.UTC)
	if got := businessDate(before); got != "20261027" {
		t.Fatalf("date before Beijing midnight = %s", got)
	}
	if got := businessDate(after); got != "20261028" {
		t.Fatalf("date after Beijing midnight = %s", got)
	}
}

func TestWatermarkFloorsAndFailClosed(t *testing.T) {
	dir := t.TempDir()
	key := watermarkKey{Kind: "ticket", SiteCode: "F001", Date: "20261027"}
	if floor, err := watermarkFloor(dir, key); err != nil || floor != 0 {
		t.Fatalf("missing file floor = %d, %v", floor, err)
	}
	if err := raiseWatermark(dir, key, 7); err != nil {
		t.Fatal(err)
	}
	if err := raiseWatermark(dir, key, 3); err != nil {
		t.Fatal(err)
	}
	if floor, err := watermarkFloor(dir, key); err != nil || floor != 7 {
		t.Fatalf("monotonic floor = %d, %v", floor, err)
	}
	other := watermarkKey{Kind: "ticket", SiteCode: "F001", Date: "20261028"}
	if err := raiseWatermark(dir, other, 2); err != nil {
		t.Fatal(err)
	}
	if floor, err := watermarkFloor(dir, key); err != nil || floor != 7 {
		t.Fatalf("other day changed floor = %d, %v", floor, err)
	}
	// An empty or corrupt floor file must stop numbering instead of silently
	// resetting every issued serial.
	for _, contents := range []string{"", "{not-json"} {
		if err := os.WriteFile(watermarkPath(dir), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := watermarkFloor(dir, key); err == nil {
			t.Fatalf("corrupt watermark %q accepted", contents)
		}
		if err := raiseWatermark(dir, key, 9); err == nil {
			t.Fatalf("corrupt watermark %q allowed a new serial", contents)
		}
	}
}

func TestWatermarkConcurrentRaiseKeepsHighest(t *testing.T) {
	dir := t.TempDir()
	key := watermarkKey{Kind: "ticket", SiteCode: "F001", Date: "20261027"}
	var wg sync.WaitGroup
	for serial := int64(1); serial <= 20; serial++ {
		wg.Add(1)
		go func(value int64) {
			defer wg.Done()
			if err := raiseWatermark(dir, key, value); err != nil {
				t.Errorf("raise %d: %v", value, err)
			}
		}(serial)
	}
	wg.Wait()
	if floor, err := watermarkFloor(dir, key); err != nil || floor != 20 {
		t.Fatalf("concurrent floor = %d, %v", floor, err)
	}
	if _, err := os.Stat(filepath.Join(dir, watermarkFile)); err != nil {
		t.Fatal(err)
	}
}
