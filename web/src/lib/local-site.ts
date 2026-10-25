import type { WeighingCapabilities } from '@/api/weighing';

// The local database is the only source of the site's identity. Never infer it
// from account grants, which are business permissions rather than identity.
export function localSiteId(capabilities: Pick<WeighingCapabilities, 'localSiteId'>): number | undefined {
  return capabilities.localSiteId > 0 ? capabilities.localSiteId : undefined;
}

export const localSiteSetupMessage = '请由本厂管理员完成首次配置。配置后厂区、地磅编号及 Agent 身份将固定保存。';
