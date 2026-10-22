import { http } from '@/lib/http';
export interface NodeStatus {
 role: 'central' | 'site'; siteId: number; initialized: boolean;
 sites: {siteId:number;lastSuccess:string;lastError:string;pendingImages:number;pending:number;connected:boolean}[];
}
export interface TemporaryParty {partyId:number;siteId:number;name:string;state:string;revision:number;formalPartyId:number|null;reason:string}
export interface PendingConfiguration {revision:number;appliedRevision:number;configuration:{name:string;scaleName:string;enabled:number;serverUrl:string}}
export const nodeApi = {
 status: () => http.get<NodeStatus>('/api/node/status'),
 temporary: () => http.get<TemporaryParty[]>('/api/weighing/temporary-parties'),
 saveTemporary: (id:number|undefined,body:unknown) => id ? http.put(`/api/weighing/temporary-parties/${id}`,body) : http.post('/api/weighing/temporary-parties',body),
 review: (id:number,body:unknown) => http.post(`/api/weighing/temporary-parties/${id}/review`,body),
 pendingConfiguration: () => http.get<PendingConfiguration>('/api/weighing/configuration/pending'),
 applyConfiguration: (revision:number) => http.post('/api/weighing/configuration/apply',{revision}),
};
