package camsim

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

func fixedClock(value time.Time) func() time.Time {
	return func() time.Time { return value }
}

func TestRecognizeAndReportCurrent(t *testing.T) {
	fixed := time.Date(2026, 10, 30, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))
	server, requests := fakeServer(t, snapshotACK)
	sim, err := New(Config{ServerURL: server.URL, SerialNumber: "camera-sim-1", Now: fixedClock(fixed)})
	if err != nil {
		t.Fatal(err)
	}
	initial := sim.State()
	if !initial.Simulator || initial.SerialNumber != "camera-sim-1" || initial.ServerURL != server.URL || initial.EventURL != server.URL+"/api/v1/event/pull" {
		t.Fatalf("unexpected initial state: %+v", initial)
	}
	if !weighing.IsBusinessPlate(initial.Plate) {
		t.Fatalf("initial plate is invalid: %q", initial.Plate)
	}

	state := sim.Recognize(context.Background())
	if state.LastReport == nil || !state.LastReport.Success || state.LastReport.ACKOperator != "CarLicenseSnapshot-Ack" {
		t.Fatalf("recognize did not succeed: %+v", state.LastReport)
	}
	if !weighing.IsBusinessPlate(state.Plate) || state.RecognizedAt == nil || state.EventID == "" {
		t.Fatalf("unexpected recognition: %+v", state)
	}
	id, err := strconv.ParseInt(state.EventID, 10, 64)
	if err != nil || id <= 1<<53 {
		t.Fatalf("event ID must be a large JSON number: %q (%v)", state.EventID, err)
	}
	got := requests()
	if len(got) != 1 {
		t.Fatalf("requests = %d", len(got))
	}
	first := got[0]
	if first.Path != "/api/v1/event/pull" || first.ContentType != "application/json" || first.Operator != "CarLicenseSnapshot" || first.Serial != "camera-sim-1" {
		t.Fatalf("unexpected request: %+v", first)
	}
	if first.EventID != state.EventID || first.Plate != state.Plate || first.Time != fixed.Format(time.RFC3339) {
		t.Fatalf("request does not match state: %+v", first)
	}

	next, err := sim.ReportCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if next.LastReport == nil || !next.LastReport.Success || next.LastReport.Attempts != 1 {
		t.Fatalf("re-report did not succeed: %+v", next.LastReport)
	}
	if next.EventID == state.EventID || next.Plate != state.Plate {
		t.Fatalf("re-report must keep the plate and use a new event: %+v", next)
	}
	if got := requests(); len(got) != 2 || got[1].EventID != next.EventID {
		t.Fatalf("unexpected re-report request: %+v", got)
	}
	if _, err := sim.Retry(context.Background()); err == nil {
		t.Fatal("retry accepted after a successful report")
	}
}

func TestRetryResendsSameEventBody(t *testing.T) {
	fail := true
	server, requests := fakeServer(t, func(request capturedRequest) (int, string) {
		if fail {
			return http.StatusServiceUnavailable, "database unavailable"
		}
		return snapshotACK(request)
	})
	sim, err := New(Config{ServerURL: server.URL, SerialNumber: "camera-sim-1"})
	if err != nil {
		t.Fatal(err)
	}
	failed := sim.Recognize(context.Background())
	if failed.LastReport == nil || failed.LastReport.Success || !failed.LastReport.CanRetry {
		t.Fatalf("expected retryable failure: %+v", failed.LastReport)
	}
	if failed.LastReport.HTTPStatus != 503 || !strings.Contains(failed.LastReport.Error, "HTTP 503") {
		t.Fatalf("failure detail: %+v", failed.LastReport)
	}
	fail = false
	retried, err := sim.Retry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if retried.LastReport == nil || !retried.LastReport.Success || retried.LastReport.Attempts != 2 || retried.LastReport.CanRetry {
		t.Fatalf("retry state: %+v", retried.LastReport)
	}
	if retried.LastReport.EventID != failed.LastReport.EventID || retried.Plate != failed.Plate {
		t.Fatalf("retry changed identity: %+v", retried.LastReport)
	}
	all := requests()
	if len(all) != 2 || !bytes.Equal(all[0].Body, all[1].Body) {
		t.Fatal("retry did not resend the identical request body")
	}
	if _, err := sim.Retry(context.Background()); err == nil {
		t.Fatal("second retry accepted after success")
	}
}

