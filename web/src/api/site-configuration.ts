import { http } from '@/lib/http';

export interface SiteConfiguration {
 serverUrl?:string; pendingRevision?:number; appliedRevision?:number; id: number; code: string; name: string; enabled: number; version: number;
 scaleId: number; scaleCode: string; scaleName: string; agentId: string;
}
export type SiteConfigurationInput = Omit<SiteConfiguration, 'id' | 'scaleId' | 'pendingRevision' | 'appliedRevision'>;
export interface AgentConnection {
 serverUrl: string; siteId: number; siteCode: string; siteName: string;
 scaleId: number; scaleCode: string; scaleName: string; agentId: string; agentKey: string;
}
const base = '/api/weighing';
export const siteConfigurationApi = {
 list: () => http.get<SiteConfiguration[]>(`${base}/site-config`),
 settings: () => http.get<{serverUrl: string}>(`${base}/agent-settings`),
 saveSettings: (serverUrl: string) => http.put(`${base}/agent-settings`, {serverUrl}),
 save: (id: number | undefined, body: SiteConfigurationInput) => id ? http.put<SiteConfiguration>(`${base}/site-config/${id}`, body) : http.post<SiteConfiguration>(`${base}/site-config`, body),
 connection: (id: number) => http.get<AgentConnection>(`${base}/site-config/${id}/connection`),
};
