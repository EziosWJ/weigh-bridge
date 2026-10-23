export type OperatorRole = 'operator' | 'supervisor' | 'admin';
export type Direction = 'inbound' | 'outbound';
export type TaskStatus = 'first' | 'second' | 'completed' | 'terminated';
export type PartyKind = 'supplier' | 'customer' | 'carrier';
export type PartyNature = 'enterprise' | 'individual';
export interface MaterialCategory { version?: number; id: string; name: string; enabled: boolean }
export interface Material { version?: number; id: string; code: string; name: string; categoryId: string; spec: string; unit: '吨'; enabled: boolean; remark: string }
export interface PartyContact { id: string; name: string; phone: string; purpose: string; remark: string; isDefault: boolean }
export interface Party { version?: number; id: string; code: string; name: string; shortName: string; nature: PartyNature; roles: PartyKind[]; phone: string; address: string; creditCode: string; enabled: boolean; remark: string; contacts: PartyContact[] }
export const partyRoleLabels: Record<PartyKind, string> = { supplier: '供货商', customer: '客户', carrier: '承运商' };
export const partyNatureLabels: Record<PartyNature, string> = { enterprise: '企业', individual: '个人' };
export const emptyTaskInput = (): TaskInput => ({ plate: '', direction: 'inbound', materialId: '', partyId: '', carrierId: '', contactName: '', contactPhone: '', driver: '', driverPhone: '', remark: '' });
export interface BusinessInfo { plate: string; direction: Direction; materialId: string; materialName: string; partyId: string; partyName: string; driver: string; remark: string; materialCode?: string; materialSpec?: string; materialCategoryName?: string; partyCode?: string; partyNature?: PartyNature; partyPhone?: string; partyAddress?: string; carrierId?: string; carrierName?: string; carrierCode?: string; contactName?: string; contactPhone?: string; driverPhone?: string }
export type TaskInput = Pick<BusinessInfo, 'plate' | 'direction' | 'materialId' | 'partyId' | 'driver' | 'remark' | 'carrierId' | 'contactName' | 'contactPhone' | 'driverPhone'>;
export type CameraPreset = 'front' | 'rear';
export interface PlateEvidence { source: 'agent' | 'simulated' | 'manual'; scenePlate?: string; candidate?: string; confirmedPlate: string; at: string }
export interface CaptureSnapshot { captureRequired?: boolean; capturePolicyRevision?: string; imageStatus?: 'complete' | 'disabled' | 'capture_failed' | 'upload_failed'; imageError?: string; stage?: 'first' | 'second'; plateEvidence?: PlateEvidence; id: string; preset: CameraPreset; imageSrc: string; source?: 'agent' | 'simulated'; images?: DeviceImage[]; cameraLabel: string; at: string; weight: number; sourceWeightKg?: number; plate?: string }
export interface WeighRecord { actor?: string; role?: OperatorRole; retainedRecordId?: string; id: string; stage: 'first' | 'second'; kind: 'gross' | 'tare'; weight: number; sourceWeightKg?: number; at: string; evidence: 'agent' | 'simulated'; capture?: CaptureSnapshot; replaces?: string; reason?: string }
export interface HistoryEntry { actor?: string; at: string; role?: OperatorRole; action: string; reason?: string }
export interface TransportTask extends BusinessInfo { siteId?:number;scaleId?:number; taskNo?: string; version?: number; id: string; createdAt: string; status: TaskStatus; records: WeighRecord[]; history: HistoryEntry[]; verifiedDate?: string; crossDayDoubt?: boolean; example?: boolean }
export type PrintStatus = 'unprinted' | 'failed' | 'unknown' | 'printed';
export interface WeightValues { gross: number; tare: number; net: number; sourceKg?: { gross: number; tare: number; net: number } }
export interface WeightDeduction { grossDeduction: number; tareDeduction: number; reason?: string }
export type FinalWeightInput = Pick<WeightValues, 'gross' | 'tare'> & { reason?: string };
export interface WeightAdjustment extends WeightDeduction { source?: 'manual' | 'collected'; actor: string; role?: OperatorRole; at: string; original: WeightValues; before: WeightValues; after: WeightValues }
export interface PrintSheet { template: string; widthMm: number; heightMm: number; layers: number; title: string; marks: string[]; rows: { label: string; value: string }[][] }
export interface WeighingTicket extends BusinessInfo { siteId?:number;siteName?:string; printSheet?: PrintSheet; legacyTest?: boolean; lastPrintAttemptId?: string; ticketNo?: string; id: string; taskId: string; issuedAt: string; status: 'valid' | 'void'; gross: number; tare: number; net: number; sourceKg?: { gross: number; tare: number; net: number }; adjustment?: WeightAdjustment; unit: '吨'; records: WeighRecord[]; history: HistoryEntry[]; previousId?: string; printStatus: PrintStatus; reprintCount: number; reprintAuthorized: boolean; example?: boolean }
export interface DeviceImage { id: string; view: 'front' | 'rear'; url: string; sha256: string }
export interface DeviceState { captureRequired?: boolean; capturePolicyRevision?: string; captureBlockedReason?: string; connectionId?:string; agentId: string; scaleId: number; weight: string; stable: boolean; deviceOk: boolean; online: boolean; revision: string; receivedAt: string; plate: { candidate: string; failed: boolean; at: string }; images: DeviceImage[] }
export interface DeviceConfig { role: OperatorRole; weight: number; stable: boolean; timedOut: boolean; deviceError: boolean; online: boolean; evidence: boolean }

