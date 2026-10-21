import { useEffect, useState } from 'react';
import { weighingApi, type WeighingSite, type SiteGrant } from '@/api/weighing';
import { FormDialog } from '@/components/common/form-dialog';
import { Select } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { getErrorMessage } from '@/lib/api-error';
import type { UserRecord } from '@/types';

export function SiteAssignDialog({user,onClose}:{user:UserRecord;onClose:()=>void}){
 const [sites,setSites]=useState<WeighingSite[]>([]);
 const [grants,setGrants]=useState<SiteGrant[]>([]);
 const [loading,setLoading]=useState(true);
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState('');
 const [revision,setRevision]=useState(0);
 useEffect(()=>{
  let active=true;setLoading(true);setError('');
  void Promise.all([weighingApi.siteOptions(),weighingApi.siteGrants(user.id)]).then(([options,rows])=>{if(active){setSites(options);setGrants(rows);}}).catch(e=>{if(active)setError(getErrorMessage(e,'厂区授权加载失败'));}).finally(()=>{if(active)setLoading(false);});
  return()=>{active=false;};
 },[user.id,revision]);
 async function save(){setBusy(true);setError('');try{await weighingApi.saveSiteGrants(user.id,grants);onClose();}catch(e){setError(getErrorMessage(e,'厂区授权保存失败'));}finally{setBusy(false);}}
 return <FormDialog open title="厂区授权" description={`${user.nickname||user.username} · 业务动作仍受角色权限控制，查询授权不赋予操作权。`} loading={busy} submitDisabled={loading||!!error} onCancel={onClose} onSubmit={save}>
  {loading?<p role="status">正在加载厂区授权…</p>:<div className="space-y-3">{sites.map(site=>{
   const grant=grants.find(g=>g.siteId===site.id);
   return <label key={site.id} className="flex flex-wrap items-center justify-between gap-3 text-sm"><span>{site.name} <span className="text-text-tertiary">{site.code}</span></span><Select aria-label={`${site.name}授权范围`} disabled={busy} value={grant?grant.canOperate?'operate':'read':'none'} onChange={e=>setGrants(old=>[...old.filter(g=>g.siteId!==site.id),...(e.target.value==='none'?[]:[{siteId:site.id,canOperate:e.target.value==='operate'?1:0}])])}><option value="none">未授权</option><option value="read">仅查询</option><option value="operate">查询与操作</option></Select></label>;
  })}{!sites.length&&<p>暂无启用厂区。</p>}</div>}
  {error&&<div role="alert" className="mt-3 text-sm text-error">{error}<Button size="sm" onClick={()=>setRevision(v=>v+1)}>重新加载</Button></div>}
 </FormDialog>;
}
