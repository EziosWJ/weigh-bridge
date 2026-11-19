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
  await dialog.getByText('两次称重均已完成且系统没有称重记录时，核对现场记录后补单。', { exact: true }).waitFor();
  await dialog.getByRole('button', { name: '取消', exact: true }).click();

  await page.goto(`${base}/tests/manual-supplement-entry.html?permission=no`, { waitUntil: 'networkidle' });
  await page.getByRole('navigation', { name: '工作台操作' }).waitFor();
  assert.equal(await page.getByRole('button', { name: '人工补单', exact: true }).count(), 0, '无补单权限时不应显示人工补单入口');

  await page.goto(`${base}/tests/manual-supplement-entry.html?permission=yes&operate=no`, { waitUntil: 'networkidle' });
  await page.getByRole('alert').getByText('无本厂地磅称重操作权限。', { exact: false }).waitFor();
  assert.equal(await page.getByRole('button', { name: '人工补单', exact: true }).count(), 0, '无可用厂区时不应显示人工补单入口');

  assert.deepEqual(errors, []);
  console.log('工作台人工补单入口：权限可见性、顶栏位置与弹窗打开通过。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
