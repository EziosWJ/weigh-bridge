import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4199';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4199', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 120_000);
try {
  for (let n = 0; n < 80; n++) {
    try { if ((await fetch(`${base}/tests/dashboard.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1366, height: 1000 } });
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  let scenario = 'normal';
  let permissions = ['weighing:read', 'agent:maintenance:view', 'weighing:camera:view'];
  const requests = [];
  const writes = [];
  const business = () => ({
    date: '2026-10-26', checkedAt: '2026-10-26T02:30:00Z', validTickets: 3,
    inbound: { taskCount: 2, net: '20.125' }, outbound: { taskCount: 1, net: '10' },
    awaitingCount: 7, overnightCount: 2,
    awaitingTasks: [{ id: 1, firstConfirmedAt: '2026-10-25T10:00:00Z', business: { plate: '川A12345', direction: 'inbound', materialName: '高粱', materialSpec: '红高粱' } }],
    recentTickets: [{ id: 8, ticketNo: 'WB202610260008', business: { plate: '川A54321', direction: 'outbound', materialName: '酒糟', materialSpec: '' }, finalNet: '10', issuedAt: '2026-10-26T02:00:00Z' }],
  });
  await page.route('**/api/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    requests.push(path);
    if (request.method() !== 'GET') writes.push(request.method() + ' ' + path);
    const data = value => route.fulfill({ json: { code: 200, message: 'success', data: value } });
    if (path === '/api/weighing/capabilities') {
      if (scenario === 'caps-failure') return route.fulfill({ status: 500, json: { code: 500, message: '无法读取权限' } });
      return data({ permissionCodes: permissions, localSiteCode: 'F001', localScaleCode: 'WB01', configured: true, initialized: true, maintenanceEnabled: true, scales: [{ code: 'WB01', siteCode: 'F001', enabled: 1, canOperate: true }], sites: [], userId: 1 });
    }
    if (path === '/api/weighing/overview') {
      if (scenario === 'business-failure') return route.fulfill({ status: 500, json: { code: 500, message: '统计查询失败' } });
      if (scenario === 'empty') return data({ ...business(), validTickets: 0, inbound: { taskCount: 0, net: '0' }, outbound: { taskCount: 0, net: '0' }, awaitingCount: 0, overnightCount: 0, awaitingTasks: [], recentTickets: [] });
      if (scenario === 'many') return data({ ...business(), awaitingTasks: Array.from({ length: 5 }, (_, index) => ({ ...business().awaitingTasks[0], id: index + 1 })) });
      return data(business());
    }
    if (path === '/api/weighing/scales/WB01/device') {
      if (scenario === 'device-failure') return route.fulfill({ status: 503, json: { code: 503, message: '设备状态读取失败' } });
      return data({ online: scenario !== 'offline', deviceOk: true, stable: true, receivedAt: '2026-10-26T02:30:00Z', captureRequired: scenario !== 'optional' });
    }
    if (path === '/api/system/agent-maintenance/status') return route.fulfill({ json: scenario === 'offline'
      ? { agentProcessOnline: false, maintenanceAvailable: false, lastError: 'Agent 离线' }
      : { agentProcessOnline: true, agentKnown: true, agent: '运行中', cupsKnown: true, cups: '运行中', queueKnown: true, queueState: '已启用', queue: '五联打印机', jobsKnown: false, checkedAt: '2026-10-26T02:30:00Z' } });
    if (path === '/api/weighing/cameras') return data({ cameras: scenario === 'no-cameras' ? [] : [{ id: 1, name: '磅房全景', enabled: true, captureEnabled: true, stream: { state: 'error' } }], video: { syncState: 'error', whepBaseUrl: 'http://video.local', updateTime: '2026-10-26T02:30:00Z' } });
    throw new Error('Unexpected request ' + path);
  });
  const ready = async () => {
    await page.goto(`${base}/tests/dashboard.html`, { waitUntil: 'networkidle' });
    await page.getByRole('button', { name: '刷新', exact: true }).waitFor();
  };
  await ready();
  assert.equal(await page.locator('h1').textContent(), '系统概览');
  assert.ok(await page.locator('h1').evaluate(node => node.getBoundingClientRect().height <= 1), 'page heading should be visually hidden');
  assert.equal(await page.getByText('本厂今日业务、待回磅任务与设备最近状态。', { exact: true }).count(), 0);
  assert.equal(await page.getByText(/统计日期：|业务最近更新：/).count(), 0);
  assert.ok((await page.locator('main').innerText()).includes('20.125'));
  assert.equal(await page.getByText('隔日', { exact: true }).count(), 1);
  assert.equal(await page.getByRole('link', { name: '查看任务', exact: true }).getAttribute('href'), '/weighing/tasks/1');
  assert.equal(await page.getByRole('link', { name: '查看单据', exact: true }).getAttribute('href'), '/weighing/tickets/8');
  await page.getByText('1 台参与抓拍，连通性未检测').waitFor();
  await page.getByText('同步失败', { exact: true }).waitFor();
  await page.getByText('作业状态待核实', { exact: true }).waitFor();
  assert.equal(await page.getByRole('link', { name: '设备维护', exact: true }).count(), 0, 'maintenance shortcut should be removed');
  assert.equal(await page.getByRole('link', { name: '摄像头管理', exact: true }).count(), 0, 'camera shortcut should be removed');
  const alignedCards = async () => {
    const task = await page.locator('section').filter({ has: page.getByRole('heading', { name: '待回磅任务', exact: true }) }).boundingBox();
    const device = await page.locator('.dashboard-devices').boundingBox();
    assert.ok(Math.abs(task.y - device.y) < 1 && Math.abs(task.height - device.height) < 1, 'adjacent cards should align at top and bottom');
  };
  assert.ok(await page.locator('.dashboard-devices').evaluate(node => node.getBoundingClientRect().height < 440), 'device summary should remain compact');
  assert.equal(await page.getByText('今日访问', { exact: true }).count(), 0);
  for (const width of [1864, 1366, 1024, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `viewport overflow at ${width}`);
    const columns = await page.locator('.dashboard-device-grid').evaluate(node => getComputedStyle(node).gridTemplateColumns.split(' ').length);
    assert.equal(columns, width === 1024 ? 4 : 2, `device grid did not adapt to available card width at ${width}`);
    if (width >= 1280) await alignedCards();
  }
  await page.setViewportSize({ width: 1366, height: 1000 });
  await page.screenshot({ path: '/tmp/weighbridge-dashboard.png', fullPage: true });

  scenario = 'business-failure';
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByText(/业务概览读取失败/).waitFor();
  assert.equal(await page.getByText('20.125', { exact: true }).count(), 0, 'failed refresh retained stale totals');
  assert.equal(await page.getByText('暂无待回磅任务', { exact: true }).count(), 0, 'failure rendered as empty data');
  await page.getByText('采集中', { exact: true }).waitFor();

  scenario = 'empty'; await ready();
  await page.getByText('暂无待回磅任务', { exact: true }).waitFor();
  await page.getByText('暂无有效单据', { exact: true }).waitFor();
  await alignedCards();
  await page.screenshot({ path: '/tmp/weighbridge-dashboard-empty.png', fullPage: true });
  scenario = 'many'; await ready();
  await alignedCards();
  scenario = 'offline'; await ready();
  await page.getByText('离线或数据过期', { exact: true }).waitFor();
  await page.getByText('维护连接不可用', { exact: true }).waitFor();
  scenario = 'device-failure'; await ready();
  await page.getByText('设备状态读取失败', { exact: true }).waitFor();
  await page.getByText('策略未知', { exact: true }).waitFor();
  scenario = 'no-cameras'; await ready();
  await page.getByText('未配置抓拍摄像头', { exact: true }).waitFor();
  scenario = 'optional'; await ready();
  await page.getByText('强制抓拍关闭', { exact: true }).waitFor();

  scenario = 'normal'; permissions = ['weighing:read']; requests.length = 0; await ready();
  assert.equal(requests.some(path => path.includes('/agent-maintenance/') || path.includes('/cameras')), false, 'read-only business user requested maintenance data');
  assert.equal(await page.getByRole('link', { name: '设备维护', exact: true }).count(), 0);
  permissions = ['weighing:camera:view']; requests.length = 0; await ready();
  assert.equal(requests.includes('/api/weighing/overview'), false);
  assert.equal(await page.getByText('今日有效单', { exact: true }).count(), 0);
  permissions = []; requests.length = 0; await ready();
  await page.getByText('暂无可查看的概览内容', { exact: true }).waitFor();
  assert.deepEqual(requests, ['/api/weighing/capabilities']);
  scenario = 'caps-failure'; await ready();
  await page.getByText(/权限与本厂信息读取失败/).waitFor();
  assert.deepEqual(writes, [], 'overview performed a write');
  assert.deepEqual(errors, []);
  console.log('dashboard passed: data, empty/error, offline, capture policy, permissions, links, responsive layout, GET-only refresh');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
