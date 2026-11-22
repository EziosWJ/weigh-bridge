import { RotateCw, Search, Stamp } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { syncApi, type SyncChange, type SyncConfirmation, type SyncReview, type SyncTicketDetail } from "@/api/sync";
import { weighingApi } from "@/api/weighing";
import { HelpHint } from "@/components/common/help-hint";
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
import { buttonStyles } from "@/components/ui/button-styles";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage } from "@/lib/api-error";
import { directionLabels } from "@/types/weighing";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";
import { categoryLabels, changeTypeLabels, isOldSitePause, protocolLabel, reconcileLabels, reviewReasonLabels, ticketSyncLabel } from "./labels";

const fieldLabels: Record<string, string> = {
  entryType: "出单来源", firstWeighedAt: "首磅实际时间", secondWeighedAt: "回磅实际时间", netAdjustments: "净重调整明细",
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

const weightFields = new Set([
  "originalGross", "originalTare", "finalGross", "finalTare", "finalNet", "netAdjustment",
  "previousFinalGross", "previousFinalTare", "previousFinalNet", "previousNetAdjustment", "amount",
]);

const valueLabels: Record<string, Record<string, string>> = {
  status: { valid: "有效", void: "作废" },
  direction: directionLabels,
  entryType: { manual_supplement: "人工补单" },
  grossSource: { collected: "设备称重", manual: "人工录入", adjusted: "人工更正", manual_supplement: "人工补单" },
  tareSource: { collected: "设备称重", manual: "人工录入", adjusted: "人工更正", manual_supplement: "人工补单" },
  mode: { amount: "按重量", percent: "按比例" },
};

function differenceValue(field: string, value: unknown): string {
  if (value == null) return "未提供";
  if (value === "") return "空字符串";
  if (field === "netAdjustmentRateBp" || field === "rateBp") {
    const rate = Number(value);
    if (Number.isFinite(rate)) return `${rate / 100}%`;
  }
  if (weightFields.has(field)) return `${String(value)} 吨`;
  if (field.endsWith("At") && typeof value === "string") return formatDateTime(value);
  if (field === "netAdjustments" && Array.isArray(value)) {
    if (!value.length) return "无调整项";
    return value.map(item => {
      if (!item || typeof item !== "object") return "调整项未知";
      const category = item.categoryName || item.category || "未分类";
      const rate = item.mode === "percent" ? differenceValue("rateBp", item.rateBp) : "";
      return [category, rate, differenceValue("amount", item.amount), item.remark].filter(Boolean).join("，");
    }).join("\n");
  }
  const labelKey = field === "netAdjustmentMode" ? "mode" : field;
  return valueLabels[labelKey] ? valueLabels[labelKey][String(value)] ?? "未知" : String(value);
}

function DifferencesTable({ items, label }: { items: FieldDifference[]; label: string }) {
  return <div className="overflow-x-auto rounded-control border border-border">
    <table className="w-full min-w-[300px] table-fixed text-left text-sm" aria-label={label}>
      <thead className="bg-neutral-background text-text-secondary"><tr><th scope="col" className="w-[28%] p-space-3 font-medium">字段</th><th scope="col" className="p-space-3 font-medium">厂端</th><th scope="col" className="p-space-3 font-medium">云端</th></tr></thead>
      <tbody>{items.map(item => <tr key={item.field} className="border-t border-border">
        <th scope="row" className="p-space-3 text-left font-medium [overflow-wrap:anywhere]">{fieldLabels[item.field] ?? "其他字段"}</th>
        <td className="whitespace-pre-line p-space-3 [overflow-wrap:anywhere]">{differenceValue(item.field, item.local)}</td>
        <td className="whitespace-pre-line p-space-3 [overflow-wrap:anywhere]">{differenceValue(item.field, item.cloud)}</td>
      </tr>)}</tbody>
    </table>
  </div>;
}

export function SyncTicketDetailPage() {
  const { id } = useParams();
  const [detail, setDetail] = useState<SyncTicketDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [capabilities, setCapabilities] = useState<Awaited<ReturnType<typeof weighingApi.capabilities>> | null>(null);
  const canManage = capabilities?.permissionCodes.includes("weighing:sync:manage") === true;
  const enabled = capabilities?.masterSyncEnabled;
  const bound = capabilities?.masterSyncBound;
  const syncActive = enabled !== false && bound !== false;
  const [actionError, setActionError] = useState("");
  const [conclusionError, setConclusionError] = useState("");
  const [lookupError, setLookupError] = useState("");
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
    void weighingApi.capabilities().then(setCapabilities).catch(() => setCapabilities(null));
  }, []);

  const retry = async (change: SyncChange) => {
    setBusy(true);
    setActionError("");
    try {
      const result = await syncApi.retryChange(change.id, change.protocol);
      toast.success({ title: "已排入重试", description: result.message });
      await load();
    } catch (failure) {
      setActionError(getErrorMessage(failure, "重试失败"));
    } finally {
      setBusy(false);
    }
  };
  const reconcile = async () => {
    setBusy(true);
    setActionError("");
    try {
      const result = await syncApi.reconcileTicket(id!);
      toast.success({ title: "核对完成", description: result.message });
      await load();
    } catch (failure) {
      setActionError(getErrorMessage(failure, "核对失败"));
    } finally {
      setBusy(false);
    }
  };
  const recheck = async (review: Pick<SyncReview, "id">) => {
    setBusy(true);
    setActionError("");
    try {
      const result = await syncApi.recheck(review.id);
      toast.success({ title: "重新核查完成", description: result.message });
    } catch (failure) {
      setActionError(getErrorMessage(failure, "重新核查失败"));
    } finally {
      await load();
      setBusy(false);
    }
  };
  const saveConclusion = async (recheckAfterSave = false) => {
    if (!reviewing || !conclusion.trim() || busy) return;
    setBusy(true);
    setConclusionError("");
    try {
      await syncApi.recordConclusion(reviewing.id, conclusion);
    } catch (failure) {
      setConclusionError(getErrorMessage(failure, "保存核查结论失败"));
      setBusy(false);
      return;
    }
    toast.success("核查结论已记录");
    setReviewing(null);
    setConclusion("");
    if (recheckAfterSave) {
      await recheck(reviewing);
    } else {
      await load();
      setBusy(false);
    }
  };
  const lookupConfirmation = async () => {
    setBusy(true);
    setLookup(null);
    setLookupError("");
    try {
      setLookup(await syncApi.lookupConfirmation(lookupSite.trim(), lookupDigest.trim().toLowerCase()));
    } catch (failure) {
      setLookupError(getErrorMessage(failure, "查询云端确认失败"));
    } finally {
      setBusy(false);
    }
  };

  const changeColumns: DataTableColumn<SyncChange>[] = [
    { title: "类型", key: "type", width: 80, render: (_, row) => changeTypeLabels[row.changeType] ?? row.changeType },
    { title: "状态", key: "status", width: 130, render: (_, row) => { const item = ticketSyncLabel(row.status, enabled, bound, row.reasonCode || row.reason); return <StatusTag tone={item.tone}>{item.label}</StatusTag>; } },
    { title: "类别 / 原因", key: "reason", render: (_, row) => <div>{row.category ? <span className="text-xs text-text-tertiary">{categoryLabels[row.category] ?? "其他原因"}</span> : null}<div>{row.reason || "—"}</div></div> },
    { title: "尝试 / 下次", key: "attempts", width: 172, render: (_, row) => <div className="tabular-nums">{row.attempts} 次{row.nextAttemptAt ? <div className="text-text-tertiary"><span className="block">下次</span><span className="whitespace-nowrap">{formatDateTime(row.nextAttemptAt)}</span></div> : null}</div> },
    { title: "云端确认", key: "confirmed", width: 175, nowrap: true, render: (_, row) => <span className="tabular-nums">{row.syncedAt ? formatDateTime(row.receivedAt ?? row.syncedAt) : "—"}</span> },
  ];
  if (canManage) changeColumns.push({
    title: "操作", key: "actions", width: 112, nowrap: true,
    render: (_, row) => syncActive && row.status !== "synced" && row.status !== "conflict" && !isOldSitePause(row.reasonCode || row.reason) && !detail?.reviews.some(review => review.status === "open" && review.blocking)
      ? <TableActions><Button size="sm" disabled={busy} onClick={() => void retry(row)}>立即重试</Button></TableActions>
      : <span className="text-text-tertiary">—</span>,
  });

  if (loading && !detail) return <><PageHeader title="单据同步详情" /><ContentCard><p role="status">正在加载…</p></ContentCard></>;
  if (error) return <><PageHeader title="单据同步详情" actions={<Link className={buttonStyles()} to="/weighing/sync/tickets">返回单据同步</Link>} /><ContentCard><p role="alert" className="text-error">{error}</p><Button className="mt-space-3" onClick={() => void load()}>重新加载</Button></ContentCard></>;
  if (!detail) return <><PageHeader title="单据同步详情" /><ContentCard><EmptyState title="未找到该单据" description="请返回列表核对单据编号。" /></ContentCard></>;
  const ticket = detail.ticket;
  const latest = detail.changes.at(-1);
  const legacyLatest = detail.legacyChanges?.at(-1);
  const latestReason = latest?.reasonCode || latest?.reason || legacyLatest?.reason || ticket.syncReason;
  const latestSync = ticketSyncLabel(latest?.status ?? legacyLatest?.status ?? ticket.syncStatus, enabled, bound, latestReason);
  const oldSitePaused = isOldSitePause(latestReason);
  const reconcileInfo = detail.reconcile ? reconcileLabels[detail.reconcile.status] ?? reconcileLabels.unknown : null;
  const differences = fieldDifferences(detail.reconcile?.detail);
  const reviews = [...detail.reviews].sort((a, b) => Number(b.status === "open") - Number(a.status === "open"));
  const hasOpenReview = reviews.some(review => review.status === "open");
  return <>
    <PageHeader title="单据同步详情" actions={<>
      <Link className={buttonStyles()} to="/weighing/sync/tickets">返回单据同步</Link>
      {canManage && detail.changes.length > 0 && <Button variant="primary" disabled={busy || !syncActive} onClick={() => void reconcile()}><Search className="h-4 w-4" aria-hidden />立即核对</Button>}
    </>} />
    <div className="space-y-space-6">
      <ContentCard title="单据与同步状态" extra={<StatusTag tone={latestSync.tone}>{latestSync.label}</StatusTag>}>
        <div className="grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">
          <DetailItem label="单号" value={ticket.ticketNo} />
          <DetailItem label="车牌" value={ticket.business?.plate ?? "—"} />
          <DetailItem label="物料 / 供收方" value={`${ticket.business?.materialName ?? "—"} / ${ticket.business?.partyName ?? "—"}`} />
          <DetailItem label="单据状态" value={ticket.status === "valid" ? "有效" : "已作废"} />
          <DetailItem label="出单时间" value={formatDateTime(ticket.issuedAt)} />
          <DetailItem label="关联原单" value={detail.predecessor ? <Link className="text-primary hover:underline" to={`/weighing/sync/tickets/${detail.predecessor.id}`}>{detail.predecessor.ticketNo}</Link> : "无"} />
          <DetailItem label="关联重开单" value={detail.successor ? <Link className="text-primary hover:underline" to={`/weighing/sync/tickets/${detail.successor.id}`}>{detail.successor.ticketNo}</Link> : "无"} />
          {(hasOpenReview || latestSync.tone === "error") && <DetailItem label="处理责任" value="本厂管理员" />}
        </div>
        {(latest?.reason || legacyLatest?.reason || ticket.syncReason) && <p className="mt-space-3 text-sm text-text-secondary">{latest?.reason || legacyLatest?.reason || ticket.syncReason}</p>}
        {canManage && hasOpenReview && <a className={buttonStyles({ size: "sm", className: "mt-space-3" })} href="#sync-reviews">处理核查</a>}
        {canManage && oldSitePaused && latest?.digest && <a className={buttonStyles({ size: "sm", className: "mt-space-3" })} href="#confirmation-lookup" onClick={() => {
          setLookupSite(latest.siteCode);
          setLookupDigest(latest.digest ?? "");
          setLookup(null);
          setLookupError("");
        }}>查询历史回执</a>}
        {actionError && <p role="alert" className="mt-space-3 text-sm text-error">{actionError}</p>}
      </ContentCard>

      <ContentCard title="最近对账" extra={<div className="flex items-center gap-space-2">
        {reconcileInfo && <StatusTag tone={reconcileInfo.tone}>{reconcileInfo.label}</StatusTag>}
        <HelpHint label="对账结果说明" align="right">对账以云端保存的单据和变化确认记录为依据。连接失败时结果为未知。</HelpHint>
      </div>}>
        {detail.reconcile ? <>
          {differences.length > 0 && <DifferencesTable items={differences} label="单据字段差异" />}
          <div className={`grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4 ${differences.length ? "mt-space-4" : ""}`}>
            <DetailItem label="对账时间" value={formatDateTime(detail.reconcile.checkedAt)} />
            <DetailItem label="触发方式" value={detail.reconcile.triggerType === "automatic" ? "自动" : "手动"} />
            <DetailItem label="云端单据" value={detail.reconcile.cloudFound == null ? "未知" : detail.reconcile.cloudFound ? differenceValue("status", detail.reconcile.cloudStatus) : "缺单"} />
            <DetailItem label="云端接收时间" value={formatDateTime(detail.reconcile.cloudReceivedAt)} />
          </div>
          <details className="mt-space-4 rounded-control border border-border p-space-3">
            <summary className="cursor-pointer text-sm font-medium">对账技术详情</summary>
            <pre className="mt-space-2 max-h-80 overflow-auto whitespace-pre-wrap break-all text-xs text-text-secondary">{pretty(detail.reconcile.detail)}</pre>
          </details>
        </> : <p className="text-sm text-text-secondary">尚未对账</p>}
      </ContentCard>

      {reviews.length > 0 && <ContentCard title="核查记录">
        <div id="sync-reviews" className="space-y-space-4">
          {reviews.map(review => {
            const reviewDifferences = fieldDifferences(review.detail);
            const open = review.status === "open";
            return <div key={review.id} className="rounded-admin border border-border p-space-3">
              <div className="flex flex-wrap items-center gap-space-2">
                <StatusTag tone={open ? (review.blocking ? "error" : "warning") : "neutral"}>{open ? (review.blocking ? "冲突暂停" : "待核查") : "已结束"}</StatusTag>
                <span className="text-sm font-medium">{reviewReasonLabels[review.reasonCode] ?? "需核查的同步差异"}</span>
              </div>
              <DetailItem className="mt-space-3" label="最近核查" value={formatDateTime(review.lastCheckedAt)} />
              {reviewDifferences.length > 0 && !differences.length && <div className="mt-space-3"><DifferencesTable items={reviewDifferences} label="核查字段差异" /></div>}
              {review.conclusion && <div className="mt-space-3 grid gap-space-3 sm:grid-cols-2">
                <DetailItem label="核查结论" value={review.conclusion} />
                <DetailItem label="结论记录人" value={[review.concludedByName || "管理员", formatDateTime(review.concludedAt)].join(" / ")} />
              </div>}
              {canManage && open && <div className="mt-space-3 flex flex-wrap gap-space-2">
                <Button size="sm" variant="secondary" disabled={busy} onClick={() => {
                  setReviewing(review);
                  setConclusion(review.conclusion);
                  setConclusionError("");
                }}><Stamp className="h-4 w-4" aria-hidden />{review.conclusion ? "更新核查结论" : "记录核查结论"}</Button>
                <Button size="sm" disabled={busy || !syncActive || !review.conclusion.trim()} onClick={() => void recheck(review)}><RotateCw className="h-4 w-4" aria-hidden />重新核查</Button>
              </div>}
              <details className="mt-space-3"><summary className="cursor-pointer text-sm text-text-secondary">核查技术详情</summary>
                <p className="mt-space-2 break-all font-mono text-xs">{review.digest}</p>
                <pre className="mt-space-2 max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs text-text-secondary">{pretty(review.detail)}</pre>
              </details>
            </div>;
          })}
        </div>
      </ContentCard>}

      <DataTableCard>
        <TableToolbar title="变化与确认状态" />
        <DataTable columns={changeColumns} dataSource={detail.changes} rowKey="id" loading={loading} error={error} minWidth={canManage ? 820 : 708} compact stickyHeader fixedEdges={canManage} ariaLabel="变化与确认状态" empty={<EmptyState title="暂无摘要变化" />} />
      </DataTableCard>

      {detail.legacyChanges && detail.legacyChanges.length > 0 && <DataTableCard>
        <TableToolbar title="旧版协议变化（只读）" />
        <DataTable columns={[
          { title: "类型", key: "type", width: 80, render: (_, row) => changeTypeLabels[row.changeType] ?? row.changeType },
          { title: "状态", key: "status", width: 130, render: (_, row) => { const item = ticketSyncLabel(row.status, enabled, bound, row.reason); return <StatusTag tone={item.tone}>{item.label}</StatusTag>; } },
          { title: "变化标识", key: "changeId", width: 220, render: (_, row) => <span className="font-mono text-xs">{row.changeId}</span> },
          { title: "说明", key: "reason", render: (_, row) => row.reason || "—" },
          { title: "时间", key: "time", width: 170, render: (_, row) => formatDateTime(row.syncedAt ?? row.lastAttemptAt ?? row.createdAt) },
        ]} dataSource={detail.legacyChanges} rowKey="id" minWidth={780} compact stickyHeader ariaLabel="旧版协议变化" />
      </DataTableCard>}

      {detail.changes.length > 0 && <ContentCard>
        <details>
          <summary className="cursor-pointer text-sm font-medium">上传技术详情</summary>
          <ul className="mt-space-3 space-y-space-3">
            {detail.changes.map(change => <li key={`${change.protocol}-${change.id}`} className="text-sm">
              <span>{changeTypeLabels[change.changeType]} / {protocolLabel(change.protocol)} / {formatDateTime(change.createdAt)}</span>
              <code className="mt-space-1 block break-all text-xs">{change.digest}</code>
              {canManage && change.digest && <a href="#confirmation-lookup" className="text-sm text-primary hover:underline" onClick={() => {
                setLookupSite(change.siteCode);
                setLookupDigest(change.digest ?? "");
                setLookup(null);
                setLookupError("");
              }}>填入回执查询</a>}
            </li>)}
          </ul>
        </details>
      </ContentCard>}

      {canManage && <ContentCard title="旧编码历史确认查询" extra={<HelpHint label="历史确认查询规则" align="right">按出单时的厂区编码和已保存的变化摘要查询对应云端回执。</HelpHint>}>
        <div id="confirmation-lookup" className="grid gap-space-3 sm:grid-cols-3">
          <Field label="出单厂区编码" htmlFor="sync-lookup-site"><Input id="sync-lookup-site" value={lookupSite} maxLength={40} onChange={event => setLookupSite(event.target.value)} /></Field>
          <Field label="变化摘要" htmlFor="sync-lookup-digest"><Input id="sync-lookup-digest" value={lookupDigest} maxLength={64} onChange={event => setLookupDigest(event.target.value)} /></Field>
          <div className="flex items-end"><Button variant="secondary" disabled={busy || enabled === false || lookupSite.trim() === "" || lookupDigest.trim().length !== 64} onClick={() => void lookupConfirmation()}>查询云端确认</Button></div>
        </div>
        {lookupError && <p role="alert" className="mt-space-3 text-sm text-error">{lookupError}</p>}
        {lookup && <div className="mt-space-3 rounded-control border border-border p-space-3 text-sm">
          {lookup.found && lookup.confirmation ? <div className="grid gap-space-2 sm:grid-cols-2"><DetailItem label="单号" value={lookup.confirmation.ticketNo} /><DetailItem label="类型" value={changeTypeLabels[lookup.confirmation.changeType] ?? lookup.confirmation.changeType} /><DetailItem label="摘要版本" value={`${lookup.confirmation.digestVersion} / 协议 ${lookup.confirmation.schemaVersion}`} /><DetailItem label="云端接收时间" value={formatDateTime(lookup.confirmation.receivedAt)} /></div> : <p className="text-text-secondary">未查到该变化的云端确认。</p>}
        </div>}
      </ContentCard>}
    </div>
    <FormDialog open={!!reviewing} title="记录核查结论" loading={busy} submitDisabled={!conclusion.trim()} submitText="保存结论" onCancel={() => setReviewing(null)} onSubmit={() => void saveConclusion()}>
      <Field label="核查结论" htmlFor="sync-conclusion" required error={conclusionError}>
        <Textarea id="sync-conclusion" rows={5} maxLength={2000} value={conclusion} onChange={event => setConclusion(event.target.value)} />
      </Field>
      {syncActive && <Button variant="secondary" disabled={busy || !conclusion.trim()} onClick={() => void saveConclusion(true)}>保存并重新核查</Button>}
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
