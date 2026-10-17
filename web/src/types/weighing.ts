export type DemoRole = 'operator' | 'supervisor' | 'admin';
export type Direction = 'inbound' | 'outbound';
export type TaskStatus = 'first' | 'second' | 'completed' | 'terminated';
export type PartyKind = 'supplier' | 'customer' | 'carrier';
export type PartyNature = 'enterprise' | 'individual';
export interface MaterialCategory { id: string; name: string; enabled: boolean }
export interface Material { id: string; code: string; name: string; categoryId: string; spec: string; unit: '吨'; enabled: boolean; remark: string }
export interface PartyContact { id: string; name: string; phone: string; purpose: string; remark: string; isDefault: boolean }
export interface Party { id: string; code: string; name: string; shortName: string; nature: PartyNature; roles: PartyKind[]; phone: string; address: string; creditCode: string; enabled: boolean; remark: string; contacts: PartyContact[] }
export const partyRoleLabels: Record<PartyKind, string> = { supplier: '供货商', customer: '客户', carrier: '承运商' };
export const partyNatureLabels: Record<PartyNature, string> = { enterprise: '企业', individual: '个人' };
export const emptyTaskInput = (): TaskInput => ({ plate: '', direction: 'inbound', materialId: '', partyId: '', carrierId: '', contactName: '', contactPhone: '', driver: '', driverPhone: '', remark: '' });
export interface BusinessInfo { plate: string; direction: Direction; materialId: string; materialName: string; partyId: string; partyName: string; driver: string; remark: string; materialCode?: string; materialSpec?: string; materialCategoryName?: string; partyCode?: string; partyNature?: PartyNature; partyPhone?: string; partyAddress?: string; carrierId?: string; carrierName?: string; carrierCode?: string; contactName?: string; contactPhone?: string; driverPhone?: string }
export type TaskInput = Pick<BusinessInfo, 'plate' | 'direction' | 'materialId' | 'partyId' | 'driver' | 'remark' | 'carrierId' | 'contactName' | 'contactPhone' | 'driverPhone'>;
export type CameraPreset = 'front' | 'rear';
export interface SimulatedCapture { id: string; preset: CameraPreset; imageSrc: string; cameraLabel: string; at: string; weight: number; sourceWeightKg?: number; plate?: string }
export interface WeighRecord { id: string; stage: 'first' | 'second'; kind: 'gross' | 'tare'; weight: number; sourceWeightKg?: number; at: string; evidence: 'simulated'; capture?: SimulatedCapture; replaces?: string; reason?: string }
export interface HistoryEntry { at: string; role: DemoRole; action: string; reason?: string }
export interface TransportTask extends BusinessInfo { id: string; createdAt: string; status: TaskStatus; records: WeighRecord[]; history: HistoryEntry[]; verifiedDate?: string; crossDayDoubt?: boolean; example?: boolean }
export type PrintStatus = 'unprinted' | 'failed' | 'unknown' | 'printed';
export interface WeightValues { gross: number; tare: number; net: number; sourceKg?: { gross: number; tare: number; net: number } }
export interface WeightDeduction { grossDeduction: number; tareDeduction: number; reason?: string }
export type FinalWeightInput = Pick<WeightValues, 'gross' | 'tare'> & { reason?: string };
export interface WeightAdjustment extends WeightDeduction { actor: string; role: DemoRole; at: string; original: WeightValues; before: WeightValues; after: WeightValues }
export interface TestTicket extends BusinessInfo { id: string; taskId: string; issuedAt: string; status: 'valid' | 'void'; gross: number; tare: number; net: number; sourceKg?: { gross: number; tare: number; net: number }; adjustment?: WeightAdjustment; unit: '吨'; records: WeighRecord[]; history: HistoryEntry[]; previousId?: string; printStatus: PrintStatus; reprintCount: number; reprintAuthorized: boolean; example?: boolean }
export interface DemoConfig { cameraPreset?: CameraPreset; role: DemoRole; weight: number; sourceWeightKg?: number; stable: boolean; timedOut: boolean; deviceError: boolean; online: boolean; evidence: boolean; submission: 'normal' | 'unknown-success' | 'unknown-failure'; printOutcome: 'printed' | 'failed' | 'unknown' }
export interface PendingSubmission { taskId: string; stage: 'first' | 'second'; committed: boolean; requestId?: string; plate?: string }
export interface WeighingState { version: 3; categories: MaterialCategory[]; draft: TaskInput; draftRequestId: string; firstRequests: Record<string, string>; materials: Material[]; parties: Party[]; tasks: TransportTask[]; tickets: TestTicket[]; config: DemoConfig; capture?: SimulatedCapture; sampleReady: boolean; owner: 'self' | 'other' | null; pending?: PendingSubmission; history: HistoryEntry[]; sequence: number }
export type WeighingCommand =
 | { type: 'config'; patch: Partial<DemoConfig> }
 | { type: 'capture'; taskId?: string; plate?: string }
 | { type: 'draft'; patch: Partial<TaskInput> }
 | { type: 'clearDraft' }
 | { type: 'firstWeigh'; input: TaskInput; checked: boolean; requestId: string }
 | { type: 'createTask'; input: TaskInput }
 | { type: 'confirm'; taskId: string; stage: 'first' | 'second'; checked: boolean; finalWeights?: FinalWeightInput; actor?: string }
 | { type: 'resolveSubmission' }
 | { type: 'correctTask'; taskId: string; input: TaskInput; reason: string }
 | { type: 'reweigh'; taskId: string; reason: string; sameLoad: boolean; checked: boolean }
 | { type: 'terminate'; taskId: string; reason: string }
 | { type: 'verifyDay'; taskId: string; reason: string; doubtful: boolean; confirmed: boolean }
 | { type: 'voidTicket'; ticketId: string; reason: string }
 | { type: 'reopenTicket'; ticketId: string; input: TaskInput; reason: string; evidenceValid: boolean; finalWeights?: FinalWeightInput; actor?: string }
 | { type: 'previewPrint'; ticketId: string }
 | { type: 'resolvePrint'; ticketId: string; printed: boolean; reason: string }
 | { type: 'authorizeReprint'; ticketId: string; reason: string }
 | { type: 'occupy'; owner: 'self' | 'other' | null }
 | { type: 'takeover'; reason: string }
 | { type: 'saveMaterial'; id?: string; name: string; enabled: boolean; categoryId?: string; spec?: string; remark?: string }
 | { type: 'saveParty'; id?: string; name: string; enabled: boolean; kind?: PartyKind; nature?: PartyNature; roles?: PartyKind[]; shortName?: string; phone?: string; address?: string; creditCode?: string; remark?: string; contacts?: PartyContact[] }
 | { type: 'saveCategory'; id?: string; name: string; enabled: boolean }
 | { type: 'deleteCategory'; id: string }
 | { type: 'deleteMaterial'; id: string }
 | { type: 'deleteParty'; id: string }
 | { type: 'reset' };
export interface WeighingFilter { keyword?: string; direction?: string; materialId?: string; partyId?: string; status?: string; from?: string; to?: string }
export const roleLabels: Record<DemoRole, string> = { operator: '操作员', supervisor: '主管', admin: '管理员' };
export const directionLabels: Record<Direction, string> = { inbound: '入厂卸货', outbound: '出厂装货' };
export const taskStatusLabels: Record<TaskStatus, string> = { first: '待首次称重', second: '待二次称重', completed: '已完成', terminated: '已终止' };
export const printStatusLabels: Record<PrintStatus, string> = { unprinted: '未模拟打印', failed: '明确失败未出纸', unknown: '待核实', printed: '已模拟打印' };
