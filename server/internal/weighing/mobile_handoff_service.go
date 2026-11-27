package weighing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/localsite"
)

// ScanMobileHandoff validates a cloud QR, captures the factory weight/photos,
// reuses the normal factory first/second commands, and reports the result back
// to the cloud. The one-time cloud handoff id is the idempotency key.
func (s *Service) ScanMobileHandoff(ctx context.Context, code string, e audit.Event) (MobileHandoffResult, error) {
	return s.scanMobileHandoff(ctx, code, "operator", "", e)
}

// ScanMobileHandoffFrom is the serial Agent entry point. source/device are
// persisted on the local handoff row for audit and the read-only feedback page.
// The Agent passes an audit.Event with ActorID 0 (system identity), never a
// user login token.
func (s *Service) ScanMobileHandoffFrom(ctx context.Context, code, source, device string, e audit.Event) (MobileHandoffResult, error) {
	return s.scanMobileHandoff(ctx, code, source, device, e)
}

func (s *Service) scanMobileHandoff(ctx context.Context, code, source, device string, e audit.Event) (MobileHandoffResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "operator"
	}
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(code), "fc-handoff:"))
	if token == "" {
		return MobileHandoffResult{}, ErrInvalid
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return MobileHandoffResult{}, ErrForbidden
	}
	local, err := s.LocalStatus(ctx)
	if err != nil {
		return MobileHandoffResult{}, err
	}
	if !local.Configured || local.SiteCode == "" || local.ScaleCode == "" {
		return MobileHandoffResult{}, fmt.Errorf("本厂身份未配置")
	}
	claim, err := s.claimMobileHandoff(ctx, token, local.SiteCode, local.ScaleCode)
	if err != nil {
		return MobileHandoffResult{}, err
	}
	if claim.SiteCode != local.SiteCode || claim.ScaleCode != local.ScaleCode {
		return MobileHandoffResult{}, RejectionError("二维码不属于当前厂区或地磅")
	}
	// The factory task/ticket is still created by the system scanner identity,
	// but operators should see the mobile account that initiated the weighing.
	e.Metadata.ActorName = mobileActorName(claim.MobileAccount)
	row, err := repo.MobileHandoff(ctx, claim.HandoffID, claim.Action)
	if err != nil {
		return MobileHandoffResult{}, err
	}
	if row != nil && row.Status == MobileHandoffCompleted {
		result := decodeMobileResult(row.Result)
		// Best effort: a previous factory success may not have reached the cloud.
		s.replayMobileResult(ctx, token, local.SiteCode, local.ScaleCode, claim, result)
		return result, nil
	}
	if claim.Status == "completed" {
		return MobileHandoffResult{}, RejectionError("二维码已使用，请查询厂端过磅记录")
	}
	if row == nil {
		row = &MobileHandoff{
			HandoffID: claim.HandoffID, WeighingID: claim.WeighingID,
			Action: claim.Action, SiteCode: local.SiteCode, ScaleCode: local.ScaleCode,
			Plate: claim.Plate, MobileAccount: claim.MobileAccount,
			Source: source, Device: device,
			Status: MobileHandoffClaimed, ClaimedAt: time.Now().UTC(),
		}
	} else {
		row.Status = MobileHandoffClaimed
		row.Source = source
		row.Device = device
		row.Error = ""
	}
	if err := repo.SaveMobileHandoff(ctx, row); err != nil {
		return MobileHandoffResult{}, err
	}
	// A previous attempt may have committed the factory task/ticket and then
	// failed while saving or reporting the handoff. Recover from the local
	// records instead of trying to create a duplicate. This also handles a
	// new QR generated after the previous token failed after task creation.
	if result, ok := s.recoverMobileHandoff(ctx, repo, claim, row); ok {
		row.Status = MobileHandoffCompleted
		row.Result = snapshot(result)
		row.Error = ""
		if err := repo.SaveMobileHandoff(ctx, row); err == nil {
			_ = s.reportMobileResult(context.WithoutCancel(ctx), token, local.SiteCode, local.ScaleCode, claim, result)
			return result, nil
		}
	}

	clientID, reason := "mobile-scanner", "手机二维码扫码"
	if source == "serial_agent" {
		clientID, reason = "serial-scanner", "串口扫码枪扫码"
	}
	sessionInput := SessionInput{ClientID: clientID, Reason: reason}
	session, err := s.ChangeSession(ctx, local.ScaleCode, "acquire", sessionInput, e)
	if err != nil {
		result := failedMobileResult(claim, "地磅已被其他工作台占用")
		s.finishMobileFailure(ctx, repo, token, local, claim, row, result)
		return result, err
	}
	sessionInput.SessionToken = session.SessionToken
	defer func() {
		_, _ = s.ChangeSession(context.WithoutCancel(ctx), local.ScaleCode, "release", sessionInput, e)
	}()

	result, err := s.captureMobileHandoff(ctx, repo, local, claim, row, sessionInput, e)
	if err != nil {
		failure := failedMobileResult(claim, mobileHandoffMessage(err))
		s.finishMobileFailure(ctx, repo, token, local, claim, row, failure)
		return failure, err
	}
	row.Status = MobileHandoffCompleted
	row.TaskID = result.taskID
	row.TicketID = result.ticketID
	row.FirstRecordID = result.firstRecordID
	row.SecondRecordID = result.secondRecordID
	row.Result = snapshot(result.MobileHandoffResult)
	row.Error = ""
	if err := repo.SaveMobileHandoff(ctx, row); err != nil {
		return result.MobileHandoffResult, err
	}
	if err := s.reportMobileResult(ctx, token, local.SiteCode, local.ScaleCode, claim, result.MobileHandoffResult); err != nil {
		result.MobileHandoffResult.Message = "厂端已完成，云端反馈待同步：" + err.Error()
	}
	return result.MobileHandoffResult, nil
}

