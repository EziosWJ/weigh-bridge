package weighing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCloudDigestV2Vectors(t *testing.T) {
	data, err := os.ReadFile("testdata/digest_v2_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []struct {
			Name     string       `json:"name"`
			Change   syncChangeV4 `json:"change"`
			Expected string       `json:"expectedDigest"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	for _, vector := range file.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			got, err := computeChangeDigestV4(vector.Change)
			if err != nil || got != vector.Expected {
				t.Fatalf("digest=%q, err=%v; want %q", got, err, vector.Expected)
			}
		})
	}
}

func TestSingleTicketRequiresMatchingLatestReceipt(t *testing.T) {
	digest := strings.Repeat("a", 64)
	base := fmt.Sprintf(`{"siteCode":"F001","ticketNo":"T1","found":true,"status":"valid","ticket":{"ticketNo":"T1"},"latestDigest":%q,"latestDigestVersion":2,"latestChangeType":"issue","latestReceivedAt":"2026-01-01T00:00:00Z","changes":%%s}`, digest)
	for name, history := range map[string]string{
		"missing":        `[]`,
		"different type": fmt.Sprintf(`[{"digest":%q,"ticketNo":"T1","changeType":"void","digestVersion":2,"schemaVersion":4,"receivedAt":"2026-01-01T00:00:00Z"}]`, digest),
		"different time": fmt.Sprintf(`[{"digest":%q,"ticketNo":"T1","changeType":"issue","digestVersion":2,"schemaVersion":4,"receivedAt":"2026-01-01T00:00:01Z"}]`, digest),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeCloudTicket(json.RawMessage(fmt.Sprintf(base, history)), syncTicketRef{SiteCode: "F001", TicketNo: "T1"}, true)
			if err == nil {
				t.Fatal("an inconsistent latest receipt must remain unknown")
			}
		})
	}
}

func TestSyncFieldDifferencesNormalizesEquivalentValues(t *testing.T) {
	data, err := os.ReadFile("testdata/digest_v2_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []struct {
			Change syncChangeV4 `json:"change"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	change := file.Vectors[0].Change
	payload, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	ticket := *change.Ticket
	// Identical instants and equivalent decimal weights must not be conflicts.
	ticket.FirstConfirmedAt = ticket.FirstConfirmedAt.In(time.FixedZone("CST", 8*3600))
	ticket.OriginalGross += ".000"
	differences, err := syncFieldDifferences(ticketSyncChangeV4{Payload: string(payload)}, syncCloudTicket{Ticket: &ticket, Status: "valid"})
	if err != nil || len(differences) != 0 {
		t.Fatalf("differences=%v, err=%v", differences, err)
	}
}

func TestReconcileRejectsMalformedOrMismatchedResponse(t *testing.T) {
	for _, body := range []string{
		`{"code":200,"data":null}`,
		`{"code":200,"data":{}}`,
		`{"code":200,"data":{"siteCode":"F001","ticketNo":"OTHER","found":false}}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			_, syncErr := newSyncV4Client().fetchTicket(context.Background(), server.URL, "F001", "T1")
			if syncErr == nil {
				t.Fatal("invalid cloud response must remain unknown")
			}
		})
	}
}

func TestBatchRejectsMismatchedTicketIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"code":200,"data":{"items":[{"siteCode":"F001","ticketNo":"OTHER","found":false}]}}`)
	}))
	defer server.Close()
	_, syncErr := newSyncV4Client().fetchBatch(context.Background(), server.URL, []syncTicketRef{{SiteCode: "F001", TicketNo: "T1"}})
	if syncErr == nil {
		t.Fatal("mismatched batch ticket must remain unknown")
	}
}

func TestMissingCloudTicketDoesNotClearConflict(t *testing.T) {
	changes := []ticketSyncChangeV4{v4Change(1, "issue", "a", SyncChangeConflict)}
	decision := decideSyncReconcile(changes, syncCloudTicket{Found: false}, "automatic")
	if decision.Resend || decision.Status != SyncReconcileConflict {
		t.Fatalf("unresolved conflict must stay blocked: %+v", decision)
	}
}

func TestReconcileDetectsCloudFieldDifferenceWithMatchingDigest(t *testing.T) {
	data, err := os.ReadFile("testdata/digest_v2_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []struct {
			Change syncChangeV4 `json:"change"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	change := file.Vectors[0].Change
	payload, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	local := ticketSyncChangeV4{Digest: change.Digest, DigestVersion: change.DigestVersion, SchemaVersion: change.SchemaVersion, TicketNo: change.TicketNo, ChangeType: change.Type, Payload: string(payload), Status: SyncChangeSynced}
	cloud := cloudResult(change.Digest, change.DigestVersion, "valid", "issue")
	ticket := *change.Ticket
	ticket.Plate = "晋A99999"
	cloud.Ticket = &ticket
	decision := decideSyncReconcile([]ticketSyncChangeV4{local}, cloud, "manual")
	differences, ok := decision.Detail["fieldDifferences"].([]map[string]any)
	if decision.Status != SyncReconcileConflict || !ok || len(differences) != 1 || differences[0]["field"] != "plate" {
		t.Fatalf("mismatched cloud plate must be visible as conflict: %+v", decision)
	}
}
