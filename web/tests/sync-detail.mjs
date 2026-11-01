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
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  let conclusion = '';
  const detail = () => ({
    ticket: { id: 42, ticketNo: 'F001-TEST', siteCode: 'F001', status: 'valid', syncProtocol: 4, business: { plate: '晋A12345', materialName: '测试物料', partyName: '测试供货方' } },
    changes: [{ id: 1, protocol: 4, digest: 'a'.repeat(64), changeType: 'issue', status: 'conflict', attempts: 1, createdAt: '2026-11-01T00:00:00Z' }],
    legacyChanges: [], predecessor: null, successor: null,
    reconcile: { status: 'conflict', cloudFound: true, cloudStatus: 'valid', localLatestDigest: 'a'.repeat(64), cloudLatestDigest: 'a'.repeat(64), cloudLatestDigestVersion: 2, cloudLatestChangeType: 'issue', cloudReceivedAt: '2026-11-01T00:00:00Z', checkedAt: '2026-11-01T00:00:00Z', triggerType: 'manual', detail: { fieldDifferences: [{ field: 'adjustedBy', local: null, cloud: '' }, { field: 'netAdjustmentRateBp', local: 0, cloud: 2 }] } },
    reviews: [{ id: 7, digest: 'a'.repeat(64), reasonCode: 'snapshot_mismatch', status: 'open', blocking: true, detail: {}, conclusion, concludedByName: '', concludedAt: null, lastCheckedAt: '2026-11-01T00:00:00Z', createdAt: '2026-11-01T00:00:00Z' }],
  });
  await page.route('**/api/weighing/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let data;
    if (path.endsWith('/capabilities')) data = { permissionCodes: ['weighing:sync:manage'] };
    else if (path.endsWith('/sync/tickets/42')) data = detail();
    else if (path.endsWith('/sync/reviews/7/conclusion')) { conclusion = request.postDataJSON().conclusion; data = null; }
    else throw new Error(`unexpected request: ${path}`);
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ code: 200, message: 'success', data }) });
  });

  await page.goto(`${base}/tests/sync-detail.html`, { waitUntil: 'networkidle' });
  const differences = page.getByRole('table', { name: '单据字段差异' });
  const adjustment = differences.getByRole('row').filter({ hasText: '调整人' }).locator('td');
  assert.equal((await adjustment.nth(1).textContent()).trim(), '未提供');
  assert.equal((await adjustment.nth(2).textContent()).trim(), '空字符串');
  const rate = differences.getByRole('row').filter({ hasText: '净重调整比例' }).locator('td');
  assert.equal((await rate.nth(1).textContent()).trim(), '0');
  assert.equal((await rate.nth(2).textContent()).trim(), '2');
  const recheck = page.getByRole('button', { name: '重新核查', exact: true });
  assert.equal(await recheck.isDisabled(), true);
  await page.getByRole('button', { name: '记录核查结论', exact: true }).click();
  await page.getByRole('textbox', { name: /核查结论/ }).fill('测试已核对');
  await page.getByRole('button', { name: '保存结论', exact: true }).click();
  await page.getByText('核查结论：测试已核对', { exact: false }).waitFor();
  assert.equal(await recheck.isEnabled(), true);
  assert.deepEqual(errors, []);
  console.log('同步详情：空值与空字符串、数值差异及核查结论操作通过。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
