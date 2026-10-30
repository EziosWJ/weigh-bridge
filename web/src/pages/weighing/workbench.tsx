import { useEffect, useRef, useState, type FormEvent } from 'react';
import { ClipboardCheck, FileText, LoaderCircle, Scale, Truck, TriangleAlert, X } from 'lucide-react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { ContentCard } from '@/components/common/content-card';
import { SectionTabs } from '@/components/common/section-tabs';
import { DetailItem } from '@/components/common/detail-item';
import { PageHeader } from '@/components/common/page-header';
import { StatusTag } from '@/components/common/status-tag';
import { HelpHint } from '@/components/common/help-hint';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/common/toast-store';
import { Dialog, DialogOverlay, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogClose, DialogBody, DialogFooter } from '@/components/ui/dialog';
import { PlateInput } from './plate-input';
import { normalizePlate, plateError } from '@/lib/weighing/plate';
import { beijingDate, beijingTime } from '@/lib/weighing/query';
import { useWeighingStore } from '@/store/weighing-store';
import { workflowApi, type PlateCandidate } from '@/api/weighing-workflow';
import { roleLabels, taskStatusLabels, type PlateEvidence, type NetAdjustmentInput, type TaskInput, type TransportTask, type WeightValues, type WeighingCommand, type WeighingState } from '@/types/weighing';
import { useTransportDirections } from '@/hooks/use-transport-directions';
import { sameWeighingReview } from '@/lib/weighing/review';
import { formatWeight, formatWeightValue, weightValues } from '@/lib/weighing/weights';
import { BusinessDirectionControl, BusinessRequiredFields, BusinessSupplementaryFields } from './shared';
import { WeighingReviewDialog, type WeighingReviewValue } from './review-dialog';
import { DeviceCameraPanel, type CameraSize } from './camera-panel';
import { TicketPrintActions } from './ticket-print-actions';
import './workbench.css';

const normalizeLegacy = (value: string) => value.trim().toUpperCase();
const sourceCameraNames = (item: PlateCandidate) => [...new Set(item.sourceEvents.map(source => source.cameraName).filter(Boolean))].join('、');
const clockTime = (value: string) => beijingTime(value).slice(11, 19);

type WorkbenchStatus = { tone: 'success' | 'warning' | 'error' | 'info' | 'neutral'; label: string; detail: string; next: string };

function reviewTitle(latest: WeighingState, taskId?: string) {
  const task = taskId ? latest.tasks.find(item => item.id === taskId) : undefined;
  const stage = task?.status === 'second' ? 'second' : 'first';
  const direction = task?.direction ?? latest.draft.direction;
  const kind = (direction === 'outbound') === (stage === 'first') ? '皮重' : '毛重';
  return `${stage === 'first' ? '首磅' : '回磅'}（${kind}）`;
}

function expectedWeights(records: TransportTask['records'], capture?: { weight: number }): WeightValues | undefined {
  const first = records.filter(record => record.stage === 'first').slice(-1)[0];
  if (!first || !capture) return undefined;
  try {
    return weightValues(first.kind === 'gross' ? first.weight : capture.weight, first.kind === 'tare' ? first.weight : capture.weight);
  } catch {
    return undefined;
  }
}

export function WeighingWorkbenchPage() {
  const { ready, storageError, refresh } = useWeighingStore();
  if (!ready) return <><PageHeader title="称重工作台" /><p role="status">{storageError || '正在加载服务端资料与权限…'}</p>{storageError && <Button onClick={() => void refresh()}>重新加载</Button>}</>;
  return <WorkbenchContent />;
}

