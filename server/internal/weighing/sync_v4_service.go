package weighing

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/sysconfig"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const syncReconcileDefaultTime = "02:30"

// RunSyncV4 uploads due digest-protocol changes in tick order. A failure never
// blocks other tickets; reopen depends on the original ticket's void.
func (s *Service) RunSyncV4(ctx context.Context) error {
	if !s.masterSyncEnabled {
		return nil
	}
	ready, err := s.syncV4Ready(ctx)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	s.ticketSyncMu.Lock()
	defer s.ticketSyncMu.Unlock()
	changes, err := repo.dueSyncChangesV4(ctx, 100)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		receipt, cloudErr := s.syncV4Client.uploadChange(ctx, s.masterSyncAddress, change.Payload, change.Digest, change.TicketNo, change.ChangeType)
		now := time.Now().UTC()
		dbCtx := context.WithoutCancel(ctx)
		if cloudErr == nil {
			if err := repo.markSyncChangeV4Synced(dbCtx, change.ID, receipt.ReceivedAt, now); err != nil {
				return err
			}
			// A confirmed predecessor makes dependent changes eligible
			// immediately instead of waiting for their previous backoff.
			if err := repo.wakeWaitingDependents(dbCtx, change.TicketID); err != nil {
				return err
			}
			if err := repo.writeSyncLog(dbCtx, "automatic", change.ChangeType, change.TicketNo, change.Digest, "", true, ""); err != nil {
				return err
			}
			continue
		}
		status, next := syncFailureState(cloudErr, change.Attempts+1)
		if err := repo.markSyncChangeV4Failure(dbCtx, change.ID, status, cloudErr.Category, cloudErr.Reason, cloudErr.Message, next, now); err != nil {
			return err
		}
		if err := repo.writeSyncLog(dbCtx, "automatic", change.ChangeType, change.TicketNo, change.Digest, cloudErr.Category, false, cloudErr.Message); err != nil {
			return err
		}
		if cloudErr.Category == SyncCategoryConflict {
			detail, _ := json.Marshal(map[string]any{"digest": change.Digest, "ticketNo": change.TicketNo, "changeType": change.ChangeType, "status": status, "category": cloudErr.Category, "reasonCode": cloudErr.Reason, "message": cloudErr.Message})
			if _, err := repo.upsertSyncReview(dbCtx, change.TicketID, change.Digest, cloudErr.Reason, string(detail), true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) syncV4Ready(ctx context.Context) (bool, error) {
	status, err := s.MasterSyncStatus(ctx)
	if err != nil {
		return false, err
	}
	return status.Bound && status.LastSuccess != nil, nil
}

func syncFailureState(cloudErr *SyncCloudError, attempt int) (string, *time.Time) {
	switch cloudErr.Category {
	case SyncCategoryPredecessorPending:
		next := time.Now().UTC().Add(syncRetryBackoff(attempt))
		return SyncChangeWaitingPredecessor, &next
	case SyncCategoryConfigOrData:
		return SyncChangeNeedsAdmin, nil
	case SyncCategoryConflict:
		return SyncChangeConflict, nil
	default:
		next := time.Now().UTC().Add(syncRetryBackoff(attempt))
		return SyncChangeTemporaryFailure, &next
	}
}

func syncRetryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 7 {
		shift = 7
	}
	delay := 10 * time.Second * time.Duration(1<<shift)
	if delay > 15*time.Minute {
		delay = 15 * time.Minute
	}
	return delay
}

type SyncReconcileResult struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reasonCode"`
	Message    string `json:"message"`
}

type SyncRetryResult struct {
	Queued             bool   `json:"queued"`
	WaitingPredecessor bool   `json:"waitingPredecessor"`
	Message            string `json:"message"`
}

type reconcileDecision struct {
	Status        string
	ReasonCode    string
	Message       string
	Confirmed     map[string]time.Time
	Resend        bool
	Blocking      bool
	PauseChangeID int64
	Detail        map[string]any
}

