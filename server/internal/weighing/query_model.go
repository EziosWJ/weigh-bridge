package weighing

import "time"

type OriginalView struct {
	Task
	OriginalGross       Tonnes    `json:"originalGross" swaggertype:"string"`
	OriginalTare        Tonnes    `json:"originalTare" swaggertype:"string"`
	OriginalNet         Tonnes    `json:"originalNet" gorm:"-" swaggertype:"string"`
	FirstConfirmedAt    time.Time `json:"firstConfirmedAt"`
	SecondConfirmedAt   time.Time `json:"secondConfirmedAt"`
	CurrentTicketID     *int64    `json:"currentTicketId"`
	CurrentTicketStatus string    `json:"currentTicketStatus"`
}
type SummaryLine struct {
	SiteCode     string `json:"siteCode"`
	Direction    string `json:"direction"`
	MaterialID   string `json:"materialId"`
	MaterialName string `json:"materialName"`
	MaterialSpec string `json:"materialSpec"`
	PartyID      string `json:"partyId"`
	PartyName    string `json:"partyName"`
	TaskCount    int64  `json:"taskCount"`
	Gross        Tonnes `json:"gross" swaggertype:"string"`
	Tare         Tonnes `json:"tare" swaggertype:"string"`
	Net          Tonnes `json:"net" swaggertype:"string"`
	Adjustment   Tonnes `json:"adjustment" swaggertype:"string"`
}
type Summary struct {
	Rows       []SummaryLine `json:"rows"`
	TaskCount  int64         `json:"taskCount"`
	Gross      Tonnes        `json:"gross" swaggertype:"string"`
	Tare       Tonnes        `json:"tare" swaggertype:"string"`
	Net        Tonnes        `json:"net" swaggertype:"string"`
	Adjustment Tonnes        `json:"adjustment" swaggertype:"string"`
}
