import { useTransportDirections } from '@/hooks/use-transport-directions';
import { useWeighingPage, useWeighingSummary, downloadWeighingCsv } from '@/hooks/use-weighing-page';
import type { WireTicket } from '@/api/weighing-workflow';
import { workflowApi } from '@/api/weighing-workflow';
import { WorkbenchLink } from "@/components/common/workbench-link";
import { useDeferredValue, useEffect, useState } from "react";
import { Link, Navigate, useParams, useSearchParams } from "react-router-dom";
import { HelpHint } from "@/components/common/help-hint";
import { ContentCard } from "@/components/common/content-card";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailItem } from "@/components/common/detail-item";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { StatusTag } from "@/components/common/status-tag";
import { TableActionMenu, TableActions } from "@/components/common/table-actions";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { buttonStyles } from "@/components/ui/button-styles";
import { Input } from "@/components/ui/input";
import { useWeighingStore } from "@/store/weighing-store";
import { beijingTime } from "@/lib/weighing/query";
import { formatDateTime } from "@/lib/datetime";
import { reconcileLabels, ticketSyncLabel } from "./sync/labels";
import type { DataTableColumn } from "@/types";
import { printStatusLabels, supplementReasonLabels, weighingSourceLabel, activeWeighRecord, type WeighingTicket, type WeighingFilter, type TaskTripBasis } from "@/types/weighing";
import { HistoryList, WeighingFilters, ListDate } from "./shared";
import { BusinessSnapshot, WeighRecords } from "./record-info";
import { TicketPrintActions } from "./ticket-print-actions";
import { TicketSheet } from "./ticket-sheet";
import { TicketReopenDialog } from "./ticket-reopen-dialog";

import { originalWeights } from "@/lib/weighing/weights";
import { WeightAdjustmentDetails } from "./deduction-fields";
import { formatWeight, needsAdjustmentReview, netAdjustmentText } from "@/lib/weighing/weights";

const linkClass = "text-primary hover:underline";
type TicketOperation = "void" | "reopen";

function TicketWeights({ ticket }: { ticket: WeighingTicket }) {
  const original = ticket.adjustment?.original ?? originalWeights(ticket.records);
  const final = { gross: ticket.gross, tare: ticket.tare, net: ticket.net, sourceKg: ticket.sourceKg };
  return <div className="space-y-3">{needsAdjustmentReview(original, final) && <p role="alert" className="rounded-control border border-warning-border bg-warning-background px-3 py-2 text-sm text-warning">旧记录换算为吨并保留三位小数后出现重量关系异常，请核对迁移前来源重量。</p>}<div className="grid gap-space-4 sm:grid-cols-4"><DetailItem label="最终毛重" value={formatWeight(ticket.gross)} /><DetailItem label="最终皮重" value={formatWeight(ticket.tare)} /><DetailItem label="净重调整" value={netAdjustmentText(ticket.netAdjustment ?? 0)} /><DetailItem label="最终净重" value={<strong className="tabular-nums">{formatWeight(ticket.net)}</strong>} /></div></div>;
}

