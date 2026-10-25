package agentproto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Protocol revision 2 adds keyless admission, explicit capabilities and a
// measurement window that keeps the original instrument basis valid across the
// camera capture interval and until the operator confirms the weighing.
// Revision 1 messages remain readable for device state, printing and legacy
// diagnostics, but formal capture must not fall back to weak validation when
// the required capability is missing.
const (
	ProtocolVersion       = 2
	LegacyProtocolVersion = 1
)

// Message type names for the revision 2 contract. Existing v1 handlers keep
// using their historical string literals; new code should use these constants.
const (
	TypeHello    = "agent.hello"
	TypeHelloAck = "agent.hello.ack"

	TypeMeasurementBeginRequest  = "measurement.begin.request"
	TypeMeasurementBeginResult   = "measurement.begin.result"
	TypeMeasurementEndRequest    = "measurement.end.request"
	TypeMeasurementEndResult     = "measurement.end.result"
	TypeMeasurementCancelRequest = "measurement.cancel.request"
	TypeMeasurementCancelResult  = "measurement.cancel.result"
	TypeMeasurementVerifyRequest = "measurement.verify.request"
	TypeMeasurementVerifyResult  = "measurement.verify.result"
	TypeMeasurementInvalidated   = "measurement.invalidated"

	TypeAgentConfigState          = "agent.config.state"
	TypeAgentConfigApplyRequest   = "agent.config.apply.request"
	TypeAgentConfigApplyResult    = "agent.config.apply.result"
	TypeAgentConfigRestoreRequest = "agent.config.restore.request"
	TypeAgentConfigRestoreResult  = "agent.config.restore.result"
	TypeAgentConfigBackup         = "agent.config.backup"

	TypePrinterQueuesRequest = "printer.queues.request"
	TypePrinterQueuesResult  = "printer.queues.result"
)

// Capability names are advertised by agent.hello and checked before commands
// are sent. Unknown capability names are ignored so newer clients can add more.
const (
	CapabilityMeasurementWindow = "measurement.window.v2"
	CapabilityEffectiveConfig   = "maintenance.effective-config.v2"
	CapabilityCupsQueues        = "maintenance.cups-queues.v1"
	CapabilityParameterBackup   = "maintenance.parameter-backup.v1"
)

// FormalCaptureCapabilities lists the minimum client capabilities for a
// formal capture. A client without them connects but cannot produce a sample.
var FormalCaptureCapabilities = []string{CapabilityMeasurementWindow}

// Error codes carried by an error message or by a correlated result.
const (
	ErrorSiteNotInitialized       = "site_not_initialized"
	ErrorScaleNotInitialized      = "scale_not_initialized"
	ErrorServerUnavailable        = "server_unavailable"
	ErrorAgentConflict            = "agent_conflict"
	ErrorAgentProtocolUnsupported = "agent_protocol_unsupported"
	ErrorAgentCapabilityMissing   = "agent_capability_missing"
	ErrorInvalidMessage           = "invalid_message"
	ErrorInvalidPayload           = "invalid_payload"
	ErrorDeviceUnavailable        = "device_unavailable"
	ErrorDeviceUnstable           = "device_unstable"
	ErrorDeviceFault              = "device_fault"
	ErrorDeviceStale              = "device_stale"
	ErrorWeightChanged            = "weight_changed"
	ErrorGenerationInvalid        = "generation_invalid"
	ErrorGenerationSuperseded     = "generation_superseded"
	ErrorCancelled                = "cancelled"
	ErrorConnectionChanged        = "connection_changed"
	ErrorConfigConflict           = "config_conflict"
	ErrorConfigInvalid            = "config_invalid"
	ErrorDeviceBusy               = "device_busy"
	ErrorApplyUnknown             = "apply_unknown"
	ErrorRollbackFailed           = "rollback_failed"
	ErrorQueuesUnavailable        = "queues_unavailable"
)

