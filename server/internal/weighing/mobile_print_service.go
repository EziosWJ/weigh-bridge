package weighing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

const (
	mobilePrintClaimPath  = "/api/v1/grain-outbound/factory/mobile-prints/claim"
	mobilePrintScanPath   = "/api/v1/grain-outbound/factory/mobile-prints/scan"
	mobilePrintResultPath = "/api/v1/grain-outbound/factory/mobile-prints/%d/result"
	mobilePrintSyncPath   = "/api/v1/grain-outbound/factory/print-attempts"
)

// RunMobilePrintCommands pulls cloud print commands and pushes dirty factory
// print-attempt snapshots. It is safe to call repeatedly; the cloud claim is
// idempotent and the factory keeps a unique cloud request id per attempt.
func (s *Service) RunMobilePrintCommands(ctx context.Context) {
	if !s.masterSyncEnabled || strings.TrimSpace(s.masterSyncAddress) == "" {
		return
	}
	s.mobilePrintMu.Lock()
	defer s.mobilePrintMu.Unlock()
	siteCode, err := s.LocalSiteCode(ctx)
	if err != nil || strings.TrimSpace(siteCode) == "" {
		return
	}
	commands, err := s.claimMobilePrintCommands(ctx, siteCode)
	if err == nil {
		for _, command := range commands {
			s.executeMobilePrintCommand(ctx, siteCode, command)
		}
	}
	s.syncMobilePrintAttempts(ctx, siteCode)
}

func (s *Service) claimMobilePrintCommands(ctx context.Context, siteCode string) ([]MobilePrintCommand, error) {
	body, _ := json.Marshal(MobilePrintClaimRequest{SiteCode: siteCode})
	envelope, status, err := s.postCloud(ctx, mobilePrintClaimPath, body)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK || envelope.Code != 200 {
		return nil, fmt.Errorf("领取打印命令失败")
	}
	var commands []MobilePrintCommand
	if err := json.Unmarshal(envelope.Data, &commands); err != nil {
		return nil, fmt.Errorf("打印命令格式无效")
	}
	return commands, nil
}

func (s *Service) claimMobilePrintScan(ctx context.Context, siteCode, token string) (*MobilePrintCommand, error) {
	body, _ := json.Marshal(MobilePrintScanRequest{SiteCode: siteCode, Token: token})
	envelope, status, err := s.postCloud(ctx, mobilePrintScanPath, body)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK || envelope.Code != 200 {
		return nil, fmt.Errorf("补打二维码无效或已过期")
	}
	var command MobilePrintCommand
	if err := json.Unmarshal(envelope.Data, &command); err != nil {
		return nil, fmt.Errorf("补打命令格式无效")
	}
	return &command, nil
}

func (s *Service) reportMobilePrintResult(ctx context.Context, command MobilePrintCommand, status, message string, attemptID int64, intent string) error {
	body, _ := json.Marshal(MobilePrintResultRequest{
		SiteCode: command.SiteCode, Status: status, FactoryAttempt: attemptID,
		FactoryIntent: intent, Message: message,
	})
	path := fmt.Sprintf(mobilePrintResultPath, command.ID)
	envelope, httpStatus, err := s.postCloud(ctx, path, body)
	if err != nil {
		return err
	}
	if httpStatus != http.StatusOK || envelope.Code != 200 {
		return fmt.Errorf("打印结果回传失败")
	}
	return nil
}

func (s *Service) postCloud(ctx context.Context, path string, body []byte) (syncEnvelope, int, error) {
	target, err := syncV4URL(s.masterSyncAddress, path, nil)
	if err != nil {
		return syncEnvelope{}, 0, fmt.Errorf("云端地址无效")
	}
	return s.syncV4Client.do(ctx, http.MethodPost, target, body)
}

