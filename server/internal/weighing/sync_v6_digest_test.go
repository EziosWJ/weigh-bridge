package weighing

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestManualSupplementDigestV6(t *testing.T) {
	data, err := os.ReadFile("testdata/digest_v3_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []struct {
			Change json.RawMessage `json:"change"`
		} `json:"vectors"`
	}
	if err = json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		syncChangeV4
		Ticket *syncTicketV5 `json:"ticket"`
	}
	if err = json.Unmarshal(file.Vectors[0].Change, &payload); err != nil {
		t.Fatal(err)
	}
	c := payload.syncChangeV4
	c.Ticket = &payload.Ticket.syncTicket
	c.SchemaVersion, c.DigestVersion = SyncSchemaVersionV6, SyncDigestVersion4
	c.Ticket.GrossSource, c.Ticket.TareSource = "manual", "manual"
	manual := syncManualSupplementV6{EntryType: "manual_supplement", FirstWeighedAt: c.Ticket.FirstConfirmedAt, SecondWeighedAt: c.Ticket.SecondConfirmedAt}
	digest, err := computeChangeDigestV6(c, payload.Ticket.NetAdjustments, manual)
	if err != nil {
		t.Fatal(err)
	}
	shifted := manual
	shifted.FirstWeighedAt = shifted.FirstWeighedAt.Add(-time.Second)
	other, err := computeChangeDigestV6(c, payload.Ticket.NetAdjustments, shifted)
	if err != nil || other == digest {
		t.Fatalf("actual time must affect digest: %s %v", other, err)
	}
	shifted.FirstWeighedAt = shifted.SecondWeighedAt.Add(time.Second)
	if _, err = computeChangeDigestV6(c, payload.Ticket.NetAdjustments, shifted); err == nil {
		t.Fatal("reversed times accepted")
	}
	c.Ticket.GrossSource = "collected"
	if _, err = computeChangeDigestV6(c, payload.Ticket.NetAdjustments, manual); err == nil {
		t.Fatal("collected source accepted")
	}
}

func TestManualSupplementV6WireContainsActualTimes(t *testing.T) {
	first := mustDigestTime(t, "2026-11-17T01:02:03Z")
	second := first.Add(time.Hour)
	wire := struct {
		*syncTicketV5
		*syncManualSupplementV6
	}{&syncTicketV5{syncTicket: syncTicket{TicketNo: "T1"}, NetAdjustments: []syncNetAdjustmentItemV5{}}, &syncManualSupplementV6{EntryType: "manual_supplement", FirstWeighedAt: first, SecondWeighedAt: second}}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		EntryType string                    `json:"entryType"`
		First     time.Time                 `json:"firstWeighedAt"`
		Second    time.Time                 `json:"secondWeighedAt"`
		Items     []syncNetAdjustmentItemV5 `json:"netAdjustments"`
	}
	if err = json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.EntryType != "manual_supplement" || !got.First.Equal(first) || !got.Second.Equal(second) || got.Items == nil {
		t.Fatalf("wire=%s", data)
	}
}
