import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { WeighingWorkbenchPage } from '@/pages/weighing/workbench';
import { useWeighingStore } from '@/store/weighing-store';
import '@/styles/globals.css';

const scenario = new URLSearchParams(location.search).get('scenario');
const initial = useWeighingStore.getState().data;
const plate = '晋A12345';
useWeighingStore.setState({
  ready: true,
  capabilities: {
    userId: 1,
    permissionCodes: scenario === 'forbidden' ? ['weighing:read'] : ['weighing:read', 'weighing:confirm'],
    sites: [{ id: 1, code: 'site', name: '测试厂区', enabled: 1, canOperate: true }],
    scales: [{ id: 1, code: 'scale', name: '测试地磅', enabled: 1, siteId: 1, siteName: '测试厂区', canOperate: true }],
  },
  data: {
    ...initial,
    draft: { ...initial.draft, plate },
    draftPlateEvidence: { source: 'manual', confirmedPlate: plate, at: new Date().toISOString() },
    owner: scenario === 'occupied' ? 'other' : null,
    config: {
      ...initial.config, weight: scenario === 'zero' ? 0 : 18.665,
      online: scenario !== 'offline', stable: scenario !== 'dynamic',
      deviceError: scenario === 'fault', timedOut: scenario === 'offline', evidence: false,
    },
  },
  device: {
    agentId: 'real-agent', scaleId: 1, weight: '18.665', stable: true, deviceOk: true,
    online: true, revision: '1', receivedAt: new Date().toISOString(),
    plate: { candidate: '', failed: false, at: '' }, images: [],
  },
});

export function Fixture() {
  const data = useWeighingStore(s => s.data);
  return <MemoryRouter initialEntries={['/weighing/workbench?scaleId=1']}>
    <WeighingWorkbenchPage />
    <output hidden>{JSON.stringify({ sampleReady: data.sampleReady, images: data.capture?.images?.length ?? 0 })}</output>
  </MemoryRouter>;
}

createRoot(document.getElementById('root')!).render(<Fixture />);
