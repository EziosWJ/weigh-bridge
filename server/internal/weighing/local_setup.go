package weighing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LocalSiteInput struct {
	SiteCode  string `json:"siteCode"`
	ScaleCode string `json:"scaleCode"`
	Name      string `json:"name"`
	ScaleName string `json:"scaleName"`
}

// InitializeLocalSite installs the head-office assigned codes as the one-time
// local identity (ADR 0033). Codes are fixed after this transaction; names
// default to internal labels and stay editable. The existing setting
// singleton serializes simultaneous first-time submissions.
func (s *Service) InitializeLocalSite(ctx context.Context, actor int64, in LocalSiteInput, e audit.Event) (SiteConfiguration, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return SiteConfiguration{}, ErrForbidden
	}
	in.SiteCode = strings.TrimSpace(in.SiteCode)
	in.ScaleCode = strings.TrimSpace(in.ScaleCode)
	if in.ScaleCode == "" {
		in.ScaleCode = "WB01"
	}
	in.Name = strings.TrimSpace(in.Name)
	in.ScaleName = strings.TrimSpace(in.ScaleName)
	if in.Name == "" {
		in.Name = "本厂"
	}
	if in.ScaleName == "" {
		in.ScaleName = "本厂地磅"
	}
	if !siteCodePattern.MatchString(in.SiteCode) || !siteCodePattern.MatchString(in.ScaleCode) || !validName(in.Name) || !validName(in.ScaleName) {
		return SiteConfiguration{}, RejectionError("厂区编码由总部预先分配、格式为字母数字组合，地磅编码默认 WB01，请核对后填写")
	}
	var out SiteConfiguration
	err := repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockRow[AgentSetting](tx, 1); err != nil {
			return err
		}
		identity, err := localsite.Read(ctx, tx)
		if err != nil {
			return err
		}
		if identity.SiteCode != "" {
			return ErrConflict
		}
		// Reference master data (categories, materials, parties) may be
		// pre-seeded on a fresh installation; only transaction evidence blocks
		// installing the identity.
		for _, table := range []string{"wb_site", "wb_scale", "wb_transport_task", "wb_sample", "wb_weigh_record", "wb_ticket", "wb_print_attempt", "wb_operation", "wb_business_event", "wb_number_sequence"} {
			var count int64
			if err := tx.Table(table).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return RejectionError("缺少本厂身份但已有业务数据（" + table + "），请检查数据库与备份，不可自动重建身份")
			}
		}
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		site := Site{Code: in.SiteCode, Name: in.Name, Enabled: 1, Version: 1}
		if err := tx.Create(&site).Error; err != nil {
			return normalizeSiteError(err)
		}
		scale := Scale{SiteCode: in.SiteCode, Code: in.ScaleCode, Name: in.ScaleName, Enabled: 1}
		if err := tx.Create(&scale).Error; err != nil {
			return normalizeSiteError(err)
		}
		identityRow := localsite.Identity{ID: 1, SiteCode: in.SiteCode, ScaleCode: in.ScaleCode}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"site_code", "scale_code"}),
		}).Create(&identityRow).Error; err != nil {
			return err
		}
		if err := saveIdentityConfigs(tx, in.SiteCode, in.ScaleCode, in.Name, in.ScaleName); err != nil {
			return err
		}
		binding := AgentBinding{LocalDevice: true, ScaleCode: in.ScaleCode, AgentID: "agent-" + in.ScaleCode, AgentKey: hex.EncodeToString(secret)}
		if err := tx.Create(&binding).Error; err != nil {
			return normalizeSiteError(err)
		}
		if err := tx.Exec("INSERT INTO wb_local_user_access(user_id,can_operate) VALUES (?,1)", actor).Error; err != nil {
			return err
		}
		out = SiteConfiguration{Site: site, ScaleCode: scale.Code, ScaleName: scale.Name, AgentID: binding.AgentID}
		e.Action = "site.initialize"
		e.Resource = "weighing.site"
		e.ResourceID = 1
		e.Summary = string(snapshot(out))
		return audit.RecordOn(ctx, tx, e)
	})
	return out, err
}
