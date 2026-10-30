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
  for (const width of [1366, 390]) {
    const page = await browser.newPage({ viewport: { width, height: 768 } });
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
      if (url.pathname.endsWith('/tasks/active')) { lookupCount++; return ok(null); }
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
    assert.equal(lookupCount, 0);
    await page.getByRole('button', { name: '确认车辆' }).click();
    await page.getByText('已确认车辆，未找到已有运输任务，请现场选择本趟资料。').waitFor();
    assert.equal(lookupCount, 1);
    assert.equal(confirmed.confirmedPlate, '晋A12346');
    assert.equal(confirmed.evidence.candidate, '晋A12345');
    assert.equal(confirmed.evidence.corrected, true);
    assert.deepEqual(confirmed.evidence.sourceEventRecordIds, [1]);
    await page.getByRole('button', { name: '采集重量', exact: true }).click();
    await page.getByRole('dialog', { name: '核对首磅（毛重）', exact: true }).getByText('还缺物料、供货方', { exact: true }).waitFor();
    assert.equal(sampleInput.plateEvidence.confirmationId, 77);
    events = [...events, item(4, '晋A88888', '相机四')];
    await page.getByRole('button', { name: /晋A88888/ }).waitFor({ timeout: 6000 });
    assert.equal(await input.inputValue(), 'A12346', '采样后不得覆盖车牌');
    assert.deepEqual(errors, []);
    await page.close();
  }
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
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
  const reconnect = await browser.newPage({ viewport: { width: 1366, height: 768 } });
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
  const timezone = await browser.newPage();
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
  const fallback = await browser.newPage();
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
  const agentPage = await browser.newPage();
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
  const freshAgent = await browser.newPage();
  await freshAgent.route('**/api/weighing/**', async route => {
    const path = new URL(route.request().url()).pathname;
    const data = path.endsWith('/plate-candidates') ? { items: [], nextCursor: '0', serverTime: new Date().toISOString() } : path.endsWith('/cameras') ? { cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { whepBaseUrl: '', syncState: 'pending' } } : { records: [], total: 0 };
    return route.fulfill({ json: { code: 200, data } });
  });
  await freshAgent.goto(`${base}/tests/plate-candidates.html?scale=WB01`);
  await freshAgent.evaluate(() => window.dispatchEvent(new Event('fixture-agent')));
  await freshAgent.waitForFunction(() => document.querySelector('#workbench-plate')?.value === 'A22222');
  await freshAgent.close();
  const savedDraft = await browser.newPage();
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
  for (const [width, allowedShift] of [[1920, 8], [1366, 80]]) {
    const compact = await browser.newPage({ viewport: { width, height: 768 } });
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
    const phaseTop = await compact.locator('.workbench-phase').evaluate(node => node.getBoundingClientRect().top);
    const sameMoment = new Date();
    const candidate = (id, plate, cameras) => ({ id, plate, firstReceivedAt: sameMoment.toISOString(), lastReceivedAt: sameMoment.toISOString(), expiresAt: new Date(sameMoment.getTime() + 60_000).toISOString(), sourceEvents: cameras.map((name, index) => ({ eventRecordId: id + index, cameraId: index + 1, cameraName: name, receivedAt: sameMoment.toISOString(), recognizedAt: sameMoment.toISOString() })) });
    compactEvents = [candidate(1, '晋AB6P44', ['抓拍01', '抓拍01', '抓拍01'])];
    const chip = compact.getByRole('button', { name: /晋AB6P44/ });
    await chip.waitFor();
    assert.equal((await chip.innerText()).match(/抓拍01/g).length, 1, '同一相机只显示一次');
    assert.equal(await compact.locator('form .plate-candidates').count(), 1, '候选紧跟车牌输入');
    const chipTop = await chip.evaluate(node => node.getBoundingClientRect().top);
    assert.ok(chipTop < phaseTop, '候选不能排到阶段状态之后');
    const shiftedTop = await compact.locator('.workbench-phase').evaluate(node => node.getBoundingClientRect().top);
    assert.ok(shiftedTop - phaseTop <= allowedShift, `候选把下方内容推移过多：${shiftedTop - phaseTop}`);
    compactEvents = [...compactEvents, candidate(10, '晋AB6P44', ['抓拍01'])];
    await delay(2500);
    assert.equal(await compact.getByRole('button', { name: /晋AB6P44/ }).count(), 1, '同一车牌重复批次只保留一项');
    assert.equal((await compact.getByRole('button', { name: /晋AB6P44/ }).innerText()).match(/抓拍01/g).length, 1, '合并后相机仍只显示一次');
    await compact.close();
  }
  console.log('车牌候选工作台：双尺寸预填、冲突、人工输入、采样保护、下一辆水位、焦点和重连通过。');
} finally { clearTimeout(deadline); await browser?.close(); vite.kill('SIGTERM'); }
