package weighing

type TaskActionInput struct {
	RequestID string `json:"requestId"`
	Version   int64  `json:"version"`
	Reason    string `json:"reason"`
}
type CorrectionInput struct {
	TaskActionInput
	Business BusinessInput `json:"business"`
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
	RequestID     string              `json:"requestId"`
	TaskVersion   int64               `json:"taskVersion"`
	Reason        string              `json:"reason"`
	Business      BusinessInput       `json:"business"`
	BasisValid    bool                `json:"basisValid"`
	FinalWeights  *FinalWeights       `json:"finalWeights"`
	NetAdjustment *NetAdjustmentInput `json:"netAdjustment"`
	AllowAdjust   bool                `json:"-"`
	AllowCorrect  bool                `json:"-"`
}
