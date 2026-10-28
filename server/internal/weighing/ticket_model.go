package weighing

import "time"

type Ticket struct {
	SyncStatus            string        `json:"syncStatus" gorm:"->"`
	SiteCode              string        `json:"siteCode"`
	SiteName              string        `json:"siteName"`
	PrintStatus           string        `json:"printStatus" gorm:"->"`
	ReprintCount          int64         `json:"reprintCount" gorm:"->"`
	LastPrintAttemptID    *int64        `json:"lastPrintAttemptId" gorm:"->"`
	ID                    int64         `json:"id" gorm:"primaryKey"`
	TicketNo              string        `json:"ticketNo"`
	TaskID                int64         `json:"taskId"`
	Revision              int64         `json:"revision"`
	PreviousTicketID      *int64        `json:"previousTicketId"`
	Mode                  string        `json:"mode"`
	Status                string        `json:"status"`
	FirstRecordID         int64         `json:"firstRecordId"`
	SecondRecordID        int64         `json:"secondRecordId"`
	Business              BusinessInfo  `json:"business" gorm:"column:business_snapshot;serializer:json"`
	Basis                 []WeighRecord `json:"basis" gorm:"column:basis_snapshot;serializer:json"`
	SnapshotSchemaVersion int           `json:"snapshotSchemaVersion"`
	OriginalGross         Tonnes        `json:"originalGross" gorm:"column:original_gross_milli_tonne" swaggertype:"string"`
	OriginalTare          Tonnes        `json:"originalTare" gorm:"column:original_tare_milli_tonne" swaggertype:"string"`
	FinalGross            Tonnes        `json:"finalGross" gorm:"column:final_gross_milli_tonne" swaggertype:"string"`
	FinalTare             Tonnes        `json:"finalTare" gorm:"column:final_tare_milli_tonne" swaggertype:"string"`
	GrossSource           string        `json:"grossSource"`
	TareSource            string        `json:"tareSource"`
	AdjustmentRemark      string        `json:"adjustmentRemark"`
	AdjustedBy            *int64        `json:"adjustedBy"`
	AdjustedAt            *time.Time    `json:"adjustedAt"`
	IssuedAt              time.Time     `json:"issuedAt"`
	IssuedBy              int64         `json:"issuedBy"`
	IssuerName            string        `json:"issuerName"`
	TicketTitle           string        `json:"ticketTitle"`
	VoidedAt              *time.Time    `json:"voidedAt"`
	VoidedBy              *int64        `json:"voidedBy"`
	VoidReason            string        `json:"voidReason"`
}

func (Ticket) TableName() string { return "wb_ticket" }

type PriorFinalWeights struct {
	Gross Tonnes `json:"gross" swaggertype:"string"`
	Tare  Tonnes `json:"tare" swaggertype:"string"`
	Net   Tonnes `json:"net" swaggertype:"string"`
}
type TicketView struct {
	PrintSheet    *PrintSheet        `json:"printSheet,omitempty"`
	PreviousFinal *PriorFinalWeights `json:"previousFinal"`
	Ticket
	OriginalNet     Tonnes           `json:"originalNet" swaggertype:"string"`
	FinalNet        Tonnes           `json:"finalNet" swaggertype:"string"`
	GrossDifference WeightDifference `json:"grossDifference" swaggertype:"string"`
	TareDifference  WeightDifference `json:"tareDifference" swaggertype:"string"`
}

func ticketView(ticket Ticket) TicketView {
	return TicketView{Ticket: ticket, OriginalNet: ticket.OriginalGross - ticket.OriginalTare, FinalNet: ticket.FinalGross - ticket.FinalTare, GrossDifference: WeightDifference(ticket.OriginalGross - ticket.FinalGross), TareDifference: WeightDifference(ticket.OriginalTare - ticket.FinalTare)}
}
