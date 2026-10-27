package camera

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

var (
	// ErrInvalid and ErrConflict are transport-visible business errors. They
	// deliberately never contain source URLs or credentials.
	ErrInvalid  = errors.New("参数错误")
	ErrNotFound = errors.New("数据不存在")
	ErrConflict = errors.New("摄像头配置已变化，请刷新后重新核对")
)

const (
	defaultCaptureTimeoutMS = 6000
	minCaptureTimeoutMS     = 500
	maxCaptureTimeoutMS     = 60000
)

// cameraRecord is the repository row. Password is kept only for Server-side
// acquisition and is never copied into Camera.
type cameraRecord struct {
	ID             int64
	PublicID       string
	Name           string
	SourceURL      string
	Username       string
	Password       string
	CaptureEnabled bool
	Enabled        bool
	Deleted        bool
	Version        int64
	CreateTime     time.Time
	UpdateTime     time.Time
}

type streamRecord struct {
	CameraID       int64
	MediaPath      string
	DesiredVersion int64
	SyncedVersion  int64
	State          string
	LastError      string
	UpdatedAt      time.Time
	SyncedAt       *time.Time
	RemovedAt      *time.Time
}

type settingsRecord struct {
	ConfigVersion       int64
	CaptureTimeoutMS    int
	RecognitionURL      string
	RecognitionCameraID *int64
	MediaMTXAPIURL      string
	MediaMTXAPIUsername string
	MediaMTXAPIPassword string
	MediaMTXWHEPBaseURL string
	VideoVersion        int64
	VideoSyncState      string
	VideoSyncError      string
	VideoSyncedVersion  int64
	UpdateTime          time.Time
}

type imageRecord struct {
	ID             string
	ScaleCode      string
	CameraID       *int64
	CameraPublicID string
	CameraName     string
	CameraSource   string
	Source         string
	View           string
	Status         string
	Error          string
	FileName       string
	SHA256         string
	Width          int
	Height         int
	Bytes          int64
	CapturedAt     time.Time
	CreateTime     time.Time
}

// Store is the persistence seam used by the camera service. Implementations
// own transactions and optimistic version checks; the service owns validation,
// secret semantics and HTTP-facing read models.
type Store interface {
	ListState(context.Context) (settingsRecord, []cameraRecord, []streamRecord, error)
	Camera(context.Context, int64) (cameraRecord, error)
	CreateCamera(context.Context, cameraRecord, streamRecord, audit.Event) (int64, settingsRecord, error)
	UpdateCamera(context.Context, int64, int64, cameraRecord, bool, bool, audit.Event) (settingsRecord, error)
	DeleteCamera(context.Context, int64, int64, audit.Event) (settingsRecord, error)

	UpdateCaptureSettings(context.Context, int64, int, *int64, string, bool, audit.Event) (settingsRecord, error)
	UpdateVideoSettings(context.Context, int64, string, string, string, string, audit.Event) (settingsRecord, error)

	SaveImage(context.Context, imageRecord) error
	Image(context.Context, string) (imageRecord, error)

	SyncSnapshot(context.Context) (settingsRecord, []streamRecord, map[int64]cameraRecord, error)
	MarkStreamSynced(context.Context, int64, int64) error
	MarkStreamError(context.Context, int64, int64, string) error
	MarkStreamRemoved(context.Context, int64, int64) error
	UpdateVideoSync(context.Context, int64, string, string, bool) error

	RecordAudit(context.Context, audit.Event) error
}