function WorkbenchContent() {
  const directionLabels = useTransportDirections();
  const navigate = useNavigate();
  const { data, dispatch, storageError, syncNotice, capabilities, busy, device } = useWeighingStore();
  const plateInput = useRef<HTMLInputElement>(null);
  const [businessOpen, setBusinessOpen] = useState(false);
  const [params, setSearchParams] = useSearchParams();
  const setParams = (next: Record<string, string>) => setSearchParams({ scale: capabilities?.localScaleCode || params.get('scale') || '', ...next });
  const location = useLocation();
  const linkedTask = data.tasks.find((task) => task.id === params.get('task'));
  const draftTask = data.tasks.find((task) => task.plate === normalizeLegacy(data.draft.plate) && (task.status === 'first' || task.status === 'second'));
  const [plate, setPlate] = useState(linkedTask?.plate ?? data.draft.plate);
  const [searchedPlate, setSearchedPlate] = useState(normalizeLegacy(linkedTask?.plate ?? (data.firstRequests[data.draftRequestId] && !draftTask ? '' : data.draftPlateEvidence?.confirmedPlate ?? '')));
  const [lookingUp, setLookingUp] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [reviewError, setReviewError] = useState('');
  const [cameraSize, setCameraSize] = useState<CameraSize>('standard');
  const [review, setReview] = useState<{ data: WeighingState; title: string } | null>(null);
  const [success, setSuccess] = useState<{ taskId: string; stage: 'first' | 'second' } | null>(null);
  const initialAgentPlate = device?.plate?.at && !device.plate.at.startsWith('0001') && !device.plate.failed ? normalizePlate(device.plate.candidate) : '';
  const [recognition, setRecognition] = useState<{ scenePlate: string; candidate?: string } | null>(initialAgentPlate ? { scenePlate: '', candidate: initialAgentPlate } : null);
  const [recognitionHint, setRecognitionHint] = useState(initialAgentPlate ? `识别候选 ${initialAgentPlate}，请人工核对` : '');
  const observedAt = useRef(device?.plate?.at ?? '');
  const observedInitialized = useRef(!!device?.plate?.at && !device.plate.at.startsWith('0001'));
  const [offerManualSource, setOfferManualSource] = useState(false);
  const editing = useRef(!!linkedTask || Object.entries(data.draft).some(([key, value]) => key === 'direction' ? value !== 'inbound' : !!value));
  const [candidates, setCandidates] = useState<PlateCandidate[]>([]);
  const selectedCandidate = useRef<PlateCandidate | null>(null);
  const candidateOwner = useRef<'empty' | 'suggested' | 'manual' | 'confirmed'>(editing.current ? 'manual' : 'empty');
  const candidateEpoch = useRef(0);
  const [candidateCycle, setCandidateCycle] = useState(0);
  const located = !!searchedPlate && searchedPlate === normalizeLegacy(plate) && (!linkedTask || linkedTask.plate === searchedPlate);
  const selected = linkedTask ?? (located ? data.tasks.find((task) => task.plate === searchedPlate && (task.status === 'first' || task.status === 'second')) : undefined);
  const awaitingResult = !!data.pending;
  const firstRecord = selected?.records.filter((record) => record.stage === 'first').slice(-1)[0];
  const successfulTask = (success && data.tasks.find((task) => task.id === success.taskId)) || (selected?.status === 'completed' ? selected : undefined);
  const successfulStage = success?.stage ?? 'second';
  const ticket = successfulTask && data.tickets.find((item) => item.taskId === successfulTask.id && item.status === 'valid');
  const stage = selected?.status === 'second' ? 'second' : 'first';
  const direction = selected?.direction ?? data.draft.direction;
  const kind = (direction === 'outbound') === (stage === 'first') ? '皮重' : '毛重';
  const canWeigh = !success && !awaitingResult && located && (!selected || selected.status === 'first' || selected.status === 'second');
  const returnTo = `${location.pathname}${location.search}`;
  const material = data.materials.find(item => item.id === data.draft.materialId);
  const party = data.parties.find(item => item.id === data.draft.partyId);
  const taskInfo = successfulTask || selected;
  const summaryPlate = taskInfo?.plate || data.draft.plate || plate;
  const summaryDirection = taskInfo?.direction ?? data.draft.direction;
  const summaryMaterial = taskInfo ? [taskInfo.materialName, taskInfo.materialSpec, taskInfo.materialCode].filter(Boolean).join(' · ') : material ? [material.name, material.spec, material.code].filter(Boolean).join(' · ') : '未选择';
  const summaryParty = taskInfo ? [taskInfo.partyName, taskInfo.partyCode].filter(Boolean).join(' · ') : party ? `${party.name} · ${party.code}` : '未选择';
  const businessPartyLabel = summaryDirection === 'inbound' ? '供货方' : '收货方';
  const missingFields = !selected && located && !awaitingResult ? [
    !material?.enabled ? '物料' : '',
    !party?.enabled || !party.roles.includes(direction === 'inbound' ? 'supplier' : 'customer') ? businessPartyLabel : '',
  ].filter(Boolean) as string[] : [];
  const needsDayVerification = !!selected?.crossDayDoubt;
  const conditionIssue = !capabilities?.permissionCodes.includes('weighing:confirm') ? '当前账号没有称重确认权限' : data.owner === 'other' ? '现场正在由其他会话占用，请操作员接管' : !data.config.online ? 'Agent 离线或重量数据已过期' : data.config.deviceError ? '设备异常，请恢复后重新采集' : !data.config.stable ? '重量未稳定，请等待设备稳定' : !Number.isFinite(data.config.weight) || data.config.weight <= 0 ? '等待有效上磅重量' : '';
  const shouldVerifyDay = !conditionIssue && needsDayVerification;
  const businessBlocked = !capabilities?.permissionCodes.includes('weighing:confirm') ? '当前账号没有称重确认权限' : data.owner === 'other' ? '现场正在由其他会话占用，请操作员接管' : needsDayVerification ? '请先核实跨天运输，再继续称重' : '';
  const confirmedRecord = !awaitingResult ? successfulTask?.records.filter(record => record.stage === successfulStage).slice(-1)[0] : undefined;
  const sampleReady = data.sampleReady && data.capture?.plate === summaryPlate;
  const frozenCapture = !confirmedRecord && sampleReady ? data.capture : undefined;
  const shownWeight = confirmedRecord?.weight ?? frozenCapture?.weight ?? data.config.weight;
  const displayStage = data.pending?.stage ?? (successfulTask ? successfulStage : stage);
  const displayKind = confirmedRecord ? (confirmedRecord.kind === 'gross' ? '毛重' : '皮重') : (summaryDirection === 'outbound') === (displayStage === 'first') ? '皮重' : '毛重';
  const hasWeighingContext = located || !!taskInfo || awaitingResult;
  const blocked = businessBlocked || (!sampleReady ? conditionIssue || device?.captureBlockedReason : '') || '';
  const candidateBlocked = useRef(false);
  candidateBlocked.current = !!selected || located || !!data.capture || !!review || !!success || awaitingResult || lookingUp || submitting || businessOpen || data.owner === 'other';
  const candidateConflict = new Set(candidates.map(item => item.plate)).size > 1;
  // 候选标签已在车牌输入后列出，冲突时提示选择即可，不再重复选中的候选文字。
  const candidateNotice = candidateConflict && !located ? '存在多个候选，请核对后选择' : recognitionHint.startsWith('已选择 ') ? '' : recognitionHint;
  const scaleCode = capabilities?.localScaleCode || params.get('scale') || '';
  const canConfirm = !!capabilities?.permissionCodes.includes('weighing:confirm');
  const expected = stage === 'second' ? expectedWeights(selected?.records ?? [], data.capture) : undefined;
  const liveVisible = !!confirmedRecord || !!frozenCapture || (!!device && data.config.online && !data.config.deviceError);

  const status: WorkbenchStatus = (() => {
    if (awaitingResult) return { tone: 'warning', label: '提交结果待核实', detail: '上次提交结果未知，不能新建请求重复称重或出单。', next: '先查询原结果，再决定是否按原请求重试。' };
    if (successfulTask) return successfulStage === 'first'
      ? { tone: 'success', label: '首磅已完成', detail: '首次称重已保存，车辆等待回磅。', next: '装卸完成后按同一车牌确认车辆并办理回磅。' }
      : { tone: 'success', label: ticket ? '回磅已完成 · 已出单' : '回磅已完成', detail: ticket ? `正式单据已生成，最终净重 ${formatWeight(ticket.net)}。` : '第二次称重已保存。', next: '可打印单据、查看详情或办理下一辆车。' };
    if (!canConfirm) return { tone: 'error', label: '无称重操作权限', detail: '当前账号不能确认正式称重。', next: '请联系管理员分配称重权限。' };
    if (data.owner === 'other') return { tone: 'warning', label: '操作权被占用', detail: '现场正在由其他会话办理。', next: '由当前会话释放后接手，或填写原因后接管操作权。' };
    if (frozenCapture) return { tone: 'info', label: '已采集待确认', detail: `已冻结 ${formatWeight(frozenCapture.weight)} · ${beijingTime(frozenCapture.at)}${frozenCapture.captureRequired === false ? ' · 本次未强制抓拍' : frozenCapture.imageStatus === 'capture_failed' ? ' · 本次抓拍失败' : ''}${!data.config.online ? ' · 设备当前离线，冻结值仍可确认' : data.config.deviceError ? ' · 设备当前异常，冻结值仍可确认' : ''}。`, next: missingFields.length ? `补齐${missingFields.join('、')}后集中核对确认。` : '核对车牌、资料、重量与图片后确认；设备变化不影响本次冻结依据。' };
    if (data.config.deviceError) return { tone: 'error', label: '设备异常', detail: '仪表或采集链路报告故障，不能取得有效重量。', next: '检查现场 Agent 与仪表，恢复后重新采集。' };
    if (!data.config.online) return { tone: 'warning', label: '设备离线', detail: device?.receivedAt && !device.receivedAt.startsWith('0001') ? `最近上报 ${beijingTime(device.receivedAt)}；离线期间不把旧值当实时重量。` : '尚未收到 Agent 上报。', next: '检查现场 Agent 与网络，等待重量恢复上报。' };
    if (taskInfo && !canWeigh) return { tone: 'warning', label: '当前任务不能继续称重', detail: '任务已完成或已终止。', next: '查看任务详情与异常处置。' };
    if (!hasWeighingContext) return { tone: 'neutral', label: '等待确认车辆', detail: '识别候选或手工录入车牌后确认车辆，即可定位首磅或回磅。', next: '核对车牌后点击“确认车辆”。' };
    if (!Number.isFinite(data.config.weight) || data.config.weight <= 0) return { tone: 'info', label: '等待有效上磅', detail: '当前没有可用的上磅重量。', next: '请车辆完全上磅并等待设备读数。' };
    if (!data.config.stable) return { tone: 'warning', label: '重量未稳定', detail: `当前读数 ${formatWeight(data.config.weight)}，仍在变化。`, next: '等待稳定状态后再采集。' };
    if (busy && !sampleReady) return { tone: 'info', label: '采集中', detail: '正在冻结重量与现场抓拍。', next: '请保持车辆完全上磅，等待采集结果。' };
    if (!located) return { tone: 'neutral', label: '等待确认车辆', detail: '尚未定位本趟运输。', next: '核对车牌后点击“确认车辆”。' };
    return { tone: 'success', label: '稳定可采集', detail: `当前读数 ${formatWeight(data.config.weight)}，设备稳定。`, next: '确认车辆完全上磅后点击“采集重量”。' };
  })();

  // 快照保留冻结称重与图片；操作会话失效或资料变更后仍可查看原核对内容。
  const reviewTask = review && selected ? review.data.tasks.find(task => task.id === selected.id) ?? selected : undefined;
  const reviewCapture = review?.data.capture;
  const reviewSecond = reviewTask?.status === 'second';
  const reviewDirection = reviewTask?.direction ?? review?.data.draft.direction ?? data.draft.direction;
  const reviewKind = (reviewDirection === 'outbound') === (!reviewSecond) ? '皮重' : '毛重';
  const reviewFirstRecord = reviewTask?.records.filter(record => record.stage === 'first').slice(-1)[0];
  const reviewExpected = reviewSecond ? expectedWeights(reviewTask?.records ?? [], reviewCapture) : undefined;
  const reviewValue: WeighingReviewValue | undefined = review ? {
    plate: reviewTask?.plate || data.draft.plate || plate,
    direction: directionLabels[reviewDirection],
    material: reviewTask ? [reviewTask.materialName, reviewTask.materialSpec, reviewTask.materialCode].filter(Boolean).join(' · ') : material ? [material.name, material.spec, material.code].filter(Boolean).join(' · ') : '未选择',
    partyLabel: reviewDirection === 'inbound' ? '供货方' : '收货方',
    party: reviewTask ? [reviewTask.partyName, reviewTask.partyCode].filter(Boolean).join(' · ') : party ? `${party.name} · ${party.code}` : '未选择',
    weight: reviewCapture?.weight ?? 0,
    kind: reviewKind,
    capture: reviewCapture,
    firstWeight: reviewFirstRecord ? `${reviewFirstRecord.kind === 'gross' ? '毛重' : '皮重'} ${formatWeight(reviewFirstRecord.weight)} · ${beijingTime(reviewFirstRecord.at)}` : undefined,
    original: reviewSecond ? reviewExpected : undefined,
    expected: reviewExpected,
    supplementary: [
      { label: '承运方', value: reviewTask ? reviewTask.carrierName ?? '' : data.parties.find(item => item.id === data.draft.carrierId)?.name ?? '' },
      { label: '本趟联系人', value: [reviewTask ? reviewTask.contactName : data.draft.contactName, reviewTask ? reviewTask.contactPhone : data.draft.contactPhone].filter(Boolean).join(' / ') },
      { label: '司机', value: [reviewTask ? reviewTask.driver : data.draft.driver, reviewTask ? reviewTask.driverPhone : data.draft.driverPhone].filter(Boolean).join(' / ') },
      { label: '备注', value: reviewTask?.remark ?? data.draft.remark },
    ].filter(item => item.value),
  } : undefined;
  const reviewInvalidReason = !review || submitting
    ? ''
    : !sameWeighingReview(review.data, data, selected?.id) || !canWeigh || !!businessBlocked || !sampleReady
      ? syncNotice || businessBlocked || '核对资料或采集结果已变更，请返回重新核对。'
      : '';

  useEffect(() => {
    if (!scaleCode || !canConfirm) return;
    let stopped = false;
    const epochRef = candidateEpoch;
    let cursor: string | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let request: AbortController | undefined;
    let epoch = candidateEpoch.current;
    const reset = () => { cursor = undefined; epoch = ++candidateEpoch.current; };
    const visibility = () => { if (document.visibilityState === 'visible') reset(); else ++candidateEpoch.current; };
    document.addEventListener('visibilitychange', visibility);
    const poll = async () => {
      if (stopped) return;
      if (document.visibilityState === 'visible') {
        const requestEpoch = epoch;
        request = new AbortController();
        const timeout = setTimeout(() => request?.abort(), 5000);
        try {
          const page = await workflowApi.plateCandidates(scaleCode, cursor, request.signal);
          if (stopped || requestEpoch !== candidateEpoch.current) return;
          if (!Array.isArray(page.items) || typeof page.nextCursor !== 'string' || typeof page.serverTime !== 'string') throw new Error('识别候选响应无效');
          const initial = cursor === undefined;
          cursor = page.nextCursor;
          setCandidates(previous => {
            // 同一车牌只保留最近接收的候选项，避免分页或新批次重复占位。
            const combined = new Map(previous.map(item => [item.plate, item]));
            for (const item of page.items) {
              const existing = combined.get(item.plate);
              if (!existing || Date.parse(item.lastReceivedAt) >= Date.parse(existing.lastReceivedAt)) combined.set(item.plate, item);
            }
            return [...combined.values()].filter(item => Date.parse(item.expiresAt) > Date.parse(page.serverTime)).sort((a, b) => Date.parse(b.lastReceivedAt) - Date.parse(a.lastReceivedAt)).slice(0, 5);
          });
          if (!initial && page.items.length && !candidateBlocked.current && !useWeighingStore.getState().busy) {
            const candidate = page.items[page.items.length - 1];
            if (candidateOwner.current === 'suggested' && selectedCandidate.current?.plate === candidate.plate) {
              selectedCandidate.current = candidate;
              setRecognitionHint(`识别候选 ${candidate.plate} · ${sourceCameraNames(candidate)} · ${clockTime(candidate.sourceEvents.at(-1)?.recognizedAt ?? candidate.lastReceivedAt)} · 待人工核对`);
            } else if (candidateOwner.current === 'empty' && document.activeElement !== plateInput.current) {
              if (useWeighingStore.getState().data.owner === null) {
                try { await useWeighingStore.getState().dispatch({ type: 'occupy', owner: 'self' }); } catch { return; }
              }
              if (stopped || requestEpoch !== candidateEpoch.current || candidateBlocked.current || useWeighingStore.getState().busy || candidateOwner.current !== 'empty' || document.activeElement === plateInput.current || useWeighingStore.getState().data.owner !== 'self') return;
              candidateOwner.current = 'suggested'; selectedCandidate.current = candidate; editing.current = true;
              setPlate(candidate.plate);
              setRecognitionHint(`识别候选 ${candidate.plate} · ${sourceCameraNames(candidate)} · ${clockTime(candidate.sourceEvents.at(-1)?.recognizedAt ?? candidate.lastReceivedAt)} · 待人工核对`);
            }
          }
        } catch { /* Manual entry remains available; calibrate on reconnect. */ cursor = undefined; }
        finally { clearTimeout(timeout); request = undefined; if (!stopped) timer = setTimeout(poll, 2000); }
        return;
      }
      if (!stopped) timer = setTimeout(poll, 2000);
    };
    void poll();
    return () => { stopped = true; ++epochRef.current; request?.abort(); if (timer) clearTimeout(timer); document.removeEventListener('visibilitychange', visibility); };
  }, [scaleCode, canConfirm, candidateCycle]);
  useEffect(() => {
    const resume = (event: MessageEvent) => {
      if (event.origin !== window.location.origin || event.data?.type !== 'weighbridge-locate-task') return;
      if (success?.taskId === event.data.taskId) { setSuccess(null); setRecognition(null); setRecognitionHint(''); }
    };
    window.addEventListener('message', resume);
    return () => window.removeEventListener('message', resume);
  }, [success]);

  async function run(command: WeighingCommand, errorTitle: string | null = '操作失败') {
    try {
      await dispatch(command);
      return true;
    } catch (error) {
      const text = error instanceof Error ? error.message : '操作未完成，请核对输入和设备状态';
      if (errorTitle) toast.error({ title: errorTitle, description: text, duration: 5000 });
      return false;
    }
  }

  function warn(title: string, description?: string) {
    toast.warning({ title, description, duration: 4500 });
  }

  function patchDraft(patch: Partial<TaskInput>) {
    const hadCapture = !!useWeighingStore.getState().data.capture;
    void run({ type: 'draft', patch }).then(ok => {
      if (ok && hadCapture && patch.direction) toast.warning({ title: '运输方向已变更', description: '已采集重量已失效，请重新采集后确认。' });
    });
  }

  async function patchReview(patch: Partial<TaskInput>) {
    if (!await run({ type: 'draft', patch })) return;
    setReview(current => current ? { ...current, data: useWeighingStore.getState().data } : current);
  }

  async function lookup(event: FormEvent) {
    event.preventDefault();
    if (lookingUp) return;
    const normalized = normalizeLegacy(plate);
    if (!normalized) {
      warn('请先填写车牌', '输入普通或新能源车牌后再确认车辆。');
      plateInput.current?.focus();
      return;
    }
    const legacy = data.tasks.some(task => task.plate === normalized) || data.tickets.some(ticket => ticket.plate === normalized);
    if (!legacy && plateError(normalized)) {
      warn('车牌格式不正确', plateError(normalized));
      plateInput.current?.focus();
      return;
    }
    if (data.pending) {
      warn('上次提交结果待核实', '请先查询原结果，避免重复称重或出单。');
      return;
    }
    candidateBlocked.current = true;
    setLookingUp(true);
    try {
      let existing;
      try {
        existing = await useWeighingStore.getState().locate(normalized);
      } catch (error) {
        const text = error instanceof Error ? error.message : '服务端任务查找失败';
        toast.error({ title: '查找运输失败', description: text, duration: 5000 });
        return;
      }
      setPlate(normalized); setSuccess(null); setReview(null);
      if (data.firstRequests[data.draftRequestId] && !existing && !await run({ type: 'clearDraft' }, '车牌核对失败')) return;
      if (!await run({ type: 'draft', patch: { plate: normalized } }, '车牌核对失败')) return;
      const chosen = selectedCandidate.current;
      const evidence: PlateEvidence = chosen ? { source: 'server', candidate: chosen.plate, confirmedPlate: normalized, at: new Date().toISOString(), corrected: chosen.plate !== normalized, sourceEventRecordIds: chosen.sourceEvents.map(source => source.eventRecordId) } : { source: recognition?.candidate ? 'agent' : 'manual', scenePlate: recognition?.scenePlate, candidate: recognition?.candidate, confirmedPlate: normalized, at: new Date().toISOString() };
      if (!await run({ type: 'verifyPlate', evidence }, chosen ? null : '车牌核对失败')) {
        if (!chosen || !await run({ type: 'verifyPlate', evidence: { source: 'manual', confirmedPlate: normalized, at: new Date().toISOString() } })) return;
        selectedCandidate.current = null;
        setRecognitionHint('识别来源不可用，已按人工核对车牌办理。');
      }
      setSearchedPlate(normalized); editing.current = true; candidateOwner.current = 'confirmed';
      if (existing) {
        setParams({ task: existing.id });
        toast.success({ title: '已确认车辆', description: `${existing.taskNo ?? existing.id} · ${existing.plate}` });
      } else {
        setParams({});
        toast.success({ title: '已确认车辆', description: '暂无历史任务，请选择物料和供收方。' });
      }
    } finally {
      setLookingUp(false);
    }
  }

  function openReview() {
    const latest = useWeighingStore.getState().data;
    if (!latest.capture) return;
    setBusinessOpen(false);
    setReview({ data: latest, title: reviewTitle(latest, selected?.id) });
  }

  async function capture(repeat = false) {
    const ok = await run({
      type: 'capture',
      taskId: selected?.id,
      plate: summaryPlate,
      plateEvidence: data.draftPlateEvidence?.confirmedPlate === summaryPlate ? data.draftPlateEvidence : { source: 'manual', confirmedPlate: summaryPlate, at: new Date().toISOString() },
    }, repeat ? '重新采集失败' : '采集失败');
    if (!ok && data.draftPlateEvidence?.source === 'server') { setOfferManualSource(true); setRecognitionHint('采集未完成；若识别来源已失效，请重新核对车牌或改为人工来源。'); }
    if (ok) {
      toast.success({
        title: repeat ? '已重新采集' : '重量已采集',
        description: '已冻结重量与现场图片，直接进入集中核对。',
      });
      openReview();
    }
  }

  function recapture() {
    setReview(null);
    setReviewError('');
    void capture(true);
  }

  async function confirm(reviewed: WeighingState, netAdjustment?: NetAdjustmentInput) {
    if (!sameWeighingReview(reviewed, useWeighingStore.getState().data, selected?.id)) {
      setReview(null);
      setReviewError('');
      warn('核对内容已变化', '请返回核对当前资料后再提交。');
      return;
    }
    const checked = true;
    const command: WeighingCommand = selected
      ? { type: 'confirm', taskId: selected.id, stage, checked, netAdjustment }
      : { type: 'firstWeigh', input: data.draft, checked, requestId: data.draftRequestId };
    setReviewError('');
    setSubmitting(true);
    try {
      await dispatch(command, reviewed);
    } catch (error) {
      const text = error instanceof Error ? error.message : '称重保存失败，请核对输入和设备状态';
      const pending = useWeighingStore.getState().data.pending;
      if (pending) {
        // A network/server failure can leave the request in an unknown state.
        // Close the review so the operator can reach "查询原结果" instead of
        // retrying a submission that the store will reject as pending.
        setReview(null);
        setReviewError('');
        toast.warning({ title: '提交结果待核实', description: '保存结果未知，请先查询原结果，避免重复称重或出单。', duration: 6000 });
        return;
      }
      setReviewError(text);
      toast.error({ title: '称重保存失败', description: text, duration: 5000 });
      return;
    } finally {
      setSubmitting(false);
    }
    const next = useWeighingStore.getState().data;
    const taskId = selected?.id ?? next.firstRequests[data.draftRequestId];
    setReview(null);
    setReviewError('');
    if (taskId) setParams({ task: taskId });
    if (next.pending) {
      toast.warning({ title: '提交结果待核实', description: '请求已发出但结果未知，请先查询原结果，避免重复称重或出单。', duration: 6000 });
    } else if (taskId) {
      setSuccess({ taskId, stage }); setRecognition(null); setRecognitionHint('');
      const savedTicket = next.tickets.find(item => item.taskId === taskId && item.status === 'valid');
      toast.success({
        title: stage === 'first' ? '首磅已保存' : '回磅已保存',
        description: savedTicket ? `已出单，最终净重 ${formatWeight(savedTicket.net)}。` : '首次称重已保存，装卸完成后按车牌办理回磅。',
      });
    } else {
      toast.success('称重已提交');
    }
  }

  async function resolve() {
    const pending = data.pending;
    if (!pending) return;
    try {
      await dispatch({ type: 'resolveSubmission' });
    } catch (error) {
      const text = error instanceof Error ? error.message : '请稍后重试';
      toast.error({ title: '查询原结果失败', description: `${text}；不要新建请求重复办理。`, duration: 5000 });
      return;
    }
    const next = useWeighingStore.getState().data;
    const resolved = useWeighingStore.getState().lastResolution;
    const taskId = resolved?.taskId || pending.taskId || (pending.requestId ? next.firstRequests[pending.requestId] : '');
    if (pending.operation) {
      toast.success({ title: '已查询原结果', description: resolved?.committed ? '沿用原提交结果，无需重复办理。' : '未查到成功结果，请按页面指引继续。' });
      const current = (pending.operation === 'print' || (pending.operation === 'reopen' && resolved?.committed)) ? next.tickets.find(ticket => ticket.taskId === taskId && ticket.status === 'valid') : undefined;
      navigate(current ? `/weighing/tickets/${encodeURIComponent(current.id)}` : `/weighing/tasks/${encodeURIComponent(taskId)}`);
      return;
    }
    if (resolved?.committed && taskId) {
      setParams({ task: taskId });
      setSuccess({ taskId, stage: pending.stage });
      toast.success({ title: '已查询到原成功结果', description: '沿用原记录，无需重复提交。' });
    } else {
      setSuccess(null);
      toast.warning({ title: '原提交未成功', description: '草稿已保留，请重新采集并核对后提交。', duration: 5500 });
    }
  }

  async function retry() {
    const pending = data.pending;
    if (!pending || !await run({ type: 'retrySubmission' }, '重试失败')) return;
    const result = useWeighingStore.getState().lastResolution;
    if (!result?.committed || !result.taskId) {
      toast.warning({ title: '仍未确认成功', description: '请勿重复提交，按页面提示继续核实原请求。', duration: 5500 });
      return;
    }
    if (pending.operation) {
      toast.success({ title: '已按原请求恢复', description: '沿用服务端记录，无需重复办理。' });
      navigate(pending.operation === 'print' ? `/weighing/tickets/${useWeighingStore.getState().data.tickets.find(t => t.taskId === result.taskId && t.status === 'valid')?.id}` : `/weighing/tasks/${result.taskId}`);
      return;
    }
    setParams({ task: result.taskId }); setSuccess({ taskId: result.taskId, stage: pending.stage });
    toast.success({ title: '已按原请求恢复', description: '沿用服务端记录，无需重复称重。' });
  }

  async function nextVehicle() {
    if (!await run({ type: 'clearDraft' }, '结束当前车辆失败')) return;
    ++candidateEpoch.current; setCandidateCycle(value => value + 1); setCandidates([]); selectedCandidate.current = null; candidateOwner.current = 'empty';
    observedAt.current = useWeighingStore.getState().device?.plate?.at ?? '';
    setRecognition(null); setRecognitionHint(''); setOfferManualSource(false); editing.current = false;
    setParams({}); setPlate(''); setSearchedPlate(''); setSuccess(null);
    toast.success('已结束当前车辆，可继续办理下一辆。');
  }

  async function changePlate(value: string) {
    candidateOwner.current = 'manual';
    editing.current = true; setSearchedPlate('');
    setPlate(value); setParams({}); setSuccess(null); setReview(null);
    if (data.firstRequests[data.draftRequestId] && !await run({ type: 'clearDraft' })) return;
    void run({ type: 'draft', patch: { plate: value } });
  }

  useEffect(() => {
    const observed = device?.plate;
    if (!observed) return;
    if (!observed.at || observed.at.startsWith('0001')) { observedInitialized.current = true; return; }
    if (observedAt.current === observed.at) return;
    observedAt.current = observed.at;
    const candidate = observed.failed ? '' : normalizePlate(observed.candidate);
    setRecognition({ scenePlate: '', candidate });
    if (!observedInitialized.current) { observedInitialized.current = true; setRecognitionHint(candidate ? `识别候选 ${candidate}，请人工核对` : '识别失败，请人工录入并核对车牌。'); return; }
    if (editing.current || candidateBlocked.current || document.activeElement === plateInput.current) { setRecognitionHint(`新识别结果：${candidate || '识别失败'}，请核对当前车辆。`); return; }
    if (candidate) {
      editing.current = true; candidateOwner.current = 'manual';
      setPlate(candidate);
      void useWeighingStore.getState().dispatch({ type: 'draft', patch: { plate: candidate } }).catch(() => {});
      setRecognitionHint(`识别候选 ${candidate}，待人工核对`);
    } else setRecognitionHint('识别失败，请人工录入并核对车牌。');
  }, [device?.plate]);

  return <div className="weighing-workbench">
    <div className="workbench-page-heading"><PageHeader title="称重工作台" actions={<><StatusTag tone={data.config.online ? 'success' : 'warning'}>{data.config.online ? 'Agent 在线' : 'Agent 离线'}</StatusTag><span className="hidden text-sm text-text-tertiary sm:inline">{roleLabels[data.config.role]}</span></>} /></div>
    {(storageError || syncNotice) && <div role="status" className="workbench-notice rounded-control border border-border bg-neutral-background px-4 py-2 text-sm">{syncNotice && <p>{syncNotice}</p>}{storageError && <p className="text-error">{storageError}</p>}</div>}
    <div className={`weighing-station weighing-camera-${cameraSize}`}>
      <DeviceCameraPanel scaleCode={capabilities?.localScaleCode || params.get('scale') || ''} scenePlate={confirmedRecord?.capture?.plateEvidence?.scenePlate ?? recognition?.scenePlate ?? data.draftPlateEvidence?.scenePlate} size={cameraSize} onSizeChange={setCameraSize} capture={confirmedRecord?.capture ?? (sampleReady ? data.capture : undefined)} saved={!!confirmedRecord} />
      <div className="weighing-console min-w-0 space-y-4">
        <ContentCard className="weighing-vehicle-bar shadow-none" bodyClassName="px-4 py-3">
          <div className="workbench-current-vehicle"><h2>当前车辆</h2></div>
          <form className="flex flex-wrap items-center gap-3" onSubmit={lookup}>
            <label className="flex items-center gap-2 text-sm font-medium" htmlFor="workbench-plate"><Truck className="h-5 w-5 text-text-tertiary" aria-hidden />车牌<span className="text-error">*</span></label>
            <div className="plate-field">
              <PlateInput inputRef={plateInput} id="workbench-plate" value={plate} legacyValue={data.tasks.find(task => task.plate === plate)?.plate} disabled={awaitingResult} clearable onChange={changePlate} />
              {candidates.length > 0 && <ul className="plate-candidates" aria-label="识别候选">
                {candidates.map(item => <li key={item.id}><Button size="sm" type="button" aria-pressed={plate === item.plate} className="plate-candidate" disabled={candidateBlocked.current || busy} title={`${item.plate} · ${sourceCameraNames(item)} · ${beijingTime(item.sourceEvents.at(-1)?.recognizedAt ?? item.lastReceivedAt)}`} onClick={() => {
                  candidateOwner.current = 'manual'; selectedCandidate.current = item;
                  setRecognitionHint(`已选择 ${item.plate} · ${sourceCameraNames(item)} · 待人工核对`);
                  void changePlate(item.plate);
                }}>{item.plate} · {sourceCameraNames(item)} · {clockTime(item.sourceEvents.at(-1)?.recognizedAt ?? item.lastReceivedAt)}</Button></li>)}
              </ul>}
            </div>
            <Button type="submit" size="lg" variant="primary" disabled={awaitingResult || lookingUp} aria-busy={lookingUp}>{lookingUp ? <LoaderCircle className="h-5 w-5 animate-spin" aria-hidden /> : null}{lookingUp ? '正在确认…' : '确认车辆'}</Button>
          </form>
          {candidateNotice && <p role="status" className="plate-recognition mt-2 text-sm text-warning"><TriangleAlert className="h-4 w-4 shrink-0" aria-hidden />{candidateNotice}</p>}
          {offerManualSource && data.draftPlateEvidence?.source === 'server' && !data.sampleReady && !awaitingResult && <Button type="button" size="sm" disabled={busy} onClick={() => {
            selectedCandidate.current = null;
            setRecognition(null);
            void run({ type: 'verifyPlate', evidence: { source: 'manual', confirmedPlate: searchedPlate, at: new Date().toISOString() } }).then(ok => { if (ok) { setOfferManualSource(false); setRecognitionHint('已改为人工录入来源，请核对现场车辆。'); } });
          }}>改为人工录入来源</Button>}
          {(taskInfo || awaitingResult || located) && <div className="workbench-business-inline" aria-label="本趟运输资料">
            {(taskInfo || awaitingResult) && <div className="workbench-business-inline-head">
              <h3>本趟运输资料</h3>
              <StatusTag tone={taskInfo?.status === 'completed' ? 'success' : 'info'}>{awaitingResult ? '提交待核实' : taskStatusLabels[taskInfo!.status]}</StatusTag>
            </div>}
            {taskInfo || awaitingResult ? <>
              <div className="workbench-inline-facts">
                <div className="workbench-inline-fact"><span>运输方向</span><strong>{directionLabels[summaryDirection]}</strong></div>
                <div className="workbench-inline-fact"><span>物料</span><strong title={summaryMaterial}>{summaryMaterial}</strong></div>
                <div className="workbench-inline-fact"><span>{businessPartyLabel}</span><strong title={summaryParty}>{summaryParty}</strong></div>
              </div>
              {firstRecord && stage === 'second' && !(successfulTask && successfulStage === 'first') && <p className="workbench-first-basis">首次依据：{firstRecord.kind === 'gross' ? '毛重' : '皮重'} <strong>{formatWeight(firstRecord.weight)}</strong> · {beijingTime(firstRecord.at)}{beijingDate(firstRecord.at) !== beijingDate(new Date().toISOString()) ? ' · 隔日回磅沿用原任务' : ''}{firstRecord.replaces ? ` · 已重称替代原记录（${firstRecord.reason}）` : ''}</p>}
              <div className="workbench-inline-actions">
                <Button size="sm" variant="secondary" onClick={() => setBusinessOpen(true)}><FileText className="h-4 w-4" aria-hidden />查看完整资料</Button>
                {taskInfo && <Link target="weighbridge-management" className="text-sm text-primary hover:underline" to={`/weighing/tasks/${taskInfo.id}`}>任务详情与纠错</Link>}
              </div>
            </> : located ? <>
              <SectionTabs label="本趟运输资料" items={[
                { value: 'basic', label: '基本信息', content: <div className="workbench-business-inline-grid">
                  <BusinessDirectionControl value={data.draft} update={patchDraft} variant="simple" />
                  <BusinessRequiredFields value={data.draft} update={patchDraft} compact />
                </div> },
                { value: 'supplementary', label: '补充信息（选填）', content: <>
                  <div className="workbench-extra-grid"><BusinessSupplementaryFields value={data.draft} update={patchDraft} /></div>
                  <p className="workbench-inline-links"><Link target="weighbridge-management" className="text-primary hover:underline" to={`/weighing/materials?returnTo=${encodeURIComponent(returnTo)}`}>维护物料</Link><Link target="weighbridge-management" className="text-primary hover:underline" to={`/weighing/parties?returnTo=${encodeURIComponent(returnTo)}`}>维护往来主体</Link></p>
                </> },
              ]} />
            </> : null}
          </div>}
        </ContentCard>
        <ContentCard className="weighing-scale shadow-none" bodyClassName="p-4">
          <div className="workbench-weighing-heading">
            <h2 className="workbench-weighing-title" aria-label={hasWeighingContext ? `${displayStage === 'first' ? '首磅' : '回磅'}（${displayKind}）` : '待定位车辆'}>
              {hasWeighingContext ? <><span className="workbench-stage-label">{displayStage === 'first' ? '首磅' : '回磅'}</span><span className="workbench-weight-kind">{displayKind}</span></> : <span className="workbench-context-empty">待定位车辆</span>}
            </h2>
            <StatusTag tone={confirmedRecord ? 'info' : frozenCapture ? 'info' : !data.config.online || data.config.deviceError ? 'warning' : data.config.stable ? 'success' : 'warning'}>{confirmedRecord ? '已确认' : frozenCapture ? '已采集' : !data.config.online ? '设备离线' : data.config.deviceError ? '设备异常' : data.config.stable ? '稳定' : '未稳定'}</StatusTag>
          </div>
          {(!successfulTask || awaitingResult) && status.label !== '等待确认车辆' && status.label !== '稳定可采集' && !(frozenCapture && status.label === '已采集待确认' && data.config.online && !data.config.deviceError && frozenCapture.imageStatus !== 'capture_failed') && <div className={`workbench-status is-${status.tone}`} role="status" aria-live="polite">
            <strong>{status.label}</strong>
            <span>{status.detail}</span><span className="workbench-status-next">下一步：{status.next}</span>
          </div>}
          <div className="weighing-instrument">
            <div className="space-y-2"><p className="instrument-label min-w-0 text-sm text-text-secondary"><Truck className="h-8 w-8" aria-hidden />{confirmedRecord ? '本次已确认重量' : frozenCapture ? '已采集重量（已冻结）' : '当前设备重量'}</p>{sampleReady && !awaitingResult && !successfulTask && <Button disabled={busy} aria-busy={busy} onClick={recapture}>{busy ? '正在采集…' : '重新采集重量'}</Button>}</div>
            <p className="weighing-readout" aria-label="当前重量">{liveVisible && Number.isFinite(shownWeight) ? formatWeightValue(shownWeight) : '—'}<span>吨</span></p>
          </div>
          <p className="instrument-source text-xs text-text-tertiary">{confirmedRecord ? `采集时间：${beijingTime(confirmedRecord.at)}` : frozenCapture ? `采集时间：${beijingTime(frozenCapture.at)}` : device?.receivedAt && !device.receivedAt.startsWith('0001') ? `最近上报：${beijingTime(device.receivedAt)}` : '等待 Agent 上报重量与稳定状态'}</p>
          {stage === 'second' && expected && <p className="workbench-expected">预计净重 <strong>{formatWeight(expected.net)}</strong><span>未出单</span><HelpHint label="预计净重说明">预计净重仅供参考，以确认后的正式单据为准。</HelpHint></p>}
        </ContentCard>
        <ContentCard className="workbench-current-action shadow-none" bodyClassName="p-4">
          <div className="space-y-3" aria-label="当前称重操作">
            {awaitingResult ? <><h2 className="font-semibold text-warning">提交结果待核实</h2><p className="text-sm text-text-secondary">先查询原结果，避免重复称重或出单。</p><><Button size="lg" variant="primary" className="w-full" disabled={busy} onClick={resolve}>查询上次提交结果</Button><Button size="lg" className="w-full" disabled={busy} onClick={retry}>按原请求重试</Button></></>
            : successfulTask ? <>
              <h2 className="font-semibold text-success">{successfulStage === 'first' ? '首磅已完成 · 等待回磅' : ticket ? '回磅已完成 · 已出单' : '回磅已完成'}</h2>
              {ticket && <p className="text-sm text-text-secondary">最终净重 {formatWeight(ticket.net)}{ticket.ticketNo ? ` · 单据号 ${ticket.ticketNo}` : ''}</p>}
              {ticket && <TicketPrintActions ticketId={ticket.id} />}
              <Button size="lg" variant="primary" className="w-full" onClick={nextVehicle}>办理下一辆车</Button>
              {ticket && <Link target="weighbridge-management" className="inline-block text-sm text-primary hover:underline" to={`/weighing/tickets/${ticket.id}`}>查看单据</Link>}
            </>
            : sampleReady ? <>
              <div className="flex items-center gap-1"><h2 className="font-semibold text-info">重量与图片已冻结</h2>{missingFields.length === 0 && <HelpHint label="核对提示">请核对车辆、重量和现场图片后确认。设备后续变化不撤销本次采集。</HelpHint>}</div>
              {missingFields.length > 0 && <p role="alert" className="text-sm text-warning">请选择{missingFields.join('、')}后再确认。</p>}
              <div className="flex flex-wrap gap-2">
                <Button size="lg" variant="primary" className="flex-1" onClick={openReview}><ClipboardCheck className="h-4 w-4" aria-hidden />继续核对{stage === 'first' ? '首磅' : '回磅'}（{kind}）</Button>
              </div>
            </>
            : blocked && located ? <><p role="alert" className="font-medium text-warning">{blocked}</p><span className="text-sm text-text-secondary">{shouldVerifyDay && selected ? <Link to={`/weighing/tasks/${selected.id}`}>前往任务详情核实</Link> : status.next}</span><Button size="lg" className="w-full" disabled>采集重量</Button></>
            : !located ? <Button size="lg" className="w-full" disabled>采集重量</Button>
            : !canWeigh ? <><p className="text-sm text-warning">当前任务不能继续称重，请查看任务详情。</p><Button size="lg" className="w-full" disabled>当前不可称重</Button></>
            : <><Button size="lg" variant="primary" className="w-full" disabled={busy || !!blocked} aria-busy={busy} onClick={() => void capture()}>{busy ? <LoaderCircle className="h-5 w-5 animate-spin" aria-hidden /> : <Scale className="h-5 w-5" aria-hidden />}{busy ? '采集中…' : '采集重量'}</Button></>}
          </div>
        </ContentCard>
      </div>
    </div>
    <Dialog open={businessOpen} onOpenChange={setBusinessOpen} trapFocus restoreFocus lockScroll>
      <DialogOverlay />
      <DialogContent className="workbench-business-dialog flex max-w-[1200px] flex-col">
        <DialogHeader><div><DialogTitle>查看运输资料</DialogTitle><DialogDescription>{taskInfo ? '本趟资料快照只读；纠错请前往任务详情。' : '当前提交信息。'}</DialogDescription></div><DialogClose aria-label="关闭运输资料"><X className="h-4 w-4" aria-hidden /></DialogClose></DialogHeader>
        <DialogBody><div className="weighing-business min-w-0">{taskInfo ? <ContentCard title={successfulTask ? '本趟运输' : '核对本趟运输'} extra={<StatusTag>{awaitingResult ? '提交待核实' : taskStatusLabels[taskInfo.status]}</StatusTag>}>
          <SectionTabs key={taskInfo.id} label="本趟运输信息" items={[
            { value: 'basic', label: '基本信息', content: <>
              <div className="grid gap-4 sm:grid-cols-2"><DetailItem label="车牌" value={taskInfo.plate} /><DetailItem label="运输方向" value={directionLabels[taskInfo.direction]} /><DetailItem label="物料" value={summaryMaterial} /><DetailItem label={businessPartyLabel} value={summaryParty} /></div>
              {firstRecord && <div className="mt-4 space-y-1 rounded-control border border-border bg-neutral-background p-3 text-sm"><p className="font-medium">首次当前依据：{firstRecord.kind === 'gross' ? '毛重' : '皮重'} {formatWeight(firstRecord.weight)}</p><p className="text-text-secondary">{beijingTime(firstRecord.at)}</p>{beijingDate(firstRecord.at) !== beijingDate(new Date().toISOString()) && <p className="text-warning">隔日回磅：沿用原任务和首次称重，无需额外说明。</p>}{firstRecord.replaces && <p>已重称替代原记录，原因：{firstRecord.reason}</p>}</div>}
            </> },
            { value: 'supplementary', label: '补充运输信息', content: <div className="grid gap-4 sm:grid-cols-2"><DetailItem label="承运方" value={[taskInfo.carrierName, taskInfo.carrierCode].filter(Boolean).join(' · ')} /><DetailItem label="本趟联系人 / 电话" value={[taskInfo.contactName, taskInfo.contactPhone].filter(Boolean).join(' / ')} /><DetailItem label="司机 / 电话" value={[taskInfo.driver, taskInfo.driverPhone].filter(Boolean).join(' / ')} /><DetailItem label="备注" value={taskInfo.remark} /></div> },
          ]} />
          <Link target="weighbridge-management" className="mt-4 inline-block text-sm text-primary hover:underline" to={`/weighing/tasks/${taskInfo.id}`}>查看任务详情与异常处置</Link>
        </ContentCard> : <ContentCard title="核对提交信息"><div className="grid gap-4 sm:grid-cols-2"><DetailItem label="车牌" value={summaryPlate} /><DetailItem label="运输方向" value={directionLabels[summaryDirection]} /><DetailItem label="物料" value={summaryMaterial} /><DetailItem label={businessPartyLabel} value={summaryParty} /></div></ContentCard>}</div></DialogBody>
        <DialogFooter><Button onClick={() => setBusinessOpen(false)}>关闭</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    {review && reviewValue && <WeighingReviewDialog
      title={review.title}
      invalidReason={reviewInvalidReason}
      value={reviewValue}
      missing={missingFields}
      businessEditor={!selected && missingFields.length ? <div className="review-business-editor">
        <p>重量与抓拍已冻结；补选以下资料后，无需重新采集即可确认。</p>
        <div className="review-business-editor-grid"><BusinessRequiredFields value={data.draft} update={patchReview} compact /></div>
      </div> : undefined}
      submitting={submitting}
      submitError={reviewError}
      onCancel={() => { setReview(null); setReviewError(''); }}
      onRecapture={recapture}
      onMismatch={() => {
        setReview(null); setReviewError('');
        toast.info({ title: '已返回修改', description: '调整后请重新打开核对并确认。' });
      }}
      onConfirm={netAdjustment => confirm(review.data, netAdjustment)}
    />}
  </div>;
}
