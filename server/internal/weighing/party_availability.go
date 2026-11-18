package weighing

import "gorm.io/gorm"

// Both the picker and transaction-time validation use the same source and
// lifecycle rules. The caller applies the role and enabled filters it needs.
func selectableParties(db *gorm.DB) *gorm.DB {
	return db.Where(`(
		(temporary_state = '' AND (
			NOT (SELECT bound FROM wb_master_sync_state WHERE id = 1)
			OR id IN (SELECT local_id FROM wb_master_party_map)
		))
		OR (
			temporary_state IN ('pending', 'needs_info')
			AND temporary_site_code = (SELECT site_code FROM wb_local_identity WHERE id = 1)
			AND (SELECT bound FROM wb_master_sync_state WHERE id = 1)
		)
	)`)
}

func requireSelectableParty(tx *gorm.DB, partyID int64) error {
	var count int64
	err := selectableParties(tx.Model(&Party{})).Where("id = ? AND enabled = 1", partyID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return RejectionError("往来主体已不可用于该业务，请重新选择")
	}
	return nil
}

func requireTripSupplier(tx *gorm.DB, partyID int64) error {
	var count int64
	if err := tx.Table("wb_master_party_map").Where("local_id = ?", partyID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return RejectionError("流转单须使用总部正式供货方")
	}
	return nil
}

func validateChangedTripSupplier(tx *gorm.DB, task Task, business BusinessInfo) error {
	if task.PartyID == business.PartyID {
		return nil
	}
	binding, err := tripBindingByTask(tx, task.ID)
	if err != nil || binding == nil {
		return err
	}
	if binding.PartyCode != business.PartyCode {
		return RejectionError("供货方与流转单不一致")
	}
	return requireTripSupplier(tx, business.PartyID)
}
