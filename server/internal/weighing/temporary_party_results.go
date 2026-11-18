package weighing

import (
	"errors"

	"github.com/EziosWJ/weighbridge/server/internal/mastersync"
	"gorm.io/gorm"
)

func applyTemporaryPartyResults(tx *gorm.DB, siteCode string, results []mastersync.TemporaryPartyResult) error {
	for _, result := range results {
		var temporary Party
		err := tx.Where("code = ? AND temporary_site_code = ? AND temporary_state IN ('pending', 'needs_info')", result.TemporaryCode, siteCode).Take(&temporary).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}

		updates := map[string]any{"temporary_state": result.Status, "enabled": 0}
		if result.Status == "merged" {
			if result.FormalPartyCode == nil {
				continue
			}
			var formal Party
			err := tx.Joins("JOIN wb_master_party_map m ON m.local_id = wb_party.id").
				Where("m.cloud_code = ? AND wb_party.enabled = 1", *result.FormalPartyCode).Take(&formal).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if formal.Nature != temporary.Nature || formal.IsSupplier < temporary.IsSupplier ||
				formal.IsCustomer < temporary.IsCustomer || formal.IsCarrier < temporary.IsCarrier {
				continue
			}
			updates["merged_party_code"] = formal.Code
		}
		// Terminal results apply by code. A locally queued correction can have
		// a newer version than the cloud's final decision, so use the current
		// local nature and roles above before making the merge effective.
		if err := tx.Model(&temporary).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}
