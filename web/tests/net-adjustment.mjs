import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4278';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4278', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 45000);
try {
  for (let n = 0; n < 80; n++) {
    try { if ((await fetch(`${base}/tests/net-adjustment.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
  await page.route('**/api/**', route => new URL(route.request().url()).pathname.startsWith('/api/') ? route.fulfill({ status: 500, json: {} }) : route.continue());
  await page.goto(`${base}/tests/net-adjustment.html`);
  const first = page.locator('#net-adjustment-rate-0');
  assert.equal(await page.locator('#net-adjustment-category-0').inputValue(), 'WATER');
  assert.equal(await first.inputValue(), '', '默认水分数值应为空');
  assert.equal(await page.getByRole('radio', { name: '按比例' }).isChecked(), true);
  assert.match(await page.getByLabel('扣减合计').innerText(), /未扣减/);
  await first.fill('');
  assert.match(await page.getByLabel('扣减合计').innerText(), /未扣减/);
  await first.fill('5');
  assert.match(await page.getByLabel('扣减合计').innerText(), /16\.473 吨/);
  assert.match(await page.locator('fieldset').innerText(), /17\.34 吨 × 5% = 扣减 0\.867 吨/);
  await page.getByRole('button', { name: '添加扣减项' }).click();
  assert.equal(await page.locator('#net-adjustment-category-1').inputValue(), 'IMPURITY');
  assert.equal(await page.getByRole('radiogroup', { name: '第 2 项录入方式' }).getByRole('radio', { name: '按吨' }).isChecked(), true);
  await page.locator('#net-adjustment-amount-1').fill('0.5');
  assert.match(await page.getByLabel('扣减合计').innerText(), /15\.973 吨/);
  await page.getByRole('radiogroup', { name: '第 1 项录入方式' }).getByText('按吨').click();
  assert.equal(await page.locator('#net-adjustment-amount-0').inputValue(), '', '首次切换方式应为空');
  await page.locator('#net-adjustment-amount-0').fill('0.2');
  await page.getByRole('radiogroup', { name: '第 1 项录入方式' }).getByText('按比例').click();
  assert.equal(await first.inputValue(), '5');
  await page.getByRole('button', { name: '删除该项' }).first().click();
  assert.equal(await page.locator('#net-adjustment-category-0').inputValue(), 'IMPURITY');
  assert.equal(await page.locator('#net-adjustment-amount-0').inputValue(), '0.5');
  await page.getByRole('button', { name: '撤销删除' }).click();
  assert.equal(await first.inputValue(), '5');
  assert.match(await page.getByLabel('扣减合计').innerText(), /15\.973 吨/);
  await page.locator('#net-adjustment-category-1').selectOption('OTHER');
  await page.getByLabel('补充说明（选填）').fill('雨水浸湿');
  assert.equal(JSON.parse(await page.locator('output').innerText())[1].remark, '雨水浸湿');
  await first.fill('0.123');
  assert.match(await page.locator('fieldset').innerText(), /最多两位小数/);
  await first.fill('-5');
  assert.match(await page.locator('fieldset').innerText(), /不允许上调净重/);
  await page.getByRole('button', { name: '加载历史上调' }).click();
  assert.match(await page.locator('fieldset').innerText(), /不允许上调净重/);
  await page.getByRole('button', { name: '删除该项' }).click();
  assert.equal(await page.locator('#net-adjustment-category-0').inputValue(), 'WATER');
  assert.match(await page.getByLabel('扣减合计').innerText(), /未扣减/);
  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(await page.locator('fieldset').evaluate(node => node.scrollWidth <= node.clientWidth), true);
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
