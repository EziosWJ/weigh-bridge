// Package camera owns the Server-side camera and video contract. The first
// implementation work package fills repository, service and handler behaviour;
// these types are frozen here so the API, capture orchestration and Web client
// agree on names and secret semantics.
package camera

import (
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
)

// Secret actions make "leave blank keeps the current password" explicit and
// require a separate operation to erase a stored credential.
const (
	SecretKeep  = "keep"
	SecretSet   = "set"
	SecretClear = "clear"
)

// Stream states describe only paths owned by this system.
const (
	StreamPending  = "pending"
	StreamSynced   = "synced"
	StreamError    = "error"
	StreamRemoving = "removing"
	StreamRemoved  = "removed"
)

// Video sync states are independent of camera CRUD success and of direct RTSP
// capture availability.
const (
	VideoSyncPending = "pending"
	VideoSyncSynced  = "synced"
	VideoSyncError   = "error"
)

// Stream is the MediaMTX ownership view of one camera.
type Stream struct {
	Path           string    `json:"path"`
	PlaybackURL    string    `json:"playbackUrl"`
	DesiredVersion int64     `json:"desiredVersion"`
	SyncedVersion  int64     `json:"syncedVersion"`
	State          string    `json:"state"`
	LastError      string    `json:"lastError,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Camera is the safe read model. Password is never included; HasPassword lets
// the editor show whether a credential exists without revealing it.
type Camera struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"publicId"`
	Name           string    `json:"name"`
	SourceURL      string    `json:"sourceUrl"`
	Username       string    `json:"username"`
	HasPassword    bool      `json:"hasPassword"`
	CaptureEnabled bool      `json:"captureEnabled"`
	Enabled        bool      `json:"enabled"`
	Version        int64     `json:"version"`
	Stream         Stream    `json:"stream"`
	CreateTime     time.Time `json:"createTime"`
	UpdateTime     time.Time `json:"updateTime"`
}

// CameraInput is shared by create and update. Version must match the current
// row on update. Empty Password keeps an existing secret; only
// PasswordAction=clear erases it. CaptureEnabled/Enabled are pointers so an
// omitted field cannot silently disable a camera.
type CameraInput struct {
	Name           string  `json:"name"`
	SourceURL      string  `json:"sourceUrl"`
	Username       *string `json:"username,omitempty"`
	Password       string  `json:"password,omitempty"`
	PasswordAction string  `json:"passwordAction,omitempty"`
	CaptureEnabled *bool   `json:"captureEnabled,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
	Version        int64   `json:"version,omitempty"`
}

// CaptureSettings controls one bounded parallel acquisition. The 6 second
// default is one deadline for the whole set, not a per-camera timeout.
type CaptureSettings struct {
	ConfigVersion       int64  `json:"configVersion"`
	CaptureTimeoutMS    int    `json:"captureTimeoutMs"`
	RecognitionCameraID *int64 `json:"recognitionCameraId,omitempty"`
	RecognitionURL      string `json:"recognitionUrl,omitempty"`
}

type CaptureSettingsInput struct {
	ConfigVersion       int64   `json:"configVersion"`
	CaptureTimeoutMS    *int    `json:"captureTimeoutMs,omitempty"`
	RecognitionCameraID *int64  `json:"recognitionCameraId,omitempty"`
	RecognitionURL      *string `json:"recognitionUrl,omitempty"`
	// ClearRecognitionCamera explicitly deselects the recognition source.
	ClearRecognitionCamera bool `json:"clearRecognitionCamera,omitempty"`
}

// VideoSettings is the Server-reachable MediaMTX management endpoint and the
// browser-facing WHEP base. API password is never returned.
type VideoSettings struct {
	APIURL         string    `json:"apiUrl"`
	APIUsername    string    `json:"apiUsername"`
	HasAPIPassword bool      `json:"hasApiPassword"`
	WHEPBaseURL    string    `json:"whepBaseUrl"`
	Version        int64     `json:"version"`
	SyncState      string    `json:"syncState"`
	SyncError      string    `json:"syncError,omitempty"`
	SyncedVersion  int64     `json:"syncedVersion"`
	UpdateTime     time.Time `json:"updateTime"`
}

type VideoSettingsInput struct {
	APIURL         string  `json:"apiUrl"`
	APIUsername    *string `json:"apiUsername,omitempty"`
	APIPassword    string  `json:"apiPassword,omitempty"`
	PasswordAction string  `json:"passwordAction,omitempty"`
	WHEPBaseURL    string  `json:"whepBaseUrl"`
	Version        int64   `json:"version,omitempty"`
}

// List is the camera-management read model. ConfigVersion is the frozen value
// a sample records; a change invalidates unconfirmed samples that used an
// earlier set.
type List struct {
	Cameras             []Camera      `json:"cameras"`
	ConfigVersion       int64         `json:"configVersion"`
	CaptureTimeoutMS    int           `json:"captureTimeoutMs"`
	RecognitionCameraID *int64        `json:"recognitionCameraId,omitempty"`
	RecognitionURL      string        `json:"recognitionUrl,omitempty"`
	Video               VideoSettings `json:"video"`
}

// SyncRequest asks for reconciliation of this system's managed paths. It may
// target the current version only; a late result must not mark a newer config
// as synced.
type SyncRequest struct {
	ConfigVersion int64 `json:"configVersion,omitempty"`
	VideoVersion  int64 `json:"videoVersion,omitempty"`
}

type SyncResult struct {
	ConfigVersion int64     `json:"configVersion"`
	VideoVersion  int64     `json:"videoVersion"`
	State         string    `json:"state"`
	Error         string    `json:"error,omitempty"`
	Streams       []Stream  `json:"streams"`
	At            time.Time `json:"at"`
}

// CapturePlan freezes the camera set and credentials for one acquisition. It
// is internal to Server and must not be returned to the browser.
type CapturePlan struct {
	ScaleCode         string
	CaptureID         string
	ConfigVersion     int64
	TimeoutMS         int
	Required          bool
	RecognitionURL    string
	RecognitionCamera int64
	Cameras           []CaptureCamera
}

type CaptureCamera struct {
	ID             int64
	PublicID       string
	Name           string
	SourceURL      string
	Username       string
	Password       string
	CaptureEnabled bool
	Version        int64
}

// CaptureOutcome preserves every camera's actual status. Images contains only
// acquired objects; failed entries never carry a URL or fake SHA-256.
type CaptureOutcome struct {
	Success       bool             `json:"success"`
	ConfigVersion int64            `json:"configVersion"`
	Images        []p.Image        `json:"images"`
	Failures      []CaptureFailure `json:"failures"`
	At            time.Time        `json:"at"`
}

type CaptureFailure struct {
	CameraID       int64  `json:"cameraId"`
	CameraPublicID string `json:"cameraPublicId"`
	CameraName     string `json:"cameraName"`
	Code           string `json:"code"`
	Message        string `json:"message"`
}

// Failure codes used by capture results and the Web UI.
const (
	CodeNotConfigured   = "camera_not_configured"
	CodeUnavailable     = "camera_unavailable"
	CodeTimeout         = "camera_timeout"
	CodeInvalidImage    = "camera_invalid_image"
	CodeSaveFailed      = "camera_save_failed"
	CodeCaptureRequired = "capture_required"
)
