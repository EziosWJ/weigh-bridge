package weighing

import "testing"

func TestResolveNetAdjustment(t *testing.T) {
	amount := func(v WeightDifference) *WeightDifference { return &v }
	rate := func(v int) *int { return &v }
	for _, test := range []struct {
		name        string
		in          *NetAdjustmentInput
		base        Tonnes
		allowUpward bool
		want        WeightDifference
		wantMode    string
		wantRate    *int
		wantErr     bool
	}{
		{name: "none", base: 20000, want: 0, wantMode: "amount"},
		{name: "amount", in: &NetAdjustmentInput{Mode: "amount", Amount: amount(1500)}, base: 20000, want: 1500, wantMode: "amount"},
		{name: "amount upward rejected", in: &NetAdjustmentInput{Mode: "amount", Amount: amount(-500)}, base: 20000, wantErr: true},
		{name: "amount upward on reopen", in: &NetAdjustmentInput{Mode: "amount", Amount: amount(-500)}, base: 20000, allowUpward: true, want: -500, wantMode: "amount"},
		{name: "percent", in: &NetAdjustmentInput{Mode: "percent", RateBP: rate(150)}, base: 20000, want: 300, wantMode: "percent", wantRate: rate(150)},
		{name: "percent rounds half away from zero", in: &NetAdjustmentInput{Mode: "percent", RateBP: rate(5)}, base: 1000, want: 1, wantMode: "percent", wantRate: rate(5)},
		{name: "percent upward on reopen", in: &NetAdjustmentInput{Mode: "percent", RateBP: rate(-5)}, base: 1000, allowUpward: true, want: -1, wantMode: "percent", wantRate: rate(-5)},
		{name: "percent upward rejected", in: &NetAdjustmentInput{Mode: "percent", RateBP: rate(-5)}, base: 1000, wantErr: true},
		{name: "percent zero is no adjustment", in: &NetAdjustmentInput{Mode: "percent", RateBP: rate(0)}, base: 1000, want: 0, wantMode: "amount"},
		{name: "percent below precision", in: &NetAdjustmentInput{Mode: "percent", RateBP: rate(1)}, base: 1, wantErr: true},
		{name: "rate without percent", in: &NetAdjustmentInput{Mode: "amount", Amount: amount(1), RateBP: rate(1)}, base: 1000, wantErr: true},
		{name: "percent missing rate", in: &NetAdjustmentInput{Mode: "percent"}, base: 1000, wantErr: true},
		{name: "percent out of range", in: &NetAdjustmentInput{Mode: "percent", RateBP: rate(10001)}, base: 1000, wantErr: true},
		{name: "unknown mode", in: &NetAdjustmentInput{Mode: "ratio"}, base: 1000, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, mode, gotRate, err := resolveNetAdjustment(test.in, test.base, test.allowUpward)
			if test.wantErr {
				if err == nil {
					t.Fatalf("accepted %+v", test.in)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want || mode != test.wantMode {
				t.Fatalf("got %v %s, want %v %s", got, mode, test.want, test.wantMode)
			}
			if (gotRate == nil) != (test.wantRate == nil) || (gotRate != nil && *gotRate != *test.wantRate) {
				t.Fatalf("rate %v, want %v", gotRate, test.wantRate)
			}
		})
	}
}

func TestMakeTicketAppliesNetAdjustment(t *testing.T) {
	task := Task{ID: 1, Direction: "inbound", SiteCode: "SITE01"}
	first := WeighRecord{ID: 10, TaskID: 1, Stage: "first", Kind: "gross", Weight: 30000}
	second := WeighRecord{ID: 11, TaskID: 1, Stage: "second", Kind: "tare", Weight: 10000}
	amount := func(v WeightDifference) *WeightDifference { return &v }
	ticket, err := makeTicket(task, first, second, nil, &NetAdjustmentInput{Mode: "amount", Amount: amount(1500), Category: "WATER", Remark: "水分大"}, "水分大", false, true, 7, "操作员", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.OriginalGross != 30000 || ticket.OriginalTare != 10000 || ticket.FinalGross != 30000 || ticket.FinalTare != 10000 || ticket.NetAdjustment != 1500 || ticket.NetAdjustmentCategory != "WATER" || ticket.NetAdjustmentCategoryName != "水分大" || ticket.AdjustmentRemark != "水分大" || ticket.AdjustedBy == nil {
		t.Fatalf("ticket adjustment: %+v", ticket)
	}
	if view := ticketView(ticket); view.FinalNet != 18500 {
		t.Fatalf("final net = %v", view.FinalNet)
	}
	if _, err := makeTicket(task, first, second, nil, &NetAdjustmentInput{Mode: "amount", Amount: amount(-500), Category: "WATER"}, "水分大", false, true, 7, "操作员", nil); err == nil {
		t.Fatal("normal issuance accepted an upward adjustment")
	}
	if _, err := makeTicket(task, first, second, nil, &NetAdjustmentInput{Mode: "amount", Amount: amount(20000), Category: "WATER"}, "水分大", false, true, 7, "操作员", nil); err == nil {
		t.Fatal("accepted a deduction that zeroes the net weight")
	}
	if _, err := makeTicket(task, first, second, nil, &NetAdjustmentInput{Mode: "amount", Amount: amount(1500)}, "", false, true, 7, "操作员", nil); err == nil {
		t.Fatal("accepted a deduction without a category")
	}
}
