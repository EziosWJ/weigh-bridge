import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const port = 4181;
const baseUrl = `http://127.0.0.1:${port}`;
const fixture = `${baseUrl}/tests/weighing-smoke.html`;
const storageKey = 'weighbridge-phase2a-v1';
const sentinelKey = 'weighbridge-smoke-unrelated';
const plate = '川A2A888';
const materialName = '冒烟高粱';
const partyName = '演示个人供运人';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], {
  cwd: new URL('..', import.meta.url), stdio: ['ignore', 'pipe', 'pipe'],
});
let serverOutput = '';
vite.stdout.on('data', (chunk) => { serverOutput += chunk; });
vite.stderr.on('data', (chunk) => { serverOutput += chunk; });
let browser;
const deadline = setTimeout(() => {
  console.error('weighing page smoke exceeded 90 seconds');
  vite.kill('SIGTERM');
  void browser?.close();
  process.exitCode = 1;
}, 90_000);

async function waitForServer() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    if (vite.exitCode !== null) throw new Error(`Vite exited: ${serverOutput}`);
    try { if ((await fetch(fixture)).ok) return; } catch { /* Vite 正在启动。 */ }
    await delay(250);
  }
  throw new Error(`Vite test server did not start: ${serverOutput}`);
}

try {
  await waitForServer();
  const fixtureHtml = await (await fetch(fixture)).text();
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 }, acceptDownloads: true });
  page.setDefaultTimeout(5000); page.setDefaultNavigationTimeout(10_000);
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  // 只将独立 fixture 的 document 刷新映射回同一入口，业务请求不做 mock。
  await page.route(`${baseUrl}/weighing/**`, (route) => route.request().resourceType() === 'document'
    ? route.fulfill({ status: 200, contentType: 'text/html', body: fixtureHtml }) : route.continue());
  const nav = async (name) => {
    await page.getByRole('navigation', { name: '页面冒烟导航' }).getByRole('link', { name, exact: true }).click();
    await page.getByRole('heading', { name: name === '往来主体' ? '往来主体资料' : name, exact: true }).waitFor();
  };
  const saved = () => page.evaluate((key) => JSON.parse(localStorage.getItem(key)), storageKey);
  const pick = async (label, keyword, name) => {
    await page.getByRole('combobox', { name: `搜索${label}`, exact: true }).fill(keyword);
    await page.getByRole('option').filter({ hasText: name }).click();
  };
  const supplementSummary = () => page.locator('summary').filter({ hasText: '补充运输信息（选填）' });
  const supplement = async (expanded) => {
    const summary = supplementSummary();
    if (await summary.evaluate((element) => element.parentElement.open) !== expanded) await summary.click();
  };
  const manage = async (name) => {
    const summary = page.locator('summary').filter({ hasText: '找不到资料？' });
    if (!await summary.evaluate((element) => element.parentElement.open)) await summary.click();
    await page.getByRole('link', { name, exact: true }).click();
  };
  const assertInViewport = async (locator, label) => {
    const box = await locator.boundingBox();
    assert.ok(box && box.y >= 64 && box.y + box.height <= 768, `${label} should fit below the header in the 1366x768 viewport: ${JSON.stringify(box)}`);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), true, '页面不应横向溢出');
  };
  const recordCounts = async () => {
    const data = await saved();
    return { tasks: data.tasks.length, records: data.tasks.reduce((sum, task) => sum + task.records.length, 0), tickets: data.tickets.length };
  };
  const imageLoaded = async (image) => {
    await image.waitFor({ state: 'visible' });
    await image.scrollIntoViewIfNeeded();
    await page.waitForFunction((element) => element.complete && element.naturalWidth > 0, await image.elementHandle());
  };
  const closeImagePreview = async () => {
    const preview = page.getByRole('dialog', { name: '模拟图片预览', exact: true });
    await imageLoaded(preview.getByRole('img').first());
    await preview.getByRole('button', { name: /关闭/ }).first().click();
    await preview.waitFor({ state: 'detached' });
  };
  const review = async ({ phase, kind, weight, currentPlate = plate, checkBack = false, screenshot = false, finalGross = weight, finalTare = weight }) => {
    const before = await recordCounts();
    await page.getByRole('button', { name: `开始核对${phase}（${kind}）`, exact: true }).click();
    const dialog = page.getByRole('dialog', { name: `逐项核对${phase}（${kind}）`, exact: true });
    await dialog.getByText('核对车牌', { exact: true }).waitFor();
    assert.match(await dialog.innerText(), new RegExp(currentPlate));
    assert.equal(await dialog.getByRole('button', { name: `确认${phase}（${kind}）`, exact: true }).count(), 0);
    await dialog.getByRole('button', { name: '一致，下一项', exact: true }).click();
    await dialog.getByText('核对业务信息', { exact: true }).waitFor();
    assert.match(await dialog.innerText(), new RegExp(materialName));
    assert.match(await dialog.innerText(), new RegExp(partyName));
    if (checkBack) {
      await dialog.getByRole('button', { name: '上一项', exact: true }).click();
      await dialog.getByText('核对车牌', { exact: true }).waitFor();
      await dialog.getByRole('button', { name: '一致，下一项', exact: true }).click();
    }
    await dialog.getByRole('button', { name: '一致，下一项', exact: true }).click();
    await dialog.getByText('核对重量', { exact: true }).waitFor();
    assert.match(await dialog.innerText(), new RegExp(`${weight}\\s*吨`));
    await dialog.getByRole('button', { name: '一致，下一项', exact: true }).click();
    await dialog.getByText('核对模拟证据', { exact: true }).waitFor();
    assert.match(await dialog.innerText(), /模拟/);
    assert.match(await dialog.innerText(), /摄像头.*未.*接入/);
    const capture = (await saved()).capture;
    assert.ok(capture, '本次采集应包含模拟抓拍');
    assert.equal(capture.plate, currentPlate);
    assert.equal(capture.weight, weight);
    const image = dialog.getByRole('img').first();
    await imageLoaded(image);
    assert.equal(new URL(await image.getAttribute('src'), page.url()).pathname, capture.imageSrc);
    const capturedAt = new Date(Date.parse(capture.at) + 8 * 3600_000).toISOString().slice(0, 19).replace('T', ' ');
    assert.ok((await dialog.innerText()).includes(capturedAt), '证据展示本次采集时间');
    assert.match(await dialog.innerText(), new RegExp(currentPlate));
    assert.match(await dialog.innerText(), new RegExp(`${weight}\\s*吨`));
    if (screenshot) {
      await page.screenshot({ path: '/tmp/weighing-capture-review.png' });
      await image.click();
      await closeImagePreview();
    }
    await dialog.getByRole('button', { name: '已核对，查看汇总', exact: true }).click();
    await dialog.getByRole('heading', { name: '确认本次称重', exact: true }).waitFor();
    assert.deepEqual(await recordCounts(), before, '最终明确确认前不应增加任务、称重或单据');
    if (screenshot) {
      await assertInViewport(dialog.getByRole('button', { name: '上一项', exact: true }), '核对返回按钮');
      await assertInViewport(dialog.getByRole('button', { name: '不一致，返回修改', exact: true }), '核对修改按钮');
      await assertInViewport(dialog.getByRole('button', { name: phase === '回磅' ? '确认称重并出单' : `确认${phase}（${kind}）`, exact: true }), '最终确认按钮');
      await page.screenshot({ path: '/tmp/weighing-review.png' });
    }
    if (phase === '回磅') {
      const finalGrossInput = dialog.getByLabel('最终毛重（吨）', { exact: true });
      const finalTareInput = dialog.getByLabel('最终皮重（吨）', { exact: true });
      assert.equal(await finalGrossInput.inputValue(), '32');
      assert.equal(await finalTareInput.inputValue(), '12');
      await finalGrossInput.fill('-1');
      assert.equal(await dialog.getByRole('button', { name: '确认称重并出单', exact: true }).isDisabled(), true);
      await finalGrossInput.fill(String(finalGross));
      await finalGrossInput.blur();
      await finalTareInput.fill(String(finalTare));
      await finalTareInput.blur();
      assert.equal(await finalGrossInput.inputValue(), String(finalGross));
      assert.equal(await finalTareInput.inputValue(), String(finalTare));
      assert.equal(await dialog.getByLabel('扣减原因（选填）', { exact: true }).inputValue(), '');
      await dialog.getByText('系统计算扣减：毛重 0.2 吨 · 皮重 0.1 吨 · 最终净重 19.9 吨', { exact: true }).waitFor();
      await page.screenshot({ path: '/tmp/weighing-adjustment-review.png' });
    }
    await dialog.getByRole('button', { name: phase === '回磅' ? '确认称重并出单' : `确认${phase}（${kind}）`, exact: true }).click();
  };
  await page.goto(fixture, { waitUntil: 'networkidle' });
  await page.evaluate((key) => localStorage.setItem(key, 'keep-me'), sentinelKey);
  await nav('称重工作台');
  const mainBox = await page.locator('main').boundingBox();
  assert.equal(mainBox.x, 240, 'fixture应保留真实桌面240px侧栏');
  assert.equal(mainBox.width, 1126, '1366px桌面下实际内容容器宽度应为1126px');
  assert.equal(await page.getByRole('button', { name: '新建测试任务', exact: true }).count(), 0);
  await page.getByLabel(/^车牌/).fill(plate);
  await page.getByRole('button', { name: '查找当前运输', exact: true }).click();
  assert.equal(await page.getByRole('button', { name: '采集模拟重量', exact: true }).isDisabled(), true);
  assert.equal(await supplementSummary().evaluate((element) => element.parentElement.open), false);
  await supplement(true);
  await page.getByLabel('备注（选填）', { exact: true }).fill('草稿往返保留');
  await page.getByRole('link', { name: '调整模拟重量 / 演示岗位', exact: true }).click();
  await page.getByLabel('演示岗位', { exact: true }).selectOption('admin');
  await page.getByLabel('模拟重量（吨）', { exact: true }).fill('32');
  await page.getByLabel('模拟摄像头画面', { exact: true }).selectOption('front');
  await page.getByRole('link', { name: '返回称重作业', exact: true }).click();
  await supplement(true);
  assert.equal(await page.getByLabel('备注（选填）', { exact: true }).inputValue(), '草稿往返保留');
  await page.reload({ waitUntil: 'networkidle' });
  assert.equal(await page.getByLabel(/^车牌/).inputValue(), plate);
  await supplement(true);
  assert.equal(await page.getByLabel('备注（选填）', { exact: true }).inputValue(), '草稿往返保留');

  await manage('维护物料');
  await page.getByRole('link', { name: '新增物料', exact: true }).click();
  await page.getByLabel(/^物料名称/).fill(materialName);
  await page.getByLabel('规格／等级', { exact: true }).fill('一级');
  await page.getByRole('button', { name: '保存物料', exact: true }).click();
  await page.getByRole('link', { name: '返回称重工作台', exact: true }).click();
  await manage('维护往来主体');
  await page.getByRole('link', { name: '新增往来主体', exact: true }).click();
  await page.getByLabel(/^名称/).fill('演示供运企业');
  await page.getByRole('checkbox', { name: '承运商', exact: true }).check();
  await page.getByRole('button', { name: '保存往来主体', exact: true }).click();
  await page.getByRole('link', { name: '新增往来主体', exact: true }).click();
  await page.getByLabel(/^名称/).fill(partyName);
  await page.getByLabel('简称', { exact: true }).fill('个人供运');
  await page.getByLabel(/^主体性质/).selectOption('individual');
  await page.getByRole('checkbox', { name: '客户', exact: true }).check();
  await page.getByRole('checkbox', { name: '承运商', exact: true }).check();
  await page.getByLabel('主体联系电话', { exact: true }).fill('13800000000');
  for (let index = 1; index <= 2; index += 1) {
    await page.getByRole('button', { name: '添加联系人', exact: true }).click();
    await page.getByLabel(new RegExp(`^姓名（联系人${index}）`)).fill(index === 1 ? '交货联络人' : '默认业务联络人');
    await page.getByLabel(`电话（联系人${index}）`, { exact: true }).fill(`1380000000${index}`);
  }
  await page.getByRole('radio', { name: '默认联系人', exact: true }).nth(1).check();
  await page.screenshot({ path: '/tmp/weighing-party.png', fullPage: true });
  await page.getByRole('button', { name: '保存往来主体', exact: true }).click();
  // 重新进入独立编辑页，确认保存后的多身份与默认联系人。
  await page.getByRole('link', { name: partyName, exact: true }).click();
  await page.getByRole('heading', { name: '编辑往来主体', exact: true }).waitFor();
  assert.equal(await page.getByLabel(/^主体性质/).inputValue(), 'individual');
  assert.equal(await page.getByRole('checkbox', { name: '承运商', exact: true }).isChecked(), true);
  assert.equal(await page.getByRole('radio', { name: '默认联系人', exact: true }).nth(1).isChecked(), true);
  await page.getByRole('button', { name: '保存往来主体', exact: true }).click();
  await page.getByRole('link', { name: '返回称重工作台', exact: true }).click();
  await pick('物料', materialName, materialName);
  await pick('供货方', '个人供运', partyName);
  assert.match(await supplementSummary().innerText(), /默认业务联络人/);
  await supplement(true);
  await pick('承运方', '个人供运', partyName);
  assert.equal(await page.getByLabel('本趟联系人（选填）', { exact: true }).inputValue(), '默认业务联络人');
  await page.getByLabel('司机姓名（选填）', { exact: true }).fill('运输司机');
  await page.getByLabel('司机电话（选填）', { exact: true }).fill('13900000000');
  await page.getByRole('link', { name: '调整模拟重量 / 演示岗位', exact: true }).click();
  await page.getByLabel('演示岗位', { exact: true }).selectOption('operator');
  await page.getByRole('switch', { name: '模拟重量稳定', exact: true }).click();
  await page.getByRole('link', { name: '返回称重作业', exact: true }).click();
  await page.getByText('重量失稳：请恢复稳定并重新采集', { exact: true }).waitFor();
  assert.match(await supplementSummary().innerText(), /运输司机/);
  assert.match(await supplementSummary().innerText(), /草稿往返保留/);
  const blockedConfirm = page.getByRole('button', { name: '开始核对首磅（毛重）', exact: true });
  assert.equal(await blockedConfirm.count() === 0 || !await blockedConfirm.isVisible() || await blockedConfirm.isDisabled(), true);
  await page.getByRole('link', { name: '调整模拟重量 / 演示岗位', exact: true }).click();
  await page.getByRole('switch', { name: '模拟重量稳定', exact: true }).click();
  await page.getByRole('link', { name: '返回称重作业', exact: true }).click();
  await page.getByRole('button', { name: '采集模拟重量', exact: true }).click();
  await page.evaluate(() => window.scrollTo(0, 0));
  await assertInViewport(page.getByRole('button', { name: '开始核对首磅（毛重）', exact: true }), '折叠首磅核对入口');
  await page.screenshot({ path: '/tmp/weighing-workbench-compact.png' });
  await supplement(true);
  await page.getByLabel('备注（选填）', { exact: true }).scrollIntoViewIfNeeded();
  await assertInViewport(page.getByRole('button', { name: '开始核对首磅（毛重）', exact: true }), '展开补充并滚动后的核对入口');
  await supplement(false);
  const beforeReview = await recordCounts();
  await page.getByRole('button', { name: '开始核对首磅（毛重）', exact: true }).click();
  const interrupted = page.getByRole('dialog', { name: '逐项核对首磅（毛重）', exact: true });
  await interrupted.getByRole('button', { name: '一致，下一项', exact: true }).click();
  await interrupted.getByRole('button', { name: '不一致，返回修改', exact: true }).click();
  await interrupted.waitFor({ state: 'detached' });
  await page.getByRole('button', { name: '开始核对首磅（毛重）', exact: true }).click();
  await interrupted.getByText('核对车牌', { exact: true }).waitFor();
  await interrupted.getByRole('button', { name: '一致，下一项', exact: true }).click();
  await page.evaluate(async () => {
    const { useWeighingStore } = await import('/src/store/weighing-store.ts');
    useWeighingStore.getState().dispatch({ type: 'config', patch: { weight: 32001 } });
  });
  await interrupted.waitFor({ state: 'detached' });
  assert.equal(await page.getByRole('button', { name: '开始核对首磅（毛重）', exact: true }).count(), 0);
  assert.equal((await saved()).sampleReady, false);
  assert.deepEqual(await recordCounts(), beforeReview);
  await page.evaluate(async () => {
    const { useWeighingStore } = await import('/src/store/weighing-store.ts');
    useWeighingStore.getState().dispatch({ type: 'config', patch: { weight: 32 } });
  });
  await page.getByRole('button', { name: '采集模拟重量', exact: true }).click();
  await page.evaluate(() => window.scrollTo(0, 0));
  await imageLoaded(page.getByRole('img').first());
  await page.screenshot({ path: '/tmp/weighing-camera.png' });
  await review({ phase: '首磅', kind: '毛重', weight: 32, checkBack: true, screenshot: true });
  await page.getByRole('heading', { name: '首磅已完成', exact: true }).waitFor();
  const afterFirst = await saved();
  const task = afterFirst.tasks.find((item) => item.plate === plate);
  assert.equal(task.records.length, 1); assert.equal(task.status, 'second');
  const firstCapture = task.records[0].capture;
  assert.equal(firstCapture.preset, 'front');
  assert.equal(task.partyName, partyName); assert.equal(task.carrierName, partyName);
  assert.equal(task.contactName, '默认业务联络人'); assert.equal(task.driver, '运输司机');
  assert.equal(afterFirst.parties.find((item) => item.name === '演示供运企业').roles.length, 2);
  assert.equal(await page.getByRole('button', { name: '办理下一辆车', exact: true }).isVisible(), true);
  // 装卸后仍用原车牌查找；无需重建任务。
  await page.getByRole('button', { name: '查找当前运输', exact: true }).click();
  await page.getByText('首次当前依据：毛重 32 吨', { exact: true }).waitFor();
  await page.getByRole('link', { name: '调整模拟重量 / 演示岗位', exact: true }).click();
  await page.getByLabel('模拟重量（吨）', { exact: true }).fill('12');
  await page.getByLabel('模拟摄像头画面', { exact: true }).selectOption('rear');
  await page.getByRole('switch', { name: '模拟证据可用', exact: true }).click();
  await page.getByRole('link', { name: '返回称重作业', exact: true }).click();
  await page.getByRole('alert').filter({ hasText: '模拟证据不可用' }).waitFor();
  assert.equal(await page.getByRole('button', { name: '开始核对回磅（皮重）', exact: true }).count(), 0);
  const afterCameraChange = await saved();
  assert.equal(afterCameraChange.sampleReady, false);
  assert.deepEqual(afterCameraChange.tasks.find((item) => item.id === task.id).records[0].capture, firstCapture, '切换模拟画面不得改写原抓拍');
  await page.getByRole('link', { name: '调整模拟重量 / 演示岗位', exact: true }).click();
  await page.getByRole('switch', { name: '模拟证据可用', exact: true }).click();
  await page.getByRole('link', { name: '返回称重作业', exact: true }).click();
  await page.getByRole('button', { name: '采集模拟重量', exact: true }).click();
  await page.evaluate(() => window.scrollTo(0, 0));
  await assertInViewport(page.getByRole('button', { name: '开始核对回磅（皮重）', exact: true }), '回磅核对入口');
  await page.screenshot({ path: '/tmp/weighing-return-compact.png' });
  await review({ phase: '回磅', kind: '皮重', weight: 12, finalGross: 31.8, finalTare: 11.9 });
  await page.getByRole('heading', { name: '回磅已完成', exact: true }).waitFor();
  const completed = await saved();
  const ticket = completed.tickets.find((item) => item.plate === plate);
  assert.equal(ticket.net, 19.9);
  assert.equal(ticket.adjustment.actor, "演示操作员");
  assert.deepEqual(ticket.adjustment.original, { gross: 32, tare: 12, net: 20 });
  assert.equal(completed.tasks.filter((item) => item.plate === plate).length, 1);
  assert.equal(completed.tickets.filter((item) => item.plate === plate).length, 1);
  const savedRecords = completed.tasks.find((item) => item.id === task.id).records;
  assert.deepEqual(savedRecords[0].capture, firstCapture);
  assert.equal(savedRecords[1].capture.preset, 'rear');
  assert.notEqual(savedRecords[1].capture.imageSrc, firstCapture.imageSrc);
  // 完成后直接查同车牌办理下一趟，必须建立新的首次请求。
  await page.getByRole('button', { name: '查找当前运输', exact: true }).click();
  await page.getByText('本次办理：首次称重 / 毛重', { exact: true }).waitFor();
  const nextTrip = await saved();
  assert.notEqual(nextTrip.draftRequestId, completed.draftRequestId);
  assert.equal(nextTrip.firstRequests[nextTrip.draftRequestId], undefined);
  await nav('运输任务');
  await page.getByRole('link', { name: task.id, exact: true }).click();
  await page.getByRole('heading', { name: '运输任务详情', exact: true }).waitFor();
  assert.match(new URL(page.url()).pathname, /\/tasks\/[^/]+$/);
  await page.reload({ waitUntil: 'networkidle' });
  await page.getByRole('heading', { name: '运输任务详情', exact: true }).waitFor();
  const historyImages = page.getByRole('img');
  assert.equal(await historyImages.count(), 2);
  for (let index = 0; index < 2; index += 1) {
    await imageLoaded(historyImages.nth(index));
    assert.equal(new URL(await historyImages.nth(index).getAttribute('src'), page.url()).pathname, savedRecords[index].capture.imageSrc);
  }
  await historyImages.first().click();
  await closeImagePreview();
  await nav('原始称重');
  await page.getByLabel('车牌或编号', { exact: true }).fill(plate);
  assert.match(await page.locator('main').innerText(), /32 吨 \/ 12 吨 \/ 20 吨/);
  await page.screenshot({ path: '/tmp/weighing-originals.png', fullPage: true });
  await nav('最终单据');
  await page.getByRole('link', { name: ticket.id, exact: true }).click();
  await page.getByRole('heading', { name: '测试过磅单详情', exact: true }).waitFor();
  await page.getByRole('link', { name: '打开只读预览', exact: true }).click();
  await page.getByRole('heading', { name: '测试单打印预览', exact: true }).waitFor();
  await page.getByText('测试 · 非正式凭证', { exact: true }).waitFor();
  await page.getByText(`测试单号：${ticket.id}`, { exact: true }).waitFor();
  assert.doesNotMatch(await page.locator('main').innerText(), /原始毛重|扣减量|32\s*吨|12\s*吨/);
  await page.getByText('31.8 吨', { exact: true }).waitFor();
  await page.screenshot({ path: '/tmp/weighing-final-preview.png', fullPage: true });
  await page.getByRole('button', { name: '演示首次打印结果', exact: true }).click();
  await page.getByText('已模拟打印', { exact: true }).waitFor();
  await nav('演示设置');
  await page.getByLabel('演示岗位', { exact: true }).selectOption('supervisor');
  await nav('最终单据');
  await page.getByLabel('车牌或编号', { exact: true }).fill(plate);
  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('button', { name: '导出明细 CSV', exact: true }).click();
  const download = await downloadPromise;
  const csv = await readFile(await download.path(), 'utf8');
  assert.match(csv, /测试/); assert.match(csv, /吨/); assert.match(csv, new RegExp(plate));
  assert.match(csv, new RegExp(partyName)); assert.doesNotMatch(csv, /川A·示例/);
  assert.match(csv, /最终毛重/);
  await nav('原始称重');
  await page.getByLabel('车牌或编号', { exact: true }).fill(plate);
  const rawDownloadPromise = page.waitForEvent('download');
  await page.getByRole('button', { name: '导出明细 CSV', exact: true }).click();
  const rawCsv = await readFile(await (await rawDownloadPromise).path(), 'utf8');
  assert.match(rawCsv, /原始毛重/); assert.match(rawCsv, /"32","12","20"/);
  await nav('最终单据');
  await page.getByLabel('车牌或编号', { exact: true }).fill(plate);
  await page.getByRole('link', { name: ticket.id, exact: true }).click();
  await page.getByRole('button', { name: '作废测试单', exact: true }).click();
  const voidDialog = page.getByRole('dialog', { name: '作废测试单', exact: true });
  await voidDialog.getByLabel(/操作原因/).fill('扣减复核');
  await voidDialog.getByRole('button', { name: '确认保存', exact: true }).click();
  await page.getByRole('button', { name: '关联更正重开', exact: true }).click();
  const reopenDialog = page.getByRole('dialog', { name: '关联更正重开', exact: true });
  await reopenDialog.getByLabel('最终毛重（吨）', { exact: true }).fill('31.700');
  await reopenDialog.getByRole('checkbox', { name: '已核验原称重依据仍有效', exact: true }).check();
  await reopenDialog.getByLabel(/操作原因/).fill('复核扣减后重开');
  await reopenDialog.getByRole('button', { name: '确认保存', exact: true }).click();
  const reissued = (await saved()).tickets.find(item => item.previousId === ticket.id);
  assert.equal(reissued.net, 19.8);
  assert.equal((await saved()).tickets.find(item => item.id === ticket.id).net, 19.9);
  await page.reload({ waitUntil: 'networkidle' });
  await page.getByText('19.8 吨', { exact: true }).waitFor();
  await nav('原始称重');
  await page.getByLabel('车牌或编号', { exact: true }).fill(plate);
  assert.equal(await page.getByRole('link', { name: task.id, exact: true }).count(), 1);
  assert.match(await page.locator('main').innerText(), /原始净重合计 20 吨/);
  // 未知提交交接给查询原结果，原子首磅仍只产生一个任务和一条依据。
  await nav('称重工作台');
  await page.getByLabel(/^车牌/).fill('川A未知01');
  await page.getByRole('button', { name: '查找当前运输', exact: true }).click();
  assert.equal((await saved()).draft.plate, '川A未知01');
  await pick('物料', materialName, materialName);
  await pick('供货方', '个人供运', partyName);
  await page.getByRole('link', { name: '调整模拟重量 / 演示岗位', exact: true }).click();
  await page.getByLabel('演示岗位', { exact: true }).selectOption('operator');
  await page.getByLabel('模拟重量（吨）', { exact: true }).fill('32');
  await page.getByLabel('模拟提交结果', { exact: true }).selectOption('unknown-success');
  await page.getByRole('link', { name: '返回称重作业', exact: true }).click();
  await page.getByRole('button', { name: '采集模拟重量', exact: true }).click();
  await review({ phase: '首磅', kind: '毛重', weight: 32, currentPlate: '川A未知01' });
  await page.getByRole('button', { name: '查询上次提交结果', exact: true }).click();
  await page.getByRole('heading', { name: '首磅已完成', exact: true }).waitFor();
  const resolved = await saved();
  assert.equal(resolved.tasks.filter((item) => item.plate === '川A未知01').length, 1);
  assert.equal(resolved.tasks.find((item) => item.plate === '川A未知01').records.length, 1);
  await nav('演示设置');
  await page.getByLabel('演示岗位', { exact: true }).selectOption('admin');
  await page.getByRole('button', { name: '重置称重测试数据', exact: true }).click();
  await page.getByRole('button', { name: '确认重置', exact: true }).click();
  const reset = await saved();
  assert.equal(reset.tasks.some((item) => item.plate === plate), false);
  assert.equal(reset.parties.some((item) => item.name === partyName), false);
  assert.equal(reset.materials.some((item) => item.name === materialName), false);
  assert.equal(await page.evaluate((key) => localStorage.getItem(key), sentinelKey), 'keep-me');
  assert.deepEqual(errors, []);
  console.log('weighing smoke passed: original/final lists, optional deduction reason, audit, CSV, void/reopen, reload, final-only preview, existing weigh/camera flows and reset');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
