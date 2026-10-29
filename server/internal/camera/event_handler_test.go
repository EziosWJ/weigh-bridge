package camera

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/gin-gonic/gin"
)

type eventFake struct {
	fakeStore
	calls      int
	serial     string
	diagnostic json.RawMessage
	fail       error
}

func (f *eventFake) EventBinding(context.Context, int64) (EventBinding, error) {
	return EventBinding{}, nil
}
func (f *eventFake) SaveEventBinding(context.Context, EventBinding, audit.Event) error { return nil }
func (f *eventFake) EventDiagnostics(context.Context, int64) ([]EventDiagnostic, error) {
	return nil, nil
}
func (f *eventFake) ReceiveEvent(_ context.Context, serial, operator, id, deviceTime, ip string, now time.Time, hash string, diagnostic json.RawMessage) error {
	f.calls++
	f.serial = serial
	f.diagnostic = diagnostic
	return f.fail
}
func TestEventHTTPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &eventFake{}
	router := gin.New()
	RegisterEventRoute(router, NewService(store, "", nil, nil, nil))
	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/event/pull", strings.NewReader(body)))
		return w
	}
	heartbeat := `{"operator":"heartbeat","deviceInfo":{"serialNumber":"serial-a"},"info":{"eventId":42,"time":"device-time"}}`
	response := post(heartbeat)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"eventId":42`) {
		t.Fatalf("heartbeat: %d %s", response.Code, response.Body.String())
	}
	if store.serial != "serial-a" || store.calls != 1 {
		t.Fatal("wrong identity")
	}
	event := `{"operator":"unknown","deviceInfo":{"serialNumber":"serial-a"},"info":{"eventId":42,"time":"wrong clock"},"image":"BASE64SECRET","password":"secret"}`
	response = post(event)
	if response.Code != 503 || strings.Contains(string(store.diagnostic), "BASE64SECRET") || strings.Contains(string(store.diagnostic), "secret") || !strings.Contains(string(store.diagnostic), "image") {
		t.Fatalf("diagnostic: %d %s", response.Code, store.diagnostic)
	}
	for _, bad := range []string{`{`, `{"operator":"heartbeat","deviceInfo":{"serialNumber":"a"},"info":{"eventId":null,"time":"x"}}`, strings.Repeat("a", eventBodyLimit+1)} {
		before := store.calls
		response = post(bad)
		if response.Code != 400 && response.Code != 413 {
			t.Fatalf("invalid code %d", response.Code)
		}
		if store.calls != before {
			t.Fatal("invalid request persisted")
		}
	}
	store.fail = ErrNotFound
	if post(heartbeat).Code != 403 {
		t.Fatal("unknown device accepted")
	}
	store.fail = errors.New("database down")
	if post(heartbeat).Code != 503 {
		t.Fatal("database failure acknowledged")
	}
}

func TestEventSourceAllowlist(t *testing.T) {
	for _, tc := range []struct {
		allowed, remote string
		want            bool
	}{
		{"192.168.0.231,127.0.0.1", "192.168.0.231:5000", true},
		{"192.168.0.231,127.0.0.1", "192.168.0.232:5000", false},
		{"192.168.0.231", "invalid", false},
		{"invalid", "127.0.0.1:123", false},
		{"", "127.0.0.1:123", true},
	} {
		if got := eventSourceAllowed(tc.allowed, tc.remote); got != tc.want {
			t.Fatalf("%q %q: %v", tc.allowed, tc.remote, got)
		}
	}
}

func TestEventHeartbeatStrategySwitch(t *testing.T) {
	t.Setenv("WEIGHBRIDGE_CAMERA_HEARTBEAT_STRATEGY", "1")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterEventRoute(router, NewService(&eventFake{}, "", nil, nil, nil))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/event/pull", strings.NewReader(`{"operator":"heartbeat","deviceInfo":{"serialNumber":"a"},"info":{"eventId":1,"time":"x"}}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"heartBeatInterval":30`) || !strings.Contains(response.Body.String(), `"isCarLicenseSnapshotEnable":true`) {
		t.Fatalf("strategy ACK: %d %s", response.Code, response.Body.String())
	}
}
