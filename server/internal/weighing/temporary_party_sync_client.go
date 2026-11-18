package weighing

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
)

const temporaryPartyRegistrationPath = "/api/v1/weighbridge/sync/temporary-parties"

type temporaryPartyRegistration struct {
	SchemaVersion int    `json:"schemaVersion"`
	SiteCode      string `json:"siteCode"`
	TemporaryCode string `json:"temporaryCode"`
	Version       int64  `json:"version"`
	Name          string `json:"name"`
	Nature        string `json:"nature"`
	IsSupplier    bool   `json:"isSupplier"`
	IsCustomer    bool   `json:"isCustomer"`
	IsCarrier     bool   `json:"isCarrier"`
}

func (c *syncV4Client) uploadTemporaryParty(ctx context.Context, address string, row temporaryPartyOutbox) *SyncCloudError {
	target, err := syncV4URL(address, temporaryPartyRegistrationPath, nil)
	if err != nil {
		return &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: "invalid_address", Message: "云端地址无效，请检查同步配置", Retryable: true}
	}
	envelope, status, err := c.do(ctx, http.MethodPost, target, []byte(row.Payload))
	if err != nil {
		return &SyncCloudError{Category: SyncCategoryTemporary, Reason: "unknown_response", Message: err.Error(), Retryable: true}
	}
	if status == http.StatusOK && envelope.Code == http.StatusOK {
		var receipt struct {
			SchemaVersion int    `json:"schemaVersion"`
			SiteCode      string `json:"siteCode"`
			TemporaryCode string `json:"temporaryCode"`
			Version       int64  `json:"version"`
			Status        string `json:"status"`
		}
		if json.Unmarshal(envelope.Data, &receipt) == nil &&
			receipt.SchemaVersion == 1 && receipt.SiteCode == row.SiteCode &&
			receipt.TemporaryCode == row.TemporaryCode && receipt.Version >= row.Version && receipt.Version <= math.MaxInt32 {
			switch receipt.Status {
			case "pending", "needs_info", "merged", "disabled":
				return nil
			}
		}
	}

	var detail struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(envelope.Data, &detail)
	switch {
	case status == http.StatusBadRequest && detail.Kind == "invalid":
		return &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: detail.Kind, Message: "云端拒绝登记资料或协议，请核对原登记"}
	case status == http.StatusUnauthorized && detail.Kind == "invalid_site":
		return &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: detail.Kind, Message: "云端拒绝厂区编码，请检查编码是否存在且启用", Retryable: true}
	case status == http.StatusConflict && detail.Kind == "version_conflict":
		return &SyncCloudError{Category: SyncCategoryConflict, Reason: detail.Kind, Message: "云端已有同版本但内容不同的登记，请核对厂端和总部资料"}
	case status == http.StatusConflict && detail.Kind == "finalized":
		return &SyncCloudError{Category: SyncCategoryConflict, Reason: detail.Kind, Message: "总部已归并或停用该登记，请同步资料并核对处理结果"}
	case status == http.StatusInternalServerError && detail.Kind == "temporary":
		return &SyncCloudError{Category: SyncCategoryTemporary, Reason: detail.Kind, Message: "云端暂时无法保存登记", Retryable: true}
	default:
		return &SyncCloudError{Category: SyncCategoryTemporary, Reason: "unknown_response", Message: "云端未返回可核实的登记回执，结果未知", Retryable: true}
	}
}
