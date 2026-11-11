// Package tripsync reads the cloud grain trip and its effective inspection.
package tripsync

import "time"

type Inspection struct {
	ID          int64      `json:"id"`
	Version     int64      `json:"version"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	Moisture    string     `json:"moisture"`
	Impurity    string     `json:"impurity"`
	Reason      string     `json:"reason"`
	ConfirmedAt *time.Time `json:"confirmedAt"`
}

// Deduction is one resolved cloud rule. Percent deductions keep their rate so
// the weighing server can apply it to the final net weight.
type Deduction struct {
	RuleID           int64  `json:"ruleId"`
	RuleCode         string `json:"ruleCode"`
	RuleVersion      int64  `json:"ruleVersion"`
	CategoryCode     string `json:"categoryCode"`
	CategoryName     string `json:"categoryName"`
	Mode             string `json:"mode"`
	BasisField       string `json:"basisField"`
	BasisValue       string `json:"basisValue"`
	AmountMilliTonne int64  `json:"amountMilliTonne"`
	RateBP           *int   `json:"rateBp"`
	Explanation      string `json:"explanation"`
}

type Trip struct {
	TripNo              string      `json:"tripNo"`
	TripID              int64       `json:"tripId"`
	Version             int64       `json:"version"`
	Stage               string      `json:"stage"`
	Plate               string      `json:"plate"`
	MaterialCode        string      `json:"materialCode"`
	MaterialName        string      `json:"materialName"`
	PartyCode           string      `json:"partyCode"`
	PartyName           string      `json:"partyName"`
	Driver              string      `json:"driver"`
	DriverPhone         string      `json:"driverPhone"`
	ContractNo          string      `json:"contractNo"`
	InspectionMethod    string      `json:"inspectionMethod"`
	Inspection          *Inspection `json:"inspection"`
	EffectiveConclusion string      `json:"effectiveConclusion"`
	RegisteredAt        time.Time   `json:"registeredAt"`
	UnloadedAt          *time.Time  `json:"unloadedAt"`
	UnloadingLocation   string      `json:"unloadingLocation"`
	TicketNo            string      `json:"ticketNo"`
	Linkable            bool        `json:"linkable"`
	Blockers            []string    `json:"blockers"`
	Deductions          []Deduction `json:"deductions"`
}
