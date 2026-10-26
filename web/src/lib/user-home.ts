import type { CurrentUser } from '@/types';
import { WORKBENCH_PATH } from '@/lib/workbench-window';

export function isWeighingOperator(user: CurrentUser | null): boolean {
  return Boolean(user?.roles.some(role => role.roleCode === 'WEIGHING_OPERATOR')) &&
    !user?.roles.some(role => role.roleCode === 'ADMIN');
}

export function userHome(user: CurrentUser | null): string {
  return isWeighingOperator(user) ? WORKBENCH_PATH : '/dashboard';
}

export function loginDestination(user: CurrentUser | null, requested?: string | null): string {
  if (!requested || requested === '/' || !requested.startsWith('/') || requested.startsWith('//') || requested.includes('://') || requested.includes('\\')) {
    return userHome(user);
  }
  return requested;
}
