import { LayoutDashboard } from "lucide-react";
import { createRoot } from "react-dom/client";
import {
  buildAppNavigation,
  convertUserMenusToNavItems,
  defaultNavItems,
  mergeNavItems,
  type NavItem,
} from "@/config/navigation";
import type { CurrentUserMenu } from "@/types";

function menu(id: number, menuName: string, path: string, sortOrder: number): CurrentUserMenu {
  return {
    id,
    parentId: 0,
    menuName,
    menuType: "MENU",
    path,
    component: null,
    externalUrl: null,
    icon: "system",
    permissionCode: null,
    sortOrder,
    visible: 1,
    children: [],
  };
}

const systemMenus: CurrentUserMenu[] = [{
  id: 1,
  parentId: 0,
  menuName: "系统管理",
  menuType: "DIR",
  path: "/system",
  component: null,
  externalUrl: null,
  icon: "Settings",
  permissionCode: "system",
  sortOrder: 1,
  visible: 1,
  children: [
    menu(2, "用户管理", "/system/user", 1),
    menu(3, "角色管理", "/system/role", 2),
    menu(4, "菜单管理", "/system/menu", 3),
  ],
}];

const systemNavigation = buildAppNavigation({
  menus: systemMenus,
  isAdmin: false,
  capabilities: {
    maintenanceEnabled: true,
    permissionCodes: ["agent:maintenance:view"],
  },
});
const systemGroup = systemNavigation.find((item) => item.path === "/system");
const localManagement = systemNavigation.find((item) => item.path === "/local-management");

const scenarios: Record<string, NavItem[]> = {
  // 本地入口与业务菜单 sortOrder 相同时，本地入口保持在前
  tie: mergeNavItems(
    defaultNavItems,
    convertUserMenusToNavItems([menu(1, "业务菜单甲", "/biz/a", 0), menu(2, "业务菜单乙", "/biz/b", 10)]),
  ),
  // 本地入口声明 sortOrder 后，与业务菜单按配置顺序交错
  interleave: mergeNavItems(
    [{ label: "固定入口", path: "/fixed", icon: LayoutDashboard, sortOrder: 5 }],
    convertUserMenusToNavItems([menu(3, "业务菜单甲", "/biz/a", 1), menu(4, "业务菜单乙", "/biz/b", 9)]),
  ),
};

createRoot(document.getElementById("root")!).render(
  <div>
    {Object.entries(scenarios).map(([name, items]) => (
      <ul key={name} data-testid={`scenario-${name}`}>
        {items.map((item) => (
          <li key={item.path}>{item.label}</li>
        ))}
      </ul>
    ))}
    <p data-testid="system-group-count">
      {systemNavigation.filter((item) => item.path === "/system").length}
    </p>
    <ul data-testid="scenario-system">
      {systemGroup?.children?.map((item) => (
        <li key={item.path}>{item.label}</li>
      ))}
    </ul>
    <ul data-testid="scenario-local">
      {localManagement?.children?.map((item) => (
        <li key={item.path}>{item.label}</li>
      ))}
    </ul>
  </div>,
);
