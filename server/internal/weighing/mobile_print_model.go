package weighing

import "time"

// MobilePrintCommand is a cloud-queued print request pulled by the factory LAN.
// Origin distinguishes the completion page (immediate first print) from a
// history/reprint request that requires a scanner proof.
type MobilePrintCommand struct {
	ID             int64     `json:"id"`
	RequestID      string    `json:"requestId"`
	SiteCode       string    `json:"siteCode"`
	TicketNo       string    `json:"ticketNo"`
	WeighingID     int64     `json:"weighingId"`
	DriverName     string    `json:"driverName"`
	Origin         string    `json:"origin"`
	Action         string    `json:"action"`
	Reason         string    `json:"reason"`
	ScanVerified   bool      `json:"scanVerified"`
	ScanTokenHash  string    `json:"scanTokenHash"`
	RetryAsReprint bool      `json:"retryAsReprint"`
	VerifyResult   string    `json:"verifyResult"`
	FactoryAttempt int64     `json:"factoryAttemptId,omitempty"`
	FactoryIntent  string    `json:"factoryIntent,omitempty"`
	Status         string    `json:"status"`
	Message        string    `json:"message"`
	CreatedAt      time.Time `json:"createdAt"`
}

type MobilePrintClaimRequest struct {
	SiteCode string `json:"siteCode"`
}

type MobilePrintScanRequest struct {
	SiteCode string `json:"siteCode"`
	Token    string `json:"token"`
}

type MobilePrintResultRequest struct {
	SiteCode       string `json:"siteCode"`
	Status         string `json:"status"`
	FactoryAttempt int64  `json:"factoryAttemptId"`
	FactoryIntent  string `json:"factoryIntent"`
	Message        string `json:"message"`
}

type PrintAttemptSync struct {
	SiteCode       string     `json:"siteCode"`
	FactoryAttempt int64      `json:"factoryAttemptId"`
	TicketNo       string     `json:"ticketNo"`
	AttemptNo      int64      `json:"attemptNo"`
	Intent         string     `json:"intent"`
	Status         string     `json:"status"`
	Source         string     `json:"source"`
	ActorName      string     `json:"actorName"`
	Reason         string     `json:"reason"`
	ScanVerified   bool       `json:"scanVerified"`
	CloudRequestID string     `json:"cloudRequestId"`
	InitiatedAt    time.Time  `json:"initiatedAt"`
	ResolvedAt     *time.Time `json:"resolvedAt"`
	Resolution     string     `json:"resolutionSource"`
	CloseReason    string     `json:"closeReason"`
}
