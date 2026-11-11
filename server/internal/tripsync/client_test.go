package tripsync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchCandidates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/weighbridge/sync/trips" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("siteCode") != "F001" || r.URL.Query().Get("plate") != "晋A12345" {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"code":200,"message":"ok","data":[{"tripNo":"LZ-1","version":2,"plate":"晋A12345","materialCode":"C-M-0010","materialName":"高粱","partyCode":"C-P-0001","partyName":"供应商","linkable":true,"blockers":[],"deductions":[{"categoryCode":"WATER","categoryName":"水分","mode":"percent","rateBp":100}],"inspection":{"id":9,"version":1,"status":"confirmed","conclusion":"accept","moisture":"13.5","impurity":"1.2"}}]}`)
	}))
	defer server.Close()
	values, err := FetchCandidates(context.Background(), server.URL, "F001", "晋A12345")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].TripNo != "LZ-1" || values[0].Inspection == nil || values[0].Deductions[0].RateBP == nil || *values[0].Deductions[0].RateBP != 100 {
		t.Fatalf("values: %+v", values)
	}
}

func TestFetchTripAndLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/weighbridge/sync/trips/LZ-1":
			_, _ = io.WriteString(w, `{"code":200,"message":"ok","data":{"tripNo":"LZ-1","version":2,"plate":"晋A12345","linkable":true,"blockers":[],"deductions":[]}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/weighbridge/sync/trips/LZ-1/link":
			body, _ := io.ReadAll(r.Body)
			var input map[string]string
			if err := json.Unmarshal(body, &input); err != nil || input["siteCode"] != "F001" || input["ticketNo"] != "T-1" {
				t.Errorf("unexpected link body %s", body)
			}
			_, _ = io.WriteString(w, `{"code":200,"message":"ok","data":{"tripNo":"LZ-1","ticketNo":"T-1"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	trip, err := FetchTrip(context.Background(), server.URL, "F001", "LZ-1")
	if err != nil || trip.TripNo != "LZ-1" || !trip.Linkable || trip.Deductions == nil {
		t.Fatalf("trip: %+v, %v", trip, err)
	}
	if err := LinkTicket(context.Background(), server.URL, "F001", "LZ-1", "T-1"); err != nil {
		t.Fatal(err)
	}
}

func TestCloudError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":401,"message":"厂区编码不存在或已停用"}`)
	}))
	defer server.Close()
	_, err := FetchCandidates(context.Background(), server.URL, "F001", "晋A12345")
	if err == nil {
		t.Fatal("expected error")
	}
	status, ok := StatusOf(err)
	if !ok || status != http.StatusUnauthorized || err.Error() != "厂区编码不存在或已停用" {
		t.Fatalf("unexpected error: %v status=%d ok=%v", err, status, ok)
	}
}
