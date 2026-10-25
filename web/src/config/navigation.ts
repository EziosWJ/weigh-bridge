import {
  Scale,
  FileSearch,
  FileText,
  ListOrdered,
  LayoutDashboard,
  Bell,
  Camera,
  Network,
  PanelLeft,
  Table2,
  Upload,
  Wrench,
  Users,
  Shield,
  Settings,
  type LucideIcon,
} from "lucide-react";
import { getMenuIcon } from "@/lib/menu-icons";
import type { CurrentUserMenu } from "@/types";

export type NavItem = {
  label: string;
  path: string;
  icon: LucideIcon;
  sortOrder?: number;
  externalUrl?: string;
  activePaths?: string[];
  children?: NavItem[];
};

export const defaultNavItems: NavItem[] = [
  {
    label: "称重作业", path: "/weighing/operations", icon: Scale, sortOrder: -40,
    children: [
      { label: "本厂称重", path: "/weighing/sites", icon: Scale },
      { label: "运输任务", path: "/weighing/tasks", icon: Table2 },
    ],
  },
  {
    label: "单据查询", path: "/weighing/documents", icon: FileSearch, sortOrder: -30,
    children: [{ label: "原始称重", path: "/weighing/originals", icon: FileSearch }, { label: "最终单据", path: "/weighing/tickets", icon: FileText }],
  },
  {
    label: "基础资料", path: "/weighing/references", icon: Table2, sortOrder: -20,
    children: [
      { label: "物料资料", path: "/weighing/materials", icon: Table2 },
      { label: "往来主体", path: "/weighing/parties", icon: Network },
      { label: "物料分类", path: "/weighing/categories", icon: ListOrdered },
    ],
  },
  { label: "系统概览", path: "/dashboard", icon: LayoutDashboard, sortOrder: 10 },
  {
    label: "消息中心", path: "/messages", icon: Bell, sortOrder: 20,
    children: [
      { label: "我的通知", path: "/notifications", icon: Bell },
      { label: "公告", path: "/announcements", icon: FileText },
    ],
  },
  {
    label: "开发示例", path: "/examples", icon: FileText, sortOrder: 100,
    children: [
      { label: "标准表单 Demo", path: "/forms/basic", icon: FileText },
      { label: "标准列表页 Demo", path: "/examples/list", icon: Table2 },
      { label: "树形列表 Demo", path: "/examples/tree", icon: Network },
      { label: "左树右表 Demo", path: "/examples/tree-table", icon: PanelLeft },
      { label: "详情页 Demo", path: "/examples/detail", icon: FileSearch },
      { label: "文件上传 Demo", path: "/examples/file-upload", icon: Upload },
      { label: "浏览选择 Demo", path: "/examples/browse-picker", icon: ListOrdered },
    ],
  },
];

export const notificationManageNavItem: NavItem = {
  label: "通知管理",
  path: "/system/notification",
  icon: Bell,
  sortOrder: 0,
};
export const announcementManageNavItem: NavItem = { label: "公告管理", path: "/system/announcement", icon: FileText, sortOrder: 0 };
export const agentMaintenanceNavItem: NavItem = { label: "设备维护", path: "/system/agent-maintenance", icon: Wrench, sortOrder: 5 };
export const cameraManagementNavItem: NavItem = { label: "摄像头管理", path: "/weighing/cameras", icon: Camera, sortOrder: 6 };
export const localManagementNavItem: NavItem = {
  label: "本地管理", path: "/local-management", icon: Shield, sortOrder: 0,
  children: [
    { label: "账号管理", path: "/system/user", icon: Users },
    { label: "角色与权限", path: "/system/role", icon: Shield },
    agentMaintenanceNavItem,
  ],
};

function collectNavPaths(items: NavItem[]) {
  const paths = new Set<string>();

  const walk = (navItems: NavItem[]) => {
    navItems.forEach((item) => {
      paths.add(item.path);
      if (item.externalUrl) paths.add(item.externalUrl);
      if (item.children?.length) walk(item.children);
    });
  };

  walk(items);
  return paths;
}

function filterDuplicateNavItems(
  items: NavItem[],
  existingPaths: Set<string>,
): NavItem[] {
  const nextItems: NavItem[] = [];

  items.forEach((item) => {
    const key = item.externalUrl ?? item.path;
    if (existingPaths.has(key)) return;

    existingPaths.add(key);

    const children = item.children?.length
      ? filterDuplicateNavItems(item.children, existingPaths)
      : undefined;

    if (item.children?.length && !children?.length) return;

    nextItems.push({
      ...item,
      children: children?.length ? children : undefined,
    });
  });

  return nextItems;
}