export interface PendingSubmission { operation?: 'reweigh' | 'reopen' | 'correct' | 'terminate' | 'void' | 'print'; taskId: string; stage: 'first' | 'second'; committed: boolean; requestId?: string; plate?: string }
export interface WeighingState { resetAt?: string; draftPlateEvidence?: PlateEvidence; version: 3; categories: MaterialCategory[]; draft: TaskInput; draftRequestId: string; firstRequests: Record<string, string>; materials: Material[]; parties: Party[]; tasks: TransportTask[]; tickets: WeighingTicket[]; config: DeviceConfig; capture?: CaptureSnapshot; sampleReady: boolean; owner: 'self' | 'other' | null; pending?: PendingSubmission; history: HistoryEntry[]; sequence: number }
export type WeighingCommand = (
 | { type: 'capture'; stage?: 'first' | 'second'; taskId?: string; plate?: string; plateEvidence?: PlateEvidence }
 | { type: 'draft'; patch: Partial<TaskInput> }
 | { type: 'verifyPlate'; evidence: PlateEvidence }
 | { type: 'clearDraft' }
 | { type: 'firstWeigh'; input: TaskInput; checked: boolean; requestId: string }
 | { type: 'createTask'; input: TaskInput }
 | { type: 'confirm'; taskId: string; stage: 'first' | 'second'; checked: boolean; finalWeights?: FinalWeightInput; actor?: string }
 | { type: 'resolveSubmission' }
 | { type: 'retrySubmission' }
 | { type: 'correctTask'; taskId: string; input: TaskInput; reason: string }
 | { type: 'reweigh'; taskId: string; reason: string; sameLoad: boolean; checked: boolean; stage?: 'first' | 'second'; otherValid?: boolean; captureId?: string }
 | { type: 'terminate'; taskId: string; reason: string }
 | { type: 'verifyDay'; taskId: string; reason: string; doubtful: boolean; confirmed: boolean }
 | { type: 'voidTicket'; ticketId: string; reason: string }
 | { type: 'reopenTicket'; ticketId: string; input: TaskInput; reason: string; evidenceValid: boolean; finalWeights?: FinalWeightInput; actor?: string }
 | { type: 'previewPrint'; ticketId: string }
 | { type: 'startPrint'; ticketId: string; intent: 'initial' | 'retry' | 'reprint'; reason?: string }
 | { type: 'resolvePrint'; ticketId: string; printed: boolean; status?: 'printed' | 'failed' | 'unknown'; reason: string }
 | { type: 'authorizeReprint'; ticketId: string; reason: string }
 | { type: 'occupy'; owner: 'self' | 'other' | null }
 | { type: 'takeover'; reason: string }
 | { type: 'saveMaterial'; id?: string; name: string; enabled: boolean; categoryId?: string; spec?: string; remark?: string }
 | { type: 'saveParty'; id?: string; name: string; enabled: boolean; kind?: PartyKind; nature?: PartyNature; roles?: PartyKind[]; shortName?: string; phone?: string; address?: string; creditCode?: string; remark?: string; contacts?: PartyContact[] }
 | { type: 'saveCategory'; id?: string; name: string; enabled: boolean }
 | { type: 'deleteCategory'; id: string }
 | { type: 'deleteMaterial'; id: string }
 | { type: 'deleteParty'; id: string }
 | { type: 'reset' }) & { actor?: string };
export interface WeighingFilter { siteId?:string; keyword?: string; direction?: string; materialId?: string; partyId?: string; status?: string; from?: string; to?: string }
export const roleLabels: Record<OperatorRole, string> = { operator: '操作员', supervisor: '主管', admin: '管理员' };
export const directionLabels: Record<Direction, string> = { inbound: '入厂卸货', outbound: '出厂装货' };
export const taskStatusLabels: Record<TaskStatus, string> = { first: '待首次称重', second: '待二次称重', completed: '已完成', terminated: '已终止' };
export const printStatusLabels: Record<PrintStatus, string> = { unprinted: '未发起打印', failed: '明确失败未出纸', unknown: '待核实', printed: '已打印' };
