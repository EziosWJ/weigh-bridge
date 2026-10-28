import { http } from '@/lib/http';
import type { ApiPageResult } from '@/types/api';
import type { Material, MaterialCategory, Party, PartyContact } from '@/types/weighing';

const base = '/api/weighing';
export interface WeighingSite {code:string;name:string;enabled:number;canOperate:boolean;version:number;serverUrl?:string}
export interface WeighingScale {siteCode:string;code:string;name:string;enabled:number;siteName:string;canOperate:boolean}
export interface SiteGrant {siteCode:string;canOperate:number}
export interface DailyTicketSequence {businessDate:string;lastIssued:number}
export interface WeighingCapabilities {
 localSiteCode: string; localScaleCode: string; configured: boolean; initialized: boolean;
 maintenanceEnabled?: boolean; masterSyncBound?: boolean;
 sites: WeighingSite[];
  userId: number;
  permissionCodes: string[];
  scales: WeighingScale[];
}
type WireMeta = { id: number; enabled: number; version: number };
type WireCategory = Omit<MaterialCategory, 'id' | 'enabled'> & WireMeta;
type WireMaterial = Omit<Material, 'id' | 'enabled' | 'categoryId'> & WireMeta & { categoryId: number | null };
type WireContact = Omit<PartyContact, 'id' | 'isDefault'> & { id: number; isDefault: number };
type WireParty = Omit<Party, 'id' | 'enabled' | 'contacts'> & WireMeta & { contacts: WireContact[] };
async function allPages<T>(path: string): Promise<T[]> {
  const rows: T[] = [];
  let page = 1;
  for (;;) {
    const result = await http.get<ApiPageResult<T>>(`${base}/${path}`, { query: { page, pageSize: 500 } });
    rows.push(...result.records);
    if (rows.length >= result.total || !result.records.length) return rows;
    page++;
  }
}
export const weighingApi = {
  siteOptions: () => http.get<WeighingSite[]>(`${base}/site-options`),
 siteGrants: (id:number) => http.get<SiteGrant[]>(`${base}/users/${id}/sites`),
 saveSiteGrants: (id:number,grants:SiteGrant[]) => http.put(`${base}/users/${id}/sites`,{grants}),
 capabilities: () => http.get<WeighingCapabilities>(`${base}/capabilities`),
 masterSyncStatus: () => http.get<{enabled:boolean;bound:boolean;lastSuccess:string|null;isAdmin:boolean}>(`${base}/master-sync/status`),
 masterSyncLogs: () => http.get<{id:number;attemptedAt:string;triggerType:string;success:boolean;reason:string}[]>(`${base}/master-sync/logs`),
 clearMasterSyncLogs: () => http.delete(`${base}/master-sync/logs`),
 runMasterSync: () => http.post<{enabled:boolean;bound:boolean;lastSuccess:string|null;isAdmin:boolean}>(`${base}/master-sync`, {}),
 siteConfiguration: () => http.get<unknown[]>(`${base}/site-config`),
 initializeLocalSite: (input: { siteCode: string; scaleCode?: string; name?: string; scaleName?: string }) => http.post(`${base}/site-config/initialize`, input),
 saveSiteConfiguration: (input: { code?: string; scaleCode?: string; name: string; scaleName: string; enabled: number; version: number }) => http.put(`${base}/site-config`, input),
 ticketSequences: () => http.get<DailyTicketSequence[]>(`${base}/ticket-sequences`),
 setTicketWatermark: (input: { businessDate: string; lastIssued: number }) => http.put<DailyTicketSequence>(`${base}/ticket-sequences`, input),
  categories: async (): Promise<MaterialCategory[]> => (await allPages<WireCategory>('material-categories')).map(row => ({ ...row, id: String(row.id), enabled: row.enabled === 1 })),
  materials: async (): Promise<Material[]> => (await allPages<WireMaterial>('materials')).map(row => ({ ...row, id: String(row.id), categoryId: row.categoryId == null ? '' : String(row.categoryId), enabled: row.enabled === 1, unit: '吨' })),
  parties: async (): Promise<Party[]> => (await allPages<WireParty>('parties')).map(row => ({ ...row, id: String(row.id), enabled: row.enabled === 1, contacts: row.contacts.map(c => ({ ...c, id: String(c.id), isDefault: c.isDefault === 1 })) })),
  save: (path: string, id: string | undefined, body: unknown) => id ? http.put(`${base}/${path}/${id}`, body) : http.post(`${base}/${path}`, body),
  remove: (path: string, id: string, version: number | undefined) => http.delete(`${base}/${path}/${id}`, { version }),
};
