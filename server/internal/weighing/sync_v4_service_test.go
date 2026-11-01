package weighing

import (
	"testing"
	"time"
)

func v4Change(id int64, changeType, digest, status string) ticketSyncChangeV4 {
	return ticketSyncChangeV4{ID: id, ChangeType: changeType, Digest: digest, Status: status}
}

func cloudResult(digest string, version int, status, changeType string) syncCloudTicket {
	now := time.Now().UTC()
	return syncCloudTicket{Found: true, Status: status, LatestDigest: &digest, LatestDigestVersion: &version, LatestChangeType: &changeType, LatestReceivedAt: &now}
}

func TestDecideReconcileMissingTicketReplaysOriginalChanges(t *testing.T) {
	changes := []ticketSyncChangeV4{v4Change(1, "issue", "a", SyncChangeSynced)}
	decision := decideSyncReconcile(changes, syncCloudTicket{Found: false}, "manual")
	if decision.Status != SyncReconcilePending || !decision.Resend || decision.ReasonCode != "ticket_missing" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestDecideReconcileMissingTicketWithoutIssueNeedsReview(t *testing.T) {
	changes := []ticketSyncChangeV4{v4Change(1, "void", "b", SyncChangePending)}
	decision := decideSyncReconcile(changes, syncCloudTicket{Found: false}, "manual")
	if decision.Status != SyncReconcileNeedsReview || decision.Resend || decision.ReasonCode != "missing_original_change" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestDecideReconcileSameLatestDigestIsConsistent(t *testing.T) {
	changes := []ticketSyncChangeV4{v4Change(1, "issue", "a", SyncChangePending)}
	decision := decideSyncReconcile(changes, cloudResult("a", SyncDigestVersion2, "valid", "issue"), "manual")
	if decision.Status != SyncReconcileConsistent {
		t.Fatalf("decision=%+v", decision)
	}
	if _, ok := decision.Confirmed["a"]; !ok {
		t.Fatal("cloud confirmation must be applied")
	}
}

func TestDecideReconcileCloudBehindWithUnconfirmedLaterIsPending(t *testing.T) {
	changes := []ticketSyncChangeV4{
		v4Change(1, "issue", "a", SyncChangeSynced),
		v4Change(2, "void", "b", SyncChangeTemporaryFailure),
	}
	decision := decideSyncReconcile(changes, cloudResult("a", SyncDigestVersion2, "valid", "issue"), "automatic")
	if decision.Status != SyncReconcilePending || decision.ReasonCode != "waiting_upload" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestDecideReconcileCloudBehindConfirmedChangesIsConflict(t *testing.T) {
	changes := []ticketSyncChangeV4{
		v4Change(1, "issue", "a", SyncChangeSynced),
		v4Change(2, "void", "b", SyncChangeSynced),
	}
	decision := decideSyncReconcile(changes, cloudResult("a", SyncDigestVersion2, "valid", "issue"), "manual")
	if decision.Status != SyncReconcileConflict || !decision.Blocking || decision.ReasonCode != "cloud_behind_confirmed" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestDecideReconcileUnknownDigestPausesTicket(t *testing.T) {
	changes := []ticketSyncChangeV4{v4Change(1, "issue", "a", SyncChangePending)}
	decision := decideSyncReconcile(changes, cloudResult("zzz", SyncDigestVersion2, "valid", "issue"), "manual")
	if decision.Status != SyncReconcileConflict || !decision.Blocking || decision.PauseChangeID != 1 {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestDecideReconcileDigestVersionMismatchNeedsReview(t *testing.T) {
	changes := []ticketSyncChangeV4{v4Change(1, "issue", "a", SyncChangeSynced)}
	decision := decideSyncReconcile(changes, cloudResult("a", 1, "valid", "issue"), "manual")
	if decision.Status != SyncReconcileNeedsReview || decision.ReasonCode != "digest_version_uncomparable" {
		t.Fatalf("decision=%+v", decision)
	}
	if len(decision.Confirmed) != 0 {
		t.Fatalf("uncomparable digest version must not confirm changes: %+v", decision.Confirmed)
	}
}

func TestDecideReconcileQueryFailureIsUnknown(t *testing.T) {
	changes := []ticketSyncChangeV4{v4Change(1, "issue", "a", SyncChangeSynced)}
	decision := decideSyncReconcile(changes, syncCloudTicket{Error: &SyncCloudError{Category: SyncCategoryTemporary, Reason: "transport", Message: "timeout"}}, "manual")
	if decision.Status != SyncReconcileUnknown {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestDecideReconcileCloudStatusConflict(t *testing.T) {
	changes := []ticketSyncChangeV4{v4Change(1, "void", "b", SyncChangeSynced)}
	decision := decideSyncReconcile(changes, cloudResult("b", SyncDigestVersion2, "valid", "void"), "manual")
	if decision.Status != SyncReconcileConflict || decision.ReasonCode != "cloud_status_conflict" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestSyncRetryBackoffIsBounded(t *testing.T) {
	if first, second := syncRetryBackoff(1), syncRetryBackoff(2); first != 10*time.Second || second != 20*time.Second {
		t.Fatalf("backoff=%v,%v", first, second)
	}
	if value := syncRetryBackoff(100); value != 15*time.Minute {
		t.Fatalf("cap=%v", value)
	}
}

func TestSyncFailureStateCategories(t *testing.T) {
	for _, test := range []struct {
		category string
		status   string
		next     bool
	}{
		{SyncCategoryTemporary, SyncChangeTemporaryFailure, true},
		{SyncCategoryPredecessorPending, SyncChangeWaitingPredecessor, true},
		{SyncCategoryConfigOrData, SyncChangeNeedsAdmin, false},
		{SyncCategoryConflict, SyncChangeConflict, false},
	} {
		status, next := syncFailureState(&SyncCloudError{Category: test.category}, 1)
		if status != test.status || (next != nil) != test.next {
			t.Errorf("category %s -> %s next=%v", test.category, status, next)
		}
	}
}