// Error is the standard error payload. messageId stays at the envelope level;
// requestId is echoed when the error correlates to a command.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Hello is sent by the Agent immediately after the WebSocket is established.
// It replaces the v1 Authorization/X-Agent-ID/X-Scale-ID headers. The Agent
// only needs the Server address in its local configuration.
type Hello struct {
	ProtocolVersion int      `json:"protocolVersion"`
	AgentVersion    string   `json:"agentVersion,omitempty"`
	Hostname        string   `json:"hostname,omitempty"`
	Capabilities    []string `json:"capabilities"`
}

// HelloAck assigns the local site and its sole scale. AgentID is an internal
// troubleshooting identity; it is not a pairing secret and need not be stored
// by the Agent beyond showing it in diagnostics.
type HelloAck struct {
	ProtocolVersion int       `json:"protocolVersion"`
	SiteID          int64     `json:"siteId"`
	ScaleID         int64     `json:"scaleId"`
	AgentID         string    `json:"agentId"`
	Capabilities    []string  `json:"capabilities"`
	ServerTime      time.Time `json:"serverTime"`
}

// MeasuredState is an instrument reading tied to the Agent's local receive
// time. The latency of device.state is not sufficient for the measurement
// window, so every measurement reply carries the same explicit reading.
type MeasuredState struct {
	DeviceState
	At time.Time `json:"at"`
}

// MeasurementBeginRequest opens one measurement window. CaptureID lets Server
// and Agent correlate the window with the images acquired after begin confirms.
type MeasurementBeginRequest struct {
	CaptureID             string `json:"captureId"`
	CapturePolicyRevision string `json:"capturePolicyRevision,omitempty"`
	CameraConfigVersion   int64  `json:"cameraConfigVersion,omitempty"`
}

// MeasurementBeginResult confirms that the Agent locked a baseline. accepted
// false carries a code from the Error* constants and no generation.
type MeasurementBeginResult struct {
	Accepted   bool          `json:"accepted"`
	Generation string        `json:"generation,omitempty"`
	State      MeasuredState `json:"state"`
	Code       string        `json:"code,omitempty"`
	Message    string        `json:"message,omitempty"`
}

// MeasurementEndRequest asks the Agent to verify that the generation survived
// the whole image capture interval without any invalidating transition.
type MeasurementEndRequest struct {
	Generation string `json:"generation"`
}

type MeasurementEndResult struct {
	Verified   bool          `json:"verified"`
	Generation string        `json:"generation"`
	State      MeasuredState `json:"state"`
	EndedAt    time.Time     `json:"endedAt"`
	Code       string        `json:"code,omitempty"`
	Message    string        `json:"message,omitempty"`
}

// MeasurementCancelRequest releases a window that will never be confirmed.
type MeasurementCancelRequest struct {
	Generation string `json:"generation"`
	Reason     string `json:"reason,omitempty"`
}

