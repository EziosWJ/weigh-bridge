import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:4194';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4194', '--strictPort'], { cwd: new URL('..', import.meta.url), env: process.env, stdio: 'ignore' });
let browser;
let page;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 45_000);
try {
  for (let n = 0; n < 60; n++) {
    try { if ((await fetch(`${base}/tests/weighing-agent-capture.html`)).ok) break; } catch { /* Starting. */ }
    await delay(100);
  }
  browser = await chromium.launch({ headless: true });
  page = await browser.newPage({ viewport: { width: 1366, height: 900 } });
  page.setDefaultTimeout(5000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  let failCapture = false;
  let optionalCapture = false;
  let captures = 0;
  let confirmations = 0;
  let rejectRegistration = false;
  let failPartySearch = false;
  let grainCandidates = [];
  let temporaryParty;
  const temporaryRequests = [];
  const formalParty = { id: 21, code: 'C-P-0001', name: '测试供货方', nature: 'enterprise', roles: ['supplier'], enabled: 1, version: 1, contacts: [] };
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jKxoAAAAASUVORK5CYII=', 'base64');
  await page.route('**/api/system/dict/TRANSPORT_DIRECTION/items', route => route.fulfill({ json: { code: 200, data: [] } }));
  await page.route('**/api/weighing/**', async route => {
    const path = new URL(route.request().url()).pathname;
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (path === '/api/weighing/cameras') return ok({ cameras: [], configVersion: 1, captureTimeoutMs: 6000, video: { apiUrl: '', apiUsername: '', hasApiPassword: false, whepBaseUrl: '', version: 1, syncState: 'pending', syncedVersion: 0, updateTime: new Date().toISOString() } });
    if (path.endsWith('/tasks/active')) return ok(null);
    if (path.endsWith('/trip-candidates')) return ok({ enabled: true, available: true, candidates: grainCandidates });
    if (path.endsWith('/materials')) return ok({ records: [{ id: 11, code: 'C-M-0001', name: '高粱', spec: '', enabled: 1, version: 1 }], total: 1 });
    if (path.endsWith('/parties')) {
      if (failPartySearch) return route.fulfill({ status: 500, json: { code: 500, message: '加载资料失败' } });
      const records = temporaryParty?.enabled ? [formalParty, temporaryParty] : [formalParty];
      return ok({ records, total: records.length });
    }
    if (path.includes('/temporary-parties')) {
      const method = route.request().method();
      const { version, ...body } = route.request().postDataJSON();
      temporaryRequests.push({ method, path, version, ...body });
      if (rejectRegistration) return route.fulfill({ status: 400, json: { code: 400, message: '本地保存失败' } });
      temporaryParty = { id: 32, code: 'T-P-LOCAL-32', enabled: 1, temporaryState: 'pending',
        contacts: [], version: (version ?? 0) + 1, ...body };
      return ok(temporaryParty);
    }
    if (path.includes('/agent-images/')) return route.fulfill({ contentType: 'image/png', body: png });
    if (path.includes('/session/')) return ok({ scaleCode: 'WB01', holderUserId: 1, generation: 1, sessionToken: 'isolated-session', expiresAt: new Date(Date.now() + 60000).toISOString() });
    if (path.endsWith('/samples')) {
      captures++;
      if (failCapture) return route.fulfill({ status: 409, json: { code: 409, message: '装车区摄像头抓拍失败' } });
      const images = optionalCapture ? [] : [{ id: 'entry', url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22320%22 height=%22180%22%3E%3Crect width=%22320%22 height=%22180%22 fill=%22%23245e86%22/%3E%3C/svg%3E', source: 'server', cameraId: 11, cameraName: '入口' }, { id: 'yard', url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22320%22 height=%22180%22%3E%3Crect width=%22320%22 height=%22180%22 fill=%22%23245e36%22/%3E%3C/svg%3E', source: 'server', cameraId: 12, cameraName: '装车区' }];
      return ok({ id: 901, taskId: null, stage: 'first', weight: '18.665', expiresAt: null,
        evidence: { id: '901', source: 'server', captureRequired: !optionalCapture, capturePolicyRevision: 'policy-1', imageStatus: optionalCapture ? 'disabled' : 'complete', images, at: new Date().toISOString(), weight: '18.665', plate: '晋A12345', cameraConfigVersion: 1 },
        plateEvidence: { source: 'manual', confirmedPlate: '晋A12345', at: new Date().toISOString() },
      });
    }
    if (path.includes('confirmation')) confirmations++;
    return ok({ records: [], total: 0 });
  });
  const open = scenario => page.goto(`${base}/tests/weighing-agent-capture.html?scale=WB01&scenario=${scenario}`);
  await open('ready');
  await page.getByRole('heading', { name: '现场画面', exact: false }).waitFor();
  const plateInput = page.getByRole('textbox', { name: '车牌号码', exact: true });
  assert.equal(await plateInput.getAttribute('placeholder'), '请输入车牌号码');
  assert.equal(await plateInput.evaluate(el => getComputedStyle(el).backgroundImage), 'none');
  await plateInput.fill('ad12345');
  assert.equal(await plateInput.inputValue(), 'AD12345');
  assert.equal(await plateInput.evaluate(el => el.scrollWidth <= el.clientWidth && el.scrollLeft === 0), true);
  await plateInput.evaluate(el => {
    const clipboardData = new DataTransfer();
    clipboardData.setData('text/plain', '苏A12345');
    el.dispatchEvent(new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData }));
  });
  assert.equal(await plateInput.inputValue(), 'A12345');
  assert.equal(await page.getByRole('button', { name: '选择车牌省份' }).innerText(), '苏');
  await open('ready');
  const capture = page.getByRole('button', { name: '采集重量', exact: true });
  assert.equal(await capture.isEnabled(), true, '真实 Agent 没有预览图片时，也必须允许发起首次采集');
  await capture.click();
  await page.waitForFunction(() => JSON.parse(document.querySelector('output').textContent).sampleReady);
  assert.equal(captures, 1, '应经过真实 store 向 /samples 发起采集');
  assert.equal(confirmations, 0, '采集不能自动确认正式称重');
  const review = page.getByRole('dialog', { name: '核对首磅（毛重）', exact: true });
  await review.waitFor();
  await review.getByText('还缺物料、供货方', { exact: true }).waitFor();
  const photos = review.getByRole('group', { name: '本次抓拍图片', exact: true });
  await photos.getByRole('img', { name: '入口抓拍', exact: true }).waitFor();
  await photos.getByRole('img', { name: '装车区抓拍', exact: true }).waitFor();
  assert.equal(await review.getByRole('button', { name: '确认首磅（毛重）', exact: true }).isDisabled(), true, '缺资料不能确认');
  await review.getByRole('button', { name: '关闭核对', exact: true }).click();

  failCapture = true;
  await open('ready');
  await capture.click();
  await page.getByText('装车区摄像头抓拍失败', { exact: true }).waitFor();
  assert.equal(JSON.parse(await page.locator('output').textContent()).sampleReady, false);
  assert.equal(confirmations, 0);
  assert.equal(await capture.isEnabled(), true, '抓拍失败后可以人工重试');
  for (const scenario of ['offline', 'dynamic', 'fault', 'zero', 'forbidden', 'occupied']) {
    await open(scenario);
    await capture.waitFor();
    assert.equal(await capture.isDisabled(), true, `${scenario} 必须继续阻止采集`);
  }
  failCapture = false;
  optionalCapture = true;
  await open('optional');
  await capture.click();
  await page.waitForFunction(() => JSON.parse(document.querySelector('output').textContent).sampleReady);
  assert.equal(JSON.parse(await page.locator('output').textContent()).images, 0);
  const optionalReview = page.getByRole('dialog', { name: '核对首磅（毛重）', exact: true });
  await optionalReview.waitFor();
  await optionalReview.getByText('本次未强制抓拍，允许无图确认；请对照现场车辆核对。', { exact: true }).waitFor();
  assert.equal(confirmations, 0, '无图采集仍不能自动确认');
  await optionalReview.getByRole('button', { name: '关闭核对', exact: true }).click();
  await open('optional-review');
  const confirm = page.getByRole('button', { name: '确认首磅（毛重）', exact: true });
  await confirm.waitFor();
  assert.equal(await confirm.isEnabled(), true, '关闭强制抓拍后不能等待不存在的图片');
  await confirm.click();
  await page.waitForFunction(() => document.querySelector('output').textContent === 'confirmed');
  await open('required-review');
  await confirm.waitFor();
  assert.equal(await confirm.isDisabled(), true, '强制抓拍无图仍须阻止确认');
  await open('changed-business-review');
  await confirm.waitFor();
  assert.equal(await confirm.isDisabled(), true, '业务资料变更后旧核对窗口必须重新核对');
  for (const scenario of ['temporary-denied', 'temporary-unbound']) {
    await open(scenario);
    await page.getByRole('combobox', { name: '搜索供货方' }).waitFor();
    assert.equal(await page.getByRole('button', { name: /登记临时供货方/ }).count(), 0);
  }

  await open('temporary');
  const basic = page.getByRole('tabpanel', { name: '基本信息', exact: true });
  const registerSupplier = basic.getByRole('button', { name: '登记临时供货方', exact: true });
  const supplier = basic.getByRole('combobox', { name: '搜索供货方' });
  const suppliers = basic.getByRole('listbox', { name: '供货方候选' });
  const create = page.getByRole('dialog', { name: '登记临时供货方' });
  const registerAndUse = create.getByRole('button', { name: '保存并选用', exact: true });
  const company = create.getByRole('radio', { name: '企业', exact: true });
  const individual = create.getByRole('radio', { name: '个人', exact: true });

  failPartySearch = true;
  await supplier.fill('个人供货方甲');
  const retrySearch = basic.getByRole('button', { name: '重试搜索', exact: true });
  await retrySearch.waitFor();
  const createFromEmpty = basic.getByRole('button', { name: '登记为临时供货方', exact: true });
  assert.equal(await createFromEmpty.count(), 0, '搜索失败时不能出现无结果登记动作');
  failPartySearch = false;
  await retrySearch.click();
  await basic.getByText('未找到供货方', { exact: true }).waitFor();
  await createFromEmpty.click();
  await create.waitFor();
  assert.equal(await create.getByRole('textbox', { name: '名称' }).inputValue(), '个人供货方甲');
  assert.equal(await company.isChecked(), false);
  assert.equal(await individual.isChecked(), false);
  assert.equal(await registerAndUse.isDisabled(), true);
  assert.equal(await create.getByRole('checkbox', { name: '供货商' }).count(), 0, '本次必需身份固定随入口带入');
  await individual.check();
  assert.equal(await registerAndUse.isEnabled(), true);
  await create.getByRole('button', { name: '取消' }).click();

  await registerSupplier.click();
  await create.getByRole('textbox', { name: '名称' }).fill('测试供货方');
  await company.check();
  await registerAndUse.click();
  await create.getByRole('region', { name: '同名档案核对' }).getByText('正式', { exact: true }).waitFor();
  assert.equal(temporaryRequests.length, 0, '先核对同名记录才允许登记');
  await create.getByRole('button', { name: '选用已有档案 C-P-0001' }).click();
  await create.waitFor({ state: 'hidden' });
  await page.waitForFunction(() => document.querySelector('[aria-label="搜索供货方"]').value === '测试供货方');
  assert.equal(temporaryRequests.length, 0, '选择已有同名档案不创建临时主体');

  await registerSupplier.click();
  await create.getByRole('textbox', { name: '名称' }).fill('测试供货方');
  await company.check();
  await create.getByRole('checkbox', { name: '承运商' }).check();
  await registerAndUse.click();
  const continueRegistration = create.getByRole('button', { name: '仍然登记并选用', exact: true });
  rejectRegistration = true;
  await continueRegistration.click();
  await create.getByRole('alert').getByText('本地保存失败').waitFor();
  assert.equal(await create.getByRole('textbox', { name: '名称' }).inputValue(), '测试供货方');
  rejectRegistration = false;
  await continueRegistration.click();
  await create.waitFor({ state: 'hidden' });
  assert.deepEqual(temporaryRequests.at(-1), {
    method: 'POST', path: '/api/weighing/temporary-parties', version: undefined,
    name: '测试供货方', nature: 'enterprise', roles: ['supplier', 'carrier'],
  });
  await basic.getByText('临时', { exact: true }).waitFor();
  await supplier.focus();
  await suppliers.getByRole('option', { name: /临时/ }).waitFor();
  assert.equal(await suppliers.getByRole('option').count(), 2, '同名的正式和临时主体都可选');
  await page.keyboard.press('Escape');
  await basic.getByRole('button', { name: '更正临时供货方' }).click();
  const edit = page.getByRole('dialog', { name: '更正临时供货方' });
  await edit.getByText('T-P-LOCAL-32').waitFor();
  await edit.getByRole('textbox', { name: '名称' }).fill('已更正供货方');
  await edit.getByRole('button', { name: '保存更正', exact: true }).click();
  await edit.waitFor({ state: 'hidden' });
  assert.deepEqual(temporaryRequests.at(-1), {
    method: 'PUT', path: '/api/weighing/temporary-parties/32', version: 1,
    name: '已更正供货方', nature: 'enterprise', roles: ['supplier', 'carrier'],
  });
  assert.equal(await supplier.inputValue(), '已更正供货方');
  await page.getByRole('tab', { name: '补充信息（选填）' }).click();
  const supplementary = page.getByRole('tabpanel', { name: '补充信息（选填）' });
  const carrier = supplementary.getByRole('combobox', { name: '搜索承运方' });
  await carrier.focus();
  await supplementary.getByRole('option', { name: /已更正供货方.*临时/ }).click();
  await page.waitForFunction(() => document.querySelector('[aria-label="搜索承运方"]').value === '已更正供货方');
  await supplementary.getByRole('button', { name: '登记临时承运方' }).click();
  const createCarrier = page.getByRole('dialog', { name: '登记临时承运方' });
  await createCarrier.getByText('承运方', { exact: true }).waitFor();
  assert.equal(await createCarrier.getByRole('checkbox', { name: '承运商' }).count(), 0);
  await createCarrier.getByRole('button', { name: '取消' }).click();
  await page.getByRole('tab', { name: '基本信息' }).click();
  await page.getByRole('radio', { name: '出厂装货' }).check();
  await basic.getByRole('button', { name: '登记临时收货方' }).click();
  const createCustomer = page.getByRole('dialog', { name: '登记临时收货方' });
  assert.equal(await createCustomer.getByRole('checkbox', { name: '客户' }).count(), 0);
  await createCustomer.getByRole('button', { name: '取消' }).click();

  await open('temporary-grain');
  const boundSupplier = basic.getByRole('group', { name: '粮食流转单供货方' });
  await boundSupplier.getByText('来自粮食流转单', { exact: true }).waitFor();
  await boundSupplier.getByText('C-P-0001', { exact: true }).waitFor();
  assert.equal(await supplier.count(), 0);
  assert.equal(await registerSupplier.count(), 0);
  await page.getByRole('tab', { name: '补充信息（选填）' }).click();
  await supplementary.getByRole('button', { name: '登记临时承运方' }).waitFor();
  await open('temporary-grain-disabled');
  await boundSupplier.getByText('指定正式供货方已停用，请核对流转单。', { exact: true }).waitFor();
  const grain = (tripNo) => ({ tripId: 1, tripNo, version: 1, plate: '晋A12345',
    partyCode: 'C-P-0001', partyName: '测试供货方', materialCode: 'C-M-0001', materialName: '高粱',
    inspectionMethod: 'visual', linkable: true, blockers: [], deductions: [] });
  grainCandidates = [grain('LT-001')];
  await page.getByRole('button', { name: '确认车辆', exact: true }).click();
  await page.getByRole('alert').getByText(/LT-001 暂无法关联。.*指定正式供货方已停用/).waitFor();
  assert.equal(await boundSupplier.count(), 0, '不可用供货方的流转单不能完成关联');
  grainCandidates = [grain('LT-001'), grain('LT-002')];
  await open('temporary-grain');
  await page.getByRole('button', { name: '确认车辆', exact: true }).click();
  const trips = page.getByLabel('云端流转单人工选择');
  await trips.getByRole('button', { name: /LT-001/ }).click();
  await boundSupplier.getByText('来自粮食流转单').waitFor();
  await trips.getByRole('button', { name: /LT-002/ }).click();
  await trips.getByText('已选 LT-002，可在此更换。', { exact: true }).waitFor();
  assert.equal(await supplier.count(), 0, '换流转单后供货方仍为只读');
  grainCandidates = [];


  await open('temporary');
  await supplier.focus();
  await suppliers.getByRole('option', { name: /已更正供货方/ }).waitFor();
  await page.keyboard.press('Escape');
  temporaryParty = { ...temporaryParty, enabled: 0, temporaryState: 'merged' };
  await supplier.press('ArrowDown');
  await page.waitForFunction(() => {
    const options = document.querySelectorAll('[aria-label="供货方候选"] [role="option"]');
    return options.length === 1 && options[0].textContent.includes('C-P-0001');
  });

  optionalCapture = false;
  await open('temporary-captured');
  const previousCaptures = captures;
  await capture.click();
  await review.getByRole('button', { name: '登记临时供货方', exact: true }).click();
  await create.waitFor();
  await page.keyboard.press('Escape');
  await create.waitFor({ state: 'hidden' });
  assert.equal(await review.isVisible(), true, '关闭登记弹窗只返回称重核对');
  await review.getByRole('button', { name: '登记临时供货方', exact: true }).click();
  await create.getByRole('textbox', { name: '名称' }).fill('采集后登记供货方');
  await individual.check();
  await registerAndUse.click();
  await create.waitFor({ state: 'hidden' });
  await review.getByText(/采集后登记供货方 · T-P-LOCAL-32/).waitFor();
  await page.waitForFunction(() => Array.from(document.querySelectorAll('button')).some(button =>
    button.textContent.trim() === '确认首磅（毛重）' && !button.disabled));
  const frozen = JSON.parse(await page.locator('output').textContent());
  assert.equal(frozen.sampleReady, true);
  assert.equal(frozen.images, 2);
  assert.equal(captures, previousCaptures + 1);
  assert.equal(confirmations, 0, '登记后保留人工首磅确认');
  assert.deepEqual(errors, []);
  console.log('临时主体工作台验证通过：授权、登记角色、更正版本、同名候选、粮食流转供货方及归并后刷新。');
  console.log('首次抓拍回归通过：无预览可请求采集、按真实摄像头名称展示多图、失败不生成采样、不自动确认、设备与权限限制保留；可选无图采集及人工确认可用，强制无图和业务资料变更仍阻断。');
} finally {
  clearTimeout(deadline);
  await browser?.close();
  vite.kill('SIGTERM');
}
