package agentmaintenance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	ph "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// configTimeout covers stopping, persisting, reloading and verifying the
// device. It is deliberately longer than the ordinary maintenance command.
const configTimeout = 45 * time.Second

type backupView struct {
	ID             int64            `json:"id"`
	ConfigVersion  int64            `json:"configVersion"`
	Digest         string           `json:"digest"`
	Settings       p.DeviceSettings `json:"settings"`
	AppliedAt      time.Time        `json:"appliedAt"`
	Source         string           `json:"source"`
	RestoredFromID *int64           `json:"restoredFromId,omitempty"`
	ReceivedAt     time.Time        `json:"receivedAt"`
	Name           string           `json:"name"`
	Notes          string           `json:"notes"`
	CreatedBy      *int64           `json:"createdBy"`
}

func toBackupView(record BackupRecord) backupView {
	return backupView{
		ID: record.ID, ConfigVersion: record.ConfigVersion, Digest: record.Digest,
		Settings: record.Settings, AppliedAt: record.AppliedAt, Source: record.Source,
		RestoredFromID: record.RestoredFromID, ReceivedAt: record.ReceivedAt,
		Name: record.Name, Notes: record.Notes, CreatedBy: record.CreatedBy,
	}
}

// effectiveFromRuntime lets the page show the last persisted success while the
// Agent is offline. It never substitutes for a live read when one is possible.
func effectiveFromRuntime(record RuntimeRecord) p.EffectiveConfig {
	effective := p.EffectiveConfig{
		ConfigVersion: record.EffectiveConfigVersion, Digest: record.EffectiveConfigDigest,
		Serial: record.EffectiveSettings.Serial, Print: record.EffectiveSettings.Print,
		DeviceState: record.DeviceState,
	}
	if record.EffectiveAppliedAt != nil {
		effective.AppliedAt = *record.EffectiveAppliedAt
	}
	return effective
}

func (r *Remote) decorateConfig(ctx context.Context, scaleCode string, raw json.RawMessage) json.RawMessage {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return raw
	}
	if effective, ok := r.hub.EffectiveConfig(scaleCode); ok {
		payload["effective"] = effective
	} else if r.store != nil {
		if runtime, ok, err := r.store.AgentRuntime(ctx, scaleCode); err == nil && ok {
			payload["effective"] = effectiveFromRuntime(runtime)
		}
	}
	// Preserve the read-only fields for existing API consumers; the Web now
	// manages restore points through the dedicated backups tab and endpoints.
	if backup, ok := r.hub.ConfigBackup(scaleCode); ok {
		payload["liveBackup"] = backup
	}
	if r.store != nil {
		records, _, err := r.store.ListConfigBackups(ctx, scaleCode, 0, 1)
		if err != nil {
			payload["backupError"] = err.Error()
		} else if len(records) > 0 {
			payload["backup"] = toBackupView(records[0])
		}
	}
	payload["capabilities"] = map[string]bool{
		"effectiveConfig":   r.hub.Supports(scaleCode, p.CapabilityEffectiveConfig),
		"cupsQueues":        r.hub.Supports(scaleCode, p.CapabilityCupsQueues),
		"parameterBackup":   r.hub.Supports(scaleCode, p.CapabilityParameterBackup),
		"measurementWindow": r.hub.Supports(scaleCode, p.CapabilityMeasurementWindow),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return data
}

func (r *Remote) beginConfigChange(c *gin.Context, scaleCode string) (func(), error) {
	if r.guard != nil {
		release, err := r.guard(c.Request.Context())
		if err != nil {
			ph.WriteError(c, http.StatusConflict, http.StatusConflict, "称重请求尚未结束，请核实后再维护", nil)
			return nil, err
		}
		r.hub.InvalidateSamples(scaleCode)
		return release, nil
	}
	r.hub.InvalidateSamples(scaleCode)
	return func() {}, nil
}

func (r *Remote) resolveScale(c *gin.Context) (Config, bool) {
	cfg, err := r.resolve(c.Request.Context())
	if err != nil {
		ph.WriteError(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "无法定位本厂设备 Agent", nil)
		return Config{}, false
	}
	return cfg, true
}

func (r *Remote) commandResult(c *gin.Context, cfg Config, kind string, payload any, timeout time.Duration) (p.Message, bool) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), timeout)
	defer cancel()
	message, err := r.hub.Command(ctx, cfg.ScaleCode, kind, payload, timeout)
	if err != nil {
		ph.WriteError(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, err.Error(), nil)
		return p.Message{}, false
	}
	return message, true
}

func (r *Remote) persistApplied(ctx context.Context, scaleCode string, result p.ConfigApplyResult) {
	if r.store != nil && result.Status == p.ConfigApplyApplied && result.Effective != nil {
		_ = r.store.SaveEffectiveConfig(ctx, scaleCode, *result.Effective)
	}
}

func (r *Remote) recordMaintenance(c *gin.Context, recorder audit.Recorder, action, summary string) {
	if recorder == nil {
		return
	}
	actor, _ := ph.UserIDFromContext(c.Request.Context())
	meta, _ := ph.RequestMetaFromContext(c.Request.Context())
	_ = recorder.Record(c.Request.Context(), audit.Event{
		Action: action, Resource: "agent-maintenance", Summary: summary,
		Metadata: audit.Metadata{ActorID: actor, RequestID: meta.RequestID, ClientIP: meta.ClientIP,
			UserAgent: meta.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.FullPath()},
	})
}

