import { CapturePhoto } from './camera-panel';
import { useWeighingPage, downloadWeighingCsv } from '@/hooks/use-weighing-page';
import type { WireTask } from '@/api/weighing-workflow';
import { plateError } from '@/lib/weighing/plate';
import { WorkbenchLink } from "@/components/common/workbench-link";
import { WeightAdjustmentDetails } from "./deduction-fields";
import { useDeferredValue, useEffect, useRef, useState } from "react";
import { Link, Navigate, useParams, useSearchParams } from "react-router-dom";
import { ContentCard } from "@/components/common/content-card";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailItem } from "@/components/common/detail-item";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { StatusTag } from "@/components/common/status-tag";
import { TableActions } from "@/components/common/table-actions";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useWeighingStore } from "@/store/weighing-store";
import { beijingTime } from "@/lib/weighing/query";
import { formatWeight } from "@/lib/weighing/weights";
import type { DataTableColumn } from "@/types";
import { directionLabels, emptyTaskInput, taskStatusLabels, type TaskInput, type WeighingFilter } from "@/types/weighing";
import { BusinessFields, DeviceBanner, HistoryList, WeighingFilters, ListDate } from "./shared";
import { BusinessSnapshot, WeighRecords } from "./record-info";

const linkClass = "text-primary hover:underline";
type TaskOperation = "correct" | "reweigh" | "terminate";
const operationTitles: Record<TaskOperation, string> = { correct: "业务纠错", reweigh: "单磅重称", terminate: "终止运输任务" };

export function WeighingTasksPage() {
  const capabilities = useWeighingStore(state => state.capabilities);
  const [params] = useSearchParams();
  const [filter, setFilter] = useState<WeighingFilter>({ status: 'second' });
  const deferred = useDeferredValue(filter);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const result = useWeighingPage<WireTask>('tasks', deferred, page, pageSize);
  const can = (code: string) => capabilities?.permissionCodes.includes(`weighing:${code}`) ?? false;
  const columns: DataTableColumn<WireTask>[] = [
    { title: '任务编号 / 车牌', key: 'task', width: 220, render: (_, row) => <div><Link className={linkClass} to={`/weighing/tasks/${row.id}`}>{row.taskNo}</Link><div className="mt-1 font-medium">{row.business.plate}</div></div> },
    {title:"厂区",key:"site",width:100,render:(_,row)=>row.siteCode||"—"},
    { title: '方向', key: 'direction', width: 100, render: (_, row) => directionLabels[row.business.direction] },
    { title: '物料 / 规格', key: 'material', width: 180, render: (_, row) => <div>{row.business.materialName}<div className="text-text-tertiary">{row.business.materialSpec || '—'} · {row.business.materialCode}</div></div> },
    { title: '供收方', key: 'party', width: 160, render: (_, row) => row.business.partyName },
    { title: '首次 / 二次重量（吨）', key: 'weights', width: 180, render: (_, row) => `${row.firstWeight ?? '—'} / ${row.secondWeight ?? '—'}` },
    { title: '任务状态', key: 'status', width: 120, render: (_, row) => <StatusTag tone={row.status === 'completed' ? 'success' : row.status === 'terminated' ? 'neutral' : 'warning'}>{row.status === 'awaiting_second' ? '待二次称重' : row.status === 'completed' ? '已完成' : '已终止'}</StatusTag> },
    { title: '创建时间', key: 'date', width: 144, render: (_, row) => <ListDate value={row.createTime} /> },
    { title: '操作', key: 'actions', width: 148, nowrap: true, render: (_, row) => <TableActions><Link className={linkClass} to={`/weighing/tasks/${row.id}`}>详情</Link>{can('confirm') && capabilities?.scales.some(s=>s.canOperate) && row.status === 'awaiting_second' && <WorkbenchLink className={linkClass} to={`/weighing/workbench?scale=${encodeURIComponent(capabilities?.localScaleCode ?? "")}&task=${row.id}`}>办理称重</WorkbenchLink>}</TableActions> },
  ];
  if (params.get('task')) return <Navigate replace to={`/weighing/tasks/${encodeURIComponent(params.get('task')!)}`} />;
  return <div className="weighing-list-page">
    <PageHeader title="待回磅任务" description="默认显示待二次称重的车辆，可切换状态查询历史任务。" />
    <DeviceBanner compact /><WeighingFilters value={filter} onChange={value => { setFilter(Object.keys(value).length ? value : { status: 'second' }); setPage(1); }} />
    <DataTableCard className="weighing-list-table"><TableToolbar className="!py-space-3" title="运输任务" description={`当前筛选 ${result.total} 条`} actions={<><Button size="sm" onClick={result.refresh}>刷新</Button>{can('export') && <Button size="sm" onClick={() => void downloadWeighingCsv('tasks', filter).then(() => toast.success("运输任务 CSV 已导出")).catch(error => toast.error({ title: "导出失败", description: error.message }))}>导出筛选明细 CSV</Button>}</>} />
      <DataTable compact stickyHeader fixedEdges ariaLabel="运输任务列表" className="weighing-list-scroll" columns={columns} dataSource={result.records} loading={result.loading} error={result.error} rowKey="id" minWidth={1352} />
      <Pagination page={page} pageSize={pageSize} total={result.total} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
  </div>;
}

