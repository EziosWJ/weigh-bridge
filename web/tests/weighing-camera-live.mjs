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
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 100_000);
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
  let holdOffer = false;
  let releaseOffer;
  let held;
  let businessRequests = 0;
  page.on('request', request => { if (request.method() !== 'GET' && request.url().includes('/api/weighing/')) businessRequests++; });
  let videoAccess = true;
  let videoEnabled = true;
  let videoRevision = 'active-1';
  const previewRoute = route => {
    if (!videoAccess) return route.fulfill({ status: 403, json: { code: 403 } });
    const scale = new URL(route.request().url()).pathname.split('/')[4];
    return route.fulfill({ json: { code: 200, data: scale !== '1' ? { mode: 'legacy' } : {
      mode: 'mediamtx', enabled: videoEnabled, ready: videoEnabled, revision: videoRevision,
      front: `${base}/media/front/whep`, rear: `${base}/media/rear/whep`,
      message: videoEnabled ? '' : '摄像头已停用',
    } } });
  };
  await page.route('**/api/weighing/scales/*/camera-preview', previewRoute);
  // The WHEP fixture negotiates against a real browser peer connection and
  // canvas video track. It exercises the shipped reader, not a mock player.
  await page.addInitScript(() => { window.mediaSessions = new Map(); });
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
    if (holdOffer) {
      held = new Promise(resolve => { releaseOffer = resolve; });
      await held;
      return route.fulfill({ status: 503 }).catch(() => {});
    }
    const sessionPath = `${path}/session-${posts}`;
    const answer = await page.evaluate(async ({ sdp, sessionPath, rear }) => {
      const pc = new RTCPeerConnection({ iceServers: [] });
      const canvas = document.createElement('canvas');
      canvas.width = 320; canvas.height = 180;
      const context = canvas.getContext('2d');
      let frame = 0;
      const draw = () => {
        context.fillStyle = rear ? '#245e36' : '#245e86';
        context.fillRect(0, 0, 320, 180);
        context.fillStyle = 'white';
        context.fillText(`${rear ? 'REAR' : 'FRONT'} ${frame++}`, 30, 60);
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
    }, { sdp: request.postData(), sessionPath, rear: path.includes('/rear/') });
    await route.fulfill({ status: 201, contentType: 'application/sdp', headers: { Location: sessionPath }, body: answer });
  });

  const feed = page.locator('.camera-live-feed');
  const live = () => page.waitForFunction(() => document.querySelector('.camera-live-feed')?.dataset.cameraState === 'live');
  const sessions = () => page.evaluate(() => window.mediaSessions.size);
  await page.goto(`${base}/tests/weighing-camera-live.html`);
  await live();
  assert.equal(posts, 1);
  assert.equal(await sessions(), 1);

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
  await page.getByRole('img', { name: '车头视角抓拍', exact: true }).waitFor();
  await page.getByRole('button', { name: '放大实时画面', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '车头视角实时画面', exact: true });
  await dialog.waitFor();
  assert.equal(await page.evaluate(() => {
    const videos = [...document.querySelectorAll('video')];
    return videos.length === 2 && videos[0].srcObject === videos[1].srcObject;
  }), true, '放大必须复用同一个 MediaStream');
  assert.equal(posts, beforeHidden + 1, '放大不得新增 WHEP 连接');
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden' });
  assert.equal(await page.getByRole('button', { name: '放大实时画面', exact: true }).evaluate(button => button === document.activeElement), true);
  await page.getByRole('button', { name: '车尾视角', exact: true }).click();
  await live();
  assert.equal(posts, beforeHidden + 2);
  assert.equal(deletes, 2, '隐藏页面和切换视角须关闭并删除旧会话');
  assert.equal(await sessions(), 1);

  // The transport stays connected, but no new source frames arrive.
  await page.evaluate(() => { for (const session of window.mediaSessions.values()) clearInterval(session.timer); });
  await page.waitForFunction(() => document.querySelector('.camera-live-feed')?.dataset.cameraState === 'reconnecting');
  await live();
  assert.equal(posts, beforeHidden + 3, '画面冻结须重建连接');
  assert.equal(await sessions(), 1);

  fail = true;
  await page.getByRole('button', { name: '车头视角', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('.camera-live-feed')?.dataset.cameraState === 'reconnecting');
  assert.equal(await feed.locator('video').evaluate(video => video.srcObject), null, '失败不能继续展示旧流');
  fail = false;
  await page.getByRole('button', { name: '重新连接', exact: true }).click();
  await live();
  await page.getByRole('button', { name: '切换地磅', exact: true }).click();
  await page.waitForFunction(() => window.mediaSessions.size === 0);
  assert.equal(await feed.count(), 0, '其他地磅不能播放该地磅的配置');
  const before = posts;
  await delay(2200);
  assert.equal(posts, before, '离开实时监控后不能遗留重连定时器');

  await page.getByRole('button', { name: '切换地磅', exact: true }).click();
  await live();
  const beforeActivation = posts;
  videoRevision = 'active-2';
  await page.waitForFunction(() => window.mediaSessions.size === 1);
  for (let n = 0; n < 100 && posts === beforeActivation; n++) await delay(50);
  assert.equal(posts, beforeActivation + 1, '生效版本变化必须替换现有播放器');
  await live();
  videoEnabled = false;
  await page.getByText('摄像头已停用', { exact: true }).first().waitFor();
  await page.waitForFunction(() => window.mediaSessions.size === 0);
  videoEnabled = true;
  await live();
  videoAccess = false;
  await page.getByText('无权读取摄像头配置', { exact: true }).first().waitFor();
  await page.waitForFunction(() => window.mediaSessions.size === 0);
  videoAccess = true;
  await live();
  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, '窄屏不得横向溢出');
  await page.screenshot({ path: '/tmp/weighbridge-camera-live-mobile.png' });
  await page.getByRole('button', { name: '切换挂载', exact: true }).click();
  await page.waitForFunction(() => window.mediaSessions.size === 0);
  holdOffer = true;
  const beforeHold = posts;
  await page.getByRole('button', { name: '切换挂载', exact: true }).click();
  for (let n = 0; n < 100 && posts === beforeHold; n++) await delay(50);
  assert.equal(posts, beforeHold + 1, '应在超时前发起协商');
  await page.getByRole('button', { name: '切换挂载', exact: true }).click();
  releaseOffer();
  await delay(2200);
  assert.equal(posts, beforeHold + 1, '协商中卸载不能因异步响应重建播放器');
  assert.equal(businessRequests, 0, '播放及恢复不能触发业务采集或确认');

  // A failed preview must not become an extra gate for formal Agent capture.
  holdOffer = false;
  fail = true;
  let samples = 0;
  let confirmations = 0;
  await page.route('**/api/weighing/**', async route => {
    const path = new URL(route.request().url()).pathname;
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
  await page.route('**/api/weighing/scales/*/camera-preview', previewRoute);
  await page.goto(`${base}/tests/weighing-agent-capture.html?scaleId=1&scenario=ready`);
  await page.waitForFunction(() => document.querySelector('.camera-live-feed')?.dataset.cameraState === 'reconnecting');
  const captureButton = page.getByRole('button', { name: '采集重量', exact: true });
  assert.equal(await captureButton.isEnabled(), true, '视频故障不阻塞原有健康设备的采集');
  await captureButton.click();
  await page.getByText('测试：Agent 收到采集请求', { exact: true }).waitFor();
  assert.equal(samples, 1);
  assert.equal(confirmations, 0);
  assert.deepEqual(errors, []);
  console.log('实时监控通过：真实 WHEP 协商与解码、双视角切换、放大复用、冻结/失败重连、隐藏暂停、地磅隔离、卸载清理、窄屏及视频故障不阻塞 Agent 采集。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
