import { RefreshCw, RotateCcw, ScanSearch } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { syncApi, type SyncOverview, type SyncTicket, type SyncTicketQuery } from "@/api/sync";
import { weighingApi } from "@/api/weighing";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { FormDialog } from "@/components/common/form-dialog";
import { HelpHint } from "@/components/common/help-hint";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableActions } from "@/components/common/table-actions";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { buttonStyles } from "@/components/ui/button-styles";
import { Field } from "@/components/common/field";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import { useTransportDirections } from "@/hooks/use-transport-directions";
import type { DataTableColumn } from "@/types";
import { reconcileLabels, syncLabels, ticketSyncLabel } from "./labels";

export function SyncTicketsPage() {
  const directionLabels = useTransportDirections();
  const loadVersion = useRef(0);
  const [overview, setOverview] = useState<SyncOverview | null>(null);
  const [data, setData] = useState<{ records: SyncTicket[]; total: number }>({ records: [], total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [canManage, setCanManage] = useState(false);
  const [filter, setFilter] = useState<SyncTicketQuery>({});
  const [revision, setRevision] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [batchOpen, setBatchOpen] = useState(false);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState("");
  const [batchError, setBatchError] = useState("");

  const load = useCallback(async () => {
    const version = ++loadVersion.current;
    setLoading(true);
    setError("");
    try {
      const [summary, pageData] = await Promise.all([
        syncApi.overview(),
        syncApi.tickets({ ...filter, page, pageSize }),
      ]);
      if (version === loadVersion.current) {
        setOverview(summary);
        setData({ records: pageData.records, total: pageData.total });
      }
    } catch (failure) {
      if (version === loadVersion.current) setError(getErrorMessage(failure, "无法读取同步状态"));
    } finally {
      if (version === loadVersion.current) setLoading(false);
    }
  }, [filter, page, pageSize]);
  useEffect(() => { void load(); }, [load, revision]);
  useEffect(() => {
    void weighingApi.capabilities().then(value => setCanManage(value.permissionCodes.includes("weighing:sync:manage"))).catch(() => setCanManage(false));
  }, []);

  const reconcileOne = async (row: SyncTicket) => {
    setBusy(true);
    setActionError("");
    try {
      const result = await syncApi.reconcileTicket(row.id);
      toast.success({ title: "核对完成", description: result.message });
      setRevision(value => value + 1);
    } catch (failure) {
      setActionError(getErrorMessage(failure, "核对失败"));
    } finally {
      setBusy(false);
    }
  };
  const reconcileRange = async () => {
    setBusy(true);
    setBatchError("");
    try {
      const result = await syncApi.reconcileRange(from, to);
      toast.success({ title: "批量对账完成", description: `检查 ${result.checked} 张：一致 ${result.consistent}，待同步 ${result.pending}，待核查 ${result.needsReview}，冲突 ${result.conflict}，未知 ${result.unknown}` });
      setBatchOpen(false);
      setRevision(value => value + 1);
    } catch (failure) {
      setBatchError(getErrorMessage(failure, "批量对账失败"));
    } finally {
      setBusy(false);
    }
  };

  const syncActive = overview?.enabled && overview.bound;

  const columns: DataTableColumn<SyncTicket>[] = [
    { title: "单号 / 车牌", key: "ticket", width: 220, render: (_, row) => <div><Link className="text-primary hover:underline" to={`/weighing/sync/tickets/${row.id}`}>{row.ticketNo}</Link><div className="mt-1 font-medium">{row.business?.plate ?? "—"}</div></div> },
    { title: "方向 / 物料", key: "business", width: 170, render: (_, row) => <div>
      <div>{row.business?.direction ? directionLabels[row.business.direction] : "—"}</div>
      <div className="text-text-secondary">{[row.business?.materialName, row.business?.materialSpec].filter(Boolean).join(" / ") || "—"}</div>
    </div> },
    { title: "单据状态", key: "status", width: 90, render: (_, row) => <StatusTag tone={row.status === "valid" ? "success" : "neutral"}>{row.status === "valid" ? "有效" : "作废"}</StatusTag> },
    { title: "同步状态", key: "sync", width: 142, render: (_, row) => {
      const item = ticketSyncLabel(row.syncStatus, overview?.enabled, overview?.bound, row.syncReason);
      return <StatusTag tone={item.tone}>{item.label}</StatusTag>;
    } },
    { title: "最近异常", key: "syncReason", render: (_, row) => <span className="block truncate text-text-secondary" title={row.syncReason || undefined}>{row.syncReason || "—"}</span> },
    { title: "最近对账", key: "reconcile", width: 176, render: (_, row) => { const item = row.reconcileStatus ? reconcileLabels[row.reconcileStatus] : null; return item ? <div><StatusTag tone={item.tone}>{item.label}</StatusTag><div className="mt-1 whitespace-nowrap tabular-nums text-text-tertiary">{formatDateTime(row.reconcileAt)}</div></div> : <span className="text-text-tertiary">未对账</span>; } },
    { title: "出单时间", key: "issuedAt", width: 176, nowrap: true, render: (_, row) => <span className="tabular-nums">{formatDateTime(row.issuedAt)}</span> },
  ];
  if (canManage) columns.push({ title: "操作", key: "actions", width: 92, nowrap: true, render: (_, row) => {
    if (row.syncStatus === "needs_admin" || row.syncStatus === "conflict" || row.reconcileStatus === "needs_review" || row.reconcileStatus === "conflict") {
      return <TableActions><Link className={buttonStyles({ size: "sm" })} to={`/weighing/sync/tickets/${row.id}`}>处理</Link></TableActions>;
    }
    if (row.syncProtocol !== 4 && row.syncProtocol !== 5 && row.syncProtocol !== 6) return <span className="text-text-tertiary">—</span>;
    return <TableActions><Button size="sm" disabled={busy || !syncActive} onClick={() => void reconcileOne(row)}>核对</Button></TableActions>;
  } });

  return <div className="weighing-list-page">
    <PageHeader title="单据同步" actions={<>
      <HelpHint label="单据同步规则" align="right">关闭同步时，待上传变化保留。对账结果根据云端单据和已确认的变化判断。</HelpHint>
      <Button variant="secondary" onClick={() => void load()} disabled={loading}><RefreshCw className="h-4 w-4" aria-hidden />刷新</Button>
    </>} />
    {overview && !overview.enabled && <p role="status" className="mb-space-4 text-sm text-text-secondary">同步已关闭</p>}
    {overview?.enabled && !overview.bound && <p role="status" className="mb-space-4 text-sm text-warning">待资料同步</p>}
    <div className="mb-space-4 grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">
      <SummaryCard label="待同步单据" value={overview?.pendingTickets ?? "—"} />
      <SummaryCard label="需处理" value={overview?.needsActionTickets ?? "—"} />
      <SummaryCard label="最近同步成功" value={formatDateTime(overview?.lastSuccess)} />
      <SummaryCard label="最近对账" value={formatDateTime(overview?.lastReconcile?.checkedAt)} detail={overview?.lastReconcile && reconcileLabels[overview.lastReconcile.status]?.label} />
    </div>
    <SearchFilterBar compact actions={<>
      <Button variant="secondary" onClick={() => { setFilter({}); setPage(1); }}><RotateCcw className="h-4 w-4" aria-hidden />重置</Button>
    </>}>
      <Input aria-label="单号或车牌" placeholder="单号或车牌" value={filter.keyword ?? ""} onChange={event => { setFilter({ ...filter, keyword: event.target.value }); setPage(1); }} />
      <Select aria-label="同步状态" value={filter.syncStatus ?? ""} disabled={overview != null && !syncActive} onChange={event => { setFilter({ ...filter, syncStatus: event.target.value }); setPage(1); }}>
        <option value="">全部同步状态</option>
        {Object.entries(syncLabels).filter(([value]) => value !== "failed").map(([value, item]) => <option key={value} value={value === "temporary_failure" ? "retry" : value}>{item.label}</option>)}
        <option value="old_site_code">旧编码暂停</option>
      </Select>
      <Select aria-label="对账状态" value={filter.reconcileStatus ?? ""} onChange={event => { setFilter({ ...filter, reconcileStatus: event.target.value }); setPage(1); }}>
        <option value="">全部对账状态</option>
        {Object.entries(reconcileLabels).map(([value, item]) => <option key={value} value={value}>{item.label}</option>)}
      </Select>
      <label className="flex items-center gap-2 text-sm text-text-secondary"><span className="shrink-0">从</span><Input type="date" aria-label="出单开始日期" className="min-w-0" value={filter.from ?? ""} onChange={event => { setFilter({ ...filter, from: event.target.value }); setPage(1); }} /></label>
      <label className="flex items-center gap-2 text-sm text-text-secondary"><span className="shrink-0">至</span><Input type="date" aria-label="出单结束日期" className="min-w-0" value={filter.to ?? ""} onChange={event => { setFilter({ ...filter, to: event.target.value }); setPage(1); }} /></label>
    </SearchFilterBar>
    <DataTableCard>
      <TableToolbar title="单据同步列表" description={`共 ${data.total} 张`} actions={canManage && <Button disabled={busy || !syncActive} onClick={() => { setBatchError(""); setBatchOpen(true); }}><ScanSearch className="h-4 w-4" aria-hidden />按日期批量对账</Button>} />
      {actionError && <p role="alert" className="px-card py-space-2 text-sm text-error">{actionError}</p>}
      <DataTable columns={columns} dataSource={data.records} rowKey="id" loading={loading} error={error} minWidth={canManage ? 1190 : 1100} compact stickyHeader fixedEdges={canManage} ariaLabel="单据同步列表" empty={<EmptyState title="暂无同步单据" description="当前筛选条件下没有单据。" />} />
      <Pagination page={page} pageSize={pageSize} total={data.total} disabled={loading} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
    <FormDialog open={batchOpen} title="按日期批量对账" submitText="开始对账" loadingText="对账中…" loading={busy} onCancel={() => setBatchOpen(false)} onSubmit={() => void reconcileRange()}>
      <div className="grid gap-space-3 sm:grid-cols-2">
        <Field label="开始日期" htmlFor="sync-from"><Input id="sync-from" type="date" value={from} disabled={busy} onChange={event => setFrom(event.target.value)} /></Field>
        <Field label="结束日期" htmlFor="sync-to"><Input id="sync-to" type="date" value={to} disabled={busy} onChange={event => setTo(event.target.value)} /></Field>
      </div>
      {batchError && <p role="alert" className="mb-space-2 text-sm text-error">{batchError}</p>}
      <HelpHint label="批量对账范围">日期留空时，检查最近 7 天发生过变化及更早未确认的单据。系统按批次执行。</HelpHint>
    </FormDialog>
  </div>;
}

function SummaryCard({ label, value, detail }: { label: string; value: string | number; detail?: string | null }) {
  return <div className="rounded-admin border border-border bg-surface p-card">
    <div className="text-xs text-text-tertiary">{label}</div>
    <div className="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
    {detail && <div className="mt-1 text-xs text-text-tertiary">{detail}</div>}
  </div>;
}