export function WeighingTicketsPage() {
  const directionLabels = useTransportDirections();
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
  const canSyncAdmin = capabilities?.permissionCodes.includes('weighing:sync:tickets') ?? false;
  const canReopenTicket = (row: WireTicket) => row.status === 'void'
    && can('reopen')
    && (capabilities?.sites.some(site => site.canOperate) ?? false);
  const download = (kind: string) => void downloadWeighingCsv(kind, filter).then(() => toast.success("过磅单 CSV 已导出")).catch(error => toast.error({ title: "导出失败", description: error.message }));
  const columns: DataTableColumn<WireTicket>[] = [
    { title: '车牌 / 单号', key: 'ticket', width: 195, render: (_, row) => <div className="break-words"><div className="font-medium">{row.business.plate}</div><Link className={linkClass} to={`/weighing/tickets/${row.id}`}>{row.ticketNo}</Link></div> },
    { title: '业务', key: 'business', width: 202, render: (_, row) => <div className="break-words">
      <div>{[row.business.materialName, row.business.materialSpec].filter(Boolean).join(' / ')}</div>
      <div className="text-xs text-text-secondary">{row.business.partyName}</div>
      <div className="text-xs text-text-tertiary">{directionLabels[row.business.direction]}</div>
    </div> },
    { title: '最终净重（吨）', key: 'weight', width: 115, align: 'right', render: (_, row) => <strong className="tabular-nums">{row.finalNet}</strong> },
    { title: '状态 / 打印', key: 'status', width: 125, render: (_, row) => <div>
      <StatusTag tone={row.status === 'valid' ? 'success' : 'neutral'}>{row.status === 'valid' ? '有效' : '作废'}</StatusTag>
      <div className="mt-1 text-xs text-text-secondary">{printStatusLabels[row.printStatus ?? 'unprinted']}</div>
    </div> },
    { title: '来源 / 同步', key: 'sync', width: 128, render: (_, row) => {
      const item = ticketSyncLabel(row.syncStatus, capabilities?.masterSyncEnabled, capabilities?.masterSyncBound, row.syncReason);
      const source = weighingSourceLabel(row.entryType, row.basis?.find(record => record.stage === 'first')?.source, row.basis?.find(record => record.stage === 'second')?.source);
      return <div><div className="mb-1">{source ?? '—'}</div><StatusTag tone={item.tone}>{item.label}</StatusTag></div>;
    } },
    { title: '出单时间', key: 'date', width: 102, render: (_, row) => <ListDate value={row.issuedAt} /> },
    {
      title: '操作', key: 'actions', width: 104, nowrap: true,
      render: (_, row) => (
        <TableActions className="gap-1">
          {can('print')
            ? <Link className={buttonStyles({ size: "sm", className: "px-2" })} to={`/weighing/tickets/${row.id}/preview`}>预览</Link>
            : <Link className={buttonStyles({ size: "sm", variant: "ghost", className: "px-2" })} to={`/weighing/tickets/${row.id}`}>详情</Link>}
          <TableActionMenu compact
            label={`${row.ticketNo} 更多操作`}
            items={[
              ...(canSyncAdmin ? [{ label: '同步详情', to: `/weighing/sync/tickets/${row.id}` }] : []),
              ...(canReopenTicket(row) ? [{ label: '更正重开', onSelect: () => setReopenTicketId(String(row.id)) }] : []),
            ]}
          />
        </TableActions>
      ),
    },
  ];
  if (params.get('ticket')) return <Navigate replace to={`/weighing/tickets/${encodeURIComponent(params.get('ticket')!)}`} />;
  return <div className="weighing-list-page"><PageHeader title="最终单据" />
    <WeighingFilters value={filter} onChange={value => { setFilter(value); setPage(1); }} ticket />
    {summary.error && <p role="alert" className="text-error">{summary.error}</p>}
    <DataTableCard className="weighing-list-table"><TableToolbar className="!py-space-3" title="单据" description={`筛选 ${result.total} 张；当前有效 ${summary.result?.taskCount ?? '—'} 趟，最终净重合计 ${summary.result?.net ?? '—'} 吨（净重调整合计 ${summary.result?.adjustment ?? '—'} 吨）`} actions={<><Button size="sm" onClick={result.refresh}>刷新</Button>{can('export') && <><Button size="sm" onClick={() => download('tickets')}>导出明细 CSV</Button><Button size="sm" onClick={() => download('tickets-summary')}>导出净重汇总 CSV</Button></>}</>} />
      <DataTable compact stickyHeader fixedEdges ariaLabel="最终单据列表" className="weighing-list-scroll" columns={columns} dataSource={result.records} loading={result.loading} error={result.error} rowKey="id" minWidth={971} />
      <Pagination page={page} pageSize={pageSize} total={result.total} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
    <TicketReopenDialog ticketId={reopenTicketId} onClose={() => setReopenTicketId(null)} />
  </div>;
}

