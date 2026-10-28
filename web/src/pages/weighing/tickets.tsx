import { useWeighingPage, useWeighingSummary, downloadWeighingCsv } from '@/hooks/use-weighing-page';
import type { WireTicket } from '@/api/weighing-workflow';
import { WorkbenchLink } from "@/components/common/workbench-link";
import { useDeferredValue, useEffect, useState } from "react";
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
import type { DataTableColumn } from "@/types";
import { directionLabels, printStatusLabels, type WeighingTicket, type WeighingFilter } from "@/types/weighing";
import { DeviceBanner, HistoryList, WeighingFilters, ListDate } from "./shared";
import { BusinessSnapshot, WeighRecords } from "./record-info";
import { TicketPrintActions } from "./ticket-print-actions";
import { TicketSheet } from "./ticket-sheet";
import { TicketReopenDialog } from "./ticket-reopen-dialog";

import { originalWeights } from "@/lib/weighing/weights";
import { WeightAdjustmentDetails } from "./deduction-fields";
import { formatWeight, needsAdjustmentReview } from "@/lib/weighing/weights";

const linkClass = "text-primary hover:underline";
type TicketOperation = "void" | "reopen";
const operationTitles: Record<"void", string> = { void: "作废单据" };

function TicketWeights({ ticket }: { ticket: WeighingTicket }) {
  const original = ticket.adjustment?.original ?? originalWeights(ticket.records);
  const final = { gross: ticket.gross, tare: ticket.tare, net: ticket.net, sourceKg: ticket.sourceKg };
  return <div className="space-y-3">{needsAdjustmentReview(original, final) && <p role="alert" className="rounded-control border border-warning-border bg-warning-background px-3 py-2 text-sm text-warning">旧记录换算为吨并保留三位小数后出现重量关系异常，请核对迁移前来源重量。</p>}<div className="grid gap-space-4 sm:grid-cols-3"><DetailItem label="最终毛重" value={formatWeight(ticket.gross)} /><DetailItem label="最终皮重" value={formatWeight(ticket.tare)} /><DetailItem label="最终净重" value={<strong className="tabular-nums">{formatWeight(ticket.net)}</strong>} /></div></div>;
}

export function WeighingTicketsPage() {
  const capabilities = useWeighingStore(state => state.capabilities);
  const [params] = useSearchParams();
  const [filter, setFilter] = useState<WeighingFilter>({});
  const deferred = useDeferredValue(filter);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [reopenTicketId, setReopenTicketId] = useState<string | null>(null);
  const result = useWeighingPage<WireTicket>('tickets', deferred, page, pageSize);
  const summary = useWeighingSummary('tickets', deferred, result.revision);
  const can = (code: string) => capabilities?.permissionCodes.includes(`weighing:${code}`) ?? false;
  const canReopenTicket = (row: WireTicket) => row.status === 'void'
    && can('reopen')
    && (capabilities?.sites.some(site => site.canOperate) ?? false);
  const download = (kind: string) => void downloadWeighingCsv(kind, filter).then(() => toast.success("过磅单 CSV 已导出")).catch(error => toast.error({ title: "导出失败", description: error.message }));
  const columns: DataTableColumn<WireTicket>[] = [
    { title: '单号 / 车牌', key: 'ticket', width: 220, render: (_, row) => <div><Link className={linkClass} to={`/weighing/tickets/${row.id}`}>{row.ticketNo}</Link><div className="mt-1 font-medium">{row.business.plate}</div></div> },
    {title:"厂区",key:"site",width:100,render:(_,row)=>row.siteName||row.siteCode||"—"},
    { title: '方向 / 物料规格', key: 'business', width: 180, render: (_, row) => <div>{directionLabels[row.business.direction]} · {row.business.materialName}<div className="text-text-tertiary">{row.business.materialSpec || '—'} · {row.business.materialCode}</div></div> },
    { title: '供收方', key: 'party', width: 160, render: (_, row) => row.business.partyName },
    { title: '最终毛 / 皮 / 净重（吨）', key: 'weight', width: 220, render: (_, row) => <span className="tabular-nums">{row.finalGross} / {row.finalTare} / <strong>{row.finalNet}</strong></span> },
    { title: '状态 / 打印', key: 'status', width: 160, render: (_, row) => <div><StatusTag tone={row.status === 'valid' ? 'success' : 'neutral'}>{row.status === 'valid' ? '有效' : '作废'}</StatusTag><div className="mt-1 text-text-tertiary">{printStatusLabels[row.printStatus ?? 'unprinted']} · 补打 {row.reprintCount ?? 0} 次</div></div> },
    { title: '同步状态', key: 'sync', width: 110, render: (_, row) => <StatusTag tone={row.syncStatus === 'synced' ? 'success' : row.syncStatus === 'failed' ? 'error' : row.syncStatus === 'pending' ? 'warning' : 'neutral'}>{row.syncStatus === 'synced' ? '已同步' : row.syncStatus === 'failed' ? '同步失败' : row.syncStatus === 'pending' ? '待同步' : '未纳入同步'}</StatusTag> },
    { title: '出单时间', key: 'date', width: 144, render: (_, row) => <ListDate value={row.issuedAt} /> },
    { title: '操作', key: 'actions', width: 168, nowrap: true, render: (_, row) => <TableActions><Link className={linkClass} to={`/weighing/tickets/${row.id}`}>详情</Link>{can('print') && <Link className={linkClass} to={`/weighing/tickets/${row.id}/preview`}>预览</Link>}{canReopenTicket(row) && <button type="button" className={linkClass} onClick={() => setReopenTicketId(String(row.id))}>重开</button>}</TableActions> },
  ];
  if (params.get('ticket')) return <Navigate replace to={`/weighing/tickets/${encodeURIComponent(params.get('ticket')!)}`} />;
  return <div className="weighing-list-page"><PageHeader title="最终单据" description="" /><DeviceBanner compact />
    <WeighingFilters value={filter} onChange={value => { setFilter(value); setPage(1); }} ticket />
    {summary.error && <p role="alert" className="text-error">{summary.error}</p>}
    <DataTableCard className="weighing-list-table"><TableToolbar className="!py-space-3" title="单据" description={`筛选 ${result.total} 张；当前有效 ${summary.result?.taskCount ?? '—'} 趟，最终净重合计 ${summary.result?.net ?? '—'} 吨`} actions={<><Button size="sm" onClick={result.refresh}>刷新</Button>{can('export') && <><Button size="sm" onClick={() => download('tickets')}>导出明细 CSV</Button><Button size="sm" onClick={() => download('tickets-summary')}>导出净重汇总 CSV</Button></>}</>} />
      <DataTable compact stickyHeader fixedEdges ariaLabel="最终单据列表" className="weighing-list-scroll" columns={columns} dataSource={result.records} loading={result.loading} error={result.error} rowKey="id" minWidth={1304} />
      <Pagination page={page} pageSize={pageSize} total={result.total} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
    <TicketReopenDialog ticketId={reopenTicketId} onClose={() => setReopenTicketId(null)} />
  </div>;
}