function toNavItem(menu: CurrentUserMenu): NavItem | null {
  if (menu.visible !== 1) return null;

  const sortedChildren = [...(menu.children ?? [])].sort(
    (a, b) => a.sortOrder - b.sortOrder,
  );
  const children = sortedChildren
    .map((child) => toNavItem(child))
    .filter((item): item is NavItem => Boolean(item));

  if (menu.menuType === "DIR" && children.length === 0) return null;

  if (menu.menuType === "LINK" && !menu.externalUrl) return null;

  const externalUrl = menu.menuType === "LINK" ? menu.externalUrl : "";
  const path = externalUrl || menu.path;

  if (!path) return null;

  return {
    label: menu.menuName,
    path,
    icon: getMenuIcon(menu.icon),
    sortOrder: menu.sortOrder,
    externalUrl: externalUrl || undefined,
    children: children.length > 0 ? children : undefined,
  };
}

export function convertUserMenusToNavItems(menus: CurrentUserMenu[]): NavItem[] {
  return [...menus]
    .sort((a, b) => a.sortOrder - b.sortOrder)
    .map((menu) => toNavItem(menu))
    .filter((item): item is NavItem => Boolean(item));
}

export function mergeNavItems(
  baseItems: NavItem[],
  userItems: NavItem[],
): NavItem[] {
  const existingPaths = collectNavPaths(baseItems);
  const dedupedUserItems = filterDuplicateNavItems(userItems, existingPaths);

  return [...baseItems, ...dedupedUserItems]
    .map((item, index) => ({ item, index }))
    .sort((a, b) => {
      const orderA = a.item.sortOrder ?? Number.MAX_SAFE_INTEGER;
      const orderB = b.item.sortOrder ?? Number.MAX_SAFE_INTEGER;
      return orderA - orderB || a.index - b.index;
    })
    .map(({ item }) => item);
}

type NavigationCapabilities = {
  maintenanceEnabled?: boolean;
  permissionCodes: string[];
};

type BuildAppNavigationOptions = {
  menus: CurrentUserMenu[];
  isAdmin: boolean;
  capabilities?: NavigationCapabilities | null;
};

function filterWeighingNavItems(
  items: NavItem[],
  capabilities: NavigationCapabilities | null | undefined,
): NavItem[] {
  const permissionCodes = capabilities?.permissionCodes ?? [];
  const has = (code: string) => permissionCodes.includes(`weighing:${code}`);

  return items.flatMap((item) => {
    if (["/weighing/temporary-parties", "/weighing/node-sync", "/weighing/node-status", "/weighing/site-config"].includes(item.path)) return [];
    if (!item.path.startsWith("/weighing")) return [item];
    if (item.children?.length) {
      const children = filterWeighingNavItems(item.children, capabilities);
      return children.length ? [{ ...item, children }] : [];
    }
    const path = item.path;
    const allowed = path === "/weighing/workbench" ? has("confirm")
      : path === "/weighing/settings" ? false
      : ["/weighing/materials", "/weighing/parties", "/weighing/categories"].includes(path) ? has("master:read")
      : has("read");
    return allowed ? [item] : [];
  });
}

function hasNavigationPath(items: NavItem[], path: string): boolean {
  return items.some(
    (item) =>
      item.path === path ||
      (item.children?.length ? hasNavigationPath(item.children, path) : false),
  );
}

function sortNavigationItems(items: NavItem[]): NavItem[] {
  return items
    .map((item, index) => ({ item, index }))
    .sort((a, b) => {
      const orderA = a.item.sortOrder ?? Number.MAX_SAFE_INTEGER;
      const orderB = b.item.sortOrder ?? Number.MAX_SAFE_INTEGER;
      return orderA - orderB || a.index - b.index;
    })
    .map(({ item }) => item);
}

/**
 * A role that may maintain the two local names should reach the same
 * 系统管理 → 配置管理 entry without being granted the generic system:config
 * permission. The backend still enforces each group's own permission.
 */
function withLocalConfigurationEntry(items: NavItem[]): NavItem[] {
  if (hasNavigationPath(items, "/system/config")) return items;
  const entry: NavItem = { label: "配置管理", path: "/system/config", icon: Settings };
  const systemIndex = items.findIndex((item) => item.path === "/system");
  if (systemIndex >= 0) {
    return items.map((item, index) =>
      index === systemIndex
        ? { ...item, children: sortNavigationItems([...(item.children ?? []), entry]) }
        : item,
    );
  }
  return sortNavigationItems([
    ...items,
    { label: "系统管理", path: "/system", icon: Settings, sortOrder: 1, children: [entry] },
  ]);
}

