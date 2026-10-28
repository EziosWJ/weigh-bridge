import { RefreshCw, RotateCcw, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { weighingApi } from "@/api/weighing";
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

type SyncLog = Awaited<ReturnType<typeof weighingApi.masterSyncLogs>>[number];

export function SystemSyncLogsPage() {
  const [logs, setLogs] = useState<SyncLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [date, setDate] = useState("");
  const [kind, setKind] = useState("");
  const [result, setResult] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [clearOpen, setClearOpen] = useState(false);
  const [clearing, setClearing] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try { setLogs(await weighingApi.masterSyncLogs()); }
    catch (err) { setError(getErrorMessage(err, "无法读取同步日志")); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);
  const filtered = useMemo(() => logs.filter(log => {
    const time = new Date(log.attemptedAt);
    const localDate = `${time.getFullYear()}-${String(time.getMonth() + 1).padStart(2, "0")}-${String(time.getDate()).padStart(2, "0")}`;
    return (!date || localDate === date) && (!kind || log.triggerType === kind) && (!result || String(log.success) === result);
  }), [logs, date, kind, result]);
  const currentPage = Math.min(page, Math.max(1, Math.ceil(filtered.length / pageSize)));
  const reset = () => { setDate(""); setKind(""); setResult(""); setPage(1); };
  const clear = async () => {
    setClearing(true);
    try { await weighingApi.clearMasterSyncLogs(); setClearOpen(false); setPage(1); toast.success("同步日志已清空"); await load(); }
    catch (err) { toast.error({ title: "清空失败", description: getErrorMessage(err, "请稍后重试") }); }
    finally { setClearing(false); }
  };
  const columns: DataTableColumn<SyncLog>[] = [
    { title: "同步时间", dataIndex: "attemptedAt", width: 200, render: (_, row) => formatDateTime(row.attemptedAt) },
    { title: "同步类型", dataIndex: "triggerType", width: 150, render: (_, row) => row.triggerType === "manual" ? "手动同步" : "自动同步" },
    { title: "结果", dataIndex: "success", width: 120, render: (_, row) => <StatusTag tone={row.success ? "success" : "error"}>{row.success ? "成功" : "失败"}</StatusTag> },
    { title: "失败原因", dataIndex: "reason", render: (_, row) => row.reason || "—" },
  ];
  return <>
    <PageHeader title="同步日志" description="查看总部基础资料自动和手动同步的记录。" />
    <SearchFilterBar actions={<Button variant="secondary" onClick={reset}><RotateCcw className="h-4 w-4" aria-hidden />重置</Button>}>
      <Input type="date" aria-label="同步日期" value={date} onChange={event => { setDate(event.target.value); setPage(1); }} />
      <Select aria-label="同步类型" value={kind} onChange={event => { setKind(event.target.value); setPage(1); }}><option value="">全部类型</option><option value="manual">手动同步</option><option value="automatic">自动同步</option></Select>
      <Select aria-label="同步结果" value={result} onChange={event => { setResult(event.target.value); setPage(1); }}><option value="">全部结果</option><option value="true">成功</option><option value="false">失败</option></Select>
    </SearchFilterBar>
    <DataTableCard>
      <TableToolbar title="同步日志列表" description={`共 ${filtered.length} 条记录。`} actions={<>
        <Button variant="secondary" size="sm" disabled={loading} onClick={() => void load()}><RefreshCw className="h-4 w-4" aria-hidden />刷新</Button>
        <Button variant="secondary" size="sm" className="text-error hover:text-error" disabled={loading || !!error || logs.length === 0} onClick={() => setClearOpen(true)}><Trash2 className="h-4 w-4" aria-hidden />清空日志</Button>
      </>} />
      <DataTable columns={columns} dataSource={filtered.slice((currentPage - 1) * pageSize, currentPage * pageSize)} rowKey="id" loading={loading} error={error} minWidth={760} empty={<EmptyState title="暂无同步日志" description="当前筛选条件下没有同步记录。" />} />
      <Pagination page={currentPage} pageSize={pageSize} total={filtered.length} disabled={loading} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
    <ConfirmDialog open={clearOpen} title="清空同步日志" description="确认清空全部同步日志吗？此操作不可恢复，不影响同步资料。" confirmText="清空" danger loading={clearing} onConfirm={() => void clear()} onCancel={() => setClearOpen(false)} />
  </>;
}
