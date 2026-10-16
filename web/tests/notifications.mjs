import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4181;
const base = `http://127.0.0.1:${port}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", String(port), "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: "pipe" });
let output = "";
vite.stdout.on("data", (chunk) => { output += chunk; });
vite.stderr.on("data", (chunk) => { output += chunk; });
const publicAnnouncement = { id: 1, title: "公开维护公告", content: "今晚维护", scope: "PUBLIC", status: "PUBLISHED", publishTime: new Date().toISOString() };
const internalAnnouncement = { ...publicAnnouncement, id: 2, title: "站内公告", content: "仅登录可见", scope: "INTERNAL" };
const notification = { id: 1, title: "业务通知", content: "订单已通过", sourceType: "BUSINESS", jumpPath: "/dashboard", isRead: 0, publishTime: new Date().toISOString() };
let browser;
try {
  for (let i = 0; i < 40; i += 1) { try { if ((await fetch(base)).ok) break; } catch { /* startup */ } await delay(250); }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext();
  await context.addInitScript(() => {
    const originalFetch = window.fetch;
    window.messageStreams = {};
    window.emitMessage = (path, change) => window.messageStreams[path]?.enqueue(new TextEncoder().encode(`data: ${JSON.stringify(change)}\n\n`));
    window.fetch = (input, init) => {
      const path = new URL(String(input), window.location.origin).pathname;
      if (!path.endsWith("/events")) return originalFetch(input, init);
      if (window.failMessageStreams) return Promise.resolve(new Response("unavailable", { status: 503 }));
      const body = new ReadableStream({ start(controller) { window.messageStreams[path] = controller; controller.enqueue(new TextEncoder().encode('data: {"type":"ready"}\n\n')); init?.signal?.addEventListener("abort", () => { delete window.messageStreams[path]; controller.close(); }); } });
      return Promise.resolve(new Response(body, { headers: { "Content-Type": "text/event-stream" } }));
    };
  });
  let active = true;
  let announcements = [publicAnnouncement];
  let role = "ADMIN";
  let unread = 1;
  let listRequests = 0;
  await context.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith("/api/")) { await route.continue(); return; }
    let data = [];
    if (path === "/api/auth/login") data = { tokenValue: "Bearer test" };
    else if (path === "/api/auth/me") data = { id: 1, username: "admin", nickname: "测试用户", roles: [{ roleCode: role }] };
    else if (path === "/api/public/announcement/page") data = { records: active ? announcements : [], total: active ? announcements.length : 0 };
    else if (path === "/api/system/announcement/page") data = { records: [internalAnnouncement], total: 1 };
    else if (path === "/api/public/announcement/1") { await route.fulfill({ json: { code: active ? 200 : 404, message: active ? "success" : "公告已失效", data: active ? publicAnnouncement : null } }); return; }
    else if (path.endsWith("/unread-count")) data = unread;
    else if (path === "/api/system/notification/page") { listRequests += 1; data = { records: [{ ...notification, isRead: unread ? 0 : 1 }], total: 1 }; }
    else if (path === "/api/system/notification/1") { unread = 0; data = { ...notification, isRead: 1 }; }
    else if (path.endsWith("/read-all")) unread = 0;
    else if (path === "/api/system/announcement-admin/page") data = { records: [], total: 0 };
    await route.fulfill({ json: { code: 200, message: "success", data } });
  });
  const page = await context.newPage();
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => { if (message.type() === "error") console.error(message.text()); });
  await page.goto(`${base}/login`);
  try { await page.getByRole("button", { name: "公开维护公告", exact: true }).waitFor({ timeout: 30000 }); } catch (error) { console.error(await page.locator("body").innerText(), errors, await page.evaluate(() => Object.keys(window.messageStreams))); throw error; }
  assert.equal(await page.getByText("站内公告", { exact: true }).count(), 0);
  await page.screenshot({ path: "/tmp/notification-public-announcements.png", fullPage: true });
  await page.getByRole("button", { name: "关闭公告：公开维护公告" }).click();
  assert.equal(await page.getByRole("button", { name: "公开维护公告", exact: true }).count(), 0);
  announcements = [{ ...publicAnnouncement, id: 3, title: "新公开公告" }, publicAnnouncement];
  await page.evaluate(() => window.emitMessage("/api/public/announcement/events", { type: "announcement" }));
  await page.getByRole("button", { name: "新公开公告", exact: true }).waitFor();
  assert.equal(await page.getByRole("button", { name: "公开维护公告", exact: true }).count(), 0, "dismissal remains local to the old announcement");
  await page.getByRole("button", { name: "查看公告", exact: true }).click();
  await page.getByRole("button", { name: "公开维护公告", exact: true }).click();
  await page.getByText("今晚维护", { exact: true }).waitFor();
  active = false;
  await page.evaluate(() => window.emitMessage("/api/public/announcement/events", { type: "announcement" }));
  await page.getByText("公告已失效", { exact: true }).waitFor();
  assert.equal(await page.getByText("今晚维护", { exact: true }).count(), 0);
  active = true;
  announcements = [publicAnnouncement];
  await page.getByRole("button", { name: "关闭", exact: true }).click();
  await page.getByPlaceholder("请输入用户名", { exact: true }).fill("admin");
  await page.getByPlaceholder("请输入密码", { exact: true }).fill("test-password");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.waitForURL("**/dashboard");
  await page.getByRole("button", { name: "站内公告", exact: true }).waitFor();
  assert.equal(await page.getByRole("button", { name: "公开维护公告", exact: true }).count(), 0, "dismissal must survive login within the same tab visit");
  await page.getByRole("link", { name: "我的通知", exact: true }).click();
  await page.getByText("业务通知", { exact: true }).first().waitFor();
  await page.screenshot({ path: "/tmp/notification-ui-desktop.png", fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.waitForFunction(() => document.documentElement.scrollWidth <= window.innerWidth, undefined, { timeout: 3000 });
  await page.screenshot({ path: "/tmp/notification-ui-mobile.png", fullPage: true, animations: "disabled" });
  const mobileFits = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  if (!mobileFits) console.error(await page.evaluate(() => [...document.querySelectorAll("body *")].map((element) => ({ tag: element.tagName, class: element.className, right: element.getBoundingClientRect().right, width: element.getBoundingClientRect().width })).filter((element) => element.right > innerWidth && element.width > 0).slice(0, 12)));
  assert.equal(mobileFits, true, "announcement and notification layout must fit mobile");
  await page.setViewportSize({ width: 1440, height: 1000 });
  const initialRequests = listRequests;
  await page.clock.install();
  await page.clock.runFor(65000);
  await delay(100);
  assert.equal(listRequests, initialRequests, "messages must not be polled");
  const otherPage = await context.newPage();
  await otherPage.goto(`${base}/notifications`);
  await otherPage.getByText("业务通知", { exact: true }).first().waitFor();
  await otherPage.bringToFront();
  // Headless Chromium can report multiple virtual windows as focused. Supply
  // the browser's background-page signals explicitly, rather than app internals.
  await page.evaluate(() => {
    Object.defineProperty(document, "hasFocus", { configurable: true, value: () => false });
    Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "hidden" });
  });
  await Promise.all([page.evaluate(() => window.emitMessage("/api/system/notification/events", { type: "notification", id: 42 })), otherPage.evaluate(() => window.emitMessage("/api/system/notification/events", { type: "notification", id: 42 }))]);
  await otherPage.getByText("收到新的站内通知", { exact: true }).waitFor();
  assert.equal(await page.getByText("收到新的站内通知", { exact: true }).count(), 0, "background page must stay quiet");
  await page.getByRole("button", { name: "查看", exact: true }).click();
  await page.getByText("订单已通过", { exact: true }).waitFor();
  await otherPage.evaluate(() => window.emitMessage("/api/system/notification/events", { type: "read" }));
  await otherPage.getByRole("cell", { name: "已读", exact: true }).waitFor();
  await otherPage.close();
  await page.evaluate(() => { delete document.hasFocus; delete document.visibilityState; });
  await page.getByRole("button", { name: "前往业务页面", exact: true }).click();
  await page.waitForURL("**/dashboard");
  role = "USER";
  await page.goto(`${base}/system/announcement`);
  await page.getByText("无权限管理公告", { exact: true }).waitFor();
  await page.getByRole("button", { name: "测试用户", exact: true }).click();
  await page.getByRole("menuitem", { name: "退出登录", exact: true }).click();
  await page.waitForURL("**/login");
  assert.equal(await page.evaluate(() => localStorage.getItem("react-admin-auth")), null);
  assert.equal(await page.evaluate(() => Boolean(window.messageStreams["/api/system/notification/events"])), false, "logout must abort the personal stream");
  assert.equal(await page.getByRole("button", { name: "站内公告", exact: true }).count(), 0);
  await context.addInitScript(() => { window.failMessageStreams = true; });
  await page.evaluate(() => localStorage.setItem("react-admin-auth", JSON.stringify({ token: "Bearer test", user: null })));
  await page.goto(`${base}/notifications`);
  await page.getByText("业务通知", { exact: true }).first().waitFor();
  await page.getByText("消息连接中断，正在重新连接；也可手动刷新。", { exact: true }).waitFor();
  const degradedRequests = listRequests;
  await page.clock.install();
  await page.clock.runFor(35000);
  await delay(100);
  assert.equal(listRequests, degradedRequests, "connection retries must not become message polling");
  assert.deepEqual(errors, []);
  await context.close();
  console.log("公告可见性、失效详情、通知已读跳转、ADMIN 权限与无轮询浏览器验证通过。");
} finally {
  await browser?.close();
  vite.kill("SIGTERM");
  if (vite.exitCode === null) await new Promise((resolve) => vite.once("exit", resolve));
  if (output.includes("Error")) process.stderr.write(output);
}
