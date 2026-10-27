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
	code, identityErr := s.LocalSiteCode(ctx)
	if identityErr != nil {
		return nil, identityErr
	}
	out := []SiteConfiguration{}
	for _, row := range rows {
		if row.Code == code {
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
func (s *Service) SaveSiteConfiguration(ctx context.Context, in SiteConfigurationInput, e audit.Event) (SiteConfiguration, error) {
	// ADR 0029: serverUrl is a compatibility field. It is ignored here and can
	// never become an editable source for the Agent connection address.
	in.ServerURL = ""
	code, err := s.LocalSiteCode(ctx)
	if err != nil {
		return SiteConfiguration{}, err
	}
	if code == "" {
		return SiteConfiguration{}, ErrNotFound
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.ScaleCode = strings.TrimSpace(in.ScaleCode)
	in.ScaleName = strings.TrimSpace(in.ScaleName)
	in.AgentID = strings.TrimSpace(in.AgentID)
	invalidCode := func(value string) bool { return value != "" && !siteCodePattern.MatchString(value) }
	invalidAgent := func(value string) bool { return value != "" && !agentIDPattern.MatchString(value) }
	if invalidCode(in.Code) || invalidCode(in.ScaleCode) || invalidAgent(in.AgentID) || !validName(in.Name) || !validName(in.ScaleName) || (in.Enabled != 0 && in.Enabled != 1) || in.Version < 1 {
		return SiteConfiguration{}, ErrInvalid
	}
	current, err := s.LocalStatus(ctx)
	if err != nil {
		return SiteConfiguration{}, err
	}
	changeCode := (in.Code != "" && in.Code != current.SiteCode) || (in.ScaleCode != "" && in.ScaleCode != current.ScaleCode)
	if changeCode {
		release, err := s.BeginMaintenance(ctx)
		if err != nil {
			return SiteConfiguration{}, err
		}
		defer release()
		s.printMu.Lock()
		defer s.printMu.Unlock()
	}
	var out SiteConfiguration
	perform := func() error {
		var connected func(string) bool
		if s.devices != nil {
			connected = s.devices.Connected
		}
		var saveErr error
		out, saveErr = s.store.SaveSiteConfiguration(ctx, in, e, connected)
		if saveErr == nil && changeCode && s.devices != nil {
			s.devices.DisconnectForIdentityChange(current.ScaleCode)
		}
		return saveErr
	}
	if s.devices != nil {
		err = s.devices.WithConnectionsPaused(perform)
	} else {
		err = perform()
	}
	return out, err
}
func (s *Service) ExportAgentConnection(ctx context.Context, serverURL string, e audit.Event) (agentconfig.Connection, error) {
	code, err := s.LocalSiteCode(ctx)
	if err != nil {
		return agentconfig.Connection{}, err
	}
	if code == "" {
		return agentconfig.Connection{}, ErrNotFound
	}
	url, err := agentprinter.CanonicalServerURL(strings.TrimSpace(serverURL))
	if err != nil {
		return agentconfig.Connection{}, RejectionError("请显式提供 Agent 主机可访问的业务服务地址")
	}
	return s.store.ExportAgentConnection(ctx, url, e)
}

// ListTicketSequences reports the recent daily ticket serials for the local
// factory. SetTicketWatermark raises one day's serial after a backup restore,
// keeps the greater value and records an audit event.
func (s *Service) ListTicketSequences(ctx context.Context) ([]DailySequence, error) {
	return s.store.ListTicketSequences(ctx)
}
func (s *Service) SetTicketWatermark(ctx context.Context, date string, lastIssued int64, e audit.Event) (DailySequence, error) {
	if strings.TrimSpace(date) == "" || lastIssued < 0 {
		return DailySequence{}, ErrInvalid
	}
	return s.store.SetTicketWatermark(ctx, strings.TrimSpace(date), lastIssued, e)
}