/**
 * Builds the single navigation tree used by both the sidebar and menu search.
 * The backend menu tree stays the source of truth for 系统管理; local-only
 * entries are merged into it instead of replacing the group.
 */
export function buildAppNavigation({
  menus,
  isAdmin,
  capabilities,
}: BuildAppNavigationOptions): NavItem[] {
  const base = defaultNavItems.map((item) =>
    item.path === "/messages" && isAdmin
      ? {
          ...item,
          children: [
            ...(item.children ?? []),
            notificationManageNavItem,
            announcementManageNavItem,
          ],
        }
      : item,
  );
  const permissionCodes = capabilities?.permissionCodes ?? [];
  const maintenanceItem = capabilities?.maintenanceEnabled &&
    (permissionCodes.includes("agent:maintenance:view") ||
      permissionCodes.includes("agent:maintenance:manage"))
    ? agentMaintenanceNavItem
    : undefined;
  const localChildren = [
    permissionCodes.includes("system:user") ? localManagementNavItem.children?.[0] : undefined,
    permissionCodes.includes("system:role") ? localManagementNavItem.children?.[1] : undefined,
    maintenanceItem,
    permissionCodes.includes("weighing:camera:view") ? cameraManagementNavItem : undefined,
  ].filter((item): item is NavItem => Boolean(item));
  const localItems = localChildren.length
    ? [...base, { ...localManagementNavItem, children: localChildren }]
    : base;
  const userItems = convertUserMenusToNavItems(menus);
  const userItemsWithMaintenance = userItems;

  const merged = mergeNavItems(
    filterWeighingNavItems(localItems, capabilities),
    filterWeighingNavItems(userItemsWithMaintenance, capabilities),
  );
  const canMaintainLocalIdentity = permissionCodes.some(
    (code) => code === "weighing:site:manage" || code === "weighing:site:export",
  );
  return canMaintainLocalIdentity ? withLocalConfigurationEntry(merged) : merged;
}

export function createUserMenuTitleMap(
  menus: CurrentUserMenu[],
): Record<string, string> {
  const titleMap: Record<string, string> = {};

  const walk = (items: CurrentUserMenu[]) => {
    items.forEach((item) => {
      if (item.visible !== 1) return;

      const path = item.menuType === "LINK" ? item.externalUrl : item.path;
      if (path) titleMap[path] = item.menuName;
      if (item.children?.length) walk(item.children);
    });
  };

  walk(menus);
  return titleMap;
}

export const staticRouteTitleMap: Record<string, string> = {
  "/weighing": "称重演示",
  "/weighing/workbench": "称重工作台",
  "/weighing/sites": "本厂称重",
  "/weighing/site-config": "本厂配置",
  "/weighing/cameras": "摄像头管理",
  "/weighing/tasks": "运输任务",
  "/weighing/tickets": "最终单据",
  "/weighing/originals": "原始称重",
  "/weighing/materials": "物料资料",
  "/weighing/parties": "往来主体",
  "/weighing/categories": "物料分类",
  "/dashboard": "系统概览",
  "/notifications": "我的通知",
  "/announcements": "公告",
  "/system/announcement": "公告管理",
  "/forms/basic": "标准表单 Demo",
  "/examples": "开发示例",
  "/examples/list": "标准列表页 Demo",
  "/examples/tree": "树形列表 Demo",
  "/examples/tree-table": "左树右表 Demo",
  "/examples/detail": "详情页 Demo",
  "/examples/file-upload": "文件上传 Demo",
  "/examples/browse-picker": "浏览选择 Demo",
  "/settings": "系统设置",
  "/system": "系统管理",
  "/system/user": "用户管理",
  "/system/dept": "部门管理",
  "/system/dict": "字典管理",
  "/system/config": "配置管理",
  "/system/role": "角色管理",
  "/system/menu": "菜单管理",
  "/system/login-log": "登录日志",
  "/system/oper-log": "操作日志",
  "/system/file": "文件管理",
  "/system/notification": "通知管理",
  "/system/agent-maintenance": "设备维护",
  "/monitor": "系统监控",
  "/monitor/server": "服务器监控",
  "/account/profile": "个人中心",
  "/account/change-password": "修改密码",
};

export const navItems = defaultNavItems;
export const routeTitleMap = staticRouteTitleMap;
