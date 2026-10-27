//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/EziosWJ/weighbridge/server/internal/app"
	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

// ADR 0033: ticket numbers are "siteCode-YYYYMMDD-NNNN" and the daily serial
// can be raised after a backup restore. Lower submissions must never re-open
// already issued numbers, and the raise must leave an audit record.
func TestPostgresTicketSequenceWatermark(t *testing.T) {
	postgres := startBusinessPostgres(t)
	db := openTemporaryDatabase(t, postgres.dsn)
	t.Cleanup(func() { _ = db.Close() })
	deps := testDependencies(t, db, t.TempDir())
	watermarkDir := t.TempDir()
	repo := weighing.NewRepository(db.GORM, weighing.RuntimeOptions{DataDir: watermarkDir})
	hub := agentbridge.NewMultiple(nil)
	hub.SetResolver(func(ctx context.Context, scaleCode string) (agentbridge.Config, error) {
		return repo.AgentConfig(ctx, scaleCode, t.TempDir())
	})
	deps.Weighing = weighing.NewService(repo, hub)
	deps.Camera, _, _ = newCaptureService(t, db, t.TempDir())
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	admin := loginAdmin(t, router)
	ensureLocalSite(t, router, admin, 1, "F001", "一厂", "WB01", "一号地磅", "integration-agent")
	addCaptureCameras(t, db, "rtsp://camera/sequence-1")

	ctx := context.Background()
	agentConfig, err := repo.AgentConfig(ctx, "WB01", t.TempDir())
	if err != nil {
		t.Fatalf("agent binding: %v", err)
	}
	device := startLocalSiteMock(t, router, hub, "WB01", agentConfig.ID, agentConfig.Key)
	date := time.Now().UTC().Add(8 * time.Hour).Format("20060102")
	numberPattern := regexp.MustCompile(`^F001-` + date + `-\d{4,}$`)

	device.weight("30")
	first := localSiteRoundTrip(t, router, admin, admin, "F001", "WB01", "晋A10001", device)
	if !numberPattern.MatchString(first.TicketNo) {
		t.Fatalf("first ticket number = %q, want F001-%s-0001", first.TicketNo, date)
	}
	device.weight("30")
	second := localSiteRoundTrip(t, router, admin, admin, "F001", "WB01", "晋A10002", device)
	if second.TicketNo != fmt.Sprintf("F001-%s-0002", date) {
		t.Fatalf("second ticket number = %q, want the next daily serial", second.TicketNo)
	}

	raised := weighingData[weighing.DailySequence](t, router, http.MethodPut, "/api/weighing/ticket-sequences",
		fmt.Sprintf(`{"businessDate":%q,"lastIssued":50}`, date), admin)
	if raised.LastIssued != 50 || raised.BusinessDate != date {
		t.Fatalf("raised watermark = %+v", raised)
	}
	// The out-of-database floor must follow the operator-confirmed serial so a
	// later database-only restore cannot drop below it.
	raw, err := os.ReadFile(filepath.Join(watermarkDir, "ticket-watermark.json"))
	if err != nil {
		t.Fatalf("watermark file: %v", err)
	}
	var floors map[string]int64
	if err := json.Unmarshal(raw, &floors); err != nil || floors["ticket|F001|"+date] != 50 {
		t.Fatalf("watermark file floors = %v err=%v", floors, err)
	}
	device.weight("30")
	third := localSiteRoundTrip(t, router, admin, admin, "F001", "WB01", "晋A10003", device)
	if third.TicketNo != fmt.Sprintf("F001-%s-0051", date) {
		t.Fatalf("ticket after watermark = %q, want 0051", third.TicketNo)
	}

	lowered := weighingData[weighing.DailySequence](t, router, http.MethodPut, "/api/weighing/ticket-sequences",
		fmt.Sprintf(`{"businessDate":%q,"lastIssued":10}`, date), admin)
	if lowered.LastIssued != 51 {
		t.Fatalf("lower watermark was applied: %+v", lowered)
	}
	device.weight("30")
	fourth := localSiteRoundTrip(t, router, admin, admin, "F001", "WB01", "晋A10004", device)
	if fourth.TicketNo != fmt.Sprintf("F001-%s-0052", date) {
		t.Fatalf("ticket after rejected lower watermark = %q, want 0052", fourth.TicketNo)
	}

	rows := weighingData[[]weighing.DailySequence](t, router, http.MethodGet, "/api/weighing/ticket-sequences", "", admin)
	if len(rows) == 0 || rows[0].BusinessDate != date || rows[0].LastIssued != 52 {
		t.Fatalf("daily sequences = %+v", rows)
	}

	invalid := serveJSON(router, http.MethodPut, "/api/weighing/ticket-sequences", `{"businessDate":"2026-10-27","lastIssued":5}`, admin)
	assertEnvelopeCode(t, invalid, http.StatusOK, http.StatusBadRequest, "参数错误")
	for _, body := range []string{
		fmt.Sprintf(`{"businessDate":%q,"lastIssued":0}`, date),
		fmt.Sprintf(`{"businessDate":%q,"lastIssued":9223372036854775807}`, date),
	} {
		bounded := serveJSON(router, http.MethodPut, "/api/weighing/ticket-sequences", body, admin)
		assertEnvelopeCode(t, bounded, http.StatusOK, http.StatusBadRequest, "请填写该日期已发放的最大流水号（大于 0）")
	}

	// Switching the local factory label and switching back must never reuse a
	// previously issued number, including after a database-only restoration.
	weighingData[weighing.DailySequence](t, router, http.MethodPut, "/api/weighing/ticket-sequences", fmt.Sprintf(`{"businessDate":%q,"lastIssued":100}`, date), admin)
	config := weighingData[[]weighing.SiteConfiguration](t, router, http.MethodGet, "/api/weighing/site-config", "", admin)[0]
	edit := weighing.SiteConfigurationInput{Code: "F002", ScaleCode: config.ScaleCode, Name: config.Name, ScaleName: config.ScaleName, AgentID: config.AgentID, Enabled: config.Enabled, Version: config.Version}
	changed := weighingData[weighing.SiteConfiguration](t, router, http.MethodPut, "/api/weighing/site-config", encodeLocalSite(t, edit), admin)
	device.weight("30")
	other := localSiteRoundTrip(t, router, admin, admin, "F002", "WB01", "晋A10005", device)
	if other.TicketNo != fmt.Sprintf("F002-%s-0001", date) {
		t.Fatalf("new code ticket=%q", other.TicketNo)
	}
	edit.Code, edit.Version = "F001", changed.Version
	weighingData[weighing.SiteConfiguration](t, router, http.MethodPut, "/api/weighing/site-config", encodeLocalSite(t, edit), admin)
	// Emulate a restored database counter while retaining the independent data directory.
	if err := db.GORM.Exec("UPDATE wb_number_sequence SET next_value=1 WHERE site_code='F001' AND kind='ticket' AND business_date=?", date).Error; err != nil {
		t.Fatal(err)
	}
	device.weight("30")
	afterRestore := localSiteRoundTrip(t, router, admin, admin, "F001", "WB01", "晋A10006", device)
	if afterRestore.TicketNo != fmt.Sprintf("F001-%s-0101", date) {
		t.Fatalf("old code restored ticket=%q", afterRestore.TicketNo)
	}
	old := weighingData[weighing.TicketView](t, router, http.MethodGet, fmt.Sprintf("/api/weighing/tickets/%d", first.ID), "", admin)
	if old.TicketNo != first.TicketNo || old.SiteCode != "F001" {
		t.Fatalf("historical ticket changed: %+v", old)
	}

	var watermarkAudits int64
	if err := db.GORM.Table("sys_oper_log").Where("module_name='weighing.ticket-sequence' AND operation_type='ticket.sequence.watermark'").Count(&watermarkAudits).Error; err != nil || watermarkAudits < 2 {
		t.Fatalf("watermark audit rows = %d, err=%v", watermarkAudits, err)
	}
}
