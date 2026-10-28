package weighing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentBinding struct {
	LocalDevice    bool `gorm:"primaryKey;autoIncrement:false"`
	ScaleCode      string
	InstallationID string `gorm:"->"`
	AgentID        string
	AgentKey       string     `json:"-"`
	RetiredAt      *time.Time `json:"-"`
}

func (AgentBinding) TableName() string { return "wb_agent_binding" }

type AgentSetting struct {
	ID        int64  `json:"-"`
	ServerURL string `json:"serverUrl"`
}

func (AgentSetting) TableName() string { return "wb_agent_setting" }

type SiteConfiguration struct {
	Site
	ScaleCode string `json:"scaleCode"`
	ScaleName string `json:"scaleName"`
	AgentID   string `json:"agentId"`
}
type SiteConfigurationInput struct {
	ServerURL string `json:"serverUrl"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	ScaleCode string `json:"scaleCode"`
	ScaleName string `json:"scaleName"`
	AgentID   string `json:"agentId"`
	Enabled   int    `json:"enabled"`
	Version   int64  `json:"version"`
}

// DailySequence is the last ticket serial allocated for one Beijing business
// date. After restoring a backup the counter can lag behind paper tickets, so
// an administrator may raise it explicitly (never lower it).
type DailySequence struct {
	BusinessDate string `json:"businessDate" gorm:"column:business_date"`
	LastIssued   int64  `json:"lastIssued" gorm:"column:next_value"`
}

func (r *Repository) ListTicketSequences(ctx context.Context) ([]DailySequence, error) {
	rows := []DailySequence{}
	err := r.db.WithContext(ctx).Table("wb_number_sequence").
		Select("business_date,next_value").
		Where("kind='ticket' AND site_code=(SELECT site_code FROM wb_local_identity WHERE id=1)").
		Order("business_date DESC").Limit(30).Find(&rows).Error
	return rows, err
}

// SetTicketWatermark raises the last issued serial for a business date. The
// database keeps the greater value, so a mistaken lower submission never
// re-opens already issued numbers. The operation is audited.
func (r *Repository) SetTicketWatermark(ctx context.Context, date string, lastIssued int64, e audit.Event) (DailySequence, error) {
	out := DailySequence{BusinessDate: date, LastIssued: lastIssued}
	if lastIssued <= 0 || lastIssued >= math.MaxInt64 {
		return out, RejectionError("请填写该日期已发放的最大流水号（大于 0）")
	}
	if _, err := time.ParseInLocation("20060102", date, beijing); err != nil {
		return out, ErrInvalid
	}
	identity, err := localsite.Read(ctx, r.db)
	if err != nil {
		return out, err
	}
	if identity.SiteCode == "" {
		return out, RejectionError("请先完成本厂身份配置")
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Raw(`INSERT INTO wb_number_sequence (site_code,kind,business_date,next_value)
			VALUES (?, 'ticket', ?, ?)
			ON CONFLICT (site_code,kind,business_date)
			DO UPDATE SET next_value=GREATEST(wb_number_sequence.next_value, EXCLUDED.next_value)
			RETURNING business_date,next_value`, identity.SiteCode, date, lastIssued).Scan(&out)
		if result.Error != nil {
			return result.Error
		}
		// Keep the out-of-database floor in step with the operator's verified
		// serial, so a later database-only restore cannot drop below it.
		if err := raiseWatermark(r.dataDir, watermarkKey{Kind: "ticket", SiteCode: identity.SiteCode, Date: date}, lastIssued); err != nil {
			return err
		}
		e.Action = "ticket.sequence.watermark"
		e.Resource = "weighing.ticket-sequence"
		e.Summary = string(snapshot(map[string]any{"businessDate": out.BusinessDate, "lastIssued": out.LastIssued, "siteCode": identity.SiteCode}))
		return audit.RecordOn(ctx, tx, e)
	})
	return out, err
}

func (r *Repository) SiteConfigurations(ctx context.Context) ([]SiteConfiguration, error) {
	rows := []SiteConfiguration{}
	err := r.db.WithContext(ctx).Table("wb_site s").Select("s.*,sc.code AS scale_code,sc.name AS scale_name,COALESCE(a.agent_id,'') AS agent_id").Joins("JOIN wb_scale sc ON sc.site_code=s.site_code").Joins("LEFT JOIN wb_agent_binding a ON a.local_device=TRUE").Order("s.site_code").Scan(&rows).Error
	return rows, err
}
func (r *Repository) AgentSettings(ctx context.Context) (AgentSetting, error) {
	return findOne[AgentSetting](r.db.WithContext(ctx), 1)
}
func (r *Repository) SaveAgentSettings(ctx context.Context, url string, e audit.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		setting, err := lockRow[AgentSetting](tx, 1)
		if err != nil {
			return err
		}
		setting.ServerURL = url
		if err = tx.Save(&setting).Error; err != nil {
			return err
		}
		e.Action = "agent.settings"
		e.Resource = "weighing.configuration"
		e.Summary = string(snapshot(setting))
		return audit.RecordOn(ctx, tx, e)
	})
}

// Import only missing rows. Existing local identities and keys remain valid;
// subsequent API starts never overwrite identities managed in the database.
func (r *Repository) ImportAgentBindings(ctx context.Context, configs []agentbridge.Config) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, c := range configs {
			if c.Key == "" || c.ScaleCode == "" {
				continue
			}
			var count int64
			if err := tx.Model(&Scale{}).Where("code=?", c.ScaleCode).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("Agent %s 的地磅不存在，请先迁移数据库", c.ID)
			}
			row := AgentBinding{LocalDevice: true, ScaleCode: c.ScaleCode, AgentID: c.ID, AgentKey: c.Key}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *Repository) AgentConfigs(ctx context.Context, imageDir string) ([]agentbridge.Config, error) {
	var rows []AgentBinding
	if err := r.db.WithContext(ctx).Table("wb_agent_binding a").Select("a.local_device,a.agent_id,a.agent_key,a.retired_at,sc.code AS scale_code,i.installation_id::text AS installation_id").Joins("JOIN wb_local_identity i ON i.id=1").Joins("JOIN wb_scale sc ON sc.code=i.scale_code AND sc.site_code=i.site_code").Where("a.local_device=TRUE AND a.retired_at IS NULL").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]agentbridge.Config, 0, len(rows))
	for _, a := range rows {
		out = append(out, agentbridge.Config{ID: a.AgentID, ScaleCode: a.ScaleCode, InstallationID: a.InstallationID, Key: a.AgentKey, ImageDir: imageDir})
	}
	return out, nil
}
func (r *Repository) AgentConfig(ctx context.Context, scaleCode string, imageDir string) (agentbridge.Config, error) {
	var a AgentBinding
	err := r.db.WithContext(ctx).Table("wb_agent_binding a").Select("a.local_device,a.agent_id,a.agent_key,a.retired_at,sc.code AS scale_code,i.installation_id::text AS installation_id").Joins("JOIN wb_local_identity i ON i.id=1").Joins("JOIN wb_scale sc ON sc.code=i.scale_code AND sc.site_code=i.site_code AND sc.enabled=1").Joins("JOIN wb_site s ON s.site_code=sc.site_code AND s.enabled=1").Where("a.local_device=TRUE AND sc.code=? AND a.retired_at IS NULL", scaleCode).Take(&a).Error
	return agentbridge.Config{ID: a.AgentID, ScaleCode: a.ScaleCode, InstallationID: a.InstallationID, Key: a.AgentKey, ImageDir: imageDir}, err
}

// LocalAgentConfig resolves the keyless runtime association for the sole
// enabled scale of the local factory. The Agent identity is internal and is
// taken from the retired binding when one exists, so historical diagnostics
// keep their name without making the key an admission requirement.
func (r *Repository) LocalAgentConfig(ctx context.Context, imageDir string) (agentbridge.Config, error) {
	var row struct {
		SiteCode       string
		ScaleCode      string
		InstallationID string
		AgentID        string
	}
	err := r.db.WithContext(ctx).Table("wb_scale sc").
		Select("sc.site_code,sc.code AS scale_code,i.installation_id::text AS installation_id,COALESCE(a.agent_id,'') AS agent_id").
		Joins("JOIN wb_site s ON s.site_code=sc.site_code AND s.enabled=1").
		Joins("JOIN wb_local_identity i ON i.site_code=sc.site_code AND i.id=1").
		Joins("LEFT JOIN wb_agent_binding a ON a.local_device=TRUE").
		Where("sc.enabled=1").Order("sc.code").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return agentbridge.Config{}, agentbridge.ErrScaleNotInitialized
	}
	if err != nil {
		return agentbridge.Config{}, err
	}
	id := row.AgentID
	if id == "" {
		id = "agent-" + row.ScaleCode
	}
	return agentbridge.Config{ID: id, SiteCode: row.SiteCode, ScaleCode: row.ScaleCode, InstallationID: row.InstallationID, ImageDir: imageDir}, nil
}
func (r *Repository) SaveSiteConfiguration(ctx context.Context, in SiteConfigurationInput, e audit.Event, connected func(string) bool) (SiteConfiguration, error) {
	identity, identityErr := localsite.Read(ctx, r.db)
	if identityErr != nil {
		return SiteConfiguration{}, identityErr
	}
	if identity.SiteCode == "" {
		return SiteConfiguration{}, ErrNotFound
	}
	var out SiteConfiguration
	connectedNow := false
	if connected != nil && in.Enabled == 0 {
		var sc Scale
		if err := r.db.WithContext(ctx).Where("site_code=?", identity.SiteCode).Order("code").Take(&sc).Error; err != nil {
			return out, err
		}
		connectedNow = connected(sc.Code)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize edits with first-time setup and keep identity changes atomic.
		if _, err := lockRow[AgentSetting](tx, 1); err != nil {
			return err
		}
		site := Site{}
		scale := Scale{}
		binding := AgentBinding{}
		var err error
		site, err = lockRowWhere[Site](tx, "site_code=?", identity.SiteCode)
		if err != nil {
			return err
		}
		if site.Version != in.Version {
			return ErrConflict
		}
		if err = tx.Where("site_code=?", site.Code).Order("code").Take(&scale).Error; err != nil {
			return err
		}
		if err = tx.Where("local_device=TRUE").Limit(1).Find(&binding).Error; err != nil {
			return err
		}
		if in.AgentID != "" && binding.AgentID != "" && binding.AgentID != in.AgentID {
			return RejectionError("已有 Agent 编号不可修改，请保留原身份")
		}
		nextSiteCode, nextScaleCode := site.Code, scale.Code
		if in.Code != "" {
			nextSiteCode = in.Code
		}
		if in.ScaleCode != "" {
			nextScaleCode = in.ScaleCode
		}
		if in.Enabled == 0 {
			reasons := []string{}
			var count int64
			if err := tx.Model(&Task{}).Where("status='awaiting_second'").Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				reasons = append(reasons, fmt.Sprintf("%d 笔未完成任务", count))
			}
			if err := tx.Table("wb_print_attempt p").Joins("JOIN wb_ticket t ON t.id=p.ticket_id").Where("p.closed_at IS NULL").Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				reasons = append(reasons, fmt.Sprintf("%d 笔未决打印", count))
			}
			if connectedNow {
				reasons = append(reasons, "Agent 尚未离线")
			}
			if len(reasons) > 0 {
				return RejectionError("不能停用：" + strings.Join(reasons, "、"))
			}
		}
		// Update only the singleton's current metadata. Historical business and
		// evidence snapshots are never rewritten when codes change.
		if err := tx.Model(&Site{}).Where("site_code=?", site.Code).Updates(map[string]any{"site_code": nextSiteCode, "name": in.Name, "enabled": in.Enabled, "version": site.Version + 1}).Error; err != nil {
			return normalizeSiteError(err)
		}
		if err := tx.Model(&Scale{}).Where("site_code=? AND code=?", scale.SiteCode, scale.Code).Updates(map[string]any{"site_code": nextSiteCode, "code": nextScaleCode, "name": in.ScaleName, "enabled": in.Enabled}).Error; err != nil {
			return normalizeSiteError(err)
		}
		if err := tx.Model(&localsite.Identity{}).Where("id=1").Updates(map[string]any{"site_code": nextSiteCode, "scale_code": nextScaleCode}).Error; err != nil {
			return err
		}
		before := SiteConfiguration{Site: site, ScaleCode: scale.Code, ScaleName: scale.Name, AgentID: binding.AgentID}
		site.Code, site.Name, site.Enabled, site.Version = nextSiteCode, in.Name, in.Enabled, site.Version+1
		scale.SiteCode, scale.Code, scale.Name, scale.Enabled = nextSiteCode, nextScaleCode, in.ScaleName, in.Enabled
		if err := saveIdentityConfigs(tx, site.Code, scale.Code, site.Name, scale.Name); err != nil {
			return err
		}
		if binding.AgentID == "" {
			// Internal association only; operators no longer supply or maintain
			// an Agent number. Existing retired bindings keep their historical id.
			binding = AgentBinding{LocalDevice: true, ScaleCode: scale.Code, AgentID: "agent-" + scale.Code, AgentKey: agentbridge.ID()}
			if err := tx.Create(&binding).Error; err != nil {
				return normalizeSiteError(err)
			}
		}

		out = SiteConfiguration{Site: site, ScaleCode: scale.Code, ScaleName: scale.Name, AgentID: binding.AgentID}
		e.Action = "site.save"
		e.Resource = "weighing.site"
		e.ResourceID = site.Version
		e.Summary = string(snapshot(map[string]any{"before": before, "after": out}))
		return audit.RecordOn(ctx, tx, e)
	})
	return out, err
}
func normalizeSiteError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique") {
		return RejectionError("厂区、地磅或 Agent 编号已存在")
	}
	return err
}
func (r *Repository) ExportAgentConnection(ctx context.Context, serverURL string, e audit.Event) (agentconfig.Connection, error) {
	identity, identityErr := localsite.Read(ctx, r.db)
	if identityErr != nil {
		return agentconfig.Connection{}, identityErr
	}
	if identity.SiteCode == "" {
		return agentconfig.Connection{}, ErrNotFound
	}
	var out agentconfig.Connection
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Table("wb_site s").Select("s.site_code,s.name AS site_name,sc.code AS scale_code,sc.name AS scale_name,a.agent_id").Joins("JOIN wb_scale sc ON sc.site_code=s.site_code").Joins("JOIN wb_agent_binding a ON a.local_device=TRUE").Where("s.site_code=?", identity.SiteCode).Take(&out)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return RejectionError("请先保存该厂区的 Agent 配置")
		}
		if result.Error != nil {
			return result.Error
		}
		out.ServerURL = serverURL
		// ADR 0030 removed the Agent key from the connection contract. The
		// legacy export remains available for old callers but must not hand
		// back an unused secret or make identity an admission requirement.
		out.AgentKey = ""
		if err := out.ValidateEndpoint(); err != nil {
			return RejectionError("Agent 配对信息或连接地址无效")
		}
		e.Action = "agent.configuration.export"
		e.Resource = "weighing.site"
		e.ResourceID = identity.NextID
		e.Summary = string(snapshot(map[string]any{"siteCode": identity.SiteCode, "agentId": out.AgentID}))
		return audit.RecordOn(ctx, tx, e)
	})
	return out, err
}
