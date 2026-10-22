import React from 'react';
import ReactDOM from 'react-dom/client';
import { createHashRouter, Navigate, RouterProvider } from 'react-router-dom';
import { AppProviders } from './app/AppProviders';
import { AppLayout } from './layouts/AppLayout';
import { OverviewPage } from './pages/OverviewPage';
import { DevicesPage } from './pages/DevicesPage';
import { PrintingPage } from './pages/PrintingPage';
import { ConfigPage } from './pages/ConfigPage';
import { RecordsPage } from './pages/RecordsPage';
import { Alert } from 'antd';
import './styles/global.css';
import './styles/maintenance.css';

const router = createHashRouter([{
  element: <AppLayout />,
  errorElement: <Alert type="error" title="页面加载失败，请刷新页面重试。" />,
  children: [
    { path: '/overview', element: <OverviewPage /> },
    { path: '/printing', element: <PrintingPage /> },
    { path: '/devices', element: <DevicesPage /> },
    { path: '/config', element: <ConfigPage /> },
    { path: '/records', element: <RecordsPage /> },
    { path: '*', element: <Navigate to="/overview" replace /> },
  ],
}]);

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode><AppProviders><RouterProvider router={router} /></AppProviders></React.StrictMode>,
);
