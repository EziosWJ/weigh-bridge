import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const webDir = new URL('..', import.meta.url);
const base = 'http://127.0.0.1:4196';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4196', '--strictPort'], { cwd: webDir, env: process.env, stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 120_000);

const jpeg = '/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wAALCAABAAEBAREA/8QAFAABAAAAAAAAAAAAAAAAAAAACf/EABQQAQAAAAAAAAAAAAAAAAAAAAD/2gAIAQEAAD8AVN//2Q==';
const now = () => new Date().toISOString();
const stream = (id, extra = {}) => ({ path: `cam_${id}`, playbackUrl: '', desiredVersion: 2, syncedVersion: 1, state: 'pending', lastError: '', updatedAt: now(), ...extra });
const camera = (id, name, extra = {}) => ({
  id, publicId: `cam_${id}`, name, sourceUrl: `rtsp://camera.local/${id}`, username: '', hasPassword: false,
  captureEnabled: true, enabled: true, version: 1, stream: stream(id), createTime: now(), updateTime: now(), ...extra,
});

try {
  for (let n = 0; n < 80; n++) {
    try { if ((await fetch(`${base}/tests/camera-management.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1366, height: 900 } });
  page.setDefaultTimeout(10_000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));

  let permissions = ['weighing:read', 'weighing:camera:view', 'weighing:camera:manage'];
  let cameras = [];
  let configVersion = 1;
  let captureTimeoutMs = 6000;
  let recognitionCameraId = null;
  let recognitionUrl = '';
  let video = { apiUrl: '', apiUsername: '', hasApiPassword: false, whepBaseUrl: '', version: 1, syncState: 'pending', syncError: '', syncedVersion: 0, updateTime: now() };
  let syncOK = true;
  const requests = [];
  const writes = [];
  const testCalls = [];
  const nextId = () => (cameras.reduce((max, item) => Math.max(max, item.id), 0) + 1);

  await page.route('**/api/weighing/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const method = request.method();
    const body = request.postData() ? JSON.parse(request.postData()) : null;
    if (!['GET', 'OPTIONS'].includes(method)) requests.push({ path, method, body });
    const data = value => route.fulfill({ json: { code: 200, message: 'ok', data: value } });
    if (path === '/api/weighing/capabilities') return data({ localSiteCode: 'F001', localScaleCode: 'WB01', configured: true, initialized: true, permissionCodes: permissions, sites: [], scales: [], userId: 1 });
    if (path === '/api/weighing/cameras' && method === 'GET') return data({ cameras, configVersion, captureTimeoutMs, recognitionCameraId, recognitionUrl, video });
    if (path === '/api/weighing/cameras' && method === 'POST') {
      writes.push(body);
      const id = nextId();
      cameras = [...cameras, camera(id, body.name, { username: body.username ?? '', hasPassword: !!body.password, captureEnabled: !!body.captureEnabled, enabled: body.enabled !== false })];
      configVersion += 1;
      return data(cameras.at(-1));
    }
    if (path === '/api/weighing/cameras/settings' && method === 'PUT') {
      writes.push(body);
      configVersion += 1;
      captureTimeoutMs = body.captureTimeoutMs;
      recognitionUrl = body.recognitionUrl ?? '';
      recognitionCameraId = body.clearRecognitionCamera ? null : (body.recognitionCameraId ?? recognitionCameraId);
      return data({ configVersion, captureTimeoutMs, recognitionCameraId, recognitionUrl });
    }
    if (path === '/api/weighing/cameras/video' && method === 'PUT') {
      writes.push(body);
      video = { ...video, apiUrl: body.apiUrl, apiUsername: body.apiUsername ?? '', whepBaseUrl: body.whepBaseUrl, version: video.version + 1, hasApiPassword: body.passwordAction === 'clear' ? false : (!!body.apiPassword || video.hasApiPassword) };
      return data(video);
    }
    if (path === '/api/weighing/cameras/sync' && method === 'POST') {
      writes.push(body);
      video = { ...video, syncState: syncOK ? 'synced' : 'error', syncError: syncOK ? '' : 'MediaMTX 不可达', syncedVersion: syncOK ? configVersion : video.syncedVersion };
      return data({ configVersion, videoVersion: video.version, state: video.syncState, error: video.syncError, streams: cameras.map(item => item.stream), at: now() });
    }
    const testMatch = path.match(/^\/api\/weighing\/cameras\/(\d+)\/test$/);
    if (testMatch && method === 'POST') {
      const target = cameras.find(item => item.id === Number(testMatch[1]));
      testCalls.push(target?.name);
      if (target?.name === '装车区') return data({ success: false, code: 'camera_unavailable', message: '摄像头不可用', at: now() });
      return data({ success: true, jpeg, width: 1, height: 1, at: now() });
    }
    const previewMatch = path.match(/^\/api\/weighing\/cameras\/(\d+)\/preview$/);
    if (previewMatch && method === 'GET') {
      const target = cameras.find(item => item.id === Number(previewMatch[1]));
      return data(target ? target.stream : stream(Number(previewMatch[1])));
    }
    const cameraMatch = path.match(/^\/api\/weighing\/cameras\/(\d+)$/);
    if (cameraMatch && method === 'PUT') {
      const id = Number(cameraMatch[1]);
      const index = cameras.findIndex(item => item.id === id);
      writes.push(body);
      if (cameras[index].version !== body.version) return route.fulfill({ json: { code: 400, message: '摄像头配置已变化，请刷新后重新核对' } });
      cameras[index] = { ...cameras[index], name: body.name, sourceUrl: body.sourceUrl, username: body.username ?? cameras[index].username, hasPassword: body.passwordAction === 'clear' ? false : (body.password ? true : cameras[index].hasPassword), captureEnabled: body.captureEnabled ?? cameras[index].captureEnabled, enabled: body.enabled ?? cameras[index].enabled, version: cameras[index].version + 1 };
      if (recognitionCameraId === id && (!cameras[index].captureEnabled || !cameras[index].enabled)) recognitionCameraId = null;
      configVersion += 1;
      return data(cameras[index]);
    }
    if (cameraMatch && method === 'DELETE') {
      const id = Number(cameraMatch[1]);
      writes.push(body);
      cameras = cameras.filter(item => item.id !== id);
      if (recognitionCameraId === id) recognitionCameraId = null;
      configVersion += 1;
      return data(null);
    }
    return data(null);
  });

  const open = () => page.goto(`${base}/tests/camera-management.html`);
  const addCamera = async (name, capture = true) => {
    await page.getByRole('button', { name: '新增摄像头', exact: true }).first().click();
    const dialog = page.getByRole('dialog', { name: '新增摄像头', exact: true });
    await dialog.getByLabel('名称', { exact: false }).fill(name);
    await dialog.getByLabel('完整源地址', { exact: false }).fill(`rtsp://camera.local/${encodeURIComponent(name)}`);
    await dialog.getByLabel('用户名', { exact: true }).fill('viewer');
    await dialog.getByLabel('密码', { exact: true }).fill('secret');
    if (capture) await dialog.getByLabel(/用于称重抓拍/).check();
    await dialog.getByRole('button', { name: '新增摄像头', exact: true }).click();
    await page.getByRole('cell', { name: new RegExp(name) }).waitFor();
  };

  await open();
  await page.getByText('尚未配置摄像头', { exact: true }).waitFor();
  await addCamera('入口');
  await addCamera('装车区');
  await addCamera('地磅北侧', false);
  assert.equal(writes.filter(body => body.name).length, 3, '支持 0..N 台摄像头');
  const created = writes.filter(body => body.name);
  assert.equal(created[0].captureEnabled, true);
  assert.equal(created[2].captureEnabled, false);
  assert.equal(await page.getByRole('cell', { name: /入口/ }).count() >= 1, true);
  assert.equal(await page.getByRole('cell', { name: /装车区/ }).count() >= 1, true);
  assert.equal(await page.getByRole('cell', { name: /地磅北侧/ }).count() >= 1, true);

  // Per-camera diagnostic test and its recent result time.
  const entryRow = page.getByRole('row').filter({ hasText: '入口' });
  await entryRow.getByRole('button', { name: '测试', exact: true }).click();
  const testDialog = page.getByRole('dialog', { name: /测试抓拍 · 入口/ });
  await testDialog.getByText('测试成功', { exact: true }).waitFor();
  await testDialog.getByRole('img', { name: '摄像头测试抓拍图片', exact: true }).waitFor();
  assert.deepEqual(testCalls, ['入口']);
  await testDialog.getByRole('button', { name: '关闭', exact: true }).click();
  await entryRow.getByText('成功', { exact: true }).waitFor();
  assert.ok((await entryRow.innerText()).includes('最近测试') === false, '表头不算行内结果');
  await page.getByRole('row').filter({ hasText: '装车区' }).getByRole('button', { name: '测试', exact: true }).click();
  await page.getByRole('dialog', { name: /测试抓拍 · 装车区/ }).getByText('摄像头不可用', { exact: false }).waitFor();
  await page.getByRole('dialog', { name: /测试抓拍 · 装车区/ }).getByRole('button', { name: '关闭', exact: true }).click();

  // Preview separately reflects pending video mapping and never runs a test.
  const beforePreviewTests = testCalls.length;
  await entryRow.getByRole('button', { name: '预览', exact: true }).click();
  const previewDialog = page.getByRole('dialog', { name: '入口 · 实时预览', exact: true });
  await previewDialog.getByText('视频映射待同步', { exact: false }).first().waitFor();
  assert.equal(testCalls.length, beforePreviewTests, '预览不得触发抓拍测试');
  await previewDialog.getByRole('button', { name: '关闭', exact: true }).click();

  // Editing a camera: blank password keeps the secret, explicit clear erases it.
  await entryRow.getByRole('button', { name: '编辑', exact: true }).click();
  const editDialog = page.getByRole('dialog', { name: '编辑摄像头', exact: true });
  await editDialog.getByRole('button', { name: '保存摄像头', exact: true }).click();
  await page.waitForFunction(() => !document.querySelector('[role="dialog"]'));
  let update = writes.filter(body => body.version && body.name === '入口').at(-1);
  assert.equal(update.passwordAction, 'keep', '编辑密码留空必须保持原值');
  await page.getByRole('row').filter({ hasText: '入口' }).getByRole('button', { name: '编辑', exact: true }).click();
  await page.getByRole('dialog', { name: '编辑摄像头', exact: true }).getByLabel(/清除已保存密码/).check();
  await page.getByRole('dialog', { name: '编辑摄像头', exact: true }).getByRole('button', { name: '保存摄像头', exact: true }).click();
  await page.waitForFunction(() => !document.querySelector('[role="dialog"]'));
  update = writes.filter(body => body.version && body.name === '入口').at(-1);
  assert.equal(update.passwordAction, 'clear', '清除密码必须使用显式 passwordAction=clear');

  // Recognition source: only capture cameras are offered; clearing asks the server to clear the choice.
  const recognitionSelect = page.getByLabel('识别源摄像头', { exact: true });
  assert.equal(await recognitionSelect.getByRole('option', { name: '地磅北侧', exact: true }).count(), 0, '仅预览摄像头不能作为识别源');
  await recognitionSelect.selectOption({ label: '入口' });
  await page.getByRole('button', { name: '保存抓拍设置', exact: true }).click();
  await page.getByText('抓拍设置已保存', { exact: false }).waitFor();
  let settingsWrite = writes.filter(body => body.configVersion && body.captureTimeoutMs).at(-1);
  assert.equal(settingsWrite.recognitionCameraId, 1);
  assert.equal(settingsWrite.clearRecognitionCamera, false);
  await recognitionSelect.selectOption({ label: '不指定（识别停用）' });
  await page.getByRole('button', { name: '保存抓拍设置', exact: true }).click();
  await page.getByText('抓拍设置已保存', { exact: false }).waitFor();
  settingsWrite = writes.filter(body => body.configVersion && body.captureTimeoutMs).at(-1);
  assert.equal(settingsWrite.clearRecognitionCamera, true, '清空识别源必须显式清除');

  // Delete sends the current version and announces that a deleted recognition source is disabled.
  await recognitionSelect.selectOption({ label: '入口' });
  await page.getByRole('button', { name: '保存抓拍设置', exact: true }).click();
  await page.getByText('抓拍设置已保存', { exact: false }).waitFor();
  await page.getByRole('row').filter({ hasText: '入口' }).getByRole('button', { name: '删除', exact: true }).click();
  const deleteDialog = page.getByRole('dialog', { name: '删除摄像头', exact: true });
  assert.ok((await deleteDialog.innerText()).includes('当前配置版本'));
  await deleteDialog.getByRole('button', { name: '确认删除', exact: true }).click();
  await page.getByText('识别源已停用', { exact: false }).waitFor();
  const deleteWrite = requests.filter(request => request.method === 'DELETE').at(-1);
  assert.equal(typeof deleteWrite.body.version, 'number', '删除必须携带版本请求体');

  // Video settings: version is always sent; blank password keeps the secret.
  await page.getByLabel('公开 WHEP 入口', { exact: true }).fill('http://mediamtx.local:8889');
  await page.getByLabel('MediaMTX 管理接口', { exact: true }).fill('http://mediamtx.local:9997');
  await page.getByRole('button', { name: '保存视频设置', exact: true }).click();
  await page.getByText('视频服务设置已保存', { exact: false }).waitFor();
  const videoWrite = writes.filter(body => body.whepBaseUrl !== undefined).at(-1);
  assert.equal(videoWrite.passwordAction, 'keep');
  assert.equal(typeof videoWrite.version, 'number');
  syncOK = false;
  await page.getByRole('button', { name: '同步视频源', exact: true }).click();
  await page.getByText('同步视频源失败', { exact: false }).waitFor();
  syncOK = true;
  await page.getByRole('button', { name: '同步视频源', exact: true }).click();
  await page.getByText('视频源已同步', { exact: false }).first().waitFor();

  // Read-only users keep preview but lose every mutation entry.
  permissions = ['weighing:read', 'weighing:camera:view'];
  await page.reload();
  await page.getByRole('cell', { name: /装车区/ }).waitFor();
  assert.equal(await page.getByRole('button', { name: '新增摄像头', exact: true }).count(), 0);
  assert.equal(await page.getByRole('button', { name: '测试', exact: true }).count(), 0);
  assert.equal(await page.getByRole('button', { name: '编辑', exact: true }).count(), 0);
  assert.equal(await page.getByText('当前账号仅可查看摄像头配置和实时预览', { exact: false }).count(), 1);
  await page.getByRole('row').filter({ hasText: '装车区' }).getByRole('button', { name: '预览', exact: true }).click();
  await page.getByRole('dialog', { name: '装车区 · 实时预览', exact: true }).waitFor();
  await page.keyboard.press('Escape');

  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, '窄屏不得横向溢出');
  assert.deepEqual(errors, []);
  console.log('摄像头管理回归通过：0/1/3 台、逐台测试与结果时间、预览与测试分开、密码留空保持与显式清除、识别源选择与删除停用提示、删除版本、视频配置版本/同步失败、只读权限及窄屏。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
