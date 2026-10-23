package weighing

import (
	"context"
	"fmt"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

// DevelopmentMaintenanceConfig uses the existing selected scale and database
// binding. It does not create an identity, rotate a key or change the database.
// The development API address is authoritative instead of a site's LAN URL.
func (r *Repository) DevelopmentMaintenanceConfig(ctx context.Context, scaleID int64, serverURL string) (agentprinter.Config, error) {
	cfg := agentprinter.Defaults()
	var err error
	cfg.ServerURL, err = agentprinter.CanonicalServerURL(serverURL)
	if err != nil {
		return cfg, err
	}
	var identity struct {
		SiteID                                  int64
		SiteCode, SiteName                      string
		ScaleID                                 int64
		ScaleCode, ScaleName, AgentID, AgentKey string
	}
	err = r.db.WithContext(ctx).Table("wb_scale sc").Select("s.id AS site_id,s.code AS site_code,s.name AS site_name,sc.id AS scale_id,sc.code AS scale_code,sc.name AS scale_name,a.agent_id,a.agent_key").
		Joins("JOIN wb_site s ON s.id=sc.site_id").Joins("JOIN wb_agent_binding a ON a.scale_id=sc.id").Where("sc.id=?", scaleID).Take(&identity).Error
	if err != nil {
		return cfg, fmt.Errorf("读取开发地磅设备身份失败，请先执行迁移并检查 Agent 配置: %w", err)
	}
	cfg.SiteID, cfg.SiteCode, cfg.SiteName = identity.SiteID, identity.SiteCode, identity.SiteName
	cfg.ScaleID, cfg.ScaleCode, cfg.ScaleName = identity.ScaleID, identity.ScaleCode, identity.ScaleName
	cfg.AgentID, cfg.AgentKey = identity.AgentID, identity.AgentKey
	return cfg, agentprinter.ValidateConfig(cfg)
}

func (r *Repository) ValidateDevelopmentMaintenanceConfig(ctx context.Context, candidate agentprinter.Config, scaleID int64, serverURL string) error {
	expected, err := r.DevelopmentMaintenanceConfig(ctx, scaleID, serverURL)
	if err != nil {
		return err
	}
	identity := maintenanceIdentity{SiteID: expected.SiteID, ScaleID: expected.ScaleID, SiteCode: expected.SiteCode, ScaleCode: expected.ScaleCode, ServerURL: expected.ServerURL, AgentID: expected.AgentID, AgentKey: expected.AgentKey}
	if !matchesMaintenanceIdentity(candidate, identity) {
		return fmt.Errorf("开发维护配置不能修改已绑定的厂区、地磅、服务地址或身份密钥")
	}
	return nil
}

// BoundMaintenanceConfig resolves identity without opening an API-local device
// directory. Agent-owned paths and runtime parameters are read over the wire.
func (r *Repository) BoundMaintenanceConfig(ctx context.Context, siteID, scaleID int64, serverURL string) (agentprinter.Config, error) {
	if siteID > 0 {
		var row struct{ ID int64 }
		if err := r.db.WithContext(ctx).Table("wb_scale").Select("id").Where("site_id=?", siteID).Order("id").Take(&row).Error; err != nil {
			return agentprinter.Config{}, err
		}
		scaleID = row.ID
	}
	return r.DevelopmentMaintenanceConfig(ctx, scaleID, serverURL)
}
