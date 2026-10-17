import { useRef, useState, type FormEvent } from 'react';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { PageHeader } from '@/components/common/page-header';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { beijingDate, beijingTime } from '@/lib/weighing/query';
import { useWeighingStore } from '@/store/weighing-store';
import { directionLabels, roleLabels, taskStatusLabels, type FinalWeightInput, type WeighingCommand, type WeighingState } from '@/types/weighing';
import { weightValues } from '@/lib/weighing/weights';
import { formatWeight, formatWeightValue } from '@/lib/weighing/weights';
import { BusinessFields } from './shared';
import { WeighingReviewDialog } from './review-dialog';
import { SimulatedCameraPanel } from './camera-panel';

const normalizePlate = (value: string) => value.trim().toUpperCase();

export function WeighingWorkbenchPage() {
  const { data, dispatch, storageError } = useWeighingStore();
  const plateInput = useRef<HTMLInputElement>(null);
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const linkedTask = data.tasks.find((task) => task.id === params.get('task'));
  const draftTask = data.tasks.find((task) => task.plate === normalizePlate(data.draft.plate) && (task.status === 'first' || task.status === 'second'));
  const [plate, setPlate] = useState(linkedTask?.plate ?? data.draft.plate);
  const [searchedPlate, setSearchedPlate] = useState(normalizePlate(linkedTask?.plate ?? (data.firstRequests[data.draftRequestId] && !draftTask ? '' : data.draft.plate)));
  const [feedback, setFeedback] = useState('');
  const [review, setReview] = useState<WeighingState | null>(null);
  const [success, setSuccess] = useState<{ taskId: string; stage: 'first' | 'second' } | null>(null);
  const located = !!searchedPlate && searchedPlate === normalizePlate(plate);
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
  const settingsUrl = `/weighing/settings?returnTo=${encodeURIComponent(returnTo)}`;
  const material = data.materials.find(item => item.id === data.draft.materialId);
  const party = data.parties.find(item => item.id === data.draft.partyId);
  const requiredHint = !located ? '请先输入车牌并查找' : !selected && !material?.enabled ? '请选择物料' : !selected && (!party?.enabled || !party.roles.includes(direction === 'inbound' ? 'supplier' : 'customer')) ? `请选择${direction === 'inbound' ? '供货方' : '收货方'}` : '';
  const needsDayVerification = !!selected && (!!selected.crossDayDoubt || (!!firstRecord && beijingDate(firstRecord.at) !== beijingDate(new Date().toISOString()) && selected.verifiedDate !== beijingDate(new Date().toISOString())));
  const conditionIssue = data.config.role === 'admin' ? '请切换操作员或主管岗位' : data.owner === 'other' ? '现场正在由其他会话占用，请主管接管' : !data.config.online ? '模拟已断网，请恢复连接' : data.config.deviceError ? '模拟设备异常，请恢复后重新采集' : !data.config.stable ? '重量失稳：请恢复稳定并重新采集' : data.config.timedOut ? '模拟重量已超时，请重新采集' : !data.config.evidence ? '模拟证据不可用，请恢复后重新采集' : !Number.isFinite(data.config.weight) || data.config.weight <= 0 ? '模拟重量无效，请设置大于零的重量' : '';
  const shouldVerifyDay = !conditionIssue && needsDayVerification;
  const blocked = conditionIssue || (needsDayVerification ? '请先核实跨天运输，再继续称重' : '');
  const taskInfo = successfulTask || selected;
  const summaryPlate = taskInfo?.plate || data.draft.plate || plate;
  const summaryDirection = taskInfo?.direction ?? data.draft.direction;
  const summaryMaterial = taskInfo ? [taskInfo.materialName, taskInfo.materialSpec, taskInfo.materialCode].filter(Boolean).join(' · ') : material ? [material.name, material.spec, material.code].filter(Boolean).join(' · ') : '未选择';
  const summaryParty = taskInfo ? [taskInfo.partyName, taskInfo.partyCode].filter(Boolean).join(' · ') : party ? `${party.name} · ${party.code}` : '未选择';
  const confirmedRecord = !awaitingResult ? successfulTask?.records.filter(record => record.stage === successfulStage).slice(-1)[0] : undefined;
  const shownWeight = confirmedRecord?.weight ?? data.config.weight;
  const phaseLabel = awaitingResult ? '提交待核实' : successfulTask ? '办理完成' : !located ? '第 1 步 · 查找车辆' : requiredHint ? '第 2 步 · 填写信息' : '第 3 步 · 称重确认';

  function run(command: WeighingCommand, message = '') {
    try {
      dispatch(command);
      setReview(null);
      setFeedback(message);
      return true;
    } catch (error) {
      setFeedback(error instanceof Error ? error.message : '操作未完成，请核对输入和演示条件');
      return false;
    }
  }

  function lookup(event: FormEvent) {
    event.preventDefault();
    const normalized = normalizePlate(plate);
    if (!normalized) { setFeedback('请先填写车牌'); return; }
    if (data.pending) { setFeedback('上次提交结果待核实，请先查询原结果'); return; }
    const existing = data.tasks.find((task) => task.plate === normalized && (task.status === 'first' || task.status === 'second'));
    setPlate(normalized); setSearchedPlate(normalized); setSuccess(null); setReview(null);
    if (existing) {
      setParams({ task: existing.id });
      setFeedback('');
    } else {
      setParams({});
      if (data.firstRequests[data.draftRequestId] && !run({ type: 'clearDraft' })) return;
      run({ type: 'draft', patch: { plate: normalized } });
    }
  }

  function confirm(reviewed: WeighingState, finalWeights: FinalWeightInput) {
    setReview(null);
    if (reviewed !== useWeighingStore.getState().data) { setFeedback('数据已变化，请重新采集并核对。'); return; }
    const checked = true;
    const command: WeighingCommand = selected
      ? { type: 'confirm', taskId: selected.id, stage, checked, finalWeights }
      : { type: 'firstWeigh', input: data.draft, checked, requestId: data.draftRequestId };
    if (!run(command)) return;
    const next = useWeighingStore.getState().data;
    const taskId = selected?.id ?? next.firstRequests[data.draftRequestId];
    if (taskId) setParams({ task: taskId });
    if (next.pending) {
      setFeedback('');
    } else if (taskId) {
      setSuccess({ taskId, stage });
      setFeedback('');
    }
  }

  function resolve() {
    const pending = data.pending;
    if (!pending || !run({ type: 'resolveSubmission' })) return;
    const next = useWeighingStore.getState().data;
    const taskId = pending.taskId || (pending.requestId ? next.firstRequests[pending.requestId] : '');
    if (pending.committed && taskId) {
      setParams({ task: taskId });
      setSuccess({ taskId, stage: pending.stage });
      setFeedback('已查询到原成功结果，沿用原记录。');
    } else {
      setSuccess(null);
      setFeedback('此前提交未成功，草稿已保留。请重新采集并人工核对后确认。');
    }
  }

  function nextVehicle() {
    if (!run({ type: 'clearDraft' })) return;
    setParams({}); setPlate(''); setSearchedPlate(''); setSuccess(null);
    requestAnimationFrame(() => plateInput.current?.focus());
  }

  function changePlate(value: string) {
    if (data.firstRequests[data.draftRequestId] && !run({ type: 'clearDraft' })) return;
    setPlate(value); setParams({}); setSuccess(null); setReview(null);
    run({ type: 'draft', patch: { plate: value } });
  }

  return <div className="space-y-4">
    <PageHeader title="称重工作台" actions={<><StatusTag tone="warning">测试环境</StatusTag><span className="hidden text-sm text-text-tertiary sm:inline">{roleLabels[data.config.role]}</span><Link className="text-sm text-primary hover:underline" to={settingsUrl}>调整模拟重量 / 演示岗位</Link></>} />
    <ContentCard bodyClassName="py-3">
      <form className="flex flex-wrap items-center gap-3" onSubmit={lookup}>
        <label className="text-sm font-medium" htmlFor="workbench-plate">车牌<span className="ml-1 text-error">*</span></label>
        <Input ref={plateInput} id="workbench-plate" className="h-10 w-full max-w-xs text-base" value={plate} disabled={awaitingResult} placeholder="输入车牌，按回车查找" onChange={event => changePlate(event.target.value)} />
        <Button type="submit" size="lg" variant={!located && !awaitingResult ? 'primary' : 'secondary'} disabled={awaitingResult}>查找当前运输</Button>
        <span className="text-sm font-medium text-text-secondary" aria-live="polite">{phaseLabel}</span>
      </form>
    </ContentCard>
    {(feedback || storageError) && <div role="status" className="rounded-control border border-border bg-neutral-background px-4 py-2 text-sm">{feedback && <p>{feedback}</p>}{storageError && <p className="text-error">{storageError}</p>}</div>}
    <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_380px]">
      <div className="grid min-w-0 items-start gap-3 xl:grid-cols-[200px_minmax(0,1fr)]">
        <SimulatedCameraPanel preset={data.config.cameraPreset} capture={confirmedRecord?.capture ?? data.capture} saved={!!confirmedRecord} />
        <div className="min-w-0">{taskInfo ? <ContentCard title={successfulTask ? '本趟运输' : '核对本趟运输'} extra={<StatusTag>{awaitingResult ? '提交待核实' : taskStatusLabels[taskInfo.status]}</StatusTag>}>
          <div className="grid gap-4 sm:grid-cols-2"><DetailItem label="车牌" value={taskInfo.plate} /><DetailItem label="运输方向" value={directionLabels[taskInfo.direction]} /><DetailItem label="物料" value={summaryMaterial} /><DetailItem label={taskInfo.direction === 'inbound' ? '供货方' : '收货方'} value={summaryParty} /></div>
          {firstRecord && <div className="mt-4 space-y-1 rounded-control border border-border bg-neutral-background p-3 text-sm"><p className="font-medium">首次当前依据：{firstRecord.kind === 'gross' ? '毛重' : '皮重'} {formatWeight(firstRecord.weight)}</p><p className="text-text-secondary">{beijingTime(firstRecord.at)}（北京时间）</p>{firstRecord.replaces && <p>已重称替代原记录，原因：{firstRecord.reason}</p>}</div>}
          <details className="mt-4 border-t border-border pt-3"><summary className="cursor-pointer rounded-control text-sm font-medium text-text-secondary focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">补充运输信息<span className="ml-2 font-normal">{[taskInfo.carrierName && `承运：${taskInfo.carrierName}`, taskInfo.driver && `司机：${taskInfo.driver}`].filter(Boolean).join(' · ')}</span></summary><div className="mt-4 grid gap-4 sm:grid-cols-2"><DetailItem label="承运方" value={[taskInfo.carrierName, taskInfo.carrierCode].filter(Boolean).join(' · ')} /><DetailItem label="本趟联系人 / 电话" value={[taskInfo.contactName, taskInfo.contactPhone].filter(Boolean).join(' / ')} /><DetailItem label="司机 / 电话" value={[taskInfo.driver, taskInfo.driverPhone].filter(Boolean).join(' / ')} /><DetailItem label="备注" value={taskInfo.remark} /></div></details>
          <Link className="mt-4 inline-block text-sm text-primary hover:underline" to={`/weighing/tasks/${taskInfo.id}`}>查看任务详情与异常处置</Link>
        </ContentCard> : located && !awaitingResult ? <ContentCard title="本趟必要信息" description="先选择物料和供收方，再采集重量。">
          <BusinessFields value={data.draft} hidePlate compact onChange={value => run({ type: 'draft', patch: value })} />
          <details className="mt-3 border-t border-border pt-3 text-sm"><summary className="cursor-pointer rounded-control text-text-tertiary focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">找不到资料？</summary><p className="mt-2 text-text-secondary">已保留草稿。管理员补充资料后，返回即可继续选择。</p><div className="mt-2 flex flex-wrap gap-3"><Link className="text-primary hover:underline" to={`/weighing/materials?returnTo=${encodeURIComponent(returnTo)}`}>维护物料</Link><Link className="text-primary hover:underline" to={`/weighing/parties?returnTo=${encodeURIComponent(returnTo)}`}>维护往来主体</Link></div></details>
        </ContentCard> : <ContentCard title={awaitingResult ? '核对提交信息' : '从车牌开始'}>
          {awaitingResult ? <div className="grid gap-4 sm:grid-cols-2"><DetailItem label="车牌" value={summaryPlate} /><DetailItem label="运输方向" value={directionLabels[summaryDirection]} /><DetailItem label="物料" value={summaryMaterial} /><DetailItem label={summaryDirection === 'inbound' ? '供货方' : '收货方'} value={summaryParty} /></div> : <ol className="list-inside list-decimal space-y-4 text-sm text-text-secondary"><li>输入车牌并查找当前运输。</li><li>新车填写本趟信息；回磅核对首次重量。</li><li>采集重量，人工核对后确认。</li></ol>}
        </ContentCard>}</div>
      </div>
      <ContentCard title="称重与确认" className="self-start lg:sticky lg:top-20" bodyClassName="space-y-4" extra={<StatusTag tone="warning">模拟</StatusTag>}>
        <p className="text-sm font-medium">本次办理：{(successfulTask ? successfulStage : stage) === 'first' ? '首次称重' : '二次称重'} / {(summaryDirection === 'outbound') === ((successfulTask ? successfulStage : stage) === 'first') ? '皮重' : '毛重'}</p>
        <div className="rounded-control border border-border bg-neutral-background p-3"><p className="text-xs text-text-tertiary">{confirmedRecord ? '本次已确认测试重量' : '当前模拟重量'}</p><p className="mt-1 font-mono text-3xl font-semibold tabular-nums">{Number.isFinite(shownWeight) ? formatWeightValue(shownWeight) : '—'} <span className="text-sm font-normal">吨</span></p><div className="mt-2 flex flex-wrap gap-2"><StatusTag tone={data.config.stable ? 'success' : 'warning'}>{data.config.stable ? '模拟稳定' : '模拟失稳'}</StatusTag><StatusTag tone={data.sampleReady || confirmedRecord ? 'info' : 'neutral'}>{confirmedRecord ? '已确认' : data.sampleReady ? '已采集' : '待采集'}</StatusTag></div></div>
        {located && <dl className="space-y-1 text-sm" aria-label="当前称重核对摘要"><div className="flex gap-2"><dt className="shrink-0 text-text-tertiary">车牌</dt><dd className="break-all font-medium">{summaryPlate}</dd></div><div className="flex gap-2"><dt className="shrink-0 text-text-tertiary">方向</dt><dd>{directionLabels[summaryDirection]}</dd></div><div className="flex gap-2"><dt className="shrink-0 text-text-tertiary">物料</dt><dd className="break-words">{summaryMaterial}</dd></div><div className="flex gap-2"><dt className="shrink-0 text-text-tertiary">{summaryDirection === 'inbound' ? '供货方' : '收货方'}</dt><dd className="break-words">{summaryParty}</dd></div></dl>}
        <div className="space-y-3 border-t border-border pt-4" aria-label="当前称重操作">
          {awaitingResult ? <><h2 className="font-semibold text-warning">提交结果待核实</h2><p className="text-sm text-text-secondary">先查询原结果，避免重复称重或出单。</p>{!data.config.online || data.config.role === 'admin' ? <><p className="text-sm text-warning">{!data.config.online ? '请先恢复模拟连接' : '请先切换操作员或主管岗位'}</p><Link className="inline-flex h-10 w-full items-center justify-center rounded-control bg-primary px-4 text-sm font-medium text-white" to={settingsUrl}>前往演示设置</Link></> : <Button size="lg" variant="primary" className="w-full" onClick={resolve}>查询上次提交结果</Button>}</>
          : successfulTask ? <><h2 className="font-semibold text-success">{successfulStage === 'first' ? '首磅已完成' : '回磅已完成'}</h2><p className="text-sm text-text-secondary">{ticket ? `本趟净重 ${formatWeight(ticket.net)}` : '已保存首次称重，装卸后按车牌办理回磅。'}</p><Button size="lg" variant="primary" className="w-full" onClick={nextVehicle}>办理下一辆车</Button><div className="flex flex-wrap gap-3 text-sm"><Link className="text-primary hover:underline" to={`/weighing/tasks/${successfulTask.id}`}>查看任务详情</Link>{ticket && <Link className="text-primary hover:underline" to={`/weighing/tickets/${ticket.id}`}>查看测试单据</Link>}</div></>
          : blocked && located ? <><p role="alert" className="font-medium text-warning">{blocked}</p><Link className="inline-flex h-10 w-full items-center justify-center rounded-control bg-primary px-4 text-sm font-medium text-white" to={shouldVerifyDay && selected ? `/weighing/tasks/${selected.id}` : settingsUrl}>{shouldVerifyDay ? '前往任务详情核实' : '前往演示设置'}</Link></>
          : requiredHint ? <><p className="text-sm text-text-secondary">下一步：{requiredHint}</p><Button size="lg" className="w-full" disabled>采集模拟重量</Button></>
          : !canWeigh ? <><p className="text-sm text-warning">当前任务不能继续称重，请查看任务详情。</p><Button size="lg" className="w-full" disabled>当前不可称重</Button></>
          : !data.sampleReady ? <><p className="text-sm text-text-secondary">信息已齐备，采集重量时同步模拟抓拍。</p><Button size="lg" variant="primary" className="w-full" onClick={() => run({ type: 'capture', taskId: selected?.id, plate: summaryPlate })}>采集模拟重量</Button></>
          : <><p className="text-sm text-text-secondary">重量已采集，接下来逐项核对车牌、业务、重量和模拟证据。</p><Button size="lg" variant="primary" className="w-full" onClick={() => { setFeedback(''); setReview(data); }}>开始核对{stage === 'first' ? '首磅' : '回磅'}（{kind}）</Button><Button size="sm" variant="ghost" className="w-full" onClick={() => run({ type: 'capture', taskId: selected?.id, plate: summaryPlate })}>重新采集模拟重量</Button></>}
        </div>
        <p className="text-xs text-text-tertiary">测试数据 · 模拟证据 · 摄像头未接入</p>
      </ContentCard>
    </div>
    {review === data && canWeigh && !blocked && !requiredHint && data.sampleReady && <WeighingReviewDialog
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
      onMismatch={step => { setReview(null); setFeedback(step === 4 ? '请核对当前车辆与本趟信息，修改后重新采集并逐项核对。' : step === 0 ? '请修改车牌并重新查找当前运输。' : step === 1 ? selected ? '请前往任务详情核实业务信息或由主管纠错。' : '请修改本趟业务信息，再重新采集和核对。' : '请在演示设置核实重量或模拟证据状态，再重新采集。'); if (step === 0) requestAnimationFrame(() => plateInput.current?.focus()); }}
      onConfirm={finalWeights => confirm(review, finalWeights)}
    />}
  </div>;
}
