import { useEffect, useState } from 'react';
import { weighingApi } from '@/api/weighing';
import { Button } from '@/components/ui/button';
import { localSiteCode } from '@/lib/local-site';

// Resolve the local scale before loading the independent workbench document.
// Its device session and UI draft are scoped to the scale code at startup.
export function WorkbenchEntry() {
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let active = true;
    void weighingApi.capabilities().then(capabilities => {
      if (!active) return;
      const siteCode = localSiteCode(capabilities);
      const site = capabilities.sites.find(item => item.code === siteCode);
      const scales = capabilities.scales.filter(item => item.siteCode === siteCode && item.enabled === 1);
      if (!capabilities.permissionCodes.includes('weighing:confirm') || site?.enabled !== 1 || !site.canOperate || scales.length !== 1 || !scales[0].canOperate) {
        setError('本厂地磅未就绪或当前账号无操作权限，请联系管理员检查。');
        return;
      }
      const destination = new URL(window.location.href);
      destination.searchParams.set('scale', scales[0].code);
      window.location.replace(destination.href);
    }).catch(error => {
      if (active) setError(error instanceof Error ? error.message : '本厂操作权限读取失败，请重试。');
    });
    return () => { active = false; };
  }, [attempt]);
  return <main className="p-6 text-sm">
    {error ? <><p role="alert">{error}</p><Button className="mt-4" onClick={() => { setError(''); setAttempt(value => value + 1); }}>重试</Button><a href="/weighing/tasks" className="ml-4 text-primary underline">返回后台查询</a></> : <p role="status">正在核对本厂操作权限…</p>}
  </main>;
}
