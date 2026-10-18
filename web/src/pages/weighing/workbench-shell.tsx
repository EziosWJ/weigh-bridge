import { useEffect, useState } from 'react';
import { Maximize, Minimize, Scale } from 'lucide-react';
import { Outlet } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { useAuthStore } from '@/store/auth-store';
import { useWeighingStore } from '@/store/weighing-store';
import { roleLabels } from '@/types/weighing';
import { WORKBENCH_WINDOW } from '@/lib/workbench-window';
import './workbench.css';

export function WorkbenchShell() {
  const user = useAuthStore(s => s.user);
  const resetAt = useWeighingStore(s => s.data.resetAt);
  const config = useWeighingStore(s => s.data.config);
  const [fullscreen, setFullscreen] = useState(!!document.fullscreenElement);
  const [error, setError] = useState('');
  useEffect(() => {
    window.name = WORKBENCH_WINDOW;
    const update = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', update);
    return () => document.removeEventListener('fullscreenchange', update);
  }, []);
  return <div className="standalone-workbench min-h-screen bg-background">
    <header className="workbench-topbar">
      <div className="workbench-brand"><span className="workbench-brand-mark" aria-hidden><Scale className="h-5 w-5" /></span><div><h1 className="text-base font-semibold">称重工作台</h1><p className="text-xs text-text-secondary">现场监看台</p></div></div>
      <span className="text-sm text-warning">测试环境</span>
      <span className="text-sm text-text-secondary">{config.online ? '模拟在线' : '模拟断网'} · {user?.nickname || user?.username} · 演示{roleLabels[config.role]}</span>
      <nav aria-label="工作台操作" className="ml-auto flex flex-wrap items-center gap-3 text-sm">
        <a className="text-primary hover:underline" href="/weighing/tasks" target="weighbridge-management">后台查询</a>
        <a className="text-primary hover:underline" href="/weighing/settings" target="weighbridge-management">演示设置</a>
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
