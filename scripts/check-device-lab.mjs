// Isolated browser smoke with the real Server, Agent and a temporary PostgreSQL.
import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { once } from 'node:events';
import { access, mkdtemp, mkdir, readdir, rm } from 'node:fs/promises';
import { createServer } from 'node:net';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { startTestPostgres } from './test-postgres.mjs';

const root = fileURLToPath(new URL('..', import.meta.url));
const { chromium } = createRequire(join(root, 'web/package.json'))('playwright');
const run = promisify(execFile);
const temporary = await mkdtemp(join(tmpdir(), 'weighbridge-lab-smoke-'));
const runtime = join(temporary, 'runtime');
const children = [];
let database;
let browser;

function start(binary, args, env) {
  const child = spawn(binary, args, { cwd: join(root, 'server'), env, stdio: ['ignore', 'pipe', 'pipe'] });
  child.output = '';
  for (const stream of [child.stdout, child.stderr]) {
    stream.on('data', chunk => { child.output = (child.output + chunk).slice(-16000); });
  }
  child.done = once(child, 'exit');
  children.push(child);
  return child;
}

async function stop(child, signal = 'SIGTERM') {
  if (child.exitCode !== null || child.signalCode !== null) return child.done;
  child.kill(signal);
  const force = setTimeout(() => child.kill('SIGKILL'), 8000);
  try {
    return await child.done;
  } finally {
    clearTimeout(force);
  }
}

async function waitFor(description, check, timeout = 15000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const result = await check();
    if (result) return result;
    await delay(100);
  }
  throw new Error('等待超时：' + description);
}

async function unusedPort() {
  const socket = createServer();
  socket.listen(0, '127.0.0.1');
  await once(socket, 'listening');
  const { port } = socket.address();
  await new Promise((resolve, reject) => socket.close(error => error ? reject(error) : resolve()));
  return port;
}

async function startLab(api, serial) {
  const child = start(join(root, '.task/device-lab/bin/device-lab'), [
    '--listen', '127.0.0.1:0', '--server', api, '--serial', serial, '--heartbeat', '1s',
  ], { ...process.env, TMPDIR: runtime });
  const url = await waitFor('联调台地址', () => {
    if (child.exitCode !== null) throw new Error(child.output);
    return child.output.match(/设备联调台 (http:\/\/127\.0\.0\.1:\d+)/)?.[1];
  });
  return { child, url };
}