func decideSyncReconcile(changes []ticketSyncChangeV4, cloud syncCloudTicket, trigger string) reconcileDecision {
	decision := reconcileDecision{Confirmed: map[string]time.Time{}, Detail: map[string]any{"trigger": trigger}}
	if len(changes) == 0 {
		decision.Status = SyncReconcileNeedsReview
		decision.ReasonCode = "missing_local_change"
		decision.Message = "厂端缺少该单据的变化记录，需人工核查"
		return decision
	}
	localLatest := changes[len(changes)-1]
	decision.Detail["localLatestDigest"] = localLatest.Digest
	decision.Detail["localChanges"] = changeSummaries(changes)
	if cloud.Error != nil {
		decision.Status = SyncReconcileUnknown
		decision.ReasonCode = cloud.Error.Reason
		decision.Message = cloud.Error.Message
		decision.Detail["error"] = cloud.Error
		return decision
	}
	if !cloud.Found {
		hasCreation := false
		for _, change := range changes {
			if change.Status == SyncChangeConflict {
				decision.Status = SyncReconcileConflict
				decision.ReasonCode = "existing_conflict"
				decision.Message = "该单据存在未处理的冲突，需管理员核查"
				decision.Blocking = true
				return decision
			}
			if change.ChangeType == "issue" || change.ChangeType == "reopen" {
				hasCreation = true
			}
		}
		decision.Detail["cloudFound"] = false
		if hasCreation {
			decision.Status = SyncReconcilePending
			decision.ReasonCode = "ticket_missing"
			decision.Message = "云端缺单，已按原变化顺序补传"
			decision.Resend = true
			return decision
		}
		decision.Status = SyncReconcileNeedsReview
		decision.ReasonCode = "missing_original_change"
		decision.Message = "云端缺单且厂端缺少原出单变化，需人工处理"
		return decision
	}
	decision.Detail["cloudFound"] = true
	decision.Detail["cloudStatus"] = cloud.Status
	decision.Detail["cloudLatestDigest"] = cloud.LatestDigest
	decision.Detail["cloudLatestDigestVersion"] = cloud.LatestDigestVersion
	decision.Detail["cloudLatestChangeType"] = cloud.LatestChangeType
	decision.Detail["cloudReceivedAt"] = cloud.LatestReceivedAt
	decision.Detail["cloudChanges"] = cloud.Changes
	if cloud.LatestDigest == nil {
		decision.Status = SyncReconcileNeedsReview
		decision.ReasonCode = "cloud_without_change"
		decision.Message = "云端存在单据但缺少变化摘要，需人工核查"
		return decision
	}
	if cloud.LatestDigestVersion == nil || (*cloud.LatestDigestVersion != SyncDigestVersion2 && *cloud.LatestDigestVersion != SyncDigestVersion3) {
		decision.Status = SyncReconcileNeedsReview
		decision.ReasonCode = "digest_version_uncomparable"
		decision.Message = "摘要规则版本不同或缺失，无法直接比较"
		return decision
	}
	if cloud.Ticket != nil {
		differences, err := syncFieldDifferences(localLatest, cloud)
		if err != nil {
			decision.Status = SyncReconcileUnknown
			decision.ReasonCode = "invalid_snapshot"
			decision.Message = "单据快照无法比较，结果未知"
			return decision
		}
		decision.Detail["fieldDifferences"] = differences
		if cloud.LatestDigest != nil && *cloud.LatestDigest == localLatest.Digest && len(differences) > 0 {
			decision.Status = SyncReconcileConflict
			decision.ReasonCode = "snapshot_mismatch"
			decision.Message = "摘要相同但单据字段不同，需人工核查"
			decision.Blocking = true
			decision.PauseChangeID = localLatest.ID
			return decision
		}
	}
	known := map[string]ticketSyncChangeV4{}
	for _, change := range changes {
		known[change.Digest] = change
	}
	for _, confirmation := range cloud.Changes {
		change, exists := known[confirmation.Digest]
		if !exists || confirmation.TicketNo != change.TicketNo || confirmation.ChangeType != change.ChangeType || confirmation.DigestVersion != change.DigestVersion || confirmation.SchemaVersion != change.SchemaVersion {
			decision.Status = SyncReconcileConflict
			decision.ReasonCode = "cloud_unknown_change"
			decision.Message = "云端存在厂端未知的变化摘要，需人工核查"
			decision.Blocking = true
			if localLatest.Status != SyncChangeSynced {
				decision.PauseChangeID = localLatest.ID
			}
			return decision
		}
	}
	for _, confirmation := range cloud.Changes {
		decision.Confirmed[confirmation.Digest] = confirmation.ReceivedAt
	}
	if latest, ok := known[*cloud.LatestDigest]; ok {
		if cloud.LatestChangeType == nil || *cloud.LatestChangeType != latest.ChangeType || cloud.LatestReceivedAt == nil || cloud.LatestReceivedAt.IsZero() {
			decision.Status = SyncReconcileUnknown
			decision.ReasonCode = "invalid_confirmation"
			decision.Message = "云端最新变化确认无效，结果未知"
			decision.Confirmed = map[string]time.Time{}
			return decision
		}
		decision.Confirmed[*cloud.LatestDigest] = *cloud.LatestReceivedAt
	}
	if cloud.LatestChangeType != nil {
		expected := "valid"
		if *cloud.LatestChangeType == "void" {
			expected = "void"
		}
		if cloud.Status != "" && cloud.Status != expected {
			decision.Status = SyncReconcileConflict
			decision.ReasonCode = "cloud_status_conflict"
			decision.Message = "云端单据状态与最新变化摘要不一致"
			decision.Blocking = true
			decision.PauseChangeID = localLatest.ID
			return decision
		}
	}
	index := -1
	for i, change := range changes {
		if change.Digest == *cloud.LatestDigest {
			index = i
			break
		}
	}
	if index == len(changes)-1 {
		decision.Status = SyncReconcileConsistent
		decision.Message = "厂端与云端最新变化摘要一致"
		return decision
	}
	if index >= 0 {
		later := changes[index+1:]
		allConfirmed := true
		hasConflict := false
		for _, change := range later {
			if change.Status != SyncChangeSynced {
				allConfirmed = false
			}
			if change.Status == SyncChangeConflict {
				hasConflict = true
			}
		}
		if allConfirmed {
			decision.Status = SyncReconcileConflict
			decision.ReasonCode = "cloud_behind_confirmed"
			decision.Message = "云端停留在较早变化，但厂端后续变化已确认"
			decision.Blocking = true
			decision.PauseChangeID = localLatest.ID
			return decision
		}
		decision.Status = SyncReconcilePending
		decision.ReasonCode = "waiting_upload"
		decision.Message = "云端停留在前序变化，等待后续变化上传"
		if hasConflict {
			decision.ReasonCode = "waiting_conflict"
			decision.Message = "后续变化处于冲突暂停状态"
		}
		return decision
	}
	decision.Status = SyncReconcileConflict
	decision.ReasonCode = "cloud_unknown_latest"
	decision.Message = "云端最新摘要不是厂端已知变化，需人工核查"
	decision.Blocking = true
	if localLatest.Status != SyncChangeSynced {
		decision.PauseChangeID = localLatest.ID
	}
	return decision
}

