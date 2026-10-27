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
  const capabilities = { localSiteCode: 'F001', localScaleCode: 'WB01', configured: true, initialized: true, maintenanceEnabled, permissionCodes, sites: [], scales: [], userId: 1 };
  const localStatus = { siteCode: 'F001', scaleCode: 'WB01', configured: true, initialized: true };
  const apiRoute = (url) => new URL(url).pathname.startsWith('/api/');
  await context.route(apiRoute, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/auth/menus') return route.fulfill({ json: { code: 200, message: 'ok', data: [] } });
    if (path === '/api/weighing/capabilities') return route.fulfill({ json: { code: 200, message: 'ok', data: capabilities } });
    if (path === '/api/weighing/local-status') return route.fulfill({ json: { code: 200, message: 'ok', data: localStatus } });
    if (path === '/api/weighing/site-config') return route.fulfill({ json: { code: 200, message: 'ok', data: [] } });
    if (path === '/api/weighing/ticket-sequences') return route.fulfill({ json: { code: 200, message: 'ok', data: [] } });
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

  // Factory names are internal; configuration now focuses on the printed title.
  const factory = await newAdminPage(['weighing:read', 'weighing:confirm', 'system:config']);
  let titleRow = {id: 20, configName: '过磅单标题', configKey: 'weighing.ticket-title', configValue: '酒厂过磅单', configType: 'SYSTEM', valueType: 'TEXT', status: 1, isBuiltin: 1};
  let failTitleLoad = false;
  let failTitleSave = false;
  const writes = [];
  const identityWrites = [];
  let identityRow = {code: 'F001', name: '本厂', version: 1, scaleCode: 'WB01', scaleName: '本厂地磅', agentId: '', enabled: 0};
  await factory.context.route(factory.apiRoute, async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const respond = data => route.fulfill({json: {code: 200, message: 'ok', data}});
    if (path === '/api/system/config/page') {
      if (failTitleLoad) return route.fulfill({status: 500, json: {code: 500, message: '标题读取失败'}});
      return respond({records: [titleRow], total: 1, page: 1, pageSize: 10});
    }
    if (path === '/api/system/config/20' && request.method() === 'PUT') {
      writes.push(request.postDataJSON());
      if (failTitleSave) return route.fulfill({status: 500, json: {code: 500, message: '标题保存失败'}});
      titleRow = {...titleRow, ...request.postDataJSON()};
      return respond(null);
    }
    if (path.startsWith('/api/system/dict-data') || path.startsWith('/api/system/dict/')) return respond([]);
    if (path === '/api/weighing/site-config' && request.method() === 'PUT') {
      const body = request.postDataJSON();
      identityWrites.push(body);
      identityRow = {...identityRow, ...body, version: body.version + 1};
      return respond(identityRow);
    }
    if (path === '/api/weighing/site-config') return respond([identityRow]);
    return route.fallback();
  });
  await factory.page.goto(`${webUrl}/weighing/site-config`);
  await factory.page.waitForURL('**/system/local-identity');
  for (const name of ['首次配置', '编辑本厂配置', '新增厂区']) assert.equal(await factory.page.getByRole('button', {name, exact: true}).count(), 0);
  // Identity codes and names can be edited without changing the stopped state.
  const siteCodeInput = factory.page.getByLabel('厂区编码', {exact: true}).first();
  await siteCodeInput.waitFor();
  assert.equal(await siteCodeInput.inputValue(), 'F001');
  assert.equal(await siteCodeInput.isDisabled(), false);
  assert.equal(await factory.page.getByLabel('地磅编码', {exact: true}).inputValue(), 'WB01');
  await siteCodeInput.fill('F002');
  await factory.page.getByLabel('地磅编码', {exact: true}).fill('WB02');
  assert.equal(await factory.page.getByLabel('厂区名称', {exact: true}).inputValue(), '本厂');
  assert.equal(await factory.page.getByLabel('地磅名称', {exact: true}).inputValue(), '本厂地磅');
  // A disabled factory must stay disabled when only its names are maintained.
  assert.equal(await factory.page.getByLabel('启停状态', {exact: true}).inputValue(), '停用');
  await factory.page.getByLabel('厂区名称', {exact: true}).fill('本厂改名');
  await factory.page.getByRole('button', {name: '保存身份', exact: true}).click();
  await factory.page.getByText('本厂身份已保存', {exact: true}).waitFor();
  assert.equal(identityWrites.length, 1);
  assert.equal(identityWrites[0].enabled, 0, 'name save must preserve the disabled state');
  assert.equal(identityWrites[0].name, '本厂改名');
  assert.equal(identityWrites[0].code, 'F002');
  assert.equal(identityWrites[0].scaleCode, 'WB02');
  await siteCodeInput.waitFor();
  assert.equal(await siteCodeInput.inputValue(), 'F002');
  await factory.page.goto(`${webUrl}/system/ticket-title`);
  const titleInput = factory.page.getByLabel('打印标题', {exact: true});
  await titleInput.waitFor();
  assert.equal(await titleInput.inputValue(), '酒厂过磅单');
  await titleInput.fill('本厂新过磅单');
  await factory.page.getByRole('button', {name: '预览标题', exact: true}).click();
  const preview = factory.page.getByRole('dialog', {name: '过磅单标题预览', exact: true});
  await preview.getByRole('heading', {name: '本厂新过磅单', exact: true}).waitFor();
  assert.equal(writes.length, 0, 'preview must not save or print');
  await preview.getByRole('button', {name: '关闭详情弹窗', exact: true}).click();
  await factory.page.getByRole('button', {name: '保存标题', exact: true}).click();
  await factory.page.getByText('过磅单标题已保存', {exact: true}).waitFor();
  assert.equal(writes[0].configValue, '本厂新过磅单');
  assert.equal(writes[0].configKey, 'weighing.ticket-title');
  await factory.page.reload();
  await titleInput.waitFor();
  assert.equal(await titleInput.inputValue(), '本厂新过磅单');
  failTitleSave = true;
  await titleInput.fill('保存失败时保留的标题');
  await factory.page.getByRole('button', {name: '保存标题', exact: true}).click();
  await factory.page.getByRole('alert').filter({hasText: '标题保存失败'}).waitFor();
  assert.equal(await titleInput.inputValue(), '保存失败时保留的标题');
  await titleInput.fill('');
  const writesBeforeInvalid = writes.length;
  await factory.page.getByRole('button', {name: '保存标题', exact: true}).click();
  await factory.page.getByRole('alert').filter({hasText: '请填写 1～100 个字符的单行标题。'}).waitFor();
  assert.equal(writes.length, writesBeforeInvalid, 'empty title is rejected locally');
  await titleInput.fill('本厂新过磅单');
  for (const viewport of [{width: 1366, height: 768}, {width: 390, height: 844}]) {
    await factory.page.setViewportSize(viewport);
    await factory.page.waitForTimeout(300);
    await factory.page.screenshot({path: `/tmp/weighbridge-ticket-title-${viewport.width}.png`, fullPage: true});
    assert.equal(await factory.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'title configuration fits the viewport');
    await factory.page.getByRole('button', {name: '预览标题', exact: true}).click();
    await preview.getByRole('heading', {name: '本厂新过磅单', exact: true}).waitFor();
    assert.equal(await factory.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'preview scrolls inside the dialog on narrow screens');
    await factory.page.keyboard.press('Escape');
    await preview.waitFor({state: 'hidden'});
  }
  failTitleLoad = true;
  await factory.page.reload();
  await factory.page.getByRole('alert').filter({hasText: '标题读取失败'}).first().waitFor();
  await factory.page.getByRole('button', {name: '重新加载标题', exact: true}).waitFor();
  failTitleLoad = false;
  await factory.page.getByRole('button', {name: '重新加载标题', exact: true}).click();
  await titleInput.waitFor();
  factory.capabilities.permissionCodes = ['weighing:site:manage'];
  await factory.page.reload();
  await factory.page.getByRole('alert').filter({hasText: '无权访问过磅单标题'}).waitFor();
  assert.equal(await factory.page.getByRole('button', {name: '保存标题', exact: true}).count(), 0, 'retired site management permission does not grant title edits');
  assert.equal(await factory.page.getByRole('link', {name: '配置管理', exact: true}).count(), 0);
  await factory.page.goto(`${webUrl}/weighing/sites`);
  await factory.page.waitForURL('**/weighing/tasks');
  assert.equal(await factory.page.getByRole('button', {name: '踢出 Agent', exact: true}).count(), 0);
  await factory.context.close();

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
      return route.fulfill({ json: { code: 200, message: 'ok', data: { localSiteCode: 'F001', localScaleCode: 'WB01', configured: true, initialized: true, maintenanceEnabled: true, permissionCodes: ['system:user', 'system:role', 'agent:maintenance:view'], sites: [], scales: [], userId: 1 } } });
    }
    if (path.endsWith('/backups')) return route.fulfill({ json: { records: [{ id: 11, name: '只读备份', notes: '', source: 'auto', configVersion: 1, settings: { serial: { port: '/dev/ttyUSB0', baud: 9600 }, print: {} } }], total: 1, live: false } });
    return route.fulfill({ json: {} });
  });
  await pending.page.goto(`${webUrl}/system/agent-maintenance`);
  await Promise.race([menusLoaded, delay(7000).then(async () => { throw new Error(`menus request timed out; page=${(await pending.page.locator('body').innerText()).slice(0, 500)} errors=${pageErrors.join(' | ')} vite=${vite.output}`); })]);
  await delay(100);
  assert.equal(await pending.page.getByText('临时主体登记与确认', { exact: true }).count(), 0, 'retired approval menu must not appear while capabilities load');
  releaseCapabilities();
  await pending.page.getByRole('heading', { name: '设备维护', exact: true }).waitFor();
  assert.equal(await pending.page.getByRole('link', { name: '临时主体登记与确认' }).count(), 0, 'retired approval menu must remain hidden');
  await pending.page.getByRole('tab', { name: '打印机管理' }).click();
  for (const actionName of ['启动设备', '停止设备', '重载设备', '重启 CUPS', '恢复目标队列', '打印校准样张']) {
    assert.equal(await pending.page.getByRole('button', { name: actionName, exact: true }).count(), 0, `view-only user must not see ${actionName}`);
  }
  await pending.page.getByRole('tab', { name: '仪表管理', exact: true }).click();
  assert.equal(await pending.page.getByRole('button', { name: '保存草稿', exact: true }).count(), 0);
  assert.equal(await pending.page.getByRole('button', { name: '保存并应用配置', exact: true }).count(), 0);
  await pending.page.getByRole('tab', { name: '配置备份', exact: true }).click();
  await pending.page.getByRole('button', { name: '查看', exact: true }).waitFor();
  for (const name of ['创建备份', '恢复', '删除']) assert.equal(await pending.page.getByRole('button', { name, exact: true }).count(), 0);
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
  let backupCreates = 0;
  let backupOnline = true;
  const backups = [{ id: 11, name: '', notes: '', configVersion: 2, digest: 'sha256:v2', settings: structuredClone(currentSettings), appliedAt: '2026-10-23T10:20:30+08:00', source: 'auto', receivedAt: '2026-10-23T10:20:31+08:00' }];
  let agentRunning = false;
  let unknownJobs = false;
  let printerKnown = false;
  let diagnosticError = '暂无可读取错误';
  let diagnosticJobs = [];
  let deviceOnline = true;
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
    if (path === '/api/weighing/capabilities') return data({ localSiteCode: 'F001', localScaleCode: 'WB01', configured: true, initialized: true, maintenanceEnabled: true, permissionCodes: ['system:user', 'system:role', 'agent:maintenance:manage'], sites: [], scales: [], userId: 1 });
    if (path.endsWith('/status')) return route.fulfill({ json: ({
      checkedAt: new Date().toISOString(), maintenance: '运行中', maintenanceAvailable: deviceOnline, agentProcessOnline: deviceOnline,
      agent: agentRunning ? '运行中' : '已停止', agentKnown: true,
      cups: printerKnown ? '运行中' : '未知', cupsKnown: printerKnown, queue: 'Aisino-SK820-raw', queueState: printerKnown ? '已启用' : '未知', queueKnown: printerKnown,
      jobsKnown: !unknownJobs, jobs: diagnosticJobs, lastError: diagnosticError,
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
      }) });
    }
    if (path.endsWith('/backups') && method === 'GET') return route.fulfill({ json: { records: backups, total: backups.length, live: backupOnline, effective: { configVersion, digest: `sha256:v${configVersion}`, ...currentSettings, appliedAt: new Date().toISOString(), serialState: 'configured', printerState: 'configured' } } });
    if (path.endsWith('/backups') && method === 'POST') {
      const payload = request.postDataJSON();
      assert.equal(payload.baseVersion, configVersion);
      assert.equal('settings' in payload, false, 'backup settings must not come from browser form inputs');
      backupCreates += 1;
      backups.unshift({ id: 12, name: payload.name, notes: payload.notes, createdBy: 1, configVersion, settings: structuredClone(currentSettings), source: 'manual', receivedAt: new Date().toISOString(), appliedAt: new Date().toISOString() });
      return route.fulfill({ status: 201, json: { id: 12 } });
    }
    if (path.endsWith('/backups/12') && method === 'DELETE') {
      backups.splice(backups.findIndex(record => record.id === 12), 1);
      return route.fulfill({ status: 204 });
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
  assert.equal(await managed.page.getByRole('tab', { name: '打印机管理', exact: true }).getAttribute('aria-selected'), 'true');
  await managed.page.getByRole('tab', { name: '打印机管理', exact: true }).press('Home');
  assert.equal(await managed.page.getByRole('tabpanel').count(), 1, 'only the active maintenance panel should be visible');

  // The legacy no-error text must not produce a warning; diagnostics live with printing.
  assert.equal(await managed.page.getByRole('heading', { name: '现场诊断', exact: true }).count(), 0);
  assert.equal(await managed.page.getByRole('heading', { name: '打印诊断', exact: true }).count(), 0);
  assert.equal(await managed.page.getByRole('heading', { name: '未结束打印作业', exact: true }).count(), 0);
  await managed.page.getByRole('tab', { name: '打印机管理', exact: true }).click();
  const diagnosis = managed.page.locator('section').filter({ has: managed.page.getByRole('heading', { name: '打印诊断', exact: true }) });
  await diagnosis.waitFor();
  await diagnosis.getByText('部分运行状态尚未确认', { exact: true }).waitFor();
  assert.equal(await diagnosis.locator('svg.text-warning').count(), 0, 'no-error placeholder must not show a warning');
  assert.equal(await diagnosis.getByText('暂无可读取错误', { exact: true }).count(), 0);
  printerKnown = true;
  await managed.page.getByRole('button', { name: '刷新状态', exact: true }).click();
  await diagnosis.getByText('未报告设备或打印服务错误', { exact: true }).waitFor();
  assert.equal(await diagnosis.locator('svg.text-success').count(), 1);
  diagnosticError = 'CUPS：打印服务不可用';
  diagnosticJobs = [{ id: 'Aisino-SK820-raw-42', queue: 'Aisino-SK820-raw' }];
  await managed.page.getByRole('button', { name: '刷新状态', exact: true }).click();
  await diagnosis.getByText(diagnosticError, { exact: true }).waitFor();
  assert.equal(await diagnosis.locator('svg.text-warning').count(), 1, 'real errors remain visible as warnings');
  await diagnosis.getByText('1 项待核实', { exact: true }).waitFor();
  await diagnosis.getByText(diagnosticJobs[0].id, { exact: true }).waitFor();
  diagnosticError = '';
  diagnosticJobs = [];
  unknownJobs = true;
  await managed.page.getByRole('button', { name: '刷新状态', exact: true }).click();
  await diagnosis.getByText('部分运行状态尚未确认', { exact: true }).waitFor();
  await diagnosis.getByText('未能确认作业状态，请先检查队列和现场纸面结果。', { exact: true }).waitFor();
  assert.equal(await diagnosis.locator('svg.text-success').count(), 0, 'unknown jobs must not imply a successful check');
  unknownJobs = false;
  deviceOnline = false;
  await managed.page.getByRole('button', { name: '刷新状态', exact: true }).click();
  await diagnosis.getByText('Agent 维护连接不可用', { exact: true }).waitFor();
  assert.equal(await diagnosis.locator('svg.text-success').count(), 0, 'offline devices must not imply a successful check');
  deviceOnline = true;
  diagnosticError = '暂无可读取错误';
  await managed.page.getByRole('button', { name: '刷新状态', exact: true }).click();
  await diagnosis.getByText('未报告设备或打印服务错误', { exact: true }).waitFor();
  await managed.page.getByRole('tab', { name: '运行总览', exact: true }).click();

  // Configuration contains exactly serial and print settings, submitted once.
  await managed.page.getByRole('tab', { name: '仪表管理', exact: true }).click();
  assert.equal(await managed.page.getByText('当前生效配置', { exact: true }).count(), 0);
  assert.equal(await managed.page.getByRole('button', { name: '恢复此备份', exact: true }).count(), 0);
  for (const legacy of ['保存草稿', '连接密钥', '车牌识别服务地址', '业务服务地址', 'RTSP 服务入口', '实时预览服务入口', '车前流名称', '车后流名称', '前后摄像头']) {
    assert.equal(await managed.page.getByText(legacy, { exact: true }).count(), 0, `${legacy} must not appear in the simplified maintenance page`);
  }
  assert.equal(await managed.page.getByLabel('串口路径', { exact: true }).inputValue(), '/dev/ttyUSB0');
  assert.equal(await managed.page.getByLabel('仪表波特率', { exact: true }).inputValue(), '9600');
  assert.equal(await managed.page.getByLabel('CUPS 打印队列', { exact: true }).count(), 0, 'instrument tab contains only serial settings');
  await managed.page.getByRole('tab', { name: '打印机管理', exact: true }).click();
  await managed.page.getByRole('heading', { name: '打印队列与校准', exact: true }).waitFor();
  assert.equal(await managed.page.getByLabel('串口路径', { exact: true }).count(), 0, 'printer tab contains only print settings');
  assert.equal(await managed.page.getByLabel('单据节长（mm）', { exact: true }).getAttribute('step'), 'any', 'form length accepts decimals');
  assert.equal(await managed.page.getByLabel('CUPS 打印队列', { exact: true }).locator('option').filter({ hasText: 'Aisino-SK820-raw' }).count(), 1, 'queues come from the Agent CUPS list');
  assert.equal(await managed.page.getByRole('button', { name: '保存并应用', exact: true }).count(), 1, 'only one submit action');
  await managed.page.getByRole('tab', { name: '仪表管理', exact: true }).click();
  await managed.page.getByRole('heading', { name: '称重仪表', exact: true }).first().waitFor();
  await managed.page.getByLabel('串口路径', { exact: true }).fill('/dev/ttyUSB9');
  await managed.page.getByLabel('仪表波特率', { exact: true }).selectOption('19200');
  await managed.page.getByRole('tab', { name: '打印机管理', exact: true }).click();
  await managed.page.getByRole('tab', { name: '仪表管理', exact: true }).click();
  assert.equal(await managed.page.getByLabel('串口路径', { exact: true }).inputValue(), '/dev/ttyUSB9', 'tab switching preserves serial edits');
  await managed.page.getByRole('tab', { name: '打印机管理', exact: true }).click();
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

  // Backups are maintained only in their own tab and only created explicitly.
  await managed.page.getByRole('tab', { name: '仪表管理', exact: true }).click();
  await managed.page.getByLabel('串口路径', { exact: true }).fill('/dev/ttyUSB8');
  assert.equal(backupCreates, 0, 'saving settings does not create a restore point');
  await managed.page.getByRole('tab', { name: '配置备份', exact: true }).click();
  await managed.page.getByText('历史自动备份', { exact: true }).waitFor();
  await managed.page.getByRole('button', { name: '创建备份', exact: true }).click();
  await managed.page.getByLabel('备份名称', { exact: true }).fill('现场校准完成');
  await managed.page.getByLabel('备份备注', { exact: true }).fill('五联纸已核对');
  await managed.page.getByRole('dialog').getByRole('button', { name: '创建备份', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '备份已创建' }).waitFor();
  assert.equal(backupCreates, 1);
  assert.equal(backups[0].settings.serial.port, '/dev/ttyUSB9', 'manual backup excludes unapplied edits');
  await managed.page.getByRole('tab', { name: '仪表管理', exact: true }).click();
  assert.equal(await managed.page.getByLabel('串口路径', { exact: true }).inputValue(), '/dev/ttyUSB8', 'visiting backup management preserves pending edits');
  await managed.page.getByRole('tab', { name: '配置备份', exact: true }).click();
  const manualBackup = managed.page.getByRole('row').filter({ hasText: '现场校准完成' });
  await manualBackup.getByRole('button', { name: '查看', exact: true }).click();
  await managed.page.getByRole('dialog').getByText('备份与当前生效参数对比', { exact: true }).waitFor({ state: 'attached' });
  await managed.page.getByRole('button', { name: '关闭详情弹窗', exact: true }).click();
  await manualBackup.getByRole('button', { name: '删除', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '备份已删除' }).waitFor();
  const historicBackup = managed.page.getByRole('row').filter({ hasText: '历史自动备份' });
  await historicBackup.getByRole('button', { name: '恢复', exact: true }).click();
  await managed.page.getByRole('dialog').getByText('（不同）', { exact: true }).first().waitFor();
  await managed.page.getByRole('button', { name: '确认恢复', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '已明确发起恢复' }).waitFor();
  assert.deepEqual(restorePayload, { backupId: 11, baseVersion: 4 }, 'restore is explicit and version checked');
  assert.equal(backupCreates, 1, 'restoring does not create another backup');
  backupOnline = false;
  await managed.page.getByRole('button', { name: '刷新备份与状态', exact: true }).click();
  await managed.page.getByText('Agent 离线或状态未确认，仅展示最近上报信息，不能创建或恢复备份。', { exact: true }).waitFor();
  assert.equal(await managed.page.getByRole('button', { name: '创建备份', exact: true }).isDisabled(), true);
  assert.equal(await historicBackup.getByRole('button', { name: '恢复', exact: true }).isDisabled(), true);
  await historicBackup.getByRole('button', { name: '查看', exact: true }).click();
  await managed.page.getByRole('button', { name: '关闭详情弹窗', exact: true }).click();
  backupOnline = true;
  await managed.page.getByRole('button', { name: '刷新备份与状态', exact: true }).click();
  await managed.page.screenshot({ path: '/tmp/weighbridge-maintenance-backups-desktop.png', fullPage: true });

  // Device execution stays controllable and a stopped Agent remains startable.
  await managed.page.getByRole('tab', { name: '运行总览', exact: true }).click();
  await managed.page.getByRole('button', { name: '启动设备', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '操作已执行' }).waitFor();
  assert.equal(actionPayload?.action, 'start-agent');
  const notifications = managed.page.getByRole('region', { name: '消息通知', exact: true });
  const startedToast = notifications.getByRole('status').filter({ hasText: '操作已执行' });
  await startedToast.waitFor();
  assert.equal(await managed.page.getByRole('tabpanel').getByRole('status').filter({ hasText: '操作已执行' }).count(), 0, 'operation feedback must not occupy a page banner');
  await managed.page.screenshot({ path: '/tmp/weighbridge-maintenance-toast-desktop.png', fullPage: true });
  await startedToast.getByRole('button', { name: '关闭通知', exact: true }).click();
  await startedToast.waitFor({ state: 'hidden' });

  await managed.page.getByRole('tab', { name: '打印机管理', exact: true }).click();
  await managed.page.getByRole('button', { name: '打印校准样张', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '校准样张请求已提交' }).waitFor();
  assert.equal(await managed.page.getByText('测试前后抓拍', { exact: true }).count(), 0, 'camera testing moved to the Server camera module');
  const printToast = notifications.getByRole('status').filter({ hasText: '校准样张请求已提交' });
  await printToast.getByRole('button', { name: '关闭通知', exact: true }).waitFor();
  await managed.page.getByRole('tab', { name: '仪表管理', exact: true }).click();
  await printToast.waitFor();
  await printToast.waitFor({ state: 'hidden', timeout: 6000 });


  // Unknown print jobs require explicit confirmation and never auto-replay.
  unknownJobs = true;
  await managed.page.getByRole('tab', { name: '运行总览', exact: true }).click();
  await managed.page.getByRole('button', { name: '重载设备', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '操作已执行' }).waitFor();
  assert.equal(actionPayload?.action, 'restart-agent');
  await managed.page.setViewportSize({ width: 390, height: 844 });
  await managed.page.screenshot({ path: '/tmp/weighbridge-maintenance-toast-mobile.png', fullPage: true });
  assert.equal(await managed.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'toast fits a narrow viewport');
  await notifications.getByRole('status').filter({ hasText: '操作已执行' }).getByRole('button', { name: '关闭通知', exact: true }).click();
  await managed.page.setViewportSize({ width: 1366, height: 900 });

  assert.equal(actionPayload?.disposition, 'unknown-confirmed', 'unknown job state is explicitly acknowledged');

  // Records and logs are read on demand.
  await managed.page.getByRole('tab', { name: '维护记录', exact: true }).click();
  await managed.page.getByRole('cell', { name: '启动设备', exact: true }).waitFor();
  await managed.page.getByText('设备维护日志：已读取状态，等待现场核实。', { exact: true }).waitFor();
  const readsAfterLoad = auditReads;
  for (const label of ['运行总览', '打印机管理', '仪表管理', '配置备份', '维护记录']) {
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
