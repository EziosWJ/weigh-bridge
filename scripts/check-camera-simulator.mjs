// Isolated CLI + browser smoke; no running Server, database or camera needed.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import fs from 'node:fs/promises';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(path.join(root, 'web/package.json'));
const { chromium } = require('playwright');
const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'weighbridge-camera-'));
let child;
let browser;
let fake;
let output = '';
let url;
let exit;

const snapshots = [];
const heartbeats = [];
let mode = 'ok';

function eventIDOf(body) {
  const match = body.match(/"eventId":\s*(\d+)/);
  return match ? match[1] : '';
}

async function startFakeServer() {
  fake = http.createServer((request, response) => {
    let body = '';
    request.setEncoding('utf8');
    request.on('data', chunk => { body += chunk; });
    request.on('end', () => {
      if (request.url !== '/api/v1/event/pull') {
        response.writeHead(404).end();
        return;
      }
      const eventId = eventIDOf(body);
      if (body.includes('"operator":"heartbeat"')) {
        heartbeats.push(body);
        response.writeHead(200, { 'Content-Type': 'application/json' });
        response.end(`{"operator":"heartbeat-ack","info":{"eventId":${eventId},"time":"${new Date().toISOString()}","eventSendMode":"realTime"},"result":{"errorNo":0,"description":"ok"}}`);
        return;
      }
      snapshots.push(body);
      if (mode === 'http503') {
        response.writeHead(503).end('unavailable');
        return;
      }
      if (mode === 'ackError') {
        response.writeHead(200, { 'Content-Type': 'application/json' });
        response.end(`{"operator":"CarLicenseSnapshot-Ack","info":{"eventID":${eventId}},"result":{"errorNo":1,"description":"simulated vendor error"}}`);
        return;
      }
      response.writeHead(200, { 'Content-Type': 'application/json' });
      response.end(`{"operator":"CarLicenseSnapshot-Ack","info":{"eventID":${eventId}},"result":{"errorNo":0,"description":"ok"}}`);
    });
  });
  fake.listen(0, '127.0.0.1');
  await once(fake, 'listening');
  return `http://127.0.0.1:${fake.address().port}`;
}

async function waitFor(name, predicate, timeout = 8000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if (await predicate()) return;
    await new Promise(resolve => setTimeout(resolve, 50));
  }
  throw new Error(`timeout: ${name}\n${output}`);
}

async function stop() {
  if (!child || child.exitCode !== null || child.signalCode !== null) return;
  child.kill('SIGTERM');
  try {
    return await Promise.race([
      exit,
      new Promise((_, reject) => {
        const timer = setTimeout(() => reject(new Error('simulator did not exit')), 5000);
        timer.unref();
      }),
    ]);
  } catch (error) {
    child.kill('SIGKILL');
    await exit;
    throw error;
  }
}