func TestHeartbeat(t *testing.T) {
	fixed := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	server, requests := fakeServer(t, heartbeatACK)
	sim, err := New(Config{ServerURL: server.URL, SerialNumber: "camera-sim-1", HeartbeatInterval: 5 * time.Second, Now: fixedClock(fixed)})
	if err != nil {
		t.Fatal(err)
	}
	attempt := sim.SendHeartbeat(context.Background())
	if !attempt.Success || attempt.ACKOperator != "heartbeat-ack" {
		t.Fatalf("heartbeat failed: %+v", attempt)
	}
	state := sim.State()
	if state.HeartbeatIntervalMS != 5000 || state.LastHeartbeat == nil || !state.LastHeartbeat.Success {
		t.Fatalf("unexpected state: %+v", state)
	}
	got := requests()
	if len(got) != 1 || got[0].Operator != "heartbeat" || got[0].Serial != "camera-sim-1" || got[0].Time != fixed.Format(time.RFC3339) {
		t.Fatalf("unexpected heartbeat request: %+v", got)
	}
	if _, err := strconv.ParseInt(got[0].EventID, 10, 64); err != nil {
		t.Fatalf("heartbeat event ID: %q", got[0].EventID)
	}
}

func TestHeartbeatFailureReasons(t *testing.T) {
	server, _ := fakeServer(t, func(capturedRequest) (int, string) {
		return http.StatusForbidden, ""
	})
	sim, err := New(Config{ServerURL: server.URL, SerialNumber: "camera-sim-1"})
	if err != nil {
		t.Fatal(err)
	}
	attempt := sim.SendHeartbeat(context.Background())
	if attempt.Success || attempt.HTTPStatus != 403 || !strings.Contains(attempt.Error, "绑定") {
		t.Fatalf("403 heartbeat: %+v", attempt)
	}

	server, _ = fakeServer(t, func(capturedRequest) (int, string) {
		return http.StatusOK, `{"operator":"heartbeat-ack","info":{"eventId":1},"result":{"errorNo":7,"description":"device disabled"}}`
	})
	sim, err = New(Config{ServerURL: server.URL, SerialNumber: "camera-sim-1"})
	if err != nil {
		t.Fatal(err)
	}
	attempt = sim.SendHeartbeat(context.Background())
	if attempt.Success || !strings.Contains(attempt.Error, "errorNo=7") || !strings.Contains(attempt.Error, "device disabled") {
		t.Fatalf("vendor error heartbeat: %+v", attempt)
	}
}

func TestReportACKValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reply   string
		wantErr string
	}{
		{"wrong operator", `{"operator":"other","info":{"eventID":%s},"result":{"errorNo":0}}`, "不是 CarLicenseSnapshot-Ack"},
		{"vendor error", `{"operator":"CarLicenseSnapshot-Ack","info":{"eventID":%s},"result":{"errorNo":3,"description":"busy"}}`, "errorNo=3"},
		{"missing error number", `{"operator":"CarLicenseSnapshot-Ack","info":{"eventID":%s},"result":{}}`, "errorNo"},
		{"wrong event", `{"operator":"CarLicenseSnapshot-Ack","info":{"eventID":123},"result":{"errorNo":0}}`, "不一致"},
		{"missing event", `{"operator":"CarLicenseSnapshot-Ack","info":{},"result":{"errorNo":0}}`, "未回填"},
		{"empty response", ``, "没有响应内容"},
		{"not json", `hello`, "不是有效的 ACK JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eventID := "999999999999999999"
			body := tc.reply
			if strings.Contains(body, "%s") {
				body = strings.Replace(body, "%s", eventID, 1)
			}
			server, _ := fakeServer(t, func(capturedRequest) (int, string) {
				return http.StatusOK, body
			})
			sim, err := New(Config{ServerURL: server.URL, SerialNumber: "camera-sim-1"})
			if err != nil {
				t.Fatal(err)
			}
			state := sim.Recognize(context.Background())
			if state.LastReport == nil || state.LastReport.Success || !strings.Contains(state.LastReport.Error, tc.wantErr) {
				t.Fatalf("error = %+v, want %q", state.LastReport, tc.wantErr)
			}
		})
	}
}

