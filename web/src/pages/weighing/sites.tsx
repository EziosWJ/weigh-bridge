import { Link } from 'react-router-dom';
import { useCallback, useEffect, useState } from 'react';
import { ExternalLink, LogOut, RefreshCw } from 'lucide-react';
import { weighingApi, type WeighingCapabilities, type WeighingScale } from '@/api/weighing';
import { workflowApi } from '@/api/weighing-workflow';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { EmptyState } from '@/components/common/empty-state';
import { PageHeader } from '@/components/common/page-header';
import { StatusTag } from '@/components/common/status-tag';
import { WorkbenchLink } from '@/components/common/workbench-link';
import { FormDialog } from '@/components/common/form-dialog';
import { Button } from '@/components/ui/button';
import { buttonStyles } from '@/components/ui/button-styles';
import { getErrorMessage } from '@/lib/api-error';
import { localSiteId, localSiteSetupMessage } from '@/lib/local-site';
import type { DeviceState } from '@/types/weighing';

export function WeighingSitesPage() {
 const [capabilities,setCapabilities]=useState<WeighingCapabilities|null>(null);
 const [device,setDevice]=useState<DeviceState|null>(null);
 const [loading,setLoading]=useState(true);
 const [error,setError]=useState('');
 const [notice,setNotice]=useState('');
 const [target,setTarget]=useState<{scale:WeighingScale;connectionId:string}|null>(null);
 const [busy,setBusy]=useState(false);
 const refresh=useCallback(async()=>{
  try {
   const caps=await weighingApi.capabilities();
   setCapabilities(caps);
   const id=localSiteId(caps);
   const scales=caps.scales.filter(s=>s.siteId===id&&s.enabled===1);
   setDevice(scales.length===1?await workflowApi.get<DeviceState>(`/scales/${scales[0].id}/device`):null);
   setError('');
  } catch(e){setDevice(null);setError(getErrorMessage(e,'本厂状态加载失败'));}
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
 const siteId=capabilities?localSiteId(capabilities):undefined;
 const site=capabilities?.sites.find(s=>s.id===siteId);
 const scales=capabilities?.scales.filter(s=>s.siteId===siteId&&s.enabled===1)??[];
 const scale=scales.length===1?scales[0]:undefined;
 const canOperate=site?.enabled===1&&site.canOperate&&scale?.canOperate;
 const canConfirm=capabilities?.permissionCodes.includes('weighing:confirm')??false;
 const canKick=capabilities?.permissionCodes.includes('weighing:agent:kick')??false;
 return <>
  <PageHeader title="本厂称重" actions={<><Button onClick={()=>void refresh()} disabled={busy}><RefreshCw className="h-4 w-4" aria-hidden />刷新状态</Button>{!siteId&&capabilities?.permissionCodes.some(code=>['weighing:site:manage','weighing:site:export'].includes(code))&&<Link className={buttonStyles({variant:'primary'})} to="/system/config">配置本厂</Link>}</>}/>
  {error&&<p role="alert" className="mb-4 text-sm text-error">{error}</p>}
  {notice&&<p role="status" className="mb-4 text-sm text-text-secondary">{notice}</p>}
  {loading?<p role="status">正在加载本厂状态…</p>:!capabilities?null:!siteId?<EmptyState title="尚未完成本厂配置" description={localSiteSetupMessage}/>:!site?<EmptyState title="暂无本厂业务权限" description="请联系管理员分配本厂查询或操作权限。"/>:
   <ContentCard title={site.name} className="max-w-5xl shadow-none" extra={<StatusTag tone={site.enabled===1&&site.canOperate?'success':'neutral'}>{site.enabled===0?'已停用 · 历史可查':site.canOperate?'可操作':'仅查询'}</StatusTag>}>
    {scale?<>
     <div className="mb-space-4 flex flex-wrap items-center justify-between gap-space-3 border-b border-border pb-space-4">
      <h3 className="font-medium">{scale.name}</h3>
      <StatusTag tone={!device?'neutral':device.online?'success':'warning'}>{!device?'状态未知':device.online?'Agent 在线':'Agent 离线'}</StatusTag>
     </div>
     <div className="grid gap-space-4 sm:grid-cols-2">
      <DetailItem label="现场 Agent" value={device?.agentId||'尚未接入 Agent'} className="[&>div]:break-all"/>
      <DetailItem label="采集设备" value={!device?'状态未知':!device.online?'等待 Agent 上线':device.deviceOk?'已就绪':'未就绪'}/>
     </div>
     <div className="mt-space-6 flex flex-wrap items-center gap-space-2">
      {canOperate&&canConfirm?<WorkbenchLink className={buttonStyles({variant:'primary'})} to={`/weighing/workbench?scaleId=${scale.id}`}><ExternalLink className="h-4 w-4" aria-hidden />进入称重台</WorkbenchLink>:<span className="text-sm text-text-tertiary">{site.enabled===0?'本厂已停用，历史业务仍可查询':'当前无称重操作权限'}</span>}
      {canOperate&&canKick&&device?.connectionId&&<Button className="text-error hover:bg-error-background active:bg-error-background" disabled={busy} onClick={()=>{setError('');setTarget({scale,connectionId:device.connectionId!});}}><LogOut className="h-4 w-4" aria-hidden />踢出 Agent</Button>}
     </div>
    </>:<p className="text-sm text-text-secondary">{scales.length?'本厂存在多台启用地磅，请联系管理员核对现场配置。':'本厂暂无启用地磅，请联系管理员检查本厂配置。'}</p>}
   </ContentCard>}
  <FormDialog open={!!target} title="踢出 Agent" description={target?`${target.scale.siteName} · ${target.scale.name}`:''} loading={busy} submitText="确认踢出" onCancel={()=>setTarget(null)} onSubmit={kick}>
   <p className="text-sm">收到指令的 Agent 将退出，未确认采样失效。已提交的打印作业可能继续出纸，结果未知时须人工核实。</p>
   {error&&<p role="alert" className="mt-3 text-sm text-error">{error}</p>}
  </FormDialog>
 </>;
}
