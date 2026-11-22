export type SyncTone = "success" | "warning" | "error" | "info" | "neutral";
type Label = { label: string; tone: SyncTone };

export const syncLabels: Record<string, Label> = {
  pending: { label: "待同步", tone: "warning" },
  waiting_predecessor: { label: "等待前序", tone: "warning" },
  temporary_failure: { label: "待自动重试", tone: "warning" },
  needs_admin: { label: "需处理", tone: "error" },
  conflict: { label: "冲突暂停", tone: "error" },
  synced: { label: "已同步", tone: "success" },
  failed: { label: "待自动重试", tone: "warning" },
  untracked: { label: "未纳入同步", tone: "neutral" },
};

export function isOldSitePause(reason?: string) {
  return reason === "old_site_code" || reason === "出单时的厂区编码已更换，已暂停上传，请联系管理员处理";
}

export function ticketSyncLabel(status?: string, enabled?: boolean, bound?: boolean, reason?: string): Label {
  if (status === "synced") return syncLabels.synced;
  if (enabled === false) return { label: "同步已关闭", tone: "neutral" };
  if (status === "needs_admin" && isOldSitePause(reason)) return { label: "旧编码暂停", tone: "error" };
  if (status === "conflict" || status === "needs_admin") return syncLabels[status];
  if (enabled === true && bound === false) return { label: "待资料同步", tone: "warning" };
  return syncLabels[status ?? "untracked"] ?? { label: "同步状态未知", tone: "neutral" };
}

export const reconcileLabels: Record<string, Label> = {
  consistent: { label: "一致", tone: "success" },
  pending: { label: "待同步", tone: "warning" },
  needs_review: { label: "待核查", tone: "warning" },
  conflict: { label: "冲突", tone: "error" },
  unknown: { label: "结果未知", tone: "neutral" },
};

export const changeTypeLabels: Record<string, string> = {
  issue: "出单",
  void: "作废",
  reopen: "重开",
};

export const categoryLabels: Record<string, string> = {
  temporary: "临时故障",
  predecessor_pending: "等待前序",
  config_or_data: "配置或数据错误",
  conflict: "冲突",
};

export const protocolLabel = (value?: number) => value === 6 ? "摘要 v6" : value === 5 ? "摘要 v5" : value === 4 ? "摘要 v4" : value === 3 ? "旧版 v3" : "未纳入";
