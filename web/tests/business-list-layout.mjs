import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4193';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4193', '--strictPort'], {
  cwd: new URL('..', import.meta.url), stdio: ['ignore', 'pipe', 'pipe'],
});
let output = '';
vite.stdout.on('data', value => output += value);
vite.stderr.on('data', value => output += value);
let browser;
const deadline = setTimeout(() => { void browser?.close(); vite.kill('SIGTERM'); }, 90_000);
const user = {id: 1, username: 'admin', nickname: '管理员', roles: []};
const capabilities = {
  userId: 1, permissionCodes: ['weighing:read', 'weighing:confirm', 'weighing:export', 'weighing:print', 'weighing:master:read', 'weighing:master:write', 'weighing:site:manage', 'weighing:site:export'],
  sites: [{id: 1, name: '一厂区', code: 'SITE01', enabled: 1, canOperate: true}],
  scales: [{id: 1, name: '一号地磅', code: 'SCALE01', siteId: 1, siteName: '一厂区', enabled: 1, canOperate: true}],
};
const rows = Array.from({length: 12}, (_, index) => ({
  id: index + 1, siteId: 1, scaleId: 1, version: 1, taskId: index + 1,
  taskNo: `TASK202610220000${String(index + 1).padStart(4, '0')}`,
  ticketNo: `WB202610220000${String(index + 1).padStart(4, '0')}`,
  business: {plate: '川A12345', direction: 'inbound', materialName: '酿酒高粱', materialSpec: '优级', materialCode: 'GL001', partyName: '四川省粮食供应有限公司'},
  firstWeight: '35.125', secondWeight: '12.5', originalGross: '35.125', originalTare: '12.5', originalNet: '22.625',
  finalGross: '35.125', finalTare: '12.5', finalNet: '22.625', currentTicketId: index + 1,
  createTime: '2026-10-22T00:12:34Z', secondConfirmedAt: '2026-10-22T00:12:34Z', issuedAt: '2026-10-22T00:12:34Z', status: 'awaiting_second',
}));
const materials = rows.map(row => ({id: row.id, version: 1, enabled: 1, name: '酿酒高粱', code: `GL${row.id}`, categoryId: 1, spec: '一级原粮', remark: '按约定规格验收入库，保留完整备注。'}));
const parties = rows.map(row => ({id: row.id, version: 1, enabled: 1, name: '四川省粮食供应有限公司', code: `PARTY${row.id}`, shortName: '粮食供应', nature: 'enterprise', roles: ['supplier', 'carrier'], phone: '13800138000', contacts: [{id: 1, name: '张师傅', isDefault: 1}], address: '四川省宜宾市翠屏区产业园区粮食仓储中心一号仓库', remark: 'LONG_REFERENCE_WITHOUT_SPACES_'.repeat(3)}));
const sites = rows.map(row => ({id: row.id, code: `SITE${row.id}`, name: `第${row.id}厂区`, enabled: 1, version: 1, scaleId: row.id, scaleName: '一号地磅', scaleCode: `SCALE${row.id}`, agentId: 'scale-agent-printer-01'}));
const pageResult = records => ({records, total: records.length, page: 1, pageSize: 10});
const pages = [
  ['tasks', '运输任务列表', 9], ['originals', '原始称重列表', 8], ['tickets', '最终单据列表', 8],
  ['materials', '物料列表', 7], ['parties', '往来主体列表', 7], ['site-config', '厂区配置列表', 5],
  ['categories', '物料分类列表', 3],
];

