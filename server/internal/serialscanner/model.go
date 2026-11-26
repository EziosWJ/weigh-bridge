// Package serialscanner persists the local serial QR scanner configuration,
// receives heartbeats from the background serial-agent, and exposes the
// loopback-only internal API that the agent uses to scan a mobile QR.
package serialscanner

import (
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// Config is the singleton serial scanner configuration. Delimiter and the
// trim fields keep the operator's escaped text (for example \r\n), so the same
// value is readable in the maintenance UI and decodable by the Agent.
type Config struct {
	ID              int64     `json:"-" gorm:"primaryKey"`
	Enabled         bool      `json:"enabled" gorm:"column:enabled"`
	Device          string    `json:"device" gorm:"column:device"`
	BaudRate        int       `json:"baudRate" gorm:"column:baud_rate"`
	DataBits        int       `json:"dataBits" gorm:"column:data_bits"`
	StopBits        int       `json:"stopBits" gorm:"column:stop_bits"`
	Parity          string    `json:"parity" gorm:"column:parity"`
	Delimiter       string    `json:"delimiter" gorm:"column:delimiter"`
	TrimPrefix      string    `json:"trimPrefix" gorm:"column:trim_prefix"`
	TrimSuffix      string    `json:"trimSuffix" gorm:"column:trim_suffix"`
	RetryCount      int       `json:"retryCount" gorm:"column:retry_count"`
	RetryIntervalMS int       `json:"retryIntervalMs" gorm:"column:retry_interval_ms"`
	Version         int64     `json:"version" gorm:"column:version"`
	UpdateBy        int64     `json:"updateBy" gorm:"column:update_by"`
	UpdateTime      time.Time `json:"updateTime" gorm:"column:update_time"`
}

func (Config) TableName() string { return "wb_serial_scanner" }

// ConfigInput is the writable subset accepted from the maintenance page.
type ConfigInput struct {
	Enabled         bool   `json:"enabled"`
	Device          string `json:"device"`
	BaudRate        int    `json:"baudRate"`
	DataBits        int    `json:"dataBits"`
	StopBits        int    `json:"stopBits"`
	Parity          string `json:"parity"`
	Delimiter       string `json:"delimiter"`
	TrimPrefix      string `json:"trimPrefix"`
	TrimSuffix      string `json:"trimSuffix"`
	RetryCount      int    `json:"retryCount"`
	RetryIntervalMS int    `json:"retryIntervalMs"`
}

// Status is the last heartbeat reported by the local Agent. Counters are for
// the current Agent run, not lifetime totals.
type Status struct {
	ID             int64      `json:"-" gorm:"primaryKey"`
	AgentID        string     `json:"agentId" gorm:"column:agent_id"`
	PID            *int       `json:"pid,omitempty" gorm:"column:pid"`
	Connected      bool       `json:"connected" gorm:"column:connected"`
	Device         string     `json:"device" gorm:"column:device"`
	ConfigVersion  int64      `json:"configVersion" gorm:"column:config_version"`
	Scans          int64      `json:"scans" gorm:"column:scans"`
	Failures       int64      `json:"failures" gorm:"column:failures"`
	LastScanAt     *time.Time `json:"lastScanAt,omitempty" gorm:"column:last_scan_at"`
	LastScanResult string     `json:"lastScanResult" gorm:"column:last_scan_result"`
	LastError      string     `json:"lastError" gorm:"column:last_error"`
	HeartbeatAt    *time.Time `json:"heartbeatAt,omitempty" gorm:"column:heartbeat_at"`
	UpdateTime     time.Time  `json:"updateTime" gorm:"column:update_time"`
}

func (Status) TableName() string { return "wb_serial_scanner_status" }

// HeartbeatInput is sent by the Agent. It never contains a user identity.
type HeartbeatInput struct {
	AgentID        string     `json:"agentId"`
	PID            int        `json:"pid"`
	Connected      bool       `json:"connected"`
	Device         string     `json:"device"`
	ConfigVersion  int64      `json:"configVersion"`
	Scans          int64      `json:"scans"`
	Failures       int64      `json:"failures"`
	LastScanAt     *time.Time `json:"lastScanAt,omitempty"`
	LastScanResult string     `json:"lastScanResult"`
	LastError      string     `json:"lastError"`
}

// HeartbeatResult tells the Agent whether its applied configuration is stale.
type HeartbeatResult struct {
	ServerTime    time.Time `json:"serverTime"`
	ConfigVersion int64     `json:"configVersion"`
	Enabled       bool      `json:"enabled"`
	Reload        bool      `json:"reload"`
}

// Snapshot is returned to the maintenance page.
type Snapshot struct {
	Config     Config                   `json:"config"`
	Status     Status                   `json:"status"`
	Online     bool                     `json:"online"`
	ServerTime time.Time                `json:"serverTime"`
	Recent     []weighing.MobileHandoff `json:"recent"`
}
