import { RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { weighingApi } from "@/api/weighing";
import { ContentCard } from "@/components/common/content-card";
import { DetailItem } from "@/components/common/detail-item";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";

type MasterStatus = Awaited<ReturnType<typeof weighingApi.masterSyncStatus>>;

export function SyncMasterPage() {
  const [status, setStatus] = useState<MasterStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setStatus(await weighingApi.masterSyncStatus());
    } catch (failure) {
      setError(getErrorMessage(failure, "无法读取资料同步状态"));
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => { void load(); }, [load]);

  const sync = async () => {
    setBusy(true);
    try {
      const next = await weighingApi.runMasterSync();
      setStatus(next);
      toast.success({ title: "总部资料同步成功", description: "资料已整体生效；本次同步不触发单据上传。" });
    } catch (failure) {
      toast.error({ title: "总部资料同步失败", description: getErrorMessage(failure, "请稍后重试") });
    } finally {
      setBusy(false);
    }
  };

  return <>
    <PageHeader title="基础资料同步" description="物料分类、物料、往来主体及联系人由总部统一维护；完整下载并校验成功后才整体生效。" actions={<Button variant="secondary" onClick={() => void load()} disabled={loading}><RefreshCw className="h-4 w-4" aria-hidden />刷新</Button>} />
    {error && <p role="alert" className="mb-space-4 text-sm text-error">{error}</p>}
    <ContentCard title="资料同步状态" extra={status ? <StatusTag tone={!status.enabled ? "neutral" : status.bound ? "success" : "warning"}>{!status.enabled ? "同步未开启" : status.bound ? "云端资料模式" : "待首次同步"}</StatusTag> : null}>
      <div className="grid gap-space-4 sm:grid-cols-2 xl:grid-cols-3">
        <DetailItem label="同步开关" value={status ? (status.enabled ? "开启" : "关闭") : "—"} />
        <DetailItem label="资料模式" value={status ? (status.bound ? "已应用总部资料" : "尚未完成首次同步") : "—"} />
        <DetailItem label="最近成功同步" value={formatDateTime(status?.lastSuccess)} />
      </div>
      <p className="mt-space-4 text-sm text-text-secondary">首次成功前不办理正式称重；失败保留上一版资料，可手动重试。成功后可继续使用上次同步资料，互联网中断不影响本地业务。</p>
      {status && !status.enabled && <p role="alert" className="mt-space-3 rounded-control border border-warning-border bg-warning-background px-space-3 py-space-2 text-sm text-warning">当前为单机模式。待上传单据变化保留；重新开启并重启后，由管理员完成手动资料同步再继续。</p>}
      <div className="mt-space-4 flex flex-wrap gap-space-2">
        <Button variant="primary" disabled={busy || !status?.enabled} onClick={() => void sync()}>{busy ? "正在同步…" : "立即同步全部资料"}</Button>
        <Link className="inline-flex h-control items-center text-sm text-primary hover:underline" to="/weighing/materials">前往物料资料</Link>
        <Link className="inline-flex h-control items-center text-sm text-primary hover:underline" to="/weighing/sync/logs">查看同步日志</Link>
      </div>
    </ContentCard>
    <ContentCard title="同步范围与失败处理" className="mt-space-6">
      <ul className="list-disc space-y-2 pl-5 text-sm text-text-secondary">
        <li>自动同步按配置间隔执行；本页与资料页的主动同步入口执行同一完整资料同步。</li>
        <li>一次同步物料分类、物料、往来主体及联系人；全部校验成功后在单个事务中生效，不产生部分更新。</li>
        <li>主动同步资料不触发单据上传；单据上传与资料同步相互独立。</li>
        <li>已同步资料在后续快照中意外消失时整次失败并保留上一版；联系人可由主体移除，历史业务快照不改写。</li>
      </ul>
    </ContentCard>
  </>;
}
