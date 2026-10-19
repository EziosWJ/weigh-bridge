import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4193';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4193', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
let browser, page;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 45_000);
try {
  for (let n = 0; n < 60; n++) {
    try { if ((await fetch(`${base}/tests/weighing-lookup.html`)).ok) break; } catch { /* Starting. */ }
    await delay(150);
  }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  page = await browser.newPage({ viewport: { width: 1366, height: 1200 } });
  page.setDefaultTimeout(4000);
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  let materials = [], parties = [], fail = false, requests = 0;
  await page.route('**/api/weighing/**', async route => {
    requests++;
    if (fail) return route.fulfill({ status: 500, json: { code: 500, message: '资料服务暂不可用' } });
    const rows = new URL(route.request().url()).pathname.endsWith('/materials') ? materials : parties;
    await route.fulfill({ json: { code: 200, data: { records: rows, total: rows.length, page: 1, pageSize: 500 } } });
  });
  await page.goto(`${base}/tests/weighing-lookup.html`);
  await page.getByRole('combobox', { name: '搜索物料', exact: true }).focus();
  await page.getByRole('combobox', { name: '搜索物料', exact: true }).press('ArrowDown');
  await page.getByText('没有匹配档案，请调整关键词。', { exact: true }).waitFor();
  await page.keyboard.press('Escape');

  // Simulate another management window saving after the workbench has opened.
  materials = [{ id: 12, name: '新增高粱', code: 'M12', spec: '一级', enabled: 1, version: 1, categoryId: null }];
  parties = [{ id: 23, name: '新增供运方', code: 'P23', nature: 'individual', roles: ['supplier', 'carrier'], enabled: 1, version: 1, contacts: [{ id: 1, name: '默认联系人', phone: '13800000000', isDefault: 1 }] }];
  await page.getByRole('combobox', { name: '搜索物料', exact: true }).focus();
  await page.getByRole('combobox', { name: '搜索物料', exact: true }).press('ArrowDown');
  await page.getByRole('option', { name: /新增高粱/ }).click();
  await page.getByRole('button', { name: '浏览选择供货方', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: /新增供运方/ }).click();
  await page.getByRole('combobox', { name: '搜索承运方', exact: true }).focus();
  await page.getByRole('combobox', { name: '搜索承运方', exact: true }).press('ArrowDown');
  await page.getByRole('option', { name: /新增供运方/ }).click();
  await page.waitForFunction(() => JSON.parse(document.querySelector('output').textContent).value.carrierId === '23');
  const selected = JSON.parse(await page.getByLabel('当前选择').textContent());
  assert.equal(selected.value.materialId, '12');
  assert.equal(selected.material, '新增高粱');
  assert.equal(selected.party, '新增供运方');
  assert.equal(selected.carrier, '新增供运方');
  assert.equal(selected.value.contactName, '默认联系人');
  const before = requests;
  fail = true;
  await page.getByRole('combobox', { name: '搜索物料', exact: true }).focus();
  await page.getByRole('combobox', { name: '搜索物料', exact: true }).press('ArrowDown');
  await page.getByRole('button', { name: '重试搜索', exact: true }).waitFor();
  assert.equal(await page.getByRole('listbox', { name: '物料候选', exact: true }).getByRole('option').count(), 0, '失败时不能继续展示旧候选');
  fail = false;
  materials = [{ ...materials[0], name: '更新高粱' }];
  await page.getByRole('button', { name: '重试搜索', exact: true }).click();
  await page.getByRole('option', { name: /更新高粱/ }).click();
  assert.ok(requests > before);
  assert.deepEqual(errors, []);
  console.log('称重资料选择通过：重新打开获取新增资料、弹窗、承运方、联系人、失败重试与资料状态同步');
} catch (error) {
  if (page) console.error(await page.locator("body").innerText());
  throw error;
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
