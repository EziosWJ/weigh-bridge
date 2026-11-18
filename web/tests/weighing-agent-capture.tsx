import { useState } from 'react';
import { WeighingReviewDialog } from '@/pages/weighing/review-dialog';
import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { ToastProvider } from '@/components/common/toast';
import { WeighingWorkbenchPage } from '@/pages/weighing/workbench';
import { useWeighingStore } from '@/store/weighing-store';
import '@/styles/globals.css';

const scenario = new URLSearchParams(location.search).get('scenario');
const initial = useWeighingStore.getState().data;
const plate = '晋A12345';
const temporary = scenario?.startsWith('temporary');
const grain = scenario?.startsWith('temporary-grain');
const hasMaterial = grain || scenario === 'temporary-captured';
const boundTrip = grain ? {
  tripNo: 'LT-001',
  version: 1,
  candidate: {
    tripId: 1, tripNo: 'LT-001', version: 1, stage: 'pending', plate,
    partyCode: 'C-P-0001', partyName: '测试供货方', materialCode: 'C-M-0001', materialName: '高粱',
    inspectionMethod: 'visual', registeredAt: new Date().toISOString(), linkable: true, blockers: [], deductions: [],
  },
} : undefined;
useWeighingStore.setState({
  ready: true,
  capabilities: {
    localSiteCode: 'F001', localScaleCode: 'WB01', configured: true, initialized: true,
    userId: 1,
    masterSyncBound: temporary && scenario !== 'temporary-unbound',
    permissionCodes: scenario === 'forbidden'
      ? ['weighing:read']
      : ['weighing:read', 'weighing:confirm', ...(temporary && scenario !== 'temporary-denied' ? ['weighing:temporary-party:write'] : [])],
    sites: [{ code: 'F001', name: '测试厂区', enabled: 1, canOperate: true }],
    scales: [{ code: 'WB01', siteCode: 'F001', name: '测试地磅', enabled: 1, siteName: '测试厂区', canOperate: true }],
  },
  data: {
    ...initial,
    draft: { ...initial.draft, plate, materialId: hasMaterial ? '11' : '', partyId: grain ? '21' : '' },
    materials: hasMaterial ? [{ id: '11', code: 'C-M-0001', name: '高粱', spec: '', enabled: true, version: 1 }] : [],
    parties: grain ? [{ id: '21', code: 'C-P-0001', name: '测试供货方', nature: 'enterprise', roles: ['supplier'], enabled: scenario !== 'temporary-grain-disabled', version: 1, contacts: [] }] : [],
    trip: boundTrip,
    draftPlateEvidence: { source: 'manual', confirmedPlate: plate, at: new Date().toISOString() },
    owner: scenario === 'occupied' ? 'other' : null,
    config: {
      ...initial.config, weight: scenario === 'zero' ? 0 : 18.665,
      online: scenario !== 'offline', stable: scenario !== 'dynamic',
      deviceError: scenario === 'fault', timedOut: scenario === 'offline', evidence: false,
    },
  },
  device: {
    agentId: 'real-agent', scaleCode: 'WB01', weight: '18.665', stable: true, deviceOk: true,
    captureRequired: scenario !== 'optional', capturePolicyRevision: 'policy-1', online: true, revision: '1', receivedAt: new Date().toISOString(),
    plate: { candidate: '', failed: false, at: '' }, images: [],
  },
});

function ReviewFixture() {
 const [confirmed, setConfirmed] = useState(false);
 const required = scenario === 'required-review';
 return <><WeighingReviewDialog title="首磅（毛重）" value={{ plate: '晋A12345', direction: '入厂卸货', material: '高粱', partyLabel: '供货方', party: '测试供货方', weight: 18.665, kind: '毛重', supplementary: [], capture: { id: 'optional-review', source: 'server', imageSrc: '', images: [], at: new Date().toISOString(), weight: 18.665, captureRequired: required, imageStatus: required ? 'capture_failed' : 'disabled' } }} invalidReason={scenario === 'changed-business-review' ? '业务资料已变更，请返回重新核对' : ''} onCancel={() => {}} onMismatch={() => {}} onConfirm={() => setConfirmed(true)} /><output hidden>{confirmed ? 'confirmed' : 'waiting'}</output></>;
}

export function Fixture() {
  const data = useWeighingStore(s => s.data);
  return <MemoryRouter initialEntries={['/weighing/workbench?scale=WB01']}>
    <WeighingWorkbenchPage />
    <output hidden>{JSON.stringify({ sampleReady: data.sampleReady, images: data.capture?.images?.length ?? 0 })}</output>
  </MemoryRouter>;
}

createRoot(document.getElementById('root')!).render(<ToastProvider>{scenario?.endsWith('-review') ? <ReviewFixture /> : <Fixture />}</ToastProvider>);
