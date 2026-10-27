import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4198';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4198', '--strictPort'], {
  cwd: new URL('..', import.meta.url), stdio: ['ignore', 'pipe', 'pipe'],
});
let output = '';
vite.stdout.on('data', value => output += value);
vite.stderr.on('data', value => output += value);
let browser;
const deadline = setTimeout(() => { void browser?.close(); vite.kill('SIGTERM'); }, 120_000);
const operator = { id: 2, username: 'operator', nickname: '操作员', roles: [{ id: 2, roleName: '地磅操作员', roleCode: 'WEIGHING_OPERATOR' }] };
const admin = { ...operator, id: 1, username: 'admin', roles: [...operator.roles, { id: 1, roleName: '管理员', roleCode: 'ADMIN' }] };
const viewer = { ...operator, id: 3, username: 'viewer', roles: [{ id: 3, roleName: '查询员', roleCode: 'VIEWER' }] };
const menu = (id, menuName, path, children = []) => ({ id, menuName, path, menuType: children.length ? 'DIR' : 'MENU', visible: 1, sortOrder: id, children });
const menus = [menu(1, '日志管理', '/log', [menu(2, '登录日志', '/system/login-log'), menu(3, '操作日志', '/system/oper-log')]), menu(4, '文件管理', '/system/file')];
const capabilities = {
  userId: 2, localSiteCode: 'F001', localScaleCode: 'WB01', configured: true, initialized: true,
  permissionCodes: ['weighing:read', 'weighing:master:read', 'weighing:confirm', 'system:log', 'system:login-log', 'system:oper-log'],
  sites: [{ code: 'F001', name: '本厂', enabled: 1, canOperate: true }],
  scales: [{ code: 'WB01', siteCode: 'F001', name: '本厂地磅', siteName: '本厂', enabled: 1, canOperate: true }],
};

async function fixture(user, { authenticated = false, denied = false, failure = false } = {}) {
  const context = await browser.newContext({ viewport: { width: 1366, height: 768 } });
  const requests = [];
  const state = { denied, failure };
  if (authenticated) await context.addInitScript(value => localStorage.setItem('react-admin-auth', JSON.stringify({ token: 'operator-fixture', user: value })), user);
  await context.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    requests.push(path);
    let data = { records: [], total: 0, page: 1, pageSize: 10 };
    if (path.endsWith('/capabilities')) {
      if (state.failure) return route.fulfill({ status: 503, json: { code: 503, message: '权限服务暂不可用' } });
      data = { ...capabilities, userId: user.id, scales: capabilities.scales.map(scale => ({ ...scale, canOperate: !state.denied })) };
    } else if (path === '/api/auth/login') data = { tokenValue: 'operator-fixture', tokenName: 'Authorization', expiresIn: 3600 };
    else if (path === '/api/auth/me') data = user;
    else if (path === '/api/auth/menus') data = menus;
    else if (path === '/api/weighing/overview') data = {
      date: '2026-10-26', checkedAt: '2026-10-26T01:00:00Z', validTickets: 0,
      inbound: { taskCount: 0, net: '0' }, outbound: { taskCount: 0, net: '0' },
      awaitingCount: 0, overnightCount: 0, awaitingTasks: [], recentTickets: [],
    };
    else if (path === '/api/weighing/cameras') data = { cameras: [], configVersion: 1 };
    else if (path.endsWith('/unread-count')) data = 0;
    else if (path.endsWith('/session')) data = { scaleCode: 'WB01', holderUserId: null, expiresAt: null };
    else if (path.endsWith('/device')) data = { online: false, stable: false, deviceOk: false, weight: '0', revision: 1 };
    await route.fulfill({ json: { code: 200, message: 'success', data } });
  });
  const page = await context.newPage();
  page.setDefaultTimeout(10_000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  return { context, page, requests, errors, state };
}

