import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const baseUrl = "http://127.0.0.1:4177";
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "4177", "--strictPort"], {
  cwd: new URL("..", import.meta.url),
  stdio: ["ignore", "pipe", "pipe"],
});

async function waitForServer() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      if ((await fetch(`${baseUrl}/tests/sidebar-scroll.html`)).ok) return;
    } catch {
      // Vite is still starting.
    }
    await delay(250);
  }
  throw new Error("Vite test server did not start");
}

async function measure(page) {
  return page.evaluate(() => {
    const aside = document.querySelector("aside");
    const nav = document.querySelector("nav");
    const links = nav.querySelectorAll("a");
    const last = links[links.length - 1].getBoundingClientRect();
    const logo = aside.firstElementChild.getBoundingClientRect();
    return {
      reachable: last.top >= logo.bottom && last.bottom <= innerHeight,
      logoTop: logo.top,
      logoHeight: logo.height,
      scrollTop: nav.scrollTop,
      scrollHeight: nav.scrollHeight,
      clientHeight: nav.clientHeight,
      pageScroll: scrollY,
    };
  });
}

async function scrollToBottom(page) {
  const before = await measure(page);
  assert.equal(before.reachable, false, "fixture must overflow the visible menu area");
  const nav = page.getByRole("navigation", { name: "主导航" });
  const box = await nav.boundingBox();
  await page.mouse.move(box.x + box.width / 2, Math.min(box.y + 100, 200));
  await page.mouse.wheel(0, 4000);
  await page.waitForFunction(() => document.querySelector("nav").scrollTop > 0, undefined, { timeout: 1500 });
  const after = await measure(page);
  assert.equal(after.reachable, true, `bottom menu unreachable after wheel: ${JSON.stringify(after)}`);
  assert.equal(after.logoTop, before.logoTop, "Logo must remain fixed");
  assert.equal(after.logoHeight, 64, "Logo must retain its height");
  assert.equal(after.pageScroll, before.pageScroll, "menu wheel must not scroll main content");
}

try {
  await waitForServer();
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}),
  });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 600 } });
    for (const scenario of ["flat", "collapsed", "grouped", "resize"]) {
      await page.setViewportSize({ width: 1280, height: 600 });
      const query = scenario === "collapsed" ? "?collapsed=true" : scenario === "grouped" ? "?grouped=true" : "";
      await page.goto(`${baseUrl}/tests/sidebar-scroll.html${query}`, { waitUntil: "networkidle" });
      if (scenario === "grouped") await page.getByRole("button", { name: "测试分组" }).click();
      if (scenario === "resize") await page.setViewportSize({ width: 1280, height: 320 });
      await scrollToBottom(page);
      await page.locator('nav a[href="/diagnosis/20"]').click();
      await page.waitForFunction(() => document.querySelector("output").textContent === "/diagnosis/20");
      assert.equal(await page.getByLabel("当前路径").textContent(), "/diagnosis/20");
      if (scenario === "collapsed") {
        await page.getByRole("button", { name: "切换侧边栏" }).click();
        await page.waitForFunction(() => document.querySelector("aside").getBoundingClientRect().width === 240);
        assert.equal((await measure(page)).reachable, true, "bottom menu remains reachable after expanding sidebar");
      }
      console.log(`sidebar scroll passed: ${scenario}`);
    }
    await page.setViewportSize({ width: 1280, height: 600 });
    await page.goto(`${baseUrl}/tests/sidebar-scroll.html?count=2`, { waitUntil: "networkidle" });
    const short = await measure(page);
    assert.equal(short.reachable, true);
    assert.equal(short.scrollHeight, short.clientHeight, "short menu must not need scrolling");
    await page.setViewportSize({ width: 640, height: 600 });
    assert.equal(await page.locator("aside").isVisible(), false, "sidebar stays hidden on small screens");
    console.log("sidebar scroll passed: short menu and small screen");
  } finally {
    await browser.close();
  }
} finally {
  vite.kill("SIGTERM");
}
