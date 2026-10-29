import { http } from '@/lib/http';
import type { BusinessInfo, HistoryEntry, PlateEvidence, CaptureSnapshot, WeighingTicket, TransportTask, WeighRecord, PrintSheet } from '@/types/weighing';

type WireBusiness = Omit<BusinessInfo, 'materialId' | 'partyId' | 'carrierId'> & { materialId: number; partyId: number; carrierId: number | null };
export interface WireEvent { action: string; reason: string; actorName: string; occurredAt: string }
export interface WireRecord {
  id: number; taskId: number; stage: 'first' | 'second'; kind: 'gross' | 'tare';
  weight: string; capturedAt: string; confirmedAt: string; actorName: string;
  replacesRecordId: number | null; retainedRecordId: number | null; reason: string;
  evidence: Omit<CaptureSnapshot, 'weight'> & { weight: string };
  plateEvidence: PlateEvidence;
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
export interface WireTicket {
 imagesComplete?:boolean;
 siteCode:string;siteName:string; syncStatus?: 'pending' | 'synced' | 'failed' | 'untracked';
  printSheet?: PrintSheet;
  mode?: string;
  id: number; taskId: number; ticketNo: string; revision: number;
  business: WireBusiness; basis: WireRecord[]; previousTicketId: number | null;
  status: 'valid' | 'void'; issuedAt: string; issuerName: string;
  originalGross: string; originalTare: string; finalGross: string; finalTare: string; finalNet: string;
  netAdjustment: string; netAdjustmentMode: 'amount' | 'percent'; netAdjustmentRateBp: number | null; netAdjustmentCategory: string; netAdjustmentCategoryName: string;
  grossSource: string; tareSource: string; adjustmentRemark: string;
  voidedAt: string | null; voidReason: string;
  printStatus?: 'unprinted' | 'unknown' | 'printed' | 'failed'; reprintCount?: number; lastPrintAttemptId?: number | null;
  previousFinal?: { gross: string; tare: string; net: string; netAdjustment?: string; categoryName?: string } | null;
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
  plateCandidates: (scale: string, after?: string, signal?: AbortSignal) => http.get<PlateCandidatePage>(`${base}/plate-candidates`, { query: { scale, after }, signal }),
  task: (id: string | number) => http.get<WireTask>(`${base}/tasks/${id}`),
  ticket: (id: string | number) => http.get<WireTicket>(`${base}/tickets/${id}`),
  operation: (requestId: string) => http.get<Operation>(`${base}/operations/${requestId}`),
};
function business(row: WireBusiness): BusinessInfo {
  return { ...row, materialId: String(row.materialId), partyId: String(row.partyId), carrierId: row.carrierId == null ? '' : String(row.carrierId) };
}
export function history(row: WireEvent): HistoryEntry { return { action: row.action, reason: row.reason || undefined, actor: row.actorName, at: row.occurredAt }; }
export function record(row: WireRecord): WeighRecord {
  const capture = { ...row.evidence, id: `sample-${row.id}`, weight: Number(row.weight), plateEvidence: row.plateEvidence };
  return { id: String(row.id), stage: row.stage, kind: row.kind, weight: Number(row.weight), at: row.confirmedAt, actor: row.actorName, evidence: row.evidence.source ?? 'agent', capture, replaces: row.replacesRecordId == null ? undefined : String(row.replacesRecordId), retainedRecordId: row.retainedRecordId == null ? undefined : String(row.retainedRecordId), reason: row.reason || undefined };
}
export function task(row: WireTask): TransportTask {
  return { ...business(row.business), siteCode:row.siteCode,scaleCode:row.scaleCode,id: String(row.id), taskNo: row.taskNo, version: row.version, createdAt: row.createTime, status: row.status === 'awaiting_second' ? 'second' : row.status, records: (row.records ?? []).map(record), history: (row.events ?? []).map(history) };
}
export function ticket(row: WireTicket, previous?: WireTicket): WeighingTicket {
  if (!row.previousFinal && previous) row = { ...row, previousFinal: { gross: previous.finalGross, tare: previous.finalTare, net: previous.finalNet, netAdjustment: previous.netAdjustment, categoryName: previous.netAdjustmentCategoryName } };
  const original = { gross: Number(row.originalGross), tare: Number(row.originalTare), net: Number(row.originalGross) - Number(row.originalTare) };
  const netAdjustment = Number(row.netAdjustment ?? 0);
  const gross = Number(row.finalGross);
  const tare = Number(row.finalTare);
  const serverNet = Number(row.finalNet);
  const after = { gross, tare, net: Number.isFinite(serverNet) ? serverNet : gross - tare - netAdjustment };
  const previousNet = row.previousFinal ? Number(row.previousFinal.net) : Number.NaN;
  const before = row.previousFinal ? { gross: Number(row.previousFinal.gross), tare: Number(row.previousFinal.tare), net: Number.isFinite(previousNet) ? previousNet : Number(row.previousFinal.gross) - Number(row.previousFinal.tare) - Number(row.previousFinal.netAdjustment ?? 0) } : original;
  return { ...business(row.business), ...after, netAdjustment, netAdjustmentMode: row.netAdjustmentMode ?? 'amount', netAdjustmentRateBp: row.netAdjustmentRateBp ?? undefined, netAdjustmentCategory: row.netAdjustmentCategory || undefined, netAdjustmentCategoryName: row.netAdjustmentCategoryName || undefined,siteCode:row.siteCode,siteName:row.siteName, printSheet: row.printSheet, legacyTest: row.mode === 'test', id: String(row.id), ticketNo: row.ticketNo, taskId: String(row.taskId), issuedAt: row.issuedAt, status: row.status, previousId: row.previousTicketId == null ? undefined : String(row.previousTicketId), unit: '吨', records: row.basis.map(record), history: [{ actor: row.issuerName, at: row.issuedAt, action: '生成过磅单' }, ...(row.voidedAt ? [{ at: row.voidedAt, action: '单据作废', reason: row.voidReason }] : [])], adjustment: { source: row.grossSource === 'manual' || row.tareSource === 'manual' || netAdjustment !== 0 ? 'manual' : 'collected', actor: row.issuerName, at: row.issuedAt, original, before, after, grossDeduction: original.gross - after.gross, tareDeduction: original.tare - after.tare, netAdjustment, netAdjustmentMode: row.netAdjustmentMode ?? 'amount', netAdjustmentRateBp: row.netAdjustmentRateBp ?? undefined, netAdjustmentCategory: row.netAdjustmentCategory || undefined, netAdjustmentCategoryName: row.netAdjustmentCategoryName || undefined, reason: row.adjustmentRemark }, printStatus: row.printStatus ?? 'unprinted', reprintCount: row.reprintCount ?? 0, lastPrintAttemptId: row.lastPrintAttemptId == null ? undefined : String(row.lastPrintAttemptId), reprintAuthorized: false };
}

export interface WireOriginal extends WireTask { originalGross: string; originalTare: string; originalNet: string; firstConfirmedAt: string; secondConfirmedAt: string; currentTicketId: number | null; currentTicketStatus: string }
