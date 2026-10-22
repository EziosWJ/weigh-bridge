package weighing

import (
	"context"
	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

type Service struct {
	imageDir           string
	maintenanceEnabled bool
	node               config.NodeConfig
	printMu            sync.Mutex
	store              Store
	devices            *agentbridge.Hub
}

func NewService(store Store, devices *agentbridge.Hub) *Service {
	s := &Service{store: store, devices: devices}
	if devices != nil {
		if repo, ok := store.(*Repository); ok {
			devices.SetPrintHandler(repo.AgentPrintResult)
		}
	}
	return s
}
func (s *Service) SetMaintenanceEnabled(enabled bool) { s.maintenanceEnabled = enabled }
func (s *Service) Agent() *agentbridge.Hub            { return s.devices }
func (s *Service) Categories(ctx context.Context, q PageQuery) (Page[Category], error) {
	return s.store.Categories(ctx, q)
}
func (s *Service) Materials(ctx context.Context, q PageQuery) (Page[Material], error) {
	return s.store.Materials(ctx, q)
}
func (s *Service) Parties(ctx context.Context, q PageQuery) (Page[Party], error) {
	return s.store.Parties(ctx, q)
}
func (s *Service) FindCategory(ctx context.Context, id int64) (Category, error) {
	return s.store.FindCategory(ctx, id)
}
func (s *Service) FindMaterial(ctx context.Context, id int64) (Material, error) {
	return s.store.FindMaterial(ctx, id)
}
func (s *Service) FindParty(ctx context.Context, id int64) (Party, error) {
	return s.store.FindParty(ctx, id)
}
func (s *Service) Scales(ctx context.Context) ([]Scale, error) { return s.store.Scales(ctx) }
func (s *Service) SaveCategory(ctx context.Context, id int64, in CategoryInput, e audit.Event) (Category, error) {
	in.Name = strings.TrimSpace(in.Name)
	if !validName(in.Name) || (id > 0 && in.Version < 1) {
		return Category{}, ErrInvalid
	}
	e.Action = "category.save"
	e.Resource = "weighing.category"
	return s.store.SaveCategory(ctx, id, in, e)
}
func (s *Service) SaveMaterial(ctx context.Context, id int64, in MaterialInput, e audit.Event) (Material, error) {
	in.Name = strings.TrimSpace(in.Name)
	if !validName(in.Name) || !maxLength(in.Spec, 200) || !maxLength(in.Remark, 2000) || (id > 0 && in.Version < 1) || (in.CategoryID != nil && *in.CategoryID < 1) {
		return Material{}, ErrInvalid
	}
	e.Action = "material.save"
	e.Resource = "weighing.material"
	return s.store.SaveMaterial(ctx, id, in, e)
}
func (s *Service) SaveParty(ctx context.Context, id int64, in PartyInput, e audit.Event) (Party, error) {
	if s.node.Role == "central" && id > 0 {
		if repo, ok := s.store.(*Repository); ok {
			var n int64
			if err := repo.db.WithContext(ctx).Table("wb_temporary_party").Where("party_id=?", id).Count(&n).Error; err != nil {
				return Party{}, err
			}
			if n > 0 {
				return Party{}, ErrForbidden
			}
		}
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validName(in.Name) || (id > 0 && in.Version < 1) || !slices.Contains([]string{"enterprise", "individual"}, in.Nature) || len(in.Roles) == 0 || len(in.Roles) > 3 || !maxLength(in.ShortName, 100) || !maxLength(in.Phone, 50) || !maxLength(in.Address, 500) || !maxLength(in.CreditCode, 50) || !maxLength(in.Remark, 2000) || len(in.Contacts) > 100 {
		return Party{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, role := range in.Roles {
		if !slices.Contains([]string{"supplier", "customer", "carrier"}, role) || seen[role] {
			return Party{}, ErrInvalid
		}
		seen[role] = true
	}
	defaults := 0
	ids := map[int64]bool{}
	for i := range in.Contacts {
		c := &in.Contacts[i]
		c.Name = strings.TrimSpace(c.Name)
		if !validName(c.Name) || c.ID < 0 || (c.ID > 0 && ids[c.ID]) || !maxLength(c.Phone, 50) || !maxLength(c.Purpose, 200) || !maxLength(c.Remark, 2000) {
			return Party{}, ErrInvalid
		}
		ids[c.ID] = true
		if c.IsDefault {
			defaults++
		}
	}
	if len(in.Contacts) > 0 && defaults != 1 {
		return Party{}, ErrInvalid
	}
	e.Action = "party.save"
	e.Resource = "weighing.party"
	return s.store.SaveParty(ctx, id, in, e)
}
func (s *Service) DeleteMaster(ctx context.Context, kind string, id, version int64, e audit.Event) error {
	if id < 1 || version < 1 {
		return ErrInvalid
	}
	e.Action = kind + ".delete"
	e.Resource = "weighing." + kind
	e.ResourceID = id
	return s.store.DeleteMaster(ctx, kind, id, version, e)
}
func (s *Service) SetMasterStatus(ctx context.Context, kind string, id, version int64, enabled int, e audit.Event) error {
	if id < 1 || version < 1 || (enabled != 0 && enabled != 1) {
		return ErrInvalid
	}
	e.Action = kind + ".status"
	e.Resource = "weighing." + kind
	e.ResourceID = id
	return s.store.SetMasterStatus(ctx, kind, id, version, enabled, e)
}
func validName(v string) bool        { return v != "" && maxLength(v, 100) }
func maxLength(v string, n int) bool { return utf8.RuneCountInString(v) <= n }
func flag(v bool) int {
	if v {
		return 1
	}
	return 0
}
func enabled(v *bool) int {
	if v == nil {
		return 1
	}
	return flag(*v)
}

func (s *Service) SetNode(cfg config.NodeConfig, imageDir string) {
	s.node = cfg
	s.imageDir = imageDir
}
