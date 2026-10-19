import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, rm, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const root = fileURLToPath(new URL('../..', import.meta.url));
const directory = await mkdtemp(join(tmpdir(), 'weighbridge-master-'));
const api = 'http://127.0.0.1:18085';
const web = 'http://127.0.0.1:4185';
const mockUrl = 'http://127.0.0.1:18090';
const children = [];
let browser;
let page;
const workflowResponses = [];
const env = { ...process.env, GOCACHE: resolve(root, '.task/go-build'), APP_ENV: 'test', APP_CONFIG_PROFILE: '', APP_DATABASE__DRIVER: 'sqlite', APP_DATABASE__URL: join(directory, 'test.db'), APP_DATABASE__USERNAME: '', APP_DATABASE__PASSWORD: '', APP_JWT__SECRET: 'isolated-browser-test-secret-never-deployed', APP_FILE__STORAGE_ROOT: join(directory, 'uploads'), APP_HTTP__ADDRESS: '127.0.0.1:18085', APP_MONITORING__ENABLED: 'false', APP_WEIGHING__AGENT_KEY: 'isolated-agent-browser-key', APP_WEIGHING__IMAGE_DIR: join(directory, 'agent-images') };
function start(command, args, cwd, extraEnv = {}) {
  const child = spawn(command, args, { cwd, env: { ...env, ...extraEnv }, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
  child.output = '';
  child.stdout.on('data', data => { child.output += data; });
  child.stderr.on('data', data => { child.output += data; });
  children.push(child);
  return child;
}
async function finish(child) { await new Promise((ok, fail) => { child.once('error', fail); child.once('exit', code => code === 0 ? ok() : fail(new Error(child.output))); }); }
async function ready(url, child) {
  for (let n = 0; n < 100; n++) {
    if (child.exitCode !== null) throw new Error(child.output);
    try { if ((await fetch(url)).ok) return; } catch { /* Startup remains bounded. */ }
    await delay(150);
  }
  throw new Error(`Server did not become ready: ${url}\n${child.output}`);
}
async function login(page, username) {
  await page.goto(`${web}/login`);
  await page.getByPlaceholder('请输入用户名', { exact: true }).fill(username);
  await page.getByPlaceholder('请输入密码', { exact: true }).fill('admin123');
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await page.waitForURL(url => !url.pathname.startsWith('/login'));
}
const deadline = setTimeout(() => { for (const child of children) { try { if(child.exitCode===null) process.kill(-child.pid, 'SIGTERM'); } catch { /* Already exited. */ } } }, 180_000);
try {
  await finish(start('go', ['run', './cmd/migrate', 'up', '--kind', 'all'], resolve(root, 'server')));
  const server = start('go', ['run', './cmd/api'], resolve(root, 'server'));
  const vite = start(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4185', '--strictPort'], resolve(root, 'web'), { VITE_API_BASE_URL: api });
  await Promise.all([ready(`${api}/ready`, server), ready(`${web}/login`, vite)]);
  const mock = start('go', ['run', './cmd/agent-mock', '--server', api, '--listen', '127.0.0.1:18090'], resolve(root, 'server'));
  await ready(`${mockUrl}/api/state`, mock);
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const admin = await browser.newContext({ viewport: { width: 1366, height: 768 }, acceptDownloads: true });
  page = await admin.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('response', async response => { if(response.url().includes('/api/weighing/') && !response.url().endsWith('/device') && !response.url().includes('/agent-images/')) { const body = await response.text().catch(()=>''); workflowResponses.push({url:response.url(),status:response.status(),body:body.slice(0,1200)}); if(workflowResponses.length>20)workflowResponses.shift(); } });
  page.setDefaultTimeout(8000);
  await login(page, 'admin');
  const oldCache = await page.evaluate(() => { localStorage.setItem('master-unrelated-sentinel', 'keep'); const legacy=JSON.stringify({legacySentinel:'keep-original',tasks:[{id:'browser-only-task',plate:'晋A99999'}]}); localStorage.setItem('weighbridge-phase2a-v1',legacy); return legacy; });
  await page.goto(`${web}/weighing/materials/new`);
  await page.locator('#material-name').fill('服务端冒烟高粱');
  await page.getByLabel('规格／等级', { exact: true }).fill('一级');
  await page.getByRole('button', { name: '保存物料', exact: true }).click();
  await page.getByRole('heading', { name: '物料资料', exact: true }).waitFor();
  await page.getByRole('link', { name: '服务端冒烟高粱', exact: true }).waitFor();
  await page.reload();
  await page.getByRole('link', { name: '服务端冒烟高粱', exact: true }).waitFor();
  await page.goto(`${web}/weighing/parties/new`);
  await page.locator('#party-name').fill('服务端个人供运人');
  await page.locator('#party-nature').selectOption('individual');
  await page.getByRole('checkbox', { name: '承运商', exact: true }).check();
  await page.getByRole('checkbox', { name: '客户', exact: true }).check();
  await page.getByRole('button', { name: '添加联系人', exact: true }).click();
  await page.getByLabel('姓名（联系人1）', { exact: false }).fill('业务联系人');
  await page.getByRole('button', { name: '保存往来主体', exact: true }).click();
  await page.getByRole('heading', { name: '往来主体资料', exact: true }).waitFor();
  await page.getByRole('link', { name: '服务端个人供运人', exact: true }).waitFor();
  assert.equal(await page.evaluate(() => localStorage.getItem('master-unrelated-sentinel')), 'keep');
  assert.equal(await page.evaluate(() => localStorage.getItem('weighbridge-phase2a-v1')), oldCache);
  const token = await page.evaluate(() => JSON.parse(localStorage.getItem('react-admin-auth')).token);
  const call = async (path, method = 'GET', body) => {
    const response = await fetch(`${api}${path}`, { method, headers: { Authorization: token, 'Content-Type': 'application/json' }, ...(body ? { body: JSON.stringify(body) } : {}) });
    const payload = await response.json();
    assert.equal(payload.code, 200, payload.message);
    return payload.data;
  };

  // The actual management menu opens one reusable standalone workbench window.
  await page.goto(`${web}/weighing/tasks`);
  const menu=page.getByRole('navigation',{name:'主导航'});
  console.log('browser: master data ready; opening standalone workbench', await page.evaluate(() => ({name:window.name,url:location.href})), errors);
  assert.notEqual(await page.evaluate(() => window.name),'weighbridge-workbench','离开工作台后不得占用独立窗口名称');
  const popupPromise=admin.waitForEvent('page');
  await menu.getByRole('link',{name:'称重工作台',exact:true}).click();
  const popup=await popupPromise.catch(async error => { console.error('popup diagnostics',page.url(),await page.locator('body').innerText(),errors); throw error; });
  await popup.getByRole('heading',{name:'称重工作台',exact:true}).waitFor();
  assert.equal(await popup.locator('aside').count(),0);
  assert.equal(await popup.getByRole('navigation',{name:'主导航'}).count(),0);
  const pageCount=admin.pages().length;
  await menu.getByRole('link',{name:'称重工作台',exact:true}).click();
  await delay(150); assert.equal(admin.pages().length,pageCount);
  await popup.close();

  // Use the actual workbench, server samples and confirmation dialogs for both directions.
  const settings = await admin.newPage();
  settings.setDefaultTimeout(8000);
  await settings.goto(mockUrl);
  await settings.locator('#target').waitFor();
  assert.equal(await settings.getByLabel('演示岗位',{exact:true}).count(),0);
  assert.equal(await settings.getByRole('button',{name:/重置/}).count(),0,'共享业务不能提供整库重置');
  assert.equal((await call('/api/weighing/tasks?keyword=browser-only-task')).total,0,'旧浏览器交易不能自动导入');
  console.log('browser: standalone window passed; connecting mock console');
  let checkedWave = false;
  const setWeight = async value => {
    await settings.locator('#target').fill(String(value));
    await settings.getByRole('button',{name:'模拟上磅',exact:true}).click();
    if (!checkedWave) {
      const weights = new Set(); const start = Date.now();
      while (Date.now() - start < 3000) { const state = await call('/api/weighing/scales/1/device'); if (state.online && !state.stable) weights.add(state.weight); await delay(250); }
      assert.ok(weights.size >= 3, '系统应收到一段变化的不稳定重量'); checkedWave = true;
    } else await settings.getByRole('button',{name:'恢复稳定',exact:true}).click();
    for(let n=0;n<40;n++){const state=await call('/api/weighing/scales/1/device');if(state.online&&state.stable&&Number(state.weight)===value&&state.images.length===2)return;await delay(200);}
    throw Error('Agent weight did not become stable');
  };
  const pick = async (label, name) => {
    const field = page.getByRole('combobox', { name: `搜索${label}`, exact: true });
    await field.scrollIntoViewIfNeeded();
    await field.fill(name);
    await page.getByRole('option').filter({ hasText: name }).first().click();
    await page.getByLabel(`${label}已选资料`,{exact:true}).waitFor();
  };
  const locate = async plate => {
    await page.getByRole('textbox', { name: '车牌号码', exact: true }).fill(plate);
    await page.getByRole('button', { name: '核对车牌并查找运输', exact: true }).click();
  };
  const reviewAndConfirm = async (stage, kind) => {
    await page.getByRole('button', { name: `开始核对${stage}（${kind}）`, exact: true }).click();
    const dialog = page.getByRole('dialog', { name: `核对${stage}（${kind}）`, exact: true });
    await dialog.getByRole('button',{name:'车尾',exact:true}).click();
    await dialog.getByRole('button',{name:'车头',exact:true}).click();
    await dialog.getByRole('button',{name:'车头',exact:true}).click();
    await dialog.getByRole('button', { name: stage === '回磅' ? '确认称重并出单' : `确认${stage}（${kind}）`, exact: true }).click();
    await page.getByRole('heading', { name: `${stage}已完成`, exact: true }).waitFor();
  };
  for (const [direction, plate, firstWeight, secondWeight] of [['inbound', '川A54321', 30, 10], ['outbound', '川A54322', 10, 30]]) {
    await setWeight(firstWeight);
    await page.goto(`${web}/weighing/workbench`);
    await locate(plate);
    await page.getByLabel('运输方向', { exact: false }).selectOption(direction);
    assert.equal(await page.locator('aside').count(), 0, '独立工作台不应显示后台侧栏');
    const actionBox=await page.getByRole('button',{name:'采集重量',exact:true}).boundingBox();
    assert.ok(actionBox&&actionBox.y>=64&&actionBox.y+actionBox.height<=768,`称重主操作应在1366x768首屏可见: ${JSON.stringify(actionBox)}`);

    // Capturing before selecting master data must remain supported.
    await page.getByRole('button', { name: '采集重量', exact: true }).click();
    await page.getByRole('button', { name: '补齐资料后核对', exact: true }).waitFor();
    await pick('物料', '服务端冒烟高粱');
    await pick(direction === 'inbound' ? '供货方' : '收货方', '服务端个人供运人');
    await reviewAndConfirm('首磅', direction === 'inbound' ? '毛重' : '皮重');
    await page.getByRole('button', { name: '办理下一辆车', exact: true }).click();
    await setWeight(secondWeight);
    await page.reload();
    await locate(plate);
    await page.getByRole('button', { name: '采集重量', exact: true }).click();
    await reviewAndConfirm('回磅', direction === 'inbound' ? '皮重' : '毛重');
    const tasks = await call(`/api/weighing/tasks?keyword=${encodeURIComponent(plate)}`);
    assert.equal(tasks.total, 1);
    assert.equal(tasks.records[0].status, 'completed');
    const detail = await call(`/api/weighing/tasks/${tasks.records[0].id}`);
    assert.equal(detail.records.length, 2);
    const tickets = await call('/api/weighing/tickets?page=1&pageSize=500');
    const savedTicket = tickets.records.find(row => row.taskId === detail.id);
    assert.ok(savedTicket && savedTicket.mode === 'normal' && savedTicket.status === 'valid');
    assert.equal(Number(savedTicket.finalGross) - Number(savedTicket.finalTare), 20);
    await page.getByRole('button', { name: '办理下一辆车', exact: true }).click();
  }


  console.log('browser: both weighing directions passed');
  const inboundTask = (await call(`/api/weighing/tasks?keyword=${encodeURIComponent('川A54321')}`)).records[0];
  const inboundTicket = (await call(`/api/weighing/tickets?taskId=${inboundTask.id}`)).records[0];
  await page.goto(`${web}/weighing/tickets/${inboundTicket.id}`);
  await page.getByRole('button', { name: '作废单据', exact: true }).click();
  const voidDialog = page.getByRole('dialog', { name: '作废单据', exact: true });
  await voidDialog.locator('#ticket-operation-reason').fill('浏览器验证作废更正');
  await voidDialog.getByRole('button', { name: '确认保存', exact: true }).click();
  await page.getByRole('button', { name: '关联更正重开', exact: true }).click();
  const reopenDialog = page.getByRole('dialog', { name: '关联更正重开', exact: true });
  await reopenDialog.locator('#final-gross').fill('31');
  await reopenDialog.locator('#final-tare').fill('11');
  await reopenDialog.getByRole('checkbox', { name: /已核验本次采用的两磅依据仍有效/ }).check();
  await reopenDialog.locator('#ticket-operation-reason').fill('人工最终值更正');
  await reopenDialog.getByRole('button', { name: '确认保存', exact: true }).click();
  await page.waitForURL(url => url.pathname.startsWith('/weighing/tickets/') && !url.pathname.endsWith(`/${inboundTicket.id}`));
  const versions = (await call(`/api/weighing/tickets?taskId=${inboundTask.id}&pageSize=500`)).records;
  assert.equal(versions.length, 2);
  assert.equal(versions.filter(row => row.status === 'valid').length, 1);
  assert.equal(versions.find(row => row.id === inboundTicket.id).finalGross, '30');
  assert.equal(versions.find(row => row.status === 'valid').finalGross, '31');


  await page.goto(`${web}/weighing/originals`);
  await page.getByRole('cell', { name: '川A54321', exact: false }).waitFor();
  await page.getByLabel('车牌或编号', { exact: true }).fill('川A54321');
  await page.getByText(/筛选 1 趟；关联有效单 1 趟，原始净重合计 20 吨/).waitFor();
  const downloadReady = page.waitForEvent('download');
  await page.getByRole('button', { name: '导出明细 CSV', exact: true }).click();
  const download = await downloadReady;
  const csvText = await readFile(await download.path(), 'utf8');
  assert.match(csvText, /川A54321/);
  assert.doesNotMatch(csvText, /川A54322/);
  await page.goto(`${web}/weighing/tickets`);
  await page.getByText(/筛选 3 张；当前有效 2 趟，最终净重合计 40 吨/).waitFor();
  const currentTicket = (await call(`/api/weighing/tickets?taskId=${inboundTask.id}&status=valid`)).records[0];
  await page.goto(`${web}/weighing/tickets/${currentTicket.id}/preview`);
  await page.getByRole('button', { name: '发起 Agent 打印', exact: true }).click();
  await settings.getByRole('heading',{name:/打印尝试 #.*待回报/}).waitFor();
  const unknownPrint = await call(`/api/weighing/tickets/${currentTicket.id}`);
  assert.equal(unknownPrint.printStatus, 'unknown');
  assert.equal(unknownPrint.reprintCount, 0);
  await page.locator('#preview-print-result').selectOption('printed');
  await page.locator('#preview-result-reason').fill('操作员确认实际已出纸');
  await page.getByRole('button', { name: '保存人工核实结果', exact: true }).click();
  await page.getByText('已保存操作员反馈。', { exact: true }).waitFor();
  await page.locator('#preview-reprint-reason').fill('客户要求补打');
  await page.getByRole('button', { name: '登记补打并打开打印', exact: true }).click();
  await settings.getByRole('heading',{name:/打印尝试 #.*待回报/}).nth(1).waitFor();
  const reprinted = await call(`/api/weighing/tickets/${currentTicket.id}`);
  assert.equal(reprinted.reprintCount, 1);
  assert.equal(reprinted.printStatus, 'unknown');
  assert.equal(reprinted.finalGross, '31');
  // Device results flow back through WSS and update the ordinary system preview.
  await settings.getByRole('button',{name:'回报成功',exact:true}).first().click();
  for(let n=0;n<30;n++){if((await call(`/api/weighing/tickets/${currentTicket.id}`)).printStatus==='printed')break;await delay(100);}
  assert.equal((await call(`/api/weighing/tickets/${currentTicket.id}`)).printStatus,'printed');



  // An unknown successful response survives reload and is resolved without a new submission.
  await page.route('**/api/weighing/tasks/first-confirmation', async route => { await route.fetch(); await route.abort('failed'); }, {times:1});
  await setWeight(25);
  await page.goto(`${web}/weighing/workbench`);
  await locate('川A54323');
  await pick('物料', '服务端冒烟高粱'); await pick('供货方', '服务端个人供运人');
  await page.getByRole('button', { name: '采集重量', exact: true }).click();
  await page.getByRole('button', { name: '开始核对首磅（毛重）', exact: true }).click();
  await page.getByRole('dialog', { name: '核对首磅（毛重）', exact: true }).getByRole('button', { name: '确认首磅（毛重）', exact: true }).click();
  await page.getByRole('heading', { name: '提交结果待核实', exact: true }).waitFor();
  await page.reload();
  await page.getByRole('button', { name: '查询上次提交结果', exact: true }).click();
  await page.getByRole('heading', { name: '首磅已完成', exact: true }).waitFor();
  const recovered = (await call(`/api/weighing/tasks?keyword=${encodeURIComponent('川A54323')}`)).records;
  assert.equal(recovered.length, 1);
  assert.equal((await call(`/api/weighing/tasks/${recovered[0].id}`)).records.length, 1);
  await page.getByRole('button', { name: '办理下一辆车', exact: true }).click();
  // A missing result remains pending; only explicit replay of the same request may execute it.
  await page.route('**/api/weighing/tasks/first-confirmation', route => route.abort('failed'), {times:1});
  await page.reload(); await locate('川A54324');
  await pick('物料', '服务端冒烟高粱'); await pick('供货方', '服务端个人供运人');
  await page.getByRole('button', { name: '采集重量', exact: true }).click();
  await page.getByRole('button', { name: '开始核对首磅（毛重）', exact: true }).click();
  await page.getByRole('dialog', { name: '核对首磅（毛重）', exact: true }).getByRole('button', { name: '确认首磅（毛重）', exact: true }).click();
  await page.getByRole('heading', { name: '提交结果待核实', exact: true }).waitFor();
  await page.getByRole('button', { name: '查询上次提交结果', exact: true }).click();
  await page.getByText(/尚未查到已提交结果，可能仍在处理/).waitFor();
  assert.equal((await call(`/api/weighing/tasks?keyword=${encodeURIComponent('川A54324')}`)).total, 0);
  await page.getByRole('button', { name: '按原请求重试', exact: true }).click();
  await page.getByRole('heading', { name: '首磅已完成', exact: true }).waitFor();
  assert.equal((await call(`/api/weighing/tasks?keyword=${encodeURIComponent('川A54324')}`)).total, 1);
  await page.getByRole('button', { name: '办理下一辆车', exact: true }).click();

  // Preserve the invalid-original and responsive-dialog regression checks against real server data.
  await setWeight(5);
  await page.goto(`${web}/weighing/workbench`); await locate('川A54327');
  await pick('物料','服务端冒烟高粱'); await pick('供货方','服务端个人供运人');
  await page.getByRole('button',{name:'采集重量',exact:true}).click();
  await reviewAndConfirm('首磅','毛重');
  await page.getByRole('button',{name:'办理下一辆车',exact:true}).click();
  await locate('川A54327');
  await page.getByRole('button',{name:'采集重量',exact:true}).click();
  await page.getByRole('button',{name:'开始核对回磅（皮重）',exact:true}).click();
  let invalidDialog=page.getByRole('dialog',{name:'核对回磅（皮重）',exact:true});
  await invalidDialog.getByLabel('最终毛重（吨）',{exact:true}).fill('4.5');
  await invalidDialog.getByLabel('最终皮重（吨）',{exact:true}).fill('1');
  const difference=label=>invalidDialog.getByText(label,{exact:true}).locator('..');
  assert.match(await difference('系统计算：毛重调整差额').innerText(),/0\.5 吨/);
  assert.match(await difference('系统计算：皮重调整差额').innerText(),/4 吨/);
  assert.equal(await invalidDialog.getByRole('button',{name:'确认称重并出单',exact:true}).isDisabled(),true);
  assert.match(await invalidDialog.getByRole('alert').last().innerText(),/无法出单：.*请返回.*重新采集/);
  const contextRegion=invalidDialog.getByRole('region',{name:'本趟运输核对依据',exact:true});
  const weightRegion=invalidDialog.getByRole('region',{name:'最终重量与调整',exact:true});
  const contextBox=await contextRegion.boundingBox(), weightBox=await weightRegion.boundingBox();
  assert.ok(weightBox.x>contextBox.x+contextBox.width);
  assert.ok((await invalidDialog.boundingBox()).width>=1000);
  await page.setViewportSize({width:390,height:844});
  assert.equal(await invalidDialog.evaluate(el=>el.scrollWidth<=el.clientWidth),true);
  const narrowContext=await contextRegion.boundingBox(), narrowWeight=await weightRegion.boundingBox();
  assert.ok(narrowWeight.y>=narrowContext.y+narrowContext.height);
  const confirmBox=await invalidDialog.getByRole('button',{name:'确认称重并出单',exact:true}).boundingBox();
  assert.ok(confirmBox.y>=0&&confirmBox.y+confirmBox.height<=844);
  await page.setViewportSize({width:1366,height:768});
  await invalidDialog.getByLabel('最终毛重（吨）',{exact:true}).fill('6');
  assert.match(await difference('系统计算：毛重调整差额').innerText(),/-1 吨（上调）/);
  await invalidDialog.getByLabel('最终毛重（吨）',{exact:true}).fill('');
  assert.match(await difference('系统计算：毛重调整差额').innerText(),/—/);
  await invalidDialog.getByRole('button',{name:'不一致，返回修改',exact:true}).click();
  await setWeight(1.2);
  await page.getByRole('button',{name:'采集重量',exact:true}).click();
  await page.getByRole('button',{name:'开始核对回磅（皮重）',exact:true}).click();
  invalidDialog=page.getByRole('dialog',{name:'核对回磅（皮重）',exact:true});
  await invalidDialog.getByLabel('最终毛重（吨）',{exact:true}).fill('4.5');
  await invalidDialog.getByLabel('最终皮重（吨）',{exact:true}).fill('1');
  await invalidDialog.getByText('系统计算调整差额：毛重 0.5 吨 · 皮重 0.2 吨',{exact:true}).waitFor();
  await invalidDialog.getByRole('button',{name:'确认称重并出单',exact:true}).click();
  await page.getByRole('heading',{name:'回磅已完成',exact:true}).waitFor();
  const correctedTask=(await call(`/api/weighing/tasks?keyword=${encodeURIComponent('川A54327')}`)).records[0];
  const correctedTicket=(await call(`/api/weighing/tickets?taskId=${correctedTask.id}`)).records[0];
  assert.deepEqual([correctedTicket.finalGross,correctedTicket.finalTare,correctedTicket.finalNet],['4.5','1','3.5']);
  assert.deepEqual((await call(`/api/weighing/tasks/${correctedTask.id}`)).records.map(row=>row.weight),['5','1.2']);
  await page.getByRole('button',{name:'办理下一辆车',exact:true}).click();

  // Agent recognition candidates still need manual confirmation.
  await settings.locator('#plate').fill('苏AD12346');
  await settings.getByRole('button',{name:'发送识别候选',exact:true}).click();
  await page.goto(`${web}/weighing/workbench`);
  await page.getByText(/识别候选 苏AD12346/).waitFor();
  assert.equal(await page.getByRole('button',{name:'采集重量',exact:true}).isDisabled(),true);
  await page.getByRole('textbox',{name:'车牌号码',exact:true}).fill('苏ad12345');
  assert.equal(await page.getByRole('textbox',{name:'车牌号码',exact:true}).inputValue(),'AD12345');
  await page.getByRole('button',{name:'核对车牌并查找运输',exact:true}).click();
  await page.getByLabel('运输方向',{exact:false}).selectOption('outbound');
  await pick('物料','服务端冒烟高粱'); await pick('收货方','服务端个人供运人');
  await page.getByRole('button',{name:'采集重量',exact:true}).click();
  for(const label of ['小窗','大窗','标准']) await page.getByRole('button',{name:label,exact:true}).click();
  await page.getByRole('button',{name:'放大摄像头画面',exact:true}).click();
  await page.getByRole('dialog',{name:'摄像头画面',exact:true}).getByRole('button',{name:'关闭图片预览'}).click();
  assert.equal(await page.getByRole('button',{name:'放大摄像头画面',exact:true}).evaluate(el=>el===document.activeElement),true);
  await reviewAndConfirm('首磅','皮重');
  const recognitionTask=(await call(`/api/weighing/tasks?keyword=${encodeURIComponent('苏AD12345')}`)).records[0];
  const recognitionRecord=(await call(`/api/weighing/tasks/${recognitionTask.id}`)).records[0];
  assert.equal(recognitionRecord.plateEvidence.candidate,'苏AD12346');
  assert.equal(recognitionRecord.plateEvidence.confirmedPlate,'苏AD12345');
  assert.equal(recognitionRecord.plateEvidence.source,'agent');
  await page.getByRole('button',{name:'办理下一辆车',exact:true}).click();

  await call('/api/system/user', 'POST', { username: 'master-reader', nickname: '只读操作员', status: 1 });
  const users = await call('/api/system/user/page?page=1&pageSize=500');
  const user = users.records.find(row => row.username === 'master-reader');
  const roles = await call('/api/system/role/options');
  const role = roles.find(row => row.roleCode === 'WEIGHING_OPERATOR');
  assert.ok(user && role);
  await call(`/api/system/user/${user.id}/roles`, 'PUT', { roleIds: [role.id] });
  const reader = await browser.newContext();
  const readerPage = await reader.newPage();
  await login(readerPage, 'master-reader');
  await readerPage.goto(`${web}/weighing/materials`);
  await readerPage.getByRole('link', { name: '服务端冒烟高粱', exact: true }).waitFor();
  assert.equal(await readerPage.getByRole('link', { name: '新增物料', exact: true }).count(), 0);
  await readerPage.getByText('资料为只读。需要维护时请联系管理员授权。', { exact: true }).waitFor();
  const denied = await readerPage.evaluate(async endpoint => {
    const auth = JSON.parse(localStorage.getItem('react-admin-auth'));
    const response = await fetch(`${endpoint}/api/weighing/materials`, { method: 'POST', headers: { Authorization: auth.token, 'Content-Type': 'application/json' }, body: JSON.stringify({ name: '越权新增' }) });
    return response.status;
  }, api);
  assert.equal(denied, 403);

  // A second real account must acquire/take over the same server lease; stale confirmation cannot succeed.
  await setWeight(30);
  await page.goto(`${web}/weighing/workbench`); await locate('川A54325');
  await pick('物料','服务端冒烟高粱'); await pick('供货方','服务端个人供运人');
  await page.getByRole('button',{name:'采集重量',exact:true}).click();
  await page.getByRole('button',{name:'开始核对首磅（毛重）',exact:true}).waitFor();
  await page.getByRole('button',{name:'开始核对首磅（毛重）',exact:true}).click();
  await readerPage.goto(`${web}/weighing/workbench`);
  await readerPage.getByRole('textbox',{name:'车牌号码',exact:true}).fill('川A54326');
  await readerPage.getByRole('button',{name:'核对车牌并查找运输',exact:true}).click();
  assert.equal(await readerPage.getByRole('button',{name:'采集重量',exact:true}).isDisabled(),true,'其他账号持有会话时必须禁用采集');
  await readerPage.locator('#takeover-reason').fill('现场操作员接管验证');
  await readerPage.getByRole('button',{name:'接管操作权',exact:true}).click();
  await readerPage.getByText('操作员已接管，记录已保留。',{exact:true}).waitFor();
  await page.getByRole('dialog',{name:'核对首磅（毛重）',exact:true}).getByRole('button',{name:'确认首磅（毛重）',exact:true}).click();
  await page.getByText('地磅操作会话已失效，请重新取得操作权',{exact:true}).waitFor();
  assert.equal((await call(`/api/weighing/tasks?keyword=${encodeURIComponent('川A54325')}`)).total,0);
  await readerPage.getByRole('button',{name:'释放操作权',exact:true}).click();

  assert.deepEqual(errors, []);
  console.log('Server-backed business loop: persistence, permissions, both weighing directions, capture before business data, immutable ticket versions, correction/reopen, server lists/CSV, Agent printing, manual feedback, invalid original recovery, responsive layout, recognition evidence and real account takeover passed.');
} catch (error) {
  if(page){console.error('browser failure:',page.url(),await page.locator('body').innerText().catch(()=>''), workflowResponses);await page.screenshot({path:resolve(root,'.task/agent-mock-browser-failure.png'),fullPage:true}).catch(()=>{});}
  throw error;
} finally {
  clearTimeout(deadline);
  await browser?.close();
  for (const child of children) { try { if(child.exitCode===null) process.kill(-child.pid, 'SIGTERM'); } catch { /* Already exited. */ } }
  await delay(150);
  await rm(directory, { recursive: true, force: true });
}
