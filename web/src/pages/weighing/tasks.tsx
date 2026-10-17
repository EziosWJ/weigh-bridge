import { WeightAdjustmentDetails } from "./deduction-fields";
import { useDeferredValue, useState } from "react";
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
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useWeighingStore } from "@/store/weighing-store";
import { beijingDate, beijingTime, downloadCsv, exportTasksCsv, filterTasks } from "@/lib/weighing/query";
import { formatWeight } from "@/lib/weighing/weights";
import type { DataTableColumn } from "@/types";
import { directionLabels, emptyTaskInput, taskStatusLabels, type TaskInput, type TransportTask, type WeighingFilter } from "@/types/weighing";
import { BusinessFields, DemoBanner, HistoryList, WeighingFilters } from "./shared";
import { BusinessSnapshot, WeighRecords } from "./record-info";

const linkClass = "text-primary hover:underline";
type TaskOperation = "correct" | "reweigh" | "terminate" | "verify";
const operationTitles: Record<TaskOperation, string> = { correct: "主管业务纠错", reweigh: "主管重称", terminate: "终止运输任务", verify: "跨天续称核实" };

export function WeighingTasksPage() {
  const data = useWeighingStore((state) => state.data);
  const [params] = useSearchParams();
  const [filter, setFilter] = useState<WeighingFilter>({});
  const deferredFilter = useDeferredValue(filter);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const tasks = filterTasks(data.tasks, deferredFilter);
  const currentPage = Math.min(page, Math.max(1, Math.ceil(tasks.length / pageSize)));
  const canWeigh = data.config.role !== "admin";
  const columns: DataTableColumn<TransportTask>[] = [
    { title: "任务编号 / 车牌", key: "task", width: 230, render: (_, task) => <div><Link className={linkClass} to={`/weighing/tasks/${encodeURIComponent(task.id)}`}>{task.id}</Link><div className="mt-space-1 font-medium">{task.plate}{task.example && <span className="ml-space-2 text-xs text-text-tertiary">示例</span>}</div></div> },
    { title: "方向", key: "direction", width: 110, render: (_, task) => directionLabels[task.direction] },
    { title: "物料 / 规格", key: "material", width: 200, render: (_, task) => <div>{task.materialName}<div className="text-text-tertiary">{task.materialSpec || "-"} · {task.materialCode || "-"}</div></div> },
    { title: "供收方 / 编码", key: "party", width: 200, render: (_, task) => <div>{task.partyName}<div className="text-text-tertiary">{task.partyCode || "-"}</div></div> },
    { title: "任务状态", key: "status", width: 150, render: (_, task) => <StatusTag tone={data.pending?.taskId === task.id ? "warning" : task.status === "completed" ? "success" : task.status === "terminated" ? "neutral" : "warning"}>{data.pending?.taskId === task.id ? "提交结果待核实" : taskStatusLabels[task.status]}</StatusTag> },
    { title: "创建时间（北京）", key: "date", width: 190, render: (_, task) => beijingTime(task.createdAt) },
    { title: "操作", key: "actions", width: 180, nowrap: true, render: (_, task) => <TableActions><Link className={linkClass} to={`/weighing/tasks/${encodeURIComponent(task.id)}`}>详情</Link>{canWeigh && (data.pending?.taskId === task.id || task.status === "first" || task.status === "second") && <Link className={linkClass} to={`/weighing/workbench?task=${encodeURIComponent(task.id)}`}>{data.pending?.taskId === task.id ? "查询提交结果" : "办理称重"}</Link>}</TableActions> },
  ];

  if (params.get("task")) return <Navigate replace to={`/weighing/tasks/${encodeURIComponent(params.get("task")!)}`} />;
  return <>
    <PageHeader title="运输任务" description="查询测试运输记录；日期按北京时间创建日期筛选。" actions={<Link className={linkClass} to="/weighing/workbench">返回称重工作台</Link>} />
    <DemoBanner />
    <WeighingFilters value={filter} onChange={(value) => { setFilter(value); setPage(1); }} />
    <DataTableCard>
      <TableToolbar title="测试运输任务" description={`当前筛选 ${tasks.length} 条；选择任务进入独立详情。`} actions={data.config.role === "supervisor" && <Button size="sm" onClick={() => downloadCsv("测试运输任务.csv", exportTasksCsv(tasks))}>导出筛选明细 CSV</Button>} />
      <DataTable columns={columns} dataSource={tasks.slice((currentPage - 1) * pageSize, currentPage * pageSize)} rowKey="id" minWidth={1260} />
      <Pagination page={currentPage} pageSize={pageSize} total={tasks.length} onPageChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
  </>;
}

