package camera

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHeartbeatProtocol(t *testing.T) {
	request, err := decodeHeartbeat([]byte(`{"operator":"heartbeat","deviceInfo":{"serialNumber":"camera-1"},"info":{"eventId":1234567890123456789,"time":"2026-10-29T12:00:00+08:00"}}`))
	if err != nil {
		t.Fatal(err)
	}
	ack, err := heartbeatACK(request, time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Operator string `json:"operator"`
		Info     struct {
			EventID json.Number `json:"eventId"`
			Time    string      `json:"time"`
			Mode    string      `json:"eventSendMode"`
		} `json:"info"`
		Result struct {
			ErrorNo     int    `json:"errorNo"`
			Description string `json:"description"`
		} `json:"result"`
		Strategy json.RawMessage `json:"strategy"`
	}
	if err := json.Unmarshal(ack, &result); err != nil {
		t.Fatal(err)
	}
	if result.Operator != "heartbeat-ack" || result.Info.EventID.String() != "1234567890123456789" || result.Info.Mode != "realTime" || result.Result.ErrorNo != 0 || result.Result.Description != "ok" || result.Strategy != nil {
		t.Fatalf("unexpected ACK: %s", ack)
	}
	if _, err := time.Parse(time.RFC3339, result.Info.Time); err != nil {
		t.Fatal(err)
	}
}

func TestHeartbeatRejectsIncompleteRequests(t *testing.T) {
	for _, body := range []string{`{`, `{}`, `{"operator":"heartbeat","deviceInfo":{"serialNumber":"a"},"info":{"eventId":null,"time":"x"}}`, `{"operator":"faceReco","deviceInfo":{"serialNumber":"a"},"info":{"eventId":1,"time":"x"}}`} {
		if _, err := decodeHeartbeat([]byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
