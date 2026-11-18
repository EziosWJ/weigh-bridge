import { RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { syncApi, type CloudConnection } from "@/api/sync";
import { ContentCard } from "@/components/common/content-card";
import { DetailItem } from "@/components/common/detail-item";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";

const readinessLabels = {
  disabled: { label: "同步已关闭", tone: "neutral" },
  unconfigured: { label: "未配置云端地址", tone: "warning" },
  invalid_address: { label: "云端地址格式无效", tone: "warning" },
  ready: { label: "云端已就绪", tone: "success" },
  not_ready: { label: "云端未就绪", tone: "warning" },
  unreachable: { label: "无法连接云端", tone: "error" },
  unexpected_response: { label: "云端就绪接口响应异常", tone: "error" },
} as const satisfies Record<CloudConnection["readiness"]["status"], { label: string; tone: "neutral" | "warning" | "success" | "error" }>;

export function CloudConnectionPage() {
  const [connection, setConnection] = useState<CloudConnection | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    void syncApi.connection(controller.signal).then(setConnection).catch((failure: unknown) => {
      if (!controller.signal.aborted) setError(getErrorMessage(failure, "无法读取云端连接状态"));
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [refresh]);

  const readiness = connection ? readinessLabels[connection.readiness.status] : null;
  const attempt = connection?.lastAttempt;

  return <>
    <PageHeader title="云端连接" actions={<Button variant="secondary" disabled={loading} onClick={() => {
      setError("");
      setLoading(true);
      setRefresh(value => value + 1);
    }}>
      <RefreshCw className="h-4 w-4" aria-hidden />{loading ? "正在检测…" : "重新检测"}
    </Button>} />
    {error && <p role="alert" className="mb-space-4 text-sm text-error">{error}</p>}
    <div className="max-w-5xl space-y-space-6" aria-busy={loading}>
      {loading && !connection && <p role="status" className="text-sm text-text-secondary">正在读取配置和云端就绪状态…</p>}
      {connection && <>
        <ContentCard title="厂端生效配置" extra={<StatusTag tone={connection.enabled ? "info" : "neutral"}>{connection.enabled ? "同步已开启" : "同步已关闭"}</StatusTag>}>
          <div className="grid gap-space-4 sm:grid-cols-3">
            <DetailItem className="sm:col-span-2" label="对接云端地址" value={connection.address
              ? <code className="break-all font-mono">{connection.address}</code>
              : connection.readiness.status === "invalid_address" ? "配置格式无效" : "无有效地址"} />
            <DetailItem label="本厂编码" value={connection.siteCode || "未配置"} />
          </div>
        </ContentCard>
        <ContentCard title="云端就绪检测" extra={readiness && <StatusTag tone={readiness.tone}>{readiness.label}</StatusTag>}>
          <DetailItem label="检测时间（北京时间）" value={formatDateTime(connection.readiness.checkedAt)} />
        </ContentCard>
        <ContentCard title="基础资料同步" extra={<Link className="text-sm text-primary hover:underline" to="/weighing/sync/logs">查看同步日志</Link>}>
          <div className="grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">
            <DetailItem label="资料模式" value={connection.bound ? "已应用总部资料" : connection.enabled ? "待首次同步" : "单机模式"} />
            <DetailItem label="最近一次同步结果" value={attempt
              ? <StatusTag tone={attempt.success ? "success" : "error"}>{attempt.success ? "成功" : "失败"}</StatusTag>
              : "暂无记录"} />
            <DetailItem label="最近一次同步时间" value={formatDateTime(attempt?.attemptedAt)} />
            <DetailItem label="最近成功同步时间" value={formatDateTime(connection.lastSuccess)} />
            {attempt && !attempt.success && attempt.reason && <DetailItem className="sm:col-span-2 xl:col-span-4" label="失败原因" value={attempt.reason} />}
          </div>
        </ContentCard>
      </>}
    </div>
  </>;
}
