import { createBrowserRouter, Navigate } from "react-router-dom";
import { RequireAuth } from "@/components/auth/require-auth";
import { WorkbenchShell } from "@/pages/weighing/workbench-shell";
import { AppShell } from "@/components/layout/app-shell";
import { LoginPage } from "@/pages/login";
import { NotFoundPage } from "@/pages/not-found";
import { RequireMessageAdmin } from "@/components/auth/require-message-admin";

export const router = createBrowserRouter([
  { path: "/weighing/design-demo", hydrateFallbackElement: <main role="status" className="p-4 text-sm">正在加载设计预览…</main>,
    lazy: () => import("@/pages/weighing/design-demo").then(module => ({ Component: module.WorkbenchDesignDemoPage })),
  },
  { path: "/weighing/workbench", hydrateFallbackElement: <main role="status" className="p-4 text-sm text-text-tertiary">正在加载工作台…</main>, element: <RequireAuth><WorkbenchShell /></RequireAuth>,
    children: [{ index: true, lazy: () => import("@/pages/weighing/workbench").then(module => ({ Component: module.WeighingWorkbenchPage })) }],
  },
  {
    path: "/login",
    element: <LoginPage />,
  },
  {
    path: "/",
    hydrateFallbackElement: (
      <main className="flex min-h-screen items-center justify-center bg-background text-sm text-text-tertiary" role="status">
        正在加载页面…
      </main>
    ),
    element: (
      <RequireAuth>
        <AppShell />
      </RequireAuth>
    ),
    children: [
      { path: "weighing", element: <Navigate to="/weighing/sites" replace /> },
      { path: "weighing/temporary-parties", lazy: () => import("@/pages/weighing/temporary-parties").then(module => ({Component:module.TemporaryPartiesPage})) },
      { path: "weighing/site-config", lazy: () => import("@/pages/weighing/site-config").then(module => ({Component:module.SiteConfigurationPage})) },
      { path: "weighing/sites", lazy: () => import("@/pages/weighing/sites").then(module => ({Component:module.WeighingSitesPage})) },
      { path: "weighing/tasks", lazy: () => import("@/pages/weighing/tasks").then((module) => ({ Component: module.WeighingTasksPage })) },
      { path: "weighing/tasks/:id", lazy: () => import("@/pages/weighing/tasks").then((module) => ({ Component: module.WeighingTaskDetailPage })) },
      { path: "weighing/originals", lazy: () => import("@/pages/weighing/originals").then((module) => ({ Component: module.WeighingOriginalsPage })) },
      { path: "weighing/tickets", lazy: () => import("@/pages/weighing/tickets").then((module) => ({ Component: module.WeighingTicketsPage })) },
      { path: "weighing/tickets/:id", lazy: () => import("@/pages/weighing/tickets").then((module) => ({ Component: module.WeighingTicketDetailPage })) },
      { path: "weighing/tickets/:id/preview", lazy: () => import("@/pages/weighing/tickets").then((module) => ({ Component: module.WeighingTicketPreviewPage })) },
      { path: "weighing/materials", lazy: async () => {
        const { WeighingMaterialsPage } = await import("@/pages/weighing/materials");
        return { element: <WeighingMaterialsPage key="materials" /> };
      } },
      { path: "weighing/materials/new", lazy: () => import("@/pages/weighing/materials").then((module) => ({ Component: module.WeighingMaterialEditPage })) },
      { path: "weighing/materials/:id/edit", lazy: () => import("@/pages/weighing/materials").then((module) => ({ Component: module.WeighingMaterialEditPage })) },
      { path: "weighing/parties", lazy: async () => {
        const { WeighingMaterialsPage } = await import("@/pages/weighing/materials");
        return { element: <WeighingMaterialsPage key="parties" parties /> };
      } },
      { path: "weighing/parties/new", lazy: () => import("@/pages/weighing/materials").then((module) => ({ Component: module.WeighingPartyEditPage })) },
      { path: "weighing/parties/:id/edit", lazy: () => import("@/pages/weighing/materials").then((module) => ({ Component: module.WeighingPartyEditPage })) },
      { path: "weighing/categories", lazy: () => import("@/pages/weighing/materials").then((module) => ({ Component: module.WeighingCategoriesPage })) },
      {
        index: true,
        element: <Navigate to="/weighing/tasks" replace />,
      },
      {
        path: "dashboard",
        lazy: () => import("@/pages/dashboard").then((module) => ({ Component: module.DashboardPage })),
      },
      {
        path: "system/user",
        lazy: () => import("@/pages/system/users").then((module) => ({ Component: module.UsersPage })),
      },
      {
        path: "forms/basic",
        lazy: () => import("@/pages/form-example").then((module) => ({ Component: module.FormExamplePage })),
      },
      {
        path: "examples/list",
        lazy: () => import("@/pages/examples/list-demo").then((module) => ({ Component: module.ListDemoPage })),
      },
      {
        path: "examples/tree",
        lazy: () => import("@/pages/examples/tree-demo").then((module) => ({ Component: module.TreeDemoPage })),
      },
      {
        path: "examples/tree-table",
        lazy: () => import("@/pages/examples/tree-table-demo").then((module) => ({ Component: module.TreeTableDemoPage })),
      },
      {
        path: "examples/detail",
        lazy: () => import("@/pages/examples/detail-demo").then((module) => ({ Component: module.DetailDemoPage })),
      },
      {
        path: "examples/file-upload",
        lazy: () => import("@/pages/examples/file-upload-demo").then((module) => ({ Component: module.FileUploadDemoPage })),
      },
      {
        path: "examples/browse-picker",
        lazy: () => import("@/pages/examples/browse-picker-demo").then((module) => ({ Component: module.BrowsePickerDemoPage })),
      },
      {
        path: "settings",
        lazy: () => import("@/pages/settings").then((module) => ({ Component: module.SettingsPage })),
      },
      {
        path: "system",
        element: <Navigate to="/system/user" replace />,
      },
      {
        path: "system/dept",
        lazy: () => import("@/pages/system/depts").then((module) => ({ Component: module.SystemDeptsPage })),
      },
      {
        path: "system/dict",
        lazy: () => import("@/pages/system/dicts").then((module) => ({ Component: module.SystemDictsPage })),
      },
      {
        path: "system/config",
        lazy: () => import("@/pages/system/configs").then((module) => ({ Component: module.SystemConfigsPage })),
      },
      {
        path: "system/role",
        lazy: () => import("@/pages/system/roles").then((module) => ({ Component: module.SystemRolesPage })),
      },
      {
        path: "system/menu",
        lazy: () => import("@/pages/system/menus").then((module) => ({ Component: module.SystemMenusPage })),
      },
      {
        path: "system/login-log",
        lazy: () => import("@/pages/system/logs/login-logs").then((module) => ({ Component: module.SystemLoginLogsPage })),
      },
      {
        path: "system/oper-log",
        lazy: () => import("@/pages/system/logs/oper-logs").then((module) => ({ Component: module.SystemOperLogsPage })),
      },
      {
        path: "system/file",
        lazy: () => import("@/pages/system/files").then((module) => ({ Component: module.SystemFilesPage })),
      },
      {
        path: "notifications",
        lazy: () => import("@/pages/notifications").then((module) => ({ Component: module.NotificationsPage })),
      },
      {
        path: "notifications/:id",
        lazy: () => import("@/pages/notifications").then((module) => ({ Component: module.NotificationsPage })),
      },
      {
        path: "system/notification",
        lazy: async () => {
          const { NotificationManagePage } = await import("@/pages/system/notifications");
          return { element: <RequireMessageAdmin resource="通知"><NotificationManagePage /></RequireMessageAdmin> };
        },
      },
      { path: "announcements", lazy: () => import("@/pages/announcements").then((module) => ({ Component: module.AnnouncementsPage })) },
      {
        path: "system/announcement",
        lazy: async () => {
          const { AnnouncementManagePage } = await import("@/pages/system/announcements");
          return { element: <RequireMessageAdmin resource="公告"><AnnouncementManagePage /></RequireMessageAdmin> };
        },
      },
      {
        path: "monitor/server",
        lazy: () => import("@/pages/monitor/server").then((module) => ({ Component: module.ServerMonitoringPage })),
      },
      {
        path: "account/profile",
        lazy: () => import("@/pages/account-profile").then((module) => ({ Component: module.AccountProfilePage })),
      },
      {
        path: "account/change-password",
        lazy: () => import("@/pages/change-password").then((module) => ({ Component: module.ChangePasswordPage })),
      },
      {
        path: "*",
        element: <NotFoundPage />,
      },
    ],
  },
]);
