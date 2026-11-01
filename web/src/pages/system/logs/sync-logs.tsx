import { RefreshCw, RotateCcw, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { syncApi, type SyncLog } from "@/api/sync";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { DataTableCard } from "@/components/common/data-table-card";
import { DataTable } from "@/components/common/data-table";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";
import { categoryLabels } from "@/pages/weighing/sync/labels";

const typeLabels: Record<string, string> = {
  "master-manual": "资料手动同步",
  "master-automatic": "资料自动同步",
  issue: "出单上传",
  void: "作废上传",
  reopen: "重开上传",
  reconcile: "单据对账",
};
const kindOf = (log: SyncLog) => log.syncType === "master" ? `master-${log.triggerType}` : log.syncType;

export function SystemSyncLogsPage() {
  const [logs, setLogs] = useState<SyncLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [date, setDate] = useState("");
  const [kind, setKind] = useState("");
  const [category, setCategory] = useState("");
  const [result, setResult] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [clearOpen, setClearOpen] = useState(false);
  const [clearing, setClearing] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try { setLogs(await syncApi.logs()); }
    catch (err) { setError(getErrorMessage(err, "无法读取同步日志")); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);
  const filtered = useMemo(() => logs.filter(log => {
    const time = new Date(log.attemptedAt);
    const localDate = `${time.getFullYear()}-${String(time.getMonth() + 1).padStart(2, "0")}-${String(time.getDate()).padStart(2, "0")}`;
    return (!date || localDate === date) && (!kind || kindOf(log) === kind) && (!category || log.category === category) && (!result || String(log.success) === result);
  }), [logs, date, kind, category, result]);
  const currentPage = Math.min(page, Math.max(1, Math.ceil(filtered.length / pageSize)));
  const reset = () => { setDate(""); setKind(""); setCategory(""); setResult(""); setPage(1); };
  const clear = async () => {
    setClearing(true);
    try { await syncApi.clearLogs(); setClearOpen(false); setPage(1); toast.success("普通同步日志已清空"); await load(); }
    catch (err) { toast.error({ title: "清空失败", description: getErrorMessage(err, "请稍后重试") }); }
    finally { setClearing(false); }
  };
  const columns: DataTableColumn<SyncLog>[] = [
    { title: "同步时间", dataIndex: "attemptedAt", width: 180, render: (_, row) => formatDateTime(row.attemptedAt) },
    { title: "同步类型", dataIndex: "syncType", width: 130, render: (_, row) => typeLabels[kindOf(row)] ?? row.syncType },
    { title: "范围 / 单号", dataIndex: "ticketNo", width: 190, render: (_, row) => row.ticketNo || (row.syncType === "reconcile" ? "对账批次" : "全部资料") },
    { title: "类别", dataIndex: "category", width: 130, render: (_, row) => row.category ? (categoryLabels[row.category] ?? row.category) : "—" },
    { title: "摘要", dataIndex: "digest", width: 150, render: (_, row) => row.digest ? <span className="font-mono text-xs" title={row.digest}>{row.digest.slice(0, 10)}…</span> : "—" },
    { title: "结果", dataIndex: "success", width: 90, render: (_, row) => <StatusTag tone={row.success ? "success" : "error"}>{row.success ? "成功" : "失败"}</StatusTag> },
    { title: "失败原因", dataIndex: "reason", render: (_, row) => row.reason || "—" },
  ];
  return <>
    <PageHeader title="同步日志" description="记录总部资料同步、单据上传尝试和对账结果；冲突、核查结论及处理记录单独保留。" actions={<Button variant="secondary" onClick={() => void load()} disabled={loading}><RefreshCw className="h-4 w-4" aria-hidden />刷新</Button>} />
    <SearchFilterBar actions={<Button variant="secondary" onClick={reset}><RotateCcw className="h-4 w-4" aria-hidden />重置</Button>}>
      <Input type="date" aria-label="同步日期" value={date} onChange={event => { setDate(event.target.value); setPage(1); }} />
      <Select aria-label="同步类型" value={kind} onChange={event => { setKind(event.target.value); setPage(1); }}>
        <option value="">全部类型</option>
        {Object.entries(typeLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
      </Select>
      <Select aria-label="错误类别" value={category} onChange={event => { setCategory(event.target.value); setPage(1); }}>
        <option value="">全部类别</option>
        {Object.entries(categoryLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
      </Select>
      <Select aria-label="同步结果" value={result} onChange={event => { setResult(event.target.value); setPage(1); }}><option value="">全部结果</option><option value="true">成功</option><option value="false">失败</option></Select>
    </SearchFilterBar>
    <DataTableCard>
      <TableToolbar title="同步日志列表" description={`共 ${filtered.length} 条普通记录；日志可手动清空，但不删除变化、对账结果或核查记录。`} actions={<>
        <Button variant="secondary" size="sm" disabled={loading} onClick={() => void load()}><RefreshCw className="h-4 w-4" aria-hidden />刷新</Button>
        <Button variant="secondary" size="sm" className="text-error hover:text-error" disabled={loading || !!error || logs.length === 0} onClick={() => setClearOpen(true)}><Trash2 className="h-4 w-4" aria-hidden />清空日志</Button>
      </>} />
      <DataTable columns={columns} dataSource={filtered.slice((currentPage - 1) * pageSize, currentPage * pageSize)} rowKey="id" loading={loading} error={error} minWidth={1020} empty={<EmptyState title="暂无同步日志" description="当前筛选条件下没有同步记录。" />} />
      <Pagination page={currentPage} pageSize={pageSize} total={filtered.length} disabled={loading} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
    <ConfirmDialog open={clearOpen} title="清空普通同步日志" description="确认清空全部普通同步日志吗？此操作不可恢复，不影响待上传变化、对账结果、冲突记录或核查结论。" confirmText="清空" danger loading={clearing} onConfirm={() => void clear()} onCancel={() => setClearOpen(false)} />
  </>;
}
