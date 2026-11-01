import { http } from '@/lib/http';
import type { ApiPageResult } from '@/types/api';
import type { WireTicket } from '@/api/weighing-workflow';

const base = '/api/weighing/sync';

export type SyncChangeStatus = 'pending' | 'waiting_predecessor' | 'temporary_failure' | 'needs_admin' | 'conflict' | 'synced';
export type SyncReconcileStatus = 'consistent' | 'pending' | 'needs_review' | 'conflict' | 'unknown';

export interface SyncTicket extends WireTicket {
  syncProtocol?: number;
  syncReason?: string;
  reconcileStatus?: SyncReconcileStatus | '';
  reconcileAt?: string | null;
}

export interface SyncReconcileSummary {
  checkedAt: string | null;
  status: string;
  checked: number;
  consistent: number;
  pending: number;
  needsReview: number;
  conflict: number;
  unknown: number;
  reason: string;
}

export interface SyncOverview {
  enabled: boolean;
  bound: boolean;
  pendingTickets: number;
  needsActionTickets: number;
  lastSuccess: string | null;
  masterLastSuccess: string | null;
  lastReconcile: SyncReconcileSummary | null;
}

export interface SyncChange {
  id: number;
  protocol: number;
  digest?: string;
  digestVersion?: number;
  schemaVersion?: number;
  siteCode: string;
  ticketNo: string;
  changeType: 'issue' | 'void' | 'reopen';
  status: SyncChangeStatus;
  category?: string;
  reasonCode?: string;
  reason?: string;
  attempts: number;
  nextAttemptAt: string | null;
  lastAttemptAt: string | null;
  syncedAt: string | null;
  receivedAt: string | null;
  createdAt: string;
}

export interface LegacyChange {
  id: number;
  changeId: string;
  ticketNo: string;
  changeType: string;
  status: string;
  reason: string;
  createdAt: string;
  lastAttemptAt: string | null;
  syncedAt: string | null;
}

export interface SyncReconcile {
  status: SyncReconcileStatus;
  cloudFound: boolean | null;
  cloudStatus: string | null;
  localLatestDigest: string | null;
  cloudLatestDigest: string | null;
  cloudLatestDigestVersion: number | null;
  cloudLatestChangeType: string | null;
  cloudReceivedAt: string | null;
  detail: unknown;
  checkedAt: string;
  triggerType: 'manual' | 'automatic';
}

export interface SyncReview {
  id: number;
  digest: string;
  reasonCode: string;
  status: 'open' | 'resolved';
  blocking: boolean;
  detail: unknown;
  conclusion: string;
  concludedByName: string;
  concludedAt: string | null;
  lastCheckedAt: string;
  createdAt: string;
}

export interface SyncTicketLink { id: number; ticketNo: string; status: string }

export interface SyncTicketDetail {
  ticket: SyncTicket;
  changes: SyncChange[];
  legacyChanges: LegacyChange[] | null;
  reconcile: SyncReconcile | null;
  reviews: SyncReview[];
  predecessor: SyncTicketLink | null;
  successor: SyncTicketLink | null;
}

export interface SyncLog {
  id: number;
  attemptedAt: string;
  triggerType: 'manual' | 'automatic';
  syncType: 'master' | 'issue' | 'void' | 'reopen' | 'reconcile';
  ticketNo: string;
  digest: string;
  category: string;
  success: boolean;
  reason: string;
}

export interface SyncTicketQuery {
  page?: number;
  pageSize?: number;
  keyword?: string;
  syncStatus?: string;
  reconcileStatus?: string;
  protocol?: number;
  from?: string;
  to?: string;
}

export interface SyncConfirmation {
  found: boolean;
  siteCode: string;
  digest: string;
  confirmation: {
    digest: string;
    ticketNo: string;
    changeType: string;
    digestVersion: number;
    schemaVersion: number;
    receivedAt: string;
  } | null;
}

export const syncApi = {
  overview: () => http.get<SyncOverview>(`${base}/overview`),
  tickets: (query: SyncTicketQuery) => http.get<ApiPageResult<SyncTicket>>(`${base}/tickets`, { query: { ...query } }),
  detail: (id: number | string) => http.get<SyncTicketDetail>(`${base}/tickets/${id}`),
  reconcileTicket: (id: number | string) => http.post<{ status: string; reasonCode: string; message: string }>(`${base}/tickets/${id}/reconcile`, {}),
  reconcileRange: (from: string, to: string) => http.post<SyncReconcileSummary>(`${base}/reconcile`, { from, to }),
  retryChange: (id: number) => http.post<{ queued: boolean; waitingPredecessor: boolean; message: string }>(`${base}/changes/${id}/retry`, {}),
  recordConclusion: (id: number, conclusion: string) => http.post(`${base}/reviews/${id}/conclusion`, { conclusion }),
  recheck: (id: number) => http.post<{ status: string; reasonCode: string; message: string }>(`${base}/reviews/${id}/recheck`, {}),
  lookupConfirmation: (siteCode: string, digest: string) => http.get<SyncConfirmation>(`${base}/confirmations`, { query: { siteCode, digest } }),
  logs: () => http.get<SyncLog[]>(`${base}/logs`),
  clearLogs: () => http.delete(`${base}/logs`),
};
