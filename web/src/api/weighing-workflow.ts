import { http } from '@/lib/http';
import type { NetAdjustmentItemView, BusinessInfo, HistoryEntry, PlateEvidence, CaptureSnapshot, WeighingTicket, TransportTask, WeighRecord, PrintSheet, TaskTripBasis, TaskTripCloudStatus, TripCandidateResult } from '@/types/weighing';

type WireBusiness = Omit<BusinessInfo, 'materialId' | 'partyId' | 'carrierId'> & { materialId: number; partyId: number; carrierId: number | null };
export interface WireEvent { action: string; reason: string; actorName: string; occurredAt: string; ticketId?: number | null }
export interface WireRecord {
  id: number; taskId: number; stage: 'first' | 'second'; kind: 'gross' | 'tare';
  weight: string; capturedAt: string; confirmedAt: string; actorName: string;
  replacesRecordId: number | null; retainedRecordId: number | null; reason: string;
  evidence: Omit<CaptureSnapshot, 'weight'> & { weight: string };
  plateEvidence: PlateEvidence;
  source?: 'collected' | 'manual';
}
export interface WireTask {
  siteCode: string; scaleCode: string; mode?: string; id: number; taskNo: string; version: number; business: WireBusiness;
  firstWeight?: string | null; secondWeight?: string | null;
  createTime: string; status: 'awaiting_second' | 'completed' | 'terminated';
  records?: WireRecord[]; events?: WireEvent[];
}
export interface WireSample {
  id: number; taskId: number | null; targetRecordId: number | null; stage: 'first' | 'second'; weight: string; expiresAt: string | null;
  evidence: Omit<CaptureSnapshot, 'weight'> & { weight: string };
  plateEvidence: PlateEvidence;
}
export interface WireSession {
  scaleCode: string; holderUserId: number | null; holderName: string;
  clientId: string; expiresAt: string | null; generation: number;
  sessionToken?: string;
}
type WireAdjustmentItem = { mode: 'amount' | 'percent'; amountMilli: number; rateBp?: number | null; category: string; categoryName: string; remark?: string };

