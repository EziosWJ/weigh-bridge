//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/EziosWJ/weighbridge/server/internal/weighing"
	"gorm.io/gorm"
)

// Exercises the actual business transactions constructed by the workflow test.
func checkTicketSyncOutbox(t *testing.T, db *gorm.DB, oldID, newID int64) {
	t.Helper()
	var rows []struct {
		ID         int64
		TicketID   int64
		ChangeType string
		Payload    string
		Status     string
	}
	read := func() {
		t.Helper()
		rows = nil
		if err := db.Table("wb_ticket_sync_change").Order("id").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
	}
	read()
	if len(rows) != 3 || rows[0].ChangeType != "issue" || rows[1].ChangeType != "void" || rows[2].ChangeType != "reopen" || rows[0].TicketID != oldID || rows[2].TicketID != newID {
		t.Fatalf("outbox sequence: %+v", rows)
	}
	var issue, reopen map[string]any
	if json.Unmarshal([]byte(rows[0].Payload), &issue) != nil || json.Unmarshal([]byte(rows[2].Payload), &reopen) != nil {
		t.Fatal("invalid outbox payload")
	}
	ticket := issue["ticket"].(map[string]any)
	if issue["siteCode"] != "SITE01" || ticket["partyName"] != "首磅供收方" || ticket["originalGross"] != "30" || ticket["finalGross"] != "30" || ticket["finalTare"] != "10" || ticket["finalNet"] != "18.5" || ticket["netAdjustment"] != "1.5" || ticket["netAdjustmentCategory"] != "WATER" || ticket["netAdjustmentCategoryName"] != "水分大" {
		t.Fatalf("snapshot mapping: %s", rows[0].Payload)
	}
	if reopen["siteCode"] != "F001" || reopen["previousSiteCode"] != issue["siteCode"] || reopen["ticket"].(map[string]any)["previousTicketNo"] != issue["ticketNo"] {
		t.Fatalf("reopen association: %s", rows[2].Payload)
	}
	var payloads []string
	failure := "unknown"
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		payloads = append(payloads, string(body))
		if r.URL.Path != "/api/v1/weighbridge/sync/changes" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected upload request")
		}
		if failure == "unknown" {
			fmt.Fprint(w, `{"code":200,"data":null}`)
			return
		}
		if failure == "rejected" {
			w.WriteHeader(401)
			return
		}
		var c struct {
			ChangeID string `json:"changeId"`
			TicketNo string `json:"ticketNo"`
		}
		if err := json.Unmarshal(body, &c); err != nil {
			t.Error(err)
		}
		fmt.Fprintf(w, `{"code":200,"message":"success","data":{"changeId":%q,"ticketNo":%q}}`, c.ChangeID, c.TicketNo)
	}))
	defer remote.Close()
	ctx := context.Background()
	service := weighing.NewService(weighing.NewRepository(db), nil)
	if err := service.ConfigureMasterSync(ctx, true, remote.URL); err != nil {
		t.Fatal(err)
	}
	if err := service.RunTicketSync(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := service.Tickets(ctx, weighing.PageQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, ticket := range page.Records {
		if ticket.ID == oldID && ticket.SyncStatus != "pending" {
			t.Fatalf("latest void should remain pending: %+v", ticket)
		}
	}
	read()
	if len(payloads) != 1 || rows[0].Status != "failed" || rows[1].Status != "pending" || rows[2].Status != "pending" {
		t.Fatalf("failure order: %+v", rows)
	}
	failure = "rejected"
	if err := service.RunTicketSync(ctx); err != nil {
		t.Fatal(err)
	}
	var reason string
	if err := db.Table("wb_ticket_sync_change").Where("id=?", rows[0].ID).Select("reason").Scan(&reason).Error; err != nil || !strings.Contains(reason, "厂区编码") {
		t.Fatalf("rejection reason: %s %v", reason, err)
	}
	if err := db.Exec("DELETE FROM wb_master_sync_log").Error; err != nil {
		t.Fatal(err)
	}
	read()
	if rows[0].Status != "failed" || rows[1].Status != "pending" {
		t.Fatal("clearing logs changed outbox")
	}
	// An unavailable endpoint leaves the same queued payload intact.
	offline := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	offlineURL := offline.URL
	offline.Close()
	if err := service.ConfigureMasterSync(ctx, true, offlineURL); err != nil {
		t.Fatal(err)
	}
	if err := service.RunTicketSync(ctx); err != nil {
		t.Fatal(err)
	}
	read()
	if rows[0].Status != "failed" || rows[0].Payload != payloads[0] {
		t.Fatal("offline attempt changed outbox")
	}
	failure = ""
	// Recreate the service to exercise persistence across restart.
	service = weighing.NewService(weighing.NewRepository(db), nil)
	if err := service.ConfigureMasterSync(ctx, true, remote.URL); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := service.RunTicketSync(ctx); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if len(payloads) != 5 || payloads[0] != payloads[1] || payloads[1] != payloads[2] || rows[0].Status != "synced" || rows[1].Status != "synced" || rows[2].Status != "synced" {
		t.Fatalf("retry/order: %+v %v", rows, payloads)
	}
	page, err = service.Tickets(ctx, weighing.PageQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, ticket := range page.Records {
		if (ticket.ID == oldID || ticket.ID == newID) && ticket.SyncStatus != "synced" {
			t.Fatalf("confirmed status: %+v", ticket)
		}
	}
	if endpoint := os.Getenv("WB_SYNC_TEST_URL"); endpoint != "" {
		post := func(body string, want int) {
			t.Helper()
			resp, err := http.Post(endpoint+"/api/v1/weighbridge/sync/changes", "application/json", bytes.NewBufferString(body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			data, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != want {
				t.Fatalf("cloud HTTP %d, want %d: %s", resp.StatusCode, want, data)
			}
		}
		if err := db.Exec("UPDATE wb_ticket_sync_change SET status='pending', synced_at=NULL").Error; err != nil {
			t.Fatal(err)
		}
		// Deliver the first change to the real cloud but lose its confirmation.
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response, err := http.Post(endpoint+r.URL.Path, "application/json", r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(502)
				return
			}
			defer response.Body.Close()
			if response.StatusCode != 200 {
				t.Errorf("cloud did not save first change: %d", response.StatusCode)
				w.WriteHeader(502)
				return
			}
			fmt.Fprint(w, `{"code":200,"data":null}`)
		}))
		if err := service.ConfigureMasterSync(ctx, true, proxy.URL); err != nil {
			t.Fatal(err)
		}
		if err := service.RunTicketSync(ctx); err != nil {
			t.Fatal(err)
		}
		proxy.Close()
		read()
		if rows[0].Status != "failed" || rows[0].Payload != payloads[0] {
			t.Fatal("lost confirmation changed the pending payload")
		}
		if err := service.ConfigureMasterSync(ctx, true, endpoint); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			if err := service.RunTicketSync(ctx); err != nil {
				t.Fatal(err)
			}
		}
		read()
		for _, row := range rows {
			if row.Status != "synced" {
				t.Fatalf("real cloud upload: %+v", rows)
			}
		}
		post(rows[0].Payload, 200)
		issue["ticket"].(map[string]any)["plate"] = "晋A99999"
		conflicting, err := json.Marshal(issue)
		if err != nil {
			t.Fatal(err)
		}
		post(string(conflicting), 409)
		altered := strings.Replace(rows[0].Payload, `"plate":`, `"unknown":true,"plate":`, 1)
		post(altered, 400)
	}
	if strings.Contains(payloads[0], "scaleCode") {
		t.Fatal("scale code sent")
	}
}
