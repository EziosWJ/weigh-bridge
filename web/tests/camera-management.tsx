import { MemoryRouter } from 'react-router-dom';
import { createRoot } from 'react-dom/client';
import { CamerasPage } from '@/pages/cameras';
import { ToastProvider } from '@/components/common/toast';
import '@/styles/globals.css';

createRoot(document.getElementById('root')!).render(<ToastProvider><MemoryRouter><CamerasPage /></MemoryRouter></ToastProvider>);
