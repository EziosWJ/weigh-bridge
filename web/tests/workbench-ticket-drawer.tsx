import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { WorkbenchShell } from '@/pages/weighing/workbench-shell';
import { useAuthStore } from '@/store/auth-store';
import { useWeighingStore } from '@/store/weighing-store';
import type { CurrentUser } from '@/types';
import '@/styles/globals.css';

window.history.replaceState({}, '', '/weighing/workbench?scale=scale');
const initial = useWeighingStore.getState().data;
useWeighingStore.setState({
  ready: true,
  capabilities: {
    localSiteCode: 'site', localScaleCode: 'scale', configured: true, initialized: true, userId: 1,
    permissionCodes: ['weighing:read', 'weighing:confirm'],
    sites: [{ code: 'site', name: '示范厂区', enabled: 1, canOperate: true }],
    scales: [{ code: 'scale', name: '1号地磅', enabled: 1, siteCode: 'site', siteName: '示范厂区', canOperate: true }],
  },
  data: { ...initial, config: { ...initial.config, online: true, role: 'operator' } },
});
useAuthStore.setState({ user: { id: 1, username: 'rzp', nickname: 'rzp', roles: [] } as CurrentUser });

createRoot(document.getElementById('root')!).render(
  <MemoryRouter initialEntries={['/weighing/workbench?scale=scale']}>
    <Routes>
      <Route path="/weighing/workbench" element={<WorkbenchShell />}>
        <Route index element={<main className="p-6 text-text-secondary">工作台内容占位</main>} />
      </Route>
    </Routes>
  </MemoryRouter>,
);
