package weighing

import (
	"strconv"
	"strings"
	"time"
)

// PrintSheet is the compact paper view shared by Web and Agent. Layers describe
// carbonless paper, never the number of print jobs or printer copies.
type PrintSheet struct {
	Template string         `json:"template"`
	WidthMM  int            `json:"widthMm"`
	HeightMM int            `json:"heightMm"`
	Layers   int            `json:"layers"`
	Title    string         `json:"title"`
	Marks    []string       `json:"marks"`
	Rows     [][]PrintField `json:"rows"`
}

type PrintField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

func compactPrintSheet(ticket TicketView, title string) *PrintSheet {
	if strings.TrimSpace(title) == "" {
		title = "酒厂过磅单"
	}
	field := func(label, value string) PrintField {
		if strings.TrimSpace(value) == "" {
			value = "—"
		}
		return PrintField{Label: label, Value: value}
	}
	confirmedAt := func(id int64) string {
		for _, record := range ticket.Basis {
			if record.ID == id && !record.ConfirmedAt.IsZero() {
				return record.ConfirmedAt.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02 15:04:05")
			}
		}
		return ""
	}
	if ticket.SiteName != "" {
		title += " · " + ticket.SiteName
	}
	partyLabel, direction := "供货方", "入厂卸货"
	if ticket.Business.Direction == "outbound" {
		partyLabel, direction = "收货方", "出厂装货"
	}
	material := ticket.Business.MaterialName
	if ticket.Business.MaterialSpec != "" {
		material += " / " + ticket.Business.MaterialSpec
	}
	marks := []string{}
	if ticket.Mode == "test" {
		marks = append(marks, "历史测试单")
	}
	if ticket.Status == "void" {
		marks = append(marks, "已作废")
	}
	if ticket.ReprintCount > 0 {
		marks = append(marks, "补打第"+strconv.FormatInt(ticket.ReprintCount, 10)+"次")
	}
	return &PrintSheet{
		Template: "weighing-compact-v1", WidthMM: 210, HeightMM: 99, Layers: 5,
		Title: title, Marks: marks,
		Rows: [][]PrintField{
			{field("单号", ticket.TicketNo), field("车牌", ticket.Business.Plate), field("业务", direction)},
			{field(partyLabel, ticket.Business.PartyName), field("物料", material)},
			{field("毛重（吨）", ticket.FinalGross.String()), field("皮重（吨）", ticket.FinalTare.String()), field("净重（吨）", ticket.FinalNet.String())},
			{field("首次称重", confirmedAt(ticket.FirstRecordID)), field("二次称重", confirmedAt(ticket.SecondRecordID))},
			{field("操作员", ticket.IssuerName), field("司机签字", "________________")},
		},
	}
}
