import { createRoot } from 'react-dom/client';
import { CameraEventDialog } from '@/pages/cameras/camera-events';
import { setAuthTokenGetter } from '@/lib/http';
import type { CameraRecord } from '@/types/camera';
setAuthTokenGetter(() => 'test-token');
createRoot(document.getElementById('root')!).render(<CameraEventDialog camera={{ id: 1, name: '入口' } as CameraRecord} canManage onClose={() => {}} />);