// applyConfig performs one explicit save-and-apply. Validation, persistence,
// device reload, verification and rollback all happen inside the Agent; the
// Server never treats a saved candidate as an applied configuration.
func (r *Remote) applyConfig(recorder audit.Recorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok := r.resolveScale(c)
		if !ok {
			return
		}
		var in struct {
			BaseVersion int64            `json:"baseVersion"`
			Settings    p.DeviceSettings `json:"settings"`
		}
		if c.ShouldBindJSON(&in) != nil {
			ph.WriteError(c, http.StatusBadRequest, http.StatusBadRequest, "配置参数格式无效", nil)
			return
		}
		if err := r.hub.RequireCapability(cfg.ScaleCode, p.CapabilityEffectiveConfig); err != nil {
			ph.WriteError(c, http.StatusConflict, http.StatusConflict, err.Error(), map[string]string{"code": p.ErrorAgentCapabilityMissing})
			return
		}
		release, err := r.beginConfigChange(c, cfg.ScaleCode)
		if err != nil {
			return
		}
		defer release()
		message, ok := r.commandResult(c, cfg, p.TypeAgentConfigApplyRequest, p.ConfigApplyRequest{BaseVersion: in.BaseVersion, Settings: in.Settings}, configTimeout)
		if !ok {
			return
		}
		var result p.ConfigApplyResult
		if message.Type != p.TypeAgentConfigApplyResult || json.Unmarshal(message.Payload, &result) != nil {
			ph.WriteError(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "Agent 配置应用结果格式无效，请核对设备实际状态", nil)
			return
		}
		r.persistApplied(context.WithoutCancel(c.Request.Context()), cfg.ScaleCode, result)
		r.recordMaintenance(c, recorder, "maintenance.config.apply", "保存并应用设备参数")
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, result)
	}
}

func (r *Remote) restoreConfig(recorder audit.Recorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok := r.resolveScale(c)
		if !ok {
			return
		}
		var in struct {
			BackupID    int64 `json:"backupId"`
			BaseVersion int64 `json:"baseVersion"`
		}
		if c.ShouldBindJSON(&in) != nil || in.BackupID < 1 {
			ph.WriteError(c, http.StatusBadRequest, http.StatusBadRequest, "请选择有效的成功配置备份", nil)
			return
		}
		if r.store == nil {
			ph.WriteError(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "服务端未配置参数备份存储", nil)
			return
		}
		record, err := r.store.ConfigBackupByID(c.Request.Context(), cfg.ScaleCode, in.BackupID)
		if err != nil {
			status := http.StatusBadRequest
			if !errors.Is(err, ErrNoBackup) {
				status = http.StatusServiceUnavailable
			}
			ph.WriteError(c, status, status, err.Error(), nil)
			return
		}
		if err := r.hub.RequireCapability(cfg.ScaleCode, p.CapabilityParameterBackup); err != nil {
			ph.WriteError(c, http.StatusConflict, http.StatusConflict, err.Error(), map[string]string{"code": p.ErrorAgentCapabilityMissing})
			return
		}
		release, err := r.beginConfigChange(c, cfg.ScaleCode)
		if err != nil {
			return
		}
		defer release()
		request := p.ConfigRestoreRequest{
			BackupID: record.ID, BaseVersion: in.BaseVersion,
			Backup: p.ConfigBackup{ConfigVersion: record.ConfigVersion, Digest: record.Digest, Settings: record.Settings, AppliedAt: record.AppliedAt},
		}
		message, ok := r.commandResult(c, cfg, p.TypeAgentConfigRestoreRequest, request, configTimeout)
		if !ok {
			return
		}
		var result p.ConfigApplyResult
		if message.Type != p.TypeAgentConfigRestoreResult || json.Unmarshal(message.Payload, &result) != nil {
			ph.WriteError(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "Agent 配置恢复结果格式无效，请核对设备实际状态", nil)
			return
		}
		r.persistApplied(context.WithoutCancel(c.Request.Context()), cfg.ScaleCode, result)
		r.recordMaintenance(c, recorder, "maintenance.config.restore", "恢复设备配置备份 #"+strconv.FormatInt(record.ID, 10))
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, result)
	}
}

// queues enumerates CUPS destinations on the Agent host. The Server never
// enumerates its own queues for a remote device.
func (r *Remote) queues(c *gin.Context) {
	cfg, ok := r.resolveScale(c)
	if !ok {
		return
	}
	if err := r.hub.RequireCapability(cfg.ScaleCode, p.CapabilityCupsQueues); err != nil {
		ph.WriteError(c, http.StatusConflict, http.StatusConflict, err.Error(), map[string]string{"code": p.ErrorAgentCapabilityMissing})
		return
	}
	message, ok := r.commandResult(c, cfg, p.TypePrinterQueuesRequest, struct{}{}, 10*time.Second)
	if !ok {
		return
	}
	var result p.CupsQueuesResult
	if message.Type != p.TypePrinterQueuesResult || json.Unmarshal(message.Payload, &result) != nil {
		ph.WriteError(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "Agent 未返回可读取的打印队列", nil)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}
