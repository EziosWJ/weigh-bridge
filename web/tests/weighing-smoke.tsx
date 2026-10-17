import { WeighingOriginalsPage } from "@/pages/weighing/originals";
import { createRoot } from 'react-dom/client';
import { BrowserRouter, Link, Route, Routes } from 'react-router-dom';
import { ToastProvider } from '@/components/common/toast';
import { WeighingWorkbenchPage } from '@/pages/weighing/workbench';
import { WeighingTaskDetailPage, WeighingTasksPage } from '@/pages/weighing/tasks';
import { WeighingTicketDetailPage, WeighingTicketPreviewPage, WeighingTicketsPage } from '@/pages/weighing/tickets';
import { WeighingCategoriesPage, WeighingMaterialEditPage, WeighingMaterialsPage, WeighingPartyEditPage } from '@/pages/weighing/materials';
import { DemoSettingsPage } from '@/pages/weighing/demo-panel';
import '@/styles/globals.css';

// 真实业务页面、路由和浏览器存储；小导航只替代需真实账号的 AppShell。
createRoot(document.getElementById('root')!).render(
  <ToastProvider>
    <BrowserRouter>
      <header className="sticky top-0 z-20 h-16 border-b border-border bg-surface px-6 py-5 text-sm">称重作业页面验证</header>
      <nav data-fixture-sidebar aria-label="页面冒烟导航" className="flex flex-wrap gap-4 border-b border-border bg-surface px-6 py-3 text-sm text-primary">
        {[['workbench', '称重工作台'], ['tasks', '运输任务'], ['originals', '原始称重'], ['tickets', '最终单据'], ['materials', '物料资料'], ['parties', '往来主体'], ['settings', '演示设置']].map(([path, name]) => <Link key={path} to={`/weighing/${path}`}>{name}</Link>)}
      </nav>
      <main data-fixture-main className="p-6">
        <Routes>
          <Route path="/weighing/workbench" element={<WeighingWorkbenchPage />} />
          <Route path="/weighing/tasks" element={<WeighingTasksPage />} />
          <Route path="/weighing/tasks/:id" element={<WeighingTaskDetailPage />} />
          <Route path="/weighing/originals" element={<WeighingOriginalsPage />} />
          <Route path="/weighing/tickets" element={<WeighingTicketsPage />} />
          <Route path="/weighing/tickets/:id" element={<WeighingTicketDetailPage />} />
          <Route path="/weighing/tickets/:id/preview" element={<WeighingTicketPreviewPage />} />
          <Route path="/weighing/materials" element={<WeighingMaterialsPage />} />
          <Route path="/weighing/materials/new" element={<WeighingMaterialEditPage />} />
          <Route path="/weighing/materials/:id/edit" element={<WeighingMaterialEditPage />} />
          <Route path="/weighing/parties" element={<WeighingMaterialsPage parties />} />
          <Route path="/weighing/parties/new" element={<WeighingPartyEditPage />} />
          <Route path="/weighing/parties/:id/edit" element={<WeighingPartyEditPage />} />
          <Route path="/weighing/categories" element={<WeighingCategoriesPage />} />
          <Route path="/weighing/settings" element={<DemoSettingsPage />} />
          <Route path="*" element={<WeighingWorkbenchPage />} />
        </Routes>
      </main>
    </BrowserRouter>
  </ToastProvider>,
);
