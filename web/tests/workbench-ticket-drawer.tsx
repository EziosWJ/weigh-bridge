import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { WorkbenchShell } from '@/pages/weighing/workbench-shell';
import { useAuthStore } from '@/store/auth-store';
import { useWeighingStore } from '@/store/weighing-store';
import type { CurrentUser } from '@/types';
import '@/styles/globals.css';

window.history.replaceState({}, '', '/weighing/workbench?scaleId=1');
const initial = useWeighingStore.getState().data;
useWeighingStore.setState({
  ready: true,
  capabilities: {
    localSiteId: 1, configured: true, initialized: true, userId: 1,
    permissionCodes: ['weighing:read', 'weighing:confirm'],
    sites: [{ id: 1, code: 'site', name: '示范厂区', enabled: 1, canOperate: true }],
    scales: [{ id: 1, code: 'scale', name: '1号地磅', enabled: 1, siteId: 1, siteName: '示范厂区', canOperate: true }],
  },
  data: { ...initial, config: { ...initial.config, online: true, role: 'operator' } },
});
useAuthStore.setState({ user: { id: 1, username: 'rzp', nickname: 'rzp', roles: [] } as CurrentUser });

createRoot(document.getElementById('root')!).render(
  <MemoryRouter initialEntries={['/weighing/workbench?scaleId=1']}>
    <Routes>
      <Route path="/weighing/workbench" element={<WorkbenchShell />}>
        <Route index element={<main className="p-6 text-text-secondary">工作台内容占位</main>} />
      </Route>
    </Routes>
  </MemoryRouter>,
);
