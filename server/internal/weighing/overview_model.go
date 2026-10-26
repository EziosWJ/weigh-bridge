package weighing

import "time"

type OverviewDirection struct {
	TaskCount int64  `json:"taskCount"`
	Net       Tonnes `json:"net" swaggertype:"string"`
}

type OverviewTask struct {
	Task
	FirstConfirmedAt time.Time `json:"firstConfirmedAt"`
}

type Overview struct {
	Date           string            `json:"date"`
	CheckedAt      time.Time         `json:"checkedAt"`
	ValidTickets   int64             `json:"validTickets"`
	Inbound        OverviewDirection `json:"inbound"`
	Outbound       OverviewDirection `json:"outbound"`
	AwaitingCount  int64             `json:"awaitingCount"`
	OvernightCount int64             `json:"overnightCount"`
	AwaitingTasks  []OverviewTask    `json:"awaitingTasks"`
	RecentTickets  []TicketView      `json:"recentTickets"`
}
