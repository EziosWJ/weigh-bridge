import type { Config, ConfigData, JobsData, Status } from './types';

export class SessionExpired extends Error {}

export async function request<T>(path: string, options: RequestInit = {}, asText = false): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin', ...options,
    headers: { 'Content-Type': 'application/json', ...options.headers },
  });
  if (!response.ok) {
    const message = await response.text();
    if (response.status === 401) throw new SessionExpired(message);
    throw new Error(message);
  }
  return asText ? await response.text() as T : response.json();
}
export const post = <T,>(path: string, data: unknown) => request<T>(path, { method: 'POST', body: JSON.stringify(data) });
export const fetchStatus = () => request<Status>('/api/status');
export const fetchConfig = () => request<ConfigData>('/api/config');
export const fetchJobs = () => request<JobsData>('/api/jobs');
export const saveConfig = (data: Config) => request('/api/config', { method: 'PUT', body: JSON.stringify(data) });
export const keys = { status: ['status'], config: ['private', 'config'], jobs: ['private', 'jobs'], audit: ['private', 'audit'] };
