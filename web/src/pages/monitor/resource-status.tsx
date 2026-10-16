import type { ReactNode } from "react";
import { ContentCard } from "@/components/common/content-card";
import { DataTable } from "@/components/common/data-table";
import { StatusTag } from "@/components/common/status-tag";
import { cn } from "@/lib/utils";

export type ResourceStatus = { status: string; collectedAt?: string | null; message?: string; stale?: boolean; partial?: boolean; data?: unknown };
const labels: Record<string, string> = { ok: "正常", collecting: "采集中", no_device: "无设备", unsupported: "不支持", error: "采集失败", stale: "过期" };

export function ResourceSection({ title, state, now, children, className, bodyClassName }: { title: string; state: ResourceStatus; now: number; children?: ReactNode; className?: string; bodyClassName?: string }) {
  const expired = state.stale || Boolean(state.collectedAt && now - Date.parse(state.collectedAt) > 15000);
  const retained = state.data != null && state.collectedAt && ["error", "unsupported"].includes(state.status);
  const status = `${labels[state.status] ?? state.status}${expired && state.status !== "stale" ? " · 过期" : ""}`;
  return <div role="region" aria-label={title} className={cn("min-w-0", className)}><ContentCard title={title} extra={<StatusTag tone={state.status === "ok" && !expired ? "success" : "warning"}>{status}</StatusTag>} className={cn("shadow-none", className)} bodyClassName={bodyClassName}>
    <p className="mb-3 text-xs text-text-tertiary">有效采集时间：{state.collectedAt ? new Date(state.collectedAt).toLocaleString() : "尚无有效样本"}{expired ? "；超过15秒无有效新样本，以下为保留的旧值" : retained ? "；保留上次有效值" : ""}</p>
    {state.message && <p role="status" className="mb-3 text-sm text-warning">{state.message}</p>}
    {state.partial && <p className="mb-3 text-sm text-warning">覆盖不完整，以下仅展示有效结果；部分字段或设备不可读取。</p>}
    {children}
  </ContentCard></div>;
}

export function ResourceTable({ headers, rows }: { headers: string[]; rows: ReactNode[][] }) {
  return <DataTable columns={headers.map((title, index) => ({ key: String(index), title, nowrap: true, render: (_: unknown, row: { cells: ReactNode[] }) => row.cells[index] }))} dataSource={rows.map((cells, id) => ({ id, cells }))} rowKey="id" />;
}
