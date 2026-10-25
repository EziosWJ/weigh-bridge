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
 await page.getByRole('button', {name: '首次配置', exact: true}).waitFor();
 const token = await page.evaluate(() => JSON.parse(localStorage.getItem('react-admin-auth')).token);
 const call = async (path, method = 'GET', body, expectedCode = 200) => {
  const response = await fetch(`${api}/api/weighing/${path}`, {method, headers: {Authorization: token, 'Content-Type': 'application/json'}, ...(body ? {body: JSON.stringify(body)} : {})});
  const result = await response.json(); assert.equal(result.code, expectedCode, result.message); return result.data;
 };
 assert.deepEqual(await call('local-status'), {siteId: 0, configured: false, initialized: false});
 assert.deepEqual(await call('site-config'), []);
 assert.equal(await page.getByRole('button', {name: '新增厂区', exact: true}).count(), 0);
 assert.equal(await page.getByRole('button', {name: '下载启动配置', exact: true}).count(), 0);
 assert.equal(await page.getByText('新增厂区', {exact: true}).count(), 0);
 await page.getByRole('button', {name: '首次配置', exact: true}).click();
 assert.equal(await page.locator('#initial-serverUrl').count(), 0, '首次配置不再填写 Server 地址');
 await page.locator('#initial-name').fill('本地酒厂'); await page.locator('#initial-scaleName').fill('本厂地磅');
 await page.getByRole('dialog').getByRole('button', {name: '保存并初始化', exact: true}).click();
 await page.getByRole('dialog').waitFor({state: 'hidden'});
 await page.getByRole('button', {name: '编辑本厂配置', exact: true}).waitFor();
 assert.equal(await page.getByRole('button', {name:'首次配置',exact:true}).count(),0,'configured identity cannot be initialized twice');
 await page.getByText('已配置',{exact:true}).waitFor();
 await page.getByText('设备连接与运行状态以设备维护页为准').waitFor();
 assert.equal(await page.getByText(/两名实名操作员/).count(), 0);
 assert.equal(await page.getByText('本厂 Server 地址', {exact:true}).count(), 0);
 const repeatedResponse = await fetch(`${api}/api/weighing/site-config/initialize`, {method:'POST',headers:{Authorization:token,'Content-Type':'application/json'},body:JSON.stringify({siteId:18,code:'OTHER',name:'第二厂',scaleCode:'SCALE18',scaleName:'第二磅',agentId:'other-agent'})});
 const repeatedResult = await repeatedResponse.json(); assert.notEqual(repeatedResult.code,200,'repeat initialize must fail rather than return a second site');
 const configs = await call('site-config'); assert.equal(configs.length, 1); assert.equal(configs[0].id, 1);
 const status = await call('local-status'); assert.deepEqual(status,{siteId:1,configured:true,initialized:true},'首次事务成功即完成配置，不再按操作员人数启用');
 const capabilities = await call('capabilities'); assert.equal(capabilities.localSiteId,1); assert.equal(capabilities.configured,true); assert.equal(typeof capabilities.initialized,'boolean'); assert.equal('nodeRole' in capabilities,false); assert.equal('nodeSiteId' in capabilities,false);
 const createAnother = await fetch(`${api}/api/weighing/site-config`, {method:'POST',headers:{Authorization:token,'Content-Type':'application/json'},body:JSON.stringify({...configs[0],code:'OTHER'})});
 assert.notEqual((await createAnother.json()).code,200,'generic site creation must remain unavailable');
 const editAnother = await fetch(`${api}/api/weighing/site-config/18`, {method:'PUT',headers:{Authorization:token,'Content-Type':'application/json'},body:JSON.stringify({...configs[0],name:'不应改动'})});
 assert.notEqual((await editAnother.json()).code,200,'a different site identity must not be writable');
 await page.getByRole('button', {name: '编辑本厂配置', exact: true}).click();
 for (const key of ['code', 'scaleCode', 'agentId']) assert.equal(await page.locator(`#site-${key}`).count(), 0, 'internal identity is not an editable form field');
 await page.locator('#site-name').fill('本厂配置冒烟');
 await page.locator('#site-scaleName').fill('本厂一号地磅');
 assert.equal(await page.locator('#site-serverUrl').count(), 0, '编辑表单不再维护 Server 地址');
 await page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click();
 await page.getByRole('dialog').waitFor({state: 'hidden'}); await page.getByText('本厂配置冒烟', {exact: true}).waitFor();
 const saved = (await call('site-config'))[0]; assert.equal(saved.name, '本厂配置冒烟'); assert.equal(saved.scaleName, '本厂一号地磅'); assert.equal(saved.serverUrl, '', '旧 serverUrl 字段不作为地址来源或被写入');
 assert.equal(saved.code, 'SITE-01'); assert.equal(saved.scaleCode, 'SCALE-01'); assert.equal(saved.agentId, 'agent-01');
 const forgedResponse = await fetch(`${api}/api/weighing/site-config/${saved.id}`, {method:'PUT',headers:{Authorization:token,'Content-Type':'application/json'},body:JSON.stringify({...saved,code:'FORGED',scaleCode:'FORGED-SCALE',agentId:'forged-agent'})});
 const forged = await forgedResponse.json();
 if (forged.code === 200) assert.deepEqual([forged.data.code,forged.data.scaleCode,forged.data.agentId],['SITE-01','SCALE-01','agent-01'],'server must keep configured hardware identity locked');
 else assert.notEqual(forged.code,200,'identity changes are rejected when sent directly to the API');
 await page.reload(); await page.getByRole('button', {name:'编辑本厂配置',exact:true}).waitFor();
 // The historical connection API remains compatible; the current page has no
 // download/import provisioning workflow or exposed connection credentials.
 const connection = await call(`site-config/${saved.id}/connection?serverUrl=${encodeURIComponent(api)}`);
 assert.deepEqual(await call(`site-config/${saved.id}/connection?serverUrl=${encodeURIComponent(api)}`), connection);
 assert.equal(connection.serverUrl, api);
 const configPath = join(dir, 'local-agent.json'); await writeFile(configPath, JSON.stringify(connection), {mode: 0o600});
 const mock = start('go', ['run', './cmd/agent-mock', '--config', configPath, '--listen', '127.0.0.1:18091'], resolve(root, 'server'));
 await ready('http://127.0.0.1:18091/api/state', mock);
 for (let i = 0; i < 60; i++) { const state = await (await fetch('http://127.0.0.1:18091/api/state')).json(); if (state.online) {assert.equal(state.siteName, '本厂配置冒烟'); break;} if (i === 59) throw Error('mock failed to connect'); await delay(100); }
 await page.getByRole('button', {name: '编辑本厂配置', exact: true}).click(); await page.locator('#site-enabled').selectOption('0'); await page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click(); await page.getByRole('alert').filter({hasText: 'Agent 尚未离线'}).waitFor();
 stop(mock);
 for (let i = 0; i < 60; i++) { if (!(await call(`scales/${connection.scaleId}/device`)).online) break; if (i === 59) throw Error('mock did not disconnect'); await delay(100); }
 await page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click(); await page.getByRole('dialog').waitFor({state: 'hidden'}); const localCard = page.locator('section').filter({hasText: '本厂配置'}); await localCard.getByText('停用', {exact: true}).waitFor();
 await page.getByRole('button', {name: '编辑本厂配置', exact: true}).click(); await page.locator('#site-enabled').selectOption('1'); await page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click(); await page.getByRole('dialog').waitFor({state: 'hidden'}); await localCard.getByText('启用', {exact: true}).waitFor();
 await page.reload(); await page.getByText('本厂配置冒烟', {exact: true}).waitFor();
 await page.setViewportSize({width: 390, height: 844}); await page.waitForTimeout(250);
 assert.ok(await page.getByRole('heading', {name: '配置管理', exact: true}).isVisible()); assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true); assert.deepEqual(errors, []);
 console.log('PASS: 新库首次配置、重复初始化失败、配置与身份持久化、仅本厂配置、Agent 连接、在线阻止停用、离线停用与重新启用、窄屏页面');
} finally {clearTimeout(deadline); await browser?.close(); children.forEach(stop); await delay(500); await postgres?.close(); await rm(dir, {recursive: true, force: true});}
