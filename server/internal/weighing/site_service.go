package weighing

import (
	"context"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

func (s *Service) Sites(ctx context.Context, user int64) ([]Site, error) {
	rows, err := s.store.Sites(ctx, user)
	out := []Site{}
	for _, v := range rows {
		if s.node.Role == "site" && v.ID != s.node.SiteID {
			continue
		}
		if s.node.Role == "central" {
			v.CanOperate = false
		}
		out = append(out, v)
	}
	return out, err
}
func (s *Service) SiteOptions(ctx context.Context) ([]Site, error) {
	rows, err := s.store.SiteOptions(ctx)
	if s.node.Role != "site" {
		return rows, err
	}
	out := []Site{}
	for _, row := range rows {
		if row.ID == s.node.SiteID {
			out = append(out, row)
		}
	}
	return out, err
}
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
