package weighing

import (
	"encoding/json"
	"reflect"
	"sort"
	"time"
)

// syncFieldDifferences compares the saved local payload with the cloud's
// business snapshot using the same weight and time normalization as the digest.
func syncFieldDifferences(change ticketSyncChangeV4, cloud syncCloudTicket) ([]map[string]any, error) {
	var local syncChangeV4
	if err := json.Unmarshal([]byte(change.Payload), &local); err != nil || local.Ticket == nil || cloud.Ticket == nil {
		return nil, ErrInvalid
	}
	localFields, err := syncTicketFields(local.Ticket)
	if err != nil {
		return nil, err
	}
	cloudFields, err := syncTicketFields(cloud.Ticket)
	if err != nil {
		return nil, err
	}
	localFields["status"] = "valid"
	if local.Type == "void" {
		localFields["status"] = "void"
	}
	cloudFields["status"] = cloud.Status
	var localPrevious any
	if local.PreviousSiteCode != "" {
		localPrevious = local.PreviousSiteCode
	}
	var cloudPrevious any
	if cloud.PreviousSiteCode != nil {
		cloudPrevious = *cloud.PreviousSiteCode
	}
	localFields["previousSiteCode"], cloudFields["previousSiteCode"] = localPrevious, cloudPrevious
	localFields["voidReason"], localFields["voidedBy"], localFields["voidedAt"] = nil, nil, nil
	if local.Void != nil {
		localFields["voidReason"], localFields["voidedBy"], localFields["voidedAt"] = local.Void.Reason, local.Void.Actor, local.Void.At.Format(time.RFC3339Nano)
	}
	cloudFields["voidReason"], cloudFields["voidedBy"], cloudFields["voidedAt"] = nil, nil, nil
	if cloud.VoidReason != nil {
		cloudFields["voidReason"] = *cloud.VoidReason
	}
	if cloud.VoidedBy != nil {
		cloudFields["voidedBy"] = *cloud.VoidedBy
	}
	if cloud.VoidedAt != nil {
		cloudFields["voidedAt"] = cloud.VoidedAt.Format(time.RFC3339Nano)
	}
	keys := make([]string, 0, len(localFields))
	for key := range localFields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	differences := make([]map[string]any, 0)
	for _, key := range keys {
		localValue, cloudValue := localFields[key], cloudFields[key]
		left, err := normalizeSyncField(key, localValue)
		if err != nil {
			return nil, err
		}
		right, err := normalizeSyncField(key, cloudValue)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(left, right) {
			differences = append(differences, map[string]any{"field": key, "local": localValue, "cloud": cloudValue})
		}
	}
	return differences, nil
}

func syncTicketFields(ticket *syncTicket) (map[string]any, error) {
	data, err := json.Marshal(ticket)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func normalizeSyncField(name string, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch name {
	case "originalGross", "originalTare", "finalGross", "finalTare", "finalNet", "previousFinalGross", "previousFinalTare", "previousFinalNet":
		text, ok := value.(string)
		if !ok {
			return nil, ErrInvalid
		}
		return canonicalDigestWeight(text, false)
	case "netAdjustment", "previousNetAdjustment":
		text, ok := value.(string)
		if !ok {
			return nil, ErrInvalid
		}
		return canonicalDigestWeight(text, true)
	case "firstConfirmedAt", "secondConfirmedAt", "issuedAt", "adjustedAt", "voidedAt":
		text, ok := value.(string)
		if !ok {
			return nil, ErrInvalid
		}
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return nil, err
		}
		return canonicalDigestTime(parsed)
	default:
		return value, nil
	}
}
