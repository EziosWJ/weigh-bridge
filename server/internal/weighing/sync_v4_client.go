package weighing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type syncV4Client struct {
	client *http.Client
}

func newSyncV4Client() *syncV4Client {
	return &syncV4Client{client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type syncEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

var syncDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func invalidCloudResponse() *SyncCloudError {
	return &SyncCloudError{Category: SyncCategoryTemporary, Reason: "invalid_response", Retryable: true, Message: "云端对账响应无效，结果未知"}
}

// decodeCloudTicket validates the response identity and required presence of
// found before any missing-ticket or confirmation decision can be made.
func decodeCloudTicket(data json.RawMessage, ref syncTicketRef, withHistory bool) (syncCloudTicket, *SyncCloudError) {
	var required struct {
		Found *bool `json:"found"`
	}
	var result syncCloudTicket
	if json.Unmarshal(data, &required) != nil || required.Found == nil || json.Unmarshal(data, &result) != nil || result.SiteCode != ref.SiteCode || result.TicketNo != ref.TicketNo {
		return syncCloudTicket{}, invalidCloudResponse()
	}
	if result.Error != nil || !result.Found {
		return result, nil
	}
	if result.Status != "valid" && result.Status != "void" {
		return syncCloudTicket{}, invalidCloudResponse()
	}
	if withHistory && (result.Ticket == nil || result.Ticket.TicketNo != ref.TicketNo || result.Changes == nil) {
		return syncCloudTicket{}, invalidCloudResponse()
	}
	if result.LatestDigest != nil {
		if !syncDigestPattern.MatchString(*result.LatestDigest) || result.LatestDigestVersion == nil || *result.LatestDigestVersion < 1 || result.LatestReceivedAt == nil || result.LatestReceivedAt.IsZero() || result.LatestChangeType == nil || !validSyncChangeType(*result.LatestChangeType) {
			return syncCloudTicket{}, invalidCloudResponse()
		}
	}
	latestFound := false
	for _, change := range result.Changes {
		if !syncDigestPattern.MatchString(change.Digest) || change.TicketNo != ref.TicketNo || !validSyncChangeType(change.ChangeType) || change.DigestVersion < 1 || (change.SchemaVersion != SyncSchemaVersionV4 && change.SchemaVersion != SyncSchemaVersionV5) || change.ReceivedAt.IsZero() {
			return syncCloudTicket{}, invalidCloudResponse()
		}
		if result.LatestDigest != nil && change.Digest == *result.LatestDigest {
			if latestFound || change.DigestVersion != *result.LatestDigestVersion || change.ChangeType != *result.LatestChangeType || !change.ReceivedAt.Equal(*result.LatestReceivedAt) {
				return syncCloudTicket{}, invalidCloudResponse()
			}
			latestFound = true
		}
	}
	if withHistory && result.LatestDigest != nil && !latestFound {
		return syncCloudTicket{}, invalidCloudResponse()
	}
	return result, nil
}

func validSyncChangeType(value string) bool {
	return value == "issue" || value == "void" || value == "reopen"
}

func syncV4URL(address, path string, query url.Values) (string, error) {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("云端地址无效")
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawPath = ""
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func (c *syncV4Client) do(ctx context.Context, method, target string, body []byte) (syncEnvelope, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return syncEnvelope{}, 0, fmt.Errorf("云端请求无效")
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return syncEnvelope{}, 0, fmt.Errorf("连接云端失败或结果未知")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return syncEnvelope{}, response.StatusCode, fmt.Errorf("云端响应读取失败，结果未知")
	}
	var envelope syncEnvelope
	if len(data) > 0 {
		_ = json.Unmarshal(data, &envelope)
	}
	return envelope, response.StatusCode, nil
}

// syncFailure turns a cloud rejection into the documented category. A response
// without a recognised category is treated as an unknown, retryable result
// instead of being inferred from the HTTP status alone.
func syncFailure(status int, envelope syncEnvelope) *SyncCloudError {
	var detail SyncCloudError
	if len(envelope.Data) > 0 {
		_ = json.Unmarshal(envelope.Data, &detail)
	}
	switch detail.Category {
	case SyncCategoryTemporary, SyncCategoryPredecessorPending, SyncCategoryConfigOrData, SyncCategoryConflict:
		detail.Retryable = detail.Category == SyncCategoryTemporary || detail.Category == SyncCategoryPredecessorPending
		if detail.Message == "" {
			detail.Message = envelope.Message
		}
		if detail.Message == "" {
			detail.Message = "云端未确认该变化"
		}
		return &detail
	}
	if status == http.StatusUnauthorized {
		return &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: "site_not_found_or_disabled", Message: "云端拒绝厂区编码：编码不存在或已停用"}
	}
	if status == http.StatusRequestEntityTooLarge {
		return &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: "request_too_large", Message: "同步请求超过云端大小限制"}
	}
	message := envelope.Message
	if message == "" {
		message = "云端响应无法分类，结果未知"
	}
	return &SyncCloudError{Category: SyncCategoryTemporary, Reason: "unknown_response", Retryable: true, Message: message}
}