func (s *Service) MobileHandoffs(ctx context.Context, q PageQuery) (Page[MobileHandoff], error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return Page[MobileHandoff]{}, ErrForbidden
	}
	return repo.MobileHandoffs(ctx, q)
}

type mobileCaptureResult struct {
	MobileHandoffResult
	taskID         *int64
	ticketID       *int64
	firstRecordID  *int64
	secondRecordID *int64
}

func (s *Service) captureMobileHandoff(ctx context.Context, repo *Repository, local localsite.Status, claim mobileClaimResponse, row *MobileHandoff, sessionInput SessionInput, e audit.Event) (mobileCaptureResult, error) {
	requestID := fmt.Sprintf("mobile-%d-%s", claim.HandoffID, claim.Action)
	if claim.Action == "first" {
		return s.captureMobileFirst(ctx, repo, local, claim, sessionInput, e, requestID)
	}
	if claim.Action == "second" {
		return s.captureMobileSecond(ctx, repo, local, claim, sessionInput, e, requestID)
	}
	return mobileCaptureResult{}, ErrInvalid
}

func (s *Service) captureMobileFirst(ctx context.Context, repo *Repository, local localsite.Status, claim mobileClaimResponse, sessionInput SessionInput, e audit.Event, requestID string) (mobileCaptureResult, error) {
	if existing, err := s.ActiveTask(ctx, claim.Plate); err != nil {
		return mobileCaptureResult{}, err
	} else if existing != nil {
		return mobileCaptureResult{}, RejectionError("该车牌已有待回磅任务，请先在厂端处理")
	}
	materialID, err := repo.MaterialIDByCloudCode(ctx, claim.MaterialCode)
	if err != nil {
		return mobileCaptureResult{}, RejectionError("厂端未同步酒糟物料，请先同步总部资料")
	}
	partyID, err := repo.PartyIDByCloudCode(ctx, claim.PartyCode)
	if err != nil {
		return mobileCaptureResult{}, RejectionError("厂端未同步客户资料，请先同步总部资料")
	}
	business := BusinessInput{Plate: claim.Plate, Direction: "outbound", MaterialID: materialID, PartyID: partyID}
	capture, err := s.Capture(ctx, CaptureInput{ScaleCode: local.ScaleCode, SessionInput: sessionInput, Business: business}, e)
	if err != nil {
		return mobileCaptureResult{}, err
	}
	checks := Checks{Plate: true, Business: true, Weight: true, Evidence: true, FullyOnScale: true}
	confirmation, err := s.ConfirmFirst(ctx, ConfirmationInput{
		CaptureInput: CaptureInput{ScaleCode: local.ScaleCode, SessionInput: sessionInput, Business: business},
		RequestID:    requestID, SampleID: capture.ID, EntryType: "mobile_scan", Checks: checks,
	}, e)
	if err != nil {
		return mobileCaptureResult{}, err
	}
	task, err := s.FindTask(ctx, confirmation.TaskID)
	if err != nil {
		return mobileCaptureResult{}, err
	}
	firstWeight := capture.Weight
	firstAt := capture.CapturedAt
	if len(task.Records) > 0 {
		firstWeight = task.Records[0].Weight
		firstAt = task.Records[0].CapturedAt
	}
	firstID := confirmation.RecordID
	result := MobileHandoffResult{
		HandoffID: claim.HandoffID, WeighingID: claim.WeighingID, Action: "first",
		Status: "first_recorded", TaskNo: task.TaskNo, FirstWeight: firstWeight,
		FirstAt: &firstAt, CaptureStatus: captureStatus(capture), Message: "",
	}
	return mobileCaptureResult{MobileHandoffResult: result, taskID: &confirmation.TaskID, firstRecordID: &firstID}, nil
}

