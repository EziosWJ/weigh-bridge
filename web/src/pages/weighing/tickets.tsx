import { useDeferredValue, useState } from "react";
import { Link, Navigate, useNavigate, useParams, useSearchParams } from "react-router-dom";
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
import { Select } from "@/components/ui/select";
import { useWeighingStore } from "@/store/weighing-store";
import { beijingTime, downloadCsv, exportSummaryCsv, exportTicketsCsv, filterTickets } from "@/lib/weighing/query";
import type { DataTableColumn } from "@/types";
import { directionLabels, emptyTaskInput, printStatusLabels, type FinalWeightInput, type TaskInput, type TestTicket, type WeighingFilter } from "@/types/weighing";
import { BusinessFields, DemoBanner, HistoryList, WeighingFilters } from "./shared";
import { BusinessSnapshot, WeighRecords } from "./record-info";
import { CapturePhoto } from "./camera-panel";

import { originalWeights } from "@/lib/weighing/weights";
import { WeightAdjustmentDetails, WeightDeductionFields } from "./deduction-fields";
import { formatWeight, needsAdjustmentReview } from "@/lib/weighing/weights";

const linkClass = "text-primary hover:underline";
type TicketOperation = "void" | "reopen" | "authorize" | "resolve";
const operationTitles: Record<TicketOperation, string> = { void: "作废测试单", reopen: "关联更正重开", authorize: "主管授权补打", resolve: "主管核实打印结果" };

function TicketWeights({ ticket }: { ticket: TestTicket }) {
  const original = ticket.adjustment?.original ?? originalWeights(ticket.records);
  const final = { gross: ticket.gross, tare: ticket.tare, net: ticket.net, sourceKg: ticket.sourceKg };
  return <div className="space-y-3">{needsAdjustmentReview(original, final) && <p role="alert" className="rounded-control border border-warning-border bg-warning-background px-3 py-2 text-sm text-warning">旧记录换算为吨并保留三位小数后出现重量关系异常，请核对迁移前来源重量。</p>}<div className="grid gap-space-4 sm:grid-cols-3"><DetailItem label="最终毛重" value={formatWeight(ticket.gross)} /><DetailItem label="最终皮重" value={formatWeight(ticket.tare)} /><DetailItem label="最终净重" value={<strong className="tabular-nums">{formatWeight(ticket.net)}</strong>} /></div></div>;
}