const (
	syncV4ChangesPath   = "/api/v1/weighbridge/sync/v4/changes"
	syncV4TicketsPath   = "/api/v1/weighbridge/sync/v4/tickets"
	syncV4ReconcilePath = "/api/v1/weighbridge/sync/v4/reconcile"
	syncV5ChangesPath   = "/api/v1/weighbridge/sync/v5/changes"
	syncV5TicketsPath   = "/api/v1/weighbridge/sync/v5/tickets"
	syncV5ReconcilePath = "/api/v1/weighbridge/sync/v5/reconcile"
)

func (c *syncV4Client) uploadChange(ctx context.Context, address, payload, digest, ticketNo, changeType string) (syncChangeV4Receipt, *SyncCloudError) {
	return c.uploadChangeAt(ctx, address, syncV4ChangesPath, payload, digest, ticketNo, changeType)
}

func (c *syncV4Client) uploadChangeV5(ctx context.Context, address, payload, digest, ticketNo, changeType string) (syncChangeV4Receipt, *SyncCloudError) {
	return c.uploadChangeAt(ctx, address, syncV5ChangesPath, payload, digest, ticketNo, changeType)
}

func (c *syncV4Client) uploadChangeAt(ctx context.Context, address, path, payload, digest, ticketNo, changeType string) (syncChangeV4Receipt, *SyncCloudError) {
	target, err := syncV4URL(address, path, nil)
	if err != nil {
		return syncChangeV4Receipt{}, &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: "invalid_address", Message: err.Error()}
	}
	envelope, status, err := c.do(ctx, http.MethodPost, target, []byte(payload))
	if err != nil {
		return syncChangeV4Receipt{}, &SyncCloudError{Category: SyncCategoryTemporary, Reason: "transport", Retryable: true, Message: err.Error()}
	}
	if status != http.StatusOK || envelope.Code != 200 {
		return syncChangeV4Receipt{}, syncFailure(status, envelope)
	}
	var receipt syncChangeV4Receipt
	if err := json.Unmarshal(envelope.Data, &receipt); err != nil || receipt.Digest != digest || receipt.TicketNo != ticketNo || receipt.ChangeType != changeType || receipt.ReceivedAt.IsZero() {
		return syncChangeV4Receipt{}, &SyncCloudError{Category: SyncCategoryTemporary, Reason: "invalid_confirmation", Retryable: true, Message: "云端确认无效，结果未知"}
	}
	return receipt, nil
}

func (c *syncV4Client) fetchTicket(ctx context.Context, address, siteCode, ticketNo string) (syncCloudTicket, *SyncCloudError) {
	query := url.Values{"siteCode": {siteCode}, "ticketNo": {ticketNo}}
	target, err := syncV4URL(address, syncV5TicketsPath, query)
	if err != nil {
		return syncCloudTicket{}, &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: "invalid_address", Message: err.Error()}
	}
	envelope, status, err := c.do(ctx, http.MethodGet, target, nil)
	if err != nil {
		return syncCloudTicket{}, &SyncCloudError{Category: SyncCategoryTemporary, Reason: "transport", Retryable: true, Message: err.Error()}
	}
	if status != http.StatusOK || envelope.Code != 200 {
		return syncCloudTicket{}, syncFailure(status, envelope)
	}
	return decodeCloudTicket(envelope.Data, syncTicketRef{SiteCode: siteCode, TicketNo: ticketNo}, true)
}

