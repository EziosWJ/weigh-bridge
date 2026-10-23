package weighing

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"strings"
	"time"
)

var (
	ErrCapturePolicyChanged = errors.New("强制抓拍配置已变更，请重新采集")
	ErrConditions           = errors.New("采集条件已失效，请重新采集并核对")
	ErrOccupied             = errors.New("地磅已被其他工作台占用")
	ErrSession              = errors.New("地磅操作会话已失效，请重新取得操作权")
	ErrRequestKey           = errors.New("同一请求编号不能用于不同操作或内容")
	ErrWeight               = errors.New("毛重必须大于皮重，原始与最终净重均须大于零")
)

// Tonnes is an exact thousandth of a tonne shared with the Agent contract.
type Tonnes = agentproto.Weight

// WeightDifference is a signed, read-only derived quantity, unlike input weights.
type WeightDifference Tonnes

func (v WeightDifference) MarshalJSON() ([]byte, error) { return Tonnes(v).MarshalJSON() }
func (v *WeightDifference) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return ErrInvalid
	}
	negative := strings.HasPrefix(text, "-")
	if negative {
		text = strings.TrimPrefix(text, "-")
	}
	encoded, _ := json.Marshal(text)
	var value Tonnes
	if err := value.UnmarshalJSON(encoded); err != nil {
		return err
	}
	if negative {
		value = -value
	}
	*v = WeightDifference(value)
	return nil
}

type JSON json.RawMessage

func (j *JSON) UnmarshalJSON(data []byte) error {
	if !json.Valid(data) {
		return ErrInvalid
	}
	*j = append((*j)[:0], data...)
	return nil
}

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "null", nil
	}
	return string(j), nil
}
func (j *JSON) Scan(v any) error {
	switch value := v.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], value...)
	case string:
		*j = append((*j)[:0], value...)
	default:
		return fmt.Errorf("unsupported JSON storage value %T", v)
	}
	return nil
}
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}
func snapshot(value any) JSON { data, _ := json.Marshal(value); return JSON(data) }

