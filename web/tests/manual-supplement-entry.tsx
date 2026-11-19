import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { WorkbenchShell } from '@/pages/weighing/workbench-shell';
import { useWeighingStore } from '@/store/weighing-store';
import '@/styles/globals.css';

const params = new URLSearchParams(window.location.search);
const authorized = params.get('permission') === 'yes';
const canOperate = params.get('operate') !== 'no';
window.history.replaceState({}, '', '/weighing/workbench?scale=scale');
const initial = useWeighingStore.getState().data;
useWeighingStore.setState({
  ready: true,
  capabilities: {
    localSiteCode: 'site', localScaleCode: 'scale', configured: true, initialized: true, userId: 1,
    permissionCodes: authorized ? ['weighing:read', 'weighing:manual-supplement'] : ['weighing:read', 'weighing:confirm'],
    sites: [{ code: 'site', name: '示范厂区', enabled: 1, canOperate }],
    scales: [{ code: 'scale', name: '1号地磅', enabled: 1, siteCode: 'site', siteName: '示范厂区', canOperate }],
  },
  data: { ...initial, config: { ...initial.config, online: true, role: 'operator' } },
});

createRoot(document.getElementById('root')!).render(
  <MemoryRouter initialEntries={['/weighing/workbench?scale=scale']}>
    <Routes>
      <Route path="/weighing/workbench" element={<WorkbenchShell />}>
        <Route index element={<main className="p-6 text-text-secondary">工作台内容占位</main>} />
      </Route>
    </Routes>
  </MemoryRouter>,
);
