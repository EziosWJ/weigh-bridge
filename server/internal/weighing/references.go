package weighing

import (
	"context"
	"strconv"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
)

func masterEvent(ctx context.Context, tx *gorm.DB, e audit.Event, before, after any) error {
	name, err := actorName(tx, e.Metadata.ActorID)
	if err != nil {
		return err
	}
	event := BusinessEvent{ResourceType: e.Resource, ResourceID: ptr(e.ResourceID), Action: e.Action, Before: snapshot(before), After: snapshot(after), ActorID: e.Metadata.ActorID, ActorName: name, OccurredAt: time.Now().UTC()}
	if err := tx.Create(&event).Error; err != nil {
		return err
	}
	return audit.RecordOn(ctx, tx, e)
}
func jsonIDExpression(tx *gorm.DB, column, key string) string {
	if tx.Dialector.Name() == "postgres" {
		return "(" + column + "->>'" + key + "')"
	}
	return "CAST(json_extract(" + column + ",'$." + key + "') AS TEXT)"
}
func usedByBusiness(tx *gorm.DB, kind string, id int64) (bool, error) {
	fields := []string{}
	switch kind {
	case "category":
		fields = []string{"materialCategoryId", "categoryId"}
	case "material":
		fields = []string{"materialId"}
	case "party":
		fields = []string{"partyId", "carrierId"}
	default:
		return false, ErrInvalid
	}
	for _, table := range []string{"wb_transport_task", "wb_ticket", "wb_business_event"} {
		columns := []string{"business_snapshot"}
		if table == "wb_business_event" {
			columns = []string{"before_snapshot", "after_snapshot"}
		}
		for _, column := range columns {
			for _, field := range fields {
				var count int64
				query := tx.Table(table).Where(jsonIDExpression(tx, column, field)+"=?", strconv.FormatInt(id, 10))
				if table == "wb_business_event" {
					query = query.Where("action NOT LIKE ?", "%.rejected")
				}
				if err := query.Count(&count).Error; err != nil {
					return false, err
				}
				if count > 0 {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
func preservePartyRoles(tx *gorm.DB, id int64, in PartyInput) error {
	var count int64
	if err := tx.Table("wb_node_identity").Where("role<>''").Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for _, role := range []struct{ name, condition string }{{"supplier", "party_id=? AND direction='inbound'"}, {"customer", "party_id=? AND direction='outbound'"}, {"carrier", "carrier_id=?"}} {
		present := false
		for _, value := range in.Roles {
			if value == role.name {
				present = true
			}
		}
		if present {
			continue
		}
		var count int64
		if err := tx.Model(&Task{}).Where("status='awaiting_second'").Where(role.condition, id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrReferenced
		}
	}
	return nil
}
