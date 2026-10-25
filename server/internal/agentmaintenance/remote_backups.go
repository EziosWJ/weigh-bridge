package agentmaintenance

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	ph "github.com/EziosWJ/weighbridge/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func (r *Remote) backupStore(c *gin.Context) (Config, bool) {
	cfg, ok := r.resolveScale(c)
	if !ok {
		return cfg, false
	}
	if r.store == nil {
		ph.WriteError(c, 503, 503, "服务端未配置参数备份存储", nil)
		return cfg, false
	}
	return cfg, true
}

func (r *Remote) listBackups(c *gin.Context) {
	cfg, ok := r.backupStore(c)
	if !ok {
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if offset < 0 || limit < 1 || limit > 100 {
		ph.WriteError(c, 400, 400, "分页参数无效", nil)
		return
	}
	records, total, err := r.store.ListConfigBackups(c.Request.Context(), cfg.ScaleID, offset, limit)
	if err != nil {
		ph.WriteError(c, 503, 503, "配置备份读取失败", nil)
		return
	}
	views := make([]backupView, 0, len(records))
	for _, record := range records {
		views = append(views, toBackupView(record))
	}
	effective, live := r.hub.EffectiveConfig(cfg.ScaleID)
	var state *p.EffectiveConfig
	if live {
		state = &effective
	} else {
		runtime, found, err := r.store.AgentRuntime(c.Request.Context(), cfg.ScaleID)
		if err != nil {
			ph.WriteError(c, 503, 503, "最近生效配置读取失败", nil)
			return
		}
		if found {
			effective = effectiveFromRuntime(runtime)
			state = &effective
		}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"records": views, "total": total, "effective": state, "live": live})
}

// Creation accepts only metadata and the version the user reviewed. Device
// settings are copied exclusively from the current Agent's confirmed report.
func (r *Remote) createBackup(recorder audit.Recorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok := r.backupStore(c)
		if !ok {
			return
		}
		var in struct {
			Name        string `json:"name"`
			Notes       string `json:"notes"`
			BaseVersion int64  `json:"baseVersion"`
			Digest      string `json:"digest"`
		}
		if c.ShouldBindJSON(&in) != nil {
			ph.WriteError(c, 400, 400, "备份参数无效", nil)
			return
		}
		in.Name, in.Notes = strings.TrimSpace(in.Name), strings.TrimSpace(in.Notes)
		if in.Name == "" || utf8.RuneCountInString(in.Name) > 100 || utf8.RuneCountInString(in.Notes) > 500 {
			ph.WriteError(c, 400, 400, "备份名称必填且不超过 100 字，备注不超过 500 字", nil)
			return
		}
		effective, live := r.hub.EffectiveConfig(cfg.ScaleID)
		backup, confirmed := r.hub.ConfigBackup(cfg.ScaleID)
		if !live || !confirmed || !r.hub.Supports(cfg.ScaleID, p.CapabilityParameterBackup) {
			ph.WriteError(c, 409, 409, "Agent 离线或尚未报告成功生效参数，不能创建备份", nil)
			return
		}
		if err := validateBackupSnapshot(effective, backup, in.BaseVersion, in.Digest); err != nil {
			ph.WriteError(c, 409, 409, err.Error(), nil)
			return
		}
		actor, _ := ph.UserIDFromContext(c.Request.Context())
		id, err := r.store.CreateConfigBackup(c.Request.Context(), cfg.ScaleID, backup, in.Name, in.Notes, actor)
		if err != nil {
			ph.WriteError(c, 503, 503, "创建备份失败，请刷新列表核实后再操作", nil)
			return
		}
		r.recordMaintenance(c, recorder, "maintenance.backup.create", "创建设备配置备份："+in.Name)
		c.JSON(http.StatusCreated, gin.H{"id": id})
	}
}

func validateBackupSnapshot(effective p.EffectiveConfig, backup p.ConfigBackup, version int64, digest string) error {
	settings := p.DeviceSettings{Serial: effective.Serial, Print: effective.Print}
	if backup.AppliedAt.IsZero() || backup.ConfigVersion != version || effective.ConfigVersion != version ||
		backup.Digest != digest || effective.Digest != digest || backup.Settings.Digest() != digest || settings.Digest() != digest {
		return errors.New("设备配置已变化或尚未核实，请刷新后重新创建备份")
	}
	return nil
}

func (r *Remote) deleteBackup(recorder audit.Recorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok := r.backupStore(c)
		if !ok {
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			ph.WriteError(c, 400, 400, "备份编号无效", nil)
			return
		}
		if err := r.store.DeleteConfigBackup(c.Request.Context(), cfg.ScaleID, id); err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, ErrNoBackup) {
				status = http.StatusNotFound
			}
			ph.WriteError(c, status, status, "备份删除失败或记录不存在", nil)
			return
		}
		r.recordMaintenance(c, recorder, "maintenance.backup.delete", "删除设备配置备份 #"+strconv.FormatInt(id, 10))
		c.Status(http.StatusNoContent)
	}
}
