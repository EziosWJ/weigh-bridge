package camera

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

// heartbeatRequest contains only fields established by the manufacturer
// documentation. EventID remains raw JSON so its value is echoed unchanged.
type heartbeatRequest struct {
	Operator   string `json:"operator"`
	DeviceInfo struct {
		SerialNumber string `json:"serialNumber"`
	} `json:"deviceInfo"`
	Info struct {
		EventID json.RawMessage `json:"eventId"`
		Time    string          `json:"time"`
	} `json:"info"`
}

func decodeHeartbeat(body []byte) (heartbeatRequest, error) {
	var request heartbeatRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return request, err
	}
	id := bytes.TrimSpace(request.Info.EventID)
	if request.Operator != "heartbeat" || request.DeviceInfo.SerialNumber == "" || len(id) == 0 || bytes.Equal(id, []byte("null")) || request.Info.Time == "" {
		return request, errors.New("invalid heartbeat")
	}
	return request, nil
}

func heartbeatACK(request heartbeatRequest, now time.Time) ([]byte, error) {
	return json.Marshal(struct {
		Operator string `json:"operator"`
		Info     struct {
			EventID       json.RawMessage `json:"eventId"`
			Time          string          `json:"time"`
			EventSendMode string          `json:"eventSendMode"`
		} `json:"info"`
		Result struct {
			ErrorNo     int    `json:"errorNo"`
			Description string `json:"description"`
		} `json:"result"`
	}{Operator: "heartbeat-ack", Info: struct {
		EventID       json.RawMessage `json:"eventId"`
		Time          string          `json:"time"`
		EventSendMode string          `json:"eventSendMode"`
	}{request.Info.EventID, now.Format(time.RFC3339), "realTime"}, Result: struct {
		ErrorNo     int    `json:"errorNo"`
		Description string `json:"description"`
	}{0, "ok"}})
}
