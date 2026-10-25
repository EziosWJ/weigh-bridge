import { MemoryRouter } from 'react-router-dom';
import { createRoot } from 'react-dom/client';
import { CamerasPage } from '@/pages/cameras';
import '@/styles/globals.css';

createRoot(document.getElementById('root')!).render(<MemoryRouter><CamerasPage /></MemoryRouter>);
