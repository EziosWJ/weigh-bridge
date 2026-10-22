package weighing

import (
	"context"
	"encoding/json"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SiteProposal struct {
	SiteID          int64  `json:"siteId" gorm:"primaryKey;autoIncrement:false"`
	Revision        int64  `json:"revision"`
	AppliedRevision int64  `json:"appliedRevision"`
	Configuration   string `json:"-"`
}

func (SiteProposal) TableName() string { return "wb_site_proposal" }

type proposalContent struct {
	Input    SiteConfigurationInput
	Binding  AgentBinding
	ScaleID  int64
	AgentKey string
}

func saveProposal(tx *gorm.DB, site Site, scale Scale, binding AgentBinding, in SiteConfigurationInput) error {
	p := SiteProposal{SiteID: site.ID}
	if err := tx.Where("site_id=?", site.ID).Find(&p).Error; err != nil {
		return err
	}
	p.Revision++
	data, err := json.Marshal(proposalContent{Input: in, Binding: binding, ScaleID: scale.ID, AgentKey: binding.AgentKey})
	if err != nil {
		return err
	}
	p.Configuration = string(data)
	return tx.Save(&p).Error
}
func (s *Service) ApplyConfiguration(ctx context.Context, revision int64, e audit.Event) error {
	if s.node.Role != "site" {
		return ErrForbidden
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrInvalid
	}
	perform := func() error {
		return repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var p SiteProposal
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, "site_id=?", s.node.SiteID).Error; err != nil {
				return ErrNotFound
			}
			if revision != p.Revision || p.AppliedRevision >= revision {
				return ErrConflict
			}
			var config proposalContent
			if json.Unmarshal([]byte(p.Configuration), &config) != nil {
				return ErrInvalid
			}
			site, err := findOne[Site](tx, s.node.SiteID)
			if err != nil {
				return err
			}
			// Reuse the existing open-task, unresolved-print and live-Agent checks.
			input := config.Input
			input.Version = site.Version
			var connected func(int64) bool
			if s.devices != nil {
				connected = s.devices.Connected
			}
			if _, err := NewRepository(tx).SaveSiteConfiguration(ctx, site.ID, input, e, connected); err != nil {
				return err
			}
			config.Binding.AgentKey = config.AgentKey
			if config.Binding.ScaleID != config.ScaleID {
				return ErrInvalid
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scale_id"}}, DoUpdates: clause.AssignmentColumns([]string{"agent_id", "agent_key"})}).Create(&config.Binding).Error; err != nil {
				return err
			}
			if input.ServerURL != "" {
				if err := tx.Model(&AgentSetting{}).Where("id=1").Update("server_url", input.ServerURL).Error; err != nil {
					return err
				}
			}
			p.AppliedRevision = revision
			return tx.Save(&p).Error
		})
	}
	if s.devices != nil {
		return s.devices.WithConnectionsPaused(perform)
	}
	return perform()
}
func registerConfigurationApplication(router gin.IRouter, h *Handler) {
	router.GET("/configuration/pending", rbac.RequirePermission(h.permissions, "weighing:site:manage"), func(c *gin.Context) {
		repo, ok := h.service.store.(*Repository)
		if !ok {
			writeResult(c, nil, ErrInvalid)
			return
		}
		var p SiteProposal
		err := repo.db.WithContext(c.Request.Context()).Where("site_id=?", h.service.node.SiteID).Find(&p).Error
		var content proposalContent
		_ = json.Unmarshal([]byte(p.Configuration), &content)
		writeResult(c, map[string]any{"revision": p.Revision, "appliedRevision": p.AppliedRevision, "configuration": content.Input}, err)
	})
	router.POST("/configuration/apply", rbac.RequirePermission(h.permissions, "weighing:site:manage"), func(c *gin.Context) {
		var in struct {
			Revision int64 `json:"revision"`
		}
		if c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		writeResult(c, nil, h.service.ApplyConfiguration(c, in.Revision, requestEvent(c)))
	})
}
