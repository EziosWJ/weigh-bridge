package mastersync

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFetch(t *testing.T) {
	status, body := http.StatusOK, validResponse
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/weighbridge/sync/master" || (r.Header.Get("Authorization") != "" || r.URL.Query().Get("siteCode") != "F001") {
			t.Error("wrong request")
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	if _, err := fetch(context.Background(), "https://example.com", "F001", client); err != nil {
		t.Fatal(err)
	}
	status = http.StatusBadGateway
	if _, err := fetch(context.Background(), "https://example.com", "F001", client); err == nil {
		t.Fatal("HTTP failure accepted")
	}
	status = http.StatusOK
	body = strings.Replace(validResponse, `"code":200`, `"code":500`, 1)
	if _, err := fetch(context.Background(), "https://example.com", "F001", client); err == nil {
		t.Fatal("business failure accepted")
	}
	if _, err := fetch(context.Background(), "http://example.com", "F001", client); err == nil {
		t.Fatal("business failure accepted over HTTP")
	}
	body = validResponse
	if _, err := fetch(context.Background(), "http://example.com", "F001", client); err != nil {
		t.Fatalf("HTTP should be supported: %v", err)
	}
}

func TestFetchTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/weighbridge/sync/master" || (r.Header.Get("Authorization") != "" || r.URL.Query().Get("siteCode") != "F001") {
			t.Error("unexpected request")
		}
		_, _ = io.WriteString(w, validResponse)
	}))
	defer server.Close()
	got, err := fetch(context.Background(), server.URL, "F001", server.Client())
	if err != nil || got.SiteCode != "F001" || len(got.Parties) != 1 || got.Parties[0].Contacts[0].ID != 1 {
		t.Fatalf("snapshot: %+v, %v", got, err)
	}
}
