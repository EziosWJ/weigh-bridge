import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { DashboardPage } from '@/pages/dashboard';
import '@/styles/globals.css';

createRoot(document.getElementById('root')!).render(<MemoryRouter><main className="p-4 md:p-6"><DashboardPage /></main></MemoryRouter>);