try {
  for (let i = 0; i < 80; i++) {
    if (vite.exitCode !== null) throw Error(output);
    try { if ((await fetch(base)).ok) break; } catch { /* Startup retry. */ }
    if (i === 79) throw Error(`Vite startup timed out: ${output}`);
    await delay(150);
  }
  browser = await chromium.launch({headless: true, ignoreDefaultArgs: ['--hide-scrollbars'], ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? {executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH} : {})});
  const context = await browser.newContext({viewport: {width: 1366, height: 768}});
  await context.addInitScript(value => localStorage.setItem('react-admin-auth', JSON.stringify({token: 'layout-fixture', user: value})), user);
  await context.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    let data = pageResult([]);
    if (path.endsWith('/capabilities')) data = capabilities;
    else if (path === '/api/auth/me') data = user;
    else if (path === '/api/auth/menus') data = [];
    else if (path.endsWith('/unread-count')) data = 0;
    else if (path.endsWith('/summary')) data = {taskCount: 12, gross: '421.5', tare: '150', net: '271.5', rows: []};
    else if (path.endsWith('/material-categories')) data = pageResult(rows.map(row => ({id: row.id, enabled: 1, name: `原粮分类${row.id}`})));
    else if (path.endsWith('/materials')) data = pageResult(materials);
    else if (path.endsWith('/parties')) data = pageResult(parties);
    else if (path.endsWith('/tasks') || path.endsWith('/originals')) data = pageResult(rows.slice(0, 10));
    else if (path.endsWith('/tickets')) data = pageResult(rows.slice(0, 10).map(row => ({...row, status: 'valid'})));
    else if (path.endsWith('/site-config')) data = sites;
    else if (path.endsWith('/agent-settings')) data = {serverUrl: 'http://192.168.1.100:8900'};
    await route.fulfill({json: {code: 200, message: 'ok', data}});
  });
  const page = await context.newPage();
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  for (const viewport of [{width: 1366, height: 768}, {width: 1280, height: 720}, {width: 1024, height: 768}, {width: 390, height: 844}]) {
    await page.setViewportSize(viewport);
    for (const [path, label, count] of pages) {
      await page.goto(`${base}/weighing/${path}`);
      const region = page.getByRole('region', {name: label, exact: true});
      await region.locator('tbody tr').first().waitFor();
      assert.equal(await region.locator('thead th').count(), count, `${path}: all columns retained`);
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${path}: page overflow at ${viewport.width}`);
      const metrics = await region.evaluate(el => {
        const rect = e => {const r = e.getBoundingClientRect(); return {x: r.x, y: r.y, right: r.right, bottom: r.bottom};};
        return {frame: rect(el), first: rect(el.querySelector('tbody td')), last: rect(el.querySelector('tbody tr td:last-child')), middle: rect(el.querySelector('tbody tr td:nth-child(2)')), head: rect(el.querySelector('thead th')), width: el.clientWidth, scrollWidth: el.scrollWidth};
      });
      if (viewport.width >= 1024) {
        assert.ok(metrics.frame.bottom < viewport.height - (path === 'categories' ? 0 : 30), `${path}: scrollbar and pagination must fit screen: ${JSON.stringify(metrics.frame)}`);
        if (path !== 'categories') {
          const next = await page.getByRole('button', {name: '下一页', exact: true}).boundingBox();
          assert.ok(next.y + next.height <= viewport.height, `${path}: pagination below screen`);
        }
      }
      await region.evaluate(el => {el.scrollLeft = el.scrollWidth; el.scrollTop = el.scrollHeight;});
      await page.waitForTimeout(50);
      const after = await region.evaluate(el => {
        const rect = e => {const r = e.getBoundingClientRect(); return {x: r.x, y: r.y, right: r.right};};
        return {first: rect(el.querySelector('tbody td')), last: rect(el.querySelector('tbody tr td:last-child')), middle: rect(el.querySelector('tbody tr td:nth-child(2)')), head: rect(el.querySelector('thead th')), top: el.getBoundingClientRect().top, scrollLeft: el.scrollLeft, clipped: [...el.querySelectorAll('td')].filter(td => td.scrollWidth > td.clientWidth + 1).map(td => td.textContent)};
      });
      assert.ok(Math.abs(after.head.y - metrics.head.y) <= 1, `${path}: header moved on vertical scroll`);
      assert.deepEqual(after.clipped, [], `${path}: clipped cell contents`);
      if (viewport.width >= 1024 && path !== 'categories' && metrics.scrollWidth > metrics.width) {
        assert.ok(Math.abs(after.first.x - metrics.first.x) <= 1, `${path}: identifier did not stay fixed`);
        assert.ok(Math.abs(after.last.x - metrics.last.x) <= 1, `${path}: actions did not stay fixed`);
        assert.ok(after.middle.x < metrics.middle.x, `${path}: middle columns did not scroll`);
      }
      if (viewport.width < 1024) {
        assert.ok(after.last.right <= metrics.frame.right + 1, `${path}: last column inaccessible`);
      }
      if (path === 'tasks' && viewport.width === 1366) {
        await region.evaluate(el => {el.scrollLeft = 0; el.scrollTop = 0;});
        const frame = await region.boundingBox();
        await page.mouse.move(frame.x + 60, frame.y + frame.height - 6);
        await page.mouse.down();
        await page.mouse.move(frame.x + 260, frame.y + frame.height - 6, {steps: 10});
        await page.mouse.up();
        assert.ok(await region.evaluate(el => el.scrollLeft > 0), 'dragging horizontal scrollbar');
        await region.evaluate(el => {el.scrollLeft = 0;});
        await region.focus(); await page.keyboard.press('ArrowRight'); await page.waitForTimeout(150);
        assert.ok(await region.evaluate(el => el.scrollLeft > 0), 'keyboard horizontal scrolling');
        await region.evaluate(el => {el.scrollLeft = 0;});
        await region.evaluate(el => el.blur());
        await page.screenshot({path: '/tmp/weighbridge-business-laptop.png'});
      }
    }
  }
  assert.deepEqual(errors, []);
  console.log('PASS: seven business lists at 1366×768 / 1280×720 / 1024×768 / 390px; all columns, fixed edges, sticky headers, unclipped long text, visible pagination, keyboard scrolling');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
