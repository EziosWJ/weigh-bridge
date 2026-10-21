// Real browser + production command runner, isolated from systemd/CUPS/devices.
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { once } from 'node:events';
import fs from 'node:fs/promises';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(path.join(root, 'web/package.json'));
const { chromium } = require('playwright');
const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'weighbridge-maintenance-'));
const data = path.join(temporary, 'data');
const commands = path.join(temporary, 'commands');
const statePath = path.join(temporary, 'state.json');
let child;
let printerChild;
let browser;
let page;
let output = '';
const state = { agent: true, cups: true, enabled: true, jobs: [], prints: 0 };
const queue = 'Aisino-SK820-raw';
const dummyKey = 'local-smoke-agent-key';

const fakeCommand = String.raw`#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
p = Path(os.environ['MAINT_SMOKE_STATE'])
s = json.loads(p.read_text())
tool, args = Path(sys.argv[0]).name, sys.argv[1:]
q = 'Aisino-SK820-raw'
def save():
    tmp = p.with_suffix('.tmp')
    tmp.write_text(json.dumps(s))
    tmp.replace(p)
if tool == 'systemctl':
    if args[:2] == ['--user', 'is-active']:
        print('active' if s['agent'] else 'inactive')
        sys.exit(0 if s['agent'] else 3)
    if args[:2] == ['--user', 'stop']: s['agent'] = False
    elif args[:2] == ['--user', 'start']: s['agent'] = True
    elif args == ['restart', 'cups.service']: s['cups'] = True
    else: raise RuntimeError('unexpected systemctl action')
    save()
elif tool == 'lpstat':
    if not s['cups']:
        print('scheduler is not running')
        sys.exit(1)
    if args == ['-r']: print('scheduler is running')
    elif args and args[0] == '-p':
        print('printer '+q+' is idle. '+('enabled' if s['enabled'] else 'disabled')+' since Tue 20 Oct 2026')
        if len(args) == 1: print('printer Office-Queue is idle. enabled since Tue 20 Oct 2026')
    elif args == ['-a']: print(q+' accepting requests since Tue 20 Oct 2026')
    elif args == ['-W', 'not-completed', '-o']:
        for job in s['jobs']: print(job+' operator 1024 bytes Tue 20 Oct 2026 10:00:00')
    else: raise RuntimeError('unexpected lpstat action')
elif tool == 'cancel':
    assert len(args) == 1 and args[0] in s['jobs']
    s['jobs'].remove(args[0]); save()
elif tool == 'cupsenable':
    assert args == [q]
    s['enabled'] = True; save()
elif tool == 'lp':
    assert args[:8] == ['-d',q,'-o','raw','-n','1','-t','weighbridge-CALIBRATION-TEST']
    payload = Path(args[8]).read_bytes()
    assert payload.startswith(b'\x1b@') and len(payload) > 1000
    s['prints'] += 1
    job = q+'-'+str(100+s['prints'])
    s['jobs'].append(job); save()
    print('request id is '+job+' (1 file(s))')
elif tool == 'journalctl': print('fixture token local-smoke-agent-key must be hidden')
else: raise RuntimeError('unexpected executable')
`;

async function writeState(value = state) { await fs.writeFile(statePath, JSON.stringify(value)); }
async function readState() { return JSON.parse(await fs.readFile(statePath, 'utf8')); }
async function freePort(host) {
  const socket = net.createServer();
  socket.listen(0, host);
  await once(socket, 'listening');
  const port = socket.address().port;
  await new Promise((resolve) => socket.close(resolve));
  return port;
}
async function stop() {
  if (!child || child.exitCode !== null) return;
  child.kill('SIGTERM');
  const timer = setTimeout(() => child?.kill('SIGKILL'), 4000);
  await once(child, 'exit');
  clearTimeout(timer);
}
async function cleanup() {
  await browser?.close();
  await stopPrinter();
  await stop();
  await fs.rm(temporary, { recursive: true, force: true });
}
async function stopPrinter() {
  if (!printerChild || printerChild.exitCode !== null) return;
  printerChild.kill('SIGTERM');
  const timer = setTimeout(() => printerChild?.kill('SIGKILL'), 4000);
  await once(printerChild, 'exit');
  clearTimeout(timer);
}
process.once('SIGTERM', () => cleanup().finally(() => process.exit(1)));
process.once('SIGINT', () => cleanup().finally(() => process.exit(1)));
const watchdog = setTimeout(() => cleanup().finally(() => process.exit(1)), 150000);

