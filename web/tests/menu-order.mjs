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

    assert.deepEqual(await labels("tie"), ["工作台", "我的通知", "公告", "页面示例", "业务菜单甲", "业务菜单乙"], "本地入口必须在同序号的业务菜单之前");
    assert.deepEqual(await labels("interleave"), ["业务菜单甲", "固定入口", "业务菜单乙"], "导航必须按 sortOrder 交错排序");
    console.log("menu order passed");
  } finally {
    await browser.close();
  }
} finally {
  vite.kill("SIGTERM");
}
