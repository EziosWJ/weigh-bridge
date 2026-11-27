package weighing

import (
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

func TestMobileActorNamePrefersAccount(t *testing.T) {
	if got := mobileActorName(" 酒糟出库 "); got != "酒糟出库" {
		t.Fatalf("mobileActorName = %q", got)
	}
	if got := mobileActorName(""); got != "手机扫码" {
		t.Fatalf("mobileActorName empty = %q", got)
	}
}

func TestActorDisplayNameUsesEventOverride(t *testing.T) {
	got, err := actorDisplayName(nil, audit.Event{Metadata: audit.Metadata{ActorName: "酒糟出库"}})
	if err != nil || got != "酒糟出库" {
		t.Fatalf("actorDisplayName = %q, %v", got, err)
	}
}
