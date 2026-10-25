package weighing

import (
	"context"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

func (s *Service) Sites(ctx context.Context, user int64) ([]Site, error) {
	id, err := s.LocalSiteID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.Sites(ctx, user)
	out := []Site{}
	for _, v := range rows {
		if v.ID == id {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *Service) SiteOptions(ctx context.Context) ([]Site, error) {
	id, err := s.LocalSiteID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.SiteOptions(ctx)
	out := []Site{}
	for _, v := range rows {
		if v.ID == id {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *Service) SiteGrants(ctx context.Context, user int64) ([]SiteGrant, error) {
	id, err := s.LocalSiteID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.SiteGrants(ctx, user)
	out := []SiteGrant{}
	for _, v := range rows {
		if v.SiteID == id {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *Service) SaveSiteGrants(ctx context.Context, user int64, rows []SiteGrant, e audit.Event) error {
	id, err := s.LocalSiteID(ctx)
	if err != nil {
		return err
	}
	if id == 0 {
		return ErrForbidden
	}
	for _, v := range rows {
		if v.SiteID != id {
			return ErrForbidden
		}
	}
	return s.store.SaveSiteGrants(ctx, user, rows, e)
}
func (s *Service) ResourceSite(ctx context.Context, kind string, id int64) (int64, error) {
	return s.store.ResourceSite(ctx, kind, id)
}
func (s *Service) RecordAgentKick(ctx context.Context, e audit.Event) error {
	return s.store.RecordAgentKick(ctx, e)
}
