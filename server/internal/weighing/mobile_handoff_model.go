package weighing

import "time"

const (
	MobileHandoffClaimed   = "claimed"
	MobileHandoffCompleted = "completed"
	MobileHandoffFailed    = "failed"
)

// MobileHandoff is the factory-side idempotency and feedback record for one
// cloud mobile QR. The actual first/second records live in the normal task and
// ticket tables.
type MobileHandoff struct {
	ID             int64      `json:"id" gorm:"primaryKey"`
	HandoffID      int64      `json:"handoffId"`
	WeighingID     int64      `json:"weighingId"`
	Action         string     `json:"action"`
	SiteCode       string     `json:"siteCode"`
	ScaleCode      string     `json:"scaleCode"`
	Plate          string     `json:"plate"`
	MobileAccount  string     `json:"mobileAccount"`
	Source         string     `json:"source"`
	Device         string     `json:"device"`
	Status         string     `json:"status"`
	TaskID         *int64     `json:"taskId,omitempty"`
	TicketID       *int64     `json:"ticketId,omitempty"`
	FirstRecordID  *int64     `json:"firstRecordId,omitempty"`
	SecondRecordID *int64     `json:"secondRecordId,omitempty"`
	Result         JSON       `json:"result,omitempty" swaggertype:"object"`
	Error          string     `json:"error"`
	ClaimedAt      time.Time  `json:"claimedAt"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

func (MobileHandoff) TableName() string { return "wb_mobile_handoff" }

// MobileHandoffResult is returned to the scanner and stored in the local
// handoff row. Weights are the factory-captured values, not phone previews.
type MobileHandoffResult struct {
	HandoffID     int64      `json:"handoffId"`
	WeighingID    int64      `json:"weighingId"`
	Action        string     `json:"action"`
	Status        string     `json:"status"`
	TaskNo        string     `json:"taskNo,omitempty"`
	TicketNo      string     `json:"ticketNo,omitempty"`
	FirstWeight   Tonnes     `json:"firstWeight,omitempty" swaggertype:"string"`
	SecondWeight  Tonnes     `json:"secondWeight,omitempty" swaggertype:"string"`
	Net           Tonnes     `json:"net,omitempty" swaggertype:"string"`
	FirstAt       *time.Time `json:"firstAt,omitempty"`
	SecondAt      *time.Time `json:"secondAt,omitempty"`
	CaptureStatus string     `json:"captureStatus,omitempty"`
	Message       string     `json:"message,omitempty"`
}

type mobileClaimResponse struct {
	HandoffID             int64     `json:"handoffId"`
	WeighingID            int64     `json:"weighingId"`
	Action                string    `json:"action"`
	Status                string    `json:"status"`
	Plate                 string    `json:"plate"`
	PartyCode             string    `json:"partyCode"`
	PartyName             string    `json:"partyName"`
	MaterialCode          string    `json:"materialCode"`
	MobileAccount         string    `json:"mobileAccount"`
	SiteCode              string    `json:"siteCode"`
	ScaleCode             string    `json:"scaleCode"`
	FactoryTaskNo         string    `json:"factoryTaskNo,omitempty"`
	FirstWeightMilliTonne int64     `json:"firstWeightMilliTonne,omitempty"`
	FirstAt               time.Time `json:"firstAt,omitempty"`
	ExpiresAt             time.Time `json:"expiresAt"`
}

type mobileResultRequest struct {
	Token                  string    `json:"token"`
	SiteCode               string    `json:"siteCode"`
	ScaleCode              string    `json:"scaleCode"`
	Action                 string    `json:"action"`
	Status                 string    `json:"status"`
	FactoryTaskNo          string    `json:"factoryTaskNo,omitempty"`
	FactoryTicketNo        string    `json:"factoryTicketNo,omitempty"`
	FirstWeightMilliTonne  int64     `json:"firstWeightMilliTonne,omitempty"`
	FirstAt                time.Time `json:"firstAt,omitempty"`
	SecondWeightMilliTonne int64     `json:"secondWeightMilliTonne,omitempty"`
	SecondAt               time.Time `json:"secondAt,omitempty"`
	NetMilliTonne          int64     `json:"netMilliTonne,omitempty"`
	CaptureStatus          string    `json:"captureStatus,omitempty"`
	Message                string    `json:"message,omitempty"`
}

type mobileTerminateRequest struct {
	SiteCode      string `json:"siteCode"`
	FactoryTaskNo string `json:"factoryTaskNo"`
	Reason        string `json:"reason"`
}

type MobileHandoffScanInput struct {
	Code string `json:"code"`
}
