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
    let message = await response.text();
    try { const body = JSON.parse(message) as { message?: string }; if (body.message) message = body.message; } catch { /* Legacy plain-text maintenance errors. */ }
    throw new Error(message || `维护请求失败（${response.status}）`);
  }
  if (response.status === 204) return undefined as T;
  const contentType = response.headers.get('Content-Type') ?? '';
  return (contentType.includes('text/') ? await response.text() : await response.json()) as T;
}

export type SerialSettings = { port: string; baud: number };
export type PrintSettings = {
  queue: string;
  formLengthMm: number;
  printWidthMm: number;
  printScale: number;
  offsetXmm: number;
  offsetYmm: number;
};
export type DeviceSettings = { serial: SerialSettings; print: PrintSettings };
export type EffectiveConfig = {
  configVersion: number;
  digest: string;
  serial: SerialSettings;
  print: PrintSettings;
  appliedAt?: string;
  running: boolean;
  deviceState: string;
  serialState?: string;
  printerState?: string;
};
export type BackupRecord = {
  id: number;
  name: string;
  notes: string;
  createdBy?: number;
  configVersion: number;
  digest: string;
  settings: DeviceSettings;
  appliedAt: string;
  source: string;
  restoredFromId?: number;
  receivedAt?: string;
};
export type CupsQueue = { name: string; state: string; description?: string; isDefault: boolean };
export type CupsQueuesResult = { queues: CupsQueue[]; at?: string; code?: string; error?: string };
export type ApplyResult = {
  status: 'applied' | 'rejected' | 'unknown' | 'rolled_back' | 'rollback_failed';
  code?: string;
  message?: string;
  configVersion: number;
  digest: string;
  appliedAt?: string;
  effective?: EffectiveConfig;
};

export type MaintenanceConfig = {
  config: Record<string, unknown>;
  active: Record<string, unknown>;
  effective?: EffectiveConfig;
  backup?: BackupRecord;
  liveBackup?: Record<string, unknown>;
  capabilities?: Record<string, boolean>;
  saved?: boolean;
  environmentKnown?: boolean;
  environmentError?: string;
  runtimeState?: string;
  hasPrevious?: boolean;
  [key: string]: unknown;
};

export const agentMaintenanceApi = {
  backups: (page = 1) => request<{ records: BackupRecord[]; total: number; effective?: EffectiveConfig; live: boolean }>(`/backups?offset=${(page - 1) * 10}&limit=10`),
  createBackup: (body: { name: string; notes: string; baseVersion: number; digest: string }) => request<{ id: number }>('/backups', { method: 'POST', body: JSON.stringify(body) }),
  deleteBackup: (id: number) => request<void>(`/backups/${id}`, { method: 'DELETE' }),
  status: () => request<Record<string, unknown>>('/status'),
  config: () => request<MaintenanceConfig>('/config'),
  apply: (settings: DeviceSettings, baseVersion: number) => request<ApplyResult>('/apply', {
    method: 'POST', body: JSON.stringify({ baseVersion, settings }),
  }),
  restore: (backupId: number, baseVersion: number) => request<ApplyResult>('/restore', {
    method: 'POST', body: JSON.stringify({ backupId, baseVersion }),
  }),
  queues: () => request<CupsQueuesResult>('/queues'),
  jobs: () => request<{ jobs?: { id: string; queue: string; title?: string }[]; unknown?: boolean; previewToken?: string; queuesOutput?: string }>('/jobs'),
  logs: () => request<string>('/logs'),
  audit: (query: URLSearchParams) => request<{ records: Record<string, unknown>[]; total: number; offset: number }>(`/audit?${query}`),
  action: (body: Record<string, unknown>) => request<{ output?: string; needsDisposition?: boolean; previewToken?: string; jobs?: { id: string; queue: string; title?: string }[]; unknown?: boolean }>('/action', { method: 'POST', body: JSON.stringify(body) }),
  testPrint: () => request<{ message?: string }>('/test-print', { method: 'POST', body: '{}' }),
};
