import { create } from 'zustand';
import { ApiError } from '@/lib/api-error';
import { plateError } from '@/lib/weighing/plate';
import { workflowApi, task as taskView, ticket as ticketView, type CommandResult, type WireSample, type WireSession, type WireTicket } from '@/api/weighing-workflow';
import type { ApiPageResult } from '@/types/api';
import type { WeighingCapabilities } from '@/api/weighing';
import { emptyTaskInput, type DeviceConfig, type DeviceState, type TaskInput, type TransportTask, type WeighingCommand, type WeighingState } from '@/types/weighing';
import { useAuthStore } from './auth-store';
import { useWeighingMasterStore } from './weighing-master-store';

// This namespace contains UI drafts/preferences and pending request ids, never business rows.
export const WEIGHING_STORAGE_KEY = 'weighbridge-server-ui-v1';
const navigation = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
let clientId = crypto.randomUUID();
try {
  const saved = sessionStorage.getItem(`${WEIGHING_STORAGE_KEY}:client`);
  if (saved && (!window.opener || navigation?.type === 'reload')) clientId = saved as `${string}-${string}-${string}-${string}-${string}`;
  sessionStorage.setItem(`${WEIGHING_STORAGE_KEY}:client`, clientId);
} catch { /* An in-memory client still obeys the server lease. */ }
const defaults = (): DeviceConfig => ({ role: 'operator', weight: 0, stable: false, timedOut: true, deviceError: false, online: false, evidence: false });
function empty(): WeighingState { return { version: 3, categories: [], materials: [], parties: [], tasks: [], tickets: [], config: defaults(), draft: emptyTaskInput(), draftRequestId: crypto.randomUUID(), firstRequests: {}, sampleReady: false, owner: null, history: [], sequence: 0 }; }
interface PendingRequest { actorId: number; path: string; body: Record<string, unknown>; requestId: string; taskId?: string; stage: 'first' | 'second'; operation?: 'reweigh' | 'reopen' | 'correct' | 'terminate' | 'void' | 'print' }
interface UIState { draft: TaskInput; draftRequestId: string; pending: PendingRequest | null }
interface Store {
  data: WeighingState; device: DeviceState | null; capabilities: WeighingCapabilities | null;
  ready: boolean; loading: boolean; busy: boolean; storageError: string; syncNotice: string;
  lastResolution: { committed: boolean; taskId: string } | null;
  refresh: () => Promise<void>;
  loadTask: (id: string | number) => Promise<TransportTask>;
  loadTicket: (id: string | number) => Promise<void>;
  loadPreview: (id: string | number) => Promise<void>;
  locate: (plate: string) => Promise<TransportTask | undefined>;
  dispatch: (command: WeighingCommand, expected?: WeighingState) => Promise<void>;
}
const isWorkbench=location.pathname==='/weighing/workbench';
let selectedScaleId=Number(new URLSearchParams(location.search).get('scaleId'))||0;
export function selectedScale(capabilities:WeighingCapabilities|null){return capabilities?.scales.find(s=>s.id===selectedScaleId&&s.enabled===1);}
let lease: WireSession | null = null;
let sample: WireSample | null = null;
let pendingRequest: PendingRequest | null = null;
let heartbeat: ReturnType<typeof setInterval> | null = null;
let refreshPromise: Promise<void> | null = null;
let refreshToken: string | null = null;
let actorId = 0;
const uiKey = () => `${WEIGHING_STORAGE_KEY}:${actorId}:${selectedScaleId}:${clientId}`;
const checks = { plate: true, business: true, weight: true, evidence: true, fullyOnScale: true };
function businessInput(value: TaskInput) { return { ...value, materialId: Number(value.materialId), partyId: Number(value.partyId), carrierId: value.carrierId ? Number(value.carrierId) : null }; }
function sessionInput() { return { clientId, sessionToken: lease?.sessionToken ?? '' }; }
function persist() {
  const { data } = useWeighingStore.getState();
  try { sessionStorage.setItem(uiKey(), JSON.stringify({ draft: data.draft, draftRequestId: data.draftRequestId, pending: pendingRequest } satisfies UIState)); }
  catch { useWeighingStore.setState({ storageError: '本地草稿或待决请求保存失败；业务记录仍由服务端保存。' }); }
}
function clearSample() { sample = null; useWeighingStore.setState(s => ({ data: { ...s.data, capture: undefined, sampleReady: false } })); }
async function invalidate() {
  const old = sample; clearSample();
  if (old && lease?.sessionToken) await workflowApi.post(`/samples/${old.id}/invalidate`, sessionInput());
}
function updateData(patch: Partial<WeighingState>) { useWeighingStore.setState(s => ({ data: { ...s.data, ...patch } })); }
function pendingView(request: PendingRequest) { return { requestId: request.requestId, taskId: request.taskId ?? '', stage: request.stage, operation: request.operation, committed: false }; }
function restoreUI() {
  try {
    const raw = sessionStorage.getItem(uiKey());
    if (raw) {
      const ui = JSON.parse(raw) as UIState;
      if (typeof ui.draft?.plate === 'string' && typeof ui.draftRequestId === 'string') updateData({ draft: ui.draft, draftRequestId: ui.draftRequestId });
      if (ui.pending?.actorId === actorId) { pendingRequest = ui.pending; updateData({ pending: pendingView(ui.pending) }); }
    }
  } catch { useWeighingStore.setState({ storageError: '本地工作草稿读取失败，未导入任何旧演示业务记录。' }); }
}
async function release(original?: PendingRequest) {
  const fallback = original && typeof original.body.scaleId === 'number' && typeof original.body.sessionToken === 'string' ? { scaleId: original.body.scaleId, sessionToken: original.body.sessionToken } : null;
  const current = lease ?? fallback; lease = null;
  if (heartbeat) { clearInterval(heartbeat); heartbeat = null; }
  updateData({ owner: null });
  if (current?.sessionToken) {
    try { await workflowApi.post(`/scales/${current.scaleId}/session/release`, { clientId, sessionToken: current.sessionToken }); }
    catch { /* The bounded server lease expires without changing committed business data. */ }
  }
}
async function acquire(action: 'acquire' | 'takeover' = 'acquire', reason = '') {
  const state = useWeighingStore.getState();
  const scale = selectedScale(state.capabilities);
  if (!scale?.canOperate) throw new Error('请从本厂称重入口进入已授权地磅');
  try { lease = await workflowApi.post<WireSession>(`/scales/${scale.id}/session/${action}`, { clientId, reason }); }
  catch (error) { updateData({ owner: 'other' }); throw error; }
  updateData({ owner: 'self' });
  if (heartbeat) clearInterval(heartbeat);
  const renewalPeriod = Math.max(250, Math.min(15_000, (Date.parse(lease.expiresAt ?? '') - Date.now()) / 4));
  heartbeat = setInterval(() => {
    if (!lease?.sessionToken) return;
    const current = lease;
    void workflowApi.post<WireSession>(`/scales/${current.scaleId}/session/renew`, sessionInput()).then(next => {
      if (lease === current) lease = { ...next, sessionToken: current.sessionToken };
    }).catch(() => { if (lease === current) { lease = null; if (heartbeat) clearInterval(heartbeat); heartbeat = null; clearSample(); updateData({ owner: null }); useWeighingStore.setState({ syncNotice: '操作会话已失效，请重新取得操作权并采集。' }); } });
  }, renewalPeriod);
}
async function ensureLease() { if (!lease?.sessionToken || !lease.expiresAt || Date.parse(lease.expiresAt) <= Date.now()) await acquire(); }
async function finishResult(result: CommandResult, request: PendingRequest) {
  if (useWeighingStore.getState().capabilities?.userId !== request.actorId) return;
  if (request.operation === 'print' && result.ticketId) await useWeighingStore.getState().loadPreview(result.ticketId);
  else { await useWeighingStore.getState().loadTask(result.taskId); if (result.ticketId) await useWeighingStore.getState().loadTicket(result.ticketId); }
  if (request.stage === 'first' && !request.operation) updateData({ firstRequests: { ...useWeighingStore.getState().data.firstRequests, [request.requestId]: String(result.taskId) } });
  pendingRequest = null; clearSample(); updateData({ pending: undefined });
  useWeighingStore.setState({ lastResolution: { committed: true, taskId: String(result.taskId) }, syncNotice: '' });
  persist(); await release(request);
}
async function send(request: PendingRequest, explicitRetry = false) {
  pendingRequest = request; updateData({ pending: pendingView(request) }); persist();
  try {
    const result = await workflowApi.post<CommandResult>(request.path, request.body);
    await finishResult(result, request);
  } catch (error) {
    // A deterministic rejection means the server did not apply the command.
    // Clear the pending marker before surfacing the error, otherwise detail and
    // list pages keep showing an "unverified submission" banner for a request
    // that was simply rejected (for example an unconfirmed weigh basis).
    const certain = error instanceof ApiError && error.type !== 'network' && error.type !== 'server';
    if (certain) {
      if(explicitRetry){
        const original=await workflowApi.operation(request.requestId);
        if(original.status==='committed'&&original.result){await finishResult(original.result,request);return;}
        if(original.status!=='rejected')throw new Error(`原请求结果仍未确认：${error.message}。请保留原请求并查询，勿新建请求重复办理。`);
      }

      pendingRequest = null;
      if (useWeighingStore.getState().capabilities?.userId === request.actorId) clearSample();
      updateData({ pending: undefined, draftRequestId: crypto.randomUUID() }); persist();
    }
    throw error;
  }
}

