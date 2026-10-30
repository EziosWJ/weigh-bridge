//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// Reuse the foundation's actual confirmed, reweighed and reopened task. Only
// fixture timestamps are changed; all PostgreSQL constraints remain enabled.
func checkBusinessOverview(t *testing.T, database *platformdatabase.Database, router http.Handler, admin, operator string) {
	t.Helper()
	db := database.GORM
	repo := weighing.NewRepository(db)
	current := weighingData[weighing.Overview](t, router, "GET", "/api/weighing/overview", "", admin)
	if current.ValidTickets != 1 || current.Inbound.Net != 18500 || current.Outbound.TaskCount != 0 || len(current.RecentTickets) != 1 {
		t.Fatalf("overview counted void/reopened versions: %+v", current)
	}
	var ticket weighing.Ticket
	if err := db.Where("status='valid'").Take(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	var task weighing.Task
	if err := db.First(&task, ticket.TaskID).Error; err != nil {
		t.Fatal(err)
	}
	var first weighing.WeighRecord
	if err := db.First(&first, *task.CurrentFirstRecordID).Error; err != nil {
		t.Fatal(err)
	}
	var sample weighing.Sample
	if err := db.First(&sample, first.SampleID).Error; err != nil {
		t.Fatal(err)
	}

	start := time.Date(2030, 3, 1, 16, 0, 0, 0, time.UTC) // March 2, Beijing midnight.
	for _, test := range []struct {
		name, direction string
		at              time.Time
		count           int64
	}{
		{"previous-day", "inbound", start.Add(-time.Nanosecond), 0},
		{"midnight", "inbound", start, 1},
		{"outbound", "outbound", start.Add(time.Hour), 1},
		{"next-midnight", "outbound", start.Add(24 * time.Hour), 0},
	} {
		// PostgreSQL stores microseconds; use a full microsecond at the boundary.
		at := test.at.Truncate(time.Microsecond)
		business := ticket.Business
		business.Direction = test.direction
		if err := db.Model(&weighing.Ticket{}).Where("id=?", ticket.ID).Updates(map[string]any{"issued_at": at, "business_snapshot": encodeLocalSite(t, business)}).Error; err != nil {
			t.Fatal(err)
		}
		got, err := repo.Overview(context.Background(), start.Add(12*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if got.Date != "2030-03-02" || got.ValidTickets != test.count {
			t.Fatalf("%s: %+v", test.name, got)
		}
		if test.count == 1 {
			direction := got.Inbound
			if test.direction == "outbound" {
				direction = got.Outbound
			}
			if direction.TaskCount != 1 || direction.Net != 18500 {
				t.Fatalf("direction/net %s: %+v", test.name, got)
			}
		}
	}
	if err := db.Model(&weighing.Ticket{}).Where("id=?", ticket.ID).Updates(map[string]any{"issued_at": ticket.IssuedAt, "business_snapshot": encodeLocalSite(t, ticket.Business)}).Error; err != nil {
		t.Fatal(err)
	}

	baseline, err := repo.Overview(context.Background(), start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for i := 0; i < 7; i++ {
		cloned := task
		cloned.ID = 0
		cloned.TaskNo = fmt.Sprintf("OVERVIEW-%d", i)
		cloned.Plate = fmt.Sprintf("川AOV%03d", i)
		cloned.Business.Plate = cloned.Plate
		cloned.Status = "awaiting_second"
		cloned.CurrentFirstRecordID, cloned.CurrentSecondRecordID = nil, nil
		cloned.CompletedAt = nil
		cloned.CreateTime = start.Add(time.Duration(i) * time.Minute) // Deliberately differs from first-confirmation order.
		if err := db.Create(&cloned).Error; err != nil {
			t.Fatal(err)
		}
		copySample := sample
		copySample.ID, copySample.TaskID, copySample.TargetRecordID = 0, nil, nil
		if err := db.Create(&copySample).Error; err != nil {
			t.Fatal(err)
		}
		copyRecord := first
		copyRecord.ID, copyRecord.TaskID, copyRecord.SampleID = 0, cloned.ID, copySample.ID
		copyRecord.ReplacesRecordID, copyRecord.RetainedRecordID = nil, nil
		copyRecord.ConfirmedAt = start.Add(time.Duration(3-i) * time.Hour)
		if err := db.Create(&copyRecord).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&cloned).Update("current_first_record_id", copyRecord.ID).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, cloned.ID)
	}
	got, err := repo.Overview(context.Background(), start.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got.AwaitingCount != baseline.AwaitingCount+7 || got.OvernightCount != baseline.OvernightCount+3 || len(got.AwaitingTasks) != 5 {
		t.Fatalf("pending/overnight/top-five: %+v", got)
	}
	for i := 1; i < len(got.AwaitingTasks); i++ {
		if got.AwaitingTasks[i].FirstConfirmedAt.Before(got.AwaitingTasks[i-1].FirstConfirmedAt) {
			t.Fatal("pending order used task creation instead of first confirmation")
		}
	}
	if err := db.Model(&weighing.Task{}).Where("id IN ?", ids[:2]).Update("status", "terminated").Error; err != nil {
		t.Fatal(err)
	}
	got, err = repo.Overview(context.Background(), start.Add(12*time.Hour))
	if err != nil || got.AwaitingCount != baseline.AwaitingCount+5 {
		t.Fatalf("terminated tasks remained pending: %+v %v", got, err)
	}

	// A task's historical codes are snapshots. Changing them must not hide its
	// ticket from the authorized local overview after an identity correction.
	beforeSnapshotChange := weighingData[weighing.Overview](t, router, "GET", "/api/weighing/overview", "", admin)
	if err := db.Model(&weighing.Task{}).Where("id=?", task.ID).Updates(map[string]any{"site_code": "OLD", "scale_code": "OLD-WB"}).Error; err != nil {
		t.Fatal(err)
	}
	local := weighingData[weighing.Overview](t, router, "GET", "/api/weighing/overview", "", admin)
	if local.ValidTickets != beforeSnapshotChange.ValidTickets || len(local.RecentTickets) != len(beforeSnapshotChange.RecentTickets) {
		t.Fatalf("historical-code ticket disappeared from local overview: before=%+v after=%+v", beforeSnapshotChange, local)
	}
	if err := db.Model(&weighing.Task{}).Where("id=?", task.ID).Updates(map[string]any{"site_code": task.SiteCode, "scale_code": task.ScaleCode}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM sys_role_menu WHERE role_id IN (SELECT id FROM sys_role WHERE role_code='WEIGHING_OPERATOR') AND menu_id IN (SELECT id FROM sys_menu WHERE permission_code='weighing:read')").Error; err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(router, "GET", "/api/weighing/overview", "", operator), 403, 403, "无权执行此操作")
}
