import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4197';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4197', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 80_000);
try {
  for (let i = 0; i < 60; i++) { try { if ((await fetch(`${base}/tests/plate-candidates.html`)).ok) break; } catch {} await delay(100); }
  browser = await chromium.launch({ headless: true });
  const newPage = async options => {
    const page = await browser.newPage(options);
    await page.route('**/api/system/dict/TRANSPORT_DIRECTION/items', route => route.fulfill({ json: { code: 200, data: [{ value: 'inbound', label: '入厂卸货（收粮）', sortOrder: 1 }, { value: 'outbound', label: '出厂装货（拉酒糟）', sortOrder: 2 }] } }));
    return page;
  };
  for (const width of [1366, 390]) {
    const page = await newPage({ viewport: { width, height: 768 } });
    page.setDefaultTimeout(7000);
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    let events = [];
    let lookupCount = 0;
    let confirmed;
    let sampleInput;
    await page.route('**/api/weighing/**', async route => {
      const url = new URL(route.request().url());
      const ok = data => route.fulfill({ json: { code: 200, data } });
      if (url.pathname.endsWith('/plate-candidates')) {
        const after = url.searchParams.get('after');
        return ok({ items: after ? events.filter(e => e.id > Number(after)) : [], nextCursor: String(events.length), serverTime: new Date().toISOString() });
      }
      if (url.pathname.endsWith('/tasks/active')) {
        if (url.searchParams.has('scale')) lookupCount++;
        return ok(null);
      }
      if (url.pathname.endsWith('/trip-candidates')) return ok({ enabled: false, available: true, candidates: [] });
      if (url.pathname.includes('/session/')) return ok({ scaleCode: 'WB01', holderUserId: 1, clientId: 'test', generation: 1, sessionToken: 'test-token', expiresAt: new Date(Date.now() + 60_000).toISOString() });
      if (url.pathname.endsWith('/plate-confirmations')) { confirmed = route.request().postDataJSON(); return ok({ confirmationId: 77 }); }
      if (url.pathname.endsWith('/samples')) {
        sampleInput = route.request().postDataJSON();
        return ok({ id: 5, taskId: null, stage: 'first', weight: '18.5', plateEvidence: { ...sampleInput.plateEvidence, sourceEvents: [events[0].sourceEvents[0]] }, evidence: { source: 'server', images: [], captureRequired: false, imageStatus: 'disabled', at: new Date().toISOString(), weight: '18.5', plate: '晋A12346' } });
      }
      if (url.pathname.endsWith('/cameras')) return ok({ cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } });
      return ok({ records: [], total: 0 });
    });
    await page.goto(`${base}/tests/plate-candidates.html?scale=WB01`);
    await delay(500);
    const item = (id, plate, cameraName) => { const now = new Date(); return { id, plate, firstReceivedAt: now.toISOString(), lastReceivedAt: now.toISOString(), expiresAt: new Date(now.getTime() + 60_000).toISOString(), sourceEvents: [{ eventRecordId: id, cameraId: id, cameraName, receivedAt: now.toISOString(), recognizedAt: now.toISOString() }] }; };
    events = [item(1, '晋A12345', '相机一')];
    const input = page.getByLabel('车牌号码');
    await page.waitForFunction(() => document.querySelector('#workbench-plate')?.value === 'A12345', null, { timeout: 6000 });
    assert.equal(lookupCount, 0, '自动预填不能查找');
    events = [...events, item(2, '晋A54321', '相机二')];
    await page.getByText('存在多个候选，请核对后选择').waitFor({ timeout: 6000 });
    assert.equal(await input.inputValue(), 'A12345', '冲突不能覆盖');
    await input.fill('A12346');
    events = [...events, item(3, '晋A99999', '相机三')];
    await page.getByRole('button', { name: /晋A99999/ }).waitFor({ timeout: 6000 });
    assert.equal(await input.inputValue(), 'A12346', '人工输入不能被覆盖');
    assert.equal(lookupCount, 0, '预查不能代替确认车辆');
    assert.equal(confirmed, undefined);
    const confirmVehicle = page.getByRole('button', { name: '确认车辆' });
    assert.match(await confirmVehicle.getAttribute('class'), /\bbg-primary-hover\b/);
    await confirmVehicle.click();
    await page.getByRole('tab', { name: '基本信息' }).waitFor();
    assert.match(await confirmVehicle.getAttribute('class'), /\bbg-surface\b/);
    const inbound = page.getByRole('radio', { name: '入厂卸货（收粮）' });
    const outbound = page.getByRole('radio', { name: '出厂装货（拉酒糟）' });
    assert.equal(await inbound.isChecked(), true);
    assert.equal(await inbound.inputValue(), 'inbound', '字典只改变名称，不改变业务编码');
    assert.equal(await outbound.inputValue(), 'outbound');
    await outbound.check();
    assert.equal(await outbound.isChecked(), true);
    await inbound.check();
    assert.equal(lookupCount, 1);
    assert.equal(confirmed.confirmedPlate, '晋A12346');
    assert.equal(confirmed.evidence.candidate, '晋A12345');
    assert.equal(confirmed.evidence.corrected, true);
    assert.deepEqual(confirmed.evidence.sourceEventRecordIds, [1]);
    await page.getByRole('tab', { name: '补充信息（选填）' }).click();
    await page.getByLabel('司机姓名（选填）').fill('测试司机');
    await page.getByRole('tab', { name: '基本信息' }).click();
    assert.equal(await page.getByRole('combobox', { name: '搜索物料' }).isVisible(), true);
    await page.getByRole('tab', { name: '补充信息（选填）' }).click();
    assert.equal(await page.getByLabel('司机姓名（选填）').inputValue(), '测试司机', '切换页签保留补充资料');
    await page.getByRole('tab', { name: '基本信息' }).click();
    await page.getByRole('button', { name: '采集重量', exact: true }).click();
    await page.getByRole('dialog', { name: '核对首磅（毛重）', exact: true }).getByText('还缺物料、供货方', { exact: true }).waitFor();
    assert.equal(sampleInput.plateEvidence.confirmationId, 77);
    events = [...events, item(4, '晋A88888', '相机四')];
    await page.getByRole('button', { name: /晋A88888/ }).waitFor({ timeout: 6000 });
    assert.equal(await input.inputValue(), 'A12346', '采样后不得覆盖车牌');
    assert.deepEqual(errors, []);
    await page.keyboard.press('Escape');
    await page.getByRole('button', { name: '清除车牌' }).click();
    assert.equal(await input.inputValue(), '', '清除车牌后输入框为空');
    assert.equal(await page.getByRole('button', { name: '选择车牌省份' }).innerText(), '省份');
    assert.equal(await page.getByRole('button', { name: '采集重量', exact: true }).isDisabled(), true, '清除车牌后须重新确认车辆');
    await page.close();
  }
  const page = await newPage({ viewport: { width: 1366, height: 768 } });
  page.setDefaultTimeout(7000);
  const makeItem = (id, plate) => { const now = new Date(); return { id, plate, firstReceivedAt: now.toISOString(), lastReceivedAt: now.toISOString(), expiresAt: new Date(now.getTime() + 60_000).toISOString(), sourceEvents: [{ eventRecordId: id, cameraId: id, cameraName: '相机', receivedAt: now.toISOString(), recognizedAt: now.toISOString() }] }; };
  let events = [makeItem(1, '晋A12345')];
  await page.route('**/api/weighing/**', async route => {
    const url = new URL(route.request().url());
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (url.pathname.endsWith('/plate-candidates')) {
      const after = url.searchParams.get('after');
      return ok({ items: after ? events.filter(item => item.id > Number(after)) : events, nextCursor: String(events.length), serverTime: new Date().toISOString() });
    }
    if (url.pathname.includes('/session/')) return ok({ scaleCode: 'WB01', holderUserId: 1, clientId: 'test', generation: 1, sessionToken: 'test-token', expiresAt: new Date(Date.now() + 60_000).toISOString() });
    if (url.pathname.endsWith('/cameras')) return ok({ cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } });
    return ok({ records: [], total: 0 });
  });
  await page.goto(`${base}/tests/plate-candidates.html?scale=WB01&scenario=completed`);
  await page.getByRole('button', { name: '办理下一辆车' }).click();
  await delay(2500);
  assert.equal(await page.getByLabel('车牌号码').inputValue(), '', '上一辆的事件不能重新预填');
  events = [...events, makeItem(2, '晋A54321')];
  await page.waitForFunction(() => document.querySelector('#workbench-plate')?.value === 'A54321', null, { timeout: 6000 });
  await page.close();
  const reconnect = await newPage({ viewport: { width: 1366, height: 768 } });
  reconnect.setDefaultTimeout(7000);
  let online = true;
  let polls = 0;
  events = [];
  await reconnect.route('**/api/weighing/**', async route => {
    const url = new URL(route.request().url());
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (url.pathname.endsWith('/plate-candidates')) {
      polls++;
      if (!online) return route.abort();
      const after = url.searchParams.get('after');
      return ok({ items: after ? events.filter(item => item.id > Number(after)) : events, nextCursor: String(events.length), serverTime: new Date().toISOString() });
    }
    if (url.pathname.endsWith('/cameras')) return ok({ cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } });
    return ok({ records: [], total: 0 });
  });
  await reconnect.goto(`${base}/tests/plate-candidates.html?scale=WB01`);
  await reconnect.getByLabel('车牌号码').focus();
  await delay(500);
  events = [makeItem(1, '晋A12345')];
  await reconnect.getByRole('button', { name: /晋A12345/ }).waitFor();
  assert.equal(await reconnect.getByLabel('车牌号码').inputValue(), '', '输入焦点期间不得自动预填');
  await reconnect.getByLabel('车牌号码').fill('A88888');
  await reconnect.getByRole('button', { name: '选择车牌省份' }).click();
  await reconnect.getByRole('button', { name: '晋', exact: true }).click();
  online = false;
  const beforeFailure = polls;
  while (polls === beforeFailure) await delay(100);
  events = [...events, makeItem(2, '晋A54321')];
  online = true;
  await reconnect.getByRole('button', { name: /晋A54321/ }).waitFor();
  assert.equal(await reconnect.getByLabel('车牌号码').inputValue(), 'A88888', '重连不得覆盖人工输入');
  await reconnect.close();
  const timezone = await newPage();
  const expired = makeItem(1, '晋A11111');
  expired.expiresAt = '2026-01-01T08:00:00+08:00';
  await timezone.route('**/api/weighing/**', async route => {
    const path = new URL(route.request().url()).pathname;
    const data = path.endsWith('/plate-candidates') ? { items: [expired], nextCursor: '1', serverTime: '2026-01-01T00:00:01Z' } : path.endsWith('/cameras') ? { cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } } : { records: [], total: 0 };
    return route.fulfill({ json: { code: 200, data } });
  });
  await timezone.goto(`${base}/tests/plate-candidates.html?scale=WB01`);
  await delay(2500);
  assert.equal(await timezone.getByRole('button', { name: /晋A11111/ }).count(), 0, '不同时间偏移的过期候选不能显示');
  await timezone.close();
  const fallback = await newPage();
  fallback.setDefaultTimeout(7000);
  let fallbackEvent = [];
  await fallback.route('**/api/weighing/**', async route => {
    const url = new URL(route.request().url());
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (url.pathname.endsWith('/plate-candidates')) return ok({ items: url.searchParams.get('after') ? fallbackEvent : [], nextCursor: String(fallbackEvent.length), serverTime: new Date().toISOString() });
    if (url.pathname.includes('/session/')) return ok({ scaleCode: 'WB01', holderUserId: 1, clientId: 'test', generation: 1, sessionToken: 'test-token', expiresAt: new Date(Date.now() + 60_000).toISOString() });
    if (url.pathname.endsWith('/plate-confirmations')) return route.fulfill({ json: { code: 400, message: '识别来源无效' } });
    if (url.pathname.endsWith('/tasks/active')) return ok(null);
    if (url.pathname.endsWith('/cameras')) return ok({ cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } });
    return ok({ records: [], total: 0 });
  });
  await fallback.goto(`${base}/tests/plate-candidates.html?scale=WB01`);
  await delay(500);
  fallbackEvent = [makeItem(1, '晋A12345')];
  await fallback.waitForFunction(() => document.querySelector('#workbench-plate')?.value === 'A12345');
  await fallback.getByRole('button', { name: '确认车辆' }).click();
  await fallback.getByText('识别来源不可用，已按人工核对车牌办理。').waitFor();
  assert.equal(await fallback.getByRole('button', { name: '采集重量', exact: true }).isEnabled(), true, '来源失效后人工办理保持可用');
  await fallback.close();
  const agentPage = await newPage();
  await agentPage.route('**/api/weighing/**', async route => {
    const path = new URL(route.request().url()).pathname;
    const data = path.endsWith('/plate-candidates') ? { items: [], nextCursor: '0', serverTime: new Date().toISOString() } : path.endsWith('/cameras') ? { cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } } : { records: [], total: 0 };
    return route.fulfill({ json: { code: 200, data } });
  });
  await agentPage.goto(`${base}/tests/plate-candidates.html?scale=WB01&scenario=agent`);
  await agentPage.getByText(/识别候选 晋A11111/).waitFor();
  assert.equal(await agentPage.getByLabel('车牌号码').inputValue(), '', '进入页面时旧 Agent 识别不能自动填入');
  await agentPage.evaluate(() => window.dispatchEvent(new Event('fixture-agent')));
  await agentPage.waitForFunction(() => document.querySelector('#workbench-plate')?.value === 'A22222');
  await agentPage.close();
  const freshAgent = await newPage();
  await freshAgent.route('**/api/weighing/**', async route => {
    const path = new URL(route.request().url()).pathname;
    const data = path.endsWith('/plate-candidates') ? { items: [], nextCursor: '0', serverTime: new Date().toISOString() } : path.endsWith('/cameras') ? { cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } } : { records: [], total: 0 };
    return route.fulfill({ json: { code: 200, data } });
  });
  await freshAgent.goto(`${base}/tests/plate-candidates.html?scale=WB01`);
  await freshAgent.evaluate(() => window.dispatchEvent(new Event('fixture-agent')));
  await freshAgent.waitForFunction(() => document.querySelector('#workbench-plate')?.value === 'A22222');
  await freshAgent.close();
  const savedDraft = await newPage();
  let draftEvents = [];
  await savedDraft.route('**/api/weighing/**', async route => {
    const url = new URL(route.request().url());
    const data = url.pathname.endsWith('/plate-candidates') ? { items: url.searchParams.get('after') ? draftEvents : [], nextCursor: String(draftEvents.length), serverTime: new Date().toISOString() } : url.pathname.endsWith('/cameras') ? { cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } } : { records: [], total: 0 };
    return route.fulfill({ json: { code: 200, data } });
  });
  await savedDraft.goto(`${base}/tests/plate-candidates.html?scale=WB01&scenario=draft`);
  await delay(500);
  draftEvents = [makeItem(1, '晋A11111')];
  await savedDraft.getByRole('button', { name: /晋A11111/ }).waitFor();
  assert.equal(await savedDraft.getByLabel('车牌号码').inputValue(), '', '已有业务草稿不得自动套入候选');
  await savedDraft.close();
  for (const width of [1920, 1366]) {
    const compact = await newPage({ viewport: { width, height: 768 } });
    compact.setDefaultTimeout(7000);
    let compactEvents = [];
    await compact.route('**/api/weighing/**', async route => {
      const url = new URL(route.request().url());
      const ok = data => route.fulfill({ json: { code: 200, data } });
      if (url.pathname.endsWith('/plate-candidates')) {
        const after = url.searchParams.get('after');
        return ok({ items: after ? compactEvents.filter(item => item.id > Number(after)) : [], nextCursor: String(compactEvents.length), serverTime: new Date().toISOString() });
      }
      if (url.pathname.endsWith('/cameras')) return ok({ cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } });
      return ok({ records: [], total: 0 });
    });
    await compact.goto(`${base}/tests/plate-candidates.html?scale=WB01`);
    await delay(500);
    const instrument = compact.locator('.weighing-scale');
    const instrumentTop = await instrument.evaluate(node => node.getBoundingClientRect().top);
    const sameMoment = new Date();
    const candidate = (id, plate, cameras) => ({ id, plate, firstReceivedAt: sameMoment.toISOString(), lastReceivedAt: sameMoment.toISOString(), expiresAt: new Date(sameMoment.getTime() + 60_000).toISOString(), sourceEvents: cameras.map((name, index) => ({ eventRecordId: id + index, cameraId: index + 1, cameraName: name, receivedAt: sameMoment.toISOString(), recognizedAt: sameMoment.toISOString() })) });
    compactEvents = [candidate(1, '晋AB6P44', ['抓拍01', '抓拍01', '抓拍01'])];
    const chip = compact.getByRole('button', { name: /晋AB6P44/ });
    await chip.waitFor();
    assert.equal((await chip.innerText()).match(/抓拍01/g).length, 1, '同一相机只显示一次');
    assert.equal(await compact.locator('form .plate-candidates').count(), 1, '候选紧跟车牌输入');
    const chipTop = await chip.evaluate(node => node.getBoundingClientRect().top);
    assert.ok(chipTop >= await instrument.evaluate(node => node.getBoundingClientRect().bottom), '候选位于仪表下方');
    assert.equal(await instrument.evaluate(node => node.getBoundingClientRect().top), instrumentTop, '候选出现时仪表位置保持不变');
    compactEvents = [...compactEvents, candidate(10, '晋AB6P44', ['抓拍01'])];
    await delay(2500);
    assert.equal(await compact.getByRole('button', { name: /晋AB6P44/ }).count(), 1, '同一车牌重复批次只保留一项');
    assert.equal((await compact.getByRole('button', { name: /晋AB6P44/ }).innerText()).match(/抓拍01/g).length, 1, '合并后相机仍只显示一次');
    await compact.close();
  }
  {
    const waiting = await newPage({ viewport: { width: 1366, height: 768 } });
    waiting.setDefaultTimeout(7000);
    const errors = [];
    waiting.on('pageerror', error => errors.push(error.message));
    const now = Date.now();
    const hoursAgo = h => new Date(now - h * 60 * 60 * 1000).toISOString();
    const makeTask = (id, plate, materialName, weight, at) => ({
      id,
      siteCode: 'F001',
      scaleCode: 'WB01',
      taskNo: `WB-20261127-${String(id).padStart(4, '0')}`,
      version: 1,
      status: 'awaiting_second',
      firstWeight: String(weight),
      createTime: at,
      business: { direction: 'inbound', plate, materialId: 1, materialCode: 'M01', materialName, partyId: 1, partyCode: 'P01', partyName: '测试供货方', carrierId: null },
      records: [{ id: id * 10, taskId: id, stage: 'first', kind: 'gross', weight: String(weight), capturedAt: at, confirmedAt: at, actorName: '操作员', replacesRecordId: null, retainedRecordId: null, reason: '', evidence: { source: 'server', images: [], captureRequired: false, imageStatus: 'disabled', at, weight: String(weight), plate }, plateEvidence: { source: 'manual', confirmedPlate: plate, at } }],
    });
    const remoteTasks = [
      makeTask(11, '晋A10001', '高粱', 28.5, hoursAgo(1)),
      makeTask(12, '晋A10002', '小麦', 26.2, hoursAgo(2)),
      makeTask(13, '晋A10003', '豌豆', 24.8, hoursAgo(3)),
      makeTask(14, '晋A10004', '大麦', 22.1, hoursAgo(4)),
      makeTask(15, '晋A10005', '稻壳', 19.6, hoursAgo(5)),
      makeTask(16, '晋A66666', '高粱（晚到回磅）', 31.4, hoursAgo(20)),
      makeTask(17, '晋A90001', '已过期任务', 25.0, hoursAgo(25)),
      makeTask(18, '晋A90002', '无时间任务', 20.0, ''),
    ];
    let waitingEvents = [];
    let tasksPageSize = null;
    let lookupCount = 0;
    let confirmed;
    await waiting.route('**/api/weighing/**', async route => {
      const url = new URL(route.request().url());
      const ok = data => route.fulfill({ json: { code: 200, data } });
      if (url.pathname.endsWith('/tasks') && url.searchParams.get('status') === 'awaiting_second') {
        tasksPageSize = url.searchParams.get('pageSize');
        return ok({ records: remoteTasks, total: remoteTasks.length });
      }
      if (url.pathname.endsWith('/plate-candidates')) {
        const after = url.searchParams.get('after');
        return ok({ items: after ? waitingEvents.filter(e => e.id > Number(after)) : [], nextCursor: String(waitingEvents.length), serverTime: new Date().toISOString() });
      }
      if (url.pathname.endsWith('/tasks/active')) {
        if (url.searchParams.has('scale')) lookupCount++;
        return ok(url.searchParams.get('plate') === '晋A66666' ? remoteTasks[5] : null);
      }
      if (url.pathname.endsWith('/trip-candidates')) return ok({ enabled: false, available: true, candidates: [] });
      if (url.pathname.includes('/trip-basis')) return ok({ bound: false });
      if (url.pathname.includes('/trip-cloud-status')) return ok({ bound: false, available: true, unloaded: false });
      if (url.pathname.includes('/session/')) return ok({ scaleCode: 'WB01', holderUserId: 1, clientId: 'test', generation: 1, sessionToken: 'test-token', expiresAt: new Date(Date.now() + 60_000).toISOString() });
      if (url.pathname.endsWith('/plate-confirmations')) { confirmed = route.request().postDataJSON(); return ok({ confirmationId: 88 }); }
      if (url.pathname.endsWith('/cameras')) return ok({ cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } });
      return ok({ records: [], total: 0 });
    });
    await waiting.goto(`${base}/tests/plate-candidates.html?scale=WB01`);
    const queue = waiting.locator('.workbench-waiting-queue');
    await queue.waitFor();
    assert.equal(tasksPageSize, '100', '工作台待回磅列表应拉取足够大的一页');
    assert.equal(await queue.locator('.workbench-waiting-window').innerText(), '24小时内');
    assert.equal(await queue.locator('.workbench-waiting-count').innerText(), '6 辆', '超过 24 小时及非法时间的待回磅任务不计入总数');
    assert.equal(await queue.getByText('晋A90001').count(), 0, '超过 24 小时的待回磅任务不展示');
    assert.equal(await queue.getByText('晋A90002').count(), 0, '无合法时间的待回磅任务不展示');
    assert.equal(await queue.locator('.workbench-waiting-item').count(), 5, '无命中时最多展示前 5 条');
    assert.equal(await queue.locator('.workbench-waiting-item[data-recognized="true"]').count(), 0);
    assert.equal(await queue.getByText('晋A66666').count(), 0, '未识别前第 6 条暂不在前 5 条内');
    await waiting.getByLabel('车牌号码').fill('A00001');
    const recognizedAt = new Date().toISOString();
    waitingEvents = [{ id: 101, plate: '晋A66666', firstReceivedAt: recognizedAt, lastReceivedAt: recognizedAt, expiresAt: new Date(Date.now() + 60_000).toISOString(), sourceEvents: [{ eventRecordId: 101, cameraId: 1, cameraName: '入口相机', receivedAt: recognizedAt, recognizedAt }] }];
    const highlighted = queue.locator('.workbench-waiting-item.is-recognized[data-recognized="true"]');
    await highlighted.waitFor({ timeout: 6000 });
    assert.equal(await highlighted.count(), 1);
    const firstRow = queue.locator('.workbench-waiting-item').first();
    assert.equal(await firstRow.getAttribute('data-recognized'), 'true', '相机识别命中的待回磅单子必须置顶到第一位');
    assert.match(await firstRow.innerText(), /晋A66666/);
    assert.match(await firstRow.innerText(), /相机已识别/);
    await firstRow.click();
    assert.equal(await waiting.getByLabel('车牌号码').inputValue(), 'A66666', '点击高亮待回磅行应填入对应车牌');
    assert.equal(await waiting.evaluate(() => document.activeElement?.id), 'workbench-plate', '点击待回磅行后应聚焦车牌输入框');
    assert.equal(lookupCount, 0, '点击高亮待回磅行不得绕过人工确认车辆');
    assert.equal(confirmed, undefined, '点击高亮待回磅行不得自动提交确认');
    await waiting.getByRole('button', { name: '确认车辆' }).click();
    await waiting.getByText('首次称重依据').waitFor();
    assert.equal(lookupCount, 1);
    assert.equal(confirmed.confirmedPlate, '晋A66666');
    assert.equal(confirmed.evidence.source, 'server', '点击高亮待回磅行后应保留摄像头识别来源');
    assert.equal(confirmed.evidence.candidate, '晋A66666');
    assert.equal(confirmed.evidence.corrected, false);
    assert.deepEqual(confirmed.evidence.sourceEventRecordIds, [101]);
    assert.deepEqual(errors, []);
    await waiting.close();
  }
  {
    const allExpired = await newPage({ viewport: { width: 1366, height: 768 } });
    allExpired.setDefaultTimeout(7000);
    const expiredAt = new Date(Date.now() - 26 * 60 * 60 * 1000).toISOString();
    await allExpired.route('**/api/weighing/**', async route => {
      const url = new URL(route.request().url());
      const ok = data => route.fulfill({ json: { code: 200, data } });
      if (url.pathname.endsWith('/tasks') && url.searchParams.get('status') === 'awaiting_second') {
        return ok({
          records: [{
            id: 99, siteCode: 'F001', scaleCode: 'WB01', taskNo: 'WB-OLD-0099', version: 1, status: 'awaiting_second', firstWeight: '20', createTime: expiredAt,
            business: { direction: 'inbound', plate: '晋A99900', materialId: 1, materialCode: 'M01', materialName: '旧单', partyId: 1, partyCode: 'P01', partyName: '测试', carrierId: null },
            records: [],
          }],
          total: 1,
        });
      }
      if (url.pathname.endsWith('/plate-candidates')) return ok({ items: [], nextCursor: '0', serverTime: new Date().toISOString() });
      if (url.pathname.endsWith('/cameras')) return ok({ cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } });
      return ok({ records: [], total: 0 });
    });
    await allExpired.goto(`${base}/tests/plate-candidates.html?scale=WB01`);
    await delay(500);
    assert.equal(await allExpired.locator('.workbench-waiting-queue').count(), 0, '24小时内无有效待回磅单子时不展示队列');
    assert.equal(await allExpired.locator('.workbench-idle-guide').count(), 1, '24小时内为 0 辆时展示 3 步作业指引卡');
    await allExpired.close();
  }
  console.log('车牌候选工作台：双尺寸预填、冲突、人工输入、采样保护、下一辆水位、焦点、重连及待回磅24小时高亮置顶通过。');
} finally { clearTimeout(deadline); await browser?.close(); vite.kill('SIGTERM'); }
