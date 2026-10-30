import { useEffect, useState } from 'react';
import { FileText, Maximize, Minimize, Monitor } from 'lucide-react';
import { Outlet, useSearchParams } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Dialog, DialogOverlay, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody, DialogFooter } from '@/components/ui/dialog';
import { Textarea } from '@/components/ui/textarea';
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
  const [params, setParams] = useSearchParams();
  const routeScale = params.get('scale') ?? '';
  const scaleCode = capabilities?.localScaleCode || routeScale;
  const scale=capabilities?.scales.find(s=>s.code===scaleCode);
  useEffect(() => {
    if (!routeScale || !capabilities?.localScaleCode || routeScale === capabilities.localScaleCode) return;
    setParams(previous => { const next = new URLSearchParams(previous); next.set('scale', capabilities.localScaleCode); return next; }, { replace: true });
  }, [routeScale, capabilities?.localScaleCode, setParams]);
  const releaseOperation = useWeighingStore(s => s.releaseOperation);
  const dispatch = useWeighingStore(s => s.dispatch);
  const busy = useWeighingStore(s => s.busy);
  const owner = useWeighingStore(s => s.data.owner);
  const pending = useWeighingStore(s => s.data.pending);
  const user = useAuthStore(s => s.user);
  const resetAt = useWeighingStore(s => s.data.resetAt);
  const config = useWeighingStore(s => s.data.config);
  const [fullscreen, setFullscreen] = useState(!!document.fullscreenElement);
  const [ticketsOpen, setTicketsOpen] = useState(false);
  const [releasing, setReleasing] = useState(false);
  const [takeoverOpen, setTakeoverOpen] = useState(false);
  const [takeoverReason, setTakeoverReason] = useState('');
  const canReadTickets = capabilities?.permissionCodes.includes('weighing:read') ?? false;
  useEffect(() => {
    const name=workbenchWindow(scaleCode);
    const previousName=window.name.startsWith(WORKBENCH_WINDOW)?'':window.name;
    window.name=name;
    const update = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', update);
    return () => { document.removeEventListener('fullscreenchange', update); if (window.name === name) window.name = previousName; };
  }, [scaleCode]);
  if(!routeScale)return <WorkbenchEntry />;
  if(ready&&(!scale?.canOperate||!capabilities?.permissionCodes.includes('weighing:confirm')))return <div className="p-6" role="alert">无本厂地磅称重操作权限。<a href="/weighing/tasks" className="text-primary underline">返回后台查询</a></div>;
  if(!ready)return <p role="status" className="p-6">正在核对本厂操作权限…</p>;
  return <div className="weighing-workbench min-h-screen bg-background">
    <a href="#main-content" className="workbench-skip">跳转到称重操作</a>
    <header className="workbench-topbar">
      <div className="workbench-brand"><img className="workbench-brand-mark" src="/weighbridge/brand/scale-mark.svg" alt="" /><div><h1 className="text-xl font-semibold">称重工作台</h1><p className="text-xs text-text-secondary">{scale?.siteName} · {scale?.name} · 人工核对确认</p></div></div>
      <span className="workbench-connection text-sm text-text-secondary"><span className={`connection-dot ${config.online ? 'is-online' : ''}`} aria-hidden />{config.online ? 'Agent 在线' : 'Agent 离线'} · {user?.nickname || user?.username} · {roleLabels[config.role]}</span>
      <nav aria-label="工作台操作" className="ml-auto flex flex-wrap items-center gap-3 text-sm">
        {owner === 'other' && capabilities?.permissionCodes.includes('weighing:takeover') && <Button size="sm" className="workbench-takeover-button" onClick={() => setTakeoverOpen(true)}>接管操作权</Button>}
        {canReadTickets && <button type="button" className="workbench-management-link" aria-haspopup="dialog" aria-expanded={ticketsOpen} title="打开过磅单列表" onClick={() => setTicketsOpen(true)}><FileText className="h-4 w-4" aria-hidden />过磅单</button>}
        <a className="workbench-management-link" href="/weighing/tasks" target="weighbridge-management"><Monitor className="h-4 w-4" aria-hidden />后台查询</a>
        {owner === 'self' && <Button size="sm" disabled={!!pending || releasing} aria-busy={releasing} title={pending ? '提交流程处理中，暂不能释放操作权' : undefined} onClick={() => {
          if (pending || releasing) return;
          setReleasing(true);
          void releaseOperation()
            .then(() => toast.success('已释放称重操作权'))
            .catch(e => toast.error({ title: '释放操作权失败', description: e instanceof Error ? e.message : '请稍后重试' }))
            .finally(() => setReleasing(false));
        }}>{releasing ? '正在释放…' : '释放操作权'}</Button>}
        <Button size="sm" onClick={async () => {
          try { if (document.fullscreenElement) await document.exitFullscreen(); else await document.documentElement.requestFullscreen(); }
          catch { toast.error('浏览器未允许全屏，请使用浏览器全屏功能或重试。'); }
        }}>{fullscreen ? <Minimize className="h-4 w-4" aria-hidden /> : <Maximize className="h-4 w-4" aria-hidden />}{fullscreen ? '退出全屏' : '全屏'}</Button>
      </nav>
    </header>
    <main id="main-content" className="workbench-content p-4"><Outlet key={resetAt || 'initial'} /></main>
    <Dialog open={takeoverOpen} onOpenChange={open => { if (!busy) { setTakeoverOpen(open); if (!open) setTakeoverReason(''); } }} trapFocus restoreFocus lockScroll closeOnEscape={!busy} closeOnOverlayClick={!busy}>
      <DialogOverlay />
      <DialogContent className="max-w-[480px]">
        <DialogHeader><div><DialogTitle>接管操作权</DialogTitle><DialogDescription>当前地磅正由其他会话使用，请填写接管原因以便追溯。</DialogDescription></div></DialogHeader>
        <DialogBody><label htmlFor="takeover-reason" className="mb-2 block text-sm font-medium">接管原因 <span className="text-error">*</span></label><Textarea id="takeover-reason" autoFocus rows={3} placeholder="请输入接管原因" value={takeoverReason} onChange={e => setTakeoverReason(e.target.value)} /></DialogBody>
        <DialogFooter><Button disabled={busy} onClick={() => { setTakeoverOpen(false); setTakeoverReason(''); }}>取消</Button><Button variant="primary" disabled={!takeoverReason.trim() || busy || !!pending} aria-busy={busy} title={pending ? '上次提交结果待核实，请先查询原结果' : undefined} onClick={() => {
          void dispatch({ type: 'takeover', reason: takeoverReason.trim() }).then(() => {
            setTakeoverOpen(false); setTakeoverReason(''); toast.success({ title: '操作员已接管', description: '当前地磅已由本会话接管。' });
          }).catch(e => toast.error({ title: '接管失败', description: e instanceof Error ? e.message : '请稍后重试' }));
        }}>{busy ? '正在接管…' : '确认接管'}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    {ticketsOpen && scale && <TicketDrawer siteName={scale.siteName} onClose={() => setTicketsOpen(false)} />}
  </div>;
}
