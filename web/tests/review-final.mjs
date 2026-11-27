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
  for (const text of ['晋AB6P44', '入厂卸货', '木片 · 散装', '王坚', '毛重 30 吨 · 2026-10-30 09:36:04', '预计净重（未出单）', '13007028190']) {
    assert.ok(await dialog.getByText(text, { exact: true }).isVisible(), `${text} 应保留`);
  }
  assert.equal(await dialog.getByText('物料及规格／编码', { exact: true }).count(), 0, '物料栏不再显示规格与编码');
  assert.ok(await dialog.getByText('物料', { exact: true }).isVisible());
  const facts = await dialog.locator('.review-facts .review-fact-value').evaluateAll(nodes => nodes.map(node => {
    const style = getComputedStyle(node);
    return { size: parseFloat(style.fontSize), lineHeight: parseFloat(style.lineHeight), weight: Number(style.fontWeight) };
  }));
  assert.ok(facts.length >= 4, '业务核对应展示全部核对项');
  assert.ok(facts.every(({ size, lineHeight, weight }) => size >= 14 && size <= 16 && lineHeight >= size * 1.4 && weight === 600), '业务核对值应采用紧凑字号及舒适行高');
  const summaryPlate = await dialog.locator('.review-plate').evaluate(node => parseFloat(getComputedStyle(node).fontSize));
  const summaryWeight = await dialog.locator('.review-weight').evaluate(node => parseFloat(getComputedStyle(node).fontSize));
  assert.ok(summaryPlate >= 20 && summaryWeight >= 26, '首屏应突出当前车牌和采集重量');
  assert.equal(await dialog.locator('.review-summary').getByText(/重量稳定|张抓拍|入厂卸货|回磅/).count(), 0);
  await dialog.getByRole('button', { name: '称重确认说明' }).focus();
  assert.ok(await page.getByRole('tooltip').filter({ hasText: '确认车辆完全上磅' }).isVisible());
  await dialog.getByRole('button', { name: '关闭核对' }).focus();
  const businessHint = dialog.getByRole('button', { name: '业务核对说明' });
  await businessHint.hover();
  assert.ok(await page.getByRole('tooltip').filter({ hasText: '请与现场车辆核对' }).isVisible());
  await page.mouse.move(0, 0);
  const photos = await dialog.getByRole('group', { name: '本次抓拍图片' }).getByRole('img').all();
  assert.ok(photos.length > 0);
  const firstFooter = await dialog.locator('.weighing-review-footer').boundingBox();
  for (const photo of photos) {
    const box = await photo.boundingBox();
    assert.ok(box && box.height >= 140 && box.height <= 160 && box.y >= 0 && box.y + box.height <= firstFooter.y,
      '1366×768 首屏应在固定底栏上方完整显示所有抓拍图片');
  }
  await page.screenshot({ path: '/tmp/weighbridge-review-first.png' });
  await dialog.getByRole('button', { name: '放大抓拍 入口' }).click();
  await page.getByRole('dialog', { name: '抓拍图片预览' }).waitFor();
  await page.keyboard.press('Escape');
  await page.getByRole('dialog', { name: '抓拍图片预览' }).waitFor({ state: 'hidden' });
  for (const width of [1024, 390]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 768 });
    assert.equal(await dialog.evaluate(node => [node, node.querySelector('.weighing-review-body')].every(area => area.scrollWidth <= area.clientWidth)), true, `${width}px 核对窗口不应横向溢出`);
    const next = await dialog.getByRole('button', { name: '下一步：核对最终重量' }).boundingBox();
    assert.ok(next.y >= 0 && next.y + next.height <= (width === 390 ? 844 : 768), `${width}px 下一步按钮应在视口内`);
  }
  await page.setViewportSize({ width: 1366, height: 768 });
  await dialog.getByRole('button', { name: '下一步：核对最终重量' }).click();
  assert.equal(await dialog.locator('.review-hero').count(), 0);
  assert.equal(await dialog.getByText(/第 2 步|可扣减/).count(), 0, '步骤行与净重调整标题行应简化');
  const finalIdentity = await dialog.locator('.review-final-context').innerText();
  for (const text of ['当前车辆', '晋AB6P44', '入厂卸货', '物料', '木片 · 散装', '供货方', '王坚']) assert.ok(finalIdentity.includes(text), `第二步应展示 ${text}`);
  const plateSize = await dialog.locator('.review-context-plate').evaluate(node => parseFloat(getComputedStyle(node).fontSize));
  assert.ok(plateSize >= 20, `第二步上下文应突出车牌：${plateSize}px`);
  await dialog.locator('#net-adjustment-category-0').selectOption('WATER');
  await dialog.getByText('按比例', { exact: true }).click();
  assert.equal(await dialog.getByRole('radio', { name: '按比例' }).isChecked(), true);
  await dialog.getByRole('heading', { name: '首磅云端化验依据' }).waitFor();
  assert.match(await dialog.getByLabel('扣减合计').innerText(), /未扣减/);
  await dialog.locator('#net-adjustment-rate-0').fill('5');
  await dialog.getByText('16.473 吨', { exact: true }).waitFor();
  const addButton = dialog.getByRole('button', { name: '添加扣减项' });
  await addButton.click();
  await dialog.locator('#net-adjustment-category-1').selectOption('IMPURITY');
  await dialog.locator('#net-adjustment-amount-1').fill('0.5');
  await dialog.getByText('15.973 吨', { exact: true }).waitFor();
  await dialog.getByRole('button', { name: '删除该项' }).nth(1).click();
  await dialog.getByText('16.473 吨', { exact: true }).waitFor();
  await dialog.getByRole('button', { name: '撤销删除' }).click();
  await dialog.getByText('15.973 吨', { exact: true }).waitFor();
  const rulesHint = dialog.getByRole('button', { name: '净重调整规则' });
  await rulesHint.hover();
  const rulesTip = page.getByRole('tooltip').filter({ hasText: '调整后净重须大于零' });
  assert.equal(await rulesTip.isVisible(), true);
  await dialog.locator('#net-adjustment-rate-0').fill('-5');
  assert.equal(await dialog.getByRole('button', { name: '确认回磅并出单' }).isDisabled(), true);
  await dialog.locator('#net-adjustment-rate-0').fill('5');
  await dialog.getByRole('button', { name: '上一步' }).click();
  await dialog.getByRole('button', { name: '下一步：核对最终重量' }).click();
  assert.equal(await dialog.locator('#net-adjustment-rate-0').inputValue(), '5');
  await dialog.getByRole('button', { name: '关闭核对' }).click();
  const closeConfirm = page.getByRole('dialog', { name: '放弃已填写的扣减内容？' });
  assert.ok(await closeConfirm.isVisible());
  await closeConfirm.getByRole('button', { name: '返回修改' }).click();
  assert.equal(await dialog.locator('#net-adjustment-rate-0').inputValue(), '5');
  const finalWeight = dialog.getByText('15.973 吨', { exact: true });
  const finalSize = await finalWeight.evaluate(node => parseFloat(getComputedStyle(node).fontSize));
  assert.ok(finalSize >= 26, '应突出最终出单净重');
  for (const viewport of [{ width: 1366, height: 768 }, { width: 1920, height: 1080 }]) {
    await page.setViewportSize(viewport);
    const result = await finalWeight.boundingBox();
    const footer = await dialog.locator('.weighing-review-footer').boundingBox();
    const body = await dialog.locator('.weighing-review-body').boundingBox();
    assert.ok(result && result.y >= body.y && result.y + result.height <= footer.y,
      `${viewport.width}×${viewport.height} 首屏应在固定底栏上方显示最终出单净重`);
  }
  await page.screenshot({ path: '/tmp/weighbridge-review-final.png' });
  for (const width of [1366, 1024, 390]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 768 });
    assert.equal(await dialog.evaluate(node => [node, node.querySelector('.weighing-review-body')].every(area => area.scrollWidth <= area.clientWidth)), true, `${width}px 弹窗不应横向溢出`);
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
