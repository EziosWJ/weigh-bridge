import { useCallback, useEffect, useState } from 'react';
import { weighingApi, type WeighingCapabilities, type WeighingScale } from '@/api/weighing';
import { workflowApi } from '@/api/weighing-workflow';
import { ContentCard } from '@/components/common/content-card';
import { EmptyState } from '@/components/common/empty-state';
import { PageHeader } from '@/components/common/page-header';
import { StatusTag } from '@/components/common/status-tag';
import { WorkbenchLink } from '@/components/common/workbench-link';
import { FormDialog } from '@/components/common/form-dialog';
import { Button } from '@/components/ui/button';
import { getErrorMessage } from '@/lib/api-error';
import type { DeviceState } from '@/types/weighing';

export function WeighingSitesPage() {
 const [capabilities,setCapabilities]=useState<WeighingCapabilities|null>(null);
 const [devices,setDevices]=useState<Record<number,DeviceState>>({});
 const [loading,setLoading]=useState(true);
 const [error,setError]=useState('');
 const [notice,setNotice]=useState('');
 const [target,setTarget]=useState<{scale:WeighingScale;connectionId:string}|null>(null);
 const [busy,setBusy]=useState(false);
 const refresh=useCallback(async()=>{
  try {
   const caps=await weighingApi.capabilities();
   const states=await Promise.all(caps.scales.map(async s=>[s.id,await workflowApi.get<DeviceState>(`/scales/${s.id}/device`)] as const));
   setCapabilities(caps);setDevices(Object.fromEntries(states));setError('');
  } catch(e){setError(getErrorMessage(e,'厂区状态加载失败'));}
  finally{setLoading(false);}
 },[]);
 useEffect(()=>{void refresh();const timer=setInterval(()=>void refresh(),3000);return()=>clearInterval(timer);},[refresh]);
 async function kick(){
  if(!target)return;setBusy(true);setError('');
  try{
   const result=await workflowApi.post<{disconnected:boolean;exitConfirmed:boolean}>(`/scales/${target.scale.id}/agent/kick`,{connectionId:target.connectionId});
   setNotice(result.exitConfirmed?'Agent 已确认收到踢出指令，连接已断开。':'连接已断开，Agent 退出未确认，请检查现场进程。');setTarget(null);await refresh();
  }catch(e){setError(getErrorMessage(e,'踢出失败'));}finally{setBusy(false);}
 }
 const canConfirm=capabilities?.permissionCodes.includes('weighing:confirm')??false;
 const canKick=capabilities?.permissionCodes.includes('weighing:agent:kick')??false;
 return <>
  <PageHeader title="厂区工作台" description="选择已授权厂区，进入对应地磅的独立称重工作台。" actions={<Button onClick={()=>void refresh()} disabled={busy}>刷新状态</Button>}/>
  {error&&<p role="alert" className="mb-4 text-sm text-error">{error}</p>}
  {notice&&<p role="status" className="mb-4 text-sm text-text-secondary">{notice}</p>}
  {loading?<p role="status">正在加载厂区…</p>:capabilities?.sites.length===0?<EmptyState title="暂无已授权厂区" description="请联系管理员分配厂区查询或操作权限。"/>:
   <div className="grid gap-space-4 md:grid-cols-2 xl:grid-cols-3">{capabilities?.sites.map(site=><ContentCard key={site.id} title={site.name} description={site.code} extra={<StatusTag tone={site.canOperate?'success':'neutral'}>{site.canOperate?'可操作':'仅查询'}</StatusTag>}>
    <div className="space-y-4">{capabilities.scales.filter(s=>s.siteId===site.id&&s.enabled===1).map(scale=>{
     const device=devices[scale.id];
     return <section key={scale.id} className="space-y-3" aria-label={scale.name}>
      <div className="flex items-center justify-between gap-3"><span className="font-medium">{scale.name}</span><StatusTag tone={!device?'neutral':device.online?'success':'warning'}>{!device?'状态未知':device.online?'Agent 在线':'Agent 离线'}</StatusTag></div>
      <p className="text-sm text-text-secondary">{device?.agentId||'尚未配置 Agent'}{device?.online&&!device.deviceOk?' · 采集设备未就绪':''}</p>
      <div className="flex flex-wrap items-center gap-3">
       {scale.canOperate&&canConfirm?<WorkbenchLink className="text-sm font-medium text-primary hover:underline" to={`/weighing/workbench?scaleId=${scale.id}`}>进入称重台</WorkbenchLink>:<span className="text-sm text-text-tertiary">当前无称重操作权限</span>}
       {scale.canOperate&&canKick&&device?.connectionId&&<Button size="sm" variant="danger" disabled={busy} onClick={()=>{setError('');setTarget({scale,connectionId:device.connectionId!});}}>踢出 Agent</Button>}
      </div>
     </section>;
    })}{!capabilities.scales.some(s=>s.siteId===site.id&&s.enabled===1)&&<p className="text-sm text-text-tertiary">暂无启用地磅</p>}</div>
   </ContentCard>)}</div>}
  <FormDialog open={!!target} title="踢出 Agent" description={target?`${target.scale.siteName} · ${target.scale.name}`:''} loading={busy} submitText="确认踢出" onCancel={()=>setTarget(null)} onSubmit={kick}>
   <p className="text-sm">收到指令的 Agent 将退出，未确认采样失效。已提交的打印作业可能继续出纸，结果未知时须人工核实。</p>
   {error&&<p role="alert" className="mt-3 text-sm text-error">{error}</p>}
  </FormDialog>
 </>;
}
