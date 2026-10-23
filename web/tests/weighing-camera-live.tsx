import { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { DeviceCameraPanel, type CameraSize } from '@/pages/weighing/camera-panel';
import type { CaptureSnapshot } from '@/types/weighing';
import '@/styles/globals.css';
import '@/pages/weighing/workbench.css';

const capture: CaptureSnapshot = {
  id: 'captured-evidence', preset: 'front', source: 'agent', cameraLabel: '车头视角',
  imageSrc: 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="320" height="180"%3E%3Crect width="320" height="180" fill="gray"/%3E%3C/svg%3E',
  at: new Date().toISOString(), weight: 18.665,
};

export function Fixture() {
  const [mounted, setMounted] = useState(true);
  const [scaleId, setScaleId] = useState(1);
  const [size, setSize] = useState<CameraSize>('standard');
  return <main className="standalone-workbench">
    <nav><button onClick={() => setMounted(value => !value)}>切换挂载</button><button onClick={() => setScaleId(value => value === 1 ? 2 : 1)}>切换地磅</button></nav>
    <div className={`weighing-station weighing-camera-${size}`}>
      {mounted && <DeviceCameraPanel scaleId={scaleId} size={size} onSizeChange={setSize} capture={capture} />}
    </div>
  </main>;
}

createRoot(document.getElementById('root')!).render(<Fixture />);
