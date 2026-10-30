package camsim

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type capturedRequest struct {
	Body        []byte
	Operator    string
	Serial      string
	EventID     string
	Time        string
	Plate       string
	ContentType string
	Path        string
}

func capture(body []byte) capturedRequest {
	var envelope struct {
		Operator   string `json:"operator"`
		DeviceInfo struct {
			SerialNumber string `json:"serialNumber"`
		} `json:"deviceInfo"`
		Info struct {
			EventID json.RawMessage `json:"eventId"`
			Time    string          `json:"time"`
			License struct {
				Number string `json:"Number"`
			} `json:"carLicenseAttriInfo"`
		} `json:"info"`
	}
	_ = json.Unmarshal(body, &envelope)
	return capturedRequest{
		Body: body, Operator: envelope.Operator, Serial: envelope.DeviceInfo.SerialNumber,
		EventID: string(envelope.Info.EventID), Time: envelope.Info.Time, Plate: envelope.Info.License.Number,
	}
}

// fakeServer records raw event requests and delegates the reply to respond.
func fakeServer(t *testing.T, respond func(capturedRequest) (int, string)) (*httptest.Server, func() []capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	var requests []capturedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		request := capture(body)
		request.ContentType = r.Header.Get("Content-Type")
		request.Path = r.URL.Path
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()
		status, text := respond(request)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, text)
	}))
	t.Cleanup(server.Close)
	snapshot := func() []capturedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedRequest(nil), requests...)
	}
	return server, snapshot
}

func snapshotACK(request capturedRequest) (int, string) {
	return http.StatusOK, `{"operator":"CarLicenseSnapshot-Ack","info":{"eventID":` + request.EventID + `},"result":{"errorNo":0,"description":"ok"}}`
}

func heartbeatACK(request capturedRequest) (int, string) {
	return http.StatusOK, `{"operator":"heartbeat-ack","info":{"eventId":` + request.EventID + `,"time":"2026-10-30T09:00:00Z","eventSendMode":"realTime"},"result":{"errorNo":0,"description":"ok"}}`
}

func TestControlPageAndState(t *testing.T) {
	server, _ := fakeServer(t, snapshotACK)
	sim, err := New(Config{ServerURL: server.URL, SerialNumber: "camera-sim-1"})
	if err != nil {
		t.Fatal(err)
	}
	handler := sim.Handler()

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "摄像头车牌识别模拟器") || !strings.Contains(page.Body.String(), "模拟器 · 非真实摄像头") || !strings.Contains(page.Body.String(), "刷新车牌并上报") {
		t.Fatalf("unexpected page: %d %s", page.Code, page.Body.String())
	}
	if page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}

	stateResponse := httptest.NewRecorder()
	handler.ServeHTTP(stateResponse, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	var state State
	if err := json.Unmarshal(stateResponse.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Simulator || state.SerialNumber != "camera-sim-1" || state.EventID != "" {
		t.Fatalf("unexpected state: %+v", state)
	}

	control := func(contentType, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/control", strings.NewReader(body))
		request.Header.Set("Content-Type", contentType)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := control("text/plain", `{"action":"recognize"}`); response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content type: %d", response.Code)
	}
	if response := control("application/json", `{"action":"unknown"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown action: %d", response.Code)
	}
	if response := control("application/json", `{"action":"retry"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("retry without failure: %d", response.Code)
	}
	if response := control("application/json", `{"action":"recognize","extra":1}`); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", response.Code)
	}
	if response := control("application/json", `{"action":"recognize"}{}`); response.Code != http.StatusBadRequest {
		t.Fatalf("two objects: %d", response.Code)
	}

	response := control("application/json", `{"action":"report_current"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("report_current: %d %s", response.Code, response.Body.String())
	}
	var reported State
	if err := json.Unmarshal(response.Body.Bytes(), &reported); err != nil {
		t.Fatal(err)
	}
	if reported.LastReport == nil || !reported.LastReport.Success || reported.LastReport.Plate != state.Plate {
		t.Fatalf("report state: %+v", reported)
	}
	response = control("application/json", `{"action":"recognize"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("recognize: %d %s", response.Code, response.Body.String())
	}
}
