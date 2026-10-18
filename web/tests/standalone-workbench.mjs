import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const port = 4184;
const base = `http://127.0.0.1:${port}`;
const key = 'weighbridge-phase2a-v1';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: ['ignore', 'pipe', 'pipe'] });
let log = '';
vite.stdout.on('data', data => { log += data; }); vite.stderr.on('data', data => { log += data; });
let browser;
let debugContext;
const deadline = setTimeout(() => { console.error('standalone workbench exceeded 120 seconds'); vite.kill(); void browser?.close(); process.exitCode = 1; }, 120_000);
const saved = page => page.evaluate(key => JSON.parse(localStorage.getItem(key)), key);
const command = (page, command) => page.evaluate(async command => { const { useWeighingStore } = await import('/src/store/weighing-store.ts'); await useWeighingStore.getState().dispatch(command); }, command);
const wait = async (page, fn, message) => {
  for (let i = 0; i < 60; i++) { if (await fn(await saved(page))) return; await delay(50); }
  throw new Error(message);
};
const pick = async (page, name, text) => { await page.getByRole('combobox', { name: `搜索${name}`, exact: true }).fill(text); await page.getByRole('option').filter({ hasText: text }).first().click(); };
const locate = async page => { await page.getByRole('button', { name: '核对车牌并查找运输', exact: true }).click(); await page.getByRole('button', { name: '采集模拟重量', exact: true }).waitFor(); };
const capture = async page => { await page.getByRole('button', { name: '采集模拟重量', exact: true }).click(); await page.getByRole('button', { name: /补齐资料后核对|开始核对/ }).waitFor(); };
const confirm = async (page, stage, kind) => {
  await page.getByRole('button', { name: `开始核对${stage}（${kind}）`, exact: true }).click();
  const dialog = page.getByRole('dialog', { name: `核对${stage}（${kind}）`, exact: true });
  await dialog.getByRole('img').first().evaluate(img => img.complete || new Promise(resolve => img.addEventListener('load', resolve, { once: true })));
  await dialog.getByRole('button', { name: stage === '首磅' ? `确认首磅（${kind}）` : '确认称重并出单', exact: true }).click();
  await page.getByRole('button', { name: '办理下一辆车', exact: true }).waitFor();
};
try {
  for (let i = 0; ; i++) {
    if (vite.exitCode !== null) throw new Error(log);
    try { if ((await fetch(base)).ok) break; } catch { /* starting */ }
    if (i > 40) throw new Error(log); await delay(200);
  }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1366, height: 768 } });
  debugContext = context;
  context.setDefaultTimeout(7000);
  const errors = [];
  context.on('page', page => page.on('pageerror', error => errors.push(error.message)));
  // 复用生产入口、真实路由/菜单/鉴权外壳，仅提供本地登录状态与无关后台接口响应。
  await context.route(url => url.pathname.startsWith('/api/'), route => route.fulfill({ json: { code: 200, message: 'success', data: route.request().url().includes('/menus') ? [] : { records: [], total: 0 } } }));
  const admin = await context.newPage();
  await admin.goto(base + '/login');
  await admin.evaluate(() => localStorage.setItem('react-admin-auth', JSON.stringify({ token: 'local-browser-test', user: { id: 1, username: 'operator', nickname: '浏览器测试员', roles: [] } })));
  await admin.goto(base + '/weighing/tasks');
  await admin.getByRole('heading', { name: '运输任务', exact: true }).waitFor();
  const menu = admin.getByRole('navigation', { name: '主导航' });
  const firstPage = context.waitForEvent('page');
  await menu.getByRole('link', { name: '称重工作台', exact: true }).click();
  let wb = await firstPage;
  await wb.getByRole('heading', { name: '称重工作台', exact: true }).waitFor();
  assert.equal(await wb.locator('aside').count(), 0);
  assert.equal(await wb.getByRole('navigation', { name: '主导航' }).count(), 0);
  assert.match(await wb.locator('.workbench-topbar').innerText(), /浏览器测试员/);
  await wb.getByRole('button', { name: '全屏', exact: true }).click();
  await wb.waitForFunction(() => !!document.fullscreenElement || document.body.textContent.includes('浏览器未允许全屏'));
  if (await wb.evaluate(() => !!document.fullscreenElement)) {
    await wb.getByRole('button', { name: '退出全屏', exact: true }).click();
    await wb.waitForFunction(() => !document.fullscreenElement);
  } else await wb.getByText(/浏览器未允许全屏/).waitFor();
  const count = context.pages().length;
  await menu.getByRole('link', { name: '称重工作台', exact: true }).click();
  await delay(150); assert.equal(context.pages().length, count, '重复菜单复用窗口');
  // 真实菜单搜索入口也复用工作台。
  await admin.getByRole('button', { name: '菜单搜索', exact: true }).click();
  await admin.getByRole('combobox', { name: '搜索菜单' }).fill('称重工作台');
  await admin.getByRole('option', { name: /称重工作台/ }).click();
  assert.equal(context.pages().length, count);
  await admin.evaluate(() => { window.testOriginalOpen = window.open; window.open = () => null; });
  await menu.getByRole('link', { name: '称重工作台', exact: true }).click();
  await admin.getByRole('link', { name: '直接打开工作台', exact: true }).waitFor();
  await admin.evaluate(() => { window.open = window.testOriginalOpen; delete window.testOriginalOpen; });
  await wb.close();
  const reopened = context.waitForEvent('page');
  await menu.getByRole('link', { name: '称重工作台', exact: true }).click(); wb = await reopened;
  await wb.getByRole('heading', { name: '称重工作台', exact: true }).waitFor();

  await wb.getByRole('button', { name: '模拟车辆到磅', exact: true }).click();
  await wb.getByText(/模拟候选 晋A12345/).waitFor();
  assert.equal(await wb.getByRole('button', { name: '采集模拟重量', exact: true }).isEnabled(), false, '候选未人工核对不能采集');
  assert.match(await wb.getByLabel('模拟现场车牌特写').innerText(), /晋A12345/);
  assert.equal((await saved(wb)).tasks.length, 7);
  await locate(wb); await capture(wb);
  const sample = (await saved(wb)).capture;
  await wb.getByRole('button', { name: '模拟车辆到磅', exact: true }).click();
  await wb.getByText(/当前车辆保持不变/).waitFor();
  assert.equal((await saved(wb)).capture.id, sample.id, '新候选只提示，保留当前采集');
  await pick(wb, '物料', '酿酒高粱'); await pick(wb, '供货方', '示例粮食供应商');

  // 开后台不回写采集；资料维护期间仍保留当前草稿、图片和时间。
  const managementPage = context.waitForEvent('page');
  await wb.getByRole('link', { name: '后台查询', exact: true }).click();
  const management = await managementPage;
  await management.getByRole('heading', { name: '运输任务', exact: true }).waitFor();
  assert.deepEqual((await saved(wb)).capture, sample);
  await management.goto(base + '/weighing/settings');
  await management.getByLabel('演示岗位', { exact: true }).selectOption('admin');
  await wait(wb, d => d.config.role === 'admin', '岗位同步');
  await management.goto(base + '/weighing/materials/new');
  await management.getByLabel(/^物料名称/).fill('跨标签新增物料');
  await management.getByRole('button', { name: '保存物料', exact: true }).click();
  await management.getByRole('heading', { name: '物料资料', exact: true }).waitFor();
  assert.deepEqual((await saved(wb)).capture, sample);
  await pick(wb, '物料', '跨标签新增物料');
  await management.goto(base + '/weighing/settings');
  await management.getByLabel('演示岗位', { exact: true }).selectOption('operator');
  await wb.getByRole('button', { name: '开始核对首磅（毛重）', exact: true }).waitFor();
  const closeup = await wb.getByLabel('模拟现场车牌特写').boundingBox();
  assert.ok(closeup.y + closeup.height < 768, '模拟现场车牌特写应在桌面首屏');
  const action = await wb.getByRole('button', { name: '开始核对首磅（毛重）', exact: true }).boundingBox();
  assert.ok(action.y + action.height < 768, '桌面主操作首屏可见');
  assert.equal(await wb.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await wb.screenshot({ path: '/tmp/issue2-workbench-desktop.png' });
  // 视野调整、放大与焦点恢复。
  for (const label of ['小窗', '大窗', '标准']) await wb.getByRole('button', { name: label, exact: true }).click();
  await wb.getByRole('button', { name: '放大摄像头画面', exact: true }).click();
  await wb.getByRole('dialog', { name: '摄像头画面', exact: true }).getByRole('button', { name: '关闭图片预览' }).click();
  assert.equal(await wb.getByRole('button', { name: '放大摄像头画面', exact: true }).evaluate(el => el === document.activeElement), true);
  assert.equal((await saved(wb)).capture.id, sample.id);
  await confirm(wb, '首磅', '毛重');
  const task = (await saved(wb)).tasks.find(t => t.plate === '晋A12345');
  assert.deepEqual(task.records[0].capture.plateEvidence, sample.plateEvidence);

  // 后台已有内存状态再保存，不能覆盖工作台刚保存的新任务。
  await command(admin, { type: 'config', patch: { printOutcome: 'failed' } });
  assert.ok((await saved(wb)).tasks.some(t => t.id === task.id));
  await management.goto(base + '/weighing/settings');
  await management.getByLabel('模拟重量（吨）', { exact: true }).fill('12');
  await wb.getByRole('button', { name: '办理下一辆车', exact: true }).click();
  await wb.getByRole('button', { name: '模拟车辆到磅', exact: true }).click();
  await wb.getByText(/模拟候选 晋A12345/).waitFor();
  await locate(wb); await capture(wb);
  await confirm(wb, '回磅', '皮重');
  let data = await saved(wb);
  assert.equal(data.tickets.find(t => t.taskId === task.id).net, 18);
  assert.equal(data.tasks.find(t => t.id === task.id).records[1].capture.plateEvidence.candidate, '晋A12345');

  // 新能源错误候选修正；先编辑时新结果也不覆盖。
  await management.getByRole('button', { name: '新能源错识示例', exact: true }).click();
  await management.getByRole('button', { name: '保存模拟识别设置', exact: true }).click();
  await management.getByText(/模拟识别设置已保存/).waitFor();
  await wb.getByRole('button', { name: '办理下一辆车', exact: true }).click();
  await wb.getByRole('button', { name: '模拟车辆到磅', exact: true }).click();
  await wb.getByText(/模拟候选 苏AD12346/).waitFor();
  await wb.getByLabel('车牌号码', { exact: true }).fill('ad12345');
  assert.equal(await wb.getByLabel('车牌号码', { exact: true }).inputValue(), 'AD12345');
  await wb.getByRole('button', { name: '模拟车辆到磅', exact: true }).click();
  await wb.getByText(/当前车辆保持不变/).waitFor();
  assert.equal(await wb.getByLabel('车牌号码', { exact: true }).inputValue(), 'AD12345');
  await locate(wb); await capture(wb);
  await wb.getByLabel(/^运输方向/).selectOption('outbound');
  await pick(wb, '物料', '酒糟'); await pick(wb, '收货方', '示例饲料收购方');
  // 当前现场条件变更会使采集失效。
  await management.getByLabel('模拟重量（吨）', { exact: true }).fill('11');
  await wb.getByText(/后台已修改当前任务或模拟现场条件/).waitFor();
  await capture(wb);
  await confirm(wb, '首磅', '皮重');
  const outbound = (await saved(wb)).tasks.find(t => t.plate === '苏AD12345');
  assert.equal(outbound.records[0].capture.plateEvidence.candidate, '苏AD12346');
  assert.equal(outbound.records[0].capture.plateEvidence.confirmedPlate, '苏AD12345');

  // 任务入口使用实际链接定位同一独立页，随后做另一方向回磅与最终重量调整。
  await management.goto(base + `/weighing/tasks/${outbound.id}`);
  await management.getByRole('link', { name: '前往工作台办理', exact: true }).click();
  await wb.waitForURL(`**/weighing/workbench?task=${outbound.id}`);
  await management.goto(base + '/weighing/settings');
  await management.getByLabel('模拟重量（吨）', { exact: true }).fill('31');
  await capture(wb);
  await wb.getByRole('button', { name: '开始核对回磅（毛重）', exact: true }).click();
  let dialog = wb.getByRole('dialog', { name: '核对回磅（毛重）', exact: true });
  await dialog.getByLabel('最终毛重（吨）', { exact: true }).fill('30.8');
  await dialog.getByLabel('最终皮重（吨）', { exact: true }).fill('10.9');
  await dialog.getByRole('button', { name: '确认称重并出单', exact: true }).click();
  await wb.getByRole('button', { name: '办理下一辆车', exact: true }).waitFor();
  data = await saved(wb);
  assert.equal(data.tickets.find(t => t.taskId === outbound.id).net, 19.9);
  // 回磅从任务入口手工定位，没有伪造新识别候选。
  assert.equal(data.tasks.find(t => t.id === outbound.id).records[1].capture.plateEvidence.source, 'manual');

  // 失败场景用同一控件人工录入，省份选择、连续键入与完整车牌粘贴。
  await management.getByLabel('模拟识别场景', { exact: true }).selectOption('failure');
  await management.getByRole('button', { name: '保存模拟识别设置', exact: true }).click();
  await wb.getByRole('button', { name: '办理下一辆车', exact: true }).click();
  await wb.getByRole('button', { name: '模拟车辆到磅', exact: true }).click();
  await wb.getByText(/模拟识别失败，请人工录入/).waitFor();
  await wb.getByRole('button', { name: '选择车牌省份', exact: true }).click();
  await wb.getByRole('group', { name: '车牌省份', exact: true }).getByRole('button', { name: '沪', exact: true }).click();
  await wb.getByLabel('车牌号码', { exact: true }).pressSequentially('b12345');
  assert.equal(await wb.getByLabel('车牌号码', { exact: true }).inputValue(), 'B12345');
  await wb.getByLabel('车牌号码', { exact: true }).evaluate(input => {
    const clipboardData = new DataTransfer(); clipboardData.setData('text/plain', '京c54321');
    input.dispatchEvent(new ClipboardEvent('paste', { clipboardData, bubbles: true, cancelable: true }));
  });
  assert.equal(await wb.getByLabel('车牌号码', { exact: true }).inputValue(), 'C54321');
  await locate(wb); await capture(wb);
  assert.equal((await saved(wb)).capture.plateEvidence.source, 'manual');
  await wb.getByLabel('车牌号码', { exact: true }).fill('c54322');
  await wait(wb, d => !d.sampleReady, '改牌清除采集');
  assert.equal(await wb.getByRole('button', { name: '采集模拟重量', exact: true }).isEnabled(), false);
  await wb.setViewportSize({ width: 390, height: 844 });
  await wb.screenshot({ path: '/tmp/issue2-workbench-mobile.png' });
  assert.equal(await wb.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, '窄屏无横向溢出');
  await wb.reload();
  assert.equal(await wb.getByLabel('车牌号码', { exact: true }).inputValue(), 'C54322');
  assert.equal((await saved(wb)).tasks.find(t => t.id === task.id).records[0].capture.plateEvidence.candidate, '晋A12345');
  await management.goto(base + `/weighing/tasks/${task.id}`);
  await management.getByText('原始模拟候选：晋A12345').first().waitFor();
  await management.goto(base + '/weighing/tasks/TEST-T-001');
  await management.getByText('历史车牌识别来源：未记录').waitFor();
  // 操作员纠错沿用专用控件、原因和留痕；后台修改当前任务使原采集失效。
  await management.goto(base + '/weighing/settings');
  await management.getByLabel('演示岗位', { exact: true }).selectOption('operator');
  await wb.goto(base + '/weighing/workbench?task=TEST-T-001');
  await capture(wb);
  await management.goto(base + '/weighing/tasks/TEST-T-001');
  await management.getByRole('button', { name: '业务纠错', exact: true }).click();
  const correction = management.getByRole('dialog', { name: '业务纠错', exact: true });
  await correction.getByLabel('车牌号码', { exact: true }).fill('晋A20001');
  await correction.getByRole('button', { name: '确认保存', exact: true }).click();
  assert.equal(await correction.getByLabel(/操作原因/).evaluate(input => input.checkValidity()), false, '缺少原因不能更正');
  await correction.getByLabel(/操作原因/).fill('核实车牌录错');
  await correction.getByRole('button', { name: '确认保存', exact: true }).click();
  await correction.waitFor({ state: 'detached' });
  await wb.getByText(/后台已修改当前任务或模拟现场条件/).waitFor();
  assert.equal(await wb.getByRole('button', { name: '采集模拟重量', exact: true }).isEnabled(), false, '任务车牌改变后须重新人工定位');
  assert.equal((await saved(wb)).tasks.find(t => t.id === 'TEST-T-001').records[0].capture.plate, '晋A10001');

  // 操作员隔夜续称、双向调整、作废后只重称皮重，复用工作台和详情真实入口。
  await command(management, { type: 'config', patch: { role: 'operator', weight: 10 } });
  await wb.setViewportSize({ width: 1366, height: 768 });
  await wb.goto(base + '/weighing/workbench?task=TEST-T-007');
  await wb.getByText(/隔日回磅：沿用原任务/).waitFor();
  await capture(wb);
  await wb.getByRole('button', { name: '开始核对回磅（皮重）', exact: true }).click();
  const overnight = wb.getByRole('dialog', { name: '核对回磅（皮重）', exact: true });
  await overnight.getByLabel('最终毛重（吨）', { exact: true }).fill('33');
  await overnight.getByLabel('最终皮重（吨）', { exact: true }).fill('11');
  await overnight.getByLabel('调整备注（选填）', { exact: true }).fill('临时说明');
  await overnight.getByLabel('调整备注（选填）', { exact: true }).fill('');
  await overnight.getByText(/最终毛重高于原始值/).waitFor();
  await overnight.getByText(/最终皮重高于原始值/).waitFor();
  await overnight.getByRole('button', { name: '确认称重并出单', exact: true }).click();
  await wb.getByRole('button', { name: '办理下一辆车', exact: true }).waitFor();
  const overnightState = await saved(wb);
  const originalTicket = overnightState.tickets.find(t => t.taskId === 'TEST-T-007');
  assert.equal(originalTicket.adjustment.reason, '磅精度调整');
  assert.equal(originalTicket.adjustment.grossDeduction, -1);
  assert.equal(originalTicket.adjustment.tareDeduction, -1);
  assert.equal(originalTicket.records[0].weight, 32);
  assert.equal(originalTicket.records[1].weight, 10);
  assert.ok(overnightState.tasks.find(t => t.id === 'TEST-T-007').history.some(h => /隔日续称/.test(h.action)));
  await management.goto(base + `/weighing/tickets/${originalTicket.id}`);
  await management.getByRole('button', { name: '作废测试单', exact: true }).click();
  const voidDialog = management.getByRole('dialog', { name: '作废测试单', exact: true });
  await voidDialog.getByLabel(/操作原因/).fill('皮重需要重新采集');
  await voidDialog.getByRole('button', { name: '确认保存', exact: true }).click();
  await voidDialog.waitFor({ state: 'detached' });
  await command(management, { type: 'config', patch: { weight: 11 } });
  await management.goto(base + '/weighing/tasks/TEST-T-007');
  await management.getByRole('button', { name: '单磅重称', exact: true }).click();
  const reweigh = management.getByRole('dialog', { name: '单磅重称', exact: true });
  assert.equal(await reweigh.getByLabel('目标称重', { exact: true }).inputValue(), 'second');
  await reweigh.getByRole('button', { name: '重新采集模拟重量', exact: true }).click();
  await reweigh.getByRole('checkbox', { name: '已人工核对车辆、重量和本次模拟抓拍图片', exact: true }).check();
  await reweigh.getByRole('checkbox', { name: '已确认车辆装载状态仍符合原称重条件', exact: true }).check();
  assert.equal(await reweigh.getByRole('button', { name: '确认当前采集重称', exact: true }).isDisabled(), true, '另一磅未确认不得沿用');
  await reweigh.getByRole('checkbox', { name: '已确认沿用的另一磅仍有效（不重新采集）', exact: true }).check();
  await reweigh.getByLabel(/操作原因/).fill('车辆空载，复核皮重');
  await reweigh.getByRole('button', { name: '确认当前采集重称', exact: true }).click();
  await reweigh.waitFor({ state: 'detached' });
  await management.goto(base + `/weighing/tickets/${originalTicket.id}`);
  await management.getByRole('button', { name: '关联更正重开', exact: true }).click();
  const reopen = management.getByRole('dialog', { name: '关联更正重开', exact: true });
  assert.equal(await reopen.getByLabel('最终毛重（吨）', { exact: true }).inputValue(), '32');
  assert.equal(await reopen.getByLabel('最终皮重（吨）', { exact: true }).inputValue(), '11');
  await reopen.getByRole('checkbox', { name: /已核验本次采用的两磅依据仍有效/ }).check();
  await reopen.getByLabel(/操作原因/).fill('沿用毛重，按新皮重重开');
  await reopen.getByRole('button', { name: '确认保存', exact: true }).click();
  await reopen.waitFor({ state: 'detached' });
  const reopenedState = await saved(management);
  const currentTicket = reopenedState.tickets.find(t => t.previousId === originalTicket.id);
  assert.equal(currentTicket.net, 21);
  assert.equal(currentTicket.records.length, 3);
  assert.equal(currentTicket.records[2].replaces, originalTicket.records[1].id);
  assert.equal(currentTicket.records[2].retainedRecordId, originalTicket.records[0].id);
  assert.deepEqual(reopenedState.tickets.find(t => t.id === originalTicket.id).records, originalTicket.records);
  assert.deepEqual(reopenedState.tickets.find(t => t.id === originalTicket.id).adjustment, originalTicket.adjustment);
  await wait(wb, d => d.tickets.some(t => t.id === currentTicket.id), '另一标签同步重开单');
  await management.reload();
  await management.getByRole('heading', { name: '测试过磅单详情', exact: true }).waitFor();
  assert.equal((await saved(management)).tasks.find(t => t.id === 'TEST-T-007').records.length, 3);
  await management.goto(base + `/weighing/tickets/${originalTicket.id}/preview`);
  await management.getByText('33 吨', { exact: true }).waitFor();
  await management.getByText('22 吨', { exact: true }).waitFor();
  await management.goto(base + `/weighing/tickets/${currentTicket.id}/preview`);
  await management.getByText('21 吨', { exact: true }).waitFor();
  await management.goto(base + '/weighing/originals');
  await management.getByLabel('车牌或编号', { exact: true }).fill('TEST-T-007');
  assert.equal(await management.getByRole('link', { name: 'TEST-T-007', exact: true }).count(), 1);
  assert.match(await management.locator('main').innerText(), /原始净重合计 21 吨/);
  const correctionDownload = management.waitForEvent('download');
  await management.getByRole('button', { name: '导出明细 CSV', exact: true }).click();
  await correctionDownload;

  // 旧缓存特殊测试值保持原样；直接访问仍受真实鉴权外壳保护。
  const legacyData = await saved(wb);
  legacyData.tasks.find(t => t.id === 'TEST-T-001').plate = '川A·示例01';
  const legacyContext = await browser.newContext();
  await legacyContext.route(url => url.pathname.startsWith('/api/'), route => route.fulfill({ json: { code: 200, message: 'success', data: [] } }));
  const legacyPage = await legacyContext.newPage();
  await legacyPage.goto(base + '/weighing/workbench?task=TEST-T-001');
  await legacyPage.waitForURL('**/login');
  await legacyPage.evaluate(({ key, data }) => {
    localStorage.setItem(key, JSON.stringify(data));
    localStorage.setItem('react-admin-auth', JSON.stringify({ token: 'legacy-test', user: { id: 1, username: 'legacy', roles: [] } }));
  }, { key, data: legacyData });
  await legacyPage.goto(base + '/weighing/workbench?task=TEST-T-001');
  await legacyPage.getByText(/历史测试车牌：可沿用原值/).waitFor();
  assert.equal((await saved(legacyPage)).tasks.find(t => t.id === 'TEST-T-001').plate, '川A·示例01');
  await legacyContext.close();
  assert.deepEqual(errors, []);
  console.log('standalone workbench passed: actual menus/routes/window reuse, multi-tab preservation, recognition/manual correction, both directions, evidence, desktop/mobile, overnight/upward/default-note/single-reweigh/reopen snapshots');
} catch (error) { for (const page of debugContext?.pages() || []) console.error(page.url(), (await page.locator('body').innerText()).slice(-2000)); throw error; } finally { clearTimeout(deadline); await browser?.close(); vite.kill('SIGTERM'); }
