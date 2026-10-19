import { useEffect, useState } from 'react';
import { Maximize, Minimize, Monitor } from 'lucide-react';
import { Outlet } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { useAuthStore } from '@/store/auth-store';
import { useWeighingStore } from '@/store/weighing-store';
import { roleLabels } from '@/types/weighing';
import { WORKBENCH_WINDOW } from '@/lib/workbench-window';
import './workbench.css';

export function WorkbenchShell() {
  const dispatch = useWeighingStore(s => s.dispatch);
  const owner = useWeighingStore(s => s.data.owner);
  const pending = useWeighingStore(s => s.data.pending);
  const user = useAuthStore(s => s.user);
  const resetAt = useWeighingStore(s => s.data.resetAt);
  const config = useWeighingStore(s => s.data.config);
  const [fullscreen, setFullscreen] = useState(!!document.fullscreenElement);
  const [error, setError] = useState('');
  useEffect(() => {
    const previousName = window.name === WORKBENCH_WINDOW ? '' : window.name;
    window.name = WORKBENCH_WINDOW;
    const update = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', update);
    return () => { document.removeEventListener('fullscreenchange', update); if (window.name === WORKBENCH_WINDOW) window.name = previousName; };
  }, []);
  return <div className="standalone-workbench min-h-screen bg-background">
    <a href="#main-content" className="workbench-skip">跳转到称重操作</a>
    <header className="workbench-topbar">
      <div className="workbench-brand"><img className="workbench-brand-mark" src="/weighbridge/brand/scale-mark.svg" alt="" /><div><h1 className="text-xl font-semibold">称重工作台</h1><p className="text-xs text-text-secondary">地磅现场作业 · 人工核对确认</p></div></div>
      <span className="workbench-connection text-sm text-text-secondary"><span className={`connection-dot ${config.online ? 'is-online' : ''}`} aria-hidden />{config.online ? 'Agent 在线' : 'Agent 离线'} · {user?.nickname || user?.username} · {roleLabels[config.role]}</span>
      <nav aria-label="工作台操作" className="ml-auto flex flex-wrap items-center gap-3 text-sm">
        <a className="workbench-management-link" href="/weighing/tasks" target="weighbridge-management"><Monitor className="h-4 w-4" aria-hidden />后台查询</a>
        {owner === 'self' && !pending && <Button size="sm" onClick={() => void dispatch({type:'occupy',owner:null}).catch(e => setError(e instanceof Error ? e.message : '释放失败'))}>释放操作权</Button>}
        <Button size="sm" onClick={async () => {
          try { if (document.fullscreenElement) await document.exitFullscreen(); else await document.documentElement.requestFullscreen(); setError(''); }
          catch { setError('浏览器未允许全屏，请使用浏览器全屏功能或重试。'); }
        }}>{fullscreen ? <Minimize className="h-4 w-4" aria-hidden /> : <Maximize className="h-4 w-4" aria-hidden />}{fullscreen ? '退出全屏' : '全屏'}</Button>
      </nav>
    </header>
    {error && <p role="status" className="px-4 text-sm text-warning">{error}</p>}
    <main id="main-content" className="workbench-content p-4"><Outlet key={resetAt || 'initial'} /></main>
  </div>;
}
