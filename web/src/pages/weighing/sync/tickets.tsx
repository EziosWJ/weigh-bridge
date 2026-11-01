import { RefreshCw, RotateCcw, ScanSearch } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { syncApi, type SyncOverview, type SyncTicket, type SyncTicketQuery } from "@/api/sync";
import { weighingApi } from "@/api/weighing";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableActions } from "@/components/common/table-actions";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Field } from "@/components/common/field";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";
import { protocolLabel, reconcileLabels, syncLabels } from "./labels";

export function SyncTicketsPage() {
  const navigate = useNavigate();
  const [overview, setOverview] = useState<SyncOverview | null>(null);
  const [data, setData] = useState<{ records: SyncTicket[]; total: number }>({ records: [], total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [canManage, setCanManage] = useState(false);
  const [filter, setFilter] = useState<SyncTicketQuery>({});
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [batchOpen, setBatchOpen] = useState(false);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [summary, pageData] = await Promise.all([
        syncApi.overview(),
        syncApi.tickets({ ...filter, page, pageSize }),
      ]);
      setOverview(summary);
      setData({ records: pageData.records, total: pageData.total });
    } catch (failure) {
      setError(getErrorMessage(failure, "无法读取同步状态"));
    } finally {
      setLoading(false);
    }
  }, [filter, page, pageSize]);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    void weighingApi.capabilities().then(value => setCanManage(value.permissionCodes.includes("weighing:sync:manage"))).catch(() => setCanManage(false));
  }, []);

  const reconcileOne = async (row: SyncTicket) => {
    setBusy(true);
    try {
      const result = await syncApi.reconcileTicket(row.id);
      toast.success({ title: "核对完成", description: result.message });
      await load();
    } catch (failure) {
      toast.error({ title: "核对失败", description: getErrorMessage(failure, "请稍后重试") });
    } finally {
      setBusy(false);
    }
  };
  const reconcileRange = async () => {
    setBusy(true);
    try {
      const result = await syncApi.reconcileRange(from, to);
      toast.success({ title: "批量对账完成", description: `检查 ${result.checked} 张：一致 ${result.consistent}，待同步 ${result.pending}，待核查 ${result.needsReview}，冲突 ${result.conflict}，未知 ${result.unknown}` });
      setBatchOpen(false);
      await load();
    } catch (failure) {
      toast.error({ title: "批量对账失败", description: getErrorMessage(failure, "请稍后重试") });
    } finally {
      setBusy(false);
    }
  };

  const columns: DataTableColumn<SyncTicket>[] = [
    { title: "单号 / 车牌", key: "ticket", width: 220, render: (_, row) => <div><Link className="text-primary hover:underline" to={`/weighing/sync/tickets/${row.id}`}>{row.ticketNo}</Link><div className="mt-1 font-medium">{row.business?.plate ?? "—"}</div></div> },
    { title: "方向 / 物料", key: "business", width: 170, render: (_, row) => <div>{row.business?.materialName ?? "—"}{row.business?.materialSpec ? <div className="text-text-tertiary">{row.business.materialSpec}</div> : null}</div> },
    { title: "单据状态", key: "status", width: 90, render: (_, row) => <StatusTag tone={row.status === "valid" ? "success" : "neutral"}>{row.status === "valid" ? "有效" : "作废"}</StatusTag> },
    { title: "同步状态", key: "sync", width: 160, render: (_, row) => { const item = syncLabels[row.syncStatus ?? "untracked"] ?? syncLabels.untracked; return <div><StatusTag tone={item.tone}>{item.label}</StatusTag><div className="mt-1 text-text-tertiary">{protocolLabel(row.syncProtocol)}</div></div>; } },
    { title: "同步说明", key: "syncReason", width: 220, render: (_, row) => <span className="text-text-secondary">{row.syncReason || "—"}</span> },
    { title: "最近对账", key: "reconcile", width: 170, render: (_, row) => { const item = row.reconcileStatus ? reconcileLabels[row.reconcileStatus] : null; return item ? <div><StatusTag tone={item.tone}>{item.label}</StatusTag><div className="mt-1 text-text-tertiary">{formatDateTime(row.reconcileAt)}</div></div> : <span className="text-text-tertiary">未对账</span>; } },
    { title: "出单时间", key: "issuedAt", width: 165, render: (_, row) => formatDateTime(row.issuedAt) },
    { title: "操作", key: "actions", width: 160, nowrap: true, render: (_, row) => <TableActions>
      <button type="button" className="text-primary hover:underline" onClick={() => navigate(`/weighing/sync/tickets/${row.id}`)}>详情</button>
      {canManage && row.syncProtocol === 4 && <button type="button" className="text-primary hover:underline" disabled={busy} onClick={() => void reconcileOne(row)}>核对</button>}
    </TableActions> },
  ];

  return <div className="weighing-list-page">
    <PageHeader title="单据同步" description="按摘要核对云端已接收变化；同步确认状态与对账结果分开显示。" actions={<Button variant="secondary" onClick={() => void load()} disabled={loading}><RefreshCw className="h-4 w-4" aria-hidden />刷新</Button>} />
    {overview && !overview.enabled && <p role="alert" className="mb-space-4 rounded-control border border-warning-border bg-warning-background px-space-3 py-space-2 text-sm text-warning">同步开关未开启。待上传变化和原摘要保留，当前停止上传和自动对账；重新启用并完成资料同步后继续处理。</p>}
    {overview && overview.enabled && !overview.bound && <p role="alert" className="mb-space-4 rounded-control border border-warning-border bg-warning-background px-space-3 py-space-2 text-sm text-warning">尚未完成首次总部资料同步，正式称重与单据上传不会开始。</p>}
    <div className="mb-space-4 grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">
      <SummaryCard label="待同步单据" value={overview?.pendingTickets ?? "—"} detail="最新变化尚未获云端确认" />
      <SummaryCard label="需处理" value={overview?.needsActionTickets ?? "—"} detail="需管理员处理或存在未解除冲突" />
      <SummaryCard label="最近成功" value={overview?.lastSuccess ? formatDateTime(overview.lastSuccess) : "—"} detail="最近一次变化获云端确认" />
      <SummaryCard label="最近对账" value={overview?.lastReconcile ? formatDateTime(overview.lastReconcile.checkedAt) : "—"} detail={overview?.lastReconcile ? `${reconcileLabels[overview.lastReconcile.status]?.label ?? overview.lastReconcile.status}：一致 ${overview.lastReconcile.consistent}，待同步 ${overview.lastReconcile.pending}，待核查 ${overview.lastReconcile.needsReview}，冲突 ${overview.lastReconcile.conflict}，未知 ${overview.lastReconcile.unknown}` : "尚未执行自动或手动对账"} />
    </div>
    <SearchFilterBar actions={<>
      {canManage && <Button variant="secondary" onClick={() => setBatchOpen(true)}><ScanSearch className="h-4 w-4" aria-hidden />按日期批量对账</Button>}
      <Button variant="secondary" onClick={() => { setFilter({}); setPage(1); }}><RotateCcw className="h-4 w-4" aria-hidden />重置</Button>
    </>}>
      <Input aria-label="单号或车牌" placeholder="单号或车牌" value={filter.keyword ?? ""} onChange={event => { setFilter({ ...filter, keyword: event.target.value }); setPage(1); }} />
      <Select aria-label="同步状态" value={filter.syncStatus ?? ""} onChange={event => { setFilter({ ...filter, syncStatus: event.target.value }); setPage(1); }}>
        <option value="">全部同步状态</option>
        {Object.entries(syncLabels).map(([value, item]) => <option key={value} value={value}>{item.label}</option>)}
      </Select>
      <Select aria-label="对账状态" value={filter.reconcileStatus ?? ""} onChange={event => { setFilter({ ...filter, reconcileStatus: event.target.value }); setPage(1); }}>
        <option value="">全部对账状态</option>
        {Object.entries(reconcileLabels).map(([value, item]) => <option key={value} value={value}>{item.label}</option>)}
      </Select>
      <Input type="date" aria-label="出单开始日期" value={filter.from ?? ""} onChange={event => { setFilter({ ...filter, from: event.target.value }); setPage(1); }} />
      <Input type="date" aria-label="出单结束日期" value={filter.to ?? ""} onChange={event => { setFilter({ ...filter, to: event.target.value }); setPage(1); }} />
    </SearchFilterBar>
    <DataTableCard>
      <TableToolbar title="单据同步列表" description={`共 ${data.total} 张；同步确认状态与最近对账结果分别显示。`} />
      <DataTable columns={columns} dataSource={data.records} rowKey="id" loading={loading} error={error} minWidth={1320} empty={<EmptyState title="暂无同步单据" description="当前筛选条件下没有单据。" />} />
      <Pagination page={page} pageSize={pageSize} total={data.total} disabled={loading} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
    <FormDialog open={batchOpen} title="按日期批量对账" description="检查所选出单日期范围内由摘要协议管理的单据。云端查询失败不会显示为一致。" submitText="开始对账" loadingText="对账中…" loading={busy} onCancel={() => setBatchOpen(false)} onSubmit={() => void reconcileRange()}>
      <div className="grid gap-space-3 sm:grid-cols-2">
        <Field label="开始日期" htmlFor="sync-from"><Input id="sync-from" type="date" value={from} disabled={busy} onChange={event => setFrom(event.target.value)} /></Field>
        <Field label="结束日期" htmlFor="sync-to"><Input id="sync-to" type="date" value={to} disabled={busy} onChange={event => setTo(event.target.value)} /></Field>
      </div>
      <p className="mt-space-3 text-sm text-text-secondary">留空时检查最近 7 天发生过变化及更早未确认的单据；单批最多 200 条，系统自动分批。</p>
    </FormDialog>
  </div>;
}

function SummaryCard({ label, value, detail }: { label: string; value: string | number; detail: string }) {
  return <div className="rounded-admin border border-border bg-surface p-card">
    <div className="text-xs text-text-tertiary">{label}</div>
    <div className="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
    <div className="mt-1 text-xs text-text-tertiary">{detail}</div>
  </div>;
}
