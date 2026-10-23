import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4194';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4194', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
let browser;
let page;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 45_000);
try {
  for (let n = 0; n < 60; n++) {
    try { if ((await fetch(`${base}/tests/weighing-agent-capture.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  page = await browser.newPage({ viewport: { width: 1366, height: 900 } });
  page.setDefaultTimeout(5000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  let failCapture = false;
  let captures = 0;
  let confirmations = 0;
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jKxoAAAAASUVORK5CYII=', 'base64');
  await page.route('**/api/weighing/**', async route => {
    const path = new URL(route.request().url()).pathname;
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (path.includes('/agent-images/')) return route.fulfill({ contentType: 'image/png', body: png });
    if (path.includes('/session/')) return ok({ scaleId: 1, holderUserId: 1, generation: 1, sessionToken: 'isolated-session', expiresAt: new Date(Date.now() + 60000).toISOString() });
    if (path.endsWith('/samples')) {
      captures++;
      if (failCapture) return route.fulfill({ status: 409, json: { code: 409, message: '后摄像头抓拍失败' } });
      const images = ['front', 'rear'].map(view => ({ id: view, view, url: `/api/weighing/agent-images/${view}`, sha256: 'test-digest' }));
      return ok({ id: 901, taskId: null, stage: 'first', weight: '18.665', expiresAt: null,
        evidence: { id: '901', preset: 'front', source: 'agent', imageSrc: images[0].url, images, cameraLabel: '车头视角', at: new Date().toISOString(), weight: '18.665', plate: '晋A12345' },
        plateEvidence: { source: 'manual', confirmedPlate: '晋A12345', at: new Date().toISOString() },
      });
    }
    if (path.includes('confirmation')) confirmations++;
    return ok({ records: [], total: 0 });
  });
  const open = scenario => page.goto(`${base}/tests/weighing-agent-capture.html?scaleId=1&scenario=${scenario}`);
  await open('ready');
  await page.getByRole('heading', { name: '现场画面', exact: false }).waitFor();
  const capture = page.getByRole('button', { name: '采集重量', exact: true });
  assert.equal(await capture.isEnabled(), true, '真实 Agent 没有预览图片时，也必须允许发起首次采集');
  await capture.click();
  await page.waitForFunction(() => JSON.parse(document.querySelector('output').textContent).sampleReady);
  assert.equal(captures, 1, '应经过真实 store 向 /samples 发起采集');
  assert.equal(confirmations, 0, '采集不能自动确认正式称重');
  const business = page.getByRole('dialog', { name: '录入运输资料', exact: true });
  if (await business.isVisible()) await page.getByRole('button', { name: '关闭运输资料', exact: true }).click();
  await page.getByRole('img', { name: '车头视角预览', exact: true }).waitFor();
  await page.getByRole('button', { name: '车尾视角', exact: true }).click();
  await page.getByRole('img', { name: '车尾视角预览', exact: true }).waitFor();

  failCapture = true;
  await open('ready');
  await capture.click();
  await page.getByText('后摄像头抓拍失败', { exact: true }).waitFor();
  assert.equal(JSON.parse(await page.locator('output').textContent()).sampleReady, false);
  assert.equal(confirmations, 0);
  assert.equal(await capture.isEnabled(), true, '抓拍失败后可以人工重试');
  for (const scenario of ['offline', 'dynamic', 'fault', 'zero', 'forbidden', 'occupied']) {
    await open(scenario);
    await capture.waitFor();
    assert.equal(await capture.isDisabled(), true, `${scenario} 必须继续阻止采集`);
  }
  assert.deepEqual(errors, []);
  console.log('首次抓拍回归通过：无预览可请求采集、双视角展示、失败不生成采样、不自动确认、设备与权限限制保留。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
