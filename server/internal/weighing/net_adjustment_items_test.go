package weighing

import (
	"testing"
)

func TestResolveNetAdjustmentItems(t *testing.T) {
	amount := func(v WeightDifference) *WeightDifference { return &v }
	rate := func(v int) *int { return &v }
	name := func(code string) (string, error) { return "名-" + code, nil }
	for _, test := range []struct {
		name    string
		in      []NetAdjustmentInput
		base    Tonnes
		want    WeightDifference
		items   int
		wantErr bool
	}{
		{name: "none", base: 20000, want: 0, items: 0},
		{name: "amount and percent", in: []NetAdjustmentInput{
			{Mode: "percent", RateBP: rate(100), Category: "WATER"},
			{Mode: "amount", Amount: amount(500), Category: "IMPURITY"},
		}, base: 20000, want: 700, items: 2},
		{name: "each percent rounds independently", in: []NetAdjustmentInput{
			{Mode: "percent", RateBP: rate(5), Category: "WATER"},
			{Mode: "percent", RateBP: rate(5), Category: "IMPURITY"},
		}, base: 1000, want: 2, items: 2},
		{name: "zero entries dropped", in: []NetAdjustmentInput{
			{Mode: "percent", RateBP: rate(0), Category: "WATER"},
		}, base: 1000, want: 0, items: 0},
		{name: "upward rejected normally", in: []NetAdjustmentInput{
			{Mode: "amount", Amount: amount(-500), Category: "WATER"},
		}, base: 1000, wantErr: true},
		{name: "upward rejected on reopen", in: []NetAdjustmentInput{
			{Mode: "amount", Amount: amount(-500), Category: "WATER"},
			{Mode: "percent", RateBP: rate(100), Category: "IMPURITY"},
		}, base: 1000, wantErr: true},
		{name: "total zeroes the net", in: []NetAdjustmentInput{
			{Mode: "amount", Amount: amount(600)},
			{Mode: "amount", Amount: amount(400)},
		}, base: 1000, wantErr: true},
		{name: "total exceeds net", in: []NetAdjustmentInput{
			{Mode: "amount", Amount: amount(600)},
			{Mode: "amount", Amount: amount(700)},
		}, base: 1000, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			items, total, err := resolveNetAdjustmentItems(test.in, test.base, name)
			if test.wantErr {
				if err == nil {
					t.Fatalf("accepted %+v", test.in)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if total != test.want || len(items) != test.items {
				t.Fatalf("total=%v items=%d, want %v/%d", total, len(items), test.want, test.items)
			}
			for _, item := range items {
				if item.CategoryName == "" {
					t.Fatalf("missing category name: %+v", item)
				}
				if item.Mode == "percent" && item.RateBP == nil {
					t.Fatalf("percent item missing rate: %+v", item)
				}
			}
		})
	}
}

func TestMakeTicketWithItems(t *testing.T) {
	task := Task{ID: 2, Direction: "inbound", SiteCode: "SITE01"}
	first := WeighRecord{ID: 20, TaskID: 2, Stage: "first", Kind: "gross", Weight: 30000}
	second := WeighRecord{ID: 21, TaskID: 2, Stage: "second", Kind: "tare", Weight: 10000}
	rate := func(v int) *int { return &v }
	amount := func(v WeightDifference) *WeightDifference { return &v }
	entries := NetAdjustmentsInput{
		{Mode: "percent", RateBP: rate(100), Category: "WATER", Remark: "水分大", ResolvedCategoryName: "水分大"},
		{Mode: "amount", Amount: amount(500), Category: "IMPURITY", ResolvedCategoryName: "杂质多"},
	}
	ticket, err := makeTicketWithItems(task, first, second, nil, entries, "", true, 7, "操作员", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.NetAdjustment != 700 || len(ticket.NetAdjustmentItems) != 2 {
		t.Fatalf("ticket: adjustment=%v items=%d", ticket.NetAdjustment, len(ticket.NetAdjustmentItems))
	}
	if ticket.NetAdjustmentItems[0].AmountMilli != 200 || ticket.NetAdjustmentItems[0].CategoryName != "水分大" {
		t.Fatalf("item0: %+v", ticket.NetAdjustmentItems[0])
	}
	if ticket.NetAdjustmentItems[1].AmountMilli != 500 || ticket.NetAdjustmentItems[1].Mode != "amount" || ticket.NetAdjustmentItems[1].RateBP != nil {
		t.Fatalf("item1: %+v", ticket.NetAdjustmentItems[1])
	}
	if view := ticketView(ticket); view.FinalNet != 19300 {
		t.Fatalf("final net = %v", view.FinalNet)
	}
	// Multi-item rows keep the legacy single-item columns neutral.
	if ticket.NetAdjustmentMode != "amount" || ticket.NetAdjustmentRateBP != nil || ticket.NetAdjustmentCategory != "" || ticket.NetAdjustmentCategoryName != "" {
		t.Fatalf("legacy columns on multi-item ticket: %+v", ticket)
	}
	if ticket.AdjustedBy == nil || ticket.AdjustmentRemark != "水分大" {
		t.Fatalf("adjustment evidence: %+v", ticket)
	}
	// Upward mixed entries are rejected on normal issuance.
	up := NetAdjustmentsInput{
		{Mode: "amount", Amount: amount(-500), Category: "WATER", ResolvedCategoryName: "水分大"},
		{Mode: "amount", Amount: amount(300), Category: "IMPURITY", ResolvedCategoryName: "杂质多"},
	}
	if _, err := makeTicketWithItems(task, first, second, nil, up, "", true, 7, "操作员", nil); err == nil {
		t.Fatal("normal issuance accepted an upward item")
	}
}