func changeSummaries(changes []ticketSyncChangeV4) []map[string]any {
	result := make([]map[string]any, 0, len(changes))
	for _, change := range changes {
		result = append(result, map[string]any{"id": change.ID, "changeType": change.ChangeType, "digest": change.Digest, "status": change.Status, "category": change.Category, "reasonCode": change.ReasonCode, "createdAt": change.CreatedAt})
	}
	return result
}

func (s *Service) ReconcileSyncTicket(ctx context.Context, ticketID int64, trigger string) (SyncReconcileResult, error) {
	s.syncReconcileMu.Lock()
	defer s.syncReconcileMu.Unlock()
	s.ticketSyncMu.Lock()
	defer s.ticketSyncMu.Unlock()
	return s.reconcileSyncTicket(ctx, ticketID, trigger, false)
}

func (s *Service) reconcileSyncTicket(ctx context.Context, ticketID int64, trigger string, recheck bool) (SyncReconcileResult, error) {
	if !s.masterSyncEnabled {
		return SyncReconcileResult{}, RejectionError("同步开关未开启")
	}
	ready, err := s.syncV4Ready(ctx)
	if err != nil {
		return SyncReconcileResult{}, err
	}
	if !ready {
		return SyncReconcileResult{}, RejectionError("请先完成首次总部资料同步")
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return SyncReconcileResult{}, ErrForbidden
	}
	changes, changesV5, err := repo.ticketSyncChanges(ctx, ticketID)
	if err != nil {
		return SyncReconcileResult{}, err
	}
	if len(changes) == 0 {
		return SyncReconcileResult{}, errSyncNotTracked
	}
	ref, err := repo.syncTicketRef(ctx, ticketID)
	if err != nil {
		return SyncReconcileResult{}, err
	}
	cloud, cloudErr := s.syncV4Client.fetchTicket(ctx, s.masterSyncAddress, ref.SiteCode, ref.TicketNo)
	if cloudErr != nil {
		cloud = syncCloudTicket{SiteCode: ref.SiteCode, TicketNo: ref.TicketNo, Error: cloudErr}
	}
	// The business transaction can add a later void while the cloud request is
	// in flight; judge the result against the latest local changes.
	changes, changesV5, err = repo.ticketSyncChanges(ctx, ticketID)
	if err != nil || len(changes) == 0 {
		if err == nil {
			err = errSyncNotTracked
		}
		return SyncReconcileResult{}, err
	}
	decision, err := s.reviewDecision(ctx, repo, ticketID, changes, &cloud, ref, trigger, recheck)
	if err != nil {
		return SyncReconcileResult{}, err
	}
	result := SyncReconcileResult{Status: decision.Status, ReasonCode: decision.ReasonCode, Message: decision.Message}
	now := time.Now().UTC()
	detail, _ := json.Marshal(decision.Detail)
	applyDecision := decision
	if recheck && decision.Resend {
		applyDecision.Resend = false
	}
	if err := s.applyReconcileDecision(context.WithoutCancel(ctx), repo, ticketID, changes, changesV5, applyDecision, ref, cloud, detail, now, trigger); err != nil {
		return SyncReconcileResult{}, err
	}
	if err := repo.writeSyncLog(context.WithoutCancel(ctx), trigger, "reconcile", ref.TicketNo, localLatestDigest(changes), decisionCategory(decision), decision.Status != SyncReconcileUnknown, decision.Message); err != nil {
		return SyncReconcileResult{}, err
	}
	return result, nil
}

