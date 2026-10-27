import { useEffect, useState } from 'react';
import { FileText, Maximize, Minimize, Monitor } from 'lucide-react';
import { Outlet } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/common/toast-store';
import { useAuthStore } from '@/store/auth-store';
import { useWeighingStore } from '@/store/weighing-store';
import { roleLabels } from '@/types/weighing';
import { WORKBENCH_WINDOW,workbenchWindow } from '@/lib/workbench-window';
import './workbench.css';
import { WorkbenchEntry } from './workbench-entry';
import { TicketDrawer } from './ticket-drawer';

export function WorkbenchShell() {
  const capabilities=useWeighingStore(s=>s.capabilities);
  const ready=useWeighingStore(s=>s.ready);
  const scaleId=Number(new URLSearchParams(location.search).get('scaleId'));
  const scale=capabilities?.scales.find(s=>s.id===scaleId);
  const dispatch = useWeighingStore(s => s.dispatch);
  const owner = useWeighingStore(s => s.data.owner);
  const pending = useWeighingStore(s => s.data.pending);
  const user = useAuthStore(s => s.user);
  const resetAt = useWeighingStore(s => s.data.resetAt);
  const config = useWeighingStore(s => s.data.config);
  const [fullscreen, setFullscreen] = useState(!!document.fullscreenElement);
  const [ticketsOpen, setTicketsOpen] = useState(false);
  const canReadTickets = capabilities?.permissionCodes.includes('weighing:read') ?? false;
  useEffect(() => {
    const name=workbenchWindow(scaleId);
    const previousName=window.name.startsWith(WORKBENCH_WINDOW)?'':window.name;
    window.name=name;
    const update = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', update);
    return () => { document.removeEventListener('fullscreenchange', update); if (window.name === name) window.name = previousName; };
  }, [scaleId]);
  if(!scaleId)return <WorkbenchEntry />;
  if(ready&&(!scale?.canOperate||!capabilities?.permissionCodes.includes('weighing:confirm')))return <div className="p-6" role="alert">无本厂地磅称重操作权限。<a href="/weighing/tasks" className="text-primary underline">返回后台查询</a></div>;
  if(!ready)return <p role="status" className="p-6">正在核对本厂操作权限…</p>;
  return <div className="weighing-workbench min-h-screen bg-background">
    <a href="#main-content" className="workbench-skip">跳转到称重操作</a>
    <header className="workbench-topbar">
      <div className="workbench-brand"><img className="workbench-brand-mark" src="/weighbridge/brand/scale-mark.svg" alt="" /><div><h1 className="text-xl font-semibold">称重工作台</h1><p className="text-xs text-text-secondary">{scale?.siteName} · {scale?.name} · 人工核对确认</p></div></div>
      <span className="workbench-connection text-sm text-text-secondary"><span className={`connection-dot ${config.online ? 'is-online' : ''}`} aria-hidden />{config.online ? 'Agent 在线' : 'Agent 离线'} · {user?.nickname || user?.username} · {roleLabels[config.role]}</span>
      <nav aria-label="工作台操作" className="ml-auto flex flex-wrap items-center gap-3 text-sm">
        {canReadTickets && <button type="button" className="workbench-management-link" aria-haspopup="dialog" aria-expanded={ticketsOpen} title="打开过磅单列表" onClick={() => setTicketsOpen(true)}><FileText className="h-4 w-4" aria-hidden />过磅单</button>}
        <a className="workbench-management-link" href="/weighing/tasks" target="weighbridge-management"><Monitor className="h-4 w-4" aria-hidden />后台查询</a>
        {owner === 'self' && !pending && <Button size="sm" onClick={() => void dispatch({type:'occupy',owner:null}).then(() => toast.success('已释放称重操作权')).catch(e => toast.error({ title: '释放操作权失败', description: e instanceof Error ? e.message : '请稍后重试' }))}>释放操作权</Button>}
        <Button size="sm" onClick={async () => {
          try { if (document.fullscreenElement) await document.exitFullscreen(); else await document.documentElement.requestFullscreen(); }
          catch { toast.error('浏览器未允许全屏，请使用浏览器全屏功能或重试。'); }
        }}>{fullscreen ? <Minimize className="h-4 w-4" aria-hidden /> : <Maximize className="h-4 w-4" aria-hidden />}{fullscreen ? '退出全屏' : '全屏'}</Button>
      </nav>
    </header>
    <main id="main-content" className="workbench-content p-4"><Outlet key={resetAt || 'initial'} /></main>
    {ticketsOpen && scale && <TicketDrawer siteId={scale.siteId} siteName={scale.siteName} onClose={() => setTicketsOpen(false)} />}
  </div>;
}
