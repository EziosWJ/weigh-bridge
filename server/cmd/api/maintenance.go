package main

import (
	"context"
	"net"

	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func openMaintenance(cfg config.Config, repository *weighing.Repository, dir string) (*agentmaintenance.Service, agentmaintenance.BusinessConfigValidator, error) {
	if cfg.Environment == config.EnvironmentDev && cfg.Node.Role == "" {
		scaleID := cfg.Weighing.ScaleID
		if len(cfg.Weighing.Agents) > 0 {
			scaleID = cfg.Weighing.Agents[0].ScaleID
		}
		host, port, err := net.SplitHostPort(cfg.HTTP.Address)
		if err != nil {
			return nil, nil, err
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		serverURL := "http://" + net.JoinHostPort(host, port)
		draft, err := repository.DevelopmentMaintenanceConfig(context.Background(), scaleID, serverURL)
		if err != nil {
			return nil, nil, err
		}
		service, err := agentmaintenance.OpenWithDraft(dir, nil, draft)
		validator := func(ctx context.Context, candidate agentmaintenance.Config) error {
			return repository.ValidateDevelopmentMaintenanceConfig(ctx, candidate, scaleID, serverURL)
		}
		return service, validator, err
	}
	service, err := agentmaintenance.Open(dir, nil)
	return service, newMaintenanceConfigValidator(repository, cfg.Node.SiteID), err
}

// newMaintenanceConfigValidator binds the maintenance UI to the one local
// Agent identity stored in the standalone site's database. Display names stay
// editable through site administration and do not form part of this binding.
func newMaintenanceConfigValidator(repository *weighing.Repository, siteID int64) agentmaintenance.BusinessConfigValidator {
	return func(ctx context.Context, candidate agentmaintenance.Config) error {
		return repository.ValidateMaintenanceConfig(ctx, candidate, siteID)
	}
}