func (s *Service) reviewDecision(ctx context.Context, repo *Repository, ticketID int64, changes []ticketSyncChangeV4, cloud *syncCloudTicket, ref syncTicketRef, trigger string, recheck bool) (reconcileDecision, error) {
	blocking, err := repo.ticketHasBlockingReview(ctx, ticketID)
	if err != nil {
		return reconcileDecision{}, err
	}
	if blocking && !recheck && cloud.Error == nil {
		detail := map[string]any{"trigger": trigger, "cloudFound": cloud.Found, "cloudChanges": cloud.Changes}
		if cloud.Found && cloud.Ticket != nil {
			differences, err := syncFieldDifferences(changes[len(changes)-1], *cloud)
			if err != nil {
				return reconcileDecision{}, err
			}
			detail["fieldDifferences"] = differences
		}
		return reconcileDecision{Status: SyncReconcileConflict, ReasonCode: "existing_conflict", Message: "该单据存在未处理的冲突，需管理员记录结论并重新核查", Confirmed: map[string]time.Time{}, Detail: detail}, nil
	}
	// A reported missing ticket can only be replayed after checking that the
	// cloud does not still hold the original change confirmation.
	if cloud.Error == nil && !cloud.Found {
		for _, change := range changes {
			if change.ChangeType != "issue" && change.ChangeType != "reopen" {
				continue
			}
			lookup, cloudErr := s.syncV4Client.lookupChange(ctx, s.masterSyncAddress, ref.SiteCode, change.Digest)
			if cloudErr != nil {
				cloud.Error = cloudErr
			} else if lookup.Found {
				return reconcileDecision{Status: SyncReconcileNeedsReview, ReasonCode: "cloud_confirmation_without_ticket", Message: "云端有变化确认但没有对应单据，需人工核查", Confirmed: map[string]time.Time{}, Blocking: true, Detail: map[string]any{"trigger": trigger, "digest": change.Digest}}, nil
			}
			break
		}
	}
	decisionChanges := changes
	if recheck {
		decisionChanges = append([]ticketSyncChangeV4(nil), changes...)
		for i := range decisionChanges {
			if decisionChanges[i].Status == SyncChangeConflict {
				decisionChanges[i].Status = SyncChangePending
			}
		}
	}
	return decideSyncReconcile(decisionChanges, *cloud, trigger), nil
}

func localLatestDigest(changes []ticketSyncChangeV4) string {
	if len(changes) == 0 {
		return ""
	}
	return changes[len(changes)-1].Digest
}

func decisionCategory(decision reconcileDecision) string {
	switch decision.Status {
	case SyncReconcileConflict:
		return SyncCategoryConflict
	case SyncReconcileNeedsReview, SyncReconcileUnknown:
		return SyncCategoryConfigOrData
	case SyncReconcilePending:
		return SyncCategoryPredecessorPending
	default:
		return ""
	}
}

