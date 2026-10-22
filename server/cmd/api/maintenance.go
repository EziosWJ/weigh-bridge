package main

import (
	"context"

	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// newMaintenanceConfigValidator binds the maintenance UI to the one local
// Agent identity stored in the standalone site's database. Display names stay
// editable through site administration and do not form part of this binding.
func newMaintenanceConfigValidator(repository *weighing.Repository, siteID int64) agentmaintenance.BusinessConfigValidator {
	return func(ctx context.Context, candidate agentmaintenance.Config) error {
		return repository.ValidateMaintenanceConfig(ctx, candidate, siteID)
	}
}
