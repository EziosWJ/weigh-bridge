import { useEffect, useRef, useState, type FormEvent } from 'react';
import { ClipboardCheck, Scale, Truck } from 'lucide-react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { ContentCard } from '@/components/common/content-card';
import { SectionTabs } from '@/components/common/section-tabs';
import { DetailItem } from '@/components/common/detail-item';
import { PageHeader } from '@/components/common/page-header';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { PlateInput } from './plate-input';
import { normalizePlate, plateError } from '@/lib/weighing/plate';
import { beijingDate, beijingTime } from '@/lib/weighing/query';
import { useWeighingStore } from '@/store/weighing-store';
import { directionLabels, roleLabels, taskStatusLabels, type PlateEvidence, type FinalWeightInput, type WeighingCommand, type WeighingState } from '@/types/weighing';
import { weightValues } from '@/lib/weighing/weights';
import { formatWeight, formatWeightValue } from '@/lib/weighing/weights';
import { BusinessFields } from './shared';
import { WeighingReviewDialog } from './review-dialog';
import { DeviceCameraPanel, type CameraSize } from './camera-panel';
import './workbench.css';

const normalizeLegacy = (value: string) => value.trim().toUpperCase();

export function WeighingWorkbenchPage() {
 const { ready, storageError, refresh } = useWeighingStore();
 if (!ready) return <><PageHeader title="称重工作台" /><p role="status">{storageError || '正在加载服务端资料与权限…'}</p>{storageError && <Button onClick={() => void refresh()}>重新加载</Button>}</>;
 return <WorkbenchContent />;
}
function WorkbenchContent() {
  const navigate = useNavigate();
  const { data, dispatch, storageError, syncNotice, capabilities, busy, device } = useWeighingStore();
  const plateInput = useRef<HTMLInputElement>(null);
  const businessPanel = useRef<HTMLDivElement>(null);
  const [businessTab, setBusinessTab] = useState('basic');
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const linkedTask = data.tasks.find((task) => task.id === params.get('task'));
  const draftTask = data.tasks.find((task) => task.plate === normalizeLegacy(data.draft.plate) && (task.status === 'first' || task.status === 'second'));
  const [plate, setPlate] = useState(linkedTask?.plate ?? data.draft.plate);
  const [searchedPlate, setSearchedPlate] = useState(normalizeLegacy(linkedTask?.plate ?? (data.firstRequests[data.draftRequestId] && !draftTask ? '' : data.draftPlateEvidence?.confirmedPlate ?? '')));
  const [feedback, setFeedback] = useState('');
  const [takeoverReason, setTakeoverReason] = useState('');
  const [cameraSize, setCameraSize] = useState<CameraSize>('standard');
  const [review, setReview] = useState<WeighingState | null>(null);
  const [success, setSuccess] = useState<{ taskId: string; stage: 'first' | 'second' } | null>(null);
  const [recognition, setRecognition] = useState<{ scenePlate: string; candidate?: string } | null>(null);
  const [recognitionHint, setRecognitionHint] = useState('');
  const editing = useRef(!!data.draft.plate || !!linkedTask);
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
  const requiredHint = !located ? '请先核对车牌并查找运输' : !selected && !material?.enabled ? '请选择物料' : !selected && (!party?.enabled || !party.roles.includes(direction === 'inbound' ? 'supplier' : 'customer')) ? `请选择${direction === 'inbound' ? '供货方' : '收货方'}` : '';
  const needsDayVerification = !!selected?.crossDayDoubt;
  const conditionIssue = !capabilities?.permissionCodes.includes('weighing:confirm') ? '当前账号没有称重确认权限' : data.owner === 'other' ? '现场正在由其他会话占用，请操作员接管' : !data.config.online ? 'Agent 离线或重量数据已过期' : data.config.deviceError ? '设备异常，请恢复后重新采集' : !data.config.stable ? '重量未稳定，请等待设备稳定' : !data.config.evidence ? '等待 Agent 提供前后视角图片' : !Number.isFinite(data.config.weight) || data.config.weight <= 0 ? '等待有效上磅重量' : '';
  const shouldVerifyDay = !conditionIssue && needsDayVerification;
  const blocked = conditionIssue || (needsDayVerification ? '请先核实跨天运输，再继续称重' : '');
  const taskInfo = successfulTask || selected;
  const summaryPlate = taskInfo?.plate || data.draft.plate || plate;
  const summaryDirection = taskInfo?.direction ?? data.draft.direction;
  const summaryMaterial = taskInfo ? [taskInfo.materialName, taskInfo.materialSpec, taskInfo.materialCode].filter(Boolean).join(' · ') : material ? [material.name, material.spec, material.code].filter(Boolean).join(' · ') : '未选择';
  const summaryParty = taskInfo ? [taskInfo.partyName, taskInfo.partyCode].filter(Boolean).join(' · ') : party ? `${party.name} · ${party.code}` : '未选择';
  const confirmedRecord = !awaitingResult ? successfulTask?.records.filter(record => record.stage === successfulStage).slice(-1)[0] : undefined;
  const shownWeight = confirmedRecord?.weight ?? data.config.weight;
  const displayStage = data.pending?.stage ?? (successfulTask ? successfulStage : stage);
  const displayKind = confirmedRecord ? (confirmedRecord.kind === 'gross' ? '毛重' : '皮重') : (summaryDirection === 'outbound') === (displayStage === 'first') ? '皮重' : '毛重';
  const hasWeighingContext = located || !!taskInfo || awaitingResult;
  const sampleReady = data.sampleReady && data.capture?.plate === summaryPlate;
  useEffect(() => {
    const resume = (event: MessageEvent) => {
      if (event.origin !== window.location.origin || event.data?.type !== 'weighbridge-locate-task') return;
      if (success?.taskId === event.data.taskId) { setSuccess(null); setRecognition(null); setRecognitionHint(''); }
    };
    window.addEventListener('message', resume);
    return () => window.removeEventListener('message', resume);
  }, [success]);
  const phaseLabel = awaitingResult ? '提交待核实' : successfulTask ? '办理完成' : !located ? '等待查找车辆' : stage === 'second' ? '回磅' : '首磅';

  async function run(command: WeighingCommand, message = '', expected?: WeighingState) {
    try {
      await dispatch(command, expected);
      setReview(null);
      setFeedback(message);
      return true;
    } catch (error) {
      setFeedback(error instanceof Error ? error.message : '操作未完成，请核对输入和设备状态');
      return false;
    }
  }

  async function lookup(event: FormEvent) {
    event.preventDefault();
    const normalized = normalizeLegacy(plate);
    if (!normalized) { setFeedback('请先填写车牌'); return; }
    const legacy = data.tasks.some(task => task.plate === normalized) || data.tickets.some(ticket => ticket.plate === normalized);
    if (!legacy && plateError(normalized)) { setFeedback(plateError(normalized)); return; }
    if (data.pending) { setFeedback('上次提交结果待核实，请先查询原结果'); return; }
    let existing;
    try { existing = await useWeighingStore.getState().locate(normalized); } catch (error) { setFeedback(error instanceof Error ? error.message : '服务端任务查找失败'); return; }
    setPlate(normalized); setSuccess(null); setReview(null); setBusinessTab('basic');
    if (data.firstRequests[data.draftRequestId] && !existing && !await run({ type: 'clearDraft' })) return;
    if (!await run({ type: 'draft', patch: { plate: normalized } })) return;
    const evidence: PlateEvidence = { source: recognition?.candidate ? 'agent' : 'manual', scenePlate: recognition?.scenePlate, candidate: recognition?.candidate, confirmedPlate: normalized, at: new Date().toISOString() };
    if (!await run({ type: 'verifyPlate', evidence })) return;
    setSearchedPlate(normalized); editing.current = true;
    if (existing) {
      setParams({ task: existing.id });
      setFeedback('');
    } else {
      setParams({});
    }
  }

  async function confirm(reviewed: WeighingState, finalWeights: FinalWeightInput) {
    setReview(null);
    if (reviewed !== useWeighingStore.getState().data) { setFeedback('数据已变化，请重新采集并核对。'); return; }
    const checked = true;
    const command: WeighingCommand = selected
      ? { type: 'confirm', taskId: selected.id, stage, checked, finalWeights }
      : { type: 'firstWeigh', input: data.draft, checked, requestId: data.draftRequestId };
    if (!await run(command, '', reviewed)) return;
    const next = useWeighingStore.getState().data;
    const taskId = selected?.id ?? next.firstRequests[data.draftRequestId];
    if (taskId) setParams({ task: taskId });
    if (next.pending) {
      setFeedback('');
    } else if (taskId) {
      setSuccess({ taskId, stage }); setRecognition(null); setRecognitionHint('');
      setFeedback('');
    }
  }

  async function resolve() {
    const pending = data.pending;
    if (!pending || !await run({ type: 'resolveSubmission' })) return;
    const next = useWeighingStore.getState().data;
    const resolved = useWeighingStore.getState().lastResolution;
    const taskId = resolved?.taskId || pending.taskId || (pending.requestId ? next.firstRequests[pending.requestId] : '');
    if (pending.operation) {
      const current = pending.operation === 'reopen' && resolved?.committed ? next.tickets.find(ticket => ticket.taskId === taskId && ticket.status === 'valid') : undefined;
      navigate(current ? `/weighing/tickets/${encodeURIComponent(current.id)}` : `/weighing/tasks/${encodeURIComponent(taskId)}`);
      return;
    }
    if (resolved?.committed && taskId) {
      setParams({ task: taskId });
      setSuccess({ taskId, stage: pending.stage });
      setFeedback('已查询到原成功结果，沿用原记录。');
    } else {
      setSuccess(null);
      setFeedback('此前提交未成功，草稿已保留。请重新采集并人工核对后确认。');
    }
  }

  async function retry() {
    const pending = data.pending;
    if (!pending || !await run({ type: 'retrySubmission' })) return;
    const result=useWeighingStore.getState().lastResolution;
    if (!result?.committed || !result.taskId) return;
    if (pending.operation) { navigate(pending.operation==='print' ? `/weighing/tickets/${useWeighingStore.getState().data.tickets.find(t=>t.taskId===result.taskId&&t.status==='valid')?.id}/preview` : `/weighing/tasks/${result.taskId}`);return; }
    setParams({task:result.taskId});setSuccess({taskId:result.taskId,stage:pending.stage});setFeedback('已按原请求恢复，沿用服务端记录。');
  }

  async function nextVehicle() {
    if (!await run({ type: 'clearDraft' })) return;
    setRecognition(null); setRecognitionHint(''); editing.current = false;
    setParams({}); setPlate(''); setSearchedPlate(''); setSuccess(null);
    requestAnimationFrame(() => plateInput.current?.focus());
  }

  async function changePlate(value: string) {
    editing.current = true; setSearchedPlate('');
    setPlate(value); setParams({}); setSuccess(null); setReview(null);
    if (data.firstRequests[data.draftRequestId] && !await run({ type: 'clearDraft' })) return;
    void run({ type: 'draft', patch: { plate: value } });
  }

  const observedAt = useRef('');
  useEffect(() => {
    const observed = device?.plate;
    if (!observed?.at || observed.at.startsWith('0001') || observedAt.current === observed.at) return;
    observedAt.current = observed.at;
    const candidate = observed.failed ? '' : normalizePlate(observed.candidate);
    setRecognition({ scenePlate: '', candidate });
    if (editing.current) { setRecognitionHint(`新识别结果：${candidate || '识别失败'}，请核对当前车辆。`); return; }
    if (candidate) {
      setPlate(candidate);
      void useWeighingStore.getState().dispatch({type:'draft',patch:{plate:candidate}}).catch(() => {});
      setRecognitionHint(`识别候选 ${candidate}，待人工核对`);
    } else setRecognitionHint('识别失败，请人工录入并核对车牌。');
  }, [device?.plate]);

  function openReview() {
    if (requiredHint) {
      setFeedback(`确认前${requiredHint}`);
      const missing = !material?.enabled ? '物料' : direction === 'inbound' ? '供货方' : '收货方';
      setBusinessTab('basic');
      requestAnimationFrame(() => businessPanel.current?.querySelector<HTMLInputElement>(`input[aria-label="搜索${missing}"]`)?.focus());
      return;
    }
    setFeedback('');
    setReview(data);
  }

  return <div className="weighing-workbench space-y-4">
    <div className="workbench-page-heading"><PageHeader title="称重工作台" actions={<><StatusTag tone={data.config.online ? 'success' : 'warning'}>{data.config.online ? 'Agent 在线' : 'Agent 离线'}</StatusTag><span className="hidden text-sm text-text-tertiary sm:inline">{roleLabels[data.config.role]}</span></>} /></div>
    {(feedback || storageError || syncNotice) && <div role="status" className="workbench-notice rounded-control border border-border bg-neutral-background px-4 py-2 text-sm">{feedback && <p>{feedback}</p>}{syncNotice && <p>{syncNotice}</p>}{storageError && <p className="text-error">{storageError}</p>}</div>}
    <div className={`weighing-station weighing-camera-${cameraSize}`}>
      <DeviceCameraPanel scenePlate={confirmedRecord?.capture?.plateEvidence?.scenePlate ?? recognition?.scenePlate ?? data.draftPlateEvidence?.scenePlate} size={cameraSize} onSizeChange={setCameraSize} capture={confirmedRecord?.capture ?? (sampleReady ? data.capture : undefined)} saved={!!confirmedRecord} />
      <div className="weighing-console min-w-0 space-y-4">
        <ContentCard className="weighing-vehicle-bar shadow-none" bodyClassName="px-4 py-3">
          <div className="workbench-current-vehicle"><h2>当前车辆</h2><span className="text-xs text-text-tertiary">人工核对后定位运输</span></div>
          <form className="flex flex-wrap items-center gap-3" onSubmit={lookup}>
            <label className="flex items-center gap-2 text-sm font-medium" htmlFor="workbench-plate"><Truck className="h-5 w-5 text-text-tertiary" aria-hidden />车牌<span className="text-error">*</span></label>
            <PlateInput inputRef={plateInput} id="workbench-plate" value={plate} legacyValue={data.tasks.find(task => task.plate === plate)?.plate} disabled={awaitingResult} onChange={changePlate} />
            <Button type="submit" size="lg" variant={!located && !awaitingResult ? 'primary' : 'secondary'} disabled={awaitingResult}>核对车牌并查找运输</Button>
            <div className="workbench-phase flex flex-1 items-center justify-end gap-3 text-sm" aria-live="polite"><span className="font-medium text-text-secondary">{phaseLabel}</span>{located && !successfulTask && !awaitingResult && <StatusTag tone="info">{directionLabels[direction]}</StatusTag>}</div>
          </form>
          {recognitionHint && <p role="status" className="mt-2 text-sm text-warning">{recognitionHint}</p>}
        </ContentCard>
        <ContentCard className="weighing-scale shadow-none" bodyClassName="space-y-3 p-4">
          <div className="workbench-weighing-heading">
            <h2 className="workbench-weighing-title" aria-label={hasWeighingContext ? `${displayStage === 'first' ? '首磅' : '回磅'}（${displayKind}）` : '待定位车辆'}>
              {hasWeighingContext ? <><span className="workbench-stage-label">{displayStage === 'first' ? '首磅' : '回磅'}</span><span className="workbench-weight-kind">{displayKind}</span></> : <span className="workbench-context-empty">待定位车辆</span>}
            </h2>
            <StatusTag tone={confirmedRecord ? 'info' : data.config.stable ? 'success' : 'warning'}>{confirmedRecord ? '已确认' : data.config.stable ? '稳定' : '未稳定'}</StatusTag>
          </div>
          {hasWeighingContext && <p className="workbench-weighing-context">{displayStage === 'first' ? '第一次称重' : '第二次称重'} · {directionLabels[summaryDirection]}{awaitingResult ? ' · 提交待核实' : confirmedRecord ? ' · 本次已确认' : ''}</p>}
          <div className="weighing-instrument">
            <p className="min-w-0 text-sm text-text-secondary">{confirmedRecord ? '本次已确认重量' : '当前设备重量'}</p>
            <p className="weighing-readout" aria-label="当前重量">{Number.isFinite(shownWeight) ? formatWeightValue(shownWeight) : '—'}<span>吨</span></p>
          </div>
        </ContentCard>
        <ContentCard className="workbench-current-action shadow-none" bodyClassName="p-4">
          <div className="space-y-3" aria-label="当前称重操作">
            {awaitingResult ? <><h2 className="font-semibold text-warning">提交结果待核实</h2><p className="text-sm text-text-secondary">先查询原结果，避免重复称重或出单。</p><><Button size="lg" variant="primary" className="w-full" disabled={busy} onClick={resolve}>查询上次提交结果</Button><Button size="lg" className="w-full" disabled={busy} onClick={retry}>按原请求重试</Button></></>
            : successfulTask ? <><h2 className="font-semibold text-success">{successfulStage === 'first' ? '首磅已完成' : '回磅已完成'}</h2><p className="text-sm text-text-secondary">{ticket ? `本趟净重 ${formatWeight(ticket.net)}` : '已保存首次称重，装卸后按车牌办理回磅。'}</p><Button size="lg" variant="primary" className="w-full" onClick={nextVehicle}>办理下一辆车</Button><div className="flex flex-wrap gap-3 text-sm"><Link target="weighbridge-management" className="text-primary hover:underline" to={`/weighing/tasks/${successfulTask.id}`}>查看任务详情</Link>{ticket && <><Link target="weighbridge-management" className="text-primary hover:underline" to={`/weighing/tickets/${ticket.id}`}>查看单据</Link><Link target="_blank" className="text-primary hover:underline" to={`/weighing/tickets/${ticket.id}/preview`}>打印预览</Link></>}</div></>
            : blocked && located ? <><p role="alert" className="font-medium text-warning">{blocked}</p><span className="text-sm text-text-secondary">{shouldVerifyDay && selected ? <Link to={`/weighing/tasks/${selected.id}`}>前往任务详情核实</Link> : '请等待设备就绪后继续'}</span>{data.owner === 'other' && capabilities?.permissionCodes.includes('weighing:takeover') && <div className="space-y-2"><label htmlFor="takeover-reason" className="text-sm">接管原因</label><input id="takeover-reason" className="w-full rounded-control border border-border px-3 py-2" value={takeoverReason} onChange={e => setTakeoverReason(e.target.value)} /><Button disabled={!takeoverReason.trim() || busy} onClick={() => run({type:'takeover',reason:takeoverReason}, '操作员已接管，记录已保留。')}>接管操作权</Button></div>}</>
            : !located ? <><p className="text-sm text-text-secondary">先核对车牌并查找运输，再为当前车辆采集。</p><Button size="lg" className="w-full" disabled>采集重量</Button></>
            : !canWeigh ? <><p className="text-sm text-warning">当前任务不能继续称重，请查看任务详情。</p><Button size="lg" className="w-full" disabled>当前不可称重</Button></>
            : !sampleReady ? <><p className="text-sm text-text-secondary">车辆完全上磅、重量稳定后即可采集，资料可随后补齐。</p><Button size="lg" variant="primary" className="w-full" onClick={() => run({ type: 'capture', taskId: selected?.id, plate: summaryPlate, plateEvidence: data.draftPlateEvidence?.confirmedPlate === summaryPlate ? data.draftPlateEvidence : { source: 'manual', confirmedPlate: summaryPlate, at: new Date().toISOString() } })}><Scale className="h-4 w-4" aria-hidden />采集重量</Button></>
            : <><p className="text-sm text-text-secondary">{requiredHint ? `重量已采集，确认前${requiredHint}。` : '重量与图片已就绪，请集中核对后确认。'}</p><div className="flex flex-wrap gap-2"><Button size="lg" variant="primary" className="flex-1" onClick={openReview}><ClipboardCheck className="h-4 w-4" aria-hidden />{requiredHint ? '补齐资料后核对' : `开始核对${stage === 'first' ? '首磅' : '回磅'}（${kind}）`}</Button><Button size="lg" onClick={() => run({ type: 'capture', taskId: selected?.id, plate: summaryPlate, plateEvidence: data.draftPlateEvidence?.confirmedPlate === summaryPlate ? data.draftPlateEvidence : { source: 'manual', confirmedPlate: summaryPlate, at: new Date().toISOString() } })}>重新采集重量</Button></div></>}
          </div>
        </ContentCard>
        <div className="weighing-business min-w-0" ref={businessPanel}>{taskInfo ? <ContentCard title={successfulTask ? '本趟运输' : '核对本趟运输'} extra={<StatusTag>{awaitingResult ? '提交待核实' : taskStatusLabels[taskInfo.status]}</StatusTag>}>
          <SectionTabs key={taskInfo.id} label="本趟运输信息" items={[
            { value: 'basic', label: '基本信息', content: <>
              <div className="grid gap-4 sm:grid-cols-2"><DetailItem label="车牌" value={taskInfo.plate} /><DetailItem label="运输方向" value={directionLabels[taskInfo.direction]} /><DetailItem label="物料" value={summaryMaterial} /><DetailItem label={taskInfo.direction === 'inbound' ? '供货方' : '收货方'} value={summaryParty} /></div>
              {firstRecord && <div className="mt-4 space-y-1 rounded-control border border-border bg-neutral-background p-3 text-sm"><p className="font-medium">首次当前依据：{firstRecord.kind === 'gross' ? '毛重' : '皮重'} {formatWeight(firstRecord.weight)}</p><p className="text-text-secondary">{beijingTime(firstRecord.at)}（北京时间）</p>{beijingDate(firstRecord.at) !== beijingDate(new Date().toISOString()) && <p className="text-warning">隔日回磅：沿用原任务和首次称重，无需额外说明。</p>}{firstRecord.replaces && <p>已重称替代原记录，原因：{firstRecord.reason}</p>}</div>}
            </> },
            { value: 'supplementary', label: '补充运输信息', content: <div className="grid gap-4 sm:grid-cols-2"><DetailItem label="承运方" value={[taskInfo.carrierName, taskInfo.carrierCode].filter(Boolean).join(' · ')} /><DetailItem label="本趟联系人 / 电话" value={[taskInfo.contactName, taskInfo.contactPhone].filter(Boolean).join(' / ')} /><DetailItem label="司机 / 电话" value={[taskInfo.driver, taskInfo.driverPhone].filter(Boolean).join(' / ')} /><DetailItem label="备注" value={taskInfo.remark} /></div> },
          ]} />
          <Link target="weighbridge-management" className="mt-4 inline-block text-sm text-primary hover:underline" to={`/weighing/tasks/${taskInfo.id}`}>查看任务详情与异常处置</Link>
        </ContentCard> : located && !awaitingResult ? <ContentCard title="本趟必要信息" description="可先采集重量，确认前补齐以下资料。">
          <BusinessFields value={data.draft} hidePlate compact tab={businessTab} onTabChange={setBusinessTab}
            onPatch={patch => { void run({ type: 'draft', patch }); }} onChange={value => run({ type: 'draft', patch: value })}
            help={<div className="space-y-3 text-sm"><p className="text-text-secondary">已保留草稿。管理员补充资料后，返回即可继续选择。</p><div className="flex flex-wrap gap-3"><Link target="weighbridge-management" className="text-primary hover:underline" to={`/weighing/materials?returnTo=${encodeURIComponent(returnTo)}`}>维护物料</Link><Link target="weighbridge-management" className="text-primary hover:underline" to={`/weighing/parties?returnTo=${encodeURIComponent(returnTo)}`}>维护往来主体</Link></div></div>}
          />
        </ContentCard> : <ContentCard title={awaitingResult ? '核对提交信息' : '从车牌开始'}>
          {awaitingResult ? <div className="grid gap-4 sm:grid-cols-2"><DetailItem label="车牌" value={summaryPlate} /><DetailItem label="运输方向" value={directionLabels[summaryDirection]} /><DetailItem label="物料" value={summaryMaterial} /><DetailItem label={summaryDirection === 'inbound' ? '供货方' : '收货方'} value={summaryParty} /></div> : <ol className="list-inside list-decimal space-y-4 text-sm text-text-secondary"><li>识别候选或手工录入车牌，人工核对并查找运输。</li><li>车辆上磅后采集重量，再补齐本趟资料。</li><li>集中核对车牌、业务、重量和图片后确认。</li></ol>}
        </ContentCard>}</div>

      </div>
    </div>
    {review === data && canWeigh && !blocked && !requiredHint && sampleReady && <WeighingReviewDialog
      title={`${stage === 'first' ? '首磅' : '回磅'}（${kind}）`}
      value={{ plate: summaryPlate, direction: directionLabels[summaryDirection], material: summaryMaterial, partyLabel: summaryDirection === 'inbound' ? '供货方' : '收货方', party: summaryParty, weight: data.config.weight, kind, capture: data.capture,
        original: stage === 'second' && firstRecord ? weightValues(firstRecord.kind === 'gross' ? firstRecord.weight : data.config.weight, firstRecord.kind === 'tare' ? firstRecord.weight : data.config.weight) : undefined,
        firstWeight: firstRecord ? `${firstRecord.kind === 'gross' ? '毛重' : '皮重'} ${formatWeight(firstRecord.weight)} · ${beijingTime(firstRecord.at)}（北京时间）` : undefined,
        supplementary: [
          { label: '承运方', value: selected ? selected.carrierName ?? '' : data.parties.find(item => item.id === data.draft.carrierId)?.name ?? '' },
          { label: '本趟联系人', value: [selected ? selected.contactName : data.draft.contactName, selected ? selected.contactPhone : data.draft.contactPhone].filter(Boolean).join(' / ') },
          { label: '司机', value: [selected ? selected.driver : data.draft.driver, selected ? selected.driverPhone : data.draft.driverPhone].filter(Boolean).join(' / ') },
          { label: '备注', value: selected?.remark ?? data.draft.remark },
        ].filter(item => item.value),
      }}
      onCancel={() => setReview(null)}
      onMismatch={() => { setReview(null); setFeedback(selected ? '请核对本趟车辆、重量与图片；业务资料有误可前往任务详情办理纠错。' : '请修改本趟资料后重新核对；车牌或现场情况有变化时重新采集。'); }}
      onConfirm={finalWeights => confirm(review, finalWeights)}
    />}
  </div>;
}