func (s *Service) applyReconcileDecision(ctx context.Context, repo *Repository, ticketID int64, changes []ticketSyncChangeV4, changesV5 bool, decision reconcileDecision, ref syncTicketRef, cloud syncCloudTicket, detail []byte, now time.Time, trigger string) error {
	localLatest := changes[len(changes)-1]
	if decision.Resend {
		if err := repo.resetTicketSyncChangesForResend(ctx, ticketID, changesV5); err != nil {
			return err
		}
	} else {
		for digest, receivedAt := range decision.Confirmed {
			for _, change := range changes {
				if change.Digest == digest && change.Status != SyncChangeSynced && change.Status != SyncChangeConflict {
					if err := repo.markTicketSyncChangeSynced(ctx, change.ID, changesV5, receivedAt, now); err != nil {
						return err
					}
				}
			}
		}
		if decision.PauseChangeID > 0 {
			if err := repo.pauseTicketSyncChange(ctx, decision.PauseChangeID, changesV5, SyncCategoryConflict, decision.ReasonCode, decision.Message); err != nil {
				return err
			}
		}
		if decision.Blocking || decision.Status == SyncReconcileNeedsReview {
			digest := localLatest.Digest
			if cloud.LatestDigest != nil {
				digest = *cloud.LatestDigest
			}
			if _, err := repo.upsertSyncReview(ctx, ticketID, digest, decision.ReasonCode, string(detail), decision.Blocking); err != nil {
				return err
			}
		}
	}
	var cloudFound *bool
	if cloud.Error == nil {
		value := cloud.Found
		cloudFound = &value
	}
	var cloudStatus *string
	if cloud.Status != "" {
		value := cloud.Status
		cloudStatus = &value
	}
	localDigest := localLatest.Digest
	row := ticketReconcile{
		TicketID:                 ticketID,
		SiteCode:                 ref.SiteCode,
		TicketNo:                 ref.TicketNo,
		Status:                   decision.Status,
		CloudFound:               cloudFound,
		CloudStatus:              cloudStatus,
		LocalLatestDigest:        &localDigest,
		CloudLatestDigest:        cloud.LatestDigest,
		CloudLatestDigestVersion: cloud.LatestDigestVersion,
		CloudLatestChangeType:    cloud.LatestChangeType,
		CloudReceivedAt:          cloud.LatestReceivedAt,
		Detail:                   string(detail),
		CheckedAt:                now,
		TriggerType:              trigger,
	}
	return repo.upsertTicketReconcile(ctx, row)
}

// ReconcileSyncRange checks v4-tracked tickets and stores one result per
// ticket. Individual item errors never turn into a consistent result.
func (s *Service) ReconcileSyncRange(ctx context.Context, from, to string, trigger string) (SyncReconcileSummary, error) {
	s.syncReconcileMu.Lock()
	defer s.syncReconcileMu.Unlock()
	if !s.masterSyncEnabled {
		return SyncReconcileSummary{}, RejectionError("同步开关未开启")
	}
	ready, err := s.syncV4Ready(ctx)
	if err != nil {
		return SyncReconcileSummary{}, err
	}
	if !ready {
		return SyncReconcileSummary{}, RejectionError("请先完成首次总部资料同步")
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return SyncReconcileSummary{}, ErrForbidden
	}
	pageQuery := PageQuery{From: from, To: to}
	fromTime, toTime, err := dateBounds(pageQuery)
	if err != nil {
		return SyncReconcileSummary{}, err
	}
	ids, err := repo.ticketIDsForReconcile(ctx, fromTime, toTime, trigger == "automatic" || (from == "" && to == ""))
	if err != nil {
		return SyncReconcileSummary{}, err
	}
	summary := SyncReconcileSummary{}
	runFailed := false
	for start := 0; start < len(ids); start += MaxBatchReconcile {
		end := start + MaxBatchReconcile
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		refs, err := repo.syncTicketRefsForBatch(ctx, batch)
		if err != nil {
			return summary, err
		}
		items, cloudErr := s.syncV4Client.fetchBatch(ctx, s.masterSyncAddress, refs)
		if cloudErr != nil {
			runFailed = true
			// Record unknown for every unchecked ticket, rather than leaving a
			// previous successful result looking like the latest check.
			for _, ticketID := range ids[start:] {
				ref, err := repo.syncTicketRef(ctx, ticketID)
				if err != nil {
					return summary, err
				}
				item := syncCloudTicket{SiteCode: ref.SiteCode, TicketNo: ref.TicketNo, Error: cloudErr}
				if _, err := s.reconcileBatchItem(ctx, repo, ticketID, ref, item, trigger); err != nil {
					return summary, err
				}
				summary.Unknown++
				summary.Checked++
			}
			if err := repo.writeSyncLog(context.WithoutCancel(ctx), trigger, "reconcile", "", "", cloudErr.Category, false, cloudErr.Message); err != nil {
				return summary, err
			}
			break
		}
		for index, item := range items {
			ticketID := batch[index]
			ref := refs[index]
			decision, err := s.reconcileBatchItem(ctx, repo, ticketID, ref, item, trigger)
			if err != nil {
				return summary, err
			}
			summary.Checked++
			switch decision.Status {
			case SyncReconcileConsistent:
				summary.Consistent++
			case SyncReconcilePending:
				summary.Pending++
			case SyncReconcileNeedsReview:
				summary.NeedsReview++
			case SyncReconcileConflict:
				summary.Conflict++
			default:
				summary.Unknown++
			}
		}
	}
	now := time.Now().UTC()
	summary.CheckedAt = &now
	switch {
	case runFailed:
		summary.Status = SyncReconcileUnknown
		summary.Reason = "云端对账请求失败，结果未知"
	case summary.Conflict > 0:
		summary.Status = SyncReconcileConflict
	case summary.Unknown > 0:
		summary.Status = SyncReconcileUnknown
	case summary.NeedsReview > 0:
		summary.Status = SyncReconcileNeedsReview
	case summary.Pending > 0:
		summary.Status = SyncReconcilePending
	default:
		summary.Status = SyncReconcileConsistent
	}
	if err := s.writeReconcileSummaryState(ctx, repo, summary, trigger, !runFailed); err != nil {
		return summary, err
	}
	if err := repo.writeSyncLog(context.WithoutCancel(ctx), trigger, "reconcile", "", "", "", !runFailed, summary.Reason); err != nil {
		return summary, err
	}
	return summary, nil
}