func (c *syncV4Client) fetchBatch(ctx context.Context, address string, items []syncTicketRef) ([]syncCloudTicket, *SyncCloudError) {
	if len(items) == 0 || len(items) > MaxBatchReconcile {
		return nil, &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: "invalid_request", Message: "批量对账数量必须为 1 至 200 条"}
	}
	body, err := json.Marshal(struct {
		Items []syncTicketRef `json:"items"`
	}{Items: items})
	if err != nil {
		return nil, &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: "invalid_request", Message: "批量对账请求无效"}
	}
	target, err := syncV4URL(address, syncV5ReconcilePath, nil)
	if err != nil {
		return nil, &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: "invalid_address", Message: err.Error()}
	}
	envelope, status, err := c.do(ctx, http.MethodPost, target, body)
	if err != nil {
		return nil, &SyncCloudError{Category: SyncCategoryTemporary, Reason: "transport", Retryable: true, Message: err.Error()}
	}
	if status != http.StatusOK || envelope.Code != 200 {
		return nil, syncFailure(status, envelope)
	}
	var result struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return nil, &SyncCloudError{Category: SyncCategoryTemporary, Reason: "invalid_response", Retryable: true, Message: "云端批量对账响应无效，结果未知"}
	}
	if len(result.Items) != len(items) {
		return nil, &SyncCloudError{Category: SyncCategoryTemporary, Reason: "invalid_response", Retryable: true, Message: "云端批量对账返回数量不符，结果未知"}
	}
	decoded := make([]syncCloudTicket, 0, len(items))
	for i, raw := range result.Items {
		item, cloudErr := decodeCloudTicket(raw, items[i], false)
		if cloudErr != nil {
			return nil, cloudErr
		}
		decoded = append(decoded, item)
	}
	return decoded, nil
}

func (c *syncV4Client) lookupChange(ctx context.Context, address, siteCode, digest string) (syncCloudLookup, *SyncCloudError) {
	query := url.Values{"siteCode": {siteCode}, "digest": {digest}}
	target, err := syncV4URL(address, syncV5ChangesPath, query)
	if err != nil {
		return syncCloudLookup{}, &SyncCloudError{Category: SyncCategoryConfigOrData, Reason: "invalid_address", Message: err.Error()}
	}
	envelope, status, err := c.do(ctx, http.MethodGet, target, nil)
	if err != nil {
		return syncCloudLookup{}, &SyncCloudError{Category: SyncCategoryTemporary, Reason: "transport", Retryable: true, Message: err.Error()}
	}
	if status != http.StatusOK || envelope.Code != 200 {
		return syncCloudLookup{}, syncFailure(status, envelope)
	}
	var result syncCloudLookup
	var required struct {
		Found *bool `json:"found"`
	}
	if err := json.Unmarshal(envelope.Data, &result); err != nil || json.Unmarshal(envelope.Data, &required) != nil || required.Found == nil || result.SiteCode != siteCode || result.Digest != digest || (result.Found && (result.Confirmation == nil || result.Confirmation.Digest != digest || result.Confirmation.TicketNo == "" || !validSyncChangeType(result.Confirmation.ChangeType) || result.Confirmation.DigestVersion < 1 || (result.Confirmation.SchemaVersion != SyncSchemaVersionV4 && result.Confirmation.SchemaVersion != SyncSchemaVersionV5) || result.Confirmation.ReceivedAt.IsZero())) {
		return syncCloudLookup{}, &SyncCloudError{Category: SyncCategoryTemporary, Reason: "invalid_response", Retryable: true, Message: "云端摘要确认响应无效，结果未知"}
	}
	return result, nil
}
