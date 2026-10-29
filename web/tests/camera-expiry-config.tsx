import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { SystemConfigsPage } from '@/pages/system/configs';
import '@/styles/globals.css';
createRoot(document.getElementById('root')!).render(<MemoryRouter><SystemConfigsPage /></MemoryRouter>);