func (s *Service) reconcileBatchItem(ctx context.Context, repo *Repository, ticketID int64, ref syncTicketRef, cloud syncCloudTicket, trigger string) (reconcileDecision, error) {
	s.ticketSyncMu.Lock()
	defer s.ticketSyncMu.Unlock()
	changes, changesV5, err := repo.ticketSyncChanges(ctx, ticketID)
	if err != nil {
		return reconcileDecision{}, err
	}
	if len(changes) == 0 {
		return reconcileDecision{}, errSyncNotTracked
	}
	if cloud.Error == nil && decideSyncReconcile(changes, cloud, trigger).Status != SyncReconcileConsistent {
		// A batch snapshot can precede a successful upload. Re-read the one
		// ticket before classifying a difference or scheduling a replay.
		var cloudErr *SyncCloudError
		cloud, cloudErr = s.syncV4Client.fetchTicket(ctx, s.masterSyncAddress, ref.SiteCode, ref.TicketNo)
		if cloudErr != nil {
			cloud = syncCloudTicket{SiteCode: ref.SiteCode, TicketNo: ref.TicketNo, Error: cloudErr}
		}
		changes, changesV5, err = repo.ticketSyncChanges(ctx, ticketID)
		if err != nil {
			return reconcileDecision{}, err
		}
	}
	decision, err := s.reviewDecision(ctx, repo, ticketID, changes, &cloud, ref, trigger, false)
	if err != nil {
		return reconcileDecision{}, err
	}
	detail, err := json.Marshal(decision.Detail)
	if err != nil {
		return reconcileDecision{}, err
	}
	if err := s.applyReconcileDecision(context.WithoutCancel(ctx), repo, ticketID, changes, changesV5, decision, ref, cloud, detail, time.Now().UTC(), trigger); err != nil {
		return reconcileDecision{}, err
	}
	return decision, nil
}

func (s *Service) writeReconcileSummaryState(ctx context.Context, repo *Repository, summary SyncReconcileSummary, trigger string, success bool) error {
	state, err := repo.readSyncReconcileState(ctx)
	if err != nil {
		return err
	}
	state.LastRunAt = summary.CheckedAt
	state.LastStatus = summary.Status
	state.LastChecked = summary.Checked
	state.LastConsistent = summary.Consistent
	state.LastPending = summary.Pending
	state.LastNeedsReview = summary.NeedsReview
	state.LastConflict = summary.Conflict
	state.LastUnknown = summary.Unknown
	state.LastReason = summary.Reason
	if trigger == "automatic" {
		today := time.Now().In(beijing)
		date := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
		state.LastAutoDate = &date
		state.LastAutoSuccess = success
	}
	return repo.writeSyncReconcileState(ctx, state)
}

func (s *Service) RecheckSyncReview(ctx context.Context, reviewID int64, e audit.Event) (SyncReconcileResult, error) {
	s.syncReconcileMu.Lock()
	defer s.syncReconcileMu.Unlock()
	s.ticketSyncMu.Lock()
	defer s.ticketSyncMu.Unlock()
	repo, ok := s.store.(*Repository)
	if !ok {
		return SyncReconcileResult{}, ErrForbidden
	}
	review, err := repo.syncReviewByID(ctx, reviewID)
	if err != nil {
		return SyncReconcileResult{}, err
	}
	if review.Status != SyncReviewOpen {
		return SyncReconcileResult{}, RejectionError("该核查记录已结束")
	}
	if strings.TrimSpace(review.Conclusion) == "" {
		return SyncReconcileResult{}, RejectionError("请先记录核查结论")
	}
	result, err := s.reconcileSyncTicket(ctx, review.TicketID, "manual", true)
	if err != nil {
		return SyncReconcileResult{}, err
	}
	if result.Status == SyncReconcileConsistent || result.Status == SyncReconcilePending {
		if err := repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var current ticketSyncReview
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, review.ID).Error; err != nil {
				return err
			}
			if current.Status != SyncReviewOpen || strings.TrimSpace(current.Conclusion) == "" {
				return RejectionError("核查记录状态已改变，请重新确认")
			}
			if err := tx.Model(&ticketSyncReview{}).Where("id = ?", review.ID).Updates(map[string]any{"status": SyncReviewResolved, "blocking": false, "last_checked_at": time.Now().UTC(), "updated_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
			var remaining int64
			if err := tx.Model(&ticketSyncReview{}).Where("ticket_id = ? AND status = ? AND blocking", review.TicketID, SyncReviewOpen).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining == 0 {
				if result.ReasonCode == "ticket_missing" {
					if err := (&Repository{db: tx}).resetTicketChangesForResend(ctx, review.TicketID); err != nil {
						return err
					}
				} else if err := (&Repository{db: tx}).resetConflictChangesForTicket(ctx, review.TicketID); err != nil {
					return err
				}
			}
			e.Action = "ticket-sync.recheck"
			e.Resource = "weighing.ticket-sync"
			e.ResourceID = review.TicketID
			e.Summary = result.Message
			return audit.RecordOn(ctx, tx, e)
		}); err != nil {
			return SyncReconcileResult{}, err
		}
	}
	return result, nil
}