func (s *Service) executeMobilePrintCommand(ctx context.Context, siteCode string, command MobilePrintCommand) {
	if strings.TrimSpace(command.TicketNo) == "" {
		_ = s.reportMobilePrintResult(ctx, command, "failed", "缺少磅单号", 0, "")
		return
	}
	event := audit.Event{
		Action: "ticket.print.mobile", Resource: "weighing.print",
		Metadata: audit.Metadata{ActorID: 0, ActorName: command.DriverName, RequestID: command.RequestID, RequestMethod: "CLOUD", RequestURL: "/factory/mobile-prints", UserAgent: "cloud"},
	}
	if command.Action == "verify" {
		s.executeMobilePrintVerify(ctx, command, event)
		return
	}
	ticket, err := s.findTicketByNo(ctx, command.TicketNo)
	if err != nil {
		_ = s.reportMobilePrintResult(ctx, command, "failed", "未找到厂端磅单", 0, "")
		return
	}
	if existing, lookupErr := s.mobilePrintAttemptByRequest(ctx, command.RequestID); lookupErr == nil && existing != nil {
		_ = s.reportMobilePrintResult(ctx, command, attemptCommandStatus(existing.Status), "", existing.ID, existing.Intent)
		return
	}
	latest, err := s.mobilePrintLatest(ctx, ticket.ID)
	if err != nil {
		_ = s.reportMobilePrintResult(ctx, command, "failed", "读取打印记录失败", 0, "")
		return
	}
	intent := "reprint"
	if latest == nil {
		intent = "initial"
		if command.RetryAsReprint {
			// The first print failed before the factory created an attempt.
			// The scan retry counts as the first reprint.
			intent = "reprint"
		}
	}
	if command.Origin == "completion" && latest != nil {
		_ = s.reportMobilePrintResult(ctx, command, "failed", "已存在打印记录，请从历史记录扫码补打", 0, "")
		return
	}
	if command.Origin != "completion" && intent == "initial" && strings.TrimSpace(command.Reason) == "" {
		_ = s.reportMobilePrintResult(ctx, command, "failed", "请填写打印原因", 0, "")
		return
	}
	result, err := s.StartPrint(ctx, ticket.ID, PrintInput{
		RequestID: command.RequestID, Intent: intent, Reason: command.Reason,
		AllowReprint: true, Source: "mobile", ActorName: command.DriverName,
		ScanVerified: command.ScanVerified, ScanTokenHash: command.ScanTokenHash,
		CloudRequestID: command.RequestID,
	}, event)
	if err != nil {
		_ = s.reportMobilePrintResult(ctx, command, "failed", err.Error(), 0, "")
		return
	}
	_ = s.reportMobilePrintResult(ctx, command, "printing", "已将打印任务提交到厂端打印机", result.PrintAttemptID, intent)
}

func (s *Service) executeMobilePrintVerify(ctx context.Context, command MobilePrintCommand, event audit.Event) {
	ticket, err := s.findTicketByNo(ctx, command.TicketNo)
	if err != nil {
		_ = s.reportMobilePrintResult(ctx, command, "failed", "未找到厂端磅单", 0, "")
		return
	}
	latest, err := s.mobilePrintLatest(ctx, ticket.ID)
	if err != nil || latest == nil {
		_ = s.reportMobilePrintResult(ctx, command, "failed", "未找到待核实的打印记录", 0, "")
		return
	}
	if latest.ClosedAt != nil {
		_ = s.reportMobilePrintResult(ctx, command, attemptCommandStatus(latest.Status), "打印结果已明确，无需核实", latest.ID, latest.Intent)
		return
	}
	status := "failed"
	reason := "司机现场核实：未出纸"
	if command.VerifyResult == "printed" {
		status = "printed"
		reason = "司机现场核实：已出纸"
	}
	result, err := s.ResolvePrint(ctx, latest.ID, PrintResultInput{
		RequestID: "mpv-" + command.RequestID, Status: status, Reason: reason,
		ResolutionSource: "driver", ActorName: command.DriverName,
	}, event)
	if err != nil {
		_ = s.reportMobilePrintResult(ctx, command, "failed", err.Error(), latest.ID, latest.Intent)
		return
	}
	_ = s.reportMobilePrintResult(ctx, command, status, reason, result.PrintAttemptID, latest.Intent)
}

