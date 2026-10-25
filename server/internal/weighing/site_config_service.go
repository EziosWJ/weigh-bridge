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
	id, identityErr := s.LocalSiteID(ctx)
	if identityErr != nil {
		return nil, identityErr
	}
	out := []SiteConfiguration{}
	for _, row := range rows {
		if row.ID == id {
			out = append(out, row)
		}
	}
	return out, err
}
func (s *Service) AgentSettings(ctx context.Context) (AgentSetting, error) {
	return s.store.AgentSettings(ctx)
}

// SaveAgentSettings is retained only as a compatibility route. The Agent's own
// connection file is authoritative and agent-provision is the only writer, so
// this legacy database setting is no longer updated.
func (s *Service) SaveAgentSettings(context.Context, string, audit.Event) error {
	return RejectionError("Agent 连接地址请通过配对工具显式设置，本厂资料不再保存该地址")
}
func (s *Service) SaveSiteConfiguration(ctx context.Context, id int64, in SiteConfigurationInput, e audit.Event) (SiteConfiguration, error) {
	// ADR 0029: serverUrl is a compatibility field. It is ignored here and can
	// never become an editable source for the Agent connection address.
	in.ServerURL = ""
	localID, err := s.LocalSiteID(ctx)
	if err != nil {
		return SiteConfiguration{}, err
	}
	if id == 0 || id != localID {
		return SiteConfiguration{}, ErrNotFound
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.ScaleCode = strings.TrimSpace(in.ScaleCode)
	in.ScaleName = strings.TrimSpace(in.ScaleName)
	in.AgentID = strings.TrimSpace(in.AgentID)
	// Current pages edit only the two names and the enabled state. The legacy
	// identity fields remain accepted for old clients, but empty values mean
	// "keep the configured identity" rather than "write an empty code".
	invalidCode := func(value string) bool { return value != "" && !siteCodePattern.MatchString(value) }
	invalidAgent := func(value string) bool { return value != "" && !agentIDPattern.MatchString(value) }
	if invalidCode(in.Code) || invalidCode(in.ScaleCode) || invalidAgent(in.AgentID) || !validName(in.Name) || !validName(in.ScaleName) || (in.Enabled != 0 && in.Enabled != 1) || (id > 0 && in.Version < 1) {
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
	if s.devices != nil {
		err = s.devices.WithConnectionsPaused(perform)
	} else {
		err = perform()
	}
	return out, err
}
func (s *Service) ExportAgentConnection(ctx context.Context, id int64, serverURL string, e audit.Event) (agentconfig.Connection, error) {
	localID, err := s.LocalSiteID(ctx)
	if err != nil {
		return agentconfig.Connection{}, err
	}
	if id == 0 || id != localID {
		return agentconfig.Connection{}, ErrNotFound
	}
	url, err := agentprinter.CanonicalServerURL(strings.TrimSpace(serverURL))
	if err != nil {
		return agentconfig.Connection{}, RejectionError("请显式提供 Agent 主机可访问的业务服务地址")
	}
	return s.store.ExportAgentConnection(ctx, id, url, e)
}