export interface WireTicket {
 entryType?: string; supplementReason?: string;
 imagesComplete?:boolean;
 siteCode:string;siteName:string; syncStatus?: 'pending' | 'waiting_predecessor' | 'temporary_failure' | 'needs_admin' | 'conflict' | 'synced' | 'failed' | 'untracked';
 syncProtocol?: number; syncReason?: string; reconcileStatus?: 'consistent' | 'pending' | 'needs_review' | 'conflict' | 'unknown' | ''; reconcileAt?: string | null;
  printSheet?: PrintSheet;
  mode?: string;
  id: number; taskId: number; ticketNo: string; revision: number;
  business: WireBusiness; basis: WireRecord[]; previousTicketId: number | null;
  status: 'valid' | 'void'; issuedAt: string; issuerName: string;
  originalGross: string; originalTare: string; finalGross: string; finalTare: string; finalNet: string;
  netAdjustment: string; netAdjustmentMode: 'amount' | 'percent'; netAdjustmentRateBp: number | null; netAdjustmentCategory: string; netAdjustmentCategoryName: string;
  netAdjustmentItems?: WireAdjustmentItem[];
  grossSource: string; tareSource: string; adjustmentRemark: string;
  voidedAt: string | null; voidReason: string;
  printStatus?: 'unprinted' | 'unknown' | 'printed' | 'failed'; reprintCount?: number; lastPrintAttemptId?: number | null;
  previousFinal?: { gross: string; tare: string; net: string; netAdjustment?: string; categoryName?: string; items?: WireAdjustmentItem[] } | null;
}
export interface CommandResult { taskId: number; recordId?: number; ticketId?: number; taskVersion: number; status: string }
export interface Operation { status: 'not_found' | 'committed' | 'rejected'; result: CommandResult | null; error: string }
export interface PlateSourceEvent { eventRecordId: number; cameraId: number; cameraName: string; receivedAt: string; recognizedAt: string; rawPlate?: string }
export interface PlateCandidate { id: number; plate: string; sourceEvents: PlateSourceEvent[]; firstReceivedAt: string; lastReceivedAt: string; expiresAt: string }
export interface PlateCandidatePage { items: PlateCandidate[]; nextCursor: string; serverTime: string }
const base = '/api/weighing';
export const workflowApi = {
  get: <T>(path: string) => http.get<T>(`${base}${path}`),
  post: <T>(path: string, body: unknown) => http.post<T>(`${base}${path}`, body),
  active: (plate: string, scale?: string) => http.get<WireTask | null>(`${base}/tasks/active`, { query: { plate,scale } }),
  tripCandidates: (plate: string) => http.get<TripCandidateResult>(`${base}/trip-candidates`, { query: { plate } }),
  taskTripBasis: (id: string | number) => http.get<TaskTripBasis>(`${base}/tasks/${id}/trip-basis`),
  taskTripCloudStatus: (id: string | number) => http.get<TaskTripCloudStatus>(`${base}/tasks/${id}/trip-cloud-status`),
  plateCandidates: (scale: string, after?: string, signal?: AbortSignal) => http.get<PlateCandidatePage>(`${base}/plate-candidates`, { query: { scale, after }, signal }),
  task: (id: string | number) => http.get<WireTask>(`${base}/tasks/${id}`),
  ticket: (id: string | number) => http.get<WireTicket>(`${base}/tickets/${id}`),
  operation: (requestId: string) => http.get<Operation>(`${base}/operations/${requestId}`),
};
function business(row: WireBusiness): BusinessInfo {
  return { ...row, materialId: String(row.materialId), partyId: String(row.partyId), carrierId: row.carrierId == null ? '' : String(row.carrierId) };
}
export function history(row: WireEvent): HistoryEntry { return { action: row.action, ticketId: row.ticketId == null ? undefined : String(row.ticketId), reason: row.reason || undefined, actor: row.actorName, at: row.occurredAt }; }
export function record(row: WireRecord): WeighRecord {
  const capture = {
    ...row.evidence,
    id: `sample-${row.id}`,
    at: row.evidence?.at || row.capturedAt,
    weight: Number(row.weight),
    plateEvidence: row.plateEvidence,
  };
  return {
    id: String(row.id), stage: row.stage, kind: row.kind,
    weight: capture.weight, at: row.confirmedAt, capturedAt: row.capturedAt,
    source: row.source === 'manual' ? 'manual' : 'collected', actor: row.actorName,
    evidence: row.source === 'manual' ? 'manual' : row.evidence?.source ?? 'agent', capture,
    replaces: row.replacesRecordId == null ? undefined : String(row.replacesRecordId),
    retainedRecordId: row.retainedRecordId == null ? undefined : String(row.retainedRecordId),
    reason: row.reason || undefined,
  };
}
export function task(row: WireTask): TransportTask {
  return { ...business(row.business), siteCode:row.siteCode,scaleCode:row.scaleCode,id: String(row.id), taskNo: row.taskNo, version: row.version, createdAt: row.createTime, status: row.status === 'awaiting_second' ? 'second' : row.status, records: (row.records ?? []).map(record), history: (row.events ?? []).map(history) };
}
export function ticket(row: WireTicket, previous?: WireTicket): WeighingTicket {
  if (!row.previousFinal && previous) row = { ...row, previousFinal: { gross: previous.finalGross, tare: previous.finalTare, net: previous.finalNet, netAdjustment: previous.netAdjustment, categoryName: previous.netAdjustmentCategoryName, items: previous.netAdjustmentItems } };
  const original = { gross: Number(row.originalGross), tare: Number(row.originalTare), net: Number(row.originalGross) - Number(row.originalTare) };
  const netAdjustment = Number(row.netAdjustment ?? 0);
  const gross = Number(row.finalGross);
  const tare = Number(row.finalTare);
  const serverNet = Number(row.finalNet);
  const after = { gross, tare, net: Number.isFinite(serverNet) ? serverNet : gross - tare - netAdjustment };
  const previousNet = row.previousFinal ? Number(row.previousFinal.net) : Number.NaN;
  const before = row.previousFinal ? { gross: Number(row.previousFinal.gross), tare: Number(row.previousFinal.tare), net: Number.isFinite(previousNet) ? previousNet : Number(row.previousFinal.gross) - Number(row.previousFinal.tare) - Number(row.previousFinal.netAdjustment ?? 0) } : original;
  return { ...business(row.business), ...after, entryType: row.entryType, supplementReason: row.supplementReason, netAdjustment, netAdjustmentMode: row.netAdjustmentMode ?? 'amount', netAdjustmentRateBp: row.netAdjustmentRateBp ?? undefined, netAdjustmentCategory: row.netAdjustmentCategory || undefined, netAdjustmentCategoryName: row.netAdjustmentCategoryName || undefined, netAdjustmentItems: wireItemsToView(row.netAdjustmentItems), previousItems: wireItemsToView(row.previousFinal?.items),siteCode:row.siteCode,siteName:row.siteName, printSheet: row.printSheet, syncStatus: row.syncStatus, syncReason: row.syncReason, reconcileStatus: row.reconcileStatus, reconcileAt: row.reconcileAt, legacyTest: row.mode === 'test', id: String(row.id), ticketNo: row.ticketNo, taskId: String(row.taskId), issuedAt: row.issuedAt, status: row.status, previousId: row.previousTicketId == null ? undefined : String(row.previousTicketId), grossSource: row.grossSource, tareSource: row.tareSource, unit: '吨', records: row.basis.map(record), history: [{ actor: row.issuerName, at: row.issuedAt, action: row.previousTicketId == null ? '生成过磅单' : '更正重开' }, ...(row.voidedAt ? [{ at: row.voidedAt, action: '单据作废', reason: row.voidReason }] : [])], adjustment: { source: row.grossSource === 'manual' || row.tareSource === 'manual' || netAdjustment !== 0 ? 'manual' : 'collected', actor: row.issuerName, at: row.issuedAt, original, before, after, grossDeduction: original.gross - after.gross, tareDeduction: original.tare - after.tare, netAdjustment, netAdjustmentMode: row.netAdjustmentMode ?? 'amount', netAdjustmentRateBp: row.netAdjustmentRateBp ?? undefined, netAdjustmentCategory: row.netAdjustmentCategory || undefined, netAdjustmentCategoryName: row.netAdjustmentCategoryName || undefined, reason: row.adjustmentRemark }, printStatus: row.printStatus ?? 'unprinted', reprintCount: row.reprintCount ?? 0, lastPrintAttemptId: row.lastPrintAttemptId == null ? undefined : String(row.lastPrintAttemptId), reprintAuthorized: false };
}

