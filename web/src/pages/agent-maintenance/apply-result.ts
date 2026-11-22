import type { ApplyResult } from '@/api/agent-maintenance';

export function applyResultLabel(status: ApplyResult['status']) {
  return {
    applied: '已核实生效',
    rejected: '配置未应用',
    unknown: '应用结果未知，请刷新状态核实',
    rolled_back: '应用失败，已恢复原配置',
    rollback_failed: '应用及回退失败，请检查设备',
  }[status];
}

export function appliedVersion(result: ApplyResult) {
  return '生效版本 v' + (result.effective?.configVersion ?? result.configVersion);
}
