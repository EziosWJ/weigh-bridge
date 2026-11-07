package weighing

import "time"

// ticketSyncChangeV5 is the persisted v5 outbox row. Payload and digest are
// written once in the business transaction and never regenerated on retry.
type ticketSyncChangeV5 struct {
	ID               int64      `gorm:"primaryKey"`
	TicketID         int64      `gorm:"column:ticket_id"`
	PreviousTicketID *int64     `gorm:"column:previous_ticket_id"`
	Digest           string     `gorm:"column:digest"`
	DigestVersion    int        `gorm:"column:digest_version"`
	SchemaVersion    int        `gorm:"column:schema_version"`
	SiteCode         string     `gorm:"column:site_code"`
	TicketNo         string     `gorm:"column:ticket_no"`
	ChangeType       string     `gorm:"column:change_type"`
	Payload          string     `gorm:"column:payload"`
	Status           string     `gorm:"column:status"`
	Category         string     `gorm:"column:category"`
	ReasonCode       string     `gorm:"column:reason_code"`
	Reason           string     `gorm:"column:reason"`
	Attempts         int        `gorm:"column:attempts"`
	NextAttemptAt    *time.Time `gorm:"column:next_attempt_at"`
	LastAttemptAt    *time.Time `gorm:"column:last_attempt_at"`
	SyncedAt         *time.Time `gorm:"column:synced_at"`
	ReceivedAt       *time.Time `gorm:"column:received_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
}

func (ticketSyncChangeV5) TableName() string { return "wb_ticket_sync_change_v5" }
