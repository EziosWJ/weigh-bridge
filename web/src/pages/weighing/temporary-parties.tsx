import { useCallback, useEffect, useState } from 'react';
import { nodeApi, type TemporaryParty } from '@/api/node';
import { weighingApi } from '@/api/weighing';
import { useWeighingStore } from '@/store/weighing-store';
import { PageHeader } from '@/components/common/page-header';
import { SearchFilterBar } from '@/components/common/search-filter-bar';
import { DataTableCard } from '@/components/common/data-table-card';
import { DataTable } from '@/components/common/data-table';
import { TableToolbar } from '@/components/common/table-toolbar';
import { TableActions } from '@/components/common/table-actions';
import { Pagination } from '@/components/common/pagination';
import { FormDialog } from '@/components/common/form-dialog';
import { Field } from '@/components/common/field';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { getErrorMessage } from '@/lib/api-error';
import type { DataTableColumn } from '@/types';
import { NodeSyncStatus } from './node-sync-status';

const labels:Record<string,string>={pending:'待中心确认',returned:'退回补充',resubmitted:'补充后待确认',approved:'已转正式',linked:'已关联正式主体'};
const legacyLabels:Record<string,string>={pending:'历史待确认状态',returned:'历史退回状态',resubmitted:'历史再次提交状态',approved:'历史已转正式',linked:'历史已关联正式主体'};
export function TemporaryPartiesPage(){
 const caps=useWeighingStore(s=>s.capabilities);
 const [rows,setRows]=useState<TemporaryParty[]>([]);
 const [keyword,setKeyword]=useState('');const [page,setPage]=useState(1);
 const [loading,setLoading]=useState(true);const [busy,setBusy]=useState(false);const [error,setError]=useState('');
 const [editing,setEditing]=useState<{id?:number;version?:number;name:string;nature:string;roles:string[];phone:string;address:string}|null>(null);
 const [review,setReview]=useState<TemporaryParty|null>(null);
 const [action,setAction]=useState('approve');const [reason,setReason]=useState('');const [formal,setFormal]=useState('');
 const [formalOptions,setFormalOptions]=useState<{id:string;name:string}[]>([]);
 const refresh=useCallback(async()=>{setLoading(true);try{setRows(await nodeApi.temporary());setError('');}catch(e){setError(getErrorMessage(e,'临时主体读取失败'));}finally{setLoading(false);}},[]);
 useEffect(()=>{void refresh();},[refresh]);
 const central=caps?.nodeRole==='central';
 const standalone=caps?.nodeRole==='standalone';
 const canCreate=!standalone&&caps?.nodeRole==='site'&&caps.permissionCodes.includes('weighing:temporary:create')&&caps.sites.some(s=>s.canOperate);
 const canReview=central&&caps?.permissionCodes.includes('weighing:master:write');
 const run=async(fn:()=>Promise<void>)=>{setBusy(true);setError('');try{await fn();await refresh();}catch(e){setError(getErrorMessage(e,'操作失败'));}finally{setBusy(false);}};
 const columns:DataTableColumn<TemporaryParty>[]=[
  {title:'临时主体',key:'name',render:(_,r)=><span className="font-medium">{r.name}</span>},
  {title:'来源厂区',dataIndex:'siteId'},
  {title:'处理状态',key:'state',render:(_,r)=>(standalone?legacyLabels:labels)[r.state]??r.state},
  {title:standalone?'历史处理信息':'中心意见 / 正式档案',key:'reason',render:(_,r)=>r.reason||(r.formalPartyId?`正式主体 ${r.formalPartyId}`:'—')},
  {title:'操作',key:'actions',render:(_,r)=><TableActions>
   {canCreate&&r.state==='returned'&&<Button variant="ghost" disabled={busy} onClick={()=>void run(async()=>{const data=(await weighingApi.parties()).find(p=>p.id===String(r.partyId));if(!data)throw new Error('主体不存在');setEditing({id:r.partyId,version:data.version,name:data.name,nature:data.nature,roles:data.roles,phone:data.phone,address:data.address});})}>补充并提交</Button>}
   {canReview&&['pending','resubmitted'].includes(r.state)&&<Button variant="ghost" disabled={busy} onClick={()=>void run(async()=>{setFormalOptions((await weighingApi.parties()).filter(p=>!rows.some(r=>r.partyId===Number(p.id))));setReview(r);setAction('approve');setReason('');setFormal('');})}>处理</Button>}
  </TableActions>},
 ];
 const filtered=rows.filter(r=>r.name.includes(keyword));
 return <>
  <PageHeader title={standalone?'遗留临时主体记录':'临时往来主体'} description={standalone?'保留历史来源、状态和关联快照；本厂新资料请在往来主体中直接建档。':'同名不会自动归并；中心确认及关联保留来源和历史单据快照。'} actions={canCreate&&<Button variant="primary" onClick={()=>setEditing({name:'',nature:'enterprise',roles:['supplier'],phone:'',address:''})}>补录临时主体</Button>}/>
  <NodeSyncStatus/>
  <SearchFilterBar><Input aria-label="查找临时主体" placeholder="主体名称" value={keyword} onChange={e=>{setKeyword(e.target.value);setPage(1);}}/></SearchFilterBar>
  {error&&<p role="alert" className="mb-3 text-error">{error}</p>}
  <DataTableCard><TableToolbar title={standalone?'历史主体记录':'临时主体列表'} description={standalone?'历史状态仅供查询，不再触发中心审批或资料下发。':'初次待确认仅本厂可选；退回及再次提交后须中心确认才可用于新任务。'} actions={<Button onClick={()=>void refresh()}>刷新</Button>}/><DataTable columns={columns} dataSource={filtered.slice((page-1)*10,page*10)} rowKey="partyId" loading={loading} minWidth={760}/><Pagination page={page} pageSize={10} total={filtered.length} onPageChange={setPage}/></DataTableCard>
  <FormDialog open={!!editing} title={editing?.id?'补充临时主体':'补录临时主体'} loading={busy} onCancel={()=>setEditing(null)} onSubmit={()=>run(async()=>{if(!editing)return;const previous=editing.id?(await weighingApi.parties()).find(p=>p.id===String(editing.id)):undefined;await nodeApi.saveTemporary(editing.id,{...previous,...editing,enabled:true,contacts:previous?.contacts.map(c=>({...c,id:Number(c.id)}))??[]});setEditing(null);await useWeighingStore.getState().refresh();})}>
   {editing&&<>
    <Field label="主体名称" required><Input required maxLength={100} value={editing.name} onChange={e=>setEditing({...editing,name:e.target.value})}/></Field>
    <Field label="主体性质"><Select value={editing.nature} onChange={e=>setEditing({...editing,nature:e.target.value})}><option value="enterprise">企业</option><option value="individual">个人</option></Select></Field>
    <Field label="业务身份"><div className="flex gap-3">{[['supplier','供货商'],['customer','客户'],['carrier','承运商']].map(([value,label])=><label key={value}><input type="checkbox" checked={editing.roles.includes(value)} onChange={e=>setEditing({...editing,roles:e.target.checked?[...editing.roles,value]:editing.roles.filter(r=>r!==value)})}/>{label}</label>)}</div></Field>
    <Field label="联系电话"><Input value={editing.phone} onChange={e=>setEditing({...editing,phone:e.target.value})}/></Field>
    <Field label="地址"><Input value={editing.address} onChange={e=>setEditing({...editing,address:e.target.value})}/></Field>
   </>}
   {error&&<p role="alert" className="text-error">{error}</p>}
  </FormDialog>
  <FormDialog open={!!review} title={`处理：${review?.name??''}`} loading={busy} onCancel={()=>setReview(null)} onSubmit={()=>run(async()=>{if(!review)return;await nodeApi.review(review.partyId,{revision:review.revision,action,reason,formalPartyId:formal?Number(formal):null});setReview(null);})}>
   <Field label="处理方式"><Select value={action} onChange={e=>setAction(e.target.value)}><option value="approve">确认转正式</option><option value="link">关联已有正式主体</option><option value="return">退回补充</option></Select></Field>
   {action==='link'&&<Field label="正式主体" required><Select required value={formal} onChange={e=>setFormal(e.target.value)}><option value="">请选择</option>{formalOptions.map(p=><option key={p.id} value={p.id}>{p.name}（{p.id}）</option>)}</Select></Field>}
   {action==='return'&&<Field label="退回原因" required><Input required value={reason} onChange={e=>setReason(e.target.value)}/></Field>}
   {error&&<p role="alert" className="text-error">{error}</p>}
  </FormDialog>
 </>;
}
