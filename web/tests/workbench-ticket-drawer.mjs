import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4195';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4195', '--strictPort'], { cwd: new URL('..', import.meta.url), env: process.env, stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 45_000);
try {
  for (let n = 0; n < 60; n++) {
    try { if ((await fetch(`${base}/tests/workbench-ticket-drawer.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const requests = [];
  page.on('request', request => {
    if (request.url().includes('/api/weighing/tickets')) requests.push(new URL(request.url()));
  });

  const tickets = Array.from({ length: 10 }, (_, index) => ({
    id: index + 1,
    taskId: index + 1,
    ticketNo: `WB20261027${String(index + 1).padStart(4, '0')}`,
    revision: 1,
    business: {
      plate: `晋A${String(12345 + index).padStart(5, '0')}`,
      direction: index % 2 ? 'outbound' : 'inbound',
      materialId: 1,
      materialName: '高粱',
      partyId: 1,
      partyName: index % 2 ? '贵州红缨子农业发展有限公司' : '仁怀市酒业有限公司',
      driver: '杨师傅',
      remark: '',
      materialSpec: '散装',
      materialCode: 'M-01',
    },
    basis: [],
    previousTicketId: null,
    status: index === 8 ? 'void' : 'valid',
    issuedAt: `2026-10-27T0${3 - Math.floor(index / 6)}:${String(55 - index * 4).padStart(2, '0')}:00Z`,
    issuerName: 'rzp',
    originalGross: '30.370',
    originalTare: '12.880',
    finalGross: '30.370',
    finalTare: '12.880',
    finalNet: '17.490',
    grossSource: 'collected',
    tareSource: 'collected',
    adjustmentRemark: '',
    voidedAt: null,
    voidReason: '',
    printStatus: index % 3 === 0 ? 'unprinted' : 'printed',
    reprintCount: index % 2,
    lastPrintAttemptId: null,
    siteCode: 'site',
    siteName: '示范厂区',
  }));
  await page.route('**/api/weighing/tickets**', async route => {
    const url = new URL(route.request().url());
    const data = url.pathname.endsWith('/summary')
      ? { taskCount: 9, gross: '273.33', tare: '115.92', net: '157.41', rows: [] }
      : { records: tickets, total: 128, page: Number(url.searchParams.get('page') ?? 1), pageSize: Number(url.searchParams.get('pageSize') ?? 10) };
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ code: 200, message: 'ok', data }) });
  });

  await page.goto(`${base}/tests/workbench-ticket-drawer.html?scale=scale`, { waitUntil: 'networkidle' });
  const ticketButton = page.getByRole('button', { name: '过磅单', exact: true });
  const managementLink = page.getByRole('link', { name: '后台查询', exact: true });
  await ticketButton.waitFor();
  const ticketBox = await ticketButton.boundingBox();
  const managementBox = await managementLink.boundingBox();
  assert.ok(ticketBox && managementBox && ticketBox.x < managementBox.x, '过磅单按钮应在后台查询左侧');

  await page.screenshot({ path: '/tmp/opencode/ticket-drawer-entry-final.png' });
  await ticketButton.click();
  const dialog = page.getByRole('dialog', { name: '过磅单列表', exact: true });
  await dialog.waitFor();
  await dialog.getByText('WB202610270001', { exact: true }).waitFor();
  assert.equal(await dialog.getByText('按出单时间倒序 · 当前厂区：示范厂区', { exact: true }).count(), 0);
  await dialog.getByText('当前厂区 · 示范厂区', { exact: true }).waitFor();
  await dialog.getByText('共 128 张 · 有效 9 张 · 最终净重 157.41 吨', { exact: true }).waitFor();
  const fromBox = await dialog.getByLabel('出单开始日期').boundingBox();
  const toBox = await dialog.getByLabel('出单结束日期').boundingBox();
  assert.ok(fromBox && toBox && Math.abs(fromBox.y - toBox.y) < 2, '开始和结束日期应在同一行');
  const clearBox = await dialog.getByRole('button', { name: '清空筛选', exact: true }).boundingBox();
  const refreshBox = await dialog.getByRole('button', { name: '刷新', exact: true }).boundingBox();
  assert.ok(clearBox && refreshBox && Math.abs(clearBox.y - refreshBox.y) < 2, '清空筛选与刷新应在同一操作组');
  await page.screenshot({ path: '/tmp/opencode/ticket-drawer-final.png' });

  const listRequest = requests.find(request => request.pathname === '/api/weighing/tickets');
  assert.equal(listRequest?.searchParams.get('siteCode'), null);
  assert.equal(listRequest?.searchParams.get('page'), '1');
  assert.equal(listRequest?.searchParams.get('pageSize'), '10');
  assert.equal(await dialog.getByRole('link', { name: '查看单据 WB202610270001' }).getAttribute('target'), 'weighbridge-management');
  assert.equal(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth), true);

  await dialog.getByRole('button', { name: '下一页', exact: true }).click();
  await page.waitForTimeout(200);
  const nextPageRequest = requests.filter(request => request.pathname === '/api/weighing/tickets').at(-1);
  assert.equal(nextPageRequest?.searchParams.get('page'), '2');

  await dialog.getByRole('button', { name: '近 7 天', exact: true }).click();
  await page.waitForTimeout(250);
  const dateRequest = requests.filter(request => request.pathname === '/api/weighing/tickets').at(-1);
  assert.ok(dateRequest?.searchParams.get('from'));
  assert.ok(dateRequest?.searchParams.get('to'));
  assert.equal(dateRequest?.searchParams.get('siteCode'), null);
  assert.equal(dateRequest?.searchParams.get('page'), '1');

  await dialog.getByRole('button', { name: '清空筛选', exact: true }).click();
  await page.waitForTimeout(250);
  const clearedRequest = requests.filter(request => request.pathname === '/api/weighing/tickets').at(-1);
  assert.equal(clearedRequest?.searchParams.get('siteCode'), null);
  assert.equal(clearedRequest?.searchParams.get('from'), null);

  await page.setViewportSize({ width: 900, height: 768 });
  await page.waitForTimeout(150);
  assert.equal(Math.round(await dialog.evaluate(element => element.getBoundingClientRect().width)), 900);

  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'detached' });
  assert.equal(await ticketButton.getAttribute('aria-expanded'), 'false');
  assert.equal(await ticketButton.evaluate(element => element === document.activeElement), true, '关闭后焦点应回到过磅单按钮');
  assert.deepEqual(errors, []);
  console.log('工作台过磅单抽屉：入口位置、当前厂区、倒序列表、筛选、分页、详情跳转、关闭与焦点恢复通过。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
