import { useEffect, useState } from 'react';
import { nodeApi, type NodeStatus } from '@/api/node';
import { useWeighingStore } from '@/store/weighing-store';
import { beijingTime } from '@/lib/weighing/query';

export function NodeSyncStatus() {
 const role=useWeighingStore(s=>s.capabilities?.nodeRole);
 const [status,setStatus]=useState<NodeStatus|null>(null);
 const [failed,setFailed]=useState(false);
 useEffect(()=>{
  if(!role)return;
  let active=true;
  const refresh=async()=>{try{const value=await nodeApi.status();if(active){setStatus(value);setFailed(false);}}catch{if(active)setFailed(true);}};
  void refresh();const timer=setInterval(()=>void refresh(),10000);
  return()=>{active=false;clearInterval(timer);};
 },[role]);
 if(!role)return null;
 if(role==='standalone')return <aside aria-label="本机服务状态" className="mb-3 border-b border-border px-4 py-2 text-sm text-text-secondary">
  <span className="font-medium">本厂独立运行</span>
  {failed?<span className="ml-3 text-warning">本机状态暂不可读取，请刷新核实</span>:!status?<span className="ml-3">正在读取本机状态…</span>:<span className="ml-3">{status.initialized?'本厂已就绪':'本机管理初始化未完成：请在用户管理中准备至少两名实名称重操作员并授予称重权限'}</span>}
 </aside>;
 return <aside aria-label="中心同步状态" className="mb-3 border-b border-border px-4 py-2 text-sm text-text-secondary">
  <span className="font-medium">{role==='central'?'中心汇总':'本厂连接状态'}</span>
  {failed?<span className="ml-3 text-warning">同步状态读取失败，请刷新重试</span>:!status?<span className="ml-3">正在读取同步状态…</span>:<>
   {role==='site'&&!status.initialized&&<p className="text-warning">首次启用尚未完成，请检查本地初始化配置。</p>}
   {!status.sites.length&&<span className="ml-3">尚未收到厂区同步，信息未更新</span>}
   {status.sites.map(site=><span key={site.siteId} className="ml-3 inline-block">厂区 {site.siteId} · {site.connected?'中心连接正常':'连接未更新'} · 待同步 {site.pending} 条 · 待补图片 {site.pendingImages} 张 · 最近同步：{site.lastSuccess?beijingTime(site.lastSuccess):'尚未同步'}{site.lastError&&` · ${site.lastError}`}</span>)}
  </>}
 </aside>;
}
