import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4279';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4279', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 45000);
try {
  for (let n = 0; n < 80; n++) {
    try { if ((await fetch(`${base}/tests/review-final.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  page.setDefaultTimeout(5000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/api/**', route => new URL(route.request().url()).pathname.startsWith('/api/') ? route.fulfill({ status: 500, json: {} }) : route.continue());
  await page.goto(`${base}/tests/review-final.html`);
  const dialog = page.getByRole('dialog', { name: '核对回磅（皮重）' });
  assert.equal(await dialog.getByRole('button', { name: '重新采集重量' }).count(), 0);
  await dialog.getByRole('img', { name: '出口抓拍' }).waitFor();
  for (const text of ['晋AB6P44', '入厂卸货', '木片 · C-M-0002', '王坚 · C-P-0001', '毛重 30 吨 · 2026-10-30 09:36:04', '预计净重（未出单）', '13007028190']) {
    assert.ok(await dialog.getByText(text, { exact: true }).isVisible(), `${text} 应保留`);
  }
  assert.equal(await dialog.locator('.review-summary').getByText(/重量稳定|张抓拍|入厂卸货|回磅/).count(), 0);
  assert.ok(await dialog.getByText('请确认车辆已完全上磅，现场与资料一致。', { exact: true }).isVisible());
  const businessHint = dialog.getByRole('button', { name: '业务核对说明' });
  await businessHint.hover();
  assert.ok(await page.getByRole('tooltip').filter({ hasText: '请与现场车辆核对' }).isVisible());
  await page.mouse.move(0, 0);
  const firstPhoto = await dialog.getByRole('button', { name: '放大抓拍 入口' }).boundingBox();
  const firstFooter = await dialog.locator('.weighing-review-footer').boundingBox();
  await page.screenshot({ path: '/tmp/weighbridge-review-first.png' });
  assert.ok(firstPhoto.y + firstPhoto.height <= firstFooter.y, '桌面首屏应完整显示抓拍图片');
  await dialog.getByRole('button', { name: '放大抓拍 入口' }).click();
  await page.getByRole('dialog', { name: '抓拍图片预览' }).waitFor();
  await page.keyboard.press('Escape');
  await page.getByRole('dialog', { name: '抓拍图片预览' }).waitFor({ state: 'hidden' });
  for (const width of [1024, 390]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 768 });
    assert.equal(await dialog.evaluate(node => node.scrollWidth <= node.clientWidth), true, `${width}px 核对窗口不应横向溢出`);
    const next = await dialog.getByRole('button', { name: '下一步：核对最终重量' }).boundingBox();
    assert.ok(next.y >= 0 && next.y + next.height <= (width === 390 ? 844 : 768), `${width}px 下一步按钮应在视口内`);
  }
  await page.setViewportSize({ width: 1366, height: 768 });
  await dialog.getByRole('button', { name: '下一步：核对最终重量' }).click();
  assert.equal(await dialog.locator('.review-compact-summary').count(), 1);
  assert.equal(await dialog.locator('.review-hero').count(), 0);
  await dialog.getByText('按比例', { exact: true }).click();
  assert.equal(await dialog.getByRole('radio', { name: '按比例' }).isChecked(), true);
  await dialog.getByLabel('净重调整比例（%）', { exact: true }).fill('5');
  await dialog.getByLabel('调整备注（扣减或上调时必填）').selectOption('QUALITY');
  await dialog.getByText('16.473 吨', { exact: true }).waitFor();
  assert.equal(await dialog.locator('.weighing-review-footer').getByText(/最终毛重|最终皮重|最终净重|净重调整：/).count(), 0);
  for (const text of ['30 吨', '12.66 吨', '17.34 吨', '0.867 吨（扣减）']) {
    const number = dialog.getByRole('region', { name: '净重调整', exact: true }).getByText(text, { exact: true });
    assert.ok(await number.isVisible());
    assert.ok(await number.evaluate(node => parseFloat(getComputedStyle(node).fontSize) >= 24), `${text} 应放大`);
  }
  const hint = dialog.getByRole('button', { name: '净重调整比例（%）说明' });
  const tip = page.getByRole('tooltip', { includeHidden: true }).filter({ hasText: '最多两位小数' });
  assert.equal(await tip.isVisible(), false);
  await hint.hover();
  assert.equal(await tip.isVisible(), true);
  await page.mouse.move(0, 0);
  await hint.focus();
  assert.equal(await tip.isVisible(), true);
  const rulesHint = dialog.getByRole('button', { name: '净重调整规则' });
  await rulesHint.hover();
  const rulesTip = page.getByRole('tooltip').filter({ hasText: '调整后净重须大于零' });
  assert.equal(await rulesTip.isVisible(), true);
  const result = await dialog.getByText('16.473 吨', { exact: true }).boundingBox();
  const footer = await dialog.locator('.weighing-review-footer').boundingBox();
  assert.ok(result.y + result.height <= footer.y, '调整后净重应完整显示在固定底栏上方');
  await page.screenshot({ path: '/tmp/weighbridge-review-final.png' });
  await dialog.getByLabel('净重调整比例（%）', { exact: true }).fill('-5');
  assert.match(await dialog.getByRole('alert').innerText(), /正常出单只能扣减净重/);
  assert.equal(await dialog.getByRole('button', { name: '确认回磅并出单' }).isDisabled(), true);
  await dialog.getByLabel('净重调整比例（%）', { exact: true }).fill('5');
  for (const width of [1366, 1024, 390]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 768 });
    assert.equal(await dialog.evaluate(node => node.scrollWidth <= node.clientWidth), true, `${width}px 弹窗不应横向溢出`);
    assert.equal(await dialog.getByRole('button', { name: '确认回磅并出单' }).isEnabled(), true);
    const button = await dialog.getByRole('button', { name: '确认回磅并出单' }).boundingBox();
    assert.ok(button.y >= 0 && button.y + button.height <= (width === 390 ? 844 : 768), `${width}px 确认按钮应在视口内`);
  }
  await rulesHint.click();
  assert.equal(await rulesTip.isVisible(), true);
  const tipBox = await rulesTip.boundingBox();
  assert.ok(tipBox.x >= 0 && tipBox.x + tipBox.width <= 390, '窄屏说明不应超出视口');
  const touchContext = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true });
  const touchPage = await touchContext.newPage();
  await touchPage.route('**/api/**', route => new URL(route.request().url()).pathname.startsWith('/api/') ? route.fulfill({ status: 500, json: {} }) : route.continue());
  await touchPage.goto(`${base}/tests/review-final.html`);
  await touchPage.getByRole('button', { name: '下一步：核对最终重量' }).tap();
  await touchPage.getByRole('button', { name: '净重调整规则' }).tap();
  assert.equal(await touchPage.getByRole('tooltip').filter({ hasText: '调整后净重须大于零' }).isVisible(), true);
  await touchContext.close();
  assert.deepEqual(errors, []);
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
