import { http } from '@/lib/http';

export interface SiteConfiguration {
 serverUrl?:string; // legacy compatibility field; never an editable connection source
 id: number; code: string; name: string; enabled: number; version: number;
 scaleId: number; scaleCode: string; scaleName: string; agentId: string;
}
export type SiteConfigurationInput = Pick<SiteConfiguration, 'name' | 'scaleName' | 'enabled' | 'version'> &
 Partial<Pick<SiteConfiguration, 'code' | 'scaleCode' | 'agentId' | 'serverUrl'>>;
export interface LocalSiteStatus { siteId: number; configured: boolean; initialized: boolean }
const base = '/api/weighing';
export const siteConfigurationApi = {
 list: () => http.get<SiteConfiguration[]>(`${base}/site-config`),
 localStatus: () => http.get<LocalSiteStatus>(`${base}/local-status`),
 initialize: (body: {name:string;scaleName:string}) => http.post<SiteConfiguration>(`${base}/site-config/initialize`, body),
 save: (id: number, body: SiteConfigurationInput) => http.put<SiteConfiguration>(`${base}/site-config/${id}`, body),
};
