package weighing

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTemporaryPartyUploadContract(t *testing.T) {
	const payload = `{"schemaVersion":1,"siteCode":"F001","temporaryCode":"T-P-a1","version":3,"name":"临时供货方","nature":"enterprise","isSupplier":true,"isCustomer":false,"isCarrier":false}`
	row := temporaryPartyOutbox{SiteCode: "F001", TemporaryCode: "T-P-a1", Version: 3, Payload: payload}
	const validReceipt = `{"schemaVersion":1,"siteCode":"F001","temporaryCode":"T-P-a1","version":3,"status":"pending"}`
	tests := []struct {
		name, data   string
		status, code int
		reason       string
		retryable    bool
	}{
		{name: "accepted", status: 200, code: 200, data: validReceipt},
		{name: "newer and merged", status: 200, code: 200, data: strings.Replace(strings.Replace(validReceipt, `"version":3`, `"version":4`, 1), "pending", "merged", 1)},
		{name: "other site", status: 200, code: 200, data: strings.Replace(validReceipt, "F001", "F002", 1), reason: "unknown_response", retryable: true},
		{name: "other code", status: 200, code: 200, data: strings.Replace(validReceipt, "T-P-a1", "T-P-a2", 1), reason: "unknown_response", retryable: true},
		{name: "stale version", status: 200, code: 200, data: strings.Replace(validReceipt, `"version":3`, `"version":2`, 1), reason: "unknown_response", retryable: true},
		{name: "unsupported protocol", status: 200, code: 200, data: strings.Replace(validReceipt, `"schemaVersion":1`, `"schemaVersion":2`, 1), reason: "unknown_response", retryable: true},
		{name: "unknown state", status: 200, code: 200, data: strings.Replace(validReceipt, "pending", "other", 1), reason: "unknown_response", retryable: true},
		{name: "failed business code", status: 200, code: 500, data: validReceipt, reason: "unknown_response", retryable: true},
		{name: "failed HTTP status", status: 503, code: 200, data: validReceipt, reason: "unknown_response", retryable: true},
		{name: "invalid request", status: 400, code: 400, data: `{"kind":"invalid"}`, reason: "invalid"},
		{name: "invalid site", status: 401, code: 401, data: `{"kind":"invalid_site"}`, reason: "invalid_site", retryable: true},
		{name: "temporary failure", status: 500, code: 500, data: `{"kind":"temporary"}`, reason: "temporary", retryable: true},
		{name: "version conflict", status: 409, code: 409, data: `{"kind":"version_conflict"}`, reason: "version_conflict"},
		{name: "finalized", status: 409, code: 409, data: `{"kind":"finalized"}`, reason: "finalized"},
		{name: "unrecognized conflict", status: 409, code: 409, data: `{"kind":"unknown"}`, reason: "unknown_response", retryable: true},
	}
	client := newSyncV4Client()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if r.URL.RequestURI() != "/cloud"+temporaryPartyRegistrationPath ||
					r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" ||
					r.Header.Get("Authorization") != "" || err != nil || string(body) != payload {
					t.Errorf("registration request: %s %s %q, body %q, error %v", r.Method, r.URL, r.Header.Get("Content-Type"), body, err)
				}
				w.WriteHeader(test.status)
				fmt.Fprintf(w, `{"code":%d,"message":"云端原始消息","data":%s}`, test.code, test.data)
			}))
			defer cloud.Close()
			failure := client.uploadTemporaryParty(context.Background(), cloud.URL+"/cloud", row)
			if test.reason == "" {
				if failure != nil {
					t.Fatalf("receipt rejected: %+v", failure)
				}
				return
			}
			if failure == nil || failure.Reason != test.reason || failure.Retryable != test.retryable || strings.Contains(failure.Message, "云端原始消息") {
				t.Fatalf("unexpected failure: %+v", failure)
			}
		})
	}
}
