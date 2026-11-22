import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const listener = createServer();
await new Promise((resolve, reject) => { listener.once('error', reject); listener.listen(0, '127.0.0.1', resolve); });
const port = listener.address().port;
await new Promise(resolve => listener.close(resolve));
const base = `http://127.0.0.1:${port}`;
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], { cwd: new URL('..', import.meta.url), env: process.env, stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 45_000);
try {
  for (let n = 0; n < 80; n++) {
    try { if ((await fetch(`${base}/tests/sync-detail.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const time = '2026-11-01T00:00:00Z';
  const digest = 'a'.repeat(64);
  const oldReason = '出单时的厂区编码已更换，已暂停上传，请联系管理员处理';
  let conclusion = '';
  let rechecks = 0;
  let enabled = true;
  let bound = true;
  let lastSuccess = time;
  let attempted = false;
  const record = (id, ticketNo, syncStatus, syncReason = '') => ({
    id, ticketNo, status: 'valid', syncProtocol: 6, syncStatus, syncReason, issuedAt: time,
    business: { plate: '晋A12345', direction: 'inbound', materialName: '高粱', partyName: '供货方' },
  });
  const records = [
    record(42, 'F001-TEST', 'conflict'),
    record(43, 'F001-ARCHIVE', 'synced'),
    record(44, 'F001-RETRY', 'temporary_failure', '无法连接云端'),
    record(45, 'F001-WAIT', 'waiting_predecessor'),
    record(46, 'OLD-PAUSED', 'needs_admin', oldReason),
  ];
  let logs = [
    { id: 10, attemptedAt: time, syncType: 'issue', ticketNo: 'F001-TEST', digest, success: true },
    { id: 11, attemptedAt: time, syncType: 'task', taskNo: 'RW-0123', digest: '', success: false, reason: '任务上传未获确认，将由系统重试' },
    { id: 12, attemptedAt: time, syncType: 'temp_party', temporaryCode: 'T-P-0123', digest: '', success: true },
  ];
  const status = () => ({
    enabled, bound, isAdmin: true, lastSuccess,
    lastAttempt: attempted ? { attemptedAt: time, success: false, reason: '无法连接云端' } : null,
  });
  const detail = () => ({
    ticket: { id: 42, ticketNo: 'F001-TEST', siteCode: 'F001', issuedAt: time, status: 'valid', syncProtocol: 4, business: { plate: '晋A12345', materialName: '测试物料', partyName: '测试供货方' } },
    changes: [{ id: 1, protocol: 4, digest: 'a'.repeat(64), changeType: 'issue', status: 'conflict', attempts: 1, createdAt: '2026-11-01T00:00:00Z' }],
    legacyChanges: [], predecessor: null, successor: null,
    reconcile: { status: rechecks ? 'consistent' : 'conflict', cloudFound: true, cloudStatus: 'valid', localLatestDigest: 'a'.repeat(64), cloudLatestDigest: 'a'.repeat(64), cloudLatestDigestVersion: 2, cloudLatestChangeType: 'issue', cloudReceivedAt: '2026-11-01T00:00:00Z', checkedAt: '2026-11-01T00:00:00Z', triggerType: 'manual', detail: { fieldDifferences: [{ field: 'adjustedBy', local: null, cloud: '' }, { field: 'netAdjustmentRateBp', local: 0, cloud: 2 }, { field: 'finalNet', local: '1.2', cloud: '1.3' }, { field: 'direction', local: 'inbound', cloud: 'outbound' }] } },
    reviews: [{ id: 7, digest: 'a'.repeat(64), reasonCode: 'snapshot_mismatch', status: rechecks ? 'resolved' : 'open', blocking: !rechecks, detail: {}, conclusion, concludedByName: '', concludedAt: null, lastCheckedAt: '2026-11-01T00:00:00Z', createdAt: '2026-11-01T00:00:00Z' }],
  });
  await page.route('**/api/weighing/**', async route => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    let data;
    if (path.endsWith('/capabilities')) data = { permissionCodes: ['weighing:sync:manage'], masterSyncEnabled: enabled, masterSyncBound: bound };
    else if (path.endsWith('/sync/tickets/42')) data = detail();
    else if (path.endsWith('/sync/tickets/46')) data = {
      ...detail(), ticket: records[4], reconcile: null, reviews: [],
      changes: [
        { id: 5, protocol: 6, status: 'synced', changeType: 'issue', syncedAt: time, attempts: 1, digest, siteCode: 'OLD' },
        { id: 6, protocol: 6, status: 'needs_admin', changeType: 'void', reasonCode: 'old_site_code', reason: oldReason, digest, siteCode: 'OLD' },
      ],
    };
    else if (path.endsWith('/sync/reviews/7/conclusion')) { conclusion = request.postDataJSON().conclusion; data = null; }
    else if (path.endsWith('/sync/reviews/7/recheck')) {
      assert.ok(conclusion);
      rechecks++;
      data = { status: 'consistent', message: '一致' };
    }
    else if (path.endsWith('/sync/overview')) data = { enabled, bound, pendingTickets: 4, needsActionTickets: 2, lastSuccess: time };
    else if (path.endsWith('/sync/tickets')) {
      const filter = url.searchParams.get('syncStatus');
      const rows = filter === 'old_site_code' ? [records[4]] : records;
      data = { total: rows.length, records: rows };
    }
    else if (path.endsWith('/master-sync/status')) data = status();
    else if (path.endsWith('/sync/connection')) data = {
      ...status(), address: 'http://cloud.test', siteCode: 'F001',
      readiness: { status: enabled ? (attempted ? 'unreachable' : 'ready') : 'disabled', checkedAt: enabled ? time : null },
    };
    else if (path.endsWith('/sync/logs')) {
      if (request.method() === 'DELETE') {
        assert.equal(url.searchParams.get('throughId'), '12');
        assert.equal(url.searchParams.get('count'), '3');
        logs = [];
        data = null;
      } else data = logs;
    }
    else throw new Error(`unexpected request: ${path}`);
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ code: 200, message: 'success', data }) });
  });

  await page.goto(`${base}/tests/sync-detail.html`, { waitUntil: 'networkidle' });
  const differences = page.getByRole('table', { name: '单据字段差异' });
  const adjustment = differences.getByRole('row').filter({ hasText: '调整人' }).locator('td');
  assert.equal((await adjustment.nth(0).textContent()).trim(), '未提供');
  assert.equal((await adjustment.nth(1).textContent()).trim(), '空字符串');
  const rate = differences.getByRole('row').filter({ hasText: '净重调整比例' }).locator('td');
  assert.equal((await rate.nth(0).textContent()).trim(), '0%');
  assert.equal((await rate.nth(1).textContent()).trim(), '0.02%');
  await differences.getByText('1.2 吨', { exact: true }).waitFor();
  await differences.getByText('出厂装货', { exact: true }).waitFor();
  await page.getByText('本厂管理员', { exact: true }).waitFor();
  const recheck = page.getByRole('button', { name: '重新核查', exact: true });
  assert.equal(await recheck.isDisabled(), true);
  await page.getByRole('button', { name: '记录核查结论', exact: true }).click();
  await page.getByRole('textbox', { name: /核查结论/ }).fill('测试已核对');
  await page.getByRole('button', { name: '保存结论', exact: true }).click();
  await page.getByText('测试已核对', { exact: true }).waitFor();
  assert.equal(await recheck.isEnabled(), true);
  await page.getByRole('button', { name: '更新核查结论' }).click();
  await page.getByRole('button', { name: '保存并重新核查' }).click();
  await page.getByText('已结束', { exact: true }).waitFor();
  assert.equal(rechecks, 1);

  const open = name => page.goto(`${base}/tests/sync-detail.html?page=${name}`, { waitUntil: 'domcontentloaded' });
  const rows = () => page.getByRole('region', { name: '单据同步列表' });
  await open('tickets');
  await rows().getByText('待自动重试', { exact: true }).waitFor();
  await rows().getByText('等待前序', { exact: true }).waitFor();
  await rows().getByText('旧编码暂停', { exact: true }).waitFor();
  await page.getByRole('combobox', { name: '同步状态' }).selectOption('old_site_code');
  await rows().getByText('OLD-PAUSED', { exact: true }).waitFor();
  await rows().getByText('F001-ARCHIVE', { exact: true }).waitFor({ state: 'hidden' });
  await rows().getByRole('link', { name: '处理' }).click();
  await page.getByRole('heading', { name: '单据同步详情' }).waitFor();
  await page.getByRole('region', { name: '变化与确认状态' }).getByText('已同步', { exact: true }).waitFor();
  assert.equal(await page.getByRole('button', { name: '立即重试' }).count(), 0);
  const hint = page.getByRole('button', { name: '历史确认查询规则' });
  await hint.focus();
  await page.getByRole('tooltip').filter({ hasText: '保存的变化摘要' }).waitFor();

  enabled = false;
  await open('tickets');
  await rows().getByText('已同步', { exact: true }).waitFor();
  assert.equal(await rows().getByText('同步已关闭', { exact: true }).count(), 4);
  assert.equal(await page.getByRole('button', { name: '按日期批量对账' }).isDisabled(), true);
  await open('master');
  await page.getByText('单机模式', { exact: true }).waitFor();
  enabled = true;
  bound = false;
  lastSuccess = null;
  await open('master');
  await page.getByText('待首次同步', { exact: true }).waitFor();
  lastSuccess = time;
  bound = true;
  attempted = true;
  await open('master');
  await page.getByText('沿用上次成功快照', { exact: true }).waitFor();
  await open('connection');
  await page.getByText('无法连接云端', { exact: true }).first().waitFor();
  await page.getByText('沿用上次成功快照', { exact: true }).waitFor();

  await open('logs');
  const filter = page.getByRole('textbox', { name: '单号、任务号或临时主体编码' });
  for (const code of ['f001-test', 'rw-0123', 't-p-0123']) {
    await filter.fill(code);
    await page.getByText('共 1 条', { exact: true }).waitFor();
  }
  await page.getByRole('button', { name: '清空全部日志' }).click();
  const dialog = page.getByRole('dialog', { name: '清空普通同步日志' });
  await dialog.getByText('永久删除已读取的 3 条普通同步日志，其中 2 条在当前筛选范围外。').waitFor();
  await dialog.getByRole('button', { name: '清空', exact: true }).click();
  await page.getByText('暂无同步日志', { exact: true }).waitFor();
  assert.deepEqual(errors, []);
  console.log('厂云同步通过：字段差异与直接重新核查、关闭后保留回执、断网与旧编码、资料状态、日志定位及清空范围。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
