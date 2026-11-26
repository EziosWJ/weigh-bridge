package weighing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	mobileHandoffClaimPath     = "/api/v1/grain-outbound/handoffs/claim"
	mobileHandoffResultPath    = "/api/v1/grain-outbound/handoffs/result"
	mobileHandoffTerminatePath = "/api/v1/grain-outbound/handoffs/terminate"
)

func (s *Service) claimMobileHandoff(ctx context.Context, token, siteCode, scaleCode string) (mobileClaimResponse, error) {
	if s.masterSyncAddress == "" {
		return mobileClaimResponse{}, fmt.Errorf("云端地址未配置")
	}
	// A scan is an operator-facing action; keep a cloud outage from blocking
	// the serial scanner for the shared 30s sync timeout.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	target, err := syncV4URL(s.masterSyncAddress, mobileHandoffClaimPath, nil)
	if err != nil {
		return mobileClaimResponse{}, err
	}
	body, _ := json.Marshal(map[string]any{"token": token, "siteCode": siteCode, "scaleCode": scaleCode})
	envelope, status, err := s.syncV4Client.do(ctx, http.MethodPost, target, body)
	if err != nil {
		return mobileClaimResponse{}, err
	}
	if status != http.StatusOK || envelope.Code != http.StatusOK {
		message := envelope.Message
		if message == "" {
			message = "云端拒绝扫码"
		}
		return mobileClaimResponse{}, RejectionError(message)
	}
	var result mobileClaimResponse
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return mobileClaimResponse{}, RejectionError("云端扫码响应无效")
	}
	return result, nil
}

func (s *Service) reportMobileHandoff(ctx context.Context, in mobileResultRequest) error {
	if s.masterSyncAddress == "" {
		return fmt.Errorf("云端地址未配置")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	target, err := syncV4URL(s.masterSyncAddress, mobileHandoffResultPath, nil)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(in)
	envelope, status, err := s.syncV4Client.do(ctx, http.MethodPost, target, body)
	if err != nil {
		return err
	}
	if status != http.StatusOK || envelope.Code != http.StatusOK {
		message := envelope.Message
		if message == "" {
			message = "云端接收扫码结果失败"
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}

func (s *Service) terminateMobileHandoff(ctx context.Context, siteCode, taskNo, reason string) error {
	if s.masterSyncAddress == "" {
		return nil
	}
	target, err := syncV4URL(s.masterSyncAddress, mobileHandoffTerminatePath, nil)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(mobileTerminateRequest{SiteCode: siteCode, FactoryTaskNo: taskNo, Reason: reason})
	envelope, status, err := s.syncV4Client.do(ctx, http.MethodPost, target, body)
	if err != nil {
		return err
	}
	if status != http.StatusOK || envelope.Code != http.StatusOK {
		message := envelope.Message
		if message == "" {
			message = "云端未确认终止"
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}
