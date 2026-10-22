import { useAuthStore } from '@/store/auth-store';
import { buildApiUrl } from '@/lib/http';

const base = '/api/system/agent-maintenance';

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  const token = useAuthStore.getState().token;
  if (token) headers.set('Authorization', token);
  if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
  let response: Response;
  try {
    response = await fetch(buildApiUrl(`${base}${path}`), { ...options, headers });
  } catch (error) {
    throw new Error(error instanceof Error ? error.message : '网络请求失败');
  }
  if (response.status === 401) {
    useAuthStore.getState().clearAuth();
    const redirect = `${location.pathname}${location.search}`;
    location.replace(`/login?redirect=${encodeURIComponent(redirect)}`);
  }
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `维护请求失败（${response.status}）`);
  }
  if (response.status === 204) return undefined as T;
  const contentType = response.headers.get('Content-Type') ?? '';
  return (contentType.includes('text/') ? await response.text() : await response.json()) as T;
}

export type MaintenanceConfig = {
  config: Record<string, unknown>;
  active: Record<string, unknown>;
  saved?: boolean;
  hasAgentKey?: boolean;
  hasActiveAgentKey?: boolean;
  keyChanged?: boolean;
  environmentKnown?: boolean;
  environmentError?: string;
  runtimeState?: string;
  hasPrevious?: boolean;
  [key: string]: unknown;
};

export const agentMaintenanceApi = {
  status: () => request<Record<string, unknown>>('/status'),
  config: () => request<MaintenanceConfig>('/config'),
  saveConfig: (config: Record<string, unknown>) => request<unknown>('/config', { method: 'PUT', body: JSON.stringify(config) }),
  importConfig: (connection: unknown, confirm = false) => request<Record<string, unknown>>('/config/import', { method: 'POST', body: JSON.stringify({ connection, confirm }) }),
  jobs: () => request<{ jobs?: { id: string; queue: string; title?: string }[]; unknown?: boolean; previewToken?: string; queuesOutput?: string }>('/jobs'),
  logs: () => request<string>('/logs'),
  audit: (query: URLSearchParams) => request<{ records: Record<string, unknown>[]; total: number; offset: number }>(`/audit?${query}`),
  action: (body: Record<string, unknown>) => request<{ output?: string; needsDisposition?: boolean; previewToken?: string; jobs?: { id: string; queue: string; title?: string }[]; unknown?: boolean }>('/action', { method: 'POST', body: JSON.stringify(body) }),
  testPrint: () => request<{ message?: string }>('/test-print', { method: 'POST', body: '{}' }),
};
