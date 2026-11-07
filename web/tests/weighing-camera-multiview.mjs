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
  let failedPreviewId = 0;
  let cameraEnabled = true;
  let streamMode = 'synced';
  let businessRequests = 0;
  page.on('request', request => { if (request.method() !== 'GET' && request.url().includes('/api/weighing/')) businessRequests++; });
  await page.addInitScript(() => { window.mediaSessions = new Map(); });

  const streamFor = id => ({
    path: `cam_${id}`, playbackUrl: cameraEnabled && streamMode === 'synced' ? `${base}/media/${id === 11 ? 'entry' : id === 12 ? 'yard' : id === 13 ? 'exit' : 'fourth'}/whep` : '',
    desiredVersion: 2, syncedVersion: streamMode === 'synced' ? 2 : 1,
    state: streamMode, lastError: streamMode === 'error' ? '测试：MediaMTX 不可达' : '', updatedAt: new Date().toISOString(),
  });
  const cameraList = () => ({
    cameras: [
      { id: 11, publicId: 'cam_entry', name: '入口', sourceUrl: 'rtsp://camera.local/entry', username: 'viewer', hasPassword: true, captureEnabled: true, enabled: cameraEnabled, version: 3, stream: streamFor(11), createTime: new Date().toISOString(), updateTime: new Date().toISOString() },
      { id: 13, publicId: 'cam_third', name: '出口', sourceUrl: 'rtsp://camera.local/third', username: '', hasPassword: false, captureEnabled: true, enabled: cameraEnabled, version: 2, stream: streamFor(13), createTime: new Date().toISOString(), updateTime: new Date().toISOString() },
      { id: 14, publicId: 'cam_fourth', name: '侧面', sourceUrl: 'rtsp://camera.local/fourth', username: '', hasPassword: false, captureEnabled: true, enabled: cameraEnabled, version: 2, stream: streamFor(14), createTime: new Date().toISOString(), updateTime: new Date().toISOString() },
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
    const id = Number(new URL(route.request().url()).pathname.split('/')[4]);
    if (id === failedPreviewId) return route.fulfill({ status: 503, json: { code: 503, message: '测试预览失败' } });
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

  await page.goto(`${base}/tests/weighing-camera-live.html`);
  await page.waitForFunction(() => document.querySelectorAll('.camera-live-feed[data-camera-state="live"]').length === 3);
  assert.equal(await page.evaluate(() => window.mediaSessions.size), 3);
  const main = await page.locator('.workbench-camera-tile.is-main').boundingBox();
  const small = await page.locator('.workbench-camera-tile:not(.is-main)').first().boundingBox();
  assert.ok(main.width > small.width && main.height > small.height && main.y > small.y, '两幅小画面位于主画面上方');
  const before = posts;
  await page.getByRole('button', { name: '放大 出口' }).click();
  assert.equal(await page.locator('.workbench-camera-tile.is-main video').getAttribute('aria-label'), '出口实时视频');
  assert.equal(posts, before, '切换画面不重建连接');
  await page.getByRole('combobox', { name: '选择摄像头' }).selectOption('12');
  await page.waitForFunction(() => document.querySelector('.workbench-camera-tile.is-main video')?.getAttribute('aria-label') === '装车区实时视频');
  await page.waitForFunction(() => document.querySelectorAll('.camera-live-feed[data-camera-state=live]').length === 3);
  assert.equal(await page.locator('.workbench-camera-tile').count(), 3, '额外相机替换一幅小画面');
  assert.equal(await page.evaluate(() => window.mediaSessions.size), 3);
  assert.equal(await page.getByRole('img', { name: '入口抓拍', exact: true }).count(), 1);
  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  failedPreviewId = 13;
  await page.reload();
  const failedTile = page.locator('.workbench-camera-tile').filter({ hasText: '预览信息读取失败' });
  await failedTile.waitFor();
  await page.waitForFunction(() => document.querySelectorAll('.camera-live-feed[data-camera-state=live]').length === 2);
  failedPreviewId = 0;
  await failedTile.getByRole('button', { name: '放大 出口' }).click();
  await failedTile.getByRole('button', { name: '重新读取预览' }).click();
  await page.waitForFunction(() => document.querySelectorAll('.camera-live-feed[data-camera-state=live]').length === 3);
  assert.deepEqual(errors, []);
  assert.equal(businessRequests, 0);
  console.log('多路视频播放、切换及抓拍展示通过');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
