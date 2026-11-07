package weighing

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDigestV3Vectors(t *testing.T) {
	data, err := os.ReadFile("testdata/digest_v3_vectors.json")
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
	if file.RulesVersion != SyncDigestVersion3 {
		t.Fatalf("rulesVersion=%d", file.RulesVersion)
	}
	for _, vector := range file.Vectors {
		var payload struct {
			syncChangeV4
			Ticket *syncTicketV5 `json:"ticket"`
		}
		if err := json.Unmarshal(vector.Change, &payload); err != nil {
			t.Fatalf("%s: %v", vector.Name, err)
		}
		change := payload.syncChangeV4
		var items []syncNetAdjustmentItemV5
		if payload.Ticket != nil {
			items = payload.Ticket.NetAdjustments
		}
		if payload.Ticket != nil {
			// The expected digest covers the literal wire payload, so the
			// published vectors must also be legal, self-consistent tickets.
			checkVectorWeights(t, vector.Name, payload.Ticket)
			change.Ticket = &payload.Ticket.syncTicket
		}
		digest, err := computeChangeDigestV5(change, items)
		if err != nil {
			t.Fatalf("%s: %v", vector.Name, err)
		}
		if vector.ExpectedDigest == "" {
			t.Logf("%s: expectedDigest placeholder -> %s", vector.Name, digest)
			continue
		}
		if digest != vector.ExpectedDigest {
			t.Errorf("%s: digest=%s want=%s", vector.Name, digest, vector.ExpectedDigest)
		}
	}
}

func TestComputeChangeDigestV5Rules(t *testing.T) {
	ticket := &syncTicket{TicketNo: "F001-20261107-0001", Title: "酒厂过磅单", Plate: "晋A12345", Direction: "inbound", MaterialCode: "C-M-0001", MaterialName: "高粱", PartyCode: "C-P-0001", PartyName: "供货方", FirstConfirmedAt: mustDigestTime(t, "2026-11-07T08:00:00Z"), SecondConfirmedAt: mustDigestTime(t, "2026-11-07T08:10:00Z"), IssuedAt: mustDigestTime(t, "2026-11-07T08:11:00Z"), IssuerName: "操作员", OriginalGross: "30", OriginalTare: "10", FinalGross: "30", FinalTare: "10", FinalNet: "18.5", NetAdjustment: "1.5", GrossSource: "collected", TareSource: "collected"}
	rate := func(v int) *int { return &v }
	multi := []syncNetAdjustmentItemV5{
		{Mode: "percent", Amount: "0.2", RateBP: rate(100), Category: "WATER", CategoryName: "水分大", Remark: "水分大"},
		{Mode: "amount", Amount: "0.5", Category: "IMPURITY", CategoryName: "杂质多"},
	}
	single := []syncNetAdjustmentItemV5{
		{Mode: "amount", Amount: "1.5", Category: "WATER", CategoryName: "水分大", Remark: "水分大"},
	}
	base := syncChangeV4{SchemaVersion: SyncSchemaVersionV5, DigestVersion: SyncDigestVersion3, SiteCode: "F001", TicketNo: ticket.TicketNo, Type: "issue", Ticket: ticket}
	digestMulti, err := computeChangeDigestV5(base, multi)
	if err != nil {
		t.Fatal(err)
	}
	digestSingle, err := computeChangeDigestV5(base, single)
	if err != nil {
		t.Fatal(err)
	}
	// Same total amount but different detail must produce a different digest:
	// the v3 rules cover the item array, not only the sum.
	if digestMulti == digestSingle {
		t.Error("multi-item and single-item digests must differ")
	}
	swapped := []syncNetAdjustmentItemV5{multi[1], multi[0]}
	digestSwapped, err := computeChangeDigestV5(base, swapped)
	if err != nil {
		t.Fatal(err)
	}
	if digestSwapped == digestMulti {
		t.Error("item order must participate in the digest")
	}
	if _, err := computeChangeDigestV5(syncChangeV4{SchemaVersion: SyncSchemaVersionV4, DigestVersion: SyncDigestVersion3, SiteCode: "F001", TicketNo: "X", Type: "issue", Ticket: ticket}, multi); err == nil {
		t.Error("schema version must match v5")
	}
	if _, err := computeChangeDigestV5(base, append(single, syncNetAdjustmentItemV5{Mode: "ratio", Amount: "1"})); err == nil {
		t.Error("unknown item mode must be rejected")
	}
}

func digestMilli(t *testing.T, value string) int64 {
	t.Helper()
	canon, err := canonicalDigestWeight(value, true)
	if err != nil {
		t.Fatal(err)
	}
	negative := strings.HasPrefix(canon, "-")
	canon = strings.TrimPrefix(canon, "-")
	whole, fraction, ok := strings.Cut(canon, ".")
	if !ok || len(fraction) != 3 {
		t.Fatalf("canonical weight %q is not a three-decimal tonne string", value)
	}
	wholeMilli, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	fractionMilli, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	milli := wholeMilli*1000 + fractionMilli
	if negative {
		milli = -milli
	}
	return milli
}

// checkVectorWeights keeps the published vectors usable as legal payloads:
// finalNet must equal finalGross-finalTare-netAdjustment, item amounts must sum
// to netAdjustment, and percent items must match their rate on the shared
// adjusted-net basis. Recomputing finalNet here previously hid an inconsistent
// vector (0.7 tonne of items with a 1.5 tonne total).
func checkVectorWeights(t *testing.T, name string, ticket *syncTicketV5) {
	t.Helper()
	base := digestMilli(t, ticket.FinalGross) - digestMilli(t, ticket.FinalTare)
	if got, want := digestMilli(t, ticket.FinalNet), base-digestMilli(t, ticket.NetAdjustment); got != want {
		t.Fatalf("%s: finalNet=%s, want %s from finalGross-finalTare-netAdjustment", name, ticket.FinalNet, fmtNet(want))
	}
	var total int64
	for index, item := range ticket.NetAdjustments {
		amount := digestMilli(t, item.Amount)
		switch item.Mode {
		case "percent":
			if item.RateBP == nil {
				t.Fatalf("%s: item %d is percent without rateBp", name, index)
			}
			if got := roundRatio(base*int64(*item.RateBP), 10000); got != amount {
				t.Fatalf("%s: item %d amount=%s, want %s from rateBp=%d", name, index, item.Amount, fmtNet(got), *item.RateBP)
			}
		case "amount":
			if item.RateBP != nil {
				t.Fatalf("%s: item %d is amount with rateBp", name, index)
			}
		default:
			t.Fatalf("%s: item %d mode=%q", name, index, item.Mode)
		}
		total += amount
	}
	if want := digestMilli(t, ticket.NetAdjustment); total != want {
		t.Fatalf("%s: item sum=%s, want netAdjustment=%s", name, fmtNet(total), fmtNet(want))
	}
}

func fmtNet(milli int64) string {
	negative := milli < 0
	if negative {
		milli = -milli
	}
	text := fmt.Sprintf("%d.%03d", milli/1000, milli%1000)
	if negative {
		return "-" + text
	}
	return text
}

func mustDigestTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
