import {
  FileSearch,
  FileText,
  ListOrdered,
  LayoutDashboard,
  Bell,
  Network,
  PanelLeft,
  Table2,
  Upload,
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
    label: "工作台",
    path: "/dashboard",
    icon: LayoutDashboard,
    sortOrder: 0,
  },
  {
    label: "我的通知",
    path: "/notifications",
    icon: Bell,
    sortOrder: 0,
  },
  { label: "公告", path: "/announcements", icon: FileText, sortOrder: 0 },

  {
    label: "页面示例",
    path: "/examples",
    icon: FileText,
    sortOrder: 0,
    children: [
      {
        label: "标准表单 Demo",
        path: "/forms/basic",
        icon: FileText,
      },
      {
        label: "标准列表页 Demo",
        path: "/examples/list",
        icon: Table2,
      },
      {
        label: "树形列表 Demo",
        path: "/examples/tree",
        icon: Network,
      },
      {
        label: "左树右表 Demo",
        path: "/examples/tree-table",
        icon: PanelLeft,
      },
      {
        label: "详情页 Demo",
        path: "/examples/detail",
        icon: FileSearch,
      },
      {
        label: "文件上传 Demo",
        path: "/examples/file-upload",
        icon: Upload,
      },
      {
        label: "浏览选择 Demo",
        path: "/examples/browse-picker",
        icon: ListOrdered,
      },
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
  "/dashboard": "工作台",
  "/notifications": "我的通知",
  "/announcements": "公告",
  "/system/announcement": "公告管理",
  "/forms/basic": "标准表单 Demo",
  "/examples": "页面示例",
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
  "/monitor": "系统监控",
  "/monitor/server": "服务器监控",
  "/account/profile": "个人中心",
  "/account/change-password": "修改密码",
};

export const navItems = defaultNavItems;
export const routeTitleMap = staticRouteTitleMap;
