import { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { DeviceCameraPanel, type CameraSize } from '@/pages/weighing/camera-panel';
import type { CaptureSnapshot } from '@/types/weighing';
import '@/styles/globals.css';
import '@/pages/weighing/workbench.css';

const image = (name: string, color: string) => `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180"><rect width="320" height="180" fill="${color}"/><text x="20" y="90" fill="white" font-size="24">${name}</text></svg>`)}`;

const capture: CaptureSnapshot = {
  id: 'captured-evidence', source: 'server', captureRequired: true, imageStatus: 'complete',
  at: new Date().toISOString(), weight: 18.665, cameraConfigVersion: 4,
  images: [
    { id: 'img-entry', url: image('ENTRY', '#245e86'), source: 'server', cameraId: 11, cameraName: '入口' },
    { id: 'img-yard', url: image('YARD', '#245e36'), source: 'server', cameraId: 12, cameraName: '装车区' },
  ],
};

export function Fixture() {
  const [mounted, setMounted] = useState(true);
  const [scaleId, setScaleId] = useState(1);
  const [size, setSize] = useState<CameraSize>('standard');
  return <main className="weighing-workbench">
    <nav><button onClick={() => setMounted(value => !value)}>切换挂载</button><button onClick={() => setScaleId(value => value === 1 ? 2 : 1)}>切换地磅</button></nav>
    <div className={`weighing-station weighing-camera-${size}`}>
      {mounted && <DeviceCameraPanel scaleId={scaleId} size={size} onSizeChange={setSize} capture={capture} />}
    </div>
  </main>;
}

createRoot(document.getElementById('root')!).render(<Fixture />);
