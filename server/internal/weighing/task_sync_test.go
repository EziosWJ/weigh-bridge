package weighing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type taskRoundTrip func(*http.Request) (*http.Response, error)

func (f taskRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTaskSyncReceiptValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"empty heartbeat", `{"code":200,"data":{"schemaVersion":1,"siteCode":"F001","receivedAt":"2026-11-18T00:00:00Z","items":[]}}`, true},
		{"wrong site", `{"code":200,"data":{"schemaVersion":1,"siteCode":"F002","receivedAt":"2026-11-18T00:00:00Z","items":[]}}`, false},
		{"missing items", `{"code":200,"data":{"schemaVersion":1,"siteCode":"F001","receivedAt":"2026-11-18T00:00:00Z"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{masterSyncAddress: "https://example.invalid", syncV4Client: &syncV4Client{client: &http.Client{Transport: taskRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/api/v1/weighbridge/sync/tasks" {
					t.Errorf("path %s", r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}}}
			err := s.sendTaskSync(context.Background(), "F001", []taskSyncItem{})
			if (err == nil) != tc.valid {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestTaskSyncRequiresMatchingAcknowledgement(t *testing.T) {
	item := taskSyncItem{TaskNo: "RW-1", Version: 3, Status: "awaiting_second"}
	for _, tc := range []struct {
		receipt string
		ok      bool
	}{
		{`{"code":200,"data":{"schemaVersion":1,"siteCode":"F001","receivedAt":"2026-11-18T00:00:00Z","items":[{"taskNo":"RW-1","version":3}]}}`, true},
		{`{"code":200,"data":{"schemaVersion":1,"siteCode":"F001","receivedAt":"2026-11-18T00:00:00Z","items":[{"taskNo":"RW-1","version":5}]}}`, true},
		{`{"code":200,"data":{"schemaVersion":1,"siteCode":"F001","receivedAt":"2026-11-18T00:00:00Z","items":[{"taskNo":"RW-1","version":2}]}}`, false},
		{`{"code":200,"data":{"schemaVersion":1,"siteCode":"F001","receivedAt":"2026-11-18T00:00:00Z","items":[{"taskNo":"RW-2","version":3}]}}`, false},
	} {
		s := &Service{masterSyncAddress: "https://example.invalid", syncV4Client: &syncV4Client{client: &http.Client{Transport: taskRoundTrip(func(r *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), `"taskNo":"RW-1"`) || !strings.Contains(string(body), `"version":3`) {
				t.Fatalf("unexpected request %s", body)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.receipt)), Header: make(http.Header)}, nil
		})}}}
		err := s.sendTaskSync(context.Background(), "F001", []taskSyncItem{item})
		if (err == nil) != tc.ok {
			t.Fatalf("receipt %s: %v", tc.receipt, err)
		}
	}
}