export function WeighingTicketsPage() {
  const data = useWeighingStore((state) => state.data);
  const [params] = useSearchParams();
  const [filter, setFilter] = useState<WeighingFilter>({});
  const deferredFilter = useDeferredValue(filter);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const tickets = filterTickets(data.tickets, deferredFilter);
  const validTickets = tickets.filter((ticket) => ticket.status === "valid");
  const currentPage = Math.min(page, Math.max(1, Math.ceil(tickets.length / pageSize)));
  const canPrint = data.config.role !== "admin";
  const columns: DataTableColumn<TestTicket>[] = [
    { title: "测试单号 / 车牌", key: "ticket", width: 230, render: (_, ticket) => <div><Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}`}>{ticket.id}</Link><div className="mt-space-1 font-medium">{ticket.plate}{ticket.example && <span className="ml-space-2 text-xs text-text-tertiary">示例</span>}</div></div> },
    { title: "方向 / 物料规格", key: "business", width: 190, render: (_, ticket) => <div>{directionLabels[ticket.direction]} · {ticket.materialName}<div className="text-text-tertiary">{ticket.materialSpec || "-"} · {ticket.materialCode || "-"}</div></div> },
    { title: "供收方 / 编码", key: "party", width: 190, render: (_, ticket) => <div>{ticket.partyName}<div className="text-text-tertiary">{ticket.partyCode || "-"}</div></div> },
    { title: "最终毛 / 皮 / 净重（吨）", key: "weight", width: 250, render: (_, ticket) => <span className="tabular-nums">{formatWeight(ticket.gross)} / {formatWeight(ticket.tare)} / <strong>{formatWeight(ticket.net)}</strong></span> },
    { title: "状态 / 打印", key: "status", width: 170, render: (_, ticket) => <div><StatusTag tone={data.pending?.taskId === ticket.taskId ? "warning" : ticket.status === "valid" ? "success" : "neutral"}>{data.pending?.taskId === ticket.taskId ? "提交结果待核实" : ticket.status === "valid" ? "有效" : "作废"}</StatusTag><div className="mt-space-1 text-text-tertiary">{printStatusLabels[ticket.printStatus]} · 补打 {ticket.reprintCount} 次</div></div> },
    { title: "出单时间（北京）", key: "date", width: 190, render: (_, ticket) => beijingTime(ticket.issuedAt) },
    { title: "操作", key: "actions", width: 150, nowrap: true, render: (_, ticket) => <TableActions><Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}`}>详情</Link>{canPrint && <Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}/preview`}>预览</Link>}</TableActions> },
  ];

  if (params.get("ticket")) return <Navigate replace to={`/weighing/tickets/${encodeURIComponent(params.get("ticket")!)}`} />;
  return <>
    <PageHeader title="最终单据" description="测试过磅单最终重量；按北京时间出单日期筛选。" />
    <DemoBanner />
    <WeighingFilters value={filter} onChange={(value) => { setFilter(value); setPage(1); }} ticket />
    <DataTableCard>
      <TableToolbar title="测试单据" description={`筛选 ${tickets.length} 张，有效 ${validTickets.length} 张，最终净重合计 ${formatWeight(validTickets.reduce((sum, ticket) => sum + ticket.net, 0))}（仅供测试核对）。`} actions={data.config.role === "supervisor" && <><Button size="sm" onClick={() => downloadCsv("测试过磅单明细.csv", exportTicketsCsv(tickets))}>导出明细 CSV</Button><Button size="sm" onClick={() => downloadCsv("测试有效单净重汇总.csv", exportSummaryCsv(tickets))}>导出净重汇总 CSV</Button></>} />
      <DataTable columns={columns} dataSource={tickets.slice((currentPage - 1) * pageSize, currentPage * pageSize)} rowKey="id" minWidth={1320} />
      <Pagination page={currentPage} pageSize={pageSize} total={tickets.length} onPageChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
  </>;
}

export function WeighingTicketDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { data, dispatch } = useWeighingStore();
  const [operation, setOperation] = useState<TicketOperation | null>(null);
  const [input, setInput] = useState<TaskInput>(emptyTaskInput);
  const [reason, setReason] = useState("");
  const [evidenceValid, setEvidenceValid] = useState(false);
  const [finalWeights, setFinalWeights] = useState<FinalWeightInput>({ gross: 0, tare: 0 });
  const [printed, setPrinted] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const ticket = data.tickets.find((item) => item.id === id);
  const supervisor = data.config.role === "supervisor";
  const canPrint = data.config.role !== "admin";

  function startOperation(next: TicketOperation) {
    if (!ticket) return;
    setInput({ plate: ticket.plate, direction: ticket.direction, materialId: ticket.materialId, partyId: ticket.partyId, carrierId: ticket.carrierId ?? "", contactName: ticket.contactName ?? "", contactPhone: ticket.contactPhone ?? "", driver: ticket.driver, driverPhone: ticket.driverPhone ?? "", remark: ticket.remark });
    setFinalWeights({ gross: ticket.gross, tare: ticket.tare, reason: ticket.adjustment?.reason ?? "" });
    setReason(""); setEvidenceValid(false); setPrinted(false); setError(""); setOperation(next);
  }

  function submitOperation() {
    if (!ticket || !operation) return;
    try {
      if (operation === "void") dispatch({ type: "voidTicket", ticketId: ticket.id, reason });
      if (operation === "reopen") dispatch({ type: "reopenTicket", ticketId: ticket.id, input, reason, evidenceValid, finalWeights });
      if (operation === "authorize") dispatch({ type: "authorizeReprint", ticketId: ticket.id, reason });
      if (operation === "resolve") dispatch({ type: "resolvePrint", ticketId: ticket.id, reason, printed });
      setOperation(null); setNotice("操作已保存；原测试单及其历史记录保留。");
      if (operation === "reopen") {
        const reopened = useWeighingStore.getState().data.tickets.find((item) => item.previousId === ticket.id);
        if (reopened) navigate(`/weighing/tickets/${encodeURIComponent(reopened.id)}`);
      }
    } catch (failure) { setError(failure instanceof Error ? failure.message : "操作失败"); }
  }

  if (!ticket) return <><PageHeader title="测试过磅单详情" actions={<Link className={linkClass} to="/weighing/tickets">返回单据列表</Link>} /><ContentCard><p>未找到该测试单，记录可能已重置。</p></ContentCard></>;
  const successors = data.tickets.filter((item) => item.previousId === ticket.id);
  const canReopen = successors.length === 0 && !data.tickets.some((item) => item.taskId === ticket.taskId && item.status === "valid");
  const pending = data.pending?.taskId === ticket.taskId;
  return <>
    <PageHeader title="测试过磅单详情" description={ticket.id} actions={<><Link className={linkClass} to="/weighing/tickets">返回单据列表</Link>{canPrint && <Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}/preview`}>打开只读预览</Link>}</>} />
    <DemoBanner />
    <div className="space-y-space-6">
      {notice && <p role="status" className="text-success">{notice}</p>}
      <ContentCard title="单据状态与重量" extra={<StatusTag tone={pending ? "warning" : ticket.status === "valid" ? "success" : "neutral"}>{pending ? "提交结果待核实" : ticket.status === "valid" ? "有效测试单" : "已作废测试单"}</StatusTag>}>
        {pending && <p className="mb-space-4 text-warning">提交结果尚未核实，请<Link className={linkClass} to={`/weighing/workbench?task=${encodeURIComponent(ticket.taskId)}`}>前往工作台查询结果</Link>。</p>}
        <TicketWeights ticket={ticket} />
        <div className="mt-space-4 grid gap-space-4 border-t border-border pt-space-4 sm:grid-cols-3"><DetailItem label="出单时间（北京）" value={beijingTime(ticket.issuedAt)} /><DetailItem label="打印状态" value={printStatusLabels[ticket.printStatus]} /><DetailItem label="补打次数" value={ticket.reprintCount} /></div>
        {supervisor && !pending && <div className="mt-space-4 flex flex-wrap items-center gap-space-2">{ticket.status === "valid" ? <Button size="sm" variant="danger" onClick={() => startOperation("void")}>作废测试单</Button> : canReopen ? <Button size="sm" onClick={() => startOperation("reopen")}>关联更正重开</Button> : <span className="text-text-tertiary">已有更正单，请沿关联记录办理。</span>}{ticket.status === "valid" && ticket.printStatus === "unknown" && <Button size="sm" onClick={() => startOperation("resolve")}>核实打印结果</Button>}{ticket.status === "valid" && ticket.printStatus === "printed" && !ticket.reprintAuthorized && <Button size="sm" onClick={() => startOperation("authorize")}>授权补打</Button>}{ticket.reprintAuthorized && <StatusTag tone="warning">已授权一次补打</StatusTag>}</div>}
        {ticket.printStatus === "unknown" && <p className="mt-space-4 text-warning">打印结果待核实，不能自动重打。请由主管先核实是否已出纸。</p>}
      </ContentCard>
      <ContentCard title="人工扣减与操作记录"><WeightAdjustmentDetails ticket={ticket} /></ContentCard>
      <ContentCard title="本趟业务信息" description="保留出单时的业务快照；旧记录未保存的字段显示“-”。"><BusinessSnapshot value={ticket} /></ContentCard>
      <ContentCard title="关联记录"><div className="grid gap-space-4 sm:grid-cols-3"><DetailItem label="运输任务" value={<Link className={linkClass} to={`/weighing/tasks/${encodeURIComponent(ticket.taskId)}`}>{ticket.taskId}</Link>} /><DetailItem label="关联原测试单" value={ticket.previousId ? <Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.previousId)}`}>{ticket.previousId}</Link> : "无"} /><DetailItem label="关联重开单" value={successors.length ? successors.map((item) => <Link key={item.id} className={`${linkClass} mr-space-2`} to={`/weighing/tickets/${encodeURIComponent(item.id)}`}>{item.id}</Link>) : "无"} /></div></ContentCard>
      <ContentCard title="原始称重记录与替代关系"><WeighRecords records={ticket.records} /></ContentCard>
      <ContentCard title="操作历史"><HistoryList entries={ticket.history} /></ContentCard>
    </div>
    <FormDialog open={!!operation} title={operation ? operationTitles[operation] : ""} onCancel={() => setOperation(null)} onSubmit={submitOperation} submitText="确认保存">
      <div className="space-y-space-4">
        {operation === "void" && <p className="text-error">作废保留原单及称重依据，不恢复任务待称重状态。更正业务信息或人工扣减应在作废后关联重开。</p>}
        {operation === "reopen" && <><p>对照原始称重录入更正后的最终毛皮重，系统计算扣减量；原单及模拟证据保留，并关联原单，同任务最多一张有效测试单。</p><BusinessFields value={input} onChange={setInput} lockDirection />{originalWeights(ticket.records) && <WeightDeductionFields original={originalWeights(ticket.records)!} value={finalWeights} onChange={setFinalWeights} />}<label className="flex items-center gap-space-2"><input type="checkbox" checked={evidenceValid} onChange={(event) => setEvidenceValid(event.target.checked)} />已核验原称重依据仍有效</label></>}
        {operation === "authorize" && <p>补打沿用原测试单号，本次授权仅供一次模拟补打，保留补打次数和原因。</p>}
        {operation === "resolve" && <Field label="核实结果" htmlFor="print-resolution"><Select id="print-resolution" value={printed ? "printed" : "failed"} onChange={(event) => setPrinted(event.target.value === "printed")}><option value="failed">明确失败且确认未出纸，允许重试</option><option value="printed">已出纸或无法排除已出纸，须授权补打</option></Select></Field>}
        <Field label="操作原因 / 核实说明" htmlFor="ticket-operation-reason" required><Input id="ticket-operation-reason" required value={reason} onChange={(event) => setReason(event.target.value)} /></Field>
        {error && <p role="alert" className="text-error">{error}</p>}
      </div>
    </FormDialog>
  </>;
}

export function WeighingTicketPreviewPage() {
  const { id } = useParams();
  const { data, dispatch } = useWeighingStore();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const ticket = data.tickets.find((item) => item.id === id);
  const canPrint = data.config.role !== "admin";

  function simulatePrint() {
    if (!ticket) return;
    try {
      dispatch({ type: "previewPrint", ticketId: ticket.id });
      setError(""); setNotice("已记录模拟打印尝试，未向物理打印机发送任务。");
    } catch (failure) { setError(failure instanceof Error ? failure.message : "模拟打印失败"); }
  }

  if (!ticket) return <><PageHeader title="测试单打印预览" actions={<Link className={linkClass} to="/weighing/tickets">返回单据列表</Link>} /><ContentCard><p>未找到该测试单，记录可能已重置。</p></ContentCard></>;
  if (!canPrint) return <><PageHeader title="测试单打印预览" actions={<Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}`}>返回单据详情</Link>} /><ContentCard><p>请切换操作员或主管演示岗位查看打印预览。</p></ContentCard></>;
  const canAttempt = !data.pending && ticket.status === "valid" && (ticket.printStatus === "unprinted" || ticket.printStatus === "failed" || ticket.reprintAuthorized);
  return <>
    <PageHeader title="测试单打印预览" description="单据内容只读；预览不发送物理打印任务。" actions={<Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}`}>返回单据详情</Link>} />
    <DemoBanner />
    <div className="space-y-space-6">
      <ContentCard title="测试过磅单" description={`测试单号：${ticket.id}`} extra={<StatusTag tone="warning">测试 · 非正式凭证</StatusTag>}>
        {ticket.status === "void" && <p className="mb-space-4 font-semibold text-error">此测试单已作废</p>}
        {data.pending?.taskId === ticket.taskId && <p className="mb-space-4 text-warning">提交结果待核实，请返回工作台查询结果后再办理。</p>}
        <TicketWeights ticket={ticket} />
        <div className="my-space-5 grid gap-space-4 border-y border-border py-space-4 sm:grid-cols-3"><DetailItem label="出单时间（北京）" value={beijingTime(ticket.issuedAt)} /><DetailItem label="补打标记" value={`补打 ${ticket.reprintCount} 次${ticket.reprintAuthorized ? "（下一次补打已授权）" : ""}`} /><DetailItem label="模拟打印状态" value={printStatusLabels[ticket.printStatus]} /></div>
        <BusinessSnapshot value={ticket} />
        <div className="mt-space-5 border-t border-border pt-space-4">
          <h3 className="mb-space-3 text-sm font-medium">本单称重模拟抓拍</h3>
          <div className="grid gap-space-4 sm:grid-cols-2">{(["first", "second"] as const).map((stage) => {
            const record = [...ticket.records].reverse().find((item) => item.stage === stage);
            return <div key={stage}><p className="mb-space-2 text-sm font-medium">{stage === "first" ? "首次称重" : "二次称重"}</p>{record?.capture ? <CapturePhoto capture={record.capture} hideWeight /> : <p className="text-sm text-text-tertiary">旧记录未保存模拟抓拍图片。</p>}</div>;
          })}</div>
        </div>
        <p className="mt-space-6 text-text-tertiary">模拟重量与模拟证据；摄像头、仪表及物理打印机均未真实接入。</p>
      </ContentCard>
      <ContentCard title="模拟打印操作" description="仅验证结果与补打流程，不进行真实打印。">
        <div className="flex flex-wrap items-center gap-space-3"><Button variant="primary" disabled={!canAttempt} onClick={simulatePrint}>{ticket.reprintAuthorized ? "演示授权补打" : ticket.printStatus === "failed" ? "演示失败后重试" : "演示首次打印结果"}</Button><span className="text-text-secondary">演示设置配置结果：{printStatusLabels[data.config.printOutcome]}</span></div>
        {ticket.printStatus === "unknown" && <p className="mt-space-4 text-warning">结果待核实，请返回详情由主管核实；当前不能再次发送模拟打印。</p>}
        {ticket.printStatus === "printed" && !ticket.reprintAuthorized && <p className="mt-space-4 text-text-secondary">如需再次模拟打印，请返回详情由主管填写原因并授权补打。</p>}
        {notice && <p role="status" className="mt-space-4 text-success">{notice}</p>}
        {error && <p role="alert" className="mt-space-4 text-error">{error}</p>}
      </ContentCard>
    </div>
  </>;
}
