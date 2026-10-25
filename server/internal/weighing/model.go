// Package weighing owns server-backed testing workflows and their business records.
package weighing

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

var (
	ErrInvalid    = errors.New("参数错误")
	ErrNotFound   = errors.New("数据不存在")
	ErrConflict   = errors.New("资料已变化，请刷新后重新核对")
	ErrReferenced = errors.New("资料已引用，不能删除或移除正在使用的身份")
)

type MasterMeta struct {
	ID         int64     `json:"id" gorm:"primaryKey"`
	Enabled    int       `json:"enabled"`
	Version    int64     `json:"version"`
	CreateTime time.Time `json:"createTime" gorm:"autoCreateTime"`
	UpdateTime time.Time `json:"updateTime" gorm:"autoUpdateTime"`
	CreateBy   int64     `json:"-"`
	UpdateBy   int64     `json:"-"`
}
type Category struct {
	MasterMeta
	Name string `json:"name"`
}

func (Category) TableName() string { return "wb_material_category" }

type Material struct {
	MasterMeta
	Code       string `json:"code"`
	Name       string `json:"name"`
	CategoryID *int64 `json:"categoryId"`
	Spec       string `json:"spec"`
	Remark     string `json:"remark"`
}

func (Material) TableName() string { return "wb_material" }

type Party struct {
	MasterMeta
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	ShortName  string    `json:"shortName"`
	Nature     string    `json:"nature"`
	IsSupplier int       `json:"-"`
	IsCustomer int       `json:"-"`
	IsCarrier  int       `json:"-"`
	Phone      string    `json:"phone"`
	Address    string    `json:"address"`
	CreditCode string    `json:"creditCode"`
	Remark     string    `json:"remark"`
	Roles      []string  `json:"roles" gorm:"-"`
	Contacts   []Contact `json:"contacts" gorm:"-"`
}

func (Party) TableName() string { return "wb_party" }

type Contact struct {
	ID        int64  `json:"id" gorm:"primaryKey"`
	PartyID   int64  `json:"partyId"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Purpose   string `json:"purpose"`
	Remark    string `json:"remark"`
	IsDefault int    `json:"isDefault"`
}

func (Contact) TableName() string { return "wb_party_contact" }

type Scale struct {
	SiteID     int64  `json:"siteId"`
	SiteName   string `json:"siteName" gorm:"->"`
	CanOperate bool   `json:"canOperate" gorm:"-"`
	ID         int64  `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Enabled    int    `json:"enabled"`
}

func (Scale) TableName() string { return "wb_scale" }

type Page[T any] struct {
	Records  []T   `json:"records"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}
type PageQuery struct {
	Direction, Status, From, To string
	MaterialID, PartyID         *int64
	Page, PageSize              int
	Keyword, Role, Nature       string
	Enabled                     *int
	CategoryID                  *int64
	SiteID                      *int64
	TaskID                      *int64
}
type CategoryInput struct {
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled"`
	Version int64  `json:"version"`
}
type MaterialInput struct {
	CategoryInput
	CategoryID *int64 `json:"categoryId"`
	Spec       string `json:"spec"`
	Remark     string `json:"remark"`
}
type ContactInput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Purpose   string `json:"purpose"`
	Remark    string `json:"remark"`
	IsDefault bool   `json:"isDefault"`
}
type PartyInput struct {
	CategoryInput
	ShortName  string         `json:"shortName"`
	Nature     string         `json:"nature"`
	Roles      []string       `json:"roles"`
	Phone      string         `json:"phone"`
	Address    string         `json:"address"`
	CreditCode string         `json:"creditCode"`
	Remark     string         `json:"remark"`
	Contacts   []ContactInput `json:"contacts"`
}
type Store interface {
	SiteConfigurations(context.Context) ([]SiteConfiguration, error)
	AgentSettings(context.Context) (AgentSetting, error)
	SaveAgentSettings(context.Context, string, audit.Event) error
	SaveSiteConfiguration(context.Context, int64, SiteConfigurationInput, audit.Event, func(int64) bool) (SiteConfiguration, error)
	ExportAgentConnection(context.Context, int64, string, audit.Event) (agentconfig.Connection, error)

	RecordAgentKick(context.Context, audit.Event) error
	Sites(context.Context, int64) ([]Site, error)
	SiteOptions(context.Context) ([]Site, error)
	SiteGrants(context.Context, int64) ([]SiteGrant, error)
	SaveSiteGrants(context.Context, int64, []SiteGrant, audit.Event) error
	ResourceSite(context.Context, string, int64) (int64, error)

	TaskEvents(context.Context, int64, PageQuery) (Page[BusinessEvent], error)
	TasksAll(context.Context, PageQuery) ([]Task, error)
	Originals(context.Context, PageQuery) (Page[OriginalView], error)
	OriginalsAll(context.Context, PageQuery) ([]OriginalView, error)
	TicketsAll(context.Context, PageQuery) ([]Ticket, error)
	Summary(context.Context, string, PageQuery) (Summary, error)
	RecordExport(context.Context, string, PageQuery, int64, audit.Event) error

	PrintAttempts(context.Context, int64) ([]PrintAttempt, error)
	StartPrint(context.Context, int64, PrintInput, audit.Event) (CommandResult, error)
	ResolvePrint(context.Context, int64, PrintResultInput, audit.Event) (CommandResult, error)

	CorrectTask(context.Context, int64, CorrectionInput, audit.Event) (CommandResult, error)
	TerminateTask(context.Context, int64, TaskActionInput, audit.Event) (CommandResult, error)
	VoidTicket(context.Context, int64, VoidInput, audit.Event) (CommandResult, error)
	Reweigh(context.Context, int64, ReweighInput, audit.Event) (CommandResult, error)
	ReopenTicket(context.Context, int64, ReopenInput, audit.Event) (CommandResult, error)

	ConfirmSecond(context.Context, int64, ConfirmationInput, audit.Event) (CommandResult, error)
	FindTicket(context.Context, int64) (TicketView, error)
	Tasks(context.Context, PageQuery) (Page[Task], error)
	Tickets(context.Context, PageQuery) (Page[TicketView], error)

	Session(context.Context, int64) (ScaleSession, error)
	ChangeSession(context.Context, int64, string, SessionInput, audit.Event) (SessionResult, error)
	Capture(context.Context, CaptureInput, audit.Event) (Sample, error)
	Sample(context.Context, int64) (Sample, error)
	InvalidateSample(context.Context, int64, SessionInput, int64) error
	ConfirmFirst(context.Context, ConfirmationInput, audit.Event) (CommandResult, error)
	Operation(context.Context, int64, string) (Operation, error)
	FindTask(context.Context, int64) (TaskDetail, error)
	ActiveTask(context.Context, string) (*TaskDetail, error)

	Categories(context.Context, PageQuery) (Page[Category], error)
	Materials(context.Context, PageQuery) (Page[Material], error)
	Parties(context.Context, PageQuery) (Page[Party], error)
	FindCategory(context.Context, int64) (Category, error)
	FindMaterial(context.Context, int64) (Material, error)
	FindParty(context.Context, int64) (Party, error)
	Scales(context.Context) ([]Scale, error)
	SaveCategory(context.Context, int64, CategoryInput, audit.Event) (Category, error)
	SaveMaterial(context.Context, int64, MaterialInput, audit.Event) (Material, error)
	SaveParty(context.Context, int64, PartyInput, audit.Event) (Party, error)
	DeleteMaster(context.Context, string, int64, int64, audit.Event) error
	SetMasterStatus(context.Context, string, int64, int64, int, audit.Event) error
}
