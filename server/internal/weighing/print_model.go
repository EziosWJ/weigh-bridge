package weighing

import "time"

type PrintAttempt struct {
	ID               int64      `json:"id" gorm:"primaryKey"`
	TicketID         int64      `json:"ticketId"`
	RequestID        string     `json:"requestId"`
	AttemptNo        int64      `json:"attemptNo"`
	Intent           string     `json:"intent"`
	Status           string     `json:"status"`
	Reason           string     `json:"reason"`
	InitiatedAt      time.Time  `json:"initiatedAt"`
	InitiatedBy      int64      `json:"initiatedBy"`
	ResolvedAt       *time.Time `json:"resolvedAt"`
	ResolvedBy       *int64     `json:"resolvedBy"`
	ResolutionSource string     `json:"resolutionSource"`
	ClosedAt         *time.Time `json:"closedAt"`
	CloseReason      string     `json:"closeReason"`
}

func (PrintAttempt) TableName() string { return "wb_print_attempt" }

type PrintInput struct {
	RequestID    string `json:"requestId"`
	Intent       string `json:"intent"`
	Reason       string `json:"reason"`
	AllowReprint bool   `json:"-"`
}
type PrintResultInput struct {
	RequestID string `json:"requestId"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}
