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
  localSiteCode: 'F001', localScaleCode: 'WB01', configured: true, initialized: true,
  masterSyncEnabled: true, userId: 1, permissionCodes: ['weighing:void', 'weighing:reopen', 'weighing:adjust', 'weighing:correct', 'weighing:read', 'weighing:confirm', 'weighing:manual-return', 'weighing:terminate', 'weighing:export', 'weighing:print', 'weighing:master:read', 'weighing:master:write', 'weighing:site:manage', 'weighing:site:export', 'weighing:sync:tickets'],
  sites: [{code: 'F001', name: '一厂区', enabled: 1, canOperate: true}],
  scales: [{code: 'WB01', siteCode: 'F001', name: '一号地磅', siteName: '一厂区', enabled: 1, canOperate: true}],
};
const rows = Array.from({length: 12}, (_, index) => ({
  id: index + 1, siteCode: 'F001', siteName: '一厂区', scaleCode: 'WB01', version: 1, taskId: index + 1,
  taskNo: `RW-F001-20261022-${String(index + 1).padStart(4, '0')}`,
  ticketNo: `F001-20261022-${String(index + 1).padStart(4, '0')}`,
  business: {materialId: 1, partyId: 1, carrierId: null, driver: '', remark: '', plate: '川A12345', direction: index === 1 ? 'outbound' : 'inbound', materialName: '酿酒高粱', materialSpec: '优级', materialCode: 'GL001', partyName: '四川省粮食供应有限公司'},
  firstWeight: '35.125', secondWeight: '12.5', originalGross: '35.125', originalTare: '12.5', originalNet: '22.625',
  finalGross: '35.125', finalTare: '12.5', finalNet: '22.625', currentTicketId: index + 1,
  createTime: '2026-10-22T00:12:34Z', secondConfirmedAt: '2026-10-22T00:12:34Z', issuedAt: '2026-10-22T00:12:34Z', status: 'awaiting_second',
}));
const firstAt = '2026-10-22T00:12:34Z';
const secondAt = '2026-10-22T00:45:12Z';
const confirmedAt = '2026-10-22T01:05:19Z';
const makeBasis = row => [
  {id: row.id * 10, stage: 'first', kind: row.id === 2 ? 'tare' : 'gross', source: row.id === 3 ? 'manual' : 'collected', weight: row.id === 2 ? row.secondWeight : row.firstWeight, capturedAt: firstAt, confirmedAt: firstAt, evidence: {}, actorName: '核对员'},
  {id: row.id * 10 + 1, stage: 'second', kind: row.id === 2 ? 'gross' : 'tare', source: row.id < 2 ? 'collected' : 'manual', weight: row.id === 2 ? row.firstWeight : row.secondWeight, capturedAt: secondAt, confirmedAt, evidence: {}, actorName: '核对员', reason: '现场记录补录'},
];
const tickets = rows.map(row => ({
  ...row, status: row.id === 2 ? 'void' : 'valid', entryType: row.id === 3 ? 'manual_supplement' : undefined,
  basis: makeBasis(row), issuerName: '核对员', printStatus: row.id === 1 ? 'printed' : 'unprinted', syncStatus: 'pending',
  printSheet: {title: '酒厂过磅单', template: 'weighing-compact-v2', widthMm: 241, heightMm: 93, layers: 5, marks: [], rows: [[{label: '单号', value: row.ticketNo}]]},
}));
const originals = tickets.map(row => ({
  ...row, firstSource: row.basis[0].source, secondSource: row.basis[1].source, secondWeighedAt: secondAt, secondConfirmedAt: confirmedAt,
  currentTicketId: row.status === 'valid' ? row.id : null, relatedTicketId: row.id, relatedTicketNo: row.ticketNo,
}));
const tasks = rows.map(row => ({
  ...row,
  records: row.id <= 2 ? [makeBasis(row)[0]] : makeBasis(row),
}));
const mutations = [];
const materials = rows.map(row => ({id: row.id, version: 1, enabled: 1, name: '酿酒高粱', code: `GL${row.id}`, categoryId: 1, spec: '一级原粮', remark: '按约定规格验收入库，保留完整备注。'}));
const parties = rows.map(row => ({id: row.id, version: 1, enabled: 1, name: '四川省粮食供应有限公司', code: `PARTY${row.id}`, shortName: '粮食供应', nature: 'enterprise', roles: ['supplier', 'carrier'], phone: '13800138000', contacts: [{id: 1, name: '张师傅', isDefault: 1}], address: '四川省宜宾市翠屏区产业园区粮食仓储中心一号仓库', remark: 'LONG_REFERENCE_WITHOUT_SPACES_'.repeat(3)}));
const pageResult = records => ({records, total: records.length, page: 1, pageSize: 10});
const pages = [
  ['tasks', '运输任务列表', 9], ['originals', '原始称重列表', 8], ['tickets', '最终单据列表', 7],
  ['materials', '物料列表', 7], ['parties', '往来主体列表', 7],
  ['categories', '物料分类列表', 3],
];
const taskQueries = [];

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
    if (path === '/api/weighing/tasks' || path === '/api/weighing/tasks/export') taskQueries.push(new URL(route.request().url()));
    if (!path.startsWith('/api/')) return route.continue();
    if (route.request().method() === 'POST' && /^\/api\/weighing\/tickets\/\d+\/(void|reopen|print-attempts)$/.test(path)) {
      mutations.push({path, body: route.request().postDataJSON()});
    }
    let data = pageResult([]);
    if (path.endsWith('/capabilities')) data = capabilities;
    else if (path === '/api/auth/me') data = user;
    else if (path === '/api/auth/menus') data = [{id: 1, menuName: '地磅业务', menuType: 'DIR', path: '/weighing', visible: 1, sortOrder: -40, children: [
      {id: 2, menuName: '本厂称重', menuType: 'MENU', path: '/weighing/workbench', visible: 1, sortOrder: 0},
      {id: 3, menuName: '运输任务', menuType: 'MENU', path: '/weighing/tasks', visible: 1, sortOrder: 1},
    ]}];
    else if (path.endsWith('/unread-count')) data = 0;
    else if (path.endsWith('/summary')) data = {taskCount: 12, gross: '421.5', tare: '150', net: '271.5', rows: []};
    else if (path.endsWith('/material-categories')) data = pageResult(rows.map(row => ({id: row.id, enabled: 1, name: `原粮分类${row.id}`})));
    else if (path.endsWith('/materials')) data = pageResult(materials);
    else if (path.endsWith('/parties')) data = pageResult(parties);
    else if (path.endsWith('/plate-candidates')) data = {items: [], nextCursor: '', serverTime: new Date().toISOString()};
    else if (path.endsWith('/cameras')) data = {cameras: []};
    else if (path === '/api/weighing/tasks/1/termination') {
      const body = route.request().postDataJSON();
      mutations.push({path, body});
      tasks[0] = {...tasks[0], status: 'terminated', version: 2, events: [{action: 'task.terminate', reason: body.reason, actorName: '管理员', occurredAt: confirmedAt}]};
      data = {taskId: 1, taskVersion: 2, status: 'terminated'};
    }
    else if (/^\/api\/weighing\/tasks\/\d+$/.test(path)) {
      const id = Number(path.split('/').at(-1));
      data = id === 2 && tickets[1].status === 'void'
        ? {...tasks[1], status: 'completed', records: tickets[1].basis}
        : tasks[id - 1];
    }
    else if (/^\/api\/weighing\/tickets\/\d+(\/preview)?$/.test(path)) data = tickets[Number(path.split('/')[4]) - 1];
    else if (path.endsWith('/tasks')) data = pageResult(tasks.slice(0, 10));
    else if (path.endsWith('/originals')) data = pageResult(originals.slice(0, 10));
    else if (path.endsWith('/tickets')) {
      const taskId = new URL(route.request().url()).searchParams.get('taskId');
      data = taskId ? pageResult(tickets.filter(ticket => String(ticket.taskId) === taskId)) : pageResult(tickets.slice(0, 10));
    }
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
      assert.equal(await page.getByLabel('筛选厂区', {exact: true}).count(), 0, 'local lists have no cross-site filter');
      const region = page.getByRole('region', {name: label, exact: true});
      await region.locator('tbody tr').first().waitFor();
      const workbench = page.locator('header').getByRole('link', {name: '称重工作台', exact: true});
      assert.equal(await workbench.getAttribute('href'), '/weighing/workbench?scale=WB01');
      const bellBox = await page.getByRole('button', {name: '站内通知', exact: true}).boundingBox();
      const workbenchBox = await workbench.boundingBox();
      assert.ok(workbenchBox.x >= bellBox.x + bellBox.width, 'workbench entry is right of the notification bell');
      if (path === 'tasks') {
        await page.getByRole('heading', {name: '待回磅任务', exact: true}).waitFor();
        assert.equal(await page.getByLabel('筛选状态', {exact: true}).inputValue(), 'second');
        assert.equal(taskQueries.at(-1).searchParams.get('status'), 'awaiting_second');
        if (viewport.width === 1366) {
          const nav = page.getByRole('navigation', {name: '主导航', exact: true});
          assert.equal(await nav.getByRole('button', {name: '称重作业', exact: true}).count(), 0);
          assert.equal(await nav.getByRole('button', {name: '地磅业务', exact: true}).count(), 0, 'legacy backend menus do not restore retired navigation');
          assert.equal(await nav.getByRole('link', {name: '待回磅任务', exact: true}).count(), 1);
          assert.equal(await nav.getByRole('link', {name: '运输任务', exact: true}).count(), 0);
          assert.equal(await nav.getByRole('button', {name: '单据查询', exact: true}).getAttribute('aria-expanded'), 'true');
          await Promise.all([
            page.waitForResponse(response => response.url().includes('/api/weighing/tasks?') && new URL(response.url()).searchParams.get('status') === 'terminated'),
            page.getByLabel('筛选状态', {exact: true}).selectOption('terminated'),
          ]);
          await Promise.all([
            page.waitForResponse(response => response.url().includes('/api/weighing/tasks?') && new URL(response.url()).searchParams.get('status') === 'awaiting_second'),
            page.getByRole('button', {name: '清空筛选', exact: true}).click(),
          ]);
          const popupPromise = page.waitForEvent('popup');
          await workbench.click();
          const popup = await popupPromise;
          await popup.waitForURL('**/weighing/workbench?scale=WB01');
          await popup.getByRole('heading', {name: '称重工作台', exact: true}).waitFor();
          assert.equal(await popup.evaluate(() => window.name), 'weighbridge-workbench-WB01');
          await page.bringToFront();
          await workbench.click();
          assert.equal(context.pages().length, 2, 'reopening reuses the scale workbench window');
          await popup.close();
        }
      }
      if (viewport.width === 1280 && ['tickets', 'originals'].includes(path)) {
        for (const source of ['设备称重', '人工补录回磅', '人工补单']) {
          await region.getByText(source, {exact: true}).first().waitFor();
        }
        if (path === 'originals') {
          const voidReference = region.getByRole('link', {name: tickets[1].ticketNo, exact: true});
          assert.equal(await voidReference.getAttribute('href'), '/weighing/tickets/2');
          assert.match(await voidReference.locator('..').textContent(), /作废/);
        } else {
          assert.ok(await region.getByText('待同步').count() > 0);
          const first = region.locator('tbody tr').first();
          assert.match(await first.textContent(), /川A12345.*F001-20261022-0001.*22\.625.*有效.*已打印/s);
          const edges = await region.evaluate(el => ({
            scrollWidth: el.scrollWidth, clientWidth: el.clientWidth, left: el.scrollLeft,
            bottom: el.querySelector('tbody tr').getBoundingClientRect().bottom,
          }));
          assert.ok(edges.scrollWidth <= edges.clientWidth + 1 && edges.left === 0 && edges.bottom <= 720, JSON.stringify(edges));
          await page.screenshot({path: '/tmp/weighbridge-business-tickets-1280.png'});
        }
      }
      await region.locator('thead th').nth(count - 1).waitFor();
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
        const row = region.locator('tbody tr').first();
        const more = row.getByRole('button', {name: `${rows[0].taskNo} 更多操作`});
        const manual = row.getByRole('button', {name: '人工补录回磅', exact: true});
        const terminate = row.getByRole('button', {name: '终止任务', exact: true});
        const weigh = row.getByRole('link', {name: '办理称重', exact: true});
        assert.equal(await weigh.isVisible(), true);
        assert.equal(await weigh.evaluate(el => getComputedStyle(el).color), 'rgb(255, 255, 255)', 'small primary button preserves its text color');
        assert.equal(await manual.count(), 0);
        await more.focus();
        await page.keyboard.press('Enter');
        await manual.waitFor();
        await page.waitForFunction(() => document.activeElement?.textContent?.trim() === '人工补录回磅');
        await page.keyboard.press('Escape');
        await manual.waitFor({state: 'hidden'});
        assert.equal(await more.evaluate(el => document.activeElement === el), true, 'Escape returns focus to the row trigger');
        await more.click();
        await manual.click();
        const manualDialog = page.getByRole('dialog', {name: '人工补录回磅', exact: true});
        const time = manualDialog.getByLabel('实际回磅时间（北京时间）');
        assert.equal(await time.inputValue(), '');
        const confirm = manualDialog.getByRole('button', {name: '确认补录并出单'});
        assert.equal(await confirm.isDisabled(), true);
        await manualDialog.getByLabel('补录皮重（吨）').fill('10');
        await time.fill('2026-10-22T07:59');
        await manualDialog.getByText('实际回磅时间不能早于有效首磅时间').waitFor();
        assert.equal(await confirm.isDisabled(), true);
        await time.fill('2099-01-01T08:00');
        await manualDialog.getByText('实际回磅时间不能晚于当前时间').waitFor();
        assert.equal(await confirm.isDisabled(), true);
        await time.fill('2026-10-22T08:30');
        await manualDialog.getByLabel('补录原因').fill('核对现场记录');
        await manualDialog.getByRole('checkbox').check();
        await manualDialog.getByText('预计出单净重').waitFor();
        assert.match(await manualDialog.getByText('预计出单净重').locator('..').textContent(), /25\.125/);
        assert.equal(await confirm.isDisabled(), false);
        await manualDialog.getByLabel('第 1 项扣减原因').selectOption('WATER');
        await manualDialog.getByLabel('扣减重量（吨）').fill('1.125');
        assert.match(await manualDialog.getByText('预计出单净重').locator('..').textContent(), /24 吨/);
        await manualDialog.getByRole('button', {name: '取消', exact: true}).click();
        await page.getByRole('button', {name: '放弃并关闭', exact: true}).click();
        assert.equal(await more.evaluate(el => document.activeElement === el), true, 'closing the dialog restores the row trigger');
        await more.click();
        await terminate.click();
        const confirmation = page.getByRole('dialog', {name: '终止运输任务', exact: true});
        assert.equal(await confirmation.getByRole('button', {name: '确认终止', exact: true}).isDisabled(), true);
        await confirmation.getByRole('button', {name: '取消', exact: true}).click();
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
  await page.setViewportSize({width: 1280, height: 720});
  await page.goto(`${base}/weighing/tickets`);
  await page.getByRole('link', {name: '预览', exact: true}).first().click();
  const preview = page.getByRole('region', {name: '纸质过磅单预览'});
  await preview.getByText(tickets[0].ticketNo, {exact: true}).waitFor();
  await page.getByRole('button', {name: '纸张规格'}).focus();
  await page.getByRole('tooltip').filter({hasText: /空白五联针打纸/}).waitFor();
  await page.getByRole('link', {name: '返回单据详情', exact: true}).click();
  await page.getByRole('button', {name: '作废单据', exact: true}).click();
  const voidDialog = page.getByRole('dialog', {name: '作废单据'});
  for (const text of [tickets[0].ticketNo, '川A12345', '22.625 吨']) {
    await voidDialog.getByText(text, {exact: true}).waitFor();
  }
  assert.equal(await voidDialog.getByRole('button', {name: '确认作废'}).isDisabled(), true);
  await voidDialog.getByRole('button', {name: '取消', exact: true}).click();

  await page.goto(`${base}/weighing/tickets/2`);
  await page.getByRole('button', {name: '关联更正重开', exact: true}).click();
  const reopen = page.getByRole('dialog', {name: '关联更正重开'});
  assert.equal(await reopen.getByRole('button', {name: '核对更正'}).isDisabled(), true);
  await reopen.getByText('更正毛重或皮重', {exact: true}).click();
  await reopen.getByLabel('更正后毛重（吨）').fill('36');
  await reopen.getByLabel('更正原因').fill('核对毛重');
  await reopen.getByRole('button', {name: '核对更正'}).click();
  const review = page.getByRole('dialog', {name: '核对并确认更正重开'});
  assert.deepEqual(await review.getByRole('row', {name: /最终净重/}).getByRole('cell').allTextContents(), ['22.625 吨', '23.5 吨']);
  await review.getByRole('button', {name: '返回修改'}).click();
  assert.equal(mutations.length, 0, 'preview, cancelled void and cancelled reopen review do not submit');

  await page.goto(`${base}/weighing/tasks`);
  await page.getByRole('button', {name: `${rows[0].taskNo} 更多操作`}).click();
  await page.getByRole('button', {name: '终止任务', exact: true}).click();
  const terminate = page.getByRole('dialog', {name: '终止运输任务'});
  await terminate.getByLabel('终止原因').fill('车辆已离场');
  await terminate.getByRole('checkbox').check();
  await terminate.getByRole('button', {name: '确认终止'}).click();
  await page.waitForURL('**/weighing/tasks/1');
  await page.getByText('已终止', {exact: true}).last().waitFor();
  await page.getByText('车辆已离场', {exact: true}).waitFor();
  assert.equal(mutations[0].body.reason, '车辆已离场');

  await page.goto(`${base}/weighing/categories`);
  capabilities.scales.push({...capabilities.scales[0], id: 2});
  await page.reload();
  await page.locator('header').getByRole('button', {name: '称重工作台', exact: true}).waitFor();
  assert.equal(await page.locator('header').getByRole('button', {name: '称重工作台', exact: true}).isDisabled(), true, 'ambiguous scale does not open a guessed target');
  capabilities.permissionCodes = ['weighing:read'];
  await page.goto(`${base}/weighing/tasks`);
  const readonlyTasks = page.getByRole('region', {name: '运输任务列表', exact: true});
  await readonlyTasks.locator('tbody tr').first().waitFor();
  assert.equal(await readonlyTasks.getByRole('button', {name: /更多操作/}).count(), 0);
  assert.equal(await readonlyTasks.getByRole('link', {name: '办理称重', exact: true}).count(), 0);
  assert.equal(await page.locator('header').getByRole('link', {name: '称重工作台', exact: true}).count(), 0, 'query-only account has no workbench shortcut');
  assert.equal(await page.locator('header').getByRole('button', {name: '称重工作台', exact: true}).count(), 0);
  assert.deepEqual(errors, []);
  console.log('PASS: six business lists at 1366×768 / 1280×720 / 1024×768 / 390px; fixed edges, sticky headers, 1280px ticket essentials, three sources, return-time validation, void / reopen review and termination traceability');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
