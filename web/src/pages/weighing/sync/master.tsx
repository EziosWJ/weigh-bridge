import { RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { weighingApi } from "@/api/weighing";
import { ContentCard } from "@/components/common/content-card";
import { DetailItem } from "@/components/common/detail-item";
import { HelpHint } from "@/components/common/help-hint";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import { masterDataLabel } from "./labels";

type MasterStatus = Awaited<ReturnType<typeof weighingApi.masterSyncStatus>>;

export function SyncMasterPage() {
  const [status, setStatus] = useState<MasterStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [syncError, setSyncError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setStatus(await weighingApi.masterSyncStatus());
    } catch (failure) {
      setStatus(null);
      setError(getErrorMessage(failure, "无法读取资料同步状态"));
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => { void load(); }, [load]);

  const sync = async () => {
    setBusy(true);
    setSyncError("");
    try {
      setStatus(await weighingApi.runMasterSync());
      toast.success("总部资料已生效");
    } catch (failure) {
      setSyncError(getErrorMessage(failure, "总部资料同步失败"));
      await load();
    } finally {
      setBusy(false);
    }
  };

  const mode = status && masterDataLabel(status);
  const attempt = status?.lastAttempt;
  return <>
    <PageHeader title="基础资料同步" actions={<Button variant="secondary" onClick={() => { setSyncError(""); void load(); }} disabled={loading || busy}><RefreshCw className="h-4 w-4" aria-hidden />刷新</Button>} />
    {error && <p role="alert" className="mb-space-4 text-sm text-error">{error}</p>}
    <ContentCard title="当前资料" extra={<HelpHint label="资料同步规则" align="right">同步范围包括物料分类、物料、往来主体及联系人。完整校验后一次生效，失败时保留上次成功资料。启用同步后，由管理员完成首次同步。</HelpHint>}>
      <div className="grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">
        <DetailItem label="资料状态" value={mode ? <StatusTag tone={mode.tone}>{mode.label}</StatusTag> : loading ? "读取中…" : "—"} />
        <DetailItem label="最近成功同步" value={formatDateTime(status?.lastSuccess)} />
        <DetailItem label="最近同步结果" value={attempt ? <StatusTag tone={attempt.success ? "success" : "error"}>{attempt.success ? "成功" : "失败"}</StatusTag> : "—"} />
        <DetailItem label="最近同步时间" value={formatDateTime(attempt?.attemptedAt)} />
      </div>
      <div className="mt-space-4 flex flex-wrap gap-space-2">
        <Button variant="primary" disabled={loading || busy || !status?.enabled || (!status.bound && !status.isAdmin)} onClick={() => void sync()}>{busy ? "正在同步…" : "立即同步全部资料"}</Button>
        <Link className="inline-flex h-control items-center text-sm text-primary hover:underline" to="/weighing/materials">物料资料</Link>
        <Link className="inline-flex h-control items-center text-sm text-primary hover:underline" to="/weighing/sync/logs">同步日志</Link>
      </div>
      {syncError ? <p role="alert" className="mt-space-3 text-sm text-error">{syncError}</p> : attempt && !attempt.success && attempt.reason && <p className="mt-space-3 text-sm text-error">{attempt.reason}</p>}
    </ContentCard>
  </>;
}