export function WeighingTaskDetailPage() {
  const { id } = useParams();
  const { data, dispatch } = useWeighingStore();
  const [operation, setOperation] = useState<TaskOperation | null>(null);
  const [input, setInput] = useState<TaskInput>(emptyTaskInput);
  const [reason, setReason] = useState("");
  const [sameLoad, setSameLoad] = useState(false);
  const [checked, setChecked] = useState(false);
  const [doubtful, setDoubtful] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [photoPreviewOpen, setPhotoPreviewOpen] = useState(false);
  const [readyCaptureId, setReadyCaptureId] = useState("");
  const task = data.tasks.find((item) => item.id === id);
  const pending = !!task && data.pending?.taskId === task.id;
  const firstRecord = task && [...task.records].reverse().find((record) => record.stage === "first");
  const today = beijingDate(new Date().toISOString());
  const needsDayVerification = !!task && (task.crossDayDoubt || (!!firstRecord && beijingDate(firstRecord.at) !== today && task.verifiedDate !== today));
  const supervisor = data.config.role === "supervisor";
  const canWeigh = data.config.role !== "admin";
  const reweighCapture = data.sampleReady && data.capture?.plate === task?.plate ? data.capture : undefined;
  const reweighImageReady = !!reweighCapture && readyCaptureId === reweighCapture.id;

  function startOperation(next: TaskOperation) {
    if (!task) return;
    setInput({ plate: task.plate, direction: task.direction, materialId: task.materialId, partyId: task.partyId, carrierId: task.carrierId ?? "", contactName: task.contactName ?? "", contactPhone: task.contactPhone ?? "", driver: task.driver, driverPhone: task.driverPhone ?? "", remark: task.remark });
    setReason(""); setSameLoad(false); setChecked(false); setDoubtful(false); setError(""); setPhotoPreviewOpen(false); setReadyCaptureId(""); setOperation(next);
  }

  function submitOperation() {
    if (!task || !operation) return;
    try {
      if (operation === "correct") dispatch({ type: "correctTask", taskId: task.id, input, reason });
      if (operation === "terminate") dispatch({ type: "terminate", taskId: task.id, reason });
      if (operation === "verify") dispatch({ type: "verifyDay", taskId: task.id, reason, doubtful, confirmed: checked });
      if (operation === "reweigh") dispatch({ type: "reweigh", taskId: task.id, reason, sameLoad, checked });
      setOperation(null); setNotice("操作已保存至本浏览器测试数据。");
    } catch (failure) { setError(failure instanceof Error ? failure.message : "操作失败"); }
  }

  if (!task) return <><PageHeader title="运输任务详情" actions={<Link className={linkClass} to="/weighing/tasks">返回任务列表</Link>} /><ContentCard><p>未找到该测试任务，记录可能已重置。</p></ContentCard></>;
  const tickets = data.tickets.filter((ticket) => ticket.taskId === task.id);
  return <>
    <PageHeader title="运输任务详情" description={task.id} actions={<><Link className={linkClass} to="/weighing/tasks">返回任务列表</Link>{canWeigh && (pending || ["first", "second"].includes(task.status)) && <Link className={linkClass} to={`/weighing/workbench?task=${encodeURIComponent(task.id)}`}>{pending ? "前往工作台查询提交结果" : "前往工作台办理"}</Link>}</>} />
    <DemoBanner />
    <div className="space-y-space-6">
      {notice && <p role="status" className="text-success">{notice}</p>}
      <ContentCard title="任务状态" extra={<StatusTag tone={pending ? "warning" : task.status === "completed" ? "success" : "neutral"}>{pending ? "提交结果待核实" : taskStatusLabels[task.status]}</StatusTag>}>
        {pending && <p role="alert" className="mb-space-4 text-warning">上次提交结果尚未核实。当前保存的模拟记录不表示已核实成功；请先到工作台查询结果，再继续办理。</p>}
        <div className="grid gap-space-4 sm:grid-cols-3"><DetailItem label="创建时间（北京）" value={beijingTime(task.createdAt)} /><DetailItem label="跨天核实日期（北京）" value={task.verifiedDate} /><DetailItem label="跨天疑点" value={task.crossDayDoubt ? "有疑点，需主管核查" : "无"} /></div>
        {!pending && task.status !== "completed" && task.status !== "terminated" && <div className="mt-space-4 flex flex-wrap gap-space-2">{supervisor && <><Button size="sm" onClick={() => startOperation("correct")}>业务纠错</Button>{task.records.length > 0 && <Button size="sm" onClick={() => startOperation("reweigh")}>重称</Button>}<Button size="sm" variant="danger" onClick={() => startOperation("terminate")}>终止任务</Button></>}{canWeigh && task.status === "second" && <Button size="sm" onClick={() => startOperation("verify")}>跨天续称核实</Button>}</div>}
      </ContentCard>
      <ContentCard title="本趟业务信息" description="保留当时的资料与联系信息快照；旧记录未保存的字段显示“-”。"><BusinessSnapshot value={task} /></ContentCard>
      <ContentCard title="原始称重记录与替代关系"><WeighRecords records={task.records} /></ContentCard>
      <ContentCard title="关联测试单">{tickets.length ? <div className="space-y-space-2">{tickets.map((ticket) => <div key={ticket.id}><Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}`}>{ticket.id}</Link><span className="ml-space-3 text-text-secondary">{ticket.status === "valid" ? "有效" : "作废"}{ticket.previousId ? ` · 更正自 ${ticket.previousId}` : ""}</span></div>)}</div> : <p className="text-text-tertiary">尚无关联测试单。</p>}</ContentCard>
      <ContentCard title="单据扣减追溯">{tickets.length ? tickets.map(ticket => <div key={ticket.id} className="mb-5 border-b border-border pb-4 last:border-0"><p className="mb-3 text-sm font-medium">{ticket.id} · {ticket.status === "valid" ? "有效" : "作废"}</p><WeightAdjustmentDetails ticket={ticket} /></div>) : <p className="text-text-tertiary">尚未出单，无已确认扣减。</p>}</ContentCard>
      <ContentCard title="操作历史"><HistoryList entries={task.history} /></ContentCard>
    </div>
    <FormDialog open={!!operation} title={operation ? operationTitles[operation] : ""} onCancel={() => setOperation(null)} onSubmit={submitOperation} submitDisabled={operation === "reweigh" && (needsDayVerification || !reweighImageReady || !checked || !sameLoad)} submitText={operation === "reweigh" ? "确认当前采集重称" : "确认保存"} trapFocus={!photoPreviewOpen} closeOnEscape={!photoPreviewOpen} closeOnOverlayClick={!photoPreviewOpen}>
      <div className="space-y-space-4">
        {operation === "correct" && <BusinessFields value={input} onChange={setInput} lockDirection={task.records.length > 0} />}
        {operation === "terminate" && <p className="text-error">终止后不可恢复；已有称重记录保留，车牌可办理新任务。</p>}
        {operation === "reweigh" && needsDayVerification && <div className="space-y-space-2"><p className="text-warning">首次称重已跨天或原运输存在疑点，请先核实首次依据，再重新采集和办理重称。</p><Button onClick={() => startOperation("verify")}>先办理跨天续称核实</Button></div>}
        {operation === "reweigh" && <><p>演示设置当前重量：{formatWeight(data.config.weight)}。先重新采集，再人工核对；原记录和替代关系保留。</p><Button onClick={() => { try { dispatch({ type: "capture", taskId: task.id }); setChecked(false); setReadyCaptureId(""); setError(""); } catch (failure) { setError(failure instanceof Error ? failure.message : "采集失败"); } }}>重新采集模拟重量</Button><p className="text-text-secondary">当前采集：{reweighCapture ? "有效，需核对本次模拟图片" : "未采集或已失效"}</p>
          {reweighCapture && <figure className="max-w-sm space-y-space-2"><img key={reweighCapture.id} src={reweighCapture.imageSrc} alt={`${reweighCapture.cameraLabel}模拟抓拍，车牌${reweighCapture.plate}`} className="w-full rounded-control border border-border object-contain" onLoad={() => setReadyCaptureId(reweighCapture.id)} onError={() => { setReadyCaptureId(""); setChecked(false); setError("模拟图片加载失败，请恢复后重新采集。"); }} /><figcaption className="text-xs text-text-tertiary">模拟图片 · 摄像头未接入 · {reweighCapture.cameraLabel}<br />{reweighCapture.plate} · {formatWeight(reweighCapture.weight)} · {beijingTime(reweighCapture.at)}（北京时间）</figcaption></figure>}
          <label className="flex items-center gap-space-2"><input type="checkbox" checked={sameLoad} onChange={(event) => setSameLoad(event.target.checked)} />已确认车辆装载状态仍符合原称重条件</label><label className="flex items-center gap-space-2"><input type="checkbox" checked={checked} disabled={!reweighImageReady} onChange={(event) => setChecked(event.target.checked)} />已人工核对车辆、重量和本次模拟抓拍图片</label></>}
        {operation === "verify" && <><p>请核对首次称重依据及是否仍属于原运输；无法确认时由主管终止任务。</p><p>{task.plate} · {directionLabels[task.direction]} · {task.materialName} · {task.partyName}</p><WeighRecords records={task.records.filter((record) => record.stage === "first")} onPreviewChange={setPhotoPreviewOpen} /><label className="flex items-center gap-space-2"><input type="checkbox" checked={doubtful} onChange={(event) => setDoubtful(event.target.checked)} />原运输存在疑点，提交主管核查</label><label className="flex items-center gap-space-2"><input type="checkbox" checked={checked} onChange={(event) => setChecked(event.target.checked)} />已核实仍属原运输，允许续称</label></>}
        <Field label={operation === "verify" ? "核实说明" : "操作原因"} htmlFor="task-operation-reason" required><Input id="task-operation-reason" required value={reason} onChange={(event) => setReason(event.target.value)} /></Field>
        {error && <p role="alert" className="text-error">{error}</p>}
      </div>
    </FormDialog>
  </>;
}