export const useWeighingStore = create<Store>((set, get) => ({
  data: empty(), device: null, capabilities: null, ready: false, loading: false, busy: false, storageError: '', syncNotice: '', lastResolution: null,
  loadTask: async id => {
    const token = useAuthStore.getState().token;
    const value = taskView(await workflowApi.task(id));
 if(isWorkbench&&value.scaleId!==selectedScaleId) throw new Error('该任务不属于当前地磅，请核对本厂任务与地磅归属');
 if(!isWorkbench) selectedScaleId=value.scaleId??0;
    const versions: WireTicket[] = [];
    for (let page = 1;; page++) {
      const batch = await workflowApi.get<ApiPageResult<WireTicket>>(`/tickets?taskId=${id}&page=${page}&pageSize=500`);
      versions.push(...batch.records);
      if (versions.length >= batch.total || !batch.records.length) break;
    }
    const tickets = versions.map(row => ticketView(row, versions.find(old => old.id === row.previousTicketId)));
    if (useAuthStore.getState().token === token) set(s => ({ data: { ...s.data, tasks: [...s.data.tasks.filter(t => t.id !== value.id), value], tickets: [...s.data.tickets.filter(t => t.taskId !== value.id), ...tickets] } }));
    return value;
  },
  loadTicket: async id => {
    const token = useAuthStore.getState().token;
    const value = ticketView(await workflowApi.ticket(id));
    if (useAuthStore.getState().token === token) set(s => ({ data: { ...s.data, tickets: [...s.data.tickets.filter(t => t.id !== value.id), value] } }));
  },
  loadPreview: async id => {
    const token = useAuthStore.getState().token;
    const value = ticketView(await workflowApi.get<WireTicket>(`/tickets/${id}/preview`));
    if (useAuthStore.getState().token === token) set(s => ({ data: { ...s.data, tickets: [...s.data.tickets.filter(t => t.id !== value.id), value] } }));
  },
  locate: async plate => {
    const token = useAuthStore.getState().token;
    const wire = await workflowApi.active(plate,selectedScaleId||undefined);
    if (!wire) return undefined;
    const value = taskView(wire);
    if (useAuthStore.getState().token === token) set(s => ({ data: { ...s.data, tasks: [...s.data.tasks.filter(t => t.id !== value.id), value] } }));
    return value;
  },
  refresh: () => {
    const token = useAuthStore.getState().token;
    if (refreshPromise && refreshToken === token) return refreshPromise;
    refreshToken = token;
    if (!token) return Promise.resolve();
    set({ loading: true, storageError: '' });
    refreshPromise = (async () => {
      try {
        await useWeighingMasterStore.getState().load();
        const masters = useWeighingMasterStore.getState();
        if (masters.error || !masters.capabilities) throw new Error(masters.error || '业务权限读取失败');
        if (useAuthStore.getState().token !== token) return;
        const firstLoad = !get().ready;
        actorId = masters.capabilities.userId;
        const role = masters.capabilities.permissionCodes.includes('weighing:confirm') ? 'operator' : 'admin';
        set(s => ({ capabilities: masters.capabilities, data: { ...s.data, ...masters.data, config: { ...s.data.config, role } } }));
        if (firstLoad) restoreUI();
        if (masters.capabilities.permissionCodes.includes('weighing:read')) {
          const queryId = new URLSearchParams(location.search).get('task');
          const taskPath = location.pathname.match(/^\/weighing\/tasks\/(\d+)$/)?.[1];
          const ticketPath = location.pathname.match(/^\/weighing\/tickets\/(\d+)/)?.[1];
          const ids = new Set(get().data.tasks.map(t => t.id));
          if (queryId) ids.add(queryId); if (taskPath) ids.add(taskPath);
          for (const id of ids) await get().loadTask(id);
          if (ticketPath) await get().loadTicket(ticketPath);
          if (!plateError(get().data.draft.plate)) await get().locate(get().data.draft.plate);
          const scale=selectedScale(masters.capabilities);
          if(scale){
            const current=await workflowApi.get<WireSession>(`/scales/${scale.id}/session`);
            const active=!!current.holderUserId&&!!current.expiresAt&&Date.parse(current.expiresAt)>Date.now();
            const own=active&&current.holderUserId===actorId&&current.clientId===clientId;
            if(lease&&(!own||current.generation!==lease.generation)){lease=null;if(heartbeat)clearInterval(heartbeat);heartbeat=null;clearSample();}
            updateData({owner:active?own?'self':'other':null});
          }

        }
        if (useAuthStore.getState().token === token) set({ ready: true });
      } catch (error) { if (useAuthStore.getState().token === token) set({ storageError: error instanceof Error ? error.message : '业务加载失败' }); }
      finally { if (refreshToken === token) { set({ loading: false }); refreshPromise = null; } }
    })();
    return refreshPromise;
  },
  dispatch: async (command, expected) => {
    if (expected && expected !== get().data) {
      const current = get().data;
      // Live device updates and a refresh of the same version do not change a
      // reviewed correction. Compare the task/basis that this action actually uses.
      const ticketId = command.type === 'reopenTicket' ? command.ticketId : undefined;
      const previousTicket = expected.tickets.find(t => t.id === ticketId);
      const currentTicket = current.tickets.find(t => t.id === ticketId);
      const taskId = command.type === 'reweigh' ? command.taskId : previousTicket?.taskId;
      const beforeTask = expected.tasks.find(t => t.id === taskId);
      const afterTask = current.tasks.find(t => t.id === taskId);
      const sameTask = !!beforeTask && !!afterTask && beforeTask.version === afterTask.version;
      const sameReview = command.type === 'reopenTicket'
        ? sameTask && previousTicket?.status === currentTicket?.status
        : command.type === 'reweigh' && sameTask && !!current.capture && expected.capture?.id === current.capture.id && current.sampleReady;
      if (!sameReview) throw new Error('数据已变化，请重新采集并核对。');
    }
    const data = get().data;
    const mutation = !['draft', 'verifyPlate'].includes(command.type);
    if (get().busy && mutation) throw new Error('操作正在提交，请稍候');
    if (data.pending && !['resolveSubmission', 'retrySubmission'].includes(command.type)) throw new Error('上次提交结果待核实，请先查询原结果');
    if (mutation) set({ busy: true });
    try {
      switch (command.type) {
        case 'draft': {
          const changedPlate = command.patch.plate !== undefined && command.patch.plate !== data.draft.plate;
          if (changedPlate || (command.patch.direction && command.patch.direction !== data.draft.direction)) await invalidate();
          updateData({ draft: { ...get().data.draft, ...command.patch }, ...(changedPlate ? { draftPlateEvidence: undefined } : {}) }); persist(); break;
        }
        case 'verifyPlate': updateData({ draftPlateEvidence: command.evidence }); break;
        case 'clearDraft': await invalidate(); updateData({ draft: emptyTaskInput(), draftRequestId: crypto.randomUUID(), draftPlateEvidence: undefined }); persist(); await release(); break;
        case 'occupy': if (command.owner === 'other') throw new Error('请用另一个真实工作台取得操作权，不能伪造其他会话。'); if (command.owner === null) { await invalidate(); await release(); } else await acquire(); break;
        case 'takeover': await acquire('takeover', command.reason); clearSample(); break;
        case 'capture': {
          await ensureLease();
          const target = command.stage ? [...(data.tasks.find(t => t.id === command.taskId)?.records ?? [])].reverse().find(r => r.stage === command.stage) : undefined;
          const value = await workflowApi.post<WireSample>('/samples', { scaleId: lease!.scaleId, ...sessionInput(), taskId: command.taskId ? Number(command.taskId) : null, targetRecordId: target ? Number(target.id) : null, plateEvidence: command.plateEvidence ?? null, business: businessInput(data.draft) });
          if (!get().data.config.online || !get().data.config.stable || Number(value.weight) !== get().data.config.weight) {
            await workflowApi.post(`/samples/${value.id}/invalidate`, sessionInput());
            throw new Error('采集期间条件已变化，请重新采集并核对。');
          }
          sample = value;
          set({ syncNotice: '' });
          updateData({ sampleReady: true, capture: { ...value.evidence, id: String(value.id), stage: value.stage, weight: Number(value.weight), plateEvidence: value.plateEvidence } });
          break;
        }
        case 'firstWeigh': {
          if (!command.checked || !sample || !lease?.sessionToken) throw new Error('请重新采集并完成人工核对');
          const body = { scaleId: lease.scaleId, ...sessionInput(), business: businessInput(command.input), sampleId: sample.id, requestId: command.requestId, checks, reviewedVersions: { material: data.materials.find(m => m.id === command.input.materialId)?.version ?? 0, party: data.parties.find(p => p.id === command.input.partyId)?.version ?? 0, carrier: data.parties.find(p => p.id === command.input.carrierId)?.version ?? 0 } };
          await send({ actorId, path: '/tasks/first-confirmation', body, requestId: command.requestId, stage: 'first' }); break;
        }
        case 'confirm': {
          if (!command.checked || !sample || !lease?.sessionToken) throw new Error('请重新采集并完成人工核对');
          const current = data.tasks.find(t => t.id === command.taskId);
          const requestId = crypto.randomUUID();
          const body = { scaleId: lease.scaleId, ...sessionInput(), sampleId: sample.id, version: current?.version, requestId, checks, ...(command.finalWeights ? { finalWeights: { gross: String(command.finalWeights.gross), tare: String(command.finalWeights.tare), remark: command.finalWeights.reason ?? '' } } : {}) };
          await send({ actorId, path: `/tasks/${command.taskId}/second-confirmation`, body, requestId, taskId: command.taskId, stage: 'second' }); break;
        }
        case 'correctTask':
        case 'terminate': {
          const current = data.tasks.find(t => t.id === command.taskId);
          const requestId = crypto.randomUUID();
          const body = { requestId, version: current?.version, reason: command.reason, ...(command.type === 'correctTask' ? { business: businessInput(command.input) } : {}) };
          await send({ actorId, path: `/tasks/${command.taskId}/${command.type === 'correctTask' ? 'corrections' : 'termination'}`, body, requestId, taskId: command.taskId, stage: 'second', operation: command.type === 'correctTask' ? 'correct' : 'terminate' }); break;
        }
        case 'voidTicket': {
          const current = data.tickets.find(t => t.id === command.ticketId);
          if (!current) throw new Error('请先刷新单据详情');
          const requestId = crypto.randomUUID();
          await send({ actorId, path: `/tickets/${command.ticketId}/void`, body: { requestId, reason: command.reason }, requestId, taskId: current.taskId, stage: 'second', operation: 'void' }); break;
        }
        case 'reopenTicket': {
          const previous = data.tickets.find(t => t.id === command.ticketId);
          const current = data.tasks.find(t => t.id === previous?.taskId);
          if (!previous || !current) throw new Error('请先刷新原任务与单据');
          const requestId = crypto.randomUUID();
          const body = { requestId, taskVersion: current.version, reason: command.reason, business: businessInput(command.input), basisValid: command.evidenceValid, ...(command.finalWeights ? { finalWeights: { gross: String(command.finalWeights.gross), tare: String(command.finalWeights.tare), remark: command.finalWeights.reason ?? '' } } : {}) };
          await send({ actorId, path: `/tickets/${command.ticketId}/reopen`, body, requestId, taskId: current.id, stage: 'second', operation: 'reopen' }); break;
        }
        case 'reweigh': {
          const current = data.tasks.find(t => t.id === command.taskId);
          if (!current || !sample || !lease?.sessionToken || !command.checked || command.captureId !== String(sample.id)) throw new Error('请重新采集并人工核对目标称重');
          const target = [...current.records].reverse().find(r => r.stage === command.stage);
          if (!target) throw new Error('未找到当前有效的目标称重');
          const requestId = crypto.randomUUID();
          const body = { requestId, version: current.version, scaleId: lease.scaleId, ...sessionInput(), sampleId: sample.id, targetRecordId: Number(target.id), checks, sameLoadConfirmed: command.sameLoad, otherBasisValid: command.otherValid ?? false, reason: command.reason };
          await send({ actorId, path: `/tasks/${command.taskId}/reweighs`, body, requestId, taskId: current.id, stage: command.stage ?? 'first', operation: 'reweigh' }); break;
        }
        case 'previewPrint':
        case 'startPrint': {
          const current = data.tickets.find(t => t.id === command.ticketId);
          if (!current) throw new Error('请先刷新单据');
          const requestId = crypto.randomUUID();
          const intent = command.type === 'startPrint' ? command.intent : current.printStatus === 'failed' ? 'retry' : 'initial';
          const body = { requestId, intent, reason: command.type === 'startPrint' ? command.reason ?? '' : '' };
          await send({ actorId, path: `/tickets/${command.ticketId}/print-attempts`, body, requestId, taskId: current.taskId, stage: 'second', operation: 'print' }); break;
        }
        case 'resolvePrint': {
          const current = data.tickets.find(t => t.id === command.ticketId);
          if (!current?.lastPrintAttemptId) throw new Error('没有可核实的打印尝试，请刷新单据');
          const requestId = crypto.randomUUID();
          const body = { requestId, status: command.status ?? (command.printed ? 'printed' : 'failed'), reason: command.reason };
          await send({ actorId, path: `/print-attempts/${current.lastPrintAttemptId}/result`, body, requestId, taskId: current.taskId, stage: 'second', operation: 'print' }); break;
        }
        case 'resolveSubmission': {
          if (!pendingRequest) throw new Error('没有本账号的待决请求');
          const request = pendingRequest;
          const result = await workflowApi.operation(request.requestId);
          if (result.status === 'not_found') throw new Error('尚未查到已提交结果，可能仍在处理。请稍后查询，或明确使用原请求重试；不能新建请求重复确认。');
          if (result.status === 'rejected') { pendingRequest = null; clearSample(); updateData({ pending: undefined, draftRequestId: crypto.randomUUID() }); persist(); set({ lastResolution: { committed: false, taskId: request.taskId ?? '' } }); throw new Error(`此前请求已被拒绝：${result.error}。请重新采集并核对。`); }
          if (!result.result) throw new Error('服务端结果尚不可用');
          await finishResult(result.result, request); break;
        }
        case 'retrySubmission': if (!pendingRequest) throw new Error('没有本账号的待决请求'); else await send(pendingRequest, true); break;
        default: throw new Error('此动作的服务端接入尚未完成，请勿按旧演示结果办理。');
      }
    } finally { if (mutation) set({ busy: false }); }
  },
}));
useAuthStore.subscribe((state, previous) => {
  if (state.token === previous.token) return;
  lease = null; clearSample(); pendingRequest = null; actorId = 0;
  if (heartbeat) { clearInterval(heartbeat); heartbeat = null; }
  useWeighingStore.setState({ data: empty(), device: null, ready: false, capabilities: null, storageError: '', syncNotice: '' });
  if (state.token) void useWeighingStore.getState().refresh();
});
if (useAuthStore.getState().token) queueMicrotask(() => { void useWeighingStore.getState().refresh(); });

