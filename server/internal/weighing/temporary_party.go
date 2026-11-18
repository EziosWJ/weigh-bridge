package weighing

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
	"gorm.io/gorm"
)

type TemporaryPartyInput struct {
	Name    string   `json:"name"`
	Nature  string   `json:"nature"`
	Roles   []string `json:"roles"`
	Version int64    `json:"version"`
}

func (s *Service) SaveTemporaryParty(ctx context.Context, id int64, input TemporaryPartyInput, event audit.Event) (Party, error) {
	input.Name = strings.TrimSpace(input.Name)
	if id < 0 || !validName(input.Name) || (input.Nature != "enterprise" && input.Nature != "individual") ||
		(id > 0 && (input.Version < 1 || input.Version >= math.MaxInt32)) ||
		len(input.Roles) == 0 || len(input.Roles) > 3 {
		return Party{}, ErrInvalid
	}
	roles := map[string]bool{}
	for _, role := range input.Roles {
		if !slices.Contains([]string{"supplier", "customer", "carrier"}, role) || roles[role] {
			return Party{}, ErrInvalid
		}
		roles[role] = true
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return Party{}, ErrForbidden
	}
	party, err := repo.saveTemporaryParty(ctx, id, input, roles, event)
	if err == nil {
		s.wakeTemporaryPartySync()
	}
	return party, err
}

func (r *Repository) saveTemporaryParty(ctx context.Context, id int64, input TemporaryPartyInput, roles map[string]bool, event audit.Event) (Party, error) {
	var party Party
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize with identity edits, configuration changes and snapshot apply.
		if _, err := lockRow[AgentSetting](tx, 1); err != nil {
			return err
		}
		identity, err := localsite.Read(ctx, tx)
		if err != nil {
			return err
		}
		if identity.SiteCode == "" || !mayOperate(ctx, identity.SiteCode) {
			return ErrForbidden
		}
		site, err := lockRowWhere[Site](tx, "site_code = ?", identity.SiteCode)
		if err != nil {
			return err
		}
		if site.Enabled != 1 {
			return ErrForbidden
		}
		var state masterSyncState
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if !state.ConfigEnabled || !state.Bound {
			return RejectionError("完成首次总部资料同步后才能登记或更正临时往来主体")
		}

		var before Party
		if id > 0 {
			party, err = lockRow[Party](tx, id)
			if err != nil {
				return err
			}
			if party.TemporarySiteCode != identity.SiteCode {
				return ErrNotFound
			}
			if party.TemporaryState != "pending" && party.TemporaryState != "needs_info" {
				return RejectionError("仅可更正待确认或待补资料的临时往来主体")
			}
			before = party
			decorateParty(&before, nil)
		} else {
			party.Code = "T-P-" + agentbridge.ID()
			party.TemporarySiteCode = identity.SiteCode
		}
		party.MasterMeta, err = meta(party.MasterMeta, id, input.Version, event.Metadata.ActorID)
		if err != nil {
			return err
		}
		party.Enabled = 1
		party.TemporaryState = "pending"
		party.Name = input.Name
		party.Nature = input.Nature
		party.IsSupplier = flag(roles["supplier"])
		party.IsCustomer = flag(roles["customer"])
		party.IsCarrier = flag(roles["carrier"])
		if err := tx.Save(&party).Error; err != nil {
			return err
		}
		if err := enqueueTemporaryPartyRegistration(tx, party.TemporarySiteCode, party); err != nil {
			return err
		}
		decorateParty(&party, nil)
		event.Action = "temporary-party.register"
		if id > 0 {
			event.Action = "temporary-party.correct"
		}
		event.Resource = "weighing.temporary-party"
		event.ResourceID = party.ID
		return masterEvent(ctx, tx, event, before, party)
	})
	return party, err
}