try {
  await mkdir(runtime);
  database = await startTestPostgres();
  const address = '127.0.0.1:' + await unusedPort();
  const api = 'http://' + address;
  const env = {
    ...process.env, ...database.env, APP_ENV: 'test',
    APP_HTTP__ADDRESS: address, APP_JWT__SECRET: 'device-lab-isolated-test-secret',
    APP_FILE__STORAGE_ROOT: join(temporary, 'uploads'),
    APP_WEIGHING__IMAGE_DIR: join(temporary, 'images'),
    APP_MONITORING__ENABLED: 'false', WEIGHBRIDGE_CAMERA_EVENT_ALLOWED_IPS: '',
  };
  const options = { cwd: join(root, 'server'), env, timeout: 60000 };
  await run('go', ['run', './cmd/migrate', 'up', '--kind', 'all'], options);
  await run('go', ['build', '-o', join(temporary, 'api'), './cmd/api'], options);
  const server = start(join(temporary, 'api'), [], env);
  await waitFor('开发 Server 就绪', async () => {
    if (server.exitCode !== null) throw new Error(server.output);
    try {
      return (await fetch(api + '/ready', { signal: AbortSignal.timeout(1000) })).ok;
    } catch {
      return false;
    }
  });

  let token;
  async function call(path, method = 'GET', body) {
    const headers = { 'Content-Type': 'application/json' };
    if (token) headers.Authorization = token;
    const response = await fetch(api + path, {
      method, headers, body: JSON.stringify(body), signal: AbortSignal.timeout(3000),
    });
    const result = await response.json();
    assert.equal(result.code, 200, result.message);
    return result.data;
  }
  token = (await call('/api/auth/login', 'POST', { username: 'admin', password: 'admin123' })).tokenValue;
  assert.ok(token);
  await call('/api/weighing/site-config/initialize', 'POST', { siteCode: 'LAB01', scaleCode: 'WB01' });
  const camera = await call('/api/weighing/cameras', 'POST', {
    name: '联调摄像头', sourceUrl: 'rtsp://camera/simulator',
  });
  const candidatePath = '/api/weighing/plate-candidates?scale=WB01';
  const baseline = await call(candidatePath);
  assert.ok(baseline.nextCursor);

  const lab = await startLab(api, 'camera-sim-01');
  browser = await chromium.launch({
    headless: true,
    ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}),
  });
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto(lab.url);
  await page.getByRole('heading', { name: '地磅', exact: true }).waitFor();
  await page.getByRole('heading', { name: '车牌识别', exact: true }).waitFor();
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  await page.locator('#agent[data-state="connected"]').waitFor();
  await page.getByText('已打开', { exact: true }).waitFor();
  const port = await page.locator('#port').textContent();
  assert.match(port, /^\/dev\/pts\/\d+$/);

  await page.getByLabel('目标重量（吨）').fill('30.125');
  await page.getByRole('button', { name: '模拟上磅', exact: true }).click();
  const devicePath = '/api/weighing/scales/WB01/device';
  await waitFor('Server 收到首磅稳定重量', async () => {
    const state = await call(devicePath);
    return state.online && state.stable && Number(state.weight) === 30.125;
  });

  await page.getByLabel('指定车牌').fill('晋A12345');
  await page.getByRole('button', { name: '上报指定车牌', exact: true }).click();
  await page.locator('#report').filter({ hasText: /HTTP 403/ }).waitFor();
  const failedEvent = await page.locator('#event').textContent();
  await call(`/api/weighing/cameras/${camera.id}/events`, 'PUT', {
    serialNumber: 'camera-sim-01', enabled: true, timeoutSeconds: 180,
  });
  await page.getByRole('button', { name: '重试上次上报', exact: true }).click();
  await page.locator('#report').filter({ hasText: /已收到 ACK/ }).waitFor();
  assert.equal(await page.locator('#event').textContent(), failedEvent);
  await page.locator('#heartbeat').filter({ hasText: /已收到 ACK/ }).waitFor();

  await page.getByRole('button', { name: '下磅归零', exact: true }).click();
  await page.getByLabel('目标重量（吨）').fill('12.5');
  await page.getByRole('button', { name: '设置稳定重量', exact: true }).click();
  await waitFor('Server 收到回磅稳定重量', async () => {
    const state = await call(devicePath);
    return state.online && state.stable && Number(state.weight) === 12.5;
  });
  await page.getByRole('button', { name: '再次上报当前车牌', exact: true }).click();
  await waitFor('工作台候选收到同车第二次上报', async () => {
    const candidates = await call(candidatePath + '&after=' + encodeURIComponent(baseline.nextCursor));
    return candidates.items.length === 1 && candidates.items[0].plate === '晋A12345' &&
      candidates.items[0].sourceEvents.length === 2;
  });

  await page.getByText('仪表故障模拟', { exact: true }).click();
  await page.getByRole('button', { name: '暂停发送', exact: true }).click();
  await waitFor('停发后 Server 判定仪表不可用', async () => !(await call(devicePath)).deviceOk);
  await page.getByRole('button', { name: '恢复发送', exact: true }).click();
  await waitFor('恢复后 Server 收到稳定重量', async () => {
    const state = await call(devicePath);
    return state.online && state.stable && Number(state.weight) === 12.5;
  });

  const duplicate = await startLab(api, 'duplicate-camera');
  await waitFor('第二个 Agent 显示接入冲突', async () => {
    const status = await fetch(duplicate.url + '/api/agent', { signal: AbortSignal.timeout(2000) });
    return (await status.json()).state === 'conflict';
  });
  assert.equal(Number((await call(devicePath)).weight), 12.5);
  assert.deepEqual(await stop(duplicate.child), [0, null]);
  assert.deepEqual(await stop(lab.child, 'SIGINT'), [0, null]);
  await assert.rejects(access(port), { code: 'ENOENT' });
  assert.deepEqual(await readdir(runtime), []);
  await waitFor('Server 收到 Agent 离线', async () => !(await call(devicePath)).online);
  assert.deepEqual(errors, []);
  console.log('本地设备联调台冒烟通过');
} catch (error) {
  for (const child of children) console.error(child.output);
  throw error;
} finally {
  await browser?.close();
  for (const child of children.reverse()) await stop(child);
  try {
    await database?.close();
  } finally {
    await rm(temporary, { recursive: true, force: true });
  }
}
