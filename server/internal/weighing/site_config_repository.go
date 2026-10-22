package weighing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentBinding struct {
	ScaleID  int64 `gorm:"primaryKey;autoIncrement:false"`
	AgentID  string
	AgentKey string `json:"-"`
}

func (AgentBinding) TableName() string { return "wb_agent_binding" }

type AgentSetting struct {
	ID        int64  `json:"-"`
	ServerURL string `json:"serverUrl"`
}

func (AgentSetting) TableName() string { return "wb_agent_setting" }

type SiteConfiguration struct {
	Site
	ScaleID         int64  `json:"scaleId"`
	ScaleCode       string `json:"scaleCode"`
	ScaleName       string `json:"scaleName"`
	AgentID         string `json:"agentId"`
	PendingRevision int64  `json:"pendingRevision"`
	AppliedRevision int64  `json:"appliedRevision"`
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

func (r *Repository) SiteConfigurations(ctx context.Context) ([]SiteConfiguration, error) {
	rows := []SiteConfiguration{}
	err := r.db.WithContext(ctx).Table("wb_site s").Select("s.*,sc.id AS scale_id,sc.code AS scale_code,sc.name AS scale_name,COALESCE(a.agent_id,'') AS agent_id,COALESCE(p.revision,0) AS pending_revision,COALESCE(p.applied_revision,0) AS applied_revision").Joins("JOIN wb_scale sc ON sc.site_id=s.id").Joins("LEFT JOIN wb_agent_binding a ON a.scale_id=sc.id").Joins("LEFT JOIN wb_site_proposal p ON p.site_id=s.id").Order("s.id").Scan(&rows).Error
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
			if c.Key == "" {
				continue
			}
			var count int64
			if err := tx.Model(&Scale{}).Where("id=?", c.ScaleID).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("Agent %s 的地磅不存在，请先迁移数据库", c.ID)
			}
			row := AgentBinding{ScaleID: c.ScaleID, AgentID: c.ID, AgentKey: c.Key}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *Repository) AgentConfigs(ctx context.Context, imageDir string) ([]agentbridge.Config, error) {
	var rows []AgentBinding
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]agentbridge.Config, 0, len(rows))
	for _, a := range rows {
		out = append(out, agentbridge.Config{ID: a.AgentID, ScaleID: a.ScaleID, Key: a.AgentKey, ImageDir: imageDir})
	}
	return out, nil
}
func (r *Repository) AgentConfig(ctx context.Context, scale int64, imageDir string) (agentbridge.Config, error) {
	var a AgentBinding
	err := r.db.WithContext(ctx).Table("wb_agent_binding a").Select("a.*").Joins("JOIN wb_scale sc ON sc.id=a.scale_id AND sc.enabled=1").Joins("JOIN wb_site s ON s.id=sc.site_id AND s.enabled=1").Where("a.scale_id=?", scale).Take(&a).Error
	return agentbridge.Config{ID: a.AgentID, ScaleID: a.ScaleID, Key: a.AgentKey, ImageDir: imageDir}, err
}
func (r *Repository) SaveSiteConfiguration(ctx context.Context, id int64, in SiteConfigurationInput, e audit.Event, connected func(int64) bool) (SiteConfiguration, error) {
	var out SiteConfiguration
	connectedNow := false
	if connected != nil && id > 0 && in.Enabled == 0 {
		var sc Scale
		if err := r.db.WithContext(ctx).Where("site_id=?", id).Take(&sc).Error; err != nil {
			return out, err
		}
		connectedNow = connected(sc.ID)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// This existing singleton also serializes allocation of site IDs on PostgreSQL.
		if _, err := lockRow[AgentSetting](tx, 1); err != nil {
			return err
		}
		site := Site{}
		scale := Scale{}
		binding := AgentBinding{}
		if id > 0 {
			var err error
			site, err = lockRow[Site](tx, id)
			if err != nil {
				return err
			}
			if site.Version != in.Version {
				return ErrConflict
			}
			if err = tx.Where("site_id=?", id).Take(&scale).Error; err != nil {
				return err
			}
			res := tx.Where("scale_id=?", scale.ID).Limit(1).Find(&binding)
			if res.Error != nil {
				return res.Error
			}
			if site.Code != in.Code || scale.Code != in.ScaleCode || (binding.AgentID != "" && binding.AgentID != in.AgentID) {
				return RejectionError("厂区、地磅及已有 Agent 编号不可修改，请保留原身份")
			}
		} else {
			if err := tx.Model(&Site{}).Select("COALESCE(MAX(id),0)+1").Scan(&site.ID).Error; err != nil {
				return err
			}
			site.Code = in.Code
			scale.Code = in.ScaleCode
			scale.SiteID = site.ID
		}
		var node struct{ Role string }
		if err := tx.Table("wb_node_identity").Where("id=1").Find(&node).Error; err != nil {
			return err
		}
		if node.Role == "central" && id > 0 {
			if binding.AgentID == "" {
				binding = AgentBinding{ScaleID: scale.ID, AgentID: in.AgentID, AgentKey: agentbridge.ID()}
				if err := tx.Create(&binding).Error; err != nil {
					return err
				}
			}
			site.Version++
			if err := tx.Model(&Site{}).Where("id=?", id).Update("version", site.Version).Error; err != nil {
				return err
			}
			if err := saveProposal(tx, site, scale, binding, in); err != nil {
				return err
			}
			out = SiteConfiguration{Site: site, ScaleID: scale.ID, ScaleCode: scale.Code, ScaleName: scale.Name, AgentID: binding.AgentID}
			e.Action = "site.configuration.propose"
			e.Resource = "weighing.site"
			e.ResourceID = site.ID
			e.Summary = string(snapshot(in))
			return audit.RecordOn(ctx, tx, e)
		}
		if in.Enabled == 0 {
			reasons := []string{}
			var count int64
			if err := tx.Model(&Task{}).Where("site_id=? AND status='awaiting_second'", site.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				reasons = append(reasons, fmt.Sprintf("%d 笔未完成任务", count))
			}
			if err := tx.Table("wb_print_attempt p").Joins("JOIN wb_ticket t ON t.id=p.ticket_id").Where("t.site_id=? AND p.closed_at IS NULL", site.ID).Count(&count).Error; err != nil {
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
		site.ServerURL = in.ServerURL
		site.Name = in.Name
		site.Enabled = in.Enabled
		site.Version++
		if id == 0 {
			if err := tx.Create(&site).Error; err != nil {
				return normalizeSiteError(err)
			}
		} else {
			if err := tx.Save(&site).Error; err != nil {
				return normalizeSiteError(err)
			}
		}
		scale.Name = in.ScaleName
		scale.Enabled = in.Enabled
		if scale.ID == 0 {
			if err := tx.Create(&scale).Error; err != nil {
				return normalizeSiteError(err)
			}
		} else {
			if err := tx.Model(&Scale{}).Where("id=?", scale.ID).Updates(map[string]any{"name": scale.Name, "enabled": scale.Enabled}).Error; err != nil {
				return normalizeSiteError(err)
			}
		}
		if binding.AgentID == "" {
			binding = AgentBinding{ScaleID: scale.ID, AgentID: in.AgentID, AgentKey: agentbridge.ID()}
			if err := tx.Create(&binding).Error; err != nil {
				return normalizeSiteError(err)
			}
		}
		if node.Role == "central" {
			if err := saveProposal(tx, site, scale, binding, in); err != nil {
				return err
			}
		}
		out = SiteConfiguration{Site: site, ScaleID: scale.ID, ScaleCode: scale.Code, ScaleName: scale.Name, AgentID: binding.AgentID}
		e.Action = "site.save"
		e.Resource = "weighing.site"
		e.ResourceID = site.ID
		e.Summary = string(snapshot(out))
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
func (r *Repository) ExportAgentConnection(ctx context.Context, id int64, e audit.Event) (agentconfig.Connection, error) {
	var out agentconfig.Connection
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Table("wb_site s").Select("s.server_url,s.id AS site_id,s.code AS site_code,s.name AS site_name,sc.id AS scale_id,sc.code AS scale_code,sc.name AS scale_name,a.agent_id,a.agent_key").Joins("JOIN wb_scale sc ON sc.site_id=s.id").Joins("JOIN wb_agent_binding a ON a.scale_id=sc.id").Where("s.id=?", id).Take(&out)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return RejectionError("请先保存该厂区的 Agent 配置")
		}
		if result.Error != nil {
			return result.Error
		}
		setting, err := findOne[AgentSetting](tx, 1)
		if err != nil {
			return err
		}
		if out.ServerURL == "" {
			var node struct{ Role string }
			if err := tx.Table("wb_node_identity").Where("id=1").Find(&node).Error; err != nil {
				return err
			}
			if node.Role == "central" {
				return RejectionError("请先为该厂区设置本厂 Server 地址并在现场应用")
			}
			out.ServerURL = setting.ServerURL
		}
		if err = out.Validate(); err != nil {
			return RejectionError("请先设置有效的现场 Server 地址和 Agent 绑定")
		}
		e.Action = "agent.configuration.export"
		e.Resource = "weighing.site"
		e.ResourceID = id
		e.Summary = string(snapshot(map[string]any{"siteId": id, "agentId": out.AgentID}))
		return audit.RecordOn(ctx, tx, e)
	})
	return out, err
}
