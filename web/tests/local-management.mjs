import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const webDir = fileURLToPath(new URL('..', import.meta.url));
const webUrl = 'http://127.0.0.1:4191';
let vite;
let browser;

function startVite() {
  vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4191', '--strictPort'], {
    cwd: webDir,
    env: { ...process.env, VITE_API_BASE_URL: '' },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  vite.output = '';
  vite.stdout.on('data', (chunk) => { vite.output += chunk; });
  vite.stderr.on('data', (chunk) => { vite.output += chunk; });
}

async function waitForVite() {
  for (let attempt = 0; attempt < 80; attempt += 1) {
    if (vite.exitCode !== null) throw new Error(`Vite stopped early:\n${vite.output}`);
    try { if ((await fetch(`${webUrl}/login`)).ok) return; } catch { /* Still starting. */ }
    await delay(125);
  }
  throw new Error(`Vite did not become ready:\n${vite.output}`);
}

async function newAdminPage(permissionCodes, maintenanceEnabled = true) {
  const context = await browser.newContext({ viewport: { width: 1366, height: 900 } });
  await context.addInitScript(() => localStorage.setItem('react-admin-auth', JSON.stringify({
    token: 'Bearer local-management-smoke',
    user: { id: 1, username: 'local-admin', nickname: '本地管理员', roles: [{ id: 1, roleName: '本地管理员', roleCode: 'ADMIN' }] },
  })));
  const page = await context.newPage();
  page.setDefaultTimeout(7000);
  const capabilities = { localSiteId: 1, configured: true, initialized: true, maintenanceEnabled, permissionCodes, sites: [], scales: [], userId: 1 };
  const localStatus = { siteId: 1, configured: true, initialized: true };
  const apiRoute = (url) => new URL(url).pathname.startsWith('/api/');
  await context.route(apiRoute, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/auth/menus') return route.fulfill({ json: { code: 200, message: 'ok', data: [] } });
    if (path === '/api/weighing/capabilities') return route.fulfill({ json: { code: 200, message: 'ok', data: capabilities } });
    if (path === '/api/weighing/local-status') return route.fulfill({ json: { code: 200, message: 'ok', data: localStatus } });
    if (path === '/api/weighing/site-config') return route.fulfill({ json: { code: 200, message: 'ok', data: [] } });
    if (path === '/api/system/config/page') return route.fulfill({ json: { code: 200, message: 'ok', data: { records: [], total: 0, page: 1, pageSize: 10 } } });
    await route.fulfill({ json: {} });
  });
  return { context, page, apiRoute, capabilities, localStatus };
}

const menu = {
  id: 99, parentId: 1, menuName: '临时主体登记与确认', menuType: 'MENU',
  path: '/weighing/temporary-parties', component: null, externalUrl: null,
  icon: 'Network', permissionCode: 'weighing:temporary:create', sortOrder: 1, visible: 1, children: [],
};

