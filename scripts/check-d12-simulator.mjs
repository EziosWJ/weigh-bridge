// Isolated CLI + browser smoke; no running Agent, database or hardware needed.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import fs from 'node:fs/promises';
import { constants } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(path.join(root, 'web/package.json'));
const { chromium } = require('playwright');
const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'weighbridge-d12-'));
let child;
let browser;
let reader;
let output = '';
let port;
let url;
let exit;

async function waitFor(name, predicate, timeout = 6000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if (await predicate()) return;
    await new Promise(resolve => setTimeout(resolve, 40));
  }
  throw new Error(`timeout: ${name}\n${output}`);
}

async function state() {
  const response = await fetch(`${url}/api/state`, { signal: AbortSignal.timeout(2000) });
  assert.equal(response.status, 200);
  return response.json();
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
  child = spawn(path.join(root, 'bin/weighbridge-d12-simulator'), ['--listen', '127.0.0.1:0'], {
    cwd: temporary, stdio: ['ignore', 'pipe', 'pipe'],
  });
  exit = once(child, 'exit');
  child.stdout.on('data', data => { output += data; });
  child.stderr.on('data', data => { output += data; });
  await waitFor('CLI address', () => {
    port = output.match(/\/dev\/pts\/\d+/)?.[0];
    url = output.match(/http:\/\/127\.0\.0\.1:\d+/)?.[0];
    return port && url;
  });
  const initial = await state();
  assert.equal(initial.weight, '0');
  assert.equal(initial.dynamic, false);
  assert.equal(initial.paused, false);
  // Open the real PTY to let the simulator send frames. Production termios
  // and Monitor behavior are verified separately by the Go behavior test.
  reader = await fs.open(port, constants.O_RDWR | constants.O_NOCTTY | constants.O_NONBLOCK);
  await waitFor('PTY reader', async () => (await state()).readerPresent);

  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1280, height: 1000 } });
  page.setDefaultTimeout(6000);
  const pageErrors = [];
  page.on('pageerror', error => pageErrors.push(error.message));
  await page.goto(url);
  await page.getByText('控制台在线', { exact: true }).waitFor();
  assert.equal(await page.locator('#port').textContent(), port);
  await page.getByLabel('目标重量（吨）').fill('30.125');
  await page.getByRole('button', { name: '设置稳定重量', exact: true }).click();
  await waitFor('set weight', async () => await page.locator('#weight').textContent() === '30.125');
  await page.getByRole('button', { name: '强制失稳', exact: true }).click();
  await page.getByText('动态 / 未稳定', { exact: true }).waitFor();
  assert.equal((await state()).weight, '30.125');
  await page.getByRole('button', { name: '恢复稳定', exact: true }).click();
  await waitFor('stable bit', async () => !(await state()).dynamic);
  await page.getByRole('button', { name: '暂停发送', exact: true }).click();
  await waitFor('paused indicator', async () => await page.locator('#transport').textContent() === '已暂停发送');
  const paused = await state();
  await new Promise(resolve => setTimeout(resolve, 450));
  assert.equal((await state()).framesSent, paused.framesSent);
  assert.equal((await state()).weight, '30.125');
  await page.getByRole('button', { name: '恢复发送', exact: true }).click();
  await waitFor('resume', async () => (await state()).framesSent > paused.framesSent);
  const badBefore = (await state()).badFramesSent;
  await page.getByRole('button', { name: '发送一个坏帧', exact: true }).click();
  await waitFor('one bad frame', async () => (await state()).badFramesSent === badBefore + 1);
  await page.getByRole('button', { name: '连续坏帧', exact: true }).click();
  await waitFor('continuous bad frames', async () => (await state()).badFramesSent >= badBefore + 4);
  await page.getByRole('button', { name: '恢复正常校验', exact: true }).click();
  await waitFor('good checksum', async () => !(await state()).lastFrameBad);
  await page.getByLabel('目标重量（吨）').fill('12.5');
  await page.getByRole('button', { name: '模拟上磅', exact: true }).click();
  await page.getByText('上磅波动中', { exact: true }).waitFor();
  await waitFor('settle at target', async () => {
    const s = await state();
    return !s.dynamic && s.weight === '12.5';
  });
  await page.getByRole('button', { name: '下磅归零', exact: true }).click();
  await waitFor('down', async () => await page.locator('#weight').textContent() === '0');
  await fs.mkdir(path.join(root, '.task'), { recursive: true });
  await page.screenshot({ path: path.join(root, '.task/d12-simulator-desktop.png'), fullPage: true });
  await page.setViewportSize({ width: 375, height: 900 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true, 'mobile horizontal overflow');
  await page.screenshot({ path: path.join(root, '.task/d12-simulator-mobile.png'), fullPage: true });
  assert.deepEqual(pageErrors, []);
  await browser.close();
  browser = null;
  await reader.close();
  reader = null;
  const result = await stop();
  assert.deepEqual(result, [0, null]);
  await assert.rejects(fs.stat(port), { code: 'ENOENT' });
  console.log('D12 smoke passed: standalone CLI, PTY, browser controls, desktop/mobile, graceful shutdown and PTY cleanup.');
} finally {
  await browser?.close();
  await reader?.close();
  await stop();
  await fs.rm(temporary, { recursive: true, force: true });
}
