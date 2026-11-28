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
	Source           string     `json:"source"`
	ActorName        string     `json:"actorName"`
	ScanVerified     bool       `json:"scanVerified"`
	ScanTokenHash    string     `json:"-"`
	CloudRequestID   string     `json:"cloudRequestId,omitempty"`
}

func (PrintAttempt) TableName() string { return "wb_print_attempt" }

type PrintInput struct {
	RequestID      string `json:"requestId"`
	Intent         string `json:"intent"`
	Reason         string `json:"reason"`
	AllowReprint   bool   `json:"-"`
	Source         string `json:"-"`
	ActorName      string `json:"-"`
	ScanVerified   bool   `json:"-"`
	ScanTokenHash  string `json:"-"`
	CloudRequestID string `json:"-"`
}
type PrintResultInput struct {
	RequestID        string `json:"requestId"`
	Status           string `json:"status"`
	Reason           string `json:"reason"`
	ResolutionSource string `json:"-"`
	ActorName        string `json:"-"`
}

// PrintSyncOutbox is a per-attempt dirty marker. Any create/result/resolve
// reopens it so the factory eventually pushes the latest state to the cloud.
type PrintSyncOutbox struct {
	AttemptID int64 `gorm:"primaryKey"`
	SyncedAt  *time.Time
	Attempts  int
	LastError string
	UpdatedAt time.Time
}

func (PrintSyncOutbox) TableName() string { return "wb_print_sync_outbox" }