try {
  const serverURL = await startFakeServer();
  child = spawn(path.join(root, 'bin/weighbridge-camera-simulator'), [
    '--listen', '127.0.0.1:0',
    '--server', serverURL,
    '--serial', 'camera-sim-1',
    '--heartbeat', '1s',
  ], { cwd: temporary, stdio: ['ignore', 'pipe', 'pipe'] });
  exit = once(child, 'exit');
  child.stdout.on('data', data => { output += data; });
  child.stderr.on('data', data => { output += data; });
  await waitFor('CLI address', () => {
    url = output.match(/控制页：(http:\/\/127\.0\.0\.1:\d+)/)?.[1];
    return Boolean(url);
  });

  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1280, height: 1000 } });
  page.setDefaultTimeout(8000);
  const pageErrors = [];
  page.on('pageerror', error => pageErrors.push(error.message));
  await page.goto(url);
  await page.getByText('模拟器 · 非真实摄像头', { exact: true }).waitFor();
  await page.getByText('控制台在线', { exact: true }).waitFor();
  await waitFor('heartbeat', async () => await page.locator('#heartbeat-state').textContent() === '心跳正常');
  const initialPlate = await page.locator('#plate').textContent();
  assert.match(initialPlate, /^[晋京津冀蒙辽吉黑沪苏浙皖闽赣鲁豫鄂湘粤桂琼渝川贵云藏陕甘青宁新][A-HJ-NP-Z][A-HJ-NP-Z0-9]{5}$|^[晋京津冀蒙辽吉黑沪苏浙皖闽赣鲁豫鄂湘粤桂琼渝川贵云藏陕甘青宁新][A-HJ-NP-Z]([DF][A-HJ-NP-Z0-9][0-9]{4}|[0-9]{5}[DF])$/);

  await page.getByRole('button', { name: '刷新车牌并上报', exact: true }).click();
  await waitFor('first report', async () => await page.locator('#report-state').textContent() === '上报成功');
  const firstPlate = await page.locator('#report-plate').textContent();
  const firstEvent = await page.locator('#report-event').textContent();
  assert.equal(firstPlate, await page.locator('#plate').textContent());
  assert.equal(snapshots.length, 1);
  assert.equal(eventIDOf(snapshots[0]), firstEvent);
  assert.ok(snapshots[0].includes(`"Number":"${firstPlate}"`));

  await page.getByRole('button', { name: '再次上报当前车牌', exact: true }).click();
  await waitFor('second report', async () => snapshots.length === 2
    && await page.locator('#report-state').textContent() === '上报成功'
    && await page.locator('#report-event').textContent() !== firstEvent);
  const secondPlate = await page.locator('#report-plate').textContent();
  const secondEvent = await page.locator('#report-event').textContent();
  assert.equal(secondPlate, firstPlate);
  assert.notEqual(secondEvent, firstEvent);
  assert.equal(eventIDOf(snapshots[1]), secondEvent);

  mode = 'http503';
  await page.getByRole('button', { name: '刷新车牌并上报', exact: true }).click();
  await waitFor('http failure', async () => await page.locator('#report-state').textContent() === '上报失败');
  assert.match(await page.locator('#report-error').textContent(), /HTTP 503/);
  assert.equal(await page.locator('#retry').isEnabled(), true);
  const failedEvent = await page.locator('#report-event').textContent();
  const failedBody = snapshots[snapshots.length - 1];

  mode = 'ok';
  await page.getByRole('button', { name: '重试上次上报', exact: true }).click();
  await waitFor('retry success', async () => await page.locator('#report-state').textContent() === '上报成功');
  assert.equal(await page.locator('#report-event').textContent(), failedEvent);
  assert.equal(await page.locator('#report-attempts').textContent(), '2 次');
  assert.equal(snapshots[snapshots.length - 1], failedBody);

  mode = 'ackError';
  await page.getByRole('button', { name: '刷新车牌并上报', exact: true }).click();
  await waitFor('ack failure', async () => await page.locator('#report-state').textContent() === '上报失败');
  assert.match(await page.locator('#report-error').textContent(), /errorNo=1/);
  assert.match(await page.locator('#report-error').textContent(), /simulated vendor error/);

  mode = 'ok';
  await page.getByRole('button', { name: '重试上次上报', exact: true }).click();
  await waitFor('ack retry success', async () => await page.locator('#report-state').textContent() === '上报成功');
  assert.ok(heartbeats.length >= 1);
  assert.deepEqual(pageErrors, []);

  await fs.mkdir(path.join(root, '.task'), { recursive: true });
  await page.screenshot({ path: path.join(root, '.task/camera-simulator-desktop.png'), fullPage: true });
  await page.setViewportSize({ width: 390, height: 900 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true, 'mobile horizontal overflow');
  await page.screenshot({ path: path.join(root, '.task/camera-simulator-mobile.png'), fullPage: true });

  await browser.close();
  browser = null;
  const result = await stop();
  assert.deepEqual(result, [0, null]);
  console.log('Camera simulator smoke passed: standalone CLI, heartbeat, report, HTTP/vendor failure, retry, mobile layout and graceful shutdown.');
} finally {
  await browser?.close();
  await stop();
  await new Promise(resolve => fake?.close(resolve));
  await fs.rm(temporary, { recursive: true, force: true });
}
