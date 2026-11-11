package weighing

import (
	"encoding/json"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/tripsync"
)

// TripSelection is the operator's choice from the cloud candidate list. The
// server reloads the cloud detail before using it, so this is only an identity
// and version hint.
type TripSelection struct {
	TripNo  string `json:"tripNo"`
	Version int64  `json:"tripVersion"`
}

// TripBinding is the local authority for task/ticket/trip linkage. The JSON
// snapshots freeze what the first weigh saw, so the second weigh can work from
// local data even when the cloud is temporarily unreachable.
type TripBinding struct {
	ID                 int64      `json:"id" gorm:"primaryKey"`
	SiteCode           string     `json:"siteCode"`
	TripNo             string     `json:"tripNo"`
	CloudTripID        int64      `json:"cloudTripId"`
	TripVersion        int64      `json:"tripVersion"`
	Plate              string     `json:"plate"`
	MaterialCode       string     `json:"materialCode"`
	MaterialName       string     `json:"materialName"`
	PartyCode          string     `json:"partyCode"`
	PartyName          string     `json:"partyName"`
	TaskID             int64      `json:"taskId"`
	TicketID           *int64     `json:"ticketId"`
	InspectionID       int64      `json:"inspectionId"`
	InspectionVersion  int64      `json:"inspectionVersion"`
	InspectionSnapshot string     `json:"inspectionSnapshot"`
	DeductionSnapshot  string     `json:"deductionSnapshot"`
	TripSnapshot       string     `json:"tripSnapshot"`
	CloudTicketNo      string     `json:"cloudTicketNo"`
	Status             string     `json:"status"`
	OperationID        string     `json:"operationId"`
	CreatedBy          int64      `json:"createdBy"`
	CreatedAt          time.Time  `json:"createdAt"`
	TicketedAt         *time.Time `json:"ticketedAt"`
	LinkedAt           *time.Time `json:"linkedAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func (TripBinding) TableName() string { return "wb_trip_binding" }

type TaskTripBasis struct {
	Bound         bool                 `json:"bound"`
	Status        string               `json:"status,omitempty"`
	TripNo        string               `json:"tripNo,omitempty"`
	TicketID      *int64               `json:"ticketId,omitempty"`
	CloudTicketNo string               `json:"cloudTicketNo,omitempty"`
	Trip          *tripsync.Trip       `json:"trip,omitempty"`
	Inspection    *tripsync.Inspection `json:"inspection,omitempty"`
	Deductions    []tripsync.Deduction `json:"deductions"`
}

// TaskTripCloudStatus is the live cloud state for a bound task. It is a
// reminder only; the workbench must still allow a local second weigh when the
// cloud is unavailable.
type TaskTripCloudStatus struct {
	Bound             bool       `json:"bound"`
	TripNo            string     `json:"tripNo,omitempty"`
	Available         bool       `json:"available"`
	Message           string     `json:"message,omitempty"`
	Stage             string     `json:"stage,omitempty"`
	Linkable          bool       `json:"linkable"`
	Unloaded          bool       `json:"unloaded"`
	UnloadedAt        *time.Time `json:"unloadedAt,omitempty"`
	UnloadingLocation string     `json:"unloadingLocation,omitempty"`
	TicketNo          string     `json:"ticketNo,omitempty"`
	Blockers          []string   `json:"blockers"`
}

func encodeSnapshot(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}