export interface WireOriginal extends WireTask { originalGross: string; originalTare: string; originalNet: string; firstConfirmedAt: string; secondConfirmedAt: string; firstWeighedAt?: string; secondWeighedAt?: string; firstSource?: WireRecord['source']; secondSource?: WireRecord['source']; currentTicketId: number | null; currentTicketStatus: string; relatedTicketId?: number; relatedTicketNo?: string; entryType?: string }

function wireItemsToView(items?: WireAdjustmentItem[]): NetAdjustmentItemView[] | undefined {
  if (!items || items.length === 0) return undefined;
  return items.map(item => ({ mode: item.mode, amount: item.amountMilli / 1000, rateBp: item.rateBp ?? undefined, category: item.category || undefined, categoryName: item.categoryName || undefined, remark: item.remark || undefined }));
}

export interface WireMobileHandoff {
  id: number;
  handoffId: number;
  weighingId: number;
  action: 'first' | 'second';
  siteCode: string;
  scaleCode: string;
  plate: string;
  mobileAccount: string;
  source?: string;
  device?: string;
  status: 'claimed' | 'completed' | 'failed';
  taskId?: number | null;
  ticketId?: number | null;
  result?: {
    status?: string;
    taskNo?: string;
    ticketNo?: string;
    firstWeight?: string;
    secondWeight?: string;
    net?: string;
    firstAt?: string;
    secondAt?: string;
    captureStatus?: string;
    message?: string;
  } | null;
  error: string;
  claimedAt: string;
  completedAt?: string | null;
}

export interface WireMobileHandoffResult {
  handoffId: number;
  weighingId: number;
  action: 'first' | 'second';
  status: string;
  taskNo?: string;
  ticketNo?: string;
  firstWeight?: string;
  secondWeight?: string;
  net?: string;
  firstAt?: string;
  secondAt?: string;
  captureStatus?: string;
  message?: string;
}

export interface WireMobileHandoffPage {
  records: WireMobileHandoff[];
  total: number;
  page: number;
  pageSize: number;
}

export const mobileHandoffApi = {
  list: (query: { page: number; pageSize: number }) =>
    http.get<WireMobileHandoffPage>(`${base}/mobile-handoffs`, { query }),
  scan: (code: string) => http.post<WireMobileHandoffResult>(`${base}/mobile-handoffs/scan`, { code }),
};
