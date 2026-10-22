import { startTestPostgres } from '../../scripts/test-postgres.mjs';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const root = fileURLToPath(new URL('../..', import.meta.url));
const dir = await mkdtemp(join(tmpdir(), 'weighbridge-site-config-'));
const api = 'http://127.0.0.1:18086', web = 'http://127.0.0.1:4186';
const children = [];
const env = {...process.env, GOCACHE: resolve(root, '.task/go-build'), APP_ENV: 'test', APP_CONFIG_PROFILE: '', APP_JWT__SECRET: 'site-config-browser-test-secret-not-deployed', APP_FILE__STORAGE_ROOT: join(dir, 'uploads'), APP_HTTP__ADDRESS: '127.0.0.1:18086', APP_MONITORING__ENABLED: 'false', APP_WEIGHING__AGENT_KEY: 'site-config-browser-key', APP_WEIGHING__AGENTS: JSON.stringify([{id:'scale-agent-1',scale_id:1,key:'site-config-browser-key'}]), APP_WEIGHING__IMAGE_DIR: join(dir, 'images')};
function start(cmd, args, cwd, extra = {}) {
 const child = spawn(cmd, args, {cwd, env: {...env, ...extra}, detached: true, stdio: ['ignore', 'pipe', 'pipe']});
 child.output = ''; child.stdout.on('data', b => child.output += b); child.stderr.on('data', b => child.output += b); children.push(child); return child;
}
function stop(child) { if (child.exitCode === null) { try {process.kill(-child.pid, 'SIGTERM');} catch { /* Process already exited. */ } } }
async function finished(child) { await new Promise((ok, fail) => {child.once('error', fail); child.once('exit', code => code === 0 ? ok() : fail(Error(child.output)));}); }
async function ready(url, child) { for (let i = 0; i < 120; i++) { if (child.exitCode !== null) throw Error(child.output); try {if ((await fetch(url)).ok) return;} catch { /* Startup retry. */ } await delay(150); } throw Error(`Not ready: ${url}\n${child.output}`); }
let browser;
let postgres;
const deadline = setTimeout(() => children.forEach(stop), 120_000);
try {
  postgres = await startTestPostgres();
  Object.assign(env, postgres.env);
 await finished(start('go', ['run', './cmd/migrate', 'up', '--kind', 'all'], resolve(root, 'server')));
 const server = start('go', ['run', './cmd/api'], resolve(root, 'server'));
 const vite = start(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4186', '--strictPort'], resolve(root, 'web'), {VITE_API_BASE_URL: api});
 await Promise.all([ready(`${api}/ready`, server), ready(`${web}/login`, vite)]);
 browser = await chromium.launch({headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? {executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH} : {})});
 const context = await browser.newContext({acceptDownloads: true, viewport: {width: 1366, height: 900}});
 const page = await context.newPage(); page.setDefaultTimeout(8000);
 const errors = []; page.on('pageerror', e => errors.push(e.message));
 await page.goto(`${web}/login`); await page.getByPlaceholder('请输入用户名', {exact: true}).fill('admin'); await page.getByPlaceholder('请输入密码', {exact: true}).fill('admin123'); await page.getByRole('button', {name: '登录', exact: true}).click(); await page.waitForURL(u => !u.pathname.startsWith('/login'));
 await page.goto(`${web}/weighing/site-config`); await page.getByRole('heading', {name: '厂区配置', exact: true}).waitFor();
 await page.getByRole('button', {name: '新增厂区', exact: true}).waitFor();
 await page.locator('#site-server-url').fill(api); await page.getByRole('button', {name: '保存地址', exact: true}).click(); await page.getByRole('status').filter({hasText: '现场连接地址已保存'}).waitFor();
 await page.getByRole('button', {name: '新增厂区', exact: true}).click();
 for (const [key, value] of Object.entries({code: 'SITE03', name: '浏览器三厂', scaleCode: 'SCALE03', scaleName: '三厂地磅', agentId: 'site-browser-3'})) await page.locator(`#site-${key}`).fill(value);
 await page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click(); await page.getByRole('dialog').waitFor({state: 'hidden'});
 const row = page.getByRole('row').filter({hasText: '浏览器三厂'}); await row.waitFor();
 const download = async n => { const pending = page.waitForEvent('download'); await row.getByRole('button', {name: '下载启动配置'}).click(); const file = await pending; const path = join(dir, `site-${n}.json`); await file.saveAs(path); return path; };
 const configPath = await download(1), secondPath = await download(2);
 const connection = JSON.parse(await readFile(configPath, 'utf8')); assert.equal(await readFile(configPath, 'utf8'), await readFile(secondPath, 'utf8'));
 assert.equal(connection.siteName, '浏览器三厂'); assert.equal(connection.serverUrl, api); assert.equal(connection.agentId, 'site-browser-3');
 const mock = start('go', ['run', './cmd/agent-mock', '--config', configPath, '--listen', '127.0.0.1:18091'], resolve(root, 'server'));
 await ready('http://127.0.0.1:18091/api/state', mock);
 for (let i = 0; i < 60; i++) { const state = await (await fetch('http://127.0.0.1:18091/api/state')).json(); if (state.online) {assert.equal(state.siteName, '浏览器三厂'); break;} if (i === 59) throw Error('mock failed to connect'); await delay(100); }
 await row.getByRole('button', {name: '编辑', exact: true}).click(); await page.locator('#site-enabled').selectOption('0'); await page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click(); await page.getByRole('alert').filter({hasText: 'Agent 尚未离线'}).waitFor();
 stop(mock); await delay(800); await page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click(); await page.getByRole('dialog').waitFor({state: 'hidden'}); await row.getByText('停用', {exact: true}).waitFor();
 await row.getByRole('button', {name: '编辑', exact: true}).click(); await page.locator('#site-enabled').selectOption('1'); await page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click(); await page.getByRole('dialog').waitFor({state: 'hidden'}); await row.getByText('启用', {exact: true}).waitFor();
 await page.setViewportSize({width: 390, height: 844}); assert.ok(await page.getByRole('heading', {name: '厂区配置', exact: true}).isVisible()); assert.deepEqual(errors, []);
 console.log('PASS: 厂区创建、地址保存、重复下载、mock 文件启动与归属、在线阻止停用、离线停用与重启用、窄屏页面');
} finally {clearTimeout(deadline); await browser?.close(); children.forEach(stop); await delay(500); await postgres?.close(); await rm(dir, {recursive: true, force: true});}