try {
  for (let attempt = 0; ; attempt++) {
    if (vite.exitCode !== null || attempt === 60) throw Error(`Vite startup failed: ${output}`);
    try { if ((await fetch(base)).ok) break; } catch { /* Startup retry. */ }
    await delay(150);
  }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  for (const [user, target] of [[operator, '/weighing/workbench?scale=WB01'], [admin, '/dashboard'], [viewer, '/dashboard']]) {
    const { context, page, requests, errors } = await fixture(user);
    await page.goto(`${base}/login`);
    await page.locator('#username').waitFor().catch(async error => {
      throw new Error(`${error.message}\nPage errors: ${JSON.stringify(errors)}\nBody: ${await page.locator('body').innerText()}`);
    });
    await page.locator('#username').fill(user.username);
    await page.locator('#password').fill('fixture-password');
    await page.getByRole('button', { name: '登录', exact: true }).click();
    await page.waitForURL(`${base}${target}`);
    if (user === operator) {
      await page.getByRole('heading', { name: '称重工作台', exact: true }).waitFor().catch(async error => {
        throw new Error(`${error.message}\nPage errors: ${JSON.stringify(errors)}\nBody: ${await page.locator('body').innerText()}\nRequests: ${JSON.stringify(requests)}`);
      });
      await page.waitForRequest('**/api/weighing/scales/WB01/device');
      assert.ok(!requests.includes('/api/weighing/scales//device'), '登录后绑定实际本厂地磅');
    } else {
      await page.getByRole('heading', { name: '系统概览', exact: true }).waitFor();
      await page.goto(`${base}/`);
      await page.waitForURL(`${base}/dashboard`);
      await page.getByRole('heading', { name: '系统概览', exact: true }).waitFor();
    }
    assert.deepEqual(errors, []);
    await context.close();
  }

  const { context, page, errors } = await fixture(operator, { authenticated: true });
  await page.goto(`${base}/`);
  await page.waitForURL(`${base}/weighing/workbench?scale=WB01`);
  await page.goto(`${base}/weighing/tasks`);
  const nav = page.getByRole('navigation', { name: '主导航', exact: true });
  await nav.getByRole('button', { name: '日志管理', exact: true }).waitFor();
  assert.deepEqual(await nav.locator(':scope > div > button, :scope > div > a').allTextContents(), ['单据查询', '基础资料', '日志管理', '系统概览', '称重工作台']);
  await nav.getByRole('link', { name: '系统概览', exact: true }).click();
  await page.waitForURL(`${base}/dashboard`);
  await page.getByRole('heading', { name: '系统概览', exact: true }).waitFor();
  await nav.getByRole('button', { name: '日志管理', exact: true }).click();
  await nav.getByRole('link', { name: '登录日志', exact: true }).click();
  await page.waitForURL(`${base}/system/login-log`);
  await page.getByRole('button', { name: '菜单搜索', exact: true }).filter({ visible: true }).click();
  const search = page.getByRole('combobox', { name: '搜索菜单' });
  await search.fill('系统概览');
  await page.getByRole('option').filter({ hasText: '系统概览' }).waitFor();
  await search.fill('开发示例');
  await page.getByText('未找到匹配菜单', { exact: true }).waitFor();
  await search.fill('称重工作台');
  const popupPromise = page.waitForEvent('popup');
  await page.getByRole('option').filter({ hasText: '称重工作台' }).click();
  const popup = await popupPromise;
  await popup.waitForURL(`${base}/weighing/workbench?scale=WB01`);
  await popup.close();
  assert.deepEqual(errors, []);
  await context.close();

  for (const options of [{ denied: true }, { failure: true }]) {
    const { context, page, state } = await fixture(operator, { authenticated: true, ...options });
    await page.goto(`${base}/weighing/workbench`);
    await page.getByRole('alert').waitFor();
    assert.equal(new URL(page.url()).pathname, '/weighing/workbench', '权限错误不自动退回查询页面');
    state.denied = false;
    state.failure = false;
    await page.getByRole('button', { name: '重试', exact: true }).click();
    await page.waitForURL(`${base}/weighing/workbench?scale=WB01`);
    await context.close();
  }

  const redirected = await fixture(operator);
  await redirected.page.goto(`${base}/login?redirect=${encodeURIComponent('/weighing/tickets?status=valid')}`);
  await redirected.page.locator('#username').fill('operator');
  await redirected.page.locator('#password').fill('fixture-password');
  await redirected.page.getByRole('button', { name: '登录', exact: true }).click();
  await redirected.page.waitForURL(`${base}/weighing/tickets?status=valid`);
  await redirected.context.close();
  console.log('operator navigation passed');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
