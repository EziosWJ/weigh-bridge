import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { WeighingWorkbenchPage } from '@/pages/weighing/workbench';
import { ToastProvider } from '@/components/common/toast';
import { useWeighingStore } from '@/store/weighing-store';
import '@/styles/globals.css';

const data = useWeighingStore.getState().data;
const completed = new URLSearchParams(location.search).get('scenario') === 'completed';
const agent = new URLSearchParams(location.search).get('scenario') === 'agent';
const draft = new URLSearchParams(location.search).get('scenario') === 'draft';
useWeighingStore.setState({ ready: true, capabilities: {
  localSiteCode: 'F001', localScaleCode: 'WB01', configured: true, initialized: true, userId: 1,
  permissionCodes: ['weighing:read', 'weighing:confirm'],
  sites: [{ id: 1, code: 'F001', name: '测试厂', enabled: 1, canOperate: true }],
  scales: [{ code: 'WB01', siteCode: 'F001', name: '测试磅', enabled: 1, siteName: '测试厂', canOperate: true }],
}, data: { ...data, draft: draft ? { ...data.draft, materialId: '1' } : data.draft, owner: 'self', tasks: completed ? [{ ...data.draft, id: '1', plate: '晋A12345', direction: 'inbound', materialName: '测试', partyName: '测试', status: 'completed', createdAt: new Date().toISOString(), records: [], history: [] }] : [], config: { ...data.config, online: true, stable: true, weight: 18.5 } },
device: { agentId: 'fixture', scaleCode: 'WB01', weight: '18.5', stable: true, deviceOk: true, online: true, revision: '1', receivedAt: new Date().toISOString(), plate: { candidate: agent ? '晋A11111' : '', failed: false, at: agent ? new Date().toISOString() : '' }, images: [] } });

window.addEventListener('fixture-agent', () => useWeighingStore.setState(state => ({ device: state.device ? { ...state.device, plate: { candidate: '晋A22222', failed: false, at: new Date(Date.now() + 1000).toISOString() } } : null })));

createRoot(document.getElementById('root')!).render(<ToastProvider><MemoryRouter initialEntries={[`/weighing/workbench?scale=WB01${completed ? '&task=1' : ''}`]}><WeighingWorkbenchPage /></MemoryRouter></ToastProvider>);