export function WeighingTaskDetailPage() {
  const { id } = useParams();
  const { data, dispatch, capabilities, loadTask } = useWeighingStore();
  const [operation, setOperation] = useState<TaskOperation | null>(null);
  const [input, setInput] = useState<TaskInput>(emptyTaskInput);
  const [reason, setReason] = useState("");
  const [reweighStage, setReweighStage] = useState<'first' | 'second'>('first');
  const [otherValid, setOtherValid] = useState(false);
  const [sameLoad, setSameLoad] = useState(false);
  const [checked, setChecked] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [photoPreviewOpen, setPhotoPreviewOpen] = useState(false);
  const [readyCaptureId, setReadyCaptureId] = useState("");
  const task = data.tasks.find((item) => item.id === id);
  const targetRecord = task && [...task.records].reverse().find(record => record.stage === reweighStage);
  const retainedRecord = task && [...task.records].reverse().find(record => record.stage !== reweighStage);
  const pending = !!task && data.pending?.taskId === task.id;
  const canOperate = !!capabilities?.permissionCodes.some(code => ["weighing:correct", "weighing:terminate", "weighing:reweigh"].includes(code));
  const siteCanOperate=capabilities?.sites.some(s=>s.canOperate)??false;
  const canWeigh = siteCanOperate&&(capabilities?.permissionCodes.includes("weighing:confirm") ?? false);
  const reweighCapture = data.sampleReady && data.capture?.plate === task?.plate && data.capture?.stage === reweighStage ? data.capture : undefined;
  const reweighImageReady = !!reweighCapture && (reweighCapture.captureRequired === false || readyCaptureId === reweighCapture.id);

  useEffect(() => { if (id) void loadTask(id).catch(failure => setError(failure instanceof Error ? failure.message : '详情加载失败')); }, [id, loadTask]);
  const can = (code: string) => siteCanOperate&&(capabilities?.permissionCodes.includes(`weighing:${code}`) ?? false);
  const operationBasis = useRef(data);

  function startOperation(next: TaskOperation) {
    if (!task) return;
    operationBasis.current = data;
    setInput({ plate: task.plate, direction: task.direction, materialId: task.materialId, partyId: task.partyId, carrierId: task.carrierId ?? "", contactName: task.contactName ?? "", contactPhone: task.contactPhone ?? "", driver: task.driver, driverPhone: task.driverPhone ?? "", remark: task.remark });
    setReweighStage(task.status === "completed" ? "second" : "first"); setOtherValid(false); setReason(""); setSameLoad(false); setChecked(false); setError(""); setPhotoPreviewOpen(false); setReadyCaptureId(""); setOperation(next);
  }

  async function submitOperation() {
    if (!task || !operation) return;
    try {
      if (operation === 'correct' && input.plate !== task.plate && plateError(input.plate)) throw new Error(plateError(input.plate));
      if (operation === "correct") await dispatch({ type: "correctTask", taskId: task.id, input, reason }, operationBasis.current);
      if (operation === "terminate") await dispatch({ type: "terminate", taskId: task.id, reason }, operationBasis.current);
      if (operation === "reweigh") await dispatch({ type: "reweigh", taskId: task.id, reason, sameLoad, checked, stage: reweighStage, otherValid, captureId: reweighCapture?.id }, operationBasis.current);
      setOperation(null);
      if (useWeighingStore.getState().data.pending) {
        setNotice("提交结果待核实，请前往工作台查询上次提交结果。");
        toast.warning({ title: "提交结果待核实", description: "请先查询原请求，再继续办理。", duration: 6000 });
      } else {
        setNotice("");
        toast.success(`${operationTitles[operation]}已完成`);
      }
    } catch (failure) { setError(failure instanceof Error ? failure.message : "操作失败"); }
  }

  if (!task) return <><PageHeader title="运输任务详情" actions={<Link className={linkClass} to="/weighing/tasks">返回任务列表</Link>} /><ContentCard><p>未找到该测试任务，请刷新或核对任务编号。</p></ContentCard></>;
  const tickets = data.tickets.filter((ticket) => ticket.taskId === task.id);
  return <>
    <PageHeader title="运输任务详情" description={task.taskNo ?? task.id} actions={<><Link className={linkClass} to="/weighing/tasks">返回任务列表</Link>{canWeigh && (pending || ["first", "second"].includes(task.status)) && <WorkbenchLink className={linkClass} to={`/weighing/workbench?task=${encodeURIComponent(task.id)}`}>{pending ? "前往工作台查询提交结果" : "前往工作台办理"}</WorkbenchLink>}</>} />
    <DeviceBanner />
    <div className="space-y-space-6">
      {notice && <p role="status" className="text-success">{notice}</p>}
      <ContentCard title="任务状态" extra={<StatusTag tone={pending ? "warning" : task.status === "completed" ? "success" : "neutral"}>{pending ? "提交结果待核实" : taskStatusLabels[task.status]}</StatusTag>}>
        {pending && <p role="alert" className="mb-space-4 text-warning">上次提交结果尚未核实。当前保存的记录不表示已核实成功；请先到工作台查询结果，再继续办理。</p>}
        <div className="grid gap-space-4 sm:grid-cols-3"><DetailItem label="创建时间" value={beijingTime(task.createdAt)} /><DetailItem label="首次确认时间" value={task.records.filter(record=>record.stage==='first').slice(-1)[0]?.at ? beijingTime(task.records.filter(record=>record.stage==='first').slice(-1)[0].at) : '尚未确认'} /><DetailItem label="二次确认时间" value={task.records.filter(record=>record.stage==='second').slice(-1)[0]?.at ? beijingTime(task.records.filter(record=>record.stage==='second').slice(-1)[0].at) : '尚未确认'} /></div>
        {!pending && task.status !== "completed" && task.status !== "terminated" && <div className="mt-space-4 flex flex-wrap gap-space-2">{canOperate && <>{can("correct") && <Button size="sm" onClick={() => startOperation("correct")}>业务纠错</Button>}{can("reweigh") && task.records.length > 0 && <Button size="sm" onClick={() => startOperation("reweigh")}>重称</Button>}{can("terminate") && <Button size="sm" variant="danger" onClick={() => startOperation("terminate")}>终止任务</Button>}</>}</div>}
      </ContentCard>
      {!pending && can("reweigh") && task.status === 'completed' && !tickets.some(ticket => ticket.status === 'valid') && <ContentCard title="作废后的单磅重称"><p className="mb-3">只采集有问题的一磅，核对目标装载状态并沿用另一有效磅，然后在关联作废单办理重开。</p><Button onClick={() => startOperation('reweigh')}>单磅重称</Button></ContentCard>}
      <ContentCard title="本趟业务信息" description="保留当时的资料与联系信息快照；旧记录未保存的字段显示“-”。"><BusinessSnapshot value={task} /></ContentCard>
      <ContentCard title="原始称重记录与替代关系"><WeighRecords records={task.records} /></ContentCard>
      <ContentCard title="关联测试单">{tickets.length ? <div className="space-y-space-2">{tickets.map((ticket) => <div key={ticket.id}><Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}`}>{ticket.id}</Link><span className="ml-space-3 text-text-secondary">{ticket.status === "valid" ? "有效" : "作废"}{ticket.previousId ? ` · 更正自 ${ticket.previousId}` : ""}</span></div>)}</div> : <p className="text-text-tertiary">尚无关联测试单。</p>}</ContentCard>
      <ContentCard title="单据调整追溯">{tickets.length ? tickets.map(ticket => <div key={ticket.id} className="mb-5 border-b border-border pb-4 last:border-0"><p className="mb-3 text-sm font-medium">{ticket.id} · {ticket.status === "valid" ? "有效" : "作废"}</p><WeightAdjustmentDetails ticket={ticket} /></div>) : <p className="text-text-tertiary">尚未出单，无已确认调整。</p>}</ContentCard>
      <ContentCard title="操作历史"><HistoryList entries={task.history} /></ContentCard>
    </div>
    <FormDialog open={!!operation} title={operation ? operationTitles[operation] : ""} onCancel={() => setOperation(null)} onSubmit={submitOperation} submitDisabled={operation === "reweigh" && (!reweighImageReady || !checked || !sameLoad || (task.records.some(record => record.stage !== reweighStage) && !otherValid))} submitText={operation === "reweigh" ? "确认当前采集重称" : "确认保存"} trapFocus={!photoPreviewOpen} closeOnEscape={!photoPreviewOpen} closeOnOverlayClick={!photoPreviewOpen}>
      <div className="space-y-space-4">
        {operation === "correct" && <BusinessFields value={input} onChange={setInput} lockDirection={task.records.length > 0} />}
        {operation === "terminate" && <p className="text-error">终止后不可恢复；已有称重记录保留，车牌可办理新任务。</p>}
        {operation === "reweigh" && <><Field label="目标称重" htmlFor="reweigh-stage"><select id="reweigh-stage" className="h-10 rounded-control border border-border px-3" value={reweighStage} onChange={event => { setReweighStage(event.target.value as 'first' | 'second'); setSameLoad(false); setOtherValid(false); setChecked(false); }}>{(['first', 'second'] as const).filter(stage => task.records.some(record => record.stage === stage)).map(stage => <option key={stage} value={stage}>{stage === 'first' ? '首次' : '二次'} · {(task.direction === 'inbound') === (stage === 'first') ? '毛重（满载）' : '皮重（空载）'}</option>)}</select></Field><div className="grid gap-3 sm:grid-cols-2"><DetailItem label="本次替代的称重依据" value={targetRecord ? `${targetRecord.id} · ${formatWeight(targetRecord.weight)} · ${beijingTime(targetRecord.at)}` : '尚未确认'} /><DetailItem label="沿用的另一磅依据" value={retainedRecord ? `${retainedRecord.id} · ${formatWeight(retainedRecord.weight)} · ${beijingTime(retainedRecord.at)}` : '另一磅尚未确认'} /></div><p className="text-warning">空车不能替代满载毛重；无法恢复目标装载状态时，请在作废单重开中人工调整最终重量。</p><p>设备当前重量：{formatWeight(data.config.weight)}。先重新采集，再人工核对；原记录和替代关系保留。</p><Button onClick={async () => { try { await dispatch({ type: "capture", taskId: task.id, stage: reweighStage }); operationBasis.current = useWeighingStore.getState().data; setOtherValid(false); setChecked(false); setReadyCaptureId(""); setError(""); } catch (failure) { setError(failure instanceof Error ? failure.message : "采集失败"); } }}>重新采集重量</Button><p className="text-text-secondary">当前采集：{reweighCapture ? (reweighCapture.captureRequired === false ? "有效，本次未强制抓拍，请核对现场车辆及重量" : "有效，需核对本次抓拍图片") : "未采集或已失效"}</p>
          {reweighCapture && <div className="max-w-sm"><CapturePhoto key={reweighCapture.id} capture={reweighCapture} onReady={ready => { setReadyCaptureId(ready ? reweighCapture.id : ''); }} onPreviewChange={setPhotoPreviewOpen} /></div>}
          {task.records.some(record => record.stage !== reweighStage) && <label className="flex items-center gap-space-2"><input type="checkbox" checked={otherValid} onChange={event => setOtherValid(event.target.checked)} />已确认沿用的另一磅仍有效（不重新采集）</label>}<label className="flex items-center gap-space-2"><input type="checkbox" checked={sameLoad} onChange={(event) => setSameLoad(event.target.checked)} />已确认车辆装载状态仍符合原称重条件</label><label className="flex items-center gap-space-2"><input type="checkbox" checked={checked} disabled={!reweighImageReady} onChange={(event) => setChecked(event.target.checked)} />已人工核对车辆、重量及本次图片状态</label></>}
        <Field label="操作原因" htmlFor="task-operation-reason" required><Input id="task-operation-reason" required value={reason} onChange={(event) => setReason(event.target.value)} /></Field>
        {error && <p role="alert" className="text-error">{error}</p>}
      </div>
    </FormDialog>
  </>;
}
