package weighing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSyncUploadUsesVersionedPaths(t *testing.T) {
	digest := strings.Repeat("a", 64)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200, "message": "ok",
			"data": map[string]any{"digest": digest, "ticketNo": "T1", "changeType": "issue", "receivedAt": time.Now().UTC(), "duplicate": false},
		})
	}))
	defer server.Close()

	client := newSyncV4Client()
	if _, cloudErr := client.uploadChange(context.Background(), server.URL, "{}", digest, "T1", "issue"); cloudErr != nil {
		t.Fatalf("v4 upload: %v", cloudErr)
	}
	if _, cloudErr := client.uploadChangeV5(context.Background(), server.URL, "{}", digest, "T1", "issue"); cloudErr != nil {
		t.Fatalf("v5 upload: %v", cloudErr)
	}
	want := []string{syncV4ChangesPath, syncV5ChangesPath}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("upload paths = %v, want %v", paths, want)
	}
}

func TestSyncReadsUseV5Paths(t *testing.T) {
	digest := strings.Repeat("b", 64)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var data any
		switch r.URL.Path {
		case syncV5TicketsPath:
			data = map[string]any{"found": false, "siteCode": "F001", "ticketNo": "T1"}
		case syncV5ReconcilePath:
			data = map[string]any{"items": []any{map[string]any{"found": false, "siteCode": "F001", "ticketNo": "T1"}}}
		case syncV5ChangesPath:
			data = map[string]any{"found": false, "siteCode": "F001", "digest": digest}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": data})
	}))
	defer server.Close()

	client := newSyncV4Client()
	if _, cloudErr := client.fetchTicket(context.Background(), server.URL, "F001", "T1"); cloudErr != nil {
		t.Fatalf("fetch ticket: %v", cloudErr)
	}
	if _, cloudErr := client.fetchBatch(context.Background(), server.URL, []syncTicketRef{{SiteCode: "F001", TicketNo: "T1"}}); cloudErr != nil {
		t.Fatalf("fetch batch: %v", cloudErr)
	}
	if _, cloudErr := client.lookupChange(context.Background(), server.URL, "F001", digest); cloudErr != nil {
		t.Fatalf("lookup change: %v", cloudErr)
	}
	want := []string{syncV5TicketsPath, syncV5ReconcilePath, syncV5ChangesPath}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("read paths = %v, want %v", paths, want)
	}
}