export function WeighingTicketDetailPage() {
  const { id } = useParams();
  const { data, dispatch, capabilities, loadTask, loadTicket } = useWeighingStore();
  const [operation, setOperation] = useState<"void" | null>(null);
  const [reopenOpen, setReopenOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");
  const ticket = data.tickets.find((item) => item.id === id);
  const canOperate = !!capabilities?.permissionCodes.some(code => ["weighing:void","weighing:reopen","weighing:print","weighing:reprint"].includes(code));
  const canPrint = capabilities?.permissionCodes.includes("weighing:print") ?? false;

  useEffect(() => {
    if (!id) return;
    void (async () => { await loadTicket(id); const found=useWeighingStore.getState().data.tickets.find(t=>t.id===id);if(found) await loadTask(found.taskId); })().catch(failure=>setError(failure instanceof Error?failure.message:'详情加载失败'));
  }, [id, loadTicket, loadTask]);
  const can = (code: string) => (code==='print'||!!capabilities?.sites.some(s=>s.canOperate))&&(capabilities?.permissionCodes.includes(`weighing:${code}`) ?? false);

  function startOperation(next: TicketOperation) {
    if (!ticket) return;
    if (next === "reopen") {
      setReopenOpen(true);
      return;
    }
    setReason(""); setError(""); setOperation(next);
  }

  async function submitOperation() {
    if (!ticket || operation !== "void") return;
    try {
      await dispatch({ type: "voidTicket", ticketId: ticket.id, reason });
      setOperation(null); toast.success("单据已作废，原单据及历史记录已保留。");
    } catch (failure) { setError(failure instanceof Error ? failure.message : "操作失败"); }
  }

  if (!ticket) return <><PageHeader title="过磅单详情" actions={<Link className={linkClass} to="/weighing/tickets">返回单据列表</Link>} /><ContentCard><p>未找到该单据，请刷新或核对单据编号。</p></ContentCard></>;
  const successors = data.tickets.filter((item) => item.previousId === ticket.id);
  const canReopen = successors.length === 0 && !data.tickets.some((item) => item.taskId === ticket.taskId && item.status === "valid");
  const pending = data.pending?.taskId === ticket.taskId;
  return <>
    <PageHeader title="过磅单详情" description={ticket.ticketNo ?? ticket.id} actions={<><Link className={linkClass} to="/weighing/tickets">返回单据列表</Link>{canPrint && <Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}/preview`}>打印预览</Link>}</>} />
    <DeviceBanner />
    <div className="space-y-space-6">
      <TicketPrintActions ticketId={ticket.id} />
      <ContentCard title="单据状态与重量" extra={<StatusTag tone={pending ? "warning" : ticket.status === "valid" ? "success" : "neutral"}>{pending ? "提交结果待核实" : ticket.status === "valid" ? "有效单据" : "已作废单据"}</StatusTag>}>
        {pending && <p className="mb-space-4 text-warning">提交结果尚未核实，请<WorkbenchLink className={linkClass} to={`/weighing/workbench?task=${encodeURIComponent(ticket.taskId)}`}>前往工作台查询结果</WorkbenchLink>。</p>}
        <div className="mb-space-4 grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4"><DetailItem label="车牌" value={ticket.plate} /><DetailItem label="业务" value={directionLabels[ticket.direction]} /><DetailItem label={ticket.direction === 'inbound' ? '供货方' : '收货方'} value={ticket.partyName} /><DetailItem label="物料" value={[ticket.materialName, ticket.materialSpec].filter(Boolean).join(' / ')} /></div>
        <TicketWeights ticket={ticket} />
        <div className="mt-space-4 grid gap-space-4 border-t border-border pt-space-4 sm:grid-cols-3"><DetailItem label="出单时间" value={beijingTime(ticket.issuedAt)} /><DetailItem label="打印状态" value={printStatusLabels[ticket.printStatus]} /><DetailItem label="补打次数" value={ticket.reprintCount} /></div>
        {canOperate && !pending && <div className="mt-space-4 flex flex-wrap items-center gap-space-2">{ticket.status === "valid" ? can("void") && <Button size="sm" variant="danger" onClick={() => startOperation("void")}>作废单据</Button> : canReopen && can("reopen") ? <Button size="sm" onClick={() => startOperation("reopen")}>关联更正重开</Button> : <span className="text-text-tertiary">已有更正单，请沿关联记录办理。</span>}{ticket.reprintAuthorized && <StatusTag tone="warning">已登记一次补打</StatusTag>}</div>}
        {ticket.printStatus === "unknown" && <p className="mt-space-4 text-warning">上次打印结果未知，重新打印可能重复出纸。</p>}
      </ContentCard>
      <details className="rounded-admin border border-border bg-surface p-card"><summary className="cursor-pointer font-medium">展开原始依据、调整与历史记录</summary><div className="mt-space-4 space-y-space-6">
      <ContentCard title="人工调整与操作记录"><WeightAdjustmentDetails ticket={ticket} /></ContentCard>
      <ContentCard title="本趟业务信息" description="保留出单时的业务快照；旧记录未保存的字段显示“-”。"><BusinessSnapshot value={ticket} /></ContentCard>
      <ContentCard title="关联记录"><div className="grid gap-space-4 sm:grid-cols-3"><DetailItem label="运输任务" value={<Link className={linkClass} to={`/weighing/tasks/${encodeURIComponent(ticket.taskId)}`}>{ticket.taskId}</Link>} /><DetailItem label="关联原单据" value={ticket.previousId ? <Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.previousId)}`}>{ticket.previousId}</Link> : "无"} /><DetailItem label="关联重开单" value={successors.length ? successors.map((item) => <Link key={item.id} className={`${linkClass} mr-space-2`} to={`/weighing/tickets/${encodeURIComponent(item.id)}`}>{item.id}</Link>) : "无"} /></div></ContentCard>
      <ContentCard title="原始称重记录与替代关系"><WeighRecords records={ticket.records} /></ContentCard>
      <ContentCard title="操作历史"><HistoryList entries={ticket.history} /></ContentCard>
      </div></details>
    </div>
    <FormDialog open={!!operation} title={operation ? operationTitles[operation] : ""} onCancel={() => setOperation(null)} onSubmit={submitOperation} submitText="确认保存">
      <div className="space-y-space-4">
        {operation === "void" && <p className="text-error">作废保留原单及称重依据，不恢复任务待称重状态。更正业务信息或人工调整应在作废后关联重开。</p>}
        <Field label="操作原因 / 核实说明" htmlFor="ticket-operation-reason" required><Input id="ticket-operation-reason" required value={reason} onChange={(event) => setReason(event.target.value)} /></Field>
        {error && <p role="alert" className="text-error">{error}</p>}
      </div>
    </FormDialog>
    <TicketReopenDialog ticketId={reopenOpen ? ticket.id : null} onClose={() => setReopenOpen(false)} />
  </>;
}

