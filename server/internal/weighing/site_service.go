package weighing

import (
	"context"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

func (s *Service) Sites(ctx context.Context, user int64) ([]Site, error) {
	code, err := s.LocalSiteCode(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.Sites(ctx, user)
	out := []Site{}
	for _, v := range rows {
		if v.Code == code {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *Service) SiteOptions(ctx context.Context) ([]Site, error) {
	code, err := s.LocalSiteCode(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.SiteOptions(ctx)
	out := []Site{}
	for _, v := range rows {
		if v.Code == code {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *Service) SiteGrants(ctx context.Context, user int64) ([]SiteGrant, error) {
	code, err := s.LocalSiteCode(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.SiteGrants(ctx, user)
	out := []SiteGrant{}
	for _, v := range rows {
		if v.SiteCode == code {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *Service) SaveSiteGrants(ctx context.Context, user int64, rows []SiteGrant, e audit.Event) error {
	code, err := s.LocalSiteCode(ctx)
	if err != nil {
		return err
	}
	if code == "" {
		return ErrForbidden
	}
	for _, v := range rows {
		if v.SiteCode != code {
			return ErrForbidden
		}
	}
	return s.store.SaveSiteGrants(ctx, user, rows, e)
}
func (s *Service) ResourceSite(ctx context.Context, kind string, id int64) (string, error) {
	return s.store.ResourceSite(ctx, kind, id)
}
func (s *Service) ScaleSite(ctx context.Context, scaleCode string) (string, error) {
	return s.store.ResourceScaleSite(ctx, scaleCode)
}
func (s *Service) RecordAgentKick(ctx context.Context, e audit.Event) error {
	return s.store.RecordAgentKick(ctx, e)
}
