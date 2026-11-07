package weighing

import "time"

type TaskActionInput struct {
	RequestID string `json:"requestId"`
	Version   int64  `json:"version"`
	Reason    string `json:"reason"`
}
type CorrectionInput struct {
	TaskActionInput
	Business BusinessInput `json:"business"`
}

// ManualReturnInput records the second weighing without a device sample. The
// weight is the operator's tonne value, ReturnedAt is the actual return time
// and is validated against the first weighing and server time.
type ManualReturnInput struct {
	RequestID  string    `json:"requestId"`
	Version    int64     `json:"version"`
	Weight     Tonnes    `json:"weight" swaggertype:"string"`
	ReturnedAt time.Time `json:"returnedAt"`
	Reason     string    `json:"reason"`
	// NetAdjustments follows the multi-item net adjustment of the issued
	// ticket, with the same rules as a normal second confirmation.
	NetAdjustments NetAdjustmentsInput `json:"netAdjustments"`
}
type VoidInput struct {
	RequestID string `json:"requestId"`
	Reason    string `json:"reason"`
}
type ReweighInput struct {
	ConfirmationInput
	SameLoadConfirmed bool   `json:"sameLoadConfirmed"`
	OtherBasisValid   bool   `json:"otherBasisValid"`
	Reason            string `json:"reason"`
}
type ReopenInput struct {
	RequestID      string              `json:"requestId"`
	TaskVersion    int64               `json:"taskVersion"`
	Reason         string              `json:"reason"`
	Business       BusinessInput       `json:"business"`
	BasisValid     bool                `json:"basisValid"`
	FinalWeights   *FinalWeights       `json:"finalWeights"`
	NetAdjustment  *NetAdjustmentInput `json:"netAdjustment"`
	NetAdjustments NetAdjustmentsInput `json:"netAdjustments"`
	AllowAdjust    bool                `json:"-"`
	AllowCorrect   bool                `json:"-"`
}