func (s *Service) captureMobileSecond(ctx context.Context, repo *Repository, local localsite.Status, claim mobileClaimResponse, sessionInput SessionInput, e audit.Event, requestID string) (mobileCaptureResult, error) {
	task, err := s.mobileTask(ctx, repo, claim)
	if err != nil {
		return mobileCaptureResult{}, err
	}
	capture, err := s.Capture(ctx, CaptureInput{ScaleCode: local.ScaleCode, SessionInput: sessionInput, TaskID: &task.ID}, e)
	if err != nil {
		return mobileCaptureResult{}, err
	}
	checks := Checks{Plate: true, Business: true, Weight: true, Evidence: true, FullyOnScale: true}
	confirmation, err := s.ConfirmSecond(ctx, task.ID, ConfirmationInput{
		CaptureInput: CaptureInput{ScaleCode: local.ScaleCode, SessionInput: sessionInput},
		RequestID:    requestID, SampleID: capture.ID, Version: task.Version,
		EntryType: "mobile_scan", Checks: checks,
	}, e)
	if err != nil {
		return mobileCaptureResult{}, err
	}
	ticket, err := s.FindTicket(ctx, confirmation.TicketID)
	if err != nil {
		return mobileCaptureResult{}, err
	}
	if len(ticket.Basis) < 2 {
		return mobileCaptureResult{}, RejectionError("厂端磅单缺少首回磅依据")
	}
	firstAt := ticket.Basis[0].CapturedAt
	secondAt := ticket.Basis[1].CapturedAt
	secondID := confirmation.RecordID
	taskID := task.ID
	ticketID := confirmation.TicketID
	result := MobileHandoffResult{
		HandoffID: claim.HandoffID, WeighingID: claim.WeighingID, Action: "second",
		Status: "completed", TaskNo: task.TaskNo, TicketNo: ticket.TicketNo,
		FirstWeight: ticket.Basis[0].Weight, SecondWeight: ticket.Basis[1].Weight,
		Net: ticket.FinalNet, FirstAt: &firstAt, SecondAt: &secondAt,
		CaptureStatus: captureStatus(capture), Message: "",
	}
	return mobileCaptureResult{MobileHandoffResult: result, taskID: &taskID, ticketID: &ticketID, firstRecordID: task.CurrentFirstRecordID, secondRecordID: &secondID}, nil
}

