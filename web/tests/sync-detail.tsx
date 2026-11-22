import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { SystemSyncLogsPage } from '@/pages/system/logs/sync-logs';
import { CloudConnectionPage } from '@/pages/weighing/sync/connection';
import { SyncTicketDetailPage } from '@/pages/weighing/sync/detail';
import { SyncMasterPage } from '@/pages/weighing/sync/master';
import { SyncTicketsPage } from '@/pages/weighing/sync/tickets';
import '@/styles/globals.css';

const initialPage = new URLSearchParams(location.search).get('page') ?? 'tickets/42';

createRoot(document.getElementById('root')!).render(
  <MemoryRouter initialEntries={[`/weighing/sync/${initialPage}`]}>
    <Routes>
      <Route path="/weighing/sync/tickets" element={<SyncTicketsPage />} />
      <Route path="/weighing/sync/tickets/:id" element={<SyncTicketDetailPage />} />
      <Route path="/weighing/sync/master" element={<SyncMasterPage />} />
      <Route path="/weighing/sync/connection" element={<CloudConnectionPage />} />
      <Route path="/weighing/sync/logs" element={<SystemSyncLogsPage />} />
    </Routes>
  </MemoryRouter>,
);
