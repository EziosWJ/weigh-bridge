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

func heartbeatACK(request heartbeatRequest, now time.Time, strategyEnabled bool) ([]byte, error) {
	var strategy map[string]any
	if strategyEnabled {
		strategy = licenseSnapshotStrategy()
	}
	return json.Marshal(struct {
		Operator string `json:"operator"`
		Info     struct {
			EventID       json.RawMessage `json:"eventId"`
			Time          string          `json:"time"`
			EventSendMode string          `json:"eventSendMode"`
			Strategy      map[string]any  `json:"strategy,omitempty"`
		} `json:"info"`
		Result struct {
			ErrorNo     int    `json:"errorNo"`
			Description string `json:"description"`
		} `json:"result"`
	}{Operator: "heartbeat-ack", Info: struct {
		EventID       json.RawMessage `json:"eventId"`
		Time          string          `json:"time"`
		EventSendMode string          `json:"eventSendMode"`
		Strategy      map[string]any  `json:"strategy,omitempty"`
	}{request.Info.EventID, now.Format(time.RFC3339), "realTime", strategy}, Result: struct {
		ErrorNo     int    `json:"errorNo"`
		Description string `json:"description"`
	}{0, "ok"}})
}

// Fields and example interval values follow the manufacturer heartbeat table.
func licenseSnapshotStrategy() map[string]any {
	result := map[string]any{"passengerStaticsInterval": 2, "heartBeatInterval": 30, "isCarLicenseSnapshotEnable": true}
	for _, key := range []string{
		"isEnableElectronicDefence", "isCrossBorderDetectEnable", "isOffDutyDetectEnable", "isPassengerFlowStaticsEnable", "isCryScreamDetectEnable", "isPetDetectEnable", "isFallDetectEnable", "isSnapshotEnable", "isPersonInfoEnable", "isPersonDetectEnable", "isCarDetectEnable", "isTimedSnapshotEnable", "isGroundLockStatusChangedEnable", "isStartupReportEnable", "isAbnormalEventEnable", "isNonWhitelistCarEnable", "isMotionDetectEnable", "isTrafficStatisticsEnable", "isTrafficStatisticsCarShapeEnable", "isKey2CallEnable", "isLicensePlateSurveillanceEnable", "isFireDetectEventEnable", "isVideoCoverEventEnable", "isElectricBikeEventEnable", "isGasContainerEventEnable", "isElectronicDefenceV2EventEnable", "isPeopleNumberStatisticsEventEnable", "isPeopleNumberOverLimitEventEnable",
	} {
		result[key] = false
	}
	return result
}
