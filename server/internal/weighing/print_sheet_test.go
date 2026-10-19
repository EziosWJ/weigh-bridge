package weighing

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCompactPrintSheetUsesTicketSnapshotAndSelectedBasis(t *testing.T) {
	for _, direction := range []string{"inbound", "outbound"} {
		t.Run(direction, func(t *testing.T) {
			first := time.Date(2026, 10, 18, 15, 59, 0, 0, time.UTC)
			second := first.Add(2 * time.Hour)
			view := ticketView(Ticket{
				TicketNo: "GB0001", Status: "valid", FirstRecordID: 2, SecondRecordID: 3,
				Business:      BusinessInfo{BusinessInput: BusinessInput{Plate: "晋A12345", Direction: direction}, PartyName: "出单时供收方", MaterialName: "高粱", MaterialSpec: "一级"},
				OriginalGross: 32123, OriginalTare: 12123, FinalGross: 31000, FinalTare: 12250,
				IssuerName: "出单操作员", ReprintCount: 2,
				Basis: []WeighRecord{{ID: 1, Stage: "first", ConfirmedAt: first.Add(-time.Hour)}, {ID: 2, Stage: "first", ConfirmedAt: first}, {ID: 3, Stage: "second", ConfirmedAt: second}},
			})
			sheet := compactPrintSheet(view, "示例酒厂过磅单")
			if sheet.Title != "示例酒厂过磅单" || sheet.WidthMM != 210 || sheet.HeightMM != 99 || sheet.Layers != 5 {
				t.Fatalf("wrong paper: %+v", sheet)
			}
			fields := map[string]string{}
			for _, row := range sheet.Rows {
				for _, field := range row {
					fields[field.Label] = field.Value
				}
			}
			wantParty, wantDirection := "供货方", "入厂卸货"
			if direction == "outbound" {
				wantParty, wantDirection = "收货方", "出厂装货"
			}
			for key, want := range map[string]string{
				"单号": "GB0001", "车牌": "晋A12345", "业务": wantDirection, wantParty: "出单时供收方", "物料": "高粱 / 一级",
				"毛重（吨）": "31", "皮重（吨）": "12.25", "净重（吨）": "18.75",
				"首次称重": "2026-10-18 23:59:00", "二次称重": "2026-10-19 01:59:00", "操作员": "出单操作员", "司机签字": "________________",
			} {
				if fields[key] != want {
					t.Errorf("%s=%q, want %q", key, fields[key], want)
				}
			}
			data, _ := json.Marshal(sheet)
			if strings.Contains(string(data), "32.123") || strings.Contains(string(data), "12.123") || len(fields) != 12 {
				t.Fatalf("paper must contain only compact final fields: %s", data)
			}
			if len(sheet.Marks) != 1 || sheet.Marks[0] != "补打第2次" {
				t.Fatalf("reprint mark: %v", sheet.Marks)
			}
		})
	}
}

func TestCompactPrintSheetPreservesLegacyAndVoidMarksWithoutInventingMissingData(t *testing.T) {
	sheet := compactPrintSheet(ticketView(Ticket{Mode: "test", Status: "void"}), "")
	if sheet.Title != "酒厂过磅单" || strings.Join(sheet.Marks, ",") != "历史测试单,已作废" {
		t.Fatalf("marks/title: %+v", sheet)
	}
	if sheet.Rows[3][0].Value != "—" || sheet.Rows[4][0].Value != "—" {
		t.Fatal("missing historical times and actor must not be replaced with current data")
	}
}
