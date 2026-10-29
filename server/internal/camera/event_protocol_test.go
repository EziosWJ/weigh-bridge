package camera

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestHeartbeatProtocol(t *testing.T) {
	request, err := decodeHeartbeat([]byte(`{"operator":"heartbeat","deviceInfo":{"serialNumber":"camera-1"},"info":{"eventId":1234567890123456789,"time":"2026-10-29T12:00:00+08:00"}}`))
	if err != nil {
		t.Fatal(err)
	}
	ack, err := heartbeatACK(request, time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Operator string `json:"operator"`
		Info     struct {
			EventID  json.Number     `json:"eventId"`
			Time     string          `json:"time"`
			Mode     string          `json:"eventSendMode"`
			Strategy json.RawMessage `json:"strategy"`
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
	if result.Operator != "heartbeat-ack" || result.Info.EventID.String() != "1234567890123456789" || result.Info.Mode != "realTime" || result.Result.ErrorNo != 0 || result.Result.Description != "ok" || (result.Strategy != nil || result.Info.Strategy != nil) {
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

func TestHeartbeatACKStrategy(t *testing.T) {
	request, err := decodeHeartbeat([]byte(`{"operator":"heartbeat","deviceInfo":{"serialNumber":"camera-1"},"info":{"eventId":42,"time":"x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	ack, err := heartbeatACK(request, time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), true)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Operator string `json:"operator"`
		Info     struct {
			EventID  int            `json:"eventId"`
			Mode     string         `json:"eventSendMode"`
			Strategy map[string]any `json:"strategy"`
		} `json:"info"`
		Result struct {
			ErrorNo     int    `json:"errorNo"`
			Description string `json:"description"`
		} `json:"result"`
		Strategy any `json:"strategy"`
	}
	if err := json.Unmarshal(ack, &result); err != nil {
		t.Fatal(err)
	}
	expected := map[string]any{"passengerStaticsInterval": float64(2), "heartBeatInterval": float64(30), "isCarLicenseSnapshotEnable": true}
	for _, key := range []string{"isEnableElectronicDefence", "isCrossBorderDetectEnable", "isOffDutyDetectEnable", "isPassengerFlowStaticsEnable", "isCryScreamDetectEnable", "isPetDetectEnable", "isFallDetectEnable", "isSnapshotEnable", "isPersonInfoEnable", "isPersonDetectEnable", "isCarDetectEnable", "isTimedSnapshotEnable", "isGroundLockStatusChangedEnable", "isStartupReportEnable", "isAbnormalEventEnable", "isNonWhitelistCarEnable", "isMotionDetectEnable", "isTrafficStatisticsEnable", "isTrafficStatisticsCarShapeEnable", "isKey2CallEnable", "isLicensePlateSurveillanceEnable", "isFireDetectEventEnable", "isVideoCoverEventEnable", "isElectricBikeEventEnable", "isGasContainerEventEnable", "isElectronicDefenceV2EventEnable", "isPeopleNumberStatisticsEventEnable", "isPeopleNumberOverLimitEventEnable"} {
		expected[key] = false
	}
	if !reflect.DeepEqual(result.Info.Strategy, expected) || result.Operator != "heartbeat-ack" || result.Info.EventID != 42 || result.Info.Mode != "realTime" || result.Result.ErrorNo != 0 || result.Result.Description != "ok" || result.Strategy != nil {
		t.Fatalf("unexpected ACK: %s", ack)
	}
}
