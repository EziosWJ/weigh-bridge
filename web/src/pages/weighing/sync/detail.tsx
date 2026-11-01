import { RotateCw, Search, Stamp } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { syncApi, type SyncChange, type SyncConfirmation, type SyncReview, type SyncTicketDetail } from "@/api/sync";
import { weighingApi } from "@/api/weighing";
import { ContentCard } from "@/components/common/content-card";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { DetailItem } from "@/components/common/detail-item";
import { EmptyState } from "@/components/common/empty-state";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { TableActions } from "@/components/common/table-actions";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";
import { categoryLabels, changeTypeLabels, protocolLabel, reconcileLabels, syncLabels } from "./labels";

const shortDigest = (value?: string | null) => value ? `${value.slice(0, 12)}…${value.slice(-8)}` : "—";
const fieldLabels: Record<string, string> = {
  ticketNo: "单号", previousTicketNo: "原单号", previousSiteCode: "原单厂区编码", title: "单据标题", plate: "车牌",
  direction: "运输方向", materialCode: "物料编码", materialName: "物料名称", materialSpec: "物料规格",
  partyCode: "供收方编码", partyName: "供收方名称", firstConfirmedAt: "首磅确认时间", secondConfirmedAt: "回磅确认时间",
  issuedAt: "出单时间", issuerName: "出单人", originalGross: "原始毛重", originalTare: "原始皮重",
  finalGross: "最终毛重", finalTare: "最终皮重", finalNet: "最终净重", netAdjustment: "净重调整量",
  netAdjustmentMode: "净重调整方式", netAdjustmentRateBp: "净重调整比例", netAdjustmentCategory: "调整类别编码",
  netAdjustmentCategoryName: "调整类别名称", previousFinalGross: "上版最终毛重", previousFinalTare: "上版最终皮重",
  previousFinalNet: "上版最终净重", previousNetAdjustment: "上版净重调整", grossSource: "毛重来源", tareSource: "皮重来源",
  adjustmentRemark: "调整备注", adjustedBy: "调整人", adjustedAt: "调整时间", driver: "司机", driverPhone: "司机电话",
  contactName: "联系人", contactPhone: "联系电话", status: "单据状态", voidReason: "作废原因", voidedBy: "作废人", voidedAt: "作废时间",
};

type FieldDifference = { field: string; local: unknown; cloud: unknown };

function fieldDifferences(detail: unknown): FieldDifference[] {
  if (!detail || typeof detail !== "object" || !("fieldDifferences" in detail) || !Array.isArray(detail.fieldDifferences)) return [];
  return detail.fieldDifferences.filter((item): item is FieldDifference => item && typeof item === "object" && typeof item.field === "string");
}

function differenceValue(value: unknown): string {
  if (value == null) return "未提供";
  if (value === "") return "空字符串";
  return String(value);
}

