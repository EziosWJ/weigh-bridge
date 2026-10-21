package weighing

import (
	"context"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

func (s *Service) Sites(ctx context.Context, user int64) ([]Site, error) {
	return s.store.Sites(ctx, user)
}
func (s *Service) SiteOptions(ctx context.Context) ([]Site, error) { return s.store.SiteOptions(ctx) }
func (s *Service) SiteGrants(ctx context.Context, user int64) ([]SiteGrant, error) {
	return s.store.SiteGrants(ctx, user)
}
func (s *Service) SaveSiteGrants(ctx context.Context, user int64, rows []SiteGrant, e audit.Event) error {
	return s.store.SaveSiteGrants(ctx, user, rows, e)
}
func (s *Service) ResourceSite(ctx context.Context, kind string, id int64) (int64, error) {
	return s.store.ResourceSite(ctx, kind, id)
}
func (s *Service) RecordAgentKick(ctx context.Context, e audit.Event) error {
	return s.store.RecordAgentKick(ctx, e)
}