func (s *Service) RecordSyncReviewConclusion(ctx context.Context, reviewID int64, conclusion string, e audit.Event) error {
	conclusion = strings.TrimSpace(conclusion)
	if conclusion == "" || len([]rune(conclusion)) > 2000 {
		return ErrInvalid
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	review, err := repo.syncReviewByID(ctx, reviewID)
	if err != nil {
		return err
	}
	if review.Status != SyncReviewOpen {
		return RejectionError("该核查记录已结束")
	}
	actorName := ""
	if err := repo.db.WithContext(ctx).Table("sys_user").Select("nickname").Where("id = ?", e.Metadata.ActorID).Scan(&actorName).Error; err != nil {
		return err
	}
	return repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ticketSyncReview{}).Where("id = ? AND status = 'open'", reviewID).Updates(map[string]any{
			"conclusion":        conclusion,
			"concluded_by":      e.Metadata.ActorID,
			"concluded_by_name": actorName,
			"concluded_at":      time.Now().UTC(),
			"updated_at":        time.Now().UTC(),
			"last_checked_at":   time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		e.Action = "ticket-sync.conclusion"
		e.Resource = "weighing.ticket-sync"
		e.ResourceID = review.TicketID
		e.Summary = conclusion
		return audit.RecordOn(ctx, tx, e)
	})
}

func (s *Service) RetrySyncChange(ctx context.Context, changeID int64, e audit.Event) (SyncRetryResult, error) {
	s.ticketSyncMu.Lock()
	defer s.ticketSyncMu.Unlock()
	if !s.masterSyncEnabled {
		return SyncRetryResult{}, RejectionError("同步开关未开启")
	}
	ready, err := s.syncV4Ready(ctx)
	if err != nil {
		return SyncRetryResult{}, err
	}
	if !ready {
		return SyncRetryResult{}, RejectionError("请先完成首次总部资料同步")
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return SyncRetryResult{}, ErrForbidden
	}
	changeV4, err := repo.syncChangeV4ByID(ctx, changeID)
	v5Change, v5Err := repo.syncChangeV5ByID(ctx, changeID)
	if err != nil && v5Err != nil {
		return SyncRetryResult{}, err
	}
	change, changesV5 := changeV4, false
	if v5Err == nil {
		change = ticketSyncChangeV4{
			ID: v5Change.ID, TicketID: v5Change.TicketID, PreviousTicketID: v5Change.PreviousTicketID,
			Digest: v5Change.Digest, SiteCode: v5Change.SiteCode, TicketNo: v5Change.TicketNo,
			ChangeType: v5Change.ChangeType, Status: v5Change.Status,
		}
		changesV5 = true
	}
	if change.Status == SyncChangeSynced {
		return SyncRetryResult{}, RejectionError("该变化已确认，无需重试")
	}
	if change.Status == SyncChangeConflict {
		return SyncRetryResult{}, RejectionError("该变化因冲突暂停，请先重新核查")
	}
	blocking, err := repo.ticketHasBlockingReview(ctx, change.TicketID)
	if err != nil {
		return SyncRetryResult{}, err
	}
	if blocking {
		return SyncRetryResult{}, RejectionError("该单据存在未解除的冲突，请先重新核查")
	}
	waiting, err := repo.changeWaitsForPredecessorAny(ctx, change, changesV5)
	if err != nil {
		return SyncRetryResult{}, err
	}
	now := time.Now().UTC()
	if err := repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		retryTable := &ticketSyncChangeV4{}
		if changesV5 {
			retryTable = nil
			if err := tx.Model(&ticketSyncChangeV5{}).Where("id = ? AND status <> 'synced'", change.ID).Updates(map[string]any{
				"status":          SyncChangePending,
				"category":        "",
				"reason_code":     "",
				"reason":          "",
				"next_attempt_at": now,
			}).Error; err != nil {
				return err
			}
		}
		if retryTable != nil {
			if err := tx.Model(retryTable).Where("id = ? AND status <> 'synced'", change.ID).Updates(map[string]any{
				"status":          SyncChangePending,
				"category":        "",
				"reason_code":     "",
				"reason":          "",
				"next_attempt_at": now,
			}).Error; err != nil {
				return err
			}
		}
		e.Action = "ticket-sync.retry"
		e.Resource = "weighing.ticket-sync"
		e.ResourceID = change.ID
		return audit.RecordOn(ctx, tx, e)
	}); err != nil {
		return SyncRetryResult{}, err
	}
	message := "已排入立即重试"
	if waiting {
		message = "已排入重试，但需等待前序变化确认"
	}
	return SyncRetryResult{Queued: true, WaitingPredecessor: waiting, Message: message}, nil
}