let pollingDevice = false;
setInterval(() => {
 const state = useWeighingStore.getState();
 const token = useAuthStore.getState().token;
 const scale = selectedScale(state.capabilities);
 if (!token || !scale || pollingDevice || !state.capabilities?.permissionCodes.includes('weighing:read')) return;
 pollingDevice = true;
 void workflowApi.get<DeviceState>(`/scales/${scale.id}/device`).then(device => {
  if (useAuthStore.getState().token !== token) return;
  const previous = useWeighingStore.getState().device;
  const meaningful = !previous || previous.online !== device.online || previous.revision !== device.revision || previous.capturePolicyRevision !== device.capturePolicyRevision || previous.captureRequired !== device.captureRequired || previous.captureBlockedReason !== device.captureBlockedReason;
  useWeighingStore.setState({ device });
  const policyChanged = !!sample && sample.evidence.capturePolicyRevision !== device.capturePolicyRevision;
  if (sample && (device.captureBlockedReason || !device.online || !device.stable || !device.deviceOk || (previous && previous.revision !== device.revision) || policyChanged)) {
    clearSample();
    if (policyChanged) useWeighingStore.setState({ syncNotice: '强制抓拍配置已变更，请重新采集并核对。' });
  }
  if (!meaningful) return;
  const config = useWeighingStore.getState().data.config;
  updateData({ config: { ...config, weight: Number(device.weight), stable: device.stable, online: device.online, timedOut: !device.online, deviceError: !device.deviceOk, evidence: device.captureRequired === false || useWeighingStore.getState().data.capture?.imageStatus === 'complete' } });
 }).catch(() => {
  if (useAuthStore.getState().token !== token) return;
  clearSample(); const config = useWeighingStore.getState().data.config;
  if (config.online) updateData({config:{...config,online:false,stable:false,timedOut:true}});
 }).finally(() => { pollingDevice = false; });
}, 500);
