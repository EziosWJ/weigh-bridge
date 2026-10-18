import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const port = 4182;
const baseUrl = `http://127.0.0.1:${port}`;
const fixture = `${baseUrl}/tests/weighing-smoke.html`;
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
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  page.setDefaultTimeout(5000);
  const fixtureHtml = await (await fetch(fixture)).text();
  await page.route(`${baseUrl}/weighing/**`, route => route.request().resourceType() === 'document' ? route.fulfill({ status: 200, contentType: 'text/html', body: fixtureHtml }) : route.continue());
  await page.goto(fixture, { waitUntil: 'networkidle' });
  await page.evaluate(async () => {
    const { useWeighingStore } = await import('/src/store/weighing-store.ts');
    const dispatch = useWeighingStore.getState().dispatch;
    await dispatch({ type: 'createTask', input: { plate: '晋AB6P44', direction: 'inbound', materialId: 'material-1', partyId: 'party-1', driver: '', remark: '' } });
    const taskId = useWeighingStore.getState().data.tasks[0].id;
    await dispatch({ type: 'config', patch: { weight: 5 } });
    await dispatch({ type: 'capture', taskId });
    await dispatch({ type: 'confirm', taskId, stage: 'first', checked: true });
  });
  await page.getByLabel(/^车牌/).fill('晋AB6P44');
  await page.getByRole('button', { name: '核对车牌并查找运输', exact: true }).click();
  const review = async () => {
    await page.getByRole('button', { name: '采集模拟重量', exact: true }).click();
    await page.getByRole('button', { name: '开始核对回磅（皮重）', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '核对回磅（皮重）', exact: true });
    return dialog;
  };
  let dialog = await review();
  await dialog.getByLabel('最终毛重（吨）', { exact: true }).fill('4.5');
  await dialog.getByLabel('最终皮重（吨）', { exact: true }).fill('1');
  const deduction = label => dialog.getByText(label, { exact: true }).locator('..');
  assert.match(await deduction('系统计算：毛重调整差额').innerText(), /0\.5 吨/, '原始净重为零时仍应实时计算毛重调整差额');
  assert.match(await deduction('系统计算：皮重调整差额').innerText(), /4 吨/);
  assert.equal(await dialog.getByRole('button', { name: '确认称重并出单', exact: true }).isDisabled(), true);
  assert.match(await dialog.getByRole('alert').last().innerText(), /无法出单：.*请返回.*重新采集/);
  const context = dialog.getByRole('region', { name: '本趟运输核对依据', exact: true });
  const adjustment = dialog.getByRole('region', { name: '最终重量与调整', exact: true });
  const contextBox = await context.boundingBox();
  const adjustmentBox = await adjustment.boundingBox();
  assert.ok(adjustmentBox.x > contextBox.x + contextBox.width, '桌面端业务依据与重量录入应左右排列');
  assert.ok((await dialog.boundingBox()).width >= 1000, '桌面核对窗口应横向扩大');
  await page.screenshot({ path: '/tmp/weighing-review-wide-invalid.png' });
  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth), true, '窄屏弹窗不能横向溢出');
  const narrowContext = await context.boundingBox();
  const narrowAdjustment = await adjustment.boundingBox();
  assert.ok(narrowAdjustment.y >= narrowContext.y + narrowContext.height, '窄屏核对内容应上下排列');
  const confirmBox = await dialog.getByRole('button', { name: '确认称重并出单', exact: true }).boundingBox();
  assert.ok(confirmBox.y >= 0 && confirmBox.y + confirmBox.height <= 844, '窄屏确认按钮应保持可见');
  await page.screenshot({ path: '/tmp/weighing-review-narrow.png' });
  await page.setViewportSize({ width: 1366, height: 768 });
  // 上调独立显示负差额，原始异常仍阻止出单；空白不沿用上次的值。
  await dialog.getByLabel('最终毛重（吨）', { exact: true }).fill('6');
  assert.match(await deduction('系统计算：毛重调整差额').innerText(), /-1 吨（上调）/);
  await dialog.getByLabel('最终毛重（吨）', { exact: true }).fill('');
  assert.match(await deduction('系统计算：毛重调整差额').innerText(), /—/);
  assert.match(await deduction('系统计算：皮重调整差额').innerText(), /4 吨/);
  await dialog.getByRole('button', { name: '不一致，返回修改', exact: true }).click();
  await page.getByRole('link', { name: '调整模拟重量 / 演示岗位', exact: true }).click();
  await page.getByLabel('模拟重量（吨）', { exact: true }).fill('1.2');
  await page.getByRole('link', { name: '返回称重作业', exact: true }).click();
  dialog = await review();
  await dialog.getByLabel('最终毛重（吨）', { exact: true }).fill('4.5');
  await dialog.getByLabel('最终皮重（吨）', { exact: true }).fill('1');
  await dialog.getByText('系统计算调整差额：毛重 0.5 吨 · 皮重 0.2 吨', { exact: true }).waitFor();
  await page.screenshot({ path: '/tmp/weighing-review-wide-valid.png' });
  await dialog.getByRole('button', { name: '确认称重并出单', exact: true }).click();
  await page.getByRole('heading', { name: '回磅已完成', exact: true }).waitFor();
  const data = await page.evaluate(() => JSON.parse(localStorage.getItem('weighbridge-phase2a-v1')));
  const ticket = data.tickets.find(item => item.plate === '晋AB6P44');
  assert.deepEqual([ticket.gross, ticket.tare, ticket.net], [4.5, 1, 3.5]);
  assert.deepEqual(ticket.records.map(item => item.weight), [5, 1.2]);
  console.log('weighing invalid original deductions and recovery passed');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