func (s *Service) LookupSyncConfirmation(ctx context.Context, siteCode, digest string) (syncCloudLookup, error) {
	if !s.masterSyncEnabled {
		return syncCloudLookup{}, RejectionError("同步开关未开启")
	}
	if !siteCodePattern.MatchString(siteCode) || len(digest) != 64 {
		return syncCloudLookup{}, ErrInvalid
	}
	result, cloudErr := s.syncV4Client.lookupChange(ctx, s.masterSyncAddress, siteCode, digest)
	if cloudErr != nil {
		return syncCloudLookup{}, RejectionError(cloudErr.Message)
	}
	return result, nil
}

func (s *Service) SyncOverview(ctx context.Context) (SyncOverview, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return SyncOverview{}, ErrForbidden
	}
	result, err := repo.SyncOverview(ctx)
	if err != nil {
		return result, err
	}
	status, err := s.MasterSyncStatus(ctx)
	if err != nil {
		return result, err
	}
	result.Enabled = status.Enabled
	result.Bound = status.Bound
	return result, nil
}

func (s *Service) SyncTickets(ctx context.Context, q SyncTicketQuery) (Page[TicketView], error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return Page[TicketView]{}, ErrForbidden
	}
	return repo.SyncTickets(ctx, q)
}

func (s *Service) SyncTicketDetail(ctx context.Context, ticketID int64) (SyncTicketDetail, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return SyncTicketDetail{}, ErrForbidden
	}
	return repo.SyncTicketDetail(ctx, ticketID)
}

// SyncReconcileClock reads the configured Beijing-time daily reconciliation
// time. Invalid or missing values fall back to 02:30.
func (s *Service) SyncReconcileClock(ctx context.Context, config *sysconfig.Service) (int, int) {
	if config == nil {
		return 2, 30
	}
	value, err := config.GetByKey(ctx, sysconfig.SyncReconcileTimeKey)
	if err != nil || value == nil {
		return 2, 30
	}
	parts := strings.Split(strings.TrimSpace(value.ConfigValue), ":")
	if len(parts) != 2 {
		return 2, 30
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 2, 30
	}
	return hour, minute
}

func (s *Service) NextSyncReconcileDelay(ctx context.Context, config *sysconfig.Service) time.Duration {
	hour, minute := s.SyncReconcileClock(ctx, config)
	now := time.Now().In(beijing)
	scheduled := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, beijing)
	delay := scheduled.Sub(now)
	repo, ok := s.store.(*Repository)
	if ok {
		if state, err := repo.readSyncReconcileState(ctx); err == nil && state.LastAutoDate != nil {
			if state.LastAutoDate.Format("2006-01-02") == now.Format("2006-01-02") {
				if state.LastAutoSuccess {
					if delay <= 0 {
						delay = scheduled.AddDate(0, 0, 1).Sub(now)
					}
				} else {
					delay = 10 * time.Minute
				}
			}
		}
	}
	if delay < time.Minute {
		delay = time.Minute
	}
	return delay
}

// RunScheduledSyncReconcile performs the daily check and catches up after a
// missed run at startup.
func (s *Service) RunScheduledSyncReconcile(ctx context.Context, config *sysconfig.Service) error {
	if !s.masterSyncEnabled {
		return nil
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	state, err := repo.readSyncReconcileState(ctx)
	if err != nil {
		return err
	}
	now := time.Now().In(beijing)
	if state.LastAutoDate != nil && state.LastAutoDate.Format("2006-01-02") == now.Format("2006-01-02") && state.LastAutoSuccess {
		return nil
	}
	_, err = s.ReconcileSyncRange(ctx, "", "", "automatic")
	return err
}
