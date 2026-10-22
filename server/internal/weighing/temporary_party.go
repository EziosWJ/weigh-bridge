package weighing

import (
	"context"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/auth"
	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TemporaryParty struct {
	PartyID          int64  `json:"partyId" gorm:"primaryKey;autoIncrement:false"`
	SiteID           int64  `json:"siteId"`
	State            string `json:"state"`
	Revision         int64  `json:"revision"`
	ReviewedRevision int64  `json:"reviewedRevision"`
	FormalPartyID    *int64 `json:"formalPartyId"`
	Reason           string `json:"reason"`
	Name             string `json:"name" gorm:"->"`
}

func (TemporaryParty) TableName() string { return "wb_temporary_party" }

type TemporaryReview struct {
	Revision      int64  `json:"revision"`
	Action        string `json:"action"`
	FormalPartyID *int64 `json:"formalPartyId"`
	Reason        string `json:"reason"`
}

func (s *Service) TemporaryParties(ctx context.Context) ([]TemporaryParty, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return nil, ErrInvalid
	}
	rows := []TemporaryParty{}
	q := repo.db.WithContext(ctx).Table("wb_temporary_party x").Select("x.*,p.name").Joins("JOIN wb_party p ON p.id=x.party_id")
	q = siteFilter(q, PageQuery{}, "x.site_id")
	err := q.Order("x.party_id DESC").Find(&rows).Error
	return rows, err
}
func (s *Service) SaveTemporaryParty(ctx context.Context, id int64, in PartyInput, e audit.Event) (Party, error) {
	if s.node.Role != "site" {
		return Party{}, ErrForbidden
	}
	// Validate with the same public master service; the transaction also commits
	// source and review state, so a temporary party cannot become an orphan master.
	repo, ok := s.store.(*Repository)
	if !ok {
		return Party{}, ErrInvalid
	}
	var out Party
	err := repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tmp TemporaryParty
		if id > 0 {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&tmp, "party_id=? AND site_id=?", id, s.node.SiteID).Error; err != nil {
				return ErrNotFound
			}
			if tmp.State != "returned" {
				return ErrConflict
			}
		}
		child := NewService(NewRepository(tx), nil)
		v, err := child.SaveParty(ctx, id, in, e)
		if err != nil {
			return err
		}
		out = v
		if id == 0 {
			tmp = TemporaryParty{PartyID: v.ID, SiteID: s.node.SiteID, State: "pending", Revision: 1}
		} else {
			tmp.State = "resubmitted"
			tmp.Revision++
			tmp.Reason = ""
		}
		return tx.Omit("Name").Save(&tmp).Error
	})
	return out, err
}
func (s *Service) ReviewTemporaryParty(ctx context.Context, id int64, in TemporaryReview, e audit.Event) error {
	if s.node.Role != "central" {
		return ErrForbidden
	}
	if in.Revision < 1 || !maxLength(in.Reason, 2000) {
		return ErrInvalid
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrInvalid
	}
	return repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tmp TemporaryParty
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&tmp, "party_id=?", id).Error; err != nil {
			return ErrNotFound
		}
		if tmp.Revision != in.Revision || (tmp.State != "pending" && tmp.State != "resubmitted") {
			return ErrConflict
		}
		switch in.Action {
		case "return":
			if in.Reason == "" {
				return ErrInvalid
			}
			tmp.State = "returned"
			tmp.Reason = in.Reason
		case "link":
			if in.FormalPartyID == nil || *in.FormalPartyID == id {
				return ErrInvalid
			}
			var n int64
			if err := tx.Table("wb_party").Where("id=? AND enabled=1 AND id NOT IN (SELECT party_id FROM wb_temporary_party)", *in.FormalPartyID).Count(&n).Error; err != nil {
				return err
			}
			if n != 1 {
				return ErrInvalid
			}
			tmp.State = "linked"
			tmp.FormalPartyID = in.FormalPartyID
		case "approve":
			source, err := findOne[Party](tx, id)
			if err != nil {
				return err
			}
			source.ID = 0
			source.Version = 1
			source.Code, err = nextNumber(tx, "party", "ZT")
			if err != nil {
				return err
			}
			if err = tx.Create(&source).Error; err != nil {
				return err
			}
			contacts := []Contact{}
			if err = tx.Where("party_id=?", id).Find(&contacts).Error; err != nil {
				return err
			}
			for _, c := range contacts {
				c.ID = 0
				c.PartyID = source.ID
				if err = tx.Create(&c).Error; err != nil {
					return err
				}
			}
			tmp.State = "approved"
			tmp.FormalPartyID = ptr(source.ID)
		default:
			return ErrInvalid
		}
		tmp.ReviewedRevision = tmp.Revision
		if err := tx.Omit("Name").Save(&tmp).Error; err != nil {
			return err
		}
		e.Resource = "weighing.temporary-party"
		e.ResourceID = id
		e.Action = "temporary-party." + in.Action
		e.Summary = string(snapshot(tmp))
		return masterEvent(ctx, tx, e, nil, tmp)
	})
}
func registerTemporaryRoutes(router gin.IRouter, h *Handler) {
	router.GET("/temporary-parties", rbac.RequirePermission(h.permissions, "weighing:master:read"), func(c *gin.Context) { v, e := h.service.TemporaryParties(c); writeResult(c, v, e) })
	save := func(c *gin.Context) {
		id := int64(0)
		var err error
		if c.Request.Method == "PUT" {
			id, err = pathID(c)
			if err != nil {
				writeResult(c, nil, err)
				return
			}
		}
		p, _ := auth.PrincipalFromContext(c.Request.Context())
		sites, e := h.service.Sites(c, p.UserID)
		allowed := false
		for _, site := range sites {
			if site.ID == h.service.node.SiteID && site.CanOperate {
				allowed = true
			}
		}
		if e != nil || !allowed {
			writeResult(c, nil, ErrForbidden)
			return
		}
		var in PartyInput
		if c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		v, e := h.service.SaveTemporaryParty(c, id, in, requestEvent(c))
		writeResult(c, v, e)
	}
	router.POST("/temporary-parties", rbac.RequirePermission(h.permissions, "weighing:temporary:create"), save)
	router.PUT("/temporary-parties/:id", rbac.RequirePermission(h.permissions, "weighing:temporary:create"), save)
	router.POST("/temporary-parties/:id/review", rbac.RequirePermission(h.permissions, "weighing:master:write"), func(c *gin.Context) {
		id, e := pathID(c)
		var in TemporaryReview
		if e != nil || c.ShouldBindJSON(&in) != nil {
			writeResult(c, nil, ErrInvalid)
			return
		}
		rows, e := h.service.TemporaryParties(c)
		allowed := false
		for _, row := range rows {
			if row.PartyID == id {
				allowed = true
			}
		}
		if e != nil || !allowed {
			writeResult(c, nil, ErrForbidden)
			return
		}
		writeResult(c, nil, h.service.ReviewTemporaryParty(c, id, in, requestEvent(c)))
	})
}
