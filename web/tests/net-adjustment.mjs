import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4278';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4278', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 30000);
try {
  for (let n = 0; n < 80; n++) {
    try { if ((await fetch(`${base}/tests/net-adjustment.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  await page.route('**/api/**', route => new URL(route.request().url()).pathname.startsWith('/api/') ? route.fulfill({ status: 500, json: {} }) : route.continue());
  await page.goto(`${base}/tests/net-adjustment.html`);
  await page.getByText('16.473 吨', { exact: true }).waitFor();
  assert.equal(await page.getByText('0.867 吨（扣减）', { exact: true }).count(), 1, '5% 扣减应显示扣减');
  assert.equal(await page.getByText(/当前为向上调整净重/).count(), 0, '扣减不应显示上调警告');
  const note = page.getByLabel('调整备注（扣减或上调时必填）', { exact: true });
  assert.equal(await page.getByLabel('自定义备注（选填）').count(), 0);
  await note.selectOption('OTHER');
  await page.getByLabel('自定义备注（选填）').fill('雨水浸湿');
  assert.equal(JSON.parse(await page.locator('output').innerText()).remark, '雨水浸湿');
  await note.selectOption('WATER');
  assert.equal(await page.getByLabel('自定义备注（选填）').count(), 0);
  assert.equal(JSON.parse(await page.locator('output').innerText()).remark, '');
  await page.getByRole('button', { name: '切换更正重开' }).click();
  await page.getByLabel('净重调整比例（%）', { exact: true }).fill('-5');
  await page.getByText('18.207 吨', { exact: true }).waitFor();
  assert.equal(await page.getByText('0.867 吨（上调）', { exact: true }).count(), 1);
  assert.equal(await page.getByText(/当前为向上调整净重 0.867 吨/).count(), 1);
  await page.getByText('按吨', { exact: true }).click();
  assert.equal(await page.getByRole('radio', { name: '按吨' }).isChecked(), true);
  await page.getByLabel('净重调整量（吨）', { exact: true }).fill('0.5');
  await page.getByText('0.5 吨（扣减）', { exact: true }).waitFor();
  assert.equal(await page.getByText(/当前为向上调整净重/).count(), 0);
  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(await page.locator('fieldset').evaluate(node => node.scrollWidth <= node.clientWidth), true);
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
