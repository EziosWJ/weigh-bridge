package weighing

import (
	"context"
	"regexp"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/agentconfig"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

var siteCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,40}$`)
var agentIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,80}$`)

func (s *Service) SiteConfigurations(ctx context.Context) ([]SiteConfiguration, error) {
	rows, err := s.store.SiteConfigurations(ctx)
	if s.node.Role != "site" {
		return rows, err
	}
	out := []SiteConfiguration{}
	for _, row := range rows {
		if row.ID == s.node.SiteID {
			out = append(out, row)
		}
	}
	return out, err
}
func (s *Service) AgentSettings(ctx context.Context) (AgentSetting, error) {
	return s.store.AgentSettings(ctx)
}
func (s *Service) SaveAgentSettings(ctx context.Context, value string, e audit.Event) error {
	url, err := agentprinter.CanonicalServerURL(strings.TrimSpace(value))
	if err != nil {
		return RejectionError("请输入有效的现场 Server 地址，例如 http://192.168.1.100:8900")
	}
	return s.store.SaveAgentSettings(ctx, url, e)
}
func (s *Service) SaveSiteConfiguration(ctx context.Context, id int64, in SiteConfigurationInput, e audit.Event) (SiteConfiguration, error) {
	if in.ServerURL != "" {
		url, err := agentprinter.CanonicalServerURL(in.ServerURL)
		if err != nil {
			return SiteConfiguration{}, ErrInvalid
		}
		in.ServerURL = url
	}
	if s.node.Role == "central" && in.ServerURL == "" {
		return SiteConfiguration{}, RejectionError("请填写该厂工控机上的本厂 Server 地址")
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.ScaleCode = strings.TrimSpace(in.ScaleCode)
	in.ScaleName = strings.TrimSpace(in.ScaleName)
	in.AgentID = strings.TrimSpace(in.AgentID)
	if !siteCodePattern.MatchString(in.Code) || !siteCodePattern.MatchString(in.ScaleCode) || !agentIDPattern.MatchString(in.AgentID) || !validName(in.Name) || !validName(in.ScaleName) || (in.Enabled != 0 && in.Enabled != 1) || (id > 0 && in.Version < 1) {
		return SiteConfiguration{}, ErrInvalid
	}
	var out SiteConfiguration
	perform := func() error {
		var err error
		var connected func(int64) bool
		if s.devices != nil {
			connected = s.devices.Connected
		}
		out, err = s.store.SaveSiteConfiguration(ctx, id, in, e, connected)
		return err
	}
	var err error
	if s.devices != nil {
		err = s.devices.WithConnectionsPaused(perform)
	} else {
		err = perform()
	}
	return out, err
}
func (s *Service) ExportAgentConnection(ctx context.Context, id int64, e audit.Event) (agentconfig.Connection, error) {
	return s.store.ExportAgentConnection(ctx, id, e)
}