func (s *Service) recoverMobileHandoff(ctx context.Context, repo *Repository, claim mobileClaimResponse, row *MobileHandoff) (MobileHandoffResult, bool) {
	if claim.Action == "first" {
		active, err := s.ActiveTask(ctx, claim.Plate)
		if err != nil || active == nil {
			return MobileHandoffResult{}, false
		}
		if !active.CreateTime.After(row.ClaimedAt.Add(-time.Minute)) || active.CreateTime.After(row.ClaimedAt.Add(10*time.Minute)) {
			return MobileHandoffResult{}, false
		}
		result := firstResultFromTask(claim, *active)
		row.TaskID = &active.ID
		row.FirstRecordID = active.CurrentFirstRecordID
		return result, true
	}
	if claim.Action == "second" {
		var task *Task
		if claim.FactoryTaskNo != "" {
			task, _ = repo.TaskByNo(ctx, claim.FactoryTaskNo)
		}
		if task == nil {
			active, err := s.ActiveTask(ctx, claim.Plate)
			if err != nil || active == nil {
				return MobileHandoffResult{}, false
			}
			task = &active.Task
		}
		ticket, err := repo.TicketByTask(ctx, task.ID)
		if err != nil || ticket == nil {
			return MobileHandoffResult{}, false
		}
		result := secondResultFromTicket(claim, *task, *ticket)
		row.TaskID = &task.ID
		row.TicketID = &ticket.ID
		row.FirstRecordID = task.CurrentFirstRecordID
		row.SecondRecordID = task.CurrentSecondRecordID
		return result, true
	}
	return MobileHandoffResult{}, false
}

func firstResultFromTask(claim mobileClaimResponse, task TaskDetail) MobileHandoffResult {
	weight := Tonnes(0)
	at := time.Time{}
	if len(task.Records) > 0 {
		weight = task.Records[0].Weight
		at = task.Records[0].CapturedAt
	}
	return MobileHandoffResult{
		HandoffID: claim.HandoffID, WeighingID: claim.WeighingID, Action: "first",
		Status: "first_recorded", TaskNo: task.TaskNo, FirstWeight: weight,
		FirstAt: &at, CaptureStatus: captureStatusFromEvidence(task.Records),
	}
}

func secondResultFromTicket(claim mobileClaimResponse, task Task, ticket TicketView) MobileHandoffResult {
	if len(ticket.Basis) < 2 {
		return MobileHandoffResult{HandoffID: claim.HandoffID, WeighingID: claim.WeighingID, Action: "second", Status: "failed", Message: "厂端磅单缺少首回磅依据"}
	}
	firstAt := ticket.Basis[0].CapturedAt
	secondAt := ticket.Basis[1].CapturedAt
	return MobileHandoffResult{
		HandoffID: claim.HandoffID, WeighingID: claim.WeighingID, Action: "second",
		Status: "completed", TaskNo: task.TaskNo, TicketNo: ticket.TicketNo,
		FirstWeight: ticket.Basis[0].Weight, SecondWeight: ticket.Basis[1].Weight,
		Net: ticket.FinalNet, FirstAt: &firstAt, SecondAt: &secondAt,
		CaptureStatus: captureStatusFromEvidence(ticket.Basis),
	}
}

func captureStatusFromEvidence(records []WeighRecord) string {
	if len(records) == 0 {
		return "unknown"
	}
	return captureStatusFromEvidenceJSON(records[len(records)-1].Evidence)
}

func captureStatusFromEvidenceJSON(raw JSON) string {
	var evidence struct {
		ImageStatus string `json:"imageStatus"`
	}
	_ = json.Unmarshal(raw, &evidence)
	if evidence.ImageStatus == "" {
		return "unknown"
	}
	return evidence.ImageStatus
}