export function WeighingTicketPreviewPage() {
  const { id } = useParams();
  const { data, capabilities, loadPreview } = useWeighingStore();
  const [error, setError] = useState('');
  const canPrint = capabilities?.permissionCodes.includes('weighing:print') ?? false;
  useEffect(() => {
    if (id && canPrint) void loadPreview(id).catch(failure => setError(failure instanceof Error ? failure.message : '预览加载失败'));
  }, [id, canPrint, loadPreview]);
  const ticket = data.tickets.find(item => item.id === id);
  if (!canPrint) return <><PageHeader title="单据打印预览" /><p>当前账号没有打印预览权限。</p></>;
  return <>
    <PageHeader title="单据打印预览" description="空白五联针打纸 · 241 × 279 mm 三等分 · 单节 241 × 93 mm（含孔边） · 五联一次复写" actions={<Link className={linkClass} to={`/weighing/tickets/${id}`}>返回单据详情</Link>} />
    <DeviceBanner />
    {error && <p role="alert" className="text-error">{error}</p>}
    {ticket && <TicketPrintActions ticketId={ticket.id} />}
    <div className="mt-space-4">
      {ticket?.printSheet ? <TicketSheet sheet={ticket.printSheet} /> : <p role="status">正在加载当版单据快照…</p>}
    </div>
  </>;
}
