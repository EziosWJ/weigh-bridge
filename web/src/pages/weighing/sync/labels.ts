export type SyncTone = "success" | "warning" | "error" | "info" | "neutral";

export const syncLabels: Record<string, { label: string; tone: SyncTone }> = {
  pending: { label: "待同步", tone: "warning" },
  waiting_predecessor: { label: "等待前序", tone: "warning" },
  temporary_failure: { label: "暂时失败", tone: "error" },
  needs_admin: { label: "需管理员处理", tone: "error" },
  conflict: { label: "冲突暂停", tone: "error" },
  synced: { label: "已同步", tone: "success" },
  failed: { label: "同步失败", tone: "error" },
  untracked: { label: "未纳入同步", tone: "neutral" },
};

export const reconcileLabels: Record<string, { label: string; tone: SyncTone }> = {
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

export const protocolLabel = (value?: number) => value === 4 ? "摘要 v4" : value === 3 ? "旧版 v3" : "未纳入";
