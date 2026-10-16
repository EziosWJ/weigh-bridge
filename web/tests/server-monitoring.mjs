import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4178;
const baseUrl = `http://127.0.0.1:${port}`;
const fixedTime = Date.now();
const collectedAt = new Date(fixedTime).toISOString();
const region = (data, status = "ok", message = "") => ({ data, status, collectedAt, message, partial: false, stale: false });
const user = (role) => ({ id: 1, username: "test", nickname: "测试用户", roles: [{ id: 1, roleName: role, roleCode: role }] });
const menu = [{ id: 30, parentId: 0, menuName: "系统监控", menuType: "DIR", path: "/monitor", icon: "monitor", sortOrder: 60, visible: 1, children: [{ id: 31, parentId: 30, menuName: "服务器监控", menuType: "MENU", path: "/monitor/server", icon: "monitor", sortOrder: 1, visible: 1, children: [] }] }];
const fixture = () => ({
  sourceId: "native:host", sampledAt: collectedAt,
  host: region({ hostname: "spec64-host", os: "Linux test", kernel: "6.8", uptimeSeconds: 172800 }),
  cpu: region({ model: "Genuine Intel(R) CPU 0000 @ 2.50GHz", logicalCores: 20, physicalCores: 10, usagePercent: 42, perCore: [80, 4, ...Array.from({ length: 18 }, (_, index) => index * 2.5)], load1: 1.2, load5: 0.8, load15: 0.5 }),
  memory: region({ totalBytes: 30.78 * 1024 ** 3, usedBytes: 10.5 * 1024 ** 3, availableBytes: 20.28 * 1024 ** 3, usagePercent: 34.1, swapTotalBytes: 0, swapUsedBytes: 0, swapUsagePercent: null }),
  filesystems: region([{ id: "root", device: "/dev/sda1", mountpoint: "/", type: "ext4", totalBytes: 100 * 1024 ** 3, usedBytes: 91 * 1024 ** 3, availableBytes: 9 * 1024 ** 3, usagePercent: 91, message: "" }, { id: "root-alias", device: "/dev/sda1", mountpoint: "/opt/data-bind", type: "ext4", totalBytes: 100 * 1024 ** 3, usedBytes: 91 * 1024 ** 3, availableBytes: 9 * 1024 ** 3, usagePercent: 91, message: "" }, { id: "run", device: "tmpfs", mountpoint: "/run", type: "tmpfs", totalBytes: 1024 ** 3, usedBytes: 0, availableBytes: 1024 ** 3, usagePercent: 0, message: "" }, { id: "docker", device: "/dev/sdb", mountpoint: "/mnt/wsl/docker-desktop-bind-mounts/example", type: "ext4", totalBytes: 1024 ** 3, usedBytes: 0, availableBytes: 1024 ** 3, usagePercent: 0, message: "" }, { id: "windows", device: "C:\\", mountpoint: "/mnt/c", type: "9p", totalBytes: 500 * 1024 ** 3, usedBytes: 220 * 1024 ** 3, availableBytes: 280 * 1024 ** 3, usagePercent: 44, message: "" }, { id: "usb", device: "/dev/sdc", mountpoint: "/run/media/user/disk", type: "ext4", totalBytes: 500 * 1024 ** 3, usedBytes: 0, availableBytes: 500 * 1024 ** 3, usagePercent: 0, message: "" }]),
  disks: region([{ id: "sda", readBytesPerSecond: 1024 ** 2, writeBytesPerSecond: 2 * 1024 ** 2, readIops: 3, writeIops: 4, message: "" }]),
network: region(["eth0", "eth1", "lo", "veth0"].map((id) => ({ id, addresses: id === "eth0" ? [{ family: "IPv4", ip: "192.168.1.10", prefixLength: 24 }, { family: "IPv4", ip: "10.0.0.5", prefixLength: 8 }, { family: "IPv6", ip: "fe80::1", prefixLength: 64 }] : [], loopback: id === "lo", virtual: id === "veth0", state: "up", speedBitsPerSecond: id === "eth0" ? 1e9 : null, receiveBytes: 1024 ** 3, transmitBytes: 2 * 1024 ** 3, receiveBytesPerSecond: 1024 ** 2, transmitBytesPerSecond: 2 * 1024 ** 2, receiveUsagePercent: id === "eth0" ? 0.84 : null, transmitUsagePercent: id === "eth0" ? 1.68 : null, receiveErrors: 1, transmitErrors: 2, receiveDropped: 3, transmitDropped: 4, message: "" }))),
  gpu: region([{ id: "gpu0", name: "NVIDIA Test", usagePercent: 45, memoryTotalBytes: 8 * 1024 ** 3, memoryUsedBytes: 4 * 1024 ** 3, temperatureCelsius: 50, powerWatts: 100, message: "" }, { id: "gpu1", name: "NVIDIA Other", usagePercent: 10, memoryTotalBytes: null, memoryUsedBytes: null, temperatureCelsius: null, powerWatts: null, message: "部分字段不支持" }]),
  processes: { ...region({ cpu: [{ pid: 123, name: "worker-cpu", cpuPercent: 35, memoryBytes: 1024 ** 3 }], memory: [{ pid: 456, name: "worker-memory", cpuPercent: 1, memoryBytes: 2 * 1024 ** 3 }] }), partial: true, message: "部分进程读取权限不足" },
  warnings: [{ resource: "filesystem", device: "root", since: collectedAt, message: "根文件系统占用超过90%" }, { resource: "cpu", device: "", since: collectedAt, message: "占用持续超过90%" }, { resource: "memory", device: "", since: collectedAt, message: "占用持续超过90%" }],
});
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", String(port), "--strictPort"], { cwd: new URL("..", import.meta.url), stdio: ["ignore", "pipe", "pipe"] });
let viteOutput = "";
vite.stderr.on("data", (chunk) => { viteOutput += chunk; });
vite.stdout.on("data", (chunk) => { viteOutput += chunk; });
async function waitForServer() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try { if ((await fetch(baseUrl)).ok) return; } catch { /* Wait for startup. */ }
    await delay(250);
  }
  throw new Error(`Vite did not start: ${viteOutput}`);
}
async function eventually(check) {
  for (let attempt = 0; attempt < 100; attempt += 1) { if (await check()) return; await delay(20); }
  throw new Error("Expected monitoring state did not arrive");
}

