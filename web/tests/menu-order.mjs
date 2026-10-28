import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const baseUrl = "http://127.0.0.1:4182";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4182", "--strictPort"], {
  cwd: new URL("..", import.meta.url),
  stdio: ["ignore", "pipe", "pipe"],
});

async function waitForServer() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      if ((await fetch(`${baseUrl}/tests/menu-order.html`)).ok) return;
    } catch {
      // Vite is still starting.
    }
    await delay(250);
  }
  throw new Error("Vite test server did not start");
}

try {
  await waitForServer();
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}),
  });
  try {
    const page = await browser.newPage();
    await page.goto(`${baseUrl}/tests/menu-order.html`, { waitUntil: "networkidle" });
    const labels = (scenario) => page.locator(`[data-testid="scenario-${scenario}"] li`).allTextContents();

    assert.deepEqual(await labels("tie"), ["单据查询", "基础资料", "业务菜单甲", "系统概览", "业务菜单乙", "消息中心", "开发示例"], "本地入口必须在同序号的业务菜单之前");
    assert.deepEqual(await labels("interleave"), ["业务菜单甲", "固定入口", "业务菜单乙"], "导航必须按 sortOrder 交错排序");
    assert.equal(await page.getByTestId("system-group-count").textContent(), "1", "系统管理继续复用后端菜单分组");
    assert.deepEqual(await labels("system"), ["用户管理", "角色管理", "菜单管理"], "后端系统管理菜单保持原样");
    assert.deepEqual(await labels("local"), ["设备维护"], "本地维护入口按 RBAC 放入本地管理");
    assert.deepEqual(await labels('operator'), ['单据查询', '基础资料', '日志管理', '系统概览', '称重工作台'], '操作员展示系统概览及原四类菜单');
    assert.deepEqual(await labels('operator-children'), ['待回磅任务', '原始称重', '最终单据', '物料资料', '往来主体', '物料分类', '登录日志', '操作日志']);
    assert.deepEqual(await labels('operator-no-permission'), ['系统概览'], '概览入口默认可见，业务菜单仍遵守已有权限');
    assert.ok((await labels('admin-operator')).includes('系统概览'), 'ADMIN 优先于操作员菜单限制');
    assert.deepEqual(await labels('admin-log-children'), ['登录日志', '操作日志', '同步日志'], '同步日志应放在管理员日志管理下');
    console.log("menu order passed");
  } finally {
    await browser.close();
  }
} finally {
  vite.kill("SIGTERM");
}