export function SyncTicketDetailPage() {
  const { id } = useParams();
  const [detail, setDetail] = useState<SyncTicketDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [canManage, setCanManage] = useState(false);
  const [reviewing, setReviewing] = useState<SyncReview | null>(null);
  const [conclusion, setConclusion] = useState("");
  const [lookupSite, setLookupSite] = useState("");
  const [lookupDigest, setLookupDigest] = useState("");
  const [lookup, setLookup] = useState<SyncConfirmation | null>(null);

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    setError("");
    try {
      const value = await syncApi.detail(id);
      setDetail(value);
      setLookupSite(value.ticket.siteCode ?? "");
    } catch (failure) {
      setError(getErrorMessage(failure, "无法读取同步详情"));
    } finally {
      setLoading(false);
    }
  }, [id]);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    void weighingApi.capabilities().then(value => setCanManage(value.permissionCodes.includes("weighing:sync:manage"))).catch(() => setCanManage(false));
  }, []);

  const retry = async (change: SyncChange) => {
    setBusy(true);
    try {
      const result = await syncApi.retryChange(change.id);
      toast.success({ title: "已排入重试", description: result.message });
      await load();
    } catch (failure) {
      toast.error({ title: "重试失败", description: getErrorMessage(failure, "请稍后重试") });
    } finally {
      setBusy(false);
    }
  };
  const reconcile = async () => {
    setBusy(true);
    try {
      const result = await syncApi.reconcileTicket(id!);
      toast.success({ title: "核对完成", description: result.message });
      await load();
    } catch (failure) {
      toast.error({ title: "核对失败", description: getErrorMessage(failure, "请稍后重试") });
    } finally {
      setBusy(false);
    }
  };
  const recheck = async (review: SyncReview) => {
    setBusy(true);
    try {
      const result = await syncApi.recheck(review.id);
      toast.success({ title: "重新核查完成", description: result.message });
      await load();
    } catch (failure) {
      toast.error({ title: "重新核查失败", description: getErrorMessage(failure, "请稍后重试") });
    } finally {
      setBusy(false);
    }
  };
  const saveConclusion = async () => {
    if (!reviewing) return;
    setBusy(true);
    try {
      await syncApi.recordConclusion(reviewing.id, conclusion);
      toast.success("核查结论已记录");
      setReviewing(null);
      setConclusion("");
      await load();
    } catch (failure) {
      toast.error({ title: "保存失败", description: getErrorMessage(failure, "请稍后重试") });
    } finally {
      setBusy(false);
    }
  };
  const lookupConfirmation = async () => {
    setBusy(true);
    setLookup(null);
    try {
      setLookup(await syncApi.lookupConfirmation(lookupSite.trim(), lookupDigest.trim().toLowerCase()));
    } catch (failure) {
      toast.error({ title: "查询失败", description: getErrorMessage(failure, "请稍后重试") });
    } finally {
      setBusy(false);
    }
  };

  const changeColumns: DataTableColumn<SyncChange>[] = [
    { title: "类型", key: "type", width: 80, render: (_, row) => changeTypeLabels[row.changeType] ?? row.changeType },
    { title: "状态", key: "status", width: 130, render: (_, row) => { const item = syncLabels[row.status] ?? syncLabels.pending; return <StatusTag tone={item.tone}>{item.label}</StatusTag>; } },
    { title: "摘要", key: "digest", width: 190, render: (_, row) => <span className="font-mono text-xs" title={row.digest}>{shortDigest(row.digest)}</span> },
    { title: "类别 / 原因", key: "reason", width: 250, render: (_, row) => <div>{row.category ? <span className="text-xs text-text-tertiary">{categoryLabels[row.category] ?? row.category}</span> : null}<div>{row.reason || "—"}</div></div> },
    { title: "尝试 / 下次", key: "attempts", width: 150, render: (_, row) => <div>{row.attempts} 次{row.nextAttemptAt ? <div className="text-text-tertiary">下次 {formatDateTime(row.nextAttemptAt)}</div> : null}</div> },
    { title: "云端确认", key: "confirmed", width: 165, render: (_, row) => row.syncedAt ? formatDateTime(row.receivedAt ?? row.syncedAt) : "—" },
    { title: "操作", key: "actions", width: 90, nowrap: true, render: (_, row) => canManage && row.status !== "synced" && row.status !== "conflict" ? <TableActions><button type="button" className="text-primary hover:underline" disabled={busy} onClick={() => void retry(row)}>立即重试</button></TableActions> : null },
  ];

  if (loading && !detail) return <><PageHeader title="单据同步详情" /><ContentCard><p role="status">正在加载…</p></ContentCard></>;
  if (error) return <><PageHeader title="单据同步详情" actions={<Link className="text-primary hover:underline" to="/weighing/sync/tickets">返回单据同步</Link>} /><ContentCard><p role="alert" className="text-error">{error}</p><Button className="mt-space-3" onClick={() => void load()}>重新加载</Button></ContentCard></>;
  if (!detail) return <><PageHeader title="单据同步详情" /><ContentCard><EmptyState title="未找到该单据" description="请返回列表核对单据编号。" /></ContentCard></>;
  const ticket = detail.ticket;
  const latest = detail.changes[detail.changes.length - 1];
  const latestSync = syncLabels[latest?.status ?? ticket.syncStatus ?? "untracked"] ?? syncLabels.untracked;
  const reconcileInfo = detail.reconcile ? reconcileLabels[detail.reconcile.status] : null;
  const differences = fieldDifferences(detail.reconcile?.detail);

  return <>
    <PageHeader title="单据同步详情" description={ticket.ticketNo} actions={<><Link className="text-primary hover:underline" to="/weighing/sync/tickets">返回单据同步</Link>{canManage && detail.changes.length > 0 && <Button variant="secondary" disabled={busy} onClick={() => void reconcile()}><Search className="h-4 w-4" aria-hidden />立即核对</Button>}</>} />
    <div className="space-y-space-6">
      <ContentCard title="单据与同步状态" extra={<StatusTag tone={latestSync.tone}>{latestSync.label}</StatusTag>}>
        <div className="grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">
          <DetailItem label="车牌" value={ticket.business?.plate ?? "—"} />
          <DetailItem label="物料 / 供收方" value={`${ticket.business?.materialName ?? "—"} / ${ticket.business?.partyName ?? "—"}`} />
          <DetailItem label="单据状态" value={ticket.status === "valid" ? "有效" : "已作废"} />
          <DetailItem label="出单时间" value={formatDateTime(ticket.issuedAt)} />
          <DetailItem label="同步协议" value={protocolLabel(ticket.syncProtocol)} />
          <DetailItem label="最新摘要" value={<span className="font-mono text-xs" title={latest?.digest}>{shortDigest(latest?.digest)}</span>} />
          <DetailItem label="关联原单" value={detail.predecessor ? <Link className="text-primary hover:underline" to={`/weighing/sync/tickets/${detail.predecessor.id}`}>{detail.predecessor.ticketNo}</Link> : "无"} />
          <DetailItem label="关联重开单" value={detail.successor ? <Link className="text-primary hover:underline" to={`/weighing/sync/tickets/${detail.successor.id}`}>{detail.successor.ticketNo}</Link> : "无"} />
        </div>
        {ticket.syncReason && <p className="mt-space-4 text-sm text-text-secondary">最新变化说明：{ticket.syncReason}</p>}
      </ContentCard>

      <DataTableCard>
        <TableToolbar title="变化与确认状态" description={detail.changes.length ? `共 ${detail.changes.length} 个摘要变化；只有云端确认对应摘要后才标记完成。` : "该单据未纳入摘要同步。"} />
        <DataTable columns={changeColumns} dataSource={detail.changes} rowKey="id" loading={loading} error={error} minWidth={1100} empty={<EmptyState title="暂无摘要变化" description="旧版协议单据保留只读记录，见下方。" />} />
      </DataTableCard>

      {detail.legacyChanges && detail.legacyChanges.length > 0 && <DataTableCard>
        <TableToolbar title="旧版协议变化（只读）" description="已使用 v2/v3 出单的单据继续沿用旧协议；不参与摘要对账，也不与新协议混用。" />
        <DataTable columns={[
          { title: "类型", key: "type", width: 80, render: (_, row) => changeTypeLabels[row.changeType] ?? row.changeType },
          { title: "状态", key: "status", width: 110, render: (_, row) => <StatusTag tone={row.status === "synced" ? "success" : row.status === "failed" ? "error" : "warning"}>{row.status === "synced" ? "已同步" : row.status === "failed" ? "同步失败" : "待同步"}</StatusTag> },
          { title: "变化标识", key: "changeId", width: 220, render: (_, row) => <span className="font-mono text-xs">{row.changeId}</span> },
          { title: "说明", key: "reason", render: (_, row) => row.reason || "—" },
          { title: "时间", key: "time", width: 170, render: (_, row) => formatDateTime(row.syncedAt ?? row.lastAttemptAt ?? row.createdAt) },
        ]} dataSource={detail.legacyChanges} rowKey="id" minWidth={820} />
      </DataTableCard>}

      <ContentCard title="最近对账" extra={reconcileInfo ? <StatusTag tone={reconcileInfo.tone}>{reconcileInfo.label}</StatusTag> : <span className="text-sm text-text-tertiary">尚未对账</span>}>
        {detail.reconcile ? <>
          <div className="grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">
            <DetailItem label="对账时间" value={formatDateTime(detail.reconcile.checkedAt)} />
            <DetailItem label="触发方式" value={detail.reconcile.triggerType === "automatic" ? "自动" : "手动"} />
            <DetailItem label="云端是否找到" value={detail.reconcile.cloudFound === null ? "—" : detail.reconcile.cloudFound ? "找到" : "缺单"} />
            <DetailItem label="云端单据状态" value={detail.reconcile.cloudStatus ?? "—"} />
            <DetailItem label="厂端最新摘要" value={<span className="font-mono text-xs" title={detail.reconcile.localLatestDigest ?? ""}>{shortDigest(detail.reconcile.localLatestDigest)}</span>} />
            <DetailItem label="云端最新摘要" value={<span className="font-mono text-xs" title={detail.reconcile.cloudLatestDigest ?? ""}>{shortDigest(detail.reconcile.cloudLatestDigest)}</span>} />
            <DetailItem label="云端摘要版本" value={detail.reconcile.cloudLatestDigestVersion ?? "—"} />
            <DetailItem label="云端接收时间" value={formatDateTime(detail.reconcile.cloudReceivedAt)} />
          </div>
          {differences.length > 0 && <div className="mt-space-4 overflow-x-auto">
            <table className="w-full text-left text-sm" aria-label="单据字段差异">
              <thead><tr><th className="p-space-2">字段</th><th className="p-space-2">厂端</th><th className="p-space-2">云端</th></tr></thead>
              <tbody>{differences.map(item => <tr key={item.field} className="border-t border-border"><td className="p-space-2">{fieldLabels[item.field] ?? item.field}</td><td className="p-space-2">{differenceValue(item.local)}</td><td className="p-space-2">{differenceValue(item.cloud)}</td></tr>)}</tbody>
            </table>
          </div>}
          <details className="mt-space-4 rounded-control border border-border p-space-3">
            <summary className="cursor-pointer text-sm font-medium">差异与云端变化详情</summary>
            <pre className="mt-space-2 max-h-80 overflow-auto whitespace-pre-wrap text-xs text-text-secondary">{pretty(detail.reconcile.detail)}</pre>
          </details>
          <p className="mt-space-3 text-xs text-text-tertiary">摘要相同只表示双方记录的最新变化摘要一致，不代表云端独立校验了业务内容；查询失败不会显示为一致。</p>
        </> : <p className="text-sm text-text-secondary">尚未执行单张或批量对账。同步确认状态与对账结果分别判断。</p>}
      </ContentCard>

      {detail.reviews.length > 0 && <ContentCard title="冲突、核查结论与处理记录" description="这些记录独立于普通同步日志，清空日志不会删除。">
        <div className="space-y-space-4">
          {detail.reviews.map(review => <div key={review.id} className="rounded-admin border border-border p-space-3">
            <div className="flex flex-wrap items-center gap-space-2">
              <StatusTag tone={review.status === "open" ? (review.blocking ? "error" : "warning") : "neutral"}>{review.status === "open" ? (review.blocking ? "冲突暂停" : "待核查") : "已结束"}</StatusTag>
              <span className="text-sm font-medium">{review.reasonCode || "核查记录"}</span>
              <span className="font-mono text-xs text-text-tertiary" title={review.digest}>{shortDigest(review.digest)}</span>
              <span className="text-xs text-text-tertiary">最近检查 {formatDateTime(review.lastCheckedAt)}</span>
            </div>
            {review.conclusion ? <p className="mt-space-2 text-sm">核查结论：{review.conclusion}<span className="ml-space-2 text-text-tertiary">{review.concludedByName || "管理员"} · {formatDateTime(review.concludedAt)}</span></p> : <p className="mt-space-2 text-sm text-text-tertiary">尚未记录核查结论。</p>}
            <details className="mt-space-2"><summary className="cursor-pointer text-xs text-text-secondary">差异详情</summary><pre className="mt-space-1 max-h-64 overflow-auto whitespace-pre-wrap text-xs text-text-secondary">{pretty(review.detail)}</pre></details>
            {canManage && review.status === "open" && <div className="mt-space-3 flex flex-wrap gap-space-2">
              <Button size="sm" variant="secondary" disabled={busy} onClick={() => { setReviewing(review); setConclusion(review.conclusion); }}><Stamp className="h-4 w-4" aria-hidden />记录核查结论</Button>
              <Button size="sm" disabled={busy || !review.conclusion.trim()} onClick={() => void recheck(review)}><RotateCw className="h-4 w-4" aria-hidden />重新核查</Button>
            </div>}
          </div>)}
        </div>
      </ContentCard>}

      {canManage && <ContentCard title="旧编码历史确认查询" description="按出单时的厂区编码和已保存摘要只读查询云端确认，用于回执丢失；不能借此继续写入旧编码变化。">
        <div className="grid gap-space-3 sm:grid-cols-3">
          <Field label="出单厂区编码" htmlFor="sync-lookup-site"><Input id="sync-lookup-site" value={lookupSite} maxLength={40} onChange={event => setLookupSite(event.target.value)} /></Field>
          <Field label="变化摘要" htmlFor="sync-lookup-digest"><Input id="sync-lookup-digest" value={lookupDigest} maxLength={64} onChange={event => setLookupDigest(event.target.value)} /></Field>
          <div className="flex items-end"><Button variant="secondary" disabled={busy || lookupSite.trim() === "" || lookupDigest.trim().length !== 64} onClick={() => void lookupConfirmation()}>查询云端确认</Button></div>
        </div>
        {lookup && <div className="mt-space-3 rounded-control border border-border p-space-3 text-sm">
          {lookup.found && lookup.confirmation ? <div className="grid gap-space-2 sm:grid-cols-2"><DetailItem label="单号" value={lookup.confirmation.ticketNo} /><DetailItem label="类型" value={changeTypeLabels[lookup.confirmation.changeType] ?? lookup.confirmation.changeType} /><DetailItem label="摘要版本" value={`${lookup.confirmation.digestVersion} / 协议 ${lookup.confirmation.schemaVersion}`} /><DetailItem label="云端接收时间" value={formatDateTime(lookup.confirmation.receivedAt)} /></div> : <p className="text-text-secondary">云端未保存该摘要（包括被拒绝的变化），不代表内容无效。</p>}
        </div>}
      </ContentCard>}
    </div>
    <FormDialog open={!!reviewing} title="记录核查结论" description="结论只作为处理记录保存；恢复同步仍需明确发起重新核查。" loading={busy} submitDisabled={!conclusion.trim()} submitText="保存结论" onCancel={() => setReviewing(null)} onSubmit={() => void saveConclusion()}>
      <Field label="核查结论" htmlFor="sync-conclusion" required help="请记录差异原因、处理方式或维护人员结论。">
        <Textarea id="sync-conclusion" rows={5} maxLength={2000} value={conclusion} onChange={event => setConclusion(event.target.value)} />
      </Field>
    </FormDialog>
  </>;
}

function pretty(value: unknown) {
  if (value == null) return "—";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}