try {
  await waitForServer();
  const browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  try {
    let role = "ADMIN";
    let data = fixture();
    let forbidden = false;
    let disabled = false;
    let emptyHistory = false;
    let overviewCount = 0;
    const historyRequests = [];
    const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
    const browserErrors = [];
    page.on("pageerror", (error) => { browserErrors.push(error.message); console.error(error.message); });
    await page.clock.setFixedTime(new Date(fixedTime));
    await page.addInitScript(() => localStorage.setItem("react-admin-auth", JSON.stringify({ token: "test-token", user: { id: 1, username: "cached-admin", nickname: "缓存管理员", roles: [{ id: 1, roleName: "ADMIN", roleCode: "ADMIN" }] } })));
    await page.route("**/api/**", async (route) => {
      const url = new URL(route.request().url());
      if (!url.pathname.startsWith("/api/")) { await route.continue(); return; }
      let result;
      let status = 200;
      if (url.pathname === "/api/auth/me") result = user(role);
      else if (url.pathname === "/api/auth/menus") result = role === "ADMIN" && !disabled ? menu : [];
      else if (url.pathname.endsWith("unread-count")) result = 0;
      else if (url.pathname.endsWith("/announcement/page")) result = { records: [], total: 0 };
      else if (url.pathname.endsWith("/overview")) { overviewCount += 1; result = data; if (forbidden) status = 403; }
      else if (url.pathname.endsWith("/history")) {
        historyRequests.push({ resource: url.searchParams.get("resource"), device: url.searchParams.get("device") });
        result = { resource: url.searchParams.get("resource"), device: url.searchParams.get("device") ?? "", windowSeconds: 1800, points: emptyHistory ? [] : [0, 1, 2, 3].map((index) => ({ collectedAt: new Date(fixedTime - 15000 + index * 5000).toISOString(), values: { usagePercent: index === 1 ? null : 20 + index, memoryUsagePercent: 50, receiveBytesPerSecond: 1024 ** 2, transmitBytesPerSecond: 2 * 1024 ** 2, readBytesPerSecond: 1024 ** 2, writeBytesPerSecond: 2 * 1024 ** 2, readIops: 3, writeIops: 4, temperatureCelsius: 50, powerWatts: 100 } })) };
      } else result = [];
      if (disabled && (url.pathname.endsWith("/overview") || url.pathname.endsWith("/history"))) { status = 503; result = { reason: "monitoring_disabled" }; }
      await route.fulfill({ status, contentType: "application/json", body: JSON.stringify({ code: status, message: status === 503 ? "服务器监控模块未启用" : status === 403 ? "ADMIN role required" : "success", data: result }) });
    });
    await page.goto(`${baseUrl}/monitor/server`, { waitUntil: "networkidle" });
    await page.getByRole("link", { name: "服务器监控" }).waitFor();
    await page.getByText("spec64-host", { exact: true }).waitFor();
    await page.getByRole("alert").getByText("文件系统 · /（/dev/sda1）", { exact: true }).waitFor();
    await page.getByRole("alert").getByText("CPU", { exact: true }).waitFor();
    await page.getByRole("alert").getByText("内存", { exact: true }).waitFor();
    assert.equal(await page.getByText("native:host", { exact: false }).count(), 0, "internal collector identity should stay out of the page");
    await page.getByText("未配置 Swap", { exact: true }).waitFor();
    assert.equal(await page.getByRole("tab", { selected: true }).textContent(), "文件系统");
    const filesystem = page.getByRole("region", { name: "文件系统容量", exact: true });
    await filesystem.getByText(/^持久存储 3 项/).waitFor();
    assert.equal(await filesystem.getByText("/opt/data-bind", { exact: true }).isVisible(), false);
    await filesystem.getByText("/mnt/c", { exact: true }).waitFor();
    await filesystem.getByText("/run/media/user/disk", { exact: true }).waitFor();
    await filesystem.getByRole("checkbox", { name: "显示全部挂载点" }).check();
    await filesystem.getByText("/opt/data-bind", { exact: true }).waitFor();
    await filesystem.getByText("/run", { exact: true }).waitFor();
    await page.getByLabel("趋势资源").selectOption("filesystem");
    assert.equal(await page.getByLabel("趋势设备").locator("option").count(), 6);
    await filesystem.getByRole("checkbox", { name: "显示全部挂载点" }).uncheck();
    assert.equal(await page.getByLabel("趋势设备").locator("option").count(), 3);
    await page.getByLabel("趋势资源").selectOption("cpu");
    await page.getByRole("tab", { name: "进程", exact: true }).click();
    await page.getByText("worker-cpu", { exact: true }).waitFor();
    await page.getByText("部分进程读取权限不足", { exact: true }).waitFor();
    await page.getByRole("tab", { name: "文件系统", exact: true }).click();
    await page.getByRole("tab", { name: "文件系统", exact: true }).press("ArrowRight");
    assert.equal(await page.getByRole("tab", { name: "块设备 I/O", exact: true }).getAttribute("aria-selected"), "true");
    await page.getByRole("tab", { name: "块设备 I/O", exact: true }).press("Home");
    assert.equal(await page.getByRole("tabpanel").count(), 1, "only the active resource panel should be visible");
    data.memory.data = { ...data.memory.data, swapTotalBytes: 8 * 1024 ** 3, swapUsedBytes: 0, swapUsagePercent: 0 };
    await page.getByRole("button", { name: "刷新", exact: true }).click();
    await page.getByRole("meter", { name: "Swap 占用率", exact: true }).waitFor();
    const cpuSection = page.getByRole("region", { name: "CPU", exact: true });
    const memorySection = page.getByRole("region", { name: "内存与 Swap", exact: true });
    assert.equal(await cpuSection.getByRole("meter").count(), 21, "all 20 cores must remain visible");
    assert.ok(Math.abs((await cpuSection.boundingBox()).height - (await memorySection.boundingBox()).height) < 2, "CPU and memory cards should align");
    await page.screenshot({ path: "/tmp/spec64-server-monitoring-desktop.png", fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: "/tmp/spec64-server-monitoring-mobile.png", fullPage: true, animations: "disabled" });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true, "mobile page must keep tables inside scroll containers");
    await page.setViewportSize({ width: 1440, height: 1000 });
    assert.equal(await page.locator('path[data-series="usagePercent"]').count(), 2, "null history sample must split the curve");
    await page.getByRole("tab", { name: "网卡", exact: true }).click();
    const network = page.getByRole("region", { name: "逐网卡流量", exact: true });
    assert.equal(await network.getByText("veth0", { exact: true }).count(), 0);
    await network.getByRole("checkbox").check();
    await network.getByText("veth0", { exact: true }).waitFor();
    await page.clock.install({ time: new Date(fixedTime) });
    const ipDialog = page.getByRole("dialog", { name: "网卡 IP 地址" });
    const openIp = (id) => network.getByRole("row").filter({ has: page.getByText(id, { exact: true }) }).getByRole("button", { name: "查看 IP" }).click();
    await openIp("eth0");
    await ipDialog.getByText("192.168.1.10/24", { exact: true }).waitFor();
    await ipDialog.getByText("10.0.0.5/8", { exact: true }).waitFor();
    await ipDialog.getByText("fe80::1/64", { exact: true }).waitFor();
    await page.screenshot({ path: "/tmp/spec77-network-ip-dialog.png", fullPage: true, animations: "disabled" });
    data.network.data[0].addresses = [{ family: "IPv4", ip: "10.0.0.7", prefixLength: 16 }];
    await page.clock.fastForward(5000);
    await ipDialog.getByText("10.0.0.7/16", { exact: true }).waitFor();
    assert.equal(await ipDialog.getByText("192.168.1.10/24", { exact: true }).count(), 0);
    data.network.data[0].addressError = "IP 地址读取失败：权限不足";
    await page.clock.fastForward(5000);
    await ipDialog.getByText("IP 地址读取失败：权限不足", { exact: true }).waitFor();
    assert.equal(await ipDialog.getByText("未分配 IP", { exact: true }).count(), 0);
    data.network.data = data.network.data.filter((item) => item.id !== "eth0");
    await page.clock.fastForward(5000);
    await ipDialog.getByText("接口已不可用", { exact: true }).waitFor();
    await ipDialog.getByRole("button", { name: "关闭详情弹窗" }).click();
    await openIp("eth1");
    await ipDialog.getByText("未分配 IP", { exact: true }).waitFor();
    await page.keyboard.press("Escape");
    data.network.data[0].addresses = undefined;
    await page.clock.fastForward(5000);
    await openIp("eth1");
    await ipDialog.getByText("IP 地址不可用：采集端未提供地址信息", { exact: true }).waitFor();
    await page.keyboard.press("Escape");
    data = fixture();
    data.host.data.os = "Microsoft Windows Server 2022";
    data.cpu.data.load1 = null; data.cpu.data.load5 = null; data.cpu.data.load15 = null;
    data.memory.data.swapUnsupported = true;
    await page.clock.fastForward(5000);
    await page.getByText("Microsoft Windows Server 2022", { exact: true }).waitFor();
    await page.getByText("系统负载（当前平台不支持）", { exact: true }).waitFor();
    await page.getByText("当前平台不支持此指标", { exact: true }).waitFor();
    await page.getByLabel("趋势资源").selectOption("network");
    await page.getByLabel("趋势设备").selectOption("eth1");
    await eventually(() => historyRequests.some((request) => request.resource === "network" && request.device === "eth1"));
    await page.getByLabel("趋势资源").selectOption("gpu");
    await page.getByLabel("趋势设备").selectOption("gpu1");
    await eventually(() => historyRequests.some((request) => request.resource === "gpu" && request.device === "gpu1"));
    assert.equal(await page.getByRole("img", { name: "最近30分钟资源趋势，采集缺口不连接" }).count(), 3);
    await page.clock.setSystemTime(new Date(fixedTime + 16000));
    await page.clock.runFor(1000);
    await eventually(() => page.getByText(/超过15秒无有效新样本/).count().then((count) => count > 0));
    data = fixture(); data.gpu = region(null, "unsupported", "当前平台不支持 NVIDIA"); data.disks = region(null, "no_device"); data.network = region(data.network.data, "error", "网卡采集失败，保留旧值");
    data.network.collectedAt = await page.evaluate(() => new Date().toISOString());
    data.sampledAt = "0001-01-01T00:00:00Z";
    await page.getByRole("button", { name: "刷新", exact: true }).click();
    await page.getByRole("tab", { name: "GPU", exact: true }).click();
    await page.getByText("当前平台不支持 NVIDIA", { exact: true }).waitFor();
    await page.getByRole("tab", { name: "块设备 I/O", exact: true }).click();
    await page.getByRole("region", { name: "块设备 I/O", exact: true }).getByText(/^无设备/).waitFor();
    await page.getByRole("tab", { name: "网卡", exact: true }).click();
    await page.getByText("网卡采集失败，保留旧值", { exact: true }).waitFor();
    await network.getByText(/保留上次有效值/).waitFor();
    await page.getByText("样本时间：采集中", { exact: true }).waitFor();
    forbidden = true;
    await page.clock.fastForward(5000);
    await page.getByText("无权访问服务器监控", { exact: true }).waitFor();
    assert.equal(await page.getByText("spec64-host", { exact: true }).count(), 0, "403 must clear host details");
    forbidden = false; role = "USER";
    const countBeforeOrdinary = overviewCount;
    await page.reload({ waitUntil: "networkidle" });
    await page.getByText("无权访问服务器监控", { exact: true }).waitFor();
    assert.equal(overviewCount, countBeforeOrdinary, "cached ADMIN must not override fresh ordinary role");
    assert.equal(await page.getByRole("link", { name: "服务器监控" }).count(), 0);
    role = "ADMIN"; forbidden = false; data = fixture();
    await page.reload({ waitUntil: "networkidle" });
    await page.getByText("spec64-host", { exact: true }).waitFor();
    disabled = true;
    await page.clock.fastForward(5000);
    await page.getByText("服务器监控模块未启用", { exact: true }).waitFor();
    assert.equal(await page.getByText("spec64-host", { exact: true }).count(), 0, "disabled state must clear old host details");
    const beforeDisabledWait = overviewCount;
    await page.clock.fastForward(15000);
    assert.equal(overviewCount, beforeDisabledWait, "disabled module must stop polling");
    await page.reload({ waitUntil: "networkidle" });
    await page.getByText("服务器监控模块未启用", { exact: true }).waitFor();
    assert.equal(await page.getByRole("link", { name: "服务器监控" }).count(), 0, "disabled menu must stay hidden on direct route access");
    role = "ADMIN"; disabled = false; data = fixture(); data.cpu = region({ ...data.cpu.data, usagePercent: null, perCore: [null, 20] }, "collecting", "首样本正在建立基线"); data.disks = region([{ ...data.disks.data[0], readBytesPerSecond: null, writeBytesPerSecond: null, readIops: null, writeIops: null, message: "建立速率基线" }], "collecting"); emptyHistory = true;
    await page.reload({ waitUntil: "networkidle" });
    await page.getByText("首样本正在建立基线", { exact: true }).waitFor();
    await page.getByText("逻辑核 0", { exact: true }).locator("..").getByText("采集中", { exact: true }).waitFor();
    await page.getByText("采集中，暂无历史样本", { exact: true }).waitFor();
    await page.getByRole("link", { name: "工作台", exact: true }).click();
    await page.getByRole("heading", { name: "工作台", exact: true }).waitFor();
    const beforeUnmount = overviewCount;
    const historyBeforeUnmount = historyRequests.length;
    await page.clock.fastForward(20000);
    await delay(100);
    assert.equal(overviewCount, beforeUnmount, "unmount must stop overview polling");
    assert.equal(historyRequests.length, historyBeforeUnmount, "unmount must stop history polling");
    assert.deepEqual(browserErrors, [], "page must not throw runtime errors");
    console.log("server monitoring browser checks passed");
  } finally { await browser.close(); }
} finally { vite.kill("SIGTERM"); }