type MeasurementCancelResult struct {
	Cancelled  bool   `json:"cancelled"`
	Generation string `json:"generation"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
}

// MeasurementVerifyRequest is sent at operator confirmation, after the capture
// window has ended. A generation must still be the Agent's current valid basis;
// a transient change that restored the same weight must already have failed it.
type MeasurementVerifyRequest struct {
	Generation string `json:"generation"`
}

type MeasurementVerifyResult struct {
	Valid      bool          `json:"valid"`
	Generation string        `json:"generation"`
	State      MeasuredState `json:"state"`
	CheckedAt  time.Time     `json:"checkedAt"`
	Code       string        `json:"code,omitempty"`
	Message    string        `json:"message,omitempty"`
}

// MeasurementInvalidated is an unsolicited Agent notification. Server records
// it against the generation; the correlated end/verify replies remain
// authoritative and must follow the same rules.
type MeasurementInvalidated struct {
	Generation string        `json:"generation"`
	Reason     string        `json:"reason,omitempty"`
	State      MeasuredState `json:"state"`
	At         time.Time     `json:"at"`
}

// DeviceSettings is the only Agent-owned editable device configuration after
// ADR 0030. It excludes Server URL, host details, connection state, measurement
// windows and execution requests.
type DeviceSettings struct {
	Serial SerialSettings `json:"serial"`
	Print  PrintSettings  `json:"print"`
}

type SerialSettings struct {
	Port string `json:"port"`
	Baud int    `json:"baud"`
}

type PrintSettings struct {
	Queue        string  `json:"queue"`
	FormLengthMM float64 `json:"formLengthMm"`
	PrintWidthMM int     `json:"printWidthMm"`
	PrintScale   float64 `json:"printScale"`
	OffsetXMM    int     `json:"offsetXmm"`
	OffsetYMM    int     `json:"offsetYmm"`
}

// EffectiveConfig is the applied-device truth reported by the Agent. Digest is
// the SHA-256 of the canonical settings, not of secrets or the connection file.
type EffectiveConfig struct {
	ConfigVersion int64          `json:"configVersion"`
	Digest        string         `json:"digest"`
	Serial        SerialSettings `json:"serial"`
	Print         PrintSettings  `json:"print"`
	AppliedAt     time.Time      `json:"appliedAt"`
	Running       bool           `json:"running"`
	DeviceState   string         `json:"deviceState"` // running, degraded, unavailable
	SerialState   string         `json:"serialState,omitempty"`
	PrinterState  string         `json:"printerState,omitempty"`
}

// ConfigApplyRequest is a single save-and-apply operation. BaseVersion is the
// Agent config version last observed by Server; zero means no precondition.
// A mismatch returns config_conflict without changing the effective settings.
type ConfigApplyRequest struct {
	BaseVersion int64          `json:"baseVersion,omitempty"`
	Settings    DeviceSettings `json:"settings"`
}

// ConfigApplyResult status values.
const (
	ConfigApplyApplied        = "applied"
	ConfigApplyRejected       = "rejected"
	ConfigApplyUnknown        = "unknown"
	ConfigApplyRolledBack     = "rolled_back"
	ConfigApplyRollbackFailed = "rollback_failed"
)

type ConfigApplyResult struct {
	Status        string           `json:"status"`
	Code          string           `json:"code,omitempty"`
	Message       string           `json:"message,omitempty"`
	ConfigVersion int64            `json:"configVersion"`
	Digest        string           `json:"digest"`
	AppliedAt     *time.Time       `json:"appliedAt,omitempty"`
	Effective     *EffectiveConfig `json:"effective,omitempty"`
}

// ConfigBackup is the Server-persisted copy of the last successfully applied
// settings. It never contains Server URL, identity, connection or runtime state.
type ConfigBackup struct {
	ConfigVersion int64          `json:"configVersion"`
	Digest        string         `json:"digest"`
	Settings      DeviceSettings `json:"settings"`
	AppliedAt     time.Time      `json:"appliedAt"`
	RestoreOf     int64          `json:"restoreOf,omitempty"`
}

// ConfigRestoreRequest explicitly restores one backup. Backup is carried in
// full so the Agent does not need Server database access and a stale backup id
// alone can never overwrite newer local settings.
type ConfigRestoreRequest struct {
	BackupID    int64        `json:"backupId"`
	BaseVersion int64        `json:"baseVersion,omitempty"`
	Backup      ConfigBackup `json:"backup"`
}

// CupsQueue is one CUPS destination enumerated on the Agent host.
type CupsQueue struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"isDefault"`
}

type CupsQueuesResult struct {
	Queues []CupsQueue `json:"queues"`
	At     time.Time   `json:"at"`
	Code   string      `json:"code,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// Digest identifies the canonical device settings independently of the local
// connection file, runtime state or diagnostics. The Server and the Agent use
// the same value to detect whether a reported configuration is already stored.
func (s DeviceSettings) Digest() string {
	data, _ := json.Marshal(s)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
