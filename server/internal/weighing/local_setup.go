package weighing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"gorm.io/gorm"
)

type LocalSiteInput struct {
	SiteID int64 `json:"siteId"`
	SiteConfigurationInput
}

// InitializeLocalSite creates the sole factory and its scale in one transaction.
// The operator supplies only the site and scale names; identity numbers, codes
// and the Agent binding are generated here and stay fixed afterwards. Legacy
// request fields (siteId, code, scaleCode, agentId) are accepted but ignored so
// older clients cannot invent identities.
// The existing setting singleton serializes simultaneous first-time submissions.
func (s *Service) InitializeLocalSite(ctx context.Context, actor int64, in LocalSiteInput, e audit.Event) (SiteConfiguration, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return SiteConfiguration{}, ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	in.ScaleName = strings.TrimSpace(in.ScaleName)
	if !validName(in.Name) || !validName(in.ScaleName) {
		return SiteConfiguration{}, ErrInvalid
	}
	// ADR 0029: the Agent connection address lives in the Agent's own
	// connection file and is supplied to agent-provision. A legacy serverUrl in
	// this request is accepted but ignored, so it can never overwrite it.
	in.ServerURL = ""
	var out SiteConfiguration
	err := repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockRow[AgentSetting](tx, 1); err != nil {
			return err
		}
		identity, err := localsite.Read(ctx, tx)
		if err != nil {
			return err
		}
		if identity.SiteID > 0 {
			return ErrConflict
		}
		for _, table := range []string{"wb_site", "wb_scale", "wb_transport_task", "wb_sample", "wb_weigh_record", "wb_ticket", "wb_print_attempt", "wb_operation", "wb_business_event", "wb_number_sequence", "wb_material_category", "wb_material", "wb_party", "wb_party_contact"} {
			var count int64
			if err := tx.Table(table).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return RejectionError("本厂首次配置要求新业务库，请先核实并清理旧联调数据")
			}
		}
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		// Identity codes and the agent id are internal and generated once; they
		// are fixed after this transaction. The numeric site id still comes from
		// the request when a provisioning script supplies one (it prefixes all
		// business numbers and must distinguish installations); the web UI omits
		// it and gets the default.
		siteID := in.SiteID
		if siteID < 1 || siteID > 255 {
			siteID = 1
		}
		site := Site{ID: siteID, Code: fmt.Sprintf("SITE-%02d", siteID), Name: in.Name, Enabled: 1, Version: 1, ServerURL: in.ServerURL}
		if err := tx.Create(&site).Error; err != nil {
			return normalizeSiteError(err)
		}
		scale := Scale{ID: siteID, Code: fmt.Sprintf("SCALE-%02d", siteID), Name: in.ScaleName, SiteID: siteID, Enabled: 1}
		if err := tx.Create(&scale).Error; err != nil {
			return normalizeSiteError(err)
		}
		if err := tx.Create(&localsite.Identity{ID: 1, SiteID: siteID}).Error; err != nil {
			return err
		}
		binding := AgentBinding{ScaleID: scale.ID, AgentID: fmt.Sprintf("agent-%02d", siteID), AgentKey: hex.EncodeToString(secret)}
		if err := tx.Create(&binding).Error; err != nil {
			return normalizeSiteError(err)
		}
		if err := tx.Exec("INSERT INTO wb_user_site(user_id,site_id,can_operate) VALUES (?,?,1)", actor, siteID).Error; err != nil {
			return err
		}
		if err := tx.Exec("SELECT setval(pg_get_serial_sequence('wb_scale','id'),GREATEST((SELECT MAX(id) FROM wb_scale),1))").Error; err != nil {
			return err
		}
		out = SiteConfiguration{Site: site, ScaleID: scale.ID, ScaleCode: scale.Code, ScaleName: scale.Name, AgentID: binding.AgentID}
		e.Action = "site.initialize"
		e.Resource = "weighing.site"
		e.ResourceID = site.ID
		e.Summary = string(snapshot(out))
		return audit.RecordOn(ctx, tx, e)
	})
	return out, err
}