func (s *Service) syncMobilePrintAttempts(ctx context.Context, siteCode string) {
	rows, err := s.mobilePrintDueSyncs(ctx, 50)
	if err != nil {
		return
	}
	for _, row := range rows {
		row.SiteCode = siteCode
		body, _ := json.Marshal(row)
		envelope, status, requestErr := s.postCloud(ctx, mobilePrintSyncPath, body)
		if requestErr != nil || status != http.StatusOK || envelope.Code != 200 {
			_ = s.markMobilePrintSynced(ctx, row.FactoryAttempt, "同步失败")
			continue
		}
		_ = s.markMobilePrintSynced(ctx, row.FactoryAttempt, "")
	}
}

func attemptCommandStatus(status string) string {
	switch status {
	case "printed", "failed", "unknown":
		return status
	default:
		return "printing"
	}
}

func (s *Service) findTicketByNo(ctx context.Context, ticketNo string) (*Ticket, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return nil, ErrForbidden
	}
	return repo.FindTicketByNo(ctx, ticketNo)
}

func (s *Service) mobilePrintLatest(ctx context.Context, ticketID int64) (*PrintAttempt, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return nil, ErrForbidden
	}
	return repo.LatestPrintAttempt(ctx, ticketID)
}

func (s *Service) mobilePrintAttemptByRequest(ctx context.Context, requestID string) (*PrintAttempt, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return nil, ErrForbidden
	}
	return repo.PrintAttemptByCloudRequest(ctx, requestID)
}

func (s *Service) mobilePrintDueSyncs(ctx context.Context, limit int) ([]PrintAttemptSync, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return nil, ErrForbidden
	}
	return repo.DuePrintSyncs(ctx, limit)
}

func (s *Service) markMobilePrintSynced(ctx context.Context, attemptID int64, failure string) error {
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	return repo.MarkPrintSynced(ctx, attemptID, failure)
}

// ScanMobilePrintFrom is called by the local serial scanner Agent when the
// driver presents a one-time reprint QR. The cloud validates and claims the
// token, then the factory executes the returned command immediately.
func (s *Service) ScanMobilePrintFrom(ctx context.Context, code, source, device string, e audit.Event) (any, error) {
	_ = source
	_ = device
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(code), "fc-print:"))
	if token == "" {
		return nil, ErrInvalid
	}
	if !s.masterSyncEnabled || strings.TrimSpace(s.masterSyncAddress) == "" {
		return nil, RejectionError("云端打印未配置")
	}
	siteCode, err := s.LocalSiteCode(ctx)
	if err != nil || strings.TrimSpace(siteCode) == "" {
		return nil, RejectionError("本厂身份未配置")
	}
	command, err := s.claimMobilePrintScan(ctx, siteCode, token)
	if err != nil {
		return nil, RejectionError(err.Error())
	}
	s.executeMobilePrintCommand(ctx, siteCode, *command)
	return map[string]any{
		"type":     "mobile-print",
		"action":   "mobile-print",
		"ticketNo": command.TicketNo,
		"status":   "submitted",
		"message":  "补打任务已提交到厂端打印机",
	}, nil
}

// ScanMobileCode dispatches the two QR families handled by the serial scanner.
func (s *Service) ScanMobileCode(ctx context.Context, code, source, device string, e audit.Event) (any, error) {
	if strings.HasPrefix(strings.TrimSpace(code), "fc-print:") {
		return s.ScanMobilePrintFrom(ctx, code, source, device, e)
	}
	return s.ScanMobileHandoffFrom(ctx, code, source, device, e)
}
