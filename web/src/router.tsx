import { createBrowserRouter, Navigate } from "react-router-dom";
import { RequireAuth } from "@/components/auth/require-auth";
import { AppShell } from "@/components/layout/app-shell";
import { AccountProfilePage } from "@/pages/account-profile";
import { ChangePasswordPage } from "@/pages/change-password";
import { DashboardPage } from "@/pages/dashboard";
import { BrowsePickerDemoPage } from "@/pages/examples/browse-picker-demo";
import { DetailDemoPage } from "@/pages/examples/detail-demo";
import { FileUploadDemoPage } from "@/pages/examples/file-upload-demo";
import { FormExamplePage } from "@/pages/form-example";
import { ListDemoPage } from "@/pages/examples/list-demo";
import { TreeDemoPage } from "@/pages/examples/tree-demo";
import { TreeTableDemoPage } from "@/pages/examples/tree-table-demo";
import { LoginPage } from "@/pages/login";
import { NotFoundPage } from "@/pages/not-found";
import { SettingsPage } from "@/pages/settings";
import { SystemConfigsPage } from "@/pages/system/configs";
import { SystemDeptsPage } from "@/pages/system/depts";
import { SystemDictsPage } from "@/pages/system/dicts";
import { SystemFilesPage } from "@/pages/system/files";
import { SystemLoginLogsPage } from "@/pages/system/logs/login-logs";
import { SystemOperLogsPage } from "@/pages/system/logs/oper-logs";
import { SystemMenusPage } from "@/pages/system/menus";
import { SystemRolesPage } from "@/pages/system/roles";
import { UsersPage } from "@/pages/system/users";
import { NotificationsPage } from "@/pages/notifications";
import { NotificationManagePage } from "@/pages/system/notifications";
import { ServerMonitoringPage } from "@/pages/monitor/server";
import { AnnouncementsPage } from "@/pages/announcements";
import { AnnouncementManagePage } from "@/pages/system/announcements";
import { RequireMessageAdmin } from "@/components/auth/require-message-admin";

export const router = createBrowserRouter([
  {
    path: "/login",
    element: <LoginPage />,
  },
  {
    path: "/",
    element: (
      <RequireAuth>
        <AppShell />
      </RequireAuth>
    ),
    children: [
      {
        index: true,
        element: <Navigate to="/dashboard" replace />,
      },
      {
        path: "dashboard",
        element: <DashboardPage />,
      },
      {
        path: "system/user",
        element: <UsersPage />,
      },
      {
        path: "forms/basic",
        element: <FormExamplePage />,
      },
      {
        path: "examples/list",
        element: <ListDemoPage />,
      },
      {
        path: "examples/tree",
        element: <TreeDemoPage />,
      },
      {
        path: "examples/tree-table",
        element: <TreeTableDemoPage />,
      },
      {
        path: "examples/detail",
        element: <DetailDemoPage />,
      },
      {
        path: "examples/file-upload",
        element: <FileUploadDemoPage />,
      },
      {
        path: "examples/browse-picker",
        element: <BrowsePickerDemoPage />,
      },
      {
        path: "settings",
        element: <SettingsPage />,
      },
      {
        path: "system",
        element: <Navigate to="/system/user" replace />,
      },
      {
        path: "system/dept",
        element: <SystemDeptsPage />,
      },
      {
        path: "system/dict",
        element: <SystemDictsPage />,
      },
      {
        path: "system/config",
        element: <SystemConfigsPage />,
      },
      {
        path: "system/role",
        element: <SystemRolesPage />,
      },
      {
        path: "system/menu",
        element: <SystemMenusPage />,
      },
      {
        path: "system/login-log",
        element: <SystemLoginLogsPage />,
      },
      {
        path: "system/oper-log",
        element: <SystemOperLogsPage />,
      },
      {
        path: "system/file",
        element: <SystemFilesPage />,
      },
      {
        path: "notifications",
        element: <NotificationsPage />,
      },
      {
        path: "notifications/:id",
        element: <NotificationsPage />,
      },
      {
        path: "system/notification",
        element: <RequireMessageAdmin resource="通知"><NotificationManagePage /></RequireMessageAdmin>,
      },
      { path: "announcements", element: <AnnouncementsPage /> },
      { path: "system/announcement", element: <RequireMessageAdmin resource="公告"><AnnouncementManagePage /></RequireMessageAdmin> },
      {
        path: "monitor/server",
        element: <ServerMonitoringPage />,
      },
      {
        path: "account/profile",
        element: <AccountProfilePage />,
      },
      {
        path: "account/change-password",
        element: <ChangePasswordPage />,
      },
      {
        path: "*",
        element: <NotFoundPage />,
      },
    ],
  },
]);
