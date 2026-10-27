package weighing

import (
	"context"
	"fmt"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

// DevelopmentMaintenanceConfig uses the existing selected scale and database
// binding. It does not create an identity, rotate a key or change the database.
// The development API address is authoritative instead of a site's LAN URL.
func (r *Repository) DevelopmentMaintenanceConfig(ctx context.Context, scaleCode string, serverURL string) (agentprinter.Config, error) {
	cfg := agentprinter.Defaults()
	var err error
	cfg.ServerURL, err = agentprinter.CanonicalServerURL(serverURL)
	if err != nil {
		return cfg, err
	}
	var identity struct {
		SiteCode, SiteName                      string
		ScaleCode, ScaleName, AgentID, AgentKey string
	}
	err = r.db.WithContext(ctx).Table("wb_scale sc").Select("s.site_code,s.name AS site_name,sc.code AS scale_code,sc.name AS scale_name,a.agent_id,a.agent_key").
		Joins("JOIN wb_site s ON s.site_code=sc.site_code").Joins("JOIN wb_agent_binding a ON a.local_device=TRUE").Where("sc.code=?", scaleCode).Take(&identity).Error
	if err != nil {
		return cfg, fmt.Errorf("读取开发地磅设备身份失败，请先执行迁移并检查 Agent 配置: %w", err)
	}
	cfg.SiteCode, cfg.SiteName = identity.SiteCode, identity.SiteName
	cfg.ScaleCode, cfg.ScaleName = identity.ScaleCode, identity.ScaleName
	cfg.AgentID, cfg.AgentKey = identity.AgentID, identity.AgentKey
	return cfg, agentprinter.ValidateConfig(cfg)
}

func (r *Repository) ValidateDevelopmentMaintenanceConfig(ctx context.Context, candidate agentprinter.Config, scaleCode string, serverURL string) error {
	expected, err := r.DevelopmentMaintenanceConfig(ctx, scaleCode, serverURL)
	if err != nil {
		return err
	}
	identity := maintenanceIdentity{SiteCode: expected.SiteCode, ScaleCode: expected.ScaleCode, ServerURL: expected.ServerURL, AgentID: expected.AgentID, AgentKey: expected.AgentKey}
	if !matchesMaintenanceIdentity(candidate, identity) {
		return fmt.Errorf("开发维护配置不能修改已绑定的厂区、地磅、服务地址或身份密钥")
	}
	return nil
}

// BoundMaintenanceConfig resolves identity without opening an API-local device
// directory. Agent-owned paths and runtime parameters are read over the wire.
func (r *Repository) BoundMaintenanceConfig(ctx context.Context, siteCode, scaleCode string, serverURL string) (agentprinter.Config, error) {
	if scaleCode == "" && siteCode != "" {
		var row struct{ Code string }
		if err := r.db.WithContext(ctx).Table("wb_scale").Select("code").Where("site_code=?", siteCode).Order("code").Take(&row).Error; err != nil {
			return agentprinter.Config{}, err
		}
		scaleCode = row.Code
	}
	return r.DevelopmentMaintenanceConfig(ctx, scaleCode, serverURL)
}