func TestReportHTTPFailureReasons(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "HTTP 400"},
		{http.StatusForbidden, "绑定"},
		{http.StatusNotFound, "HTTP 404"},
		{http.StatusRequestEntityTooLarge, "HTTP 413"},
		{http.StatusServiceUnavailable, "HTTP 503"},
		{http.StatusInternalServerError, "HTTP 500"},
	} {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			server, _ := fakeServer(t, func(capturedRequest) (int, string) {
				return tc.status, ""
			})
			sim, err := New(Config{ServerURL: server.URL, SerialNumber: "camera-sim-1"})
			if err != nil {
				t.Fatal(err)
			}
			state := sim.Recognize(context.Background())
			if state.LastReport == nil || state.LastReport.Success || state.LastReport.HTTPStatus != tc.status || !strings.Contains(state.LastReport.Error, tc.want) {
				t.Fatalf("error = %+v, want %q", state.LastReport, tc.want)
			}
		})
	}
}

func TestReportConnectionFailure(t *testing.T) {
	server, _ := fakeServer(t, snapshotACK)
	url := server.URL
	server.Close()
	sim, err := New(Config{ServerURL: url, SerialNumber: "camera-sim-1"})
	if err != nil {
		t.Fatal(err)
	}
	state := sim.Recognize(context.Background())
	if state.LastReport == nil || state.LastReport.Success || !strings.Contains(state.LastReport.Error, "无法连接 Server") {
		t.Fatalf("connection error: %+v", state.LastReport)
	}
	if !state.LastReport.CanRetry {
		t.Fatal("connection failure must be retryable")
	}
}

func TestNewValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config Config
	}{
		{"missing scheme", Config{ServerURL: "127.0.0.1:8900", SerialNumber: "camera-1"}},
		{"unsupported scheme", Config{ServerURL: "ftp://127.0.0.1:8900", SerialNumber: "camera-1"}},
		{"path", Config{ServerURL: "http://127.0.0.1:8900/api", SerialNumber: "camera-1"}},
		{"query", Config{ServerURL: "http://127.0.0.1:8900?x=1", SerialNumber: "camera-1"}},
		{"empty serial", Config{ServerURL: "http://127.0.0.1:8900", SerialNumber: ""}},
		{"padded serial", Config{ServerURL: "http://127.0.0.1:8900", SerialNumber: " camera-1"}},
		{"newline serial", Config{ServerURL: "http://127.0.0.1:8900", SerialNumber: "camera\n1"}},
	} {
		if _, err := New(tc.config); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
	sim, err := New(Config{ServerURL: "http://127.0.0.1:8900/", SerialNumber: "camera-1"})
	if err != nil {
		t.Fatal(err)
	}
	if simulated := sim.State(); simulated.HeartbeatIntervalMS != DefaultHeartbeatInterval.Milliseconds() {
		t.Fatalf("default interval: %+v", simulated)
	}
}

func TestContextCancellationDoesNotReportSuccess(t *testing.T) {
	server, _ := fakeServer(t, snapshotACK)
	sim, err := New(Config{ServerURL: server.URL, SerialNumber: "camera-sim-1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state := sim.Recognize(ctx)
	if state.LastReport == nil || state.LastReport.Success {
		t.Fatalf("cancelled report succeeded: %+v", state.LastReport)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("unexpected context state")
	}
}
