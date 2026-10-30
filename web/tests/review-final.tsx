import { createRoot } from 'react-dom/client';
import { WeighingReviewDialog } from '../src/pages/weighing/review-dialog';
import '../src/styles/globals.css';
import '../src/pages/weighing/workbench.css';

const image = 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22320%22 height=%22180%22%3E%3Crect width=%22320%22 height=%22180%22 fill=%22%23245e86%22/%3E%3C/svg%3E';
createRoot(document.getElementById('root')!).render(<div className="weighing-workbench"><WeighingReviewDialog
  title="回磅（皮重）"
  value={{ plate: '晋AB6P44', direction: '入厂卸货', kind: '皮重', weight: 12.66, material: '木片 · C-M-0002', partyLabel: '供货方', party: '王坚 · C-P-0001', firstWeight: '毛重 30 吨 · 2026-10-30 09:36:04', original: { gross: 30, tare: 12.66, net: 17.34 }, expected: { gross: 30, tare: 12.66, net: 17.34 }, capture: { id: 'test', at: '2026-10-30T00:00:00Z', weight: 12.66, captureRequired: true, imageStatus: 'complete', images: ['入口', '磅台', '出口'].map((name, index) => ({ id: String(index), cameraName: name, url: image })) }, supplementary: [{ label: '本趟联系人', value: '13007028190' }] }}
  onCancel={() => {}} onMismatch={() => {}} onRecapture={() => {}} onConfirm={() => {}}
/></div>);
