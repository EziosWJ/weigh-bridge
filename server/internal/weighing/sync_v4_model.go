package weighing

import (
	"errors"
	"time"
)

// Digest-protocol (schemaVersion 4) constants. The digest is computed by the
// factory and is the change identity; the cloud stores it and never recomputes.
const (
	SyncSchemaVersionV4 = 4
	SyncDigestVersion2  = 2
	MaxBatchReconcile   = 200
)

const (
	SyncCategoryTemporary          = "temporary"
	SyncCategoryPredecessorPending = "predecessor_pending"
	SyncCategoryConfigOrData       = "config_or_data"
	SyncCategoryConflict           = "conflict"
)

const (
	SyncChangePending            = "pending"
	SyncChangeWaitingPredecessor = "waiting_predecessor"
	SyncChangeTemporaryFailure   = "temporary_failure"
	SyncChangeNeedsAdmin         = "needs_admin"
	SyncChangeConflict           = "conflict"
	SyncChangeSynced             = "synced"
	SyncReconcileConsistent      = "consistent"
	SyncReconcilePending         = "pending"
	SyncReconcileNeedsReview     = "needs_review"
	SyncReconcileConflict        = "conflict"
	SyncReconcileUnknown         = "unknown"
	SyncReviewOpen               = "open"
	SyncReviewResolved           = "resolved"
)

// SyncCloudError is the machine-readable rejection returned by the cloud.
type SyncCloudError struct {
	Category  string `json:"category"`
	Reason    string `json:"reason"`
	Retryable bool   `json:"retryable"`
	Message   string `json:"message"`
}

func (e *SyncCloudError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "云端未确认该变化"
}

// syncChangeV4 is the request body of one issue, void or reopen change.
type syncChangeV4 struct {
	SchemaVersion    int         `json:"schemaVersion"`
	DigestVersion    int         `json:"digestVersion"`
	Digest           string      `json:"digest"`
	SiteCode         string      `json:"siteCode"`
	TicketNo         string      `json:"ticketNo"`
	Type             string      `json:"type"`
	PreviousSiteCode string      `json:"previousSiteCode,omitempty"`
	Ticket           *syncTicket `json:"ticket,omitempty"`
	Void             *syncVoid   `json:"void,omitempty"`
}

type syncChangeV4Receipt struct {
	Digest     string    `json:"digest"`
	TicketNo   string    `json:"ticketNo"`
	ChangeType string    `json:"changeType"`
	ReceivedAt time.Time `json:"receivedAt"`
	Duplicate  bool      `json:"duplicate"`
}

type syncConfirmation struct {
	Digest        string    `json:"digest"`
	TicketNo      string    `json:"ticketNo"`
	ChangeType    string    `json:"changeType"`
	DigestVersion int       `json:"digestVersion"`
	SchemaVersion int       `json:"schemaVersion"`
	ReceivedAt    time.Time `json:"receivedAt"`
}

type syncTicketRef struct {
	SiteCode string `json:"siteCode"`
	TicketNo string `json:"ticketNo"`
}

type syncCloudTicket struct {
	SiteCode            string             `json:"siteCode"`
	TicketNo            string             `json:"ticketNo"`
	Found               bool               `json:"found"`
	Status              string             `json:"status"`
	Ticket              *syncTicket        `json:"ticket"`
	VoidReason          *string            `json:"voidReason"`
	VoidedBy            *string            `json:"voidedBy"`
	VoidedAt            *time.Time         `json:"voidedAt"`
	LatestDigest        *string            `json:"latestDigest"`
	LatestDigestVersion *int               `json:"latestDigestVersion"`
	LatestChangeType    *string            `json:"latestChangeType"`
	LatestReceivedAt    *time.Time         `json:"latestReceivedAt"`
	PreviousSiteCode    *string            `json:"previousSiteCode"`
	PreviousTicketNo    *string            `json:"previousTicketNo"`
	ReopenedBy          *syncTicketRef     `json:"reopenedBy"`
	Changes             []syncConfirmation `json:"changes"`
	Error               *SyncCloudError    `json:"error"`
}

type syncCloudBatch struct {
	Items []syncCloudTicket `json:"items"`
}

type syncCloudLookup struct {
	Found        bool              `json:"found"`
	SiteCode     string            `json:"siteCode"`
	Digest       string            `json:"digest"`
	Confirmation *syncConfirmation `json:"confirmation"`
}

// ticketSyncChangeV4 is the persisted outbox row. Payload and digest are
// written once in the business transaction and never regenerated on retry.
type ticketSyncChangeV4 struct {
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

func (ticketSyncChangeV4) TableName() string { return "wb_ticket_sync_change_v4" }

type ticketReconcile struct {
	TicketID                 int64      `gorm:"primaryKey"`
	SiteCode                 string     `gorm:"column:site_code"`
	TicketNo                 string     `gorm:"column:ticket_no"`
	Status                   string     `gorm:"column:status"`
	CloudFound               *bool      `gorm:"column:cloud_found"`
	CloudStatus              *string    `gorm:"column:cloud_status"`
	LocalLatestDigest        *string    `gorm:"column:local_latest_digest"`
	CloudLatestDigest        *string    `gorm:"column:cloud_latest_digest"`
	CloudLatestDigestVersion *int       `gorm:"column:cloud_latest_digest_version"`
	CloudLatestChangeType    *string    `gorm:"column:cloud_latest_change_type"`
	CloudReceivedAt          *time.Time `gorm:"column:cloud_received_at"`
	Detail                   string     `gorm:"column:detail"`
	CheckedAt                time.Time  `gorm:"column:checked_at"`
	TriggerType              string     `gorm:"column:trigger_type"`
}

func (ticketReconcile) TableName() string { return "wb_ticket_reconcile" }

type ticketSyncReview struct {
	ID              int64      `gorm:"primaryKey"`
	TicketID        int64      `gorm:"column:ticket_id"`
	Digest          string     `gorm:"column:digest"`
	ReasonCode      string     `gorm:"column:reason_code"`
	Status          string     `gorm:"column:status"`
	Blocking        bool       `gorm:"column:blocking"`
	Detail          string     `gorm:"column:detail"`
	Conclusion      string     `gorm:"column:conclusion"`
	ConcludedBy     *int64     `gorm:"column:concluded_by"`
	ConcludedByName string     `gorm:"column:concluded_by_name"`
	ConcludedAt     *time.Time `gorm:"column:concluded_at"`
	LastCheckedAt   time.Time  `gorm:"column:last_checked_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (ticketSyncReview) TableName() string { return "wb_ticket_sync_review" }

type syncReconcileState struct {
	ID              int16      `gorm:"primaryKey"`
	LastAutoDate    *time.Time `gorm:"column:last_auto_date"`
	LastAutoSuccess bool       `gorm:"column:last_auto_success"`
	LastRunAt       *time.Time `gorm:"column:last_run_at"`
	LastStatus      string     `gorm:"column:last_status"`
	LastChecked     int        `gorm:"column:last_checked"`
	LastConsistent  int        `gorm:"column:last_consistent"`
	LastPending     int        `gorm:"column:last_pending"`
	LastNeedsReview int        `gorm:"column:last_needs_review"`
	LastConflict    int        `gorm:"column:last_conflict"`
	LastUnknown     int        `gorm:"column:last_unknown"`
	LastReason      string     `gorm:"column:last_reason"`
}

func (syncReconcileState) TableName() string { return "wb_sync_reconcile_state" }

var errSyncNotTracked = errors.New("该单据未纳入摘要同步")