type BusinessInput struct {
	Plate        string `json:"plate"`
	Direction    string `json:"direction"`
	MaterialID   int64  `json:"materialId"`
	PartyID      int64  `json:"partyId"`
	CarrierID    *int64 `json:"carrierId"`
	ContactName  string `json:"contactName"`
	ContactPhone string `json:"contactPhone"`
	Driver       string `json:"driver"`
	DriverPhone  string `json:"driverPhone"`
	Remark       string `json:"remark"`
}
type BusinessInfo struct {
	MaterialVersion int64 `json:"materialVersion"`
	PartyVersion    int64 `json:"partyVersion"`
	CarrierVersion  int64 `json:"carrierVersion"`
	BusinessInput
	MaterialCode         string `json:"materialCode"`
	MaterialName         string `json:"materialName"`
	MaterialSpec         string `json:"materialSpec"`
	MaterialCategoryID   *int64 `json:"materialCategoryId"`
	MaterialCategoryName string `json:"materialCategoryName"`
	PartyCode            string `json:"partyCode"`
	PartyName            string `json:"partyName"`
	PartyNature          string `json:"partyNature"`
	PartyPhone           string `json:"partyPhone"`
	PartyAddress         string `json:"partyAddress"`
	CarrierCode          string `json:"carrierCode"`
	CarrierName          string `json:"carrierName"`
}
type ScaleSession struct {
	ScaleID      int64      `json:"scaleId" gorm:"primaryKey;autoIncrement:false"`
	HolderUserID *int64     `json:"holderUserId"`
	HolderName   string     `json:"holderName"`
	ClientID     string     `json:"clientId"`
	TokenHash    string     `json:"-"`
	Generation   int64      `json:"generation"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	Version      int64      `json:"version"`
}

func (ScaleSession) TableName() string { return "wb_scale_session" }

type SessionInput struct {
	ClientID     string `json:"clientId"`
	SessionToken string `json:"sessionToken"`
	Reason       string `json:"reason"`
}
type SessionResult struct {
	ScaleSession
	SessionToken string `json:"sessionToken,omitempty"`
}
type Sample struct {
	ID                int64        `json:"id" gorm:"primaryKey"`
	ScaleID           int64        `json:"scaleId"`
	OwnerUserID       int64        `json:"-"`
	ClientID          string       `json:"-"`
	SessionGeneration int64        `json:"-"`
	TaskID            *int64       `json:"taskId"`
	TargetRecordID    *int64       `json:"targetRecordId"`
	TaskVersion       *int64       `json:"taskVersion"`
	BusinessHash      string       `json:"businessHash"`
	Business          BusinessInfo `json:"business" gorm:"column:business_snapshot;serializer:json"`
	Stage             string       `json:"stage"`
	Kind              string       `json:"kind"`
	Plate             string       `json:"plate"`
	Source            string       `json:"source"`
	Weight            Tonnes       `json:"weight" gorm:"column:weight_milli_tonne" swaggertype:"string"`
	CapturedAt        time.Time    `json:"capturedAt"`
	ExpiresAt         *time.Time   `json:"expiresAt"` // Legacy field; new samples have no time limit.
	Stable            int          `json:"stable"`
	DeviceOK          int          `json:"deviceOk"`
	Evidence          JSON         `json:"evidence" gorm:"column:evidence_snapshot" swaggertype:"object"`
	PlateEvidence     JSON         `json:"plateEvidence" swaggertype:"object"`
	DeviceRevision    int64        `json:"-" gorm:"column:conditions_version"`
	InvalidatedAt     *time.Time   `json:"invalidatedAt"`
}

func (Sample) TableName() string { return "wb_sample" }

type RecognitionInput struct {
	Source     string `json:"source"`
	ScenePlate string `json:"scenePlate"`
	Candidate  string `json:"candidate"`
	At         string `json:"at"`
}
type CaptureInput struct {
	PlateEvidence *RecognitionInput `json:"plateEvidence"`
	ScaleID       int64             `json:"scaleId"`
	SessionInput
	TaskID         *int64        `json:"taskId"`
	TargetRecordID *int64        `json:"targetRecordId"`
	Business       BusinessInput `json:"business"`
}
type Task struct {
	SiteID                int64        `json:"siteId"`
	FirstWeight           *Tonnes      `json:"firstWeight" gorm:"->" swaggertype:"string"`
	SecondWeight          *Tonnes      `json:"secondWeight" gorm:"->" swaggertype:"string"`
	ID                    int64        `json:"id" gorm:"primaryKey"`
	TaskNo                string       `json:"taskNo"`
	Mode                  string       `json:"mode"`
	Plate                 string       `json:"plate"`
	Direction             string       `json:"direction"`
	Status                string       `json:"status"`
	ScaleID               int64        `json:"scaleId"`
	MaterialID            int64        `json:"materialId"`
	PartyID               int64        `json:"partyId"`
	CarrierID             *int64       `json:"carrierId"`
	Business              BusinessInfo `json:"business" gorm:"column:business_snapshot;serializer:json"`
	CurrentFirstRecordID  *int64       `json:"currentFirstRecordId"`
	CurrentSecondRecordID *int64       `json:"currentSecondRecordId"`
	Version               int64        `json:"version"`
	CreateTime            time.Time    `json:"createTime"`
	UpdateTime            time.Time    `json:"updateTime"`
	ActorID               int64        `json:"actorId"`
	ActorName             string       `json:"actorName"`
	CompletedAt           *time.Time   `json:"completedAt"`
	TerminatedAt          *time.Time   `json:"terminatedAt"`
	TerminateReason       string       `json:"terminateReason"`
}

func (Task) TableName() string { return "wb_transport_task" }

type WeighRecord struct {
	ID                  int64     `json:"id" gorm:"primaryKey"`
	TaskID              int64     `json:"taskId"`
	SampleID            int64     `json:"sampleId"`
	Stage               string    `json:"stage"`
	Kind                string    `json:"kind"`
	Weight              Tonnes    `json:"weight" gorm:"column:weight_milli_tonne" swaggertype:"string"`
	CapturedAt          time.Time `json:"capturedAt"`
	ConfirmedAt         time.Time `json:"confirmedAt"`
	ScaleID             int64     `json:"scaleId"`
	ReplacesRecordID    *int64    `json:"replacesRecordId"`
	RetainedRecordID    *int64    `json:"retainedRecordId"`
	Reason              string    `json:"reason"`
	SameLoadConfirmed   int       `json:"sameLoadConfirmed"`
	OtherBasisConfirmed int       `json:"otherBasisConfirmed"`
	ActorID             int64     `json:"actorId"`
	ActorName           string    `json:"actorName"`
	Evidence            JSON      `json:"evidence" gorm:"column:evidence_snapshot" swaggertype:"object"`
	PlateEvidence       JSON      `json:"plateEvidence" swaggertype:"object"`
}

func (WeighRecord) TableName() string { return "wb_weigh_record" }

type Operation struct {
	ID              int64     `json:"id" gorm:"primaryKey"`
	ActorID         int64     `json:"-"`
	RequestID       string    `json:"requestId"`
	OriginRequestID string    `json:"originRequestId,omitempty"`
	Command         string    `json:"command"`
	RequestHash     string    `json:"-"`
	Status          string    `json:"status"`
	TaskID          *int64    `json:"taskId"`
	TicketID        *int64    `json:"ticketId"`
	Result          JSON      `json:"result" gorm:"column:result_snapshot" swaggertype:"object"`
	ErrorSnapshot   string    `json:"error"`
	CommittedAt     time.Time `json:"committedAt"`
}

func (Operation) TableName() string { return "wb_operation" }

type BusinessEvent struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	OperationID  *int64    `json:"operationId"`
	TaskID       *int64    `json:"taskId"`
	TicketID     *int64    `json:"ticketId"`
	SampleID     *int64    `json:"sampleId"`
	ResourceType string    `json:"resourceType"`
	ResourceID   *int64    `json:"resourceId"`
	Action       string    `json:"action"`
	Reason       string    `json:"reason"`
	Before       JSON      `json:"before" gorm:"column:before_snapshot" swaggertype:"object"`
	After        JSON      `json:"after" gorm:"column:after_snapshot" swaggertype:"object"`
	ActorID      int64     `json:"actorId"`
	ActorName    string    `json:"actorName"`
	OccurredAt   time.Time `json:"occurredAt"`
}

func (BusinessEvent) TableName() string { return "wb_business_event" }

type Checks struct {
	Plate        bool `json:"plate"`
	Business     bool `json:"business"`
	Weight       bool `json:"weight"`
	Evidence     bool `json:"evidence"`
	FullyOnScale bool `json:"fullyOnScale"`
}

func (c Checks) Valid() bool {
	return c.Plate && c.Business && c.Weight && c.Evidence && c.FullyOnScale
}

type ReviewedVersions struct {
	Material int64 `json:"material"`
	Party    int64 `json:"party"`
	Carrier  int64 `json:"carrier"`
}
type ConfirmationInput struct {
	ReviewedVersions *ReviewedVersions `json:"reviewedVersions"`
	AllowAdjust      bool              `json:"-"`
	CaptureInput
	RequestID    string        `json:"requestId"`
	SampleID     int64         `json:"sampleId"`
	Version      int64         `json:"version"`
	Checks       Checks        `json:"checks"`
	FinalWeights *FinalWeights `json:"finalWeights"`
}
type FinalWeights struct {
	Gross  Tonnes `json:"gross" swaggertype:"string"`
	Tare   Tonnes `json:"tare" swaggertype:"string"`
	Remark string `json:"remark"`
}
type CommandResult struct {
	PrintAttemptID int64  `json:"printAttemptId,omitempty"`
	TaskID         int64  `json:"taskId"`
	RecordID       int64  `json:"recordId,omitempty"`
	TicketID       int64  `json:"ticketId,omitempty"`
	TaskVersion    int64  `json:"taskVersion"`
	Status         string `json:"status"`
}
type TaskDetail struct {
	Task
	Records []WeighRecord   `json:"records"`
	Events  []BusinessEvent `json:"events"`
}

type RejectionError string

func (e RejectionError) Error() string { return string(e) }
func isRejection(err error) bool       { var rejected RejectionError; return errors.As(err, &rejected) }
