import type { WeighingCapabilities } from '@/api/weighing';

// The local database is the only source of the site's identity. Never infer it
// from account grants, which are business permissions rather than identity.
export function localSiteId(capabilities: Pick<WeighingCapabilities, 'localSiteId'>): number | undefined {
  return capabilities.localSiteId > 0 ? capabilities.localSiteId : undefined;
}

export const localSiteSetupMessage = '本厂内部身份应在服务启动时自动建立，请检查服务日志与数据库。';