async function start(host, tls = [], bindHost = host) {
  const port = await freePort(host);
  const url = `${tls.length ? 'https' : 'http'}://${host}:${port}`;
  child = spawn(path.join(root, 'bin/weighbridge-agent-maintenance'), ['--listen', `${bindHost}:${port}`, '--data-dir', data, ...tls], {
    env: { ...process.env, PATH: `${commands}:${process.env.PATH}`, MAINT_SMOKE_STATE: statePath },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  child.stdout.on('data', (chunk) => { output += chunk; });
  child.stderr.on('data', (chunk) => { output += chunk; });
  for (let attempt = 0; attempt < 100; attempt++) {
    if (child.exitCode !== null) throw Error(`maintenance exited: ${output}`);
    try { await fs.access(path.join(data, 'bootstrap-password.txt')); break; } catch {}
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  return url;
}

try {
  await fs.mkdir(commands);
  await fs.mkdir(data, { mode: 0o700 });
  await writeState();
  for (const command of ['systemctl', 'lpstat', 'cancel', 'cupsenable', 'lp', 'journalctl']) {
    await fs.writeFile(path.join(commands, command), fakeCommand, { mode: 0o700 });
  }
  await fs.writeFile(path.join(data, 'agent.env'), `APP_WEIGHING__AGENT_KEY="${dummyKey}"\nWEIGHBRIDGE_SERVER_URL="http://127.0.0.1:18900"\nWEIGHBRIDGE_AGENT_ID="scale-agent-1"\nWEIGHBRIDGE_SCALE_ID=1\nWEIGHBRIDGE_PRINT_QUEUE="${queue}"\n`, { mode: 0o600 });
  const url = await start('127.0.0.1');
  const password = (await fs.readFile(path.join(data, 'bootstrap-password.txt'), 'utf8')).trim();
  let executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || chromium.executablePath();
  try { await fs.access(executablePath); } catch {
    const cache = path.join(os.homedir(), '.cache/ms-playwright');
    const versions = (await fs.readdir(cache)).filter((entry) => /^chromium-\d+$/.test(entry)).sort().reverse();
    executablePath = path.join(cache, versions[0], 'chrome-linux64/chrome');
  }
  browser = await chromium.launch({ headless: true, executablePath, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, ignoreHTTPSErrors: true });
  page = await context.newPage();
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('dialog', (dialog) => dialog.accept());
  await page.goto(url);
  await page.getByText('未发现未结束作业', { exact: true }).waitFor();
  assert.equal(await page.locator('#private').isVisible(), false);
  const denied = await context.request.post(`${url}/api/action`, { data: { action: 'restart-agent' } });
  assert.equal(denied.status(), 401);
  await page.getByLabel('本地维护密码', { exact: true }).fill(password);
  await page.getByRole('button', { name: '验证并登录' }).click();
  await page.getByRole('link', { name: '配置', exact: true }).click();
  await page.getByLabel('Server 地址', { exact: true }).waitFor();
  await page.getByRole('link', { name: '记录', exact: true }).click();
  await page.getByRole('button', { name: '查询记录', exact: true }).click();
  await page.getByText('共 0 条', { exact: false }).waitFor();

  await page.getByRole('link', { name: '总览', exact: true }).click();
  state.agent = false;
  await writeState();
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByText('停止／不可读取', { exact: true }).first().waitFor();
  await page.getByRole('link', { name: '记录', exact: true }).click();
  await page.getByRole('button', { name: '读取最近 200 行' }).click();
  await page.waitForFunction(() => document.querySelector('#logs').textContent.includes('已脱敏'));
  assert.ok(!(await page.locator('#logs').textContent()).includes(dummyKey));

  await page.getByRole('link', { name: '总览', exact: true }).click();
  state.cups = false;
  await writeState();
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByText('未结束作业：未知／不可读取', { exact: true }).waitFor();
  await page.getByRole('link', { name: '打印与队列', exact: true }).click();
  await page.getByRole('button', { name: '重启 CUPS', exact: true }).click();
  await page.locator('dialog').getByText('未知／不可读取', { exact: false }).first().waitFor();
  await page.getByRole('button', { name: '接受状态未知并恢复', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('#operationMessage').textContent.includes('已完成'));
  assert.equal((await readState()).cups, true);
  assert.equal((await readState()).agent, false);

  Object.assign(state, await readState(), { agent: true, enabled: false, jobs: [`${queue}-11`, `${queue}-12`] });
  await writeState();
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByRole('button', { name: '重启 CUPS', exact: true }).click();
  await page.locator('dialog').getByText('Office-Queue', { exact: false }).waitFor();
  await page.getByRole('button', { name: '放弃操作', exact: true }).click();
  await page.getByRole('button', { name: '恢复目标队列' }).click();
  await page.getByRole('button', { name: '放弃操作', exact: true }).click();
  assert.equal((await readState()).enabled, false);
  await page.getByRole('button', { name: '恢复目标队列' }).click();
  await page.locator('dialog input[type=checkbox]').first().check();
  await page.getByRole('button', { name: '取消所选作业后继续', exact: true }).click();
  await page.waitForFunction(() => !document.querySelector('dialog'));
  await page.waitForFunction(async () => (await (await fetch('/api/status')).json()).queueState === '已启用');
  assert.deepEqual((await readState()).jobs, [`${queue}-12`]);

  await page.getByRole('link', { name: '配置', exact: true }).click();
  await page.getByLabel('打印宽度 (mm)', { exact: true }).fill('170');
  await page.getByRole('button', { name: '保存草稿', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('#saved').textContent.includes('存在未应用'));
  assert.ok((await page.locator('#activeConfig').textContent()).includes('175 mm'));
  await page.getByRole('button', { name: '应用配置', exact: true }).click();
  await page.getByRole('button', { name: '确认应用并重启', exact: true }).click();
  await page.getByRole('button', { name: '保留作业，暂不应用', exact: true }).click();
  assert.ok(!(await fs.readFile(path.join(data, 'agent.env'), 'utf8')).includes('WIDTH_MM=170'));
  await page.getByRole('button', { name: '应用配置', exact: true }).click();
  await page.getByRole('button', { name: '确认应用并重启', exact: true }).click();
  await page.locator('dialog input[type=checkbox]').check();
  await page.getByRole('button', { name: '取消所选作业后继续', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('#saved').textContent.includes('当前无未应用'));
  assert.ok((await fs.readFile(path.join(data, 'agent.env'), 'utf8')).includes('WIDTH_MM=170'));
  await page.getByRole('link', { name: '打印与队列', exact: true }).click();
  await page.locator('#queues').getByText('Office-Queue', { exact: true }).waitFor();
  await page.getByRole('button', { name: '打印一份校准样张', exact: true }).click();
  await page.getByRole('button', { name: '确认打印一份', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('#operationMessage').textContent.includes('已提交一份'));
  assert.equal((await readState()).prints, 1);
  await page.getByRole('button', { name: '查看未结束作业', exact: true }).click();
  await page.locator('#jobs input[type=checkbox]').check();
  await page.getByRole('button', { name: '取消选中作业', exact: true }).click();
  await page.locator('dialog').getByText(`${queue}-101`, { exact: true }).waitFor();
  await page.getByRole('button', { name: '确认取消所选作业', exact: true }).click();
  await page.waitForFunction(() => !document.querySelector('#private button:disabled') || !busy);
  await page.getByRole('link', { name: '记录', exact: true }).click();
  await page.getByLabel('操作类型', { exact: true }).fill('取消打印作业');
  await page.getByRole('button', { name: '查询记录', exact: true }).click();
  await page.waitForFunction(() => {
    const rows = [...document.querySelectorAll('#audit tbody tr')];
    return rows.length === 3 && rows.every((row) => row.children[2]?.textContent === '取消打印作业');
  });
  await page.locator('#audit').getByText(`${queue}-101`, { exact: true }).waitFor();
  assert.ok(!(await page.locator('body').textContent()).includes(dummyKey));

  // Use the real printer process with a loopback maintenance URL which has no
  // Agent WebSocket endpoint. It cannot receive or print business requests.
  const envFile = path.join(data, 'agent.env');
  const oldEnvironment = await fs.readFile(envFile, 'utf8');
  await fs.writeFile(envFile, oldEnvironment.replace(/^WEIGHBRIDGE_SERVER_URL=.*$/m, `WEIGHBRIDGE_SERVER_URL="${url}"`), { mode: 0o600 });
  printerChild = spawn(path.join(root, 'bin/weighbridge-agent-printer'), ['--env-file', envFile, '--state-dir', path.join(data, 'state')], {
    env: { ...process.env, WEIGHBRIDGE_SERVER_URL: 'http://stale.invalid', WEIGHBRIDGE_PRINT_WIDTH_MM: 'invalid-stale-value' },
    stdio: 'ignore',
  });
  let runtimeVerified = false;
  for (let attempt = 0; attempt < 60; attempt++) {
    if (printerChild.exitCode !== null) throw Error('real printer process rejected the shared environment');
    const effective = await (await context.request.get(`${url}/api/config`)).json();
    if (effective.runtimeState === '运行配置与 agent.env 一致') {
      assert.equal(effective.active.serverUrl, url);
      assert.equal(effective.active.printWidthMm, 170);
      runtimeVerified = true;
      break;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  assert.ok(runtimeVerified, 'real printer process and maintenance disagree on effective config');
  assert.equal((await readState()).prints, 1);
  await stopPrinter();
  await fs.rm(path.join(data, 'maintenance-status.json'), { force: true });

  await page.getByRole('link', { name: '总览', exact: true }).click();
  await page.route('**/api/status', (route) => route.abort());
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByText('检查失败，状态未知', { exact: true }).waitFor();
  assert.equal(await page.locator('#stats .ok').count(), 0);
  await page.unroute('**/api/status');
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('#checked').textContent.startsWith('检查时间：'));
  await fs.mkdir(path.join(root, '.task'), { recursive: true });
  await page.getByRole('link', { name: '打印与队列', exact: true }).click();
  await page.getByText('当前无未结束作业。', { exact: true }).waitFor();
  await page.screenshot({ path: path.join(root, '.task/agent-maintenance-smoke.png'), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  for (const menu of ['总览', '打印与队列', '设备', '配置', '记录']) {
    await page.getByRole('link', { name: menu, exact: true }).click();
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 2), `mobile ${menu} overflows horizontally`);
  }
  await page.getByRole('link', { name: '设备', exact: true }).click();
  await page.getByText('暂无重量预览', { exact: true }).waitFor();
  await page.getByText('暂无视频预览', { exact: true }).waitFor();
  assert.equal(await page.locator('[data-page=devices] input').count(), 0);
  await page.reload();
  await page.getByRole('heading', { name: '现场设备', exact: true }).waitFor();
  await page.getByRole('button', { name: '退出维护登录' }).waitFor();
  assert.deepEqual(errors, []);
  await stop();

  const lan = Object.values(os.networkInterfaces()).flat().find((address) => address?.family === 'IPv4' && !address.internal && /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(address.address));
  if (lan) {
    const lanHTTP = await start(lan.address, [], '0.0.0.0');
    await page.goto(lanHTTP);
    await page.getByRole('heading', { name: '运行总览' }).waitFor();
    await page.getByLabel('本地维护密码', { exact: true }).fill(password);
    await page.getByRole('button', { name: '验证并登录' }).click();
    await page.getByRole('button', { name: '退出维护登录' }).waitFor();
    await stop();
    const cert = path.join(temporary, 'cert.pem'), key = path.join(temporary, 'key.pem');
    execFileSync('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1', '-subj', '/CN=maintenance-smoke', '-out', cert, '-keyout', key], { stdio: 'ignore', timeout: 10000 });
    const lanURL = await start(lan.address, ['--tls-cert', cert, '--tls-key', key]);
    await page.goto(lanURL);
    await page.getByRole('heading', { name: '运行总览' }).waitFor();
    await page.getByText('未发现未结束作业', { exact: true }).waitFor();
    console.log('PASS: loopback, wildcard LAN/HTTP login and optional TLS; navigation, queues, offline/auth/recovery/jobs/config/calibration/audit/stale state/mobile/device placeholders');
  } else {
    console.log('PASS: loopback browser scenarios; LAN/TLS unavailable (no private IPv4 interface)');
  }
} catch (error) {
  if (page) {
    await fs.mkdir(path.join(root, '.task'), { recursive: true });
    await page.screenshot({ path: path.join(root, '.task/agent-maintenance-failure.png'), fullPage: true }).catch(() => {});
    console.error('Operation message:', await page.locator('#operationMessage').textContent().catch(() => 'unavailable'));
  }
  throw error;
} finally {
  clearTimeout(watchdog);
  await cleanup();
}
