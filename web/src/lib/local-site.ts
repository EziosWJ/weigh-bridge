import type { WeighingCapabilities } from '@/api/weighing';

// The local database is the only source of the site's identity. Never infer it
// from account grants, which are business permissions rather than identity.
export function localSiteCode(capabilities: Pick<WeighingCapabilities, 'localSiteCode'>): string | undefined {
  return capabilities.localSiteCode ? capabilities.localSiteCode : undefined;
}

export const localSiteSetupMessage = '本厂身份尚未配置，请管理员在系统管理→本厂身份填写总部分配的厂区编码。';