export function WeighingTicketDetailPage() {
  const directionLabels = useTransportDirections();
  const { id } = useParams();
  const { data, dispatch, capabilities, loadTask, loadTicket } = useWeighingStore();
  const [operation, setOperation] = useState<"void" | null>(null);
  const [reopenOpen, setReopenOpen] = useState(false);
  const [reason, setReason] = useState("");
  const busy = useWeighingStore(state => state.busy);
  const [error, setError] = useState("");
  const ticket = data.tickets.find((item) => item.id === id);
  const [tripBasis, setTripBasis] = useState<TaskTripBasis | null>(null);
  const canOperate = !!capabilities?.permissionCodes.some(code => ["weighing:void","weighing:reopen","weighing:print","weighing:reprint"].includes(code));
  const canPrint = capabilities?.permissionCodes.includes("weighing:print") ?? false;

  useEffect(() => {
    if (!id) return;
    void (async () => { await loadTicket(id); const found=useWeighingStore.getState().data.tickets.find(t=>t.id===id);if(found) await loadTask(found.taskId); })().catch(failure=>setError(failure instanceof Error?failure.message:'详情加载失败'));
  }, [id, loadTicket, loadTask]);
  useEffect(() => {
    let cancelled = false;
    if (!ticket?.taskId) { setTripBasis(null); return; }
    workflowApi.taskTripBasis(ticket.taskId)
      .then(value => { if (!cancelled) setTripBasis(value); })
      .catch(() => { if (!cancelled) setTripBasis(null); });
    return () => { cancelled = true; };
  }, [ticket?.taskId]);
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
    if (!ticket || operation !== "void" || !reason.trim() || busy) return;
    try {
      await dispatch({ type: "voidTicket", ticketId: ticket.id, reason: reason.trim() });
      setOperation(null);
      if (useWeighingStore.getState().data.pending) toast.warning("作废结果待核实，请查询原请求");
      else toast.success("单据已作废");
    } catch (failure) {
      if (useWeighingStore.getState().data.pending?.taskId === ticket.taskId) {
        setOperation(null);
        toast.warning("作废结果待核实，请查询原请求");
      } else setError(failure instanceof Error ? failure.message : "操作失败");
    }
  }

  const backToTickets = <Link className={buttonStyles({ size: "sm" })} to="/weighing/tickets">返回单据列表</Link>;
  if (!ticket) return <><PageHeader title="过磅单详情" actions={backToTickets} /><ContentCard>{error ? <p role="alert" className="text-error">单据详情加载失败。{error}</p> : <p role="status">正在加载单据详情…</p>}</ContentCard></>;
  const successors = data.tickets.filter((item) => item.previousId === ticket.id);
  const canReopen = successors.length === 0 && !data.tickets.some((item) => item.taskId === ticket.taskId && item.status === "valid");
  const pending = data.pending?.taskId === ticket.taskId;
  const task = data.tasks.find(item => item.id === ticket.taskId);
  const previous = data.tickets.find(item => item.id === ticket.previousId);
  const source = weighingSourceLabel(ticket.entryType, activeWeighRecord(ticket.records, 'first')?.source, activeWeighRecord(ticket.records, 'second')?.source);
  const sync = ticketSyncLabel(ticket.syncStatus, capabilities?.masterSyncEnabled, capabilities?.masterSyncBound, ticket.syncReason);
  const reconcile = ticket.reconcileStatus ? reconcileLabels[ticket.reconcileStatus] : undefined;
  const events = task?.history.filter(entry => entry.ticketId === ticket.id && ['ticket.reopen', 'ticket.void'].includes(entry.action)) ?? [];
  const history = ticket.history.map(entry => {
    const event = events.find(event => event.action === (entry.action === '更正重开' ? 'ticket.reopen' : entry.action === '单据作废' ? 'ticket.void' : ''));
    return event ? { ...entry, ...event, action: entry.action } : entry;
  });
  return <>
    <PageHeader title={ticket.ticketNo ?? "过磅单详情"} actions={<>{backToTickets}{canPrint && <Link className={buttonStyles({ variant: "primary", size: "sm" })} to={`/weighing/tickets/${encodeURIComponent(ticket.id)}/preview`}>打印预览</Link>}</>} />
    <div className="space-y-space-6">
      <TicketPrintActions ticketId={ticket.id} />
      <ContentCard title="单据状态与重量" extra={<StatusTag tone={pending ? "warning" : ticket.status === "valid" ? "success" : "neutral"}>{pending ? "提交结果待核实" : ticket.status === "valid" ? "有效单据" : "已作废单据"}</StatusTag>}>
        {pending && <p className="mb-space-4 text-warning">提交结果尚未核实，请<WorkbenchLink className={linkClass} to={`/weighing/workbench?task=${encodeURIComponent(ticket.taskId)}`}>前往工作台查询结果</WorkbenchLink>。</p>}
        <div className="mb-space-4 grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4"><DetailItem label="车牌" value={ticket.plate} /><DetailItem label="业务" value={directionLabels[ticket.direction]} /><DetailItem label={ticket.direction === 'inbound' ? '供货方' : '收货方'} value={ticket.partyName} /><DetailItem label="物料" value={[ticket.materialName, ticket.materialSpec].filter(Boolean).join(' / ')} /></div>
        <TicketWeights ticket={ticket} />
        <div className="mt-space-4 grid gap-space-4 border-t border-border pt-space-4 sm:grid-cols-4"><DetailItem label="单据来源" value={source} /><DetailItem label="出单时间" value={beijingTime(ticket.issuedAt)} /><DetailItem label="打印状态" value={printStatusLabels[ticket.printStatus]} /><DetailItem label="补打次数" value={ticket.reprintCount} /></div>
        {canOperate && !pending && <div className="mt-space-4 flex flex-wrap items-center gap-space-2">{ticket.status === "valid" ? can("void") && <Button size="sm" variant="danger" onClick={() => startOperation("void")}>作废单据</Button> : canReopen ? can("reopen") && <Button size="sm" onClick={() => startOperation("reopen")}>关联更正重开</Button> : <span className="text-text-tertiary">已有更正单，请沿关联记录办理。</span>}{ticket.reprintAuthorized && <StatusTag tone="warning">已登记一次补打</StatusTag>}</div>}
        {ticket.entryType === 'manual_supplement' && <div className="mt-space-4"><DetailItem label="补单原因" value={supplementReasonLabels[ticket.supplementReason ?? ''] ?? ticket.supplementReason} /></div>}
      </ContentCard>
      {tripBasis?.bound && <ContentCard title="云端流转单与化验依据">
        <div className="grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">
          <DetailItem label="流转单" value={tripBasis.tripNo || '—'} />
          <DetailItem label="化验单" value={tripBasis.inspection ? `HY-${String(tripBasis.inspection.id).padStart(6, '0')}` : '目测接收'} />
          <DetailItem label="绑定状态" value={{ bound: '首磅已绑定', ticketed: '已出单，待云端确认卸货后关联', linked: '云端已关联', conflict: '关联需核查', superseded: '已被新单替代' }[tripBasis.status ?? ''] ?? tripBasis.status ?? '—'} />
          <DetailItem label="云端磅单号" value={tripBasis.cloudTicketNo || '待云端确认'} />
        </div>
        {tripBasis.inspection && <div className="mt-space-4 grid gap-space-4 sm:grid-cols-2">
          <DetailItem label="水分" value={`${tripBasis.inspection.moisture}%`} />
          <DetailItem label="容量" value={tripBasis.inspection.bulkDensity ? `${tripBasis.inspection.bulkDensity} g/L` : '—'} />
        </div>}
        {tripBasis.status === 'ticketed' && <p className="mt-space-4 text-sm text-warning">请云端库管确认实际卸货；确认后系统自动关联磅单，未确认前无法开证。</p>}
      </ContentCard>}
      <details className="rounded-admin border border-border bg-surface p-card"><summary className="cursor-pointer font-medium">展开原始依据、调整与历史记录</summary><div className="mt-space-4 space-y-space-6">
      <ContentCard title="人工调整与操作记录"><WeightAdjustmentDetails ticket={ticket} /></ContentCard>
      <ContentCard title="出单业务快照"><BusinessSnapshot value={ticket} /></ContentCard>
      <ContentCard title="关联记录"><div className="grid gap-space-4 sm:grid-cols-3"><DetailItem label="运输任务" value={<Link className={linkClass} to={`/weighing/tasks/${encodeURIComponent(ticket.taskId)}`}>{task?.taskNo ?? ticket.taskId}</Link>} /><DetailItem label="关联原单据" value={ticket.previousId ? <Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(ticket.previousId)}`}>{previous?.ticketNo ?? ticket.previousId}</Link> : "无"} /><DetailItem label="关联重开单" value={successors.length ? successors.map((item) => <Link key={item.id} className={`${linkClass} mr-space-2`} to={`/weighing/tickets/${encodeURIComponent(item.id)}`}>{item.ticketNo ?? item.id}</Link>) : "无"} /></div></ContentCard>
      <ContentCard title="原始称重记录与替代关系"><WeighRecords records={ticket.records} entryType={ticket.entryType} /></ContentCard>
      <ContentCard title="厂区与同步"><div className="grid gap-space-4 sm:grid-cols-4">
        <DetailItem label="出单厂区" value={[ticket.siteName, ticket.siteCode].filter(Boolean).join(' / ')} />
        <DetailItem label="同步状态" value={<StatusTag tone={sync.tone}>{sync.label}</StatusTag>} />
        <DetailItem label="最近对账" value={reconcile ? <StatusTag tone={reconcile.tone}>{reconcile.label}</StatusTag> : '未对账'} />
        {ticket.reconcileAt && <DetailItem label="对账时间" value={formatDateTime(ticket.reconcileAt)} />}
      </div></ContentCard>
      <ContentCard title="操作历史"><HistoryList entries={history} /></ContentCard>
      </div></details>
    </div>
    <FormDialog open={!!operation} title="作废单据" onCancel={() => setOperation(null)} onSubmit={submitOperation} submitText="确认作废" submitDisabled={!reason.trim()} loading={busy}>
      <div className="space-y-space-4">
        <div className="grid gap-space-3 rounded-control border border-border bg-neutral-background p-space-3 sm:grid-cols-3">
          <DetailItem label="单号" value={ticket.ticketNo ?? ticket.id} />
          <DetailItem label="车牌" value={ticket.plate} />
          <DetailItem label="最终净重" value={formatWeight(ticket.net)} />
        </div>
        <p className="text-error">本单将失效，之后无法发起打印。原任务保持已完成。</p>
        <Field label="作废原因" htmlFor="ticket-operation-reason" required><Input id="ticket-operation-reason" required value={reason} onChange={(event) => setReason(event.target.value)} /></Field>
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
    <PageHeader title="单据打印预览" actions={<>
      <HelpHint label="纸张规格">空白五联针打纸，241 × 279 mm 三等分，单节 241 × 93 mm（含孔边）。</HelpHint>
      <Link className={linkClass} to={`/weighing/tickets/${id}`}>返回单据详情</Link>
    </>} />
    {error && <p role="alert" className="text-error">{error}</p>}
    {ticket && <TicketPrintActions ticketId={ticket.id} />}
    <div className="mt-space-4">
      {ticket?.printSheet ? <TicketSheet sheet={ticket.printSheet} /> : !error && <p role="status">正在加载当版单据快照…</p>}
    </div>
  </>;
}
