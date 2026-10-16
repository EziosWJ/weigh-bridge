import { useState } from "react";
import { createRoot } from "react-dom/client";
import { MemoryRouter, useLocation } from "react-router-dom";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { useAuthStore } from "@/store/auth-store";
import type { CurrentUserMenu } from "@/types";
import "@/styles/globals.css";

const params = new URLSearchParams(window.location.search);
const grouped = params.get("grouped") === "true";
const menus: CurrentUserMenu[] = Array.from(
  { length: Number(params.get("count") ?? 20) },
  (_, index) => ({
    id: index + 1,
    parentId: grouped ? 100 : 0,
    menuName: `测试菜单${index + 1}`,
    menuType: "MENU",
    path: `/diagnosis/${index + 1}`,
    component: null,
    icon: null,
    permissionCode: null,
    sortOrder: index,
    visible: 1,
    children: [],
  }),
);

useAuthStore.setState({
  menus: grouped
    ? [{ ...menus[0], id: 100, parentId: 0, menuName: "测试分组", menuType: "DIR", path: "/diagnosis", children: menus }]
    : menus,
});

export function SidebarScrollFixture() {
  const [collapsed, setCollapsed] = useState(params.get("collapsed") === "true");
  const location = useLocation();

  return (
    <>
      <AppSidebar collapsed={collapsed} />
      <main style={{ marginLeft: 260, minHeight: 1800 }}>
        <button onClick={() => setCollapsed((value) => !value)}>切换侧边栏</button>
        <output aria-label="当前路径">{location.pathname}</output>
        <p>主内容区</p>
      </main>
    </>
  );
}

createRoot(document.getElementById("root")!).render(
  <MemoryRouter initialEntries={["/dashboard"]}>
    <SidebarScrollFixture />
  </MemoryRouter>,
);
