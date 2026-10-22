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

async function newAdminPage(permissionCodes) {
  const context = await browser.newContext({ viewport: { width: 1366, height: 900 } });
  await context.addInitScript(() => localStorage.setItem('react-admin-auth', JSON.stringify({
    token: 'Bearer standalone-management-smoke',
    user: { id: 1, username: 'local-admin', nickname: '本地管理员', roles: [{ id: 1, roleName: '本地管理员', roleCode: 'ADMIN' }] },
  })));
  const page = await context.newPage();
  page.setDefaultTimeout(7000);
  const capabilities = { nodeRole: 'standalone', nodeSiteId: 1, permissionCodes, sites: [], scales: [], userId: 1 };
  const apiRoute = (url) => new URL(url).pathname.startsWith('/api/');
  await context.route(apiRoute, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/auth/menus') return route.fulfill({ json: { code: 200, message: 'ok', data: [] } });
    if (path === '/api/weighing/capabilities') return route.fulfill({ json: { code: 200, message: 'ok', data: capabilities } });
    await route.fulfill({ json: {} });
  });
  return { context, page, apiRoute };
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

  // While nodeRole is unresolved, old approval/sync navigation must stay hidden.
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
      return route.fulfill({ json: { code: 200, message: 'ok', data: { nodeRole: 'standalone', nodeSiteId: 1, permissionCodes: ['system:user', 'system:role', 'agent:maintenance:view'], sites: [], scales: [], userId: 1 } } });
    }
    return route.fulfill({ json: {} });
  });
  await pending.page.goto(`${webUrl}/system/agent-maintenance`);
  await Promise.race([menusLoaded, delay(7000).then(async () => { throw new Error(`menus request timed out; page=${(await pending.page.locator('body').innerText()).slice(0, 500)} errors=${pageErrors.join(' | ')} vite=${vite.output}`); })]);
  await delay(100);
  assert.equal(await pending.page.getByText('临时主体登记与确认', { exact: true }).count(), 0, 'center approval menu must not flash before node mode loads');
  releaseCapabilities();
  await pending.page.getByRole('heading', { name: '设备维护' }).waitFor();
  assert.equal(await pending.page.getByRole('link', { name: '临时主体登记与确认' }).count(), 0, 'standalone mode must hide the legacy approval entry');
  await pending.page.getByRole('button', { name: '设备与打印' }).click();
  for (const actionName of ['启动打印 Agent', '停止打印 Agent', '重启打印 Agent', '重启 CUPS', '恢复目标队列', '打印校准样张']) {
    assert.equal(await pending.page.getByRole('button', { name: actionName, exact: true }).count(), 0, `view-only user must not see ${actionName}`);
  }
  await pending.page.getByRole('button', { name: '配置', exact: true }).click();
  assert.equal(await pending.page.getByRole('button', { name: '保存草稿', exact: true }).count(), 0);
  assert.equal(await pending.page.getByRole('button', { name: '应用配置并重启 Agent', exact: true }).count(), 0);
  await pending.context.close();

  // Unknown runtime and job status are explicit; a stopped Agent remains startable.
  const managed = await newAdminPage(['system:user', 'system:role', 'agent:maintenance:manage']);
  let storedDraft;
  let actionPayload;
  let configRead = 0;
  await managed.context.unroute(managed.apiRoute);
  await managed.context.route(managed.apiRoute, async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === '/api/auth/menus') return route.fulfill({ json: { code: 200, message: 'ok', data: [] } });
    if (path === '/api/weighing/capabilities') return route.fulfill({ json: { code: 200, message: 'ok', data: { nodeRole: 'standalone', nodeSiteId: 1, permissionCodes: ['system:user', 'system:role', 'agent:maintenance:manage'], sites: [], scales: [], userId: 1 } } });
    if (path.endsWith('/status')) return route.fulfill({ json: {
      checkedAt: new Date().toISOString(), maintenance: '运行中', agent: '已停止', agentKnown: true,
      cups: '未知', cupsKnown: false, queue: 'unknown', queueState: '未知', queueKnown: false,
      jobsKnown: false, jobs: [], lastError: '', scale: { known: false, state: '未知' },
      camera: { frontConfigured: true, rearConfigured: false, recognitionConfigured: false, state: '状态仅在称重采集时确认' },
    } });
    if (path.endsWith('/config') && request.method() === 'GET') {
      configRead += 1;
      return route.fulfill({ json: {
        config: { serverUrl: 'http://127.0.0.1:8900', agentId: 'local-agent', scaleId: 1, queue: 'scale',
          printWidthMm: 210, printScale: 1, offsetXmm: 0, offsetYmm: 0, formLengthMm: 99,
          serialEnabled: false, serialPort: '/dev/ttyS0', serialBaud: 9600, serialProtocol: 'keli-d12-tf05',
          serialWeightUnit: 't', serialStaleMs: 800, cameraTimeoutMs: 3000,
          agentKey: 'secret-agent-key', cameraFrontUrl: 'rtsp://user:secret@camera/front',
          cameraRearUrl: 'rtsp://user:secret@camera/rear', plateRecognitionUrl: 'https://secret.local/recognize' },
        active: {}, hasPrevious: true, hasAgentKey: true, hasActiveAgentKey: true,
        hasCameraFrontUrl: true, hasCameraRearUrl: true, hasPlateRecognitionUrl: true,
        keyChanged: false, saved: false, environmentKnown: true, runtimeState: '存在待应用配置草稿',
      } });
    }
    if (path.endsWith('/config') && request.method() === 'PUT') {
      storedDraft = request.postDataJSON();
      return route.fulfill({ json: { saved: true } });
    }
    if (path.endsWith('/jobs')) return route.fulfill({ json: { jobs: [], previewToken: 'unknown-jobs-preview', unknown: true } });
    if (path.endsWith('/action')) {
      actionPayload = request.postDataJSON();
      return route.fulfill({ json: { output: '操作已执行' } });
    }
    return route.fulfill({ json: {} });
  });
  managed.page.on('dialog', (dialog) => dialog.accept());
  await managed.page.goto(`${webUrl}/system/agent-maintenance`);
  await managed.page.getByRole('heading', { name: '设备维护' }).waitFor();
  await managed.page.getByRole('button', { name: '启动打印 Agent', exact: true }).waitFor();
  assert.ok(await managed.page.getByText('未知', { exact: true }).count() > 0, 'unknown runtime fields should render as unknown');
  await managed.page.getByRole('button', { name: '配置', exact: true }).click();
  await managed.page.getByText('存在未应用草稿').waitFor();
  assert.equal(await managed.page.getByLabel('业务服务地址').inputValue(), 'http://127.0.0.1:8900', `config response should populate the local form (reads=${configRead})`);
  for (const label of ['连接密钥', '车前抓拍地址', '车后抓拍地址', '车牌识别服务地址']) {
    assert.equal(await managed.page.getByLabel(label).inputValue(), '', `${label} must never be echoed into the form`);
  }
  await managed.page.getByRole('button', { name: '保存草稿', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '配置草稿已保存' }).waitFor();
  for (const key of ['agentKey', 'cameraFrontUrl', 'cameraRearUrl', 'plateRecognitionUrl']) assert.equal(storedDraft?.[key], '', `${key} must be blank in frontend draft state: ${JSON.stringify(storedDraft)}`);
  await managed.page.getByRole('button', { name: '应用配置并重启 Agent', exact: true }).click();
  await managed.page.getByRole('status').filter({ hasText: '操作已执行' }).waitFor();
  assert.equal(actionPayload?.action, 'apply');
  assert.equal(actionPayload?.previewToken, 'unknown-jobs-preview', 'unknown job disposition must be bound to its preflight token');
  assert.equal(actionPayload?.disposition, 'unknown-confirmed');
  assert.ok(configRead >= 2, 'config must be reread after saving and applying');
  await managed.context.close();
  console.log('standalone management browser smoke passed');
} finally {
  await browser?.close();
  if (vite && vite.exitCode === null) vite.kill('SIGTERM');
}
