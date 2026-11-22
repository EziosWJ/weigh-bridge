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
    try { if ((await fetch(`${base}/tests/manual-supplement-entry.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route(url => url.pathname.startsWith('/api/'), route => route.abort());
  await page.route('**/api/system/dict/TRANSPORT_DIRECTION/items', route =>
    route.fulfill({ json: { code: 200, message: 'ok', data: [] } }),
  );

  const business = { plate: '晋A12345', direction: 'outbound', materialId: 11, materialName: '高粱', materialSpec: '散装', partyId: 21, partyName: '测试收货方', carrierId: null };
  const ok = (route, data) => route.fulfill({ json: { code: 200, data } });
  await page.route('**/api/weighing/materials?*', route => ok(route, { records: [{ id: 11, code: 'M-001', name: business.materialName, spec: business.materialSpec, enabled: 1 }], total: 1 }));
  await page.route('**/api/weighing/parties?*', route => ok(route, { records: [{ id: 21, code: 'P-001', name: business.partyName, nature: 'enterprise', roles: ['customer'], enabled: 1, contacts: [] }], total: 1 }));
  const issued = { id: 91, taskId: 51, status: 'valid', ticketNo: 'F001-20261122-0009', basis: [], business, issuedAt: '2026-11-22T01:00:00Z', finalGross: '28', finalTare: '10', finalNet: '17.5' };
  await page.route('**/api/weighing/tickets/91', route => ok(route, issued));
  const submissions = [];
  await page.route('**/api/weighing/manual-supplements', route => {
    submissions.push(route.request().postDataJSON());
    return ok(route, { taskId: 51, ticketId: 91, taskVersion: 1, status: 'completed' });
  });

  await page.goto(`${base}/tests/manual-supplement-entry.html?permission=yes`, { waitUntil: 'networkidle' });
  const button = page.getByRole('button', { name: '人工补单', exact: true });
  await button.waitFor();
  const box = await button.boundingBox();
  assert.ok(box && box.width > 0 && box.height > 0, '有补单权限时工作台顶栏应显示可见的人工补单按钮');
  const nav = page.getByRole('navigation', { name: '工作台操作' });
  assert.equal(await nav.getByRole('button', { name: '人工补单', exact: true }).count(), 1, '人工补单按钮应位于工作台顶栏操作区');
  await button.click();
  const dialog = page.getByRole('dialog');
  await dialog.getByText('人工补单', { exact: true }).first().waitFor();
  await dialog.getByRole('button', { name: '人工补单适用规则' }).focus();
  await page.getByRole('tooltip').filter({ hasText: '两次称重均已完成且系统没有称重记录时' }).waitFor();
  const confirmation = dialog.getByRole('region', { name: '出单核对' });
  assert.equal(await confirmation.getByLabel('首磅实际时间（北京时间）').inputValue(), '');
  assert.equal(await confirmation.getByLabel('回磅实际时间（北京时间）').inputValue(), '');
  await dialog.getByLabel('首磅毛重（吨）').fill('31');
  await dialog.getByLabel('回磅皮重（吨）').fill('12.5');
  await confirmation.getByText('18.5 吨', { exact: true }).waitFor();
  await dialog.getByRole('combobox', { name: '运输方向', exact: true }).selectOption('outbound');
  await dialog.getByLabel('首磅皮重（吨）').fill('10');
  await dialog.getByLabel('回磅毛重（吨）').fill('28');
  await confirmation.getByText('18 吨', { exact: true }).waitFor();
  await dialog.getByText('有扣减时填写', { exact: true }).click();
  await dialog.getByLabel('第 1 项扣减原因').selectOption('IMPURITY');
  await dialog.getByLabel('扣减重量（吨）').fill('0.5');
  await confirmation.getByText('17.5 吨', { exact: true }).waitFor();
  await dialog.getByRole('textbox', { name: '车牌号码' }).fill(business.plate);
  await dialog.getByRole('combobox', { name: '搜索物料' }).fill(business.materialName);
  await dialog.getByRole('option', { name: /高粱/ }).click();
  await dialog.getByRole('combobox', { name: '搜索收货方' }).fill(business.partyName);
  await dialog.getByRole('option', { name: /测试收货方/ }).click();
  await dialog.getByLabel('补单原因').selectOption({ label: '停电' });
  await confirmation.getByLabel('首磅实际时间（北京时间）').fill('2026-01-12T08:00');
  await confirmation.getByLabel('回磅实际时间（北京时间）').fill('2026-01-12T07:00');
  await dialog.getByRole('button', { name: '确认出单', exact: true }).click();
  await confirmation.getByRole('alert').getByText('首磅不能晚于回磅').waitFor();
  assert.equal(submissions.length, 0);
  await confirmation.getByLabel('回磅实际时间（北京时间）').fill('2026-01-12T09:00');
  await dialog.getByRole('button', { name: '确认出单', exact: true }).click();
  const issuedDialog = page.getByRole('dialog', { name: '人工补单已出单' });
  await issuedDialog.getByRole('link', { name: issued.ticketNo, exact: true }).waitFor();
  await issuedDialog.getByText('17.5 吨', { exact: true }).waitFor();
  await issuedDialog.getByRole('button', { name: '关闭详情弹窗' }).click();
  assert.equal(submissions.length, 1);
  assert.deepEqual(submissions[0].netAdjustments, [{ mode: 'amount', amount: '0.5', category: 'IMPURITY', remark: '' }]);

  await page.goto(`${base}/tests/manual-supplement-entry.html?permission=no`, { waitUntil: 'networkidle' });
  await page.getByRole('navigation', { name: '工作台操作' }).waitFor();
  assert.equal(await page.getByRole('button', { name: '人工补单', exact: true }).count(), 0, '无补单权限时不应显示人工补单入口');

  await page.goto(`${base}/tests/manual-supplement-entry.html?permission=yes&operate=no`, { waitUntil: 'networkidle' });
  await page.getByRole('alert').getByText('无本厂地磅称重操作权限。', { exact: false }).waitFor();
  assert.equal(await page.getByRole('button', { name: '人工补单', exact: true }).count(), 0, '无可用厂区时不应显示人工补单入口');

  assert.deepEqual(errors, []);
  console.log('工作台人工补单入口：权限、双向毛皮重、实际时间校验、扣减净重与出单回执通过。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
