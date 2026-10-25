import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4195';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4195', '--strictPort'], {
  cwd: new URL('..', import.meta.url), stdio: 'ignore',
  env: process.env,
});
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 120_000);
try {
  for (let n = 0; n < 80; n++) {
    try { if ((await fetch(`${base}/tests/weighing-camera-live.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  page.setDefaultTimeout(15_000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  let posts = 0;
  let deletes = 0;
  let fail = false;
  let previewAccess = true;
  let cameraEnabled = true;
  let streamMode = 'synced';
  let businessRequests = 0;
  page.on('request', request => { if (request.method() !== 'GET' && request.url().includes('/api/weighing/')) businessRequests++; });
  await page.addInitScript(() => { window.mediaSessions = new Map(); });

  const streamFor = id => ({
    path: `cam_${id}`, playbackUrl: cameraEnabled && streamMode === 'synced' ? `${base}/media/${id === 11 ? 'entry' : 'yard'}/whep` : '',
    desiredVersion: 2, syncedVersion: streamMode === 'synced' ? 2 : 1,
    state: streamMode, lastError: streamMode === 'error' ? '测试：MediaMTX 不可达' : '', updatedAt: new Date().toISOString(),
  });
  const cameraList = () => ({
    cameras: [
      { id: 11, publicId: 'cam_entry', name: '入口', sourceUrl: 'rtsp://camera.local/entry', username: 'viewer', hasPassword: true, captureEnabled: true, enabled: cameraEnabled, version: 3, stream: streamFor(11), createTime: new Date().toISOString(), updateTime: new Date().toISOString() },
      { id: 12, publicId: 'cam_yard', name: '装车区', sourceUrl: 'rtsp://camera.local/yard', username: '', hasPassword: false, captureEnabled: true, enabled: cameraEnabled, version: 2, stream: streamFor(12), createTime: new Date().toISOString(), updateTime: new Date().toISOString() },
    ],
    configVersion: 4, captureTimeoutMs: 6000, recognitionCameraId: 11, recognitionUrl: 'https://recognizer.local/plate',
    video: { apiUrl: 'http://mediamtx.local:9997', apiUsername: 'admin', hasApiPassword: true, whepBaseUrl: base, version: 2, syncState: 'synced', syncedVersion: 2, updateTime: new Date().toISOString() },
  });
  await page.route('**/api/weighing/cameras', route => {
    if (!previewAccess) return route.fulfill({ status: 403, json: { code: 403, message: '无权读取摄像头配置' } });
    return route.fulfill({ json: { code: 200, data: cameraList() } });
  });
  await page.route('**/api/weighing/cameras/*/preview', route => {
    if (!previewAccess) return route.fulfill({ status: 403, json: { code: 403, message: '无权读取摄像头配置' } });
    const id = Number(new URL(route.request().url()).pathname.split('/')[5]);
    return route.fulfill({ json: { code: 200, data: streamFor(id) } });
  });
  // The WHEP fixture negotiates against a real browser peer connection and
  // canvas video track. It exercises the shipped reader, not a mock player.
  await page.route('**/media/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    assert.equal(request.headers().authorization, undefined, 'MediaMTX 直连不附加业务登录凭据');
    if (request.method() === 'OPTIONS') return route.fulfill({ status: 204 });
    if (request.method() === 'DELETE') {
      deletes++;
      await page.evaluate(path => {
        const session = window.mediaSessions.get(path);
        if (!session) return;
        clearInterval(session.timer);
        session.stream.getTracks().forEach(track => track.stop());
        session.pc.close();
        window.mediaSessions.delete(path);
      }, path);
      return route.fulfill({ status: 204 });
    }
    if (request.method() === 'PATCH') {
      await page.evaluate(async ({ path, body }) => {
        const session = window.mediaSessions.get(path);
        if (!session) return;
        const lines = body.split('\r\n');
        const mid = lines.find(line => line.startsWith('a=mid:'))?.slice(6) ?? '0';
        for (const line of lines.filter(line => line.startsWith('a=candidate:'))) {
          await session.pc.addIceCandidate({ candidate: line.slice(2), sdpMid: mid });
        }
      }, { path, body: request.postData() });
      return route.fulfill({ status: 204 });
    }
    assert.equal(request.method(), 'POST');
    posts++;
    if (fail) return route.fulfill({ status: 503 });
    const sessionPath = `${path}/session-${posts}`;
    const answer = await page.evaluate(async ({ sdp, sessionPath, yard }) => {
      const pc = new RTCPeerConnection({ iceServers: [] });
      const canvas = document.createElement('canvas');
      canvas.width = 320; canvas.height = 180;
      const context = canvas.getContext('2d');
      let frame = 0;
      const draw = () => {
        context.fillStyle = yard ? '#245e36' : '#245e86';
        context.fillRect(0, 0, 320, 180);
        context.fillStyle = 'white';
        context.fillText(`${yard ? 'YARD' : 'ENTRY'} ${frame++}`, 30, 60);
      };
      draw();
      const stream = canvas.captureStream(15);
      const timer = setInterval(draw, 65);
      window.mediaSessions.set(sessionPath, { pc, stream, timer });
      await pc.setRemoteDescription({ type: 'offer', sdp });
      stream.getTracks().forEach(track => pc.addTrack(track, stream));
      await pc.setLocalDescription(await pc.createAnswer());
      if (pc.iceGatheringState !== 'complete') await new Promise(resolve => {
        pc.addEventListener('icegatheringstatechange', () => { if (pc.iceGatheringState === 'complete') resolve(); });
      });
      return pc.localDescription.sdp;
    }, { sdp: request.postData(), sessionPath, yard: path.includes('/yard/') });
    await route.fulfill({ status: 201, contentType: 'application/sdp', headers: { Location: sessionPath }, body: answer });
  });

  const live = () => page.waitForFunction(() => document.querySelector('.camera-live-feed')?.dataset.cameraState === 'live');
  const sessions = () => page.evaluate(() => window.mediaSessions.size);
  const cameraGroup = () => page.getByRole('group', { name: '摄像头', exact: true });
  await page.goto(`${base}/tests/weighing-camera-live.html`);
  await live();
  assert.equal(posts, 1);
  assert.equal(await sessions(), 1);
  assert.ok(await cameraGroup().getByRole('button', { name: /^入口/ }).getAttribute('aria-pressed') === 'true', '默认选中第一台启用摄像头');
  assert.equal(await page.getByText('尚未配置摄像头', { exact: false }).count(), 0);

  const beforeHidden = posts;
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, value: true });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await page.waitForFunction(() => window.mediaSessions.size === 0);
  await delay(1200);
  assert.equal(posts, beforeHidden, '页面隐藏时停止播放及重连');
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, value: false });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await live();
  assert.equal(await sessions(), 1);

  // All camera evidence is visible together and each thumbnail opens its own image.
  const captureGroup = page.getByRole('group', { name: '抓拍摄像头', exact: true });
  await page.getByRole('img', { name: '入口抓拍', exact: true }).waitFor();
  await page.getByRole('img', { name: '装车区抓拍', exact: true }).waitFor();
  await page.getByText('已取得 2/2 张', { exact: true }).waitFor();
  await captureGroup.getByRole('button', { name: '放大抓拍 装车区', exact: true }).click();
  const photoDialog = page.getByRole('dialog', { name: '抓拍图片预览', exact: true });
  await photoDialog.getByRole('img', { name: '装车区抓拍大图', exact: true }).waitFor();
  await page.keyboard.press('Escape');
  await photoDialog.waitFor({ state: 'hidden' });
  assert.equal(await captureGroup.getByRole('button', { name: '放大抓拍 装车区', exact: true }).evaluate(button => button === document.activeElement), true);
  for (const width of [1864, 1366, 390]) {
    await page.setViewportSize({ width, height: 844 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, '抓拍缩略图不得撑破视口');
  }
  await page.setViewportSize({ width: 1366, height: 768 });
  await page.getByRole('button', { name: '等待抓拍', exact: true }).click();
  await page.getByText('等待采集', { exact: true }).waitFor();
  assert.ok((await page.locator('.workbench-camera-evidence').boundingBox()).height <= 128, '等待空态保持紧凑');
  assert.equal(await page.locator('.workbench-camera-evidence [role="alert"]').count(), 0, '等待采集不是故障');
  await page.getByRole('button', { name: '无图抓拍', exact: true }).click();
  await page.getByText('本次无图', { exact: true }).waitFor();
  await page.getByText('本次无抓拍图片，请核对现场车辆和重量。', { exact: true }).waitFor();
  await page.getByRole('button', { name: '部分失败', exact: true }).click();
  await page.getByText('已取得 1 张 · 抓拍不完整', { exact: true }).waitFor();
  await page.getByText('装车区：摄像头连接超时', { exact: true }).waitFor();
  await page.getByRole('button', { name: '完整抓拍', exact: true }).click();
  await page.getByText('已取得 2/2 张', { exact: true }).waitFor();

  await page.getByRole('button', { name: '放大实时画面', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '入口实时画面', exact: true });
  await dialog.waitFor();
  assert.equal(await page.evaluate(() => {
    const videos = [...document.querySelectorAll('video')];
    return videos.length === 2 && videos[0].srcObject === videos[1].srcObject;
  }), true, '放大必须复用同一个 MediaStream');
  assert.equal(posts, beforeHidden + 1, '放大不得新增 WHEP 连接');
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden' });
  assert.equal(await page.getByRole('button', { name: '放大实时画面', exact: true }).evaluate(button => button === document.activeElement), true);
  await cameraGroup().getByRole('button', { name: /^装车区/ }).click();
  await live();
  assert.equal(posts, beforeHidden + 2);
  assert.ok(deletes >= 2, '隐藏页面和切换摄像头须关闭并删除旧会话');
  assert.equal(await sessions(), 1);

  // The transport stays connected, but no new source frames arrive.
  await page.evaluate(() => { for (const session of window.mediaSessions.values()) clearInterval(session.timer); });
  await page.waitForFunction(() => document.querySelector('.camera-live-feed')?.dataset.cameraState === 'reconnecting');
  await live();
  assert.equal(posts, beforeHidden + 3, '画面冻结须重建连接');
  assert.equal(await sessions(), 1);

  fail = true;
  await cameraGroup().getByRole('button', { name: /^入口/ }).click();
  await page.waitForFunction(() => document.querySelector('.camera-live-feed')?.dataset.cameraState === 'reconnecting');
  assert.equal(await page.locator('.camera-live-feed video').evaluate(video => video.srcObject), null, '失败不能继续展示旧流');
  fail = false;
  await page.getByRole('button', { name: '重新连接', exact: true }).click();
  await live();

  // Preview read failures are displayed separately and do not affect capture evidence.
  previewAccess = false;
  await cameraGroup().getByRole('button', { name: /^装车区/ }).click();
  await page.getByText('预览信息读取失败', { exact: false }).waitFor();
  await page.waitForFunction(() => window.mediaSessions.size === 0);
  assert.equal(await page.getByRole('img', { name: '入口抓拍', exact: true }).count() + await page.getByRole('img', { name: '装车区抓拍', exact: true }).count(), 2, '预览失败仍保留全部本次抓拍证据');
  previewAccess = true;
  await page.getByRole('button', { name: '重新读取预览', exact: true }).click();
  await live();

  // Disabled cameras and pending map states are explicit, not a fake old image.
  cameraEnabled = false;
  await page.getByRole('button', { name: '切换挂载', exact: true }).click();
  await page.getByRole('button', { name: '切换挂载', exact: true }).click();
  await page.getByText('摄像头已停用', { exact: false }).waitFor();
  cameraEnabled = true;
  streamMode = 'pending';
  await page.getByRole('button', { name: '切换挂载', exact: true }).click();
  await page.getByRole('button', { name: '切换挂载', exact: true }).click();
  await page.getByText('视频映射待同步', { exact: false }).first().waitFor();
  streamMode = 'error';
  await page.getByRole('button', { name: '重新读取预览', exact: true }).click();
  await page.getByText('视频映射同步失败', { exact: false }).waitFor();
  assert.match(await page.getByText('视频映射同步失败', { exact: false }).first().innerText(), /MediaMTX 不可达/);
  streamMode = 'synced';
  await page.getByRole('button', { name: '重新读取预览', exact: true }).click();
  await live();

  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, '窄屏不得横向溢出');
  await page.getByRole('button', { name: '切换挂载', exact: true }).click();
  await page.waitForFunction(() => window.mediaSessions.size === 0);
  const afterUnmount = posts;
  await delay(1200);
  assert.equal(posts, afterUnmount, '卸载后不能遗留重连定时器');
  assert.equal(businessRequests, 0, '播放及恢复不能触发业务采集或确认');

  // A failed preview must not become an extra gate for formal capture.
  let samples = 0;
  let confirmations = 0;
  await page.route('**/api/weighing/**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/weighing/cameras') return route.fulfill({ json: { code: 200, data: { ...cameraList(), cameras: cameraList().cameras.map(camera => ({ ...camera, enabled: true })) } } });
    if (path.endsWith('/preview')) return route.fulfill({ json: { code: 200, data: { ...streamFor(11), state: 'error', playbackUrl: '', lastError: '测试：预览不可用' } } });
    if (path.includes('/session/')) return route.fulfill({ json: { code: 200, data: {
      scaleId: 1, holderUserId: 1, generation: 1, sessionToken: 'isolated-session', expiresAt: new Date(Date.now() + 60_000).toISOString(),
    } } });
    if (path.endsWith('/samples')) {
      samples++;
      return route.fulfill({ status: 409, json: { code: 409, message: '测试：Agent 收到采集请求' } });
    }
    if (path.includes('confirmation')) confirmations++;
    return route.fulfill({ json: { code: 200, data: { records: [], total: 0 } } });
  });
  await page.goto(`${base}/tests/weighing-agent-capture.html?scaleId=1&scenario=ready`);
  await page.getByText('视频映射同步失败', { exact: false }).waitFor();
  const captureButton = page.getByRole('button', { name: '采集重量', exact: true });
  assert.equal(await captureButton.isEnabled(), true, '视频故障不阻塞原有健康设备的采集');
  await captureButton.click();
  await page.getByText('测试：Agent 收到采集请求', { exact: true }).waitFor();
  assert.equal(samples, 1);
  assert.equal(confirmations, 0);
  assert.deepEqual(errors, []);
  console.log('实时监控通过：动态摄像头真实名称、WHEP 协商与解码、抓拍多图并排、等待/无图/部分失败状态、逐图放大与焦点恢复、放大复用、冻结/失败重连、隐藏暂停、台账隔离、预览失败/停用/待同步/同步失败表达、卸载清理、窄屏及视频故障不阻塞采集。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
