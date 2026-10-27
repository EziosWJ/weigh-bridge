import { startTestPostgres } from '../../scripts/test-postgres.mjs';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const root = fileURLToPath(new URL('../..', import.meta.url));
const dir = await mkdtemp(join(tmpdir(), 'weighbridge-site-config-'));
const api = 'http://127.0.0.1:18086', web = 'http://127.0.0.1:4186';
const children = [];
const env = {...process.env, GOCACHE: resolve(root, '.task/go-build'), APP_ENV: 'test', APP_CONFIG_PROFILE: '', APP_JWT__SECRET: 'site-config-browser-test-secret-not-deployed', APP_FILE__STORAGE_ROOT: join(dir, 'uploads'), APP_HTTP__ADDRESS: '127.0.0.1:18086', APP_MONITORING__ENABLED: 'false', APP_WEIGHING__IMAGE_DIR: join(dir, 'images')};
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
 await finished(start('go', ['run', './cmd/local-admin-init'], resolve(root, 'server'), {WEIGHBRIDGE_ADMIN_PASSWORD: 'local-browser-password-123'}));
 const server = start('go', ['run', './cmd/api'], resolve(root, 'server'));
 const vite = start(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4186', '--strictPort'], resolve(root, 'web'), {VITE_API_BASE_URL: api});
 await Promise.all([ready(`${api}/ready`, server), ready(`${web}/login`, vite)]);
 browser = await chromium.launch({headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? {executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH} : {})});
 const context = await browser.newContext({acceptDownloads: true, viewport: {width: 1366, height: 900}});
 const page = await context.newPage(); page.setDefaultTimeout(8000);
 const errors = []; page.on('pageerror', e => errors.push(e.message));
 await page.goto(`${web}/login`); await page.getByPlaceholder('请输入用户名', {exact: true}).fill('admin'); await page.getByPlaceholder('请输入密码', {exact: true}).fill('local-browser-password-123'); await page.getByRole('button', {name: '登录', exact: true}).click(); await page.waitForURL(u => !u.pathname.startsWith('/login'));
 await page.goto(`${web}/system/config`); await page.getByRole('heading', {name: '配置管理', exact: true}).waitFor();
 await page.goto(`${web}/system/local-identity`);
 await page.getByLabel('厂区编码', {exact: true}).waitFor();
 const token = await page.evaluate(() => JSON.parse(localStorage.getItem('react-admin-auth')).token);
 const call = async (path, method = 'GET', body, expectedCode = 200) => {
  const response = await fetch(`${api}/api/weighing/${path}`, {method, headers: {Authorization: token, 'Content-Type': 'application/json'}, ...(body ? {body: JSON.stringify(body)} : {})});
  const result = await response.json(); assert.equal(result.code, expectedCode, result.message); return result.data;
 };
 // ADR 0033: a fresh database no longer auto-initializes; the administrator
 // fills the head-office assigned site code on first entry.
 const configured = await call('local-status');
 if (!configured.configured) {
  await page.getByLabel('厂区编码', {exact: true}).fill('F001');
  await page.getByLabel('地磅编码', {exact: true}).fill('WB01');
  await page.getByRole('button', {name: '确定本厂身份', exact: true}).click();
  await page.getByText('本厂身份已配置', {exact: true}).waitFor();
 }
 assert.deepEqual(await call('local-status'), {siteCode: 'F001', scaleCode: 'WB01', configured: true, initialized: true});
 assert.equal(await page.getByLabel('厂区编码', {exact: true}).first().inputValue(), 'F001');
 // Modify both codes in the independent page, then restore them for the
 // remaining lifecycle checks. The same local installation remains bound.
 await page.getByLabel('厂区编码', {exact: true}).fill('F002');
 await page.getByLabel('地磅编码', {exact: true}).fill('WB02');
 await page.getByRole('button', {name: '保存身份', exact: true}).click();
 await page.getByText('本厂身份已保存', {exact: true}).last().waitFor();
 assert.deepEqual(await call('local-status'), {siteCode:'F002',scaleCode:'WB02',configured:true,initialized:true});
 await page.getByLabel('厂区编码', {exact: true}).waitFor();
 await page.getByLabel('厂区编码', {exact: true}).fill('F001');
 await page.getByLabel('地磅编码', {exact: true}).fill('WB01');
 await page.getByRole('button', {name: '保存身份', exact: true}).click();
 for (let i = 0; i < 50; i++) {
  if ((await call('local-status')).siteCode === 'F001') break;
  if (i === 49) throw Error('identity restore did not complete');
  await delay(100);
 }
 for (const name of ['编辑本厂配置', '新增厂区', '下载启动配置']) assert.equal(await page.getByRole('button', {name, exact: true}).count(), 0);
 await page.goto(`${web}/system/ticket-title`);
 await page.getByLabel('打印标题', {exact: true}).fill('自动初始化测试过磅单');
 await page.getByRole('button', {name: '预览标题', exact: true}).click();
 const preview = page.getByRole('dialog', {name: '过磅单标题预览', exact: true});
 await preview.getByRole('heading', {name: '自动初始化测试过磅单', exact: true}).waitFor();
 await preview.getByRole('button', {name: '关闭详情弹窗', exact: true}).click();
 await page.getByRole('button', {name: '保存标题', exact: true}).click();
 await page.getByText('过磅单标题已保存', {exact: true}).waitFor();
 await page.reload();
 await page.getByLabel('打印标题', {exact: true}).waitFor();
 assert.equal(await page.getByLabel('打印标题', {exact: true}).inputValue(), '自动初始化测试过磅单');
 const repeatedResponse = await fetch(`${api}/api/weighing/site-config/initialize`, {method:'POST',headers:{Authorization:token,'Content-Type':'application/json'},body:JSON.stringify({siteCode:'F018',scaleCode:'WB18',name:'第二厂',scaleName:'第二磅'})});
 const repeatedResult = await repeatedResponse.json(); assert.notEqual(repeatedResult.code,200,'repeat initialize must fail rather than return a second site');
 const configs = await call('site-config'); assert.equal(configs.length, 1); assert.equal(configs[0].code, 'F001'); assert.equal(configs[0].scaleCode, 'WB01');
 const status = await call('local-status'); assert.deepEqual(status,{siteCode:'F001',scaleCode:'WB01',configured:true,initialized:true},'首次事务成功即完成配置，不再按操作员人数启用');
 const capabilities = await call('capabilities'); assert.equal(capabilities.localSiteCode,'F001'); assert.equal(capabilities.localScaleCode,'WB01'); assert.equal(capabilities.configured,true); assert.equal(typeof capabilities.initialized,'boolean'); assert.equal('nodeRole' in capabilities,false); assert.equal('nodeSiteId' in capabilities,false);
 const createAnother = await fetch(`${api}/api/weighing/site-config`, {method:'POST',headers:{Authorization:token,'Content-Type':'application/json'},body:JSON.stringify({...configs[0],code:'OTHER'})});
 assert.notEqual((await createAnother.json()).code,200,'generic site creation must remain unavailable');
 // Names stay maintainable, and only the local site exists.
 await call('site-config', 'PUT', {...configs[0], name: '本厂配置冒烟', scaleName: '本厂一号地磅'});
 const saved = (await call('site-config'))[0]; assert.equal(saved.name, '本厂配置冒烟'); assert.equal(saved.scaleName, '本厂一号地磅'); assert.equal(saved.serverUrl, '', '旧 serverUrl 字段不作为地址来源或被写入');
 assert.equal(saved.code, 'F001'); assert.equal(saved.scaleCode, 'WB01'); assert.equal(saved.agentId, 'agent-WB01');
 const forgedResponse = await fetch(`${api}/api/weighing/site-config`, {method:'PUT',headers:{Authorization:token,'Content-Type':'application/json'},body:JSON.stringify({...saved,code:'FORGED',scaleCode:'FORGED-SCALE',agentId:'forged-agent'})});
 const forged = await forgedResponse.json();
 assert.notEqual(forged.code,200,'Agent identity changes are rejected when sent directly to the API');
 await page.reload(); await page.getByLabel('打印标题', {exact: true}).waitFor();
 // The historical connection API remains compatible; the current page has no
 // download/import provisioning workflow or exposed connection credentials.
 const connection = await call(`site-config/connection?serverUrl=${encodeURIComponent(api)}`);
 assert.deepEqual(await call(`site-config/connection?serverUrl=${encodeURIComponent(api)}`), connection);
 assert.equal(connection.serverUrl, api);
 const configPath = join(dir, 'local-agent.json'); await writeFile(configPath, JSON.stringify(connection), {mode: 0o600});
 const mock = start('go', ['run', './cmd/agent-mock', '--config', configPath, '--listen', '127.0.0.1:18091'], resolve(root, 'server'));
 await ready('http://127.0.0.1:18091/api/state', mock);
 for (let i = 0; i < 60; i++) { const state = await (await fetch('http://127.0.0.1:18091/api/state')).json(); if (state.online) {assert.equal(state.siteName, '本厂配置冒烟'); break;} if (i === 59) throw Error('mock failed to connect'); await delay(100); }
 await call('site-config', 'PUT', {...saved, enabled: 0}, 400);
 stop(mock);
 for (let i = 0; i < 60; i++) { if (!(await call(`scales/${connection.scaleCode}/device`)).online) break; if (i === 59) throw Error('mock did not disconnect'); await delay(100); }
 const disabled = await call('site-config', 'PUT', {...saved, enabled: 0});
 assert.equal(disabled.enabled, 0);
 const enabled = await call('site-config', 'PUT', {...disabled, enabled: 1});
 assert.equal(enabled.enabled, 1);
 await page.goto(`${web}/weighing/sites`); await page.waitForURL('**/weighing/tasks');
 await page.goto(`${web}/system/ticket-title`); await page.getByLabel('打印标题', {exact: true}).waitFor();
 await page.setViewportSize({width: 390, height: 844}); await page.waitForTimeout(250);
 assert.ok(await page.getByRole('heading', {name: '过磅单标题', exact: true}).first().isVisible()); assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true); assert.deepEqual(errors, []);
 console.log('PASS: 身份首次设置与编码修改、重复初始化拒绝、标题编辑预览与持久化、旧入口跳转、Agent 连接及停用保护、窄屏页面');
} finally {clearTimeout(deadline); await browser?.close(); children.forEach(stop); await delay(500); await postgres?.close(); await rm(dir, {recursive: true, force: true});}
