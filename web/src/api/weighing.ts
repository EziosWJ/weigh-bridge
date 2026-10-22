import { http } from '@/lib/http';
import type { ApiPageResult } from '@/types/api';
import type { Material, MaterialCategory, Party, PartyContact } from '@/types/weighing';

const base = '/api/weighing';
export interface WeighingSite {id:number;code:string;name:string;enabled:number;canOperate:boolean}
export interface WeighingScale {id:number;code:string;name:string;enabled:number;siteId:number;siteName:string;canOperate:boolean}
export interface SiteGrant {siteId:number;canOperate:number}
export interface WeighingCapabilities {
 nodeRole?: 'central' | 'site' | 'standalone'; nodeSiteId?: number;
 maintenanceEnabled?: boolean;
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
  categories: async (): Promise<MaterialCategory[]> => (await allPages<WireCategory>('material-categories')).map(row => ({ ...row, id: String(row.id), enabled: row.enabled === 1 })),
  materials: async (): Promise<Material[]> => (await allPages<WireMaterial>('materials')).map(row => ({ ...row, id: String(row.id), categoryId: row.categoryId == null ? '' : String(row.categoryId), enabled: row.enabled === 1, unit: '吨' })),
  parties: async (): Promise<Party[]> => (await allPages<WireParty>('parties')).map(row => ({ ...row, id: String(row.id), enabled: row.enabled === 1, contacts: row.contacts.map(c => ({ ...c, id: String(c.id), isDefault: c.isDefault === 1 })) })),
  save: (path: string, id: string | undefined, body: unknown) => id ? http.put(`${base}/${path}/${id}`, body) : http.post(`${base}/${path}`, body),
  remove: (path: string, id: string, version: number | undefined) => http.delete(`${base}/${path}/${id}`, { version }),
};
