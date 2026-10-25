import { useState } from 'react';
import { WeighingReviewDialog } from '@/pages/weighing/review-dialog';
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
    localSiteId: 1, configured: true, initialized: true,
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
    captureRequired: scenario !== 'optional', capturePolicyRevision: 'policy-1', online: true, revision: '1', receivedAt: new Date().toISOString(),
    plate: { candidate: '', failed: false, at: '' }, images: [],
  },
});

function ReviewFixture() {
 const [confirmed, setConfirmed] = useState(false);
 const required = scenario === 'required-review';
 return <><WeighingReviewDialog title="首磅（毛重）" value={{ plate: '晋A12345', direction: '入厂卸货', material: '高粱', partyLabel: '供货方', party: '测试供货方', weight: 18.665, kind: '毛重', supplementary: [], capture: { id: 'optional-review', source: 'server', imageSrc: '', images: [], at: new Date().toISOString(), weight: 18.665, captureRequired: required, imageStatus: required ? 'capture_failed' : 'disabled' } }} invalidReason={scenario === 'changed-review' ? '强制抓拍配置已变更，请重新采集' : ''} onCancel={() => {}} onMismatch={() => {}} onConfirm={() => setConfirmed(true)} /><output hidden>{confirmed ? 'confirmed' : 'waiting'}</output></>;
}

export function Fixture() {
  const data = useWeighingStore(s => s.data);
  return <MemoryRouter initialEntries={['/weighing/workbench?scaleId=1']}>
    <WeighingWorkbenchPage />
    <output hidden>{JSON.stringify({ sampleReady: data.sampleReady, images: data.capture?.images?.length ?? 0 })}</output>
  </MemoryRouter>;
}

createRoot(document.getElementById('root')!).render(scenario?.endsWith('-review') ? <ReviewFixture /> : <Fixture />);
