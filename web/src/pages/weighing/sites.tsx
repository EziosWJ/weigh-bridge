import { Navigate } from 'react-router-dom';
import { useCallback, useEffect, useState } from 'react';
import { ExternalLink, LogOut, RefreshCw, Scale } from 'lucide-react';
import { weighingApi, type WeighingCapabilities, type WeighingScale } from '@/api/weighing';
import { workflowApi } from '@/api/weighing-workflow';
import { ContentCard } from '@/components/common/content-card';
import { EmptyState } from '@/components/common/empty-state';
import { PageHeader } from '@/components/common/page-header';
import { StatusTag } from '@/components/common/status-tag';
import { WorkbenchLink } from '@/components/common/workbench-link';
import { FormDialog } from '@/components/common/form-dialog';
import { Button } from '@/components/ui/button';
import { buttonStyles } from '@/components/ui/button-styles';
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
   const states=caps.nodeRole==='central'?[]:await Promise.all(caps.scales.map(async s=>[s.id,await workflowApi.get<DeviceState>(`/scales/${s.id}/device`)] as const));
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
 if(capabilities?.nodeRole==='central')return <Navigate to="/weighing/tickets" replace/>;
 return <>
  <PageHeader title="厂区工作台" description="选择已授权厂区，进入对应地磅的独立称重工作台。" actions={<Button onClick={()=>void refresh()} disabled={busy}><RefreshCw className="h-4 w-4" aria-hidden />刷新状态</Button>}/>
  {error&&<p role="alert" className="mb-4 text-sm text-error">{error}</p>}
  {notice&&<p role="status" className="mb-4 text-sm text-text-secondary">{notice}</p>}
  {loading?<p role="status">正在加载厂区…</p>:capabilities?.sites.length===0?<EmptyState title="暂无已授权厂区" description="请联系管理员分配厂区查询或操作权限。"/>:
   <div className="grid items-start gap-space-4 lg:grid-cols-2 2xl:grid-cols-3">{capabilities?.sites.map(site=><ContentCard key={site.id} className="min-w-0 overflow-hidden shadow-none [&>header]:bg-neutral-background [&>header>div]:min-w-0 [&>header>div]:break-words [&>header>span]:shrink-0" bodyClassName="!p-0" title={site.name} description={`厂区编号：${site.code}`} extra={<StatusTag tone={site.enabled!==0&&site.canOperate?'success':'neutral'}>{site.enabled===0?'已停用 · 历史可查':site.canOperate?'可操作':'仅查询'}</StatusTag>}>
    <div className="divide-y divide-border">{capabilities.scales.filter(s=>s.siteId===site.id&&s.enabled===1).map(scale=>{
     const device=devices[scale.id];
     return <section key={scale.id} className="p-card" aria-label={scale.name}>
      <div className="overflow-hidden rounded-control border border-border">
       <div className="flex flex-wrap items-center justify-between gap-space-3 border-b border-border bg-neutral-background px-space-3 py-space-3">
        <div className="flex min-w-0 items-center gap-space-2"><Scale className="h-4 w-4 shrink-0 text-text-tertiary" aria-hidden /><h3 className="min-w-0 break-words font-medium">{scale.name}</h3></div>
        <StatusTag tone={!device?'neutral':device.online?'success':'warning'}>{!device?'状态未知':device.online?'Agent 在线':'Agent 离线'}</StatusTag>
       </div>
       <dl className="grid grid-cols-[auto_minmax(0,1fr)] text-sm">
        <dt className="border-r border-border bg-neutral-background px-space-3 py-space-3 text-text-secondary">现场 Agent</dt><dd className="break-all px-space-3 py-space-3 text-text-primary">{device?.agentId||'尚未接入 Agent'}</dd>
        <dt className="border-r border-t border-border bg-neutral-background px-space-3 py-space-3 text-text-secondary">采集设备</dt><dd className={`border-t border-border px-space-3 py-space-3 ${device?.online&&!device.deviceOk?'text-warning':'text-text-primary'}`}>{!device?'状态未知':!device.online?'等待 Agent 上线':device.deviceOk?'已就绪':'未就绪'}</dd>
       </dl>
      </div>
      <div className="mt-space-4 flex flex-wrap items-center gap-space-2">
       {scale.canOperate&&canConfirm?<WorkbenchLink className={buttonStyles({variant:'primary'})} to={`/weighing/workbench?scaleId=${scale.id}`}><ExternalLink className="h-4 w-4" aria-hidden />进入称重台</WorkbenchLink>:<span className="text-sm text-text-tertiary">当前无称重操作权限</span>}
       {scale.canOperate&&canKick&&device?.connectionId&&<Button className="text-error hover:bg-error-background active:bg-error-background" disabled={busy} onClick={()=>{setError('');setTarget({scale,connectionId:device.connectionId!});}}><LogOut className="h-4 w-4" aria-hidden />踢出 Agent</Button>}
      </div>
     </section>;
    })}{!capabilities.scales.some(s=>s.siteId===site.id&&s.enabled===1)&&<p className="p-card text-sm text-text-tertiary">暂无启用地磅</p>}</div>
   </ContentCard>)}</div>}
  <FormDialog open={!!target} title="踢出 Agent" description={target?`${target.scale.siteName} · ${target.scale.name}`:''} loading={busy} submitText="确认踢出" onCancel={()=>setTarget(null)} onSubmit={kick}>
   <p className="text-sm">收到指令的 Agent 将退出，未确认采样失效。已提交的打印作业可能继续出纸，结果未知时须人工核实。</p>
   {error&&<p role="alert" className="mt-3 text-sm text-error">{error}</p>}
  </FormDialog>
 </>;
}