try {
  startVite();
  await waitForVite();
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });

  // Local identity controls both pages, even if an old/malformed response
  // includes another site first. Configuration-only admins need no business grant.
  const factory = await newAdminPage(['weighing:read', 'weighing:confirm', 'weighing:agent:kick', 'weighing:site:manage']);
  factory.capabilities.localSiteId = 2; factory.localStatus.siteId = 2;
  factory.capabilities.sites = [
    {id: 1, name: '其他厂区', code: 'OTHER', enabled: 1, canOperate: true},
    {id: 2, name: '本地酒厂', code: 'LOCAL', enabled: 1, canOperate: true},
  ];
  factory.capabilities.scales = factory.capabilities.sites.map(site => ({id: site.id + 10, siteId: site.id, siteName: site.name, code: `SCALE${site.id}`, name: `${site.name}地磅`, enabled: 1, canOperate: true}));
  let configuration = {id: 2, code: 'LOCAL', name: '本地酒厂', scaleId: 12, scaleCode: 'SCALE2', scaleName: '本地酒厂地磅', agentId: 'local-agent', enabled: 1, version: 4, serverUrl: 'http://192.168.1.20:8900'};
  const deviceRequests = [];
  let savedConfiguration;
  let failLoad = false;
  let failSave = false;
  await factory.context.route(factory.apiRoute, async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const respond = data => route.fulfill({json: {code: 200, message: 'ok', data}});
    if (path === '/api/weighing/site-config') {
      assert.equal(request.method(), 'GET', 'the current product must never create another site');
      if (failLoad) return route.fulfill({status: 500, json: {code: 500, message: '配置读取失败'}});
      return respond([{...configuration, id: 1, name: '其他厂区'}, configuration]);
    }
    if (path === '/api/weighing/site-config/2') {
      assert.equal(request.method(), 'PUT');
      if (failSave) return route.fulfill({status: 409, json: {code: 409, message: 'Agent 尚未离线'}});
      savedConfiguration = request.postDataJSON();
      configuration = {...configuration, ...savedConfiguration, version: configuration.version + 1};
      return respond(configuration);
    }
    if (/\/scales\/\d+\/device$/.test(path)) {
      deviceRequests.push(path);
      return respond({online: true, deviceOk: true, agentId: 'local-agent', connectionId: 'connection-12'});
    }
    return route.fallback();
  });
  await factory.page.goto(`${webUrl}/weighing/sites`);
  await factory.page.getByRole('heading', {name: '本厂称重', exact: true}).waitFor();
  await factory.page.getByRole('link', {name: '进入称重台', exact: true}).waitFor();
  assert.equal(await factory.page.getByRole('link', {name: '进入称重台', exact: true}).getAttribute('href'), '/weighing/workbench?scaleId=12');
  assert.equal(await factory.page.getByText('其他厂区', {exact: true}).count(), 0);
  assert.deepEqual([...new Set(deviceRequests)], ['/api/weighing/scales/12/device']);
  await factory.page.screenshot({path: '/tmp/weighbridge-local-entry.png', fullPage: true});
  factory.capabilities.sites = []; // manage-only accounts still maintain the configured local identity
  await factory.page.goto(`${webUrl}/system/config`);
  await factory.page.getByRole('button', {name: '编辑本厂配置', exact: true}).waitFor();
  assert.equal(await factory.page.getByRole('link', {name: '配置管理', exact: true}).count(), 1, 'configuration has one sidebar location');
  for (const name of ['新增厂区', '下载启动配置', '查找厂区', '厂区列表', '其他厂区']) assert.equal(await factory.page.getByText(name, {exact: true}).count(), 0);
  await factory.page.getByRole('button', {name: '编辑本厂配置', exact: true}).click();
  for (const key of ['code', 'scaleCode', 'agentId']) assert.equal(await factory.page.locator(`#site-${key}`).count(), 0, 'internal identity is not shown');
  await factory.page.locator('#site-name').fill('本厂新名称');
  await factory.page.locator('#site-scaleName').fill('本厂一号磅');
  await factory.page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click();
  await factory.page.getByRole('dialog').waitFor({state: 'hidden'});
  await factory.page.getByText('本厂新名称', {exact: true}).waitFor();
  assert.equal(savedConfiguration.version, 4, 'preserve optimistic locking');
  assert.equal('code' in savedConfiguration, false, 'internal codes are never submitted');
  assert.equal('scaleCode' in savedConfiguration, false, 'internal codes are never submitted');
  assert.equal('agentId' in savedConfiguration, false, 'Agent association is not an editable field');
  assert.equal('serverUrl' in savedConfiguration, false, '本厂资料编辑不再提交 Server 地址');
  assert.equal(await factory.page.getByText('本厂 Server 地址', {exact: true}).count(), 0);
  assert.equal(await factory.page.getByText(/两名实名操作员/).count(), 0);
  await factory.page.getByText('已配置', {exact: true}).waitFor();
  failSave = true;
  await factory.page.getByRole('button', {name: '编辑本厂配置', exact: true}).click();
  await factory.page.getByLabel('状态', {exact: true}).selectOption('0');
  await factory.page.getByRole('dialog').getByRole('button', {name: '保存', exact: true}).click();
  await factory.page.getByRole('alert').filter({hasText: 'Agent 尚未离线'}).waitFor();
  assert.equal(await factory.page.getByLabel('状态', {exact: true}).inputValue(), '0', 'retain rejected changes for correction');
  await factory.page.getByRole('button', {name: '取消', exact: true}).click();
  for (const viewport of [{width: 1366, height: 768}, {width: 390, height: 844}]) {
    await factory.page.setViewportSize(viewport);
    await factory.page.waitForTimeout(250);
    assert.equal(await factory.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'local configuration fits the viewport');
    await factory.page.screenshot({path: `/tmp/weighbridge-local-config-${viewport.width}.png`, fullPage: true});
  }
  factory.capabilities.permissionCodes = ['weighing:site:export'];
  await factory.page.reload();
  await factory.page.getByText('当前账号仅可查看本厂配置。', {exact: true}).waitFor();
  assert.equal(await factory.page.getByRole('button', {name: '编辑本厂配置', exact: true}).count(), 0);
  failLoad = true;
  await factory.page.getByRole('button', {name: '刷新', exact: true}).click();
  await factory.page.getByRole('alert').filter({hasText: '配置读取失败'}).waitFor();
  assert.equal(await factory.page.getByText('暂无可维护的本厂配置', {exact: true}).count(), 0, 'load errors are distinct from missing configuration');
  failLoad = false;
  factory.capabilities.localSiteId = 99; factory.localStatus.siteId = 99;
  await factory.page.reload();
  await factory.page.getByText('未找到本厂配置记录', {exact: true}).waitFor();
  assert.equal(await factory.page.getByRole('button', {name: '编辑本厂配置', exact: true}).count(), 0, 'missing local row must not fall back to another site');
  factory.capabilities.localSiteId = 2; factory.localStatus.siteId = 2;
  factory.capabilities.sites = [{id: 2, name: '本地酒厂', code: 'LOCAL', enabled: 0, canOperate: true}];
  factory.capabilities.permissionCodes = ['weighing:read', 'weighing:confirm', 'weighing:agent:kick'];
  await factory.page.goto(`${webUrl}/weighing/sites`);
  await factory.page.getByText('本厂已停用，历史业务仍可查询', {exact: true}).waitFor();
  assert.equal(await factory.page.getByRole('link', {name: '进入称重台', exact: true}).count(), 0);
  assert.equal(await factory.page.getByRole('button', {name: '踢出 Agent', exact: true}).count(), 0);
  await factory.context.close();

  const unconfigured = await newAdminPage(['weighing:site:manage']);
  unconfigured.capabilities.localSiteId = 0; unconfigured.capabilities.configured = false; unconfigured.capabilities.initialized = false;
  unconfigured.localStatus.siteId = 0; unconfigured.localStatus.configured = false; unconfigured.localStatus.initialized = false;
  const ambiguous = unconfigured;
  ambiguous.capabilities.sites = [{id: 1}, {id: 2}];
  await ambiguous.page.goto(`${webUrl}/weighing/sites`);
  await ambiguous.page.getByRole('heading', {name: '尚未完成本厂配置', exact: true}).waitFor();
  assert.equal(await ambiguous.page.getByRole('link', {name: '进入称重台', exact: true}).count(), 0);
  await ambiguous.page.goto(`${webUrl}/system/config`);
  await ambiguous.page.getByRole('button', {name: '首次配置', exact: true}).waitFor();
  assert.equal(await ambiguous.page.getByRole('link', {name: '配置管理', exact: true}).count(), 1);
  await ambiguous.context.close();

  // Maintenance remains discoverable through the same local account and RBAC grants.
  const development = await newAdminPage(['agent:maintenance:view']);
  await development.page.goto(`${webUrl}/system/agent-maintenance`);
  await development.page.getByRole('heading', { name: '设备维护', exact: true }).waitFor();
  assert.equal(await development.page.getByRole('link', { name: '设备维护', exact: true }).count(), 1, 'enabled maintenance must appear in the default development sidebar');
  await development.context.close();

  for (const [label, permissions, enabled] of [
    ['disabled backend', ['agent:maintenance:view'], false],
    ['missing permission', [], true],
  ]) {
    const hidden = await newAdminPage(permissions, enabled);
    await hidden.page.goto(`${webUrl}/system/agent-maintenance`);
    await hidden.page.getByRole('heading', { name: '设备维护', exact: true }).waitFor();
    assert.equal(await hidden.page.getByRole('link', { name: '设备维护', exact: true }).count(), 0, `${label} must hide the maintenance menu`);
    await hidden.context.close();
  }

  // Obsolete center approval menus are retired from the local navigation.
  const pending = await newAdminPage(['system:user', 'system:role', 'agent:maintenance:view']);
  const pageErrors = [];
  pending.page.on('pageerror', (error) => pageErrors.push(error.message));
  pending.page.on('console', (message) => { if (message.type() === 'error') pageErrors.push(message.text()); });
  let releaseCapabilities;
  let markMenusLoaded;
  const capabilityGate = new Promise((resolve) => { releaseCapabilities = resolve; });
  const menusLoaded = new Promise((resolve) => { markMenusLoaded = resolve; });
  await pending.context.unroute(pending.apiRoute);
  await pending.context.route(pending.apiRoute, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/auth/menus') {
      markMenusLoaded();
      return route.fulfill({ json: { code: 200, message: 'ok', data: [menu] } });
    }
    if (path === '/api/weighing/capabilities') {
      await capabilityGate;
      return route.fulfill({ json: { code: 200, message: 'ok', data: { localSiteId: 1, configured: true, initialized: true, maintenanceEnabled: true, permissionCodes: ['system:user', 'system:role', 'agent:maintenance:view'], sites: [], scales: [], userId: 1 } } });
    }
    return route.fulfill({ json: {} });
  });
  await pending.page.goto(`${webUrl}/system/agent-maintenance`);
  await Promise.race([menusLoaded, delay(7000).then(async () => { throw new Error(`menus request timed out; page=${(await pending.page.locator('body').innerText()).slice(0, 500)} errors=${pageErrors.join(' | ')} vite=${vite.output}`); })]);
  await delay(100);
  assert.equal(await pending.page.getByText('临时主体登记与确认', { exact: true }).count(), 0, 'retired approval menu must not appear while capabilities load');
  releaseCapabilities();
  await pending.page.getByRole('heading', { name: '设备维护', exact: true }).waitFor();
  assert.equal(await pending.page.getByRole('link', { name: '临时主体登记与确认' }).count(), 0, 'retired approval menu must remain hidden');
  await pending.page.getByRole('tab', { name: '设备与打印' }).click();
  for (const actionName of ['启动设备', '停止设备', '重载设备', '重启 CUPS', '恢复目标队列', '打印校准样张']) {
    assert.equal(await pending.page.getByRole('button', { name: actionName, exact: true }).count(), 0, `view-only user must not see ${actionName}`);
  }
  await pending.page.getByRole('tab', { name: '配置', exact: true }).click();
  assert.equal(await pending.page.getByRole('button', { name: '保存草稿', exact: true }).count(), 0);
  assert.equal(await pending.page.getByRole('button', { name: '保存并应用配置', exact: true }).count(), 0);
  await pending.context.close();

  // Maintenance is a single verified device-settings submit; the page must not
  // expose Agent credentials, camera configuration or a draft workflow.
  const managed = await newAdminPage(['system:user', 'system:role', 'agent:maintenance:manage']);
  let currentSettings = { serial: { port: '/dev/ttyUSB0', baud: 9600 }, print: { queue: 'Aisino-SK820-raw', formLengthMm: 99, printWidthMm: 175, printScale: 1, offsetXmm: 0, offsetYmm: 0 } };
  let configVersion = 3;
  let applyPayload;
  let restorePayload;
  let actionPayload;
  let configRead = 0;
  let agentRunning = false;
  let unknownJobs = false;
  let emptyAudit = false;
  let auditReads = 0;
  const maintenanceErrors = [];
  managed.page.on('pageerror', error => maintenanceErrors.push(error.message));
  await managed.context.unroute(managed.apiRoute);
  await managed.context.route(managed.apiRoute, async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const method = request.method();
    const data = value => route.fulfill({ json: { code: 200, message: 'ok', data: value } });
    if (path === '/api/auth/menus') return route.fulfill({ json: { code: 200, message: 'ok', data: [] } });
    if (path === '/api/weighing/capabilities') return data({ localSiteId: 1, configured: true, initialized: true, maintenanceEnabled: true, permissionCodes: ['system:user', 'system:role', 'agent:maintenance:manage'], sites: [], scales: [], userId: 1 });
    if (path.endsWith('/status')) return route.fulfill({ json: ({
      checkedAt: new Date().toISOString(), maintenance: '运行中', maintenanceAvailable: true, agentProcessOnline: true,
      agent: agentRunning ? '运行中' : '已停止', agentKnown: true,
      cups: '未知', cupsKnown: false, queue: 'unknown', queueState: '未知', queueKnown: false,
      jobsKnown: !unknownJobs, jobs: [], lastError: '',
      server: 'http://127.0.0.1:8900',
      scale: agentRunning
        ? { known: true, deviceOk: true, stale: false, state: '稳定', weight: '0.000', port: currentSettings.serial.port, baud: currentSettings.serial.baud }
        : { known: false, state: '未知' },
    }) });
    if (path.endsWith('/config') && method === 'GET') {
      configRead += 1;
      return route.fulfill({ json: ({
        config: {}, active: {}, saved: true, environmentKnown: true,
        effective: {
          configVersion, digest: `sha256:v${configVersion}`, ...currentSettings,
          appliedAt: new Date().toISOString(), running: agentRunning,
          deviceState: agentRunning ? 'ready' : 'not_configured', serialState: 'configured', printerState: 'configured',
        },
        backup: {
          id: 11, configVersion: configVersion - 1, digest: `sha256:v${configVersion - 1}`,
          settings: { serial: { port: '/dev/ttyUSB0', baud: 9600 }, print: { queue: 'Aisino-SK820-raw', formLengthMm: 99, printWidthMm: 175, printScale: 1, offsetXmm: 0, offsetYmm: 0 } },
          appliedAt: '2026-10-23T10:20:30+08:00', source: 'agent', receivedAt: '2026-10-23T10:20:31+08:00',
        },
      }) });
    }
    if (path.endsWith('/queues')) return route.fulfill({ json: { queues: [{ name: 'Aisino-SK820-raw', state: 'idle', isDefault: true }], at: new Date().toISOString() } });
    if (path.endsWith('/apply') && method === 'POST') {
      applyPayload = request.postDataJSON();
      currentSettings = applyPayload.settings;
      configVersion += 1;
      return route.fulfill({ json: { status: 'applied', configVersion, digest: `sha256:v${configVersion}`, appliedAt: new Date().toISOString() } });
    }
    if (path.endsWith('/restore') && method === 'POST') {
      restorePayload = request.postDataJSON();
      configVersion += 1;
      return route.fulfill({ json: { status: 'applied', configVersion, digest: `sha256:v${configVersion}`, appliedAt: new Date().toISOString() } });
    }
    if (path.endsWith('/jobs')) return route.fulfill({ json: { jobs: [], previewToken: 'maintenance-preview', unknown: unknownJobs } });
    if (path.endsWith('/test-print') && method === 'POST') return route.fulfill({ json: { message: '校准样张请求已提交，请核对纸面效果。' } });
    if (path.endsWith('/action')) {
      actionPayload = request.postDataJSON();
      if (actionPayload.action === 'start-agent') agentRunning = true;
      return route.fulfill({ json: { output: '操作已执行' } });
    }
    if (path.endsWith('/logs')) return route.fulfill({ contentType: 'text/plain', body: '设备维护日志：已读取状态，等待现场核实。' });
    if (path.endsWith('/audit')) {
      auditReads += 1;
      const records = emptyAudit ? [] : [{ id: 1, at: '2026-10-23T10:20:30+08:00', action: 'start-agent', target: 'local-agent', result: '操作已执行' }];
      return route.fulfill({ json: { records, total: records.length, offset: 0 } });
    }
    return route.fulfill({ json: {} });
  });
  managed.page.on('dialog', (dialog) => dialog.accept());
  await managed.page.goto(`${webUrl}/system/agent-maintenance`);
  await managed.page.getByRole('heading', { name: '设备维护', exact: true }).waitFor();
  await managed.page.getByRole('button', { name: '启动设备', exact: true }).waitFor();
  assert.ok(await managed.page.getByText('未知', { exact: true }).count() > 0, 'unknown runtime fields should render as unknown');
  await managed.page.screenshot({ path: '/tmp/weighbridge-maintenance-overview-desktop.png', fullPage: true });
  await managed.page.getByRole('tab', { name: '运行总览', exact: true }).press('ArrowRight');
  assert.equal(await managed.page.getByRole('tab', { name: '设备与打印', exact: true }).getAttribute('aria-selected'), 'true');
  await managed.page.getByRole('tab', { name: '设备与打印', exact: true }).press('Home');
  assert.equal(await managed.page.getByRole('tabpanel').count(), 1, 'only the active maintenance panel should be visible');

  // Configuration contains exactly serial and print settings, submitted once.
  await managed.page.getByRole('tab', { name: '配置', exact: true }).click();
  await managed.page.getByText('当前生效配置', { exact: true }).waitFor();
  await managed.page.getByText('成功配置备份', { exact: true }).waitFor();
  for (const legacy of ['保存草稿', '连接密钥', '车牌识别服务地址', '业务服务地址', 'RTSP 服务入口', '实时预览服务入口', '车前流名称', '车后流名称', '前后摄像头']) {
    assert.equal(await managed.page.getByText(legacy, { exact: true }).count(), 0, `${legacy} must not appear in the simplified maintenance page`);
  }
  assert.equal(await managed.page.getByLabel('串口路径', { exact: true }).inputValue(), '/dev/ttyUSB0');
  assert.equal(await managed.page.getByLabel('仪表波特率', { exact: true }).inputValue(), '9600');
  assert.equal(await managed.page.getByLabel('单据节长（mm）', { exact: true }).getAttribute('step'), 'any', 'form length accepts decimals');
  assert.equal(await managed.page.getByLabel('CUPS 打印队列', { exact: true }).locator('option').filter({ hasText: 'Aisino-SK820-raw' }).count(), 1, 'queues come from the Agent CUPS list');
  assert.equal(await managed.page.getByRole('button', { name: '保存并应用', exact: true }).count(), 1, 'only one submit action');
  await managed.page.getByLabel('串口路径', { exact: true }).fill('/dev/ttyUSB9');
  await managed.page.getByLabel('仪表波特率', { exact: true }).selectOption('19200');
  await managed.page.getByLabel('单据节长（mm）', { exact: true }).fill('93.13');
  await managed.page.getByLabel('水平偏移（mm）', { exact: true }).fill('2');
  await managed.page.getByLabel('垂直偏移（mm）', { exact: true }).fill('1.5');
  await managed.page.getByRole('button', { name: '保存并应用', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '配置已保存并应用' }).waitFor();
  assert.deepEqual(applyPayload, {
    baseVersion: 3,
    settings: {
      serial: { port: '/dev/ttyUSB9', baud: 19200 },
      print: { queue: 'Aisino-SK820-raw', formLengthMm: 93.13, printWidthMm: 175, printScale: 1, offsetXmm: 2, offsetYmm: 1.5 },
    },
  }, 'one submit carries the visible serial and print settings plus the base version');
  assert.ok(configRead >= 2, 'config must be reread after applying');

  // A clearly initiated restore uses the read-only Server backup.
  await managed.page.getByRole('button', { name: '恢复此备份', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '已明确发起恢复' }).waitFor();
  assert.deepEqual(restorePayload, { backupId: 11, baseVersion: 4 }, 'restore is explicit and version checked');

  // Device execution stays controllable and a stopped Agent remains startable.
  await managed.page.getByRole('tab', { name: '运行总览', exact: true }).click();
  await managed.page.getByRole('button', { name: '启动设备', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '操作已执行' }).waitFor();
  assert.equal(actionPayload?.action, 'start-agent');
  await managed.page.getByRole('tab', { name: '设备与打印', exact: true }).click();
  await managed.page.getByRole('button', { name: '打印校准样张', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '校准样张请求已提交' }).waitFor();
  assert.equal(await managed.page.getByText('测试前后抓拍', { exact: true }).count(), 0, 'camera testing moved to the Server camera module');

  // Unknown print jobs require explicit confirmation and never auto-replay.
  unknownJobs = true;
  await managed.page.getByRole('tab', { name: '运行总览', exact: true }).click();
  await managed.page.getByRole('button', { name: '重载设备', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '操作已执行' }).waitFor();
  assert.equal(actionPayload?.action, 'restart-agent');
  assert.equal(actionPayload?.disposition, 'unknown-confirmed', 'unknown job state is explicitly acknowledged');

  // Records and logs are read on demand.
  await managed.page.getByRole('tab', { name: '维护记录', exact: true }).click();
  await managed.page.getByRole('cell', { name: '启动设备', exact: true }).waitFor();
  await managed.page.getByText('设备维护日志：已读取状态，等待现场核实。', { exact: true }).waitFor();
  const readsAfterLoad = auditReads;
  for (const label of ['运行总览', '设备与打印', '配置', '维护记录']) {
    await managed.page.getByRole('tab', { name: label, exact: true }).click();
    await managed.page.setViewportSize({ width: 390, height: 844 });
    await managed.page.waitForTimeout(250);
    assert.equal(await managed.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, `${label} must fit a 390px viewport`);
    await managed.page.setViewportSize({ width: 1366, height: 900 });
    await managed.page.waitForTimeout(250);
  }
  await managed.page.getByRole('tab', { name: '维护记录', exact: true }).click();
  assert.equal(auditReads, readsAfterLoad, 'switching back to records should retain the loaded records');
  await managed.page.setViewportSize({ width: 1366, height: 900 });
  await managed.page.screenshot({ path: '/tmp/weighbridge-maintenance-records-desktop.png', fullPage: true, animations: 'disabled' });
  emptyAudit = true;
  await managed.page.getByRole('button', { name: '刷新记录', exact: true }).click();
  await managed.page.getByText('暂无维护记录', { exact: true }).waitFor();
  assert.deepEqual(maintenanceErrors, [], 'maintenance tabs must not produce browser runtime errors');
  await managed.context.close();
  console.log('local management browser smoke passed');
} finally {
  await browser?.close();
  if (vite && vite.exitCode === null) vite.kill('SIGTERM');
}