func (s *Service) mobileTask(ctx context.Context, repo *Repository, claim mobileClaimResponse) (*Task, error) {
	if claim.FactoryTaskNo != "" {
		task, err := repo.TaskByNo(ctx, claim.FactoryTaskNo)
		if err != nil {
			return nil, err
		}
		if task == nil || task.Status != "awaiting_second" {
			return nil, RejectionError("首磅任务不存在或已结束")
		}
		return task, nil
	}
	active, err := s.ActiveTask(ctx, claim.Plate)
	if err != nil {
		return nil, err
	}
	if active == nil {
		return nil, RejectionError("未找到待回磅任务")
	}
	task := active.Task
	return &task, nil
}

func (s *Service) finishMobileFailure(ctx context.Context, repo *Repository, token string, local localsite.Status, claim mobileClaimResponse, row *MobileHandoff, result MobileHandoffResult) {
	row.Status = MobileHandoffFailed
	row.Result = snapshot(result)
	row.Error = result.Message
	_ = repo.SaveMobileHandoff(context.WithoutCancel(ctx), row)
	_ = s.reportMobileResult(context.WithoutCancel(ctx), token, local.SiteCode, local.ScaleCode, claim, result)
}

func (s *Service) reportMobileResult(ctx context.Context, token, siteCode, scaleCode string, claim mobileClaimResponse, result MobileHandoffResult) error {
	firstAt, secondAt := time.Time{}, time.Time{}
	if result.FirstAt != nil {
		firstAt = *result.FirstAt
	}
	if result.SecondAt != nil {
		secondAt = *result.SecondAt
	}
	return s.reportMobileHandoff(ctx, mobileResultRequest{
		Token: token, SiteCode: siteCode, ScaleCode: scaleCode,
		Action: claim.Action, Status: result.Status,
		FactoryTaskNo: result.TaskNo, FactoryTicketNo: result.TicketNo,
		FirstWeightMilliTonne: int64(result.FirstWeight), FirstAt: firstAt,
		SecondWeightMilliTonne: int64(result.SecondWeight), SecondAt: secondAt,
		NetMilliTonne: int64(result.Net), CaptureStatus: result.CaptureStatus, Message: result.Message,
	})
}

func (s *Service) replayMobileResult(ctx context.Context, token, siteCode, scaleCode string, claim mobileClaimResponse, result MobileHandoffResult) {
	_ = s.reportMobileResult(context.WithoutCancel(ctx), token, siteCode, scaleCode, claim, result)
}

func failedMobileResult(claim mobileClaimResponse, message string) MobileHandoffResult {
	return MobileHandoffResult{HandoffID: claim.HandoffID, WeighingID: claim.WeighingID, Action: claim.Action, Status: "failed", Message: message}
}

func mobileActorName(account string) string {
	if name := strings.TrimSpace(account); name != "" {
		return name
	}
	return "手机扫码"
}

func decodeMobileResult(raw JSON) MobileHandoffResult {
	var result MobileHandoffResult
	_ = json.Unmarshal(raw, &result)
	return result
}

func captureStatus(sample Sample) string {
	var evidence struct {
		ImageStatus string `json:"imageStatus"`
	}
	_ = json.Unmarshal(sample.Evidence, &evidence)
	if evidence.ImageStatus == "" {
		return "unknown"
	}
	return evidence.ImageStatus
}

func mobileHandoffMessage(err error) string {
	switch {
	case errors.Is(err, ErrConditions):
		return "采集条件不满足，请保持车辆停稳后重试"
	case errors.Is(err, ErrWeight):
		return "毛重必须大于皮重"
	case errors.Is(err, ErrSession):
		return "地磅操作会话已失效，请重试"
	case errors.Is(err, ErrCaptureCameraRequired):
		return "强制抓拍已开启，但未指定抓拍摄像头"
	case errors.Is(err, ErrCapturePolicyChanged), errors.Is(err, ErrCameraConfigChanged):
		return "抓拍配置已变化，请重新扫码"
	case errors.Is(err, ErrMeasurementCapability):
		return "设备 Agent 不支持测量有效性检查，请升级 Agent"
	}
	if message := err.Error(); message != "" {
		return message
	}
	return "厂端采集失败"
}
