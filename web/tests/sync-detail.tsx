import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { SyncTicketDetailPage } from '@/pages/weighing/sync/detail';
import '@/styles/globals.css';

createRoot(document.getElementById('root')!).render(
  <MemoryRouter initialEntries={['/weighing/sync/tickets/42']}>
    <Routes><Route path="/weighing/sync/tickets/:id" element={<SyncTicketDetailPage />} /></Routes>
  </MemoryRouter>,
);
