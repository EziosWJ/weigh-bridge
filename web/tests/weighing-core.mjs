import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import test, { after } from 'node:test';
import ts from 'typescript';

// 直接编译业务模块到临时目录，复用项目 TypeScript，无需测试框架或后端。
const temporary = await mkdtemp(path.join(tmpdir(), 'weighing-core-'));
after(() => rm(temporary, { recursive: true, force: true }));
const sourceRoot = new URL('../src/', import.meta.url);
const compile = async (relative) => {
  const source = await readFile(new URL(relative, sourceRoot), 'utf8');
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 } }).outputText
    .replace(/(from\s+['"])(\.[^'"]+)(['"])/g, '$1$2.js$3');
  const output = path.join(temporary, relative.replace(/\.ts$/, '.js'));
  await mkdir(path.dirname(output), { recursive: true });
  await writeFile(output, compiled);
};
await writeFile(path.join(temporary, 'package.json'), '{"type":"module"}');
await Promise.all(['types/weighing.ts', 'lib/weighing/camera.ts', 'lib/weighing/weights.ts', 'lib/weighing/seed.ts', 'lib/weighing/engine.ts', 'lib/weighing/migration.ts', 'lib/weighing/query.ts'].map(compile));
const { createInitialState } = await import(pathToFileURL(path.join(temporary, 'lib/weighing/seed.js')));
const { formatWeightInputValue, formatWeightValue, formatWeight, needsLegacyWeightReview, weightValues } = await import(pathToFileURL(path.join(temporary, 'lib/weighing/weights.js')));
const { applyCommand } = await import(pathToFileURL(path.join(temporary, 'lib/weighing/engine.js')));
const { migrateWeighingState } = await import(pathToFileURL(path.join(temporary, 'lib/weighing/migration.js')));
const { beijingDate, filterTasks, filterTickets, exportTasksCsv, exportTicketsCsv, exportSummaryCsv, filterOriginalWeighings, exportOriginalWeighingsCsv, exportOriginalSummaryCsv } = await import(pathToFileURL(path.join(temporary, 'lib/weighing/query.js')));
const now = '2026-10-16T03:00:00.000Z';
const input = (direction = 'inbound', plate = '川A12345') => ({ plate, direction, materialId: direction === 'inbound' ? 'material-1' : 'material-2', partyId: direction === 'inbound' ? 'party-1' : 'party-2', driver: '', remark: '' });
const run = (state, command, at = now) => applyCommand(state, command, at);
const config = (state, patch) => run(state, { type: 'config', patch });
const start = (direction = 'inbound', at = now) => run(createInitialState(at), { type: 'createTask', input: input(direction) }, at);
const confirm = (state, stage, weight, at = now) => {
  const taskId = state.tasks[0].id;
  return run(run(config(state, { weight }), { type: 'capture' }, at), { type: 'confirm', taskId, stage, checked: true }, at);
};
const completed = (direction = 'inbound') => confirm(confirm(start(direction), 'first', direction === 'inbound' ? 32 : 12), 'second', direction === 'inbound' ? 12 : 32);
const legacyKgState = (source) => {
  const state = structuredClone(source); state.version = 2;
  const record = (item) => { item.weight *= 1000; if (item.capture) item.capture.weight *= 1000; };
  for (const task of state.tasks) task.records.forEach(record);
  for (const ticket of state.tickets) {
    ticket.gross *= 1000; ticket.tare *= 1000; ticket.net *= 1000; ticket.unit = 'kg'; ticket.records.forEach(record);
    if (ticket.adjustment) {
      for (const values of [ticket.adjustment.original, ticket.adjustment.before, ticket.adjustment.after]) { values.gross *= 1000; values.tare *= 1000; values.net *= 1000; }
      ticket.adjustment.grossDeduction *= 1000; ticket.adjustment.tareDeduction *= 1000;
    }
  }
  state.config.weight *= 1000; state.materials.forEach((item) => { item.unit = 'kg'; });
  return state;
};

test('入厂先毛后皮、出厂先皮后毛；阶段确认幂等且不修改旧状态', () => {
  for (const direction of ['inbound', 'outbound']) {
    const initial = start(direction);
    const before = structuredClone(initial);
    const once = confirm(initial, 'first', direction === 'inbound' ? 32 : 12);
    assert.deepEqual(initial, before);
    assert.equal(once.tasks[0].records[0].kind, direction === 'inbound' ? 'gross' : 'tare');
    assert.equal(once.tasks[0].status, 'second');
    const ready = confirm(once, 'second', direction === 'inbound' ? 12 : 32);
    assert.equal(ready.tasks[0].status, 'completed');
    const ticket = ready.tickets[0];
    assert.deepEqual([ticket.gross, ticket.tare, ticket.net], [32, 12, 20]);
    const repeated = run(ready, { type: 'confirm', taskId: initial.tasks[0].id, stage: 'second', checked: true });
    assert.equal(repeated.tickets.filter((item) => item.taskId === initial.tasks[0].id).length, 1);
    assert.deepEqual(repeated.tasks[0].records, ready.tasks[0].records);
  }
});

test('同车未结束占用、非法状态转换、非正净重被拒绝，其他车辆可以办理', () => {
  const state = start();
  assert.throws(() => run(state, { type: 'createTask', input: input() }), /未结束/);
  assert.doesNotThrow(() => run(state, { type: 'createTask', input: input('outbound', '川B12345') }));
  assert.throws(() => confirm(state, 'second', 12), /状态/);
  const first = confirm(state, 'first', 12);
  assert.throws(() => confirm(first, 'second', 32), /净重/);
  assert.equal(first.tasks[0].status, 'second');
  assert.equal(first.tasks[0].records.length, 1);
});

test('失稳、超时、设备、证据、断网及异常重量阻止确认，恢复仍须重采和人工核对', () => {
  const state = run(start(), { type: 'capture' });
  for (const patch of [{ stable: false }, { timedOut: true }, { deviceError: true }, { evidence: false }, { online: false }, { weight: 0 }, { weight: NaN }, { weight: 30.0001 }]) {
    const invalid = config(state, patch);
    assert.throws(() => run(invalid, { type: 'confirm', taskId: invalid.tasks[0].id, stage: 'first', checked: true }));
    assert.throws(() => run(invalid, { type: 'capture' }));
  }
  const restored = config(config(state, { stable: false }), { stable: true });
  assert.throws(() => run(restored, { type: 'confirm', taskId: restored.tasks[0].id, stage: 'first', checked: true }), /重新采集/);
  assert.equal(config(state, { submission: 'unknown-success' }).sampleReady, false);
  assert.equal(config(state, { printOutcome: 'unknown' }).sampleReady, false);
  assert.throws(() => run(state, { type: 'confirm', taskId: state.tasks[0].id, stage: 'first', checked: false }), /人工核对/);
});

test('结果不明两种分支先查询，成功回到原单，失败重采，不自动补传', () => {
  for (const submission of ['unknown-success', 'unknown-failure']) {
    const first = confirm(start(), 'first', 32);
    const unknown = confirm(config(first, { submission }), 'second', 12);
    const taskId = unknown.tasks[0].id;
    assert.equal(unknown.pending.committed, submission === 'unknown-success');
    assert.throws(() => run(unknown, { type: 'confirm', taskId, stage: 'second', checked: true }), /先查询/);
    assert.throws(() => run(config(unknown, { online: false }), { type: 'resolveSubmission' }), /恢复连接/);
    const resolved = run(unknown, { type: 'resolveSubmission' });
    assert.equal(resolved.pending, undefined);
    assert.equal(resolved.sampleReady, false);
    if (submission === 'unknown-failure') assert.throws(() => run(resolved, { type: 'confirm', taskId, stage: 'second', checked: true }), /重新采集/);
    const finished = submission === 'unknown-success' ? run(resolved, { type: 'confirm', taskId, stage: 'second', checked: true }) : confirm(resolved, 'second', 12);
    assert.equal(finished.tickets.filter((ticket) => ticket.taskId === taskId).length, 1);
  }
});

test('主管重称与纠错保留原记录，装载变化、方向变化拒绝；终止释放车牌且不可恢复', () => {
  const first = confirm(start(), 'first', 32);
  const taskId = first.tasks[0].id;
  assert.throws(() => run(first, { type: 'terminate', taskId, reason: '取消' }), /主管/);
  let supervisor = run(config(first, { role: 'supervisor', weight: 31 }), { type: 'capture' });
  assert.throws(() => run(supervisor, { type: 'reweigh', taskId, reason: '复核', sameLoad: false, checked: true }), /装载/);
  supervisor = run(supervisor, { type: 'reweigh', taskId, reason: '复核', sameLoad: true, checked: true });
  assert.equal(supervisor.tasks[0].records.length, 2);
  assert.equal(supervisor.tasks[0].records[0].weight, 32);
  assert.equal(supervisor.tasks[0].records[1].replaces, supervisor.tasks[0].records[0].id);
  assert.throws(() => run(supervisor, { type: 'correctTask', taskId, input: input('outbound'), reason: '改方向' }), /锁定/);
  supervisor = run(supervisor, { type: 'correctTask', taskId, input: { ...input(), driver: '张三' }, reason: '司机录入修正' });
  assert.equal(supervisor.tasks[0].driver, '张三');
  const stopped = run(supervisor, { type: 'terminate', taskId, reason: '取消运输' });
  assert.throws(() => run(stopped, { type: 'confirm', taskId, stage: 'first', checked: true }), /终止/);
  assert.doesNotThrow(() => run(stopped, { type: 'createTask', input: input() }));
});

test('作废关联重开保留重量与原快照，同任务唯一有效单，重开链不分叉', () => {
  const state = config(completed(), { role: 'supervisor' });
  const old = structuredClone(state.tickets[0]);
  const command = { type: 'reopenTicket', ticketId: old.id, input: { ...input(), plate: '川A54321' }, reason: '车牌录错', evidenceValid: true };
  assert.throws(() => run(state, command), /先作废/);
  const voided = run(state, { type: 'voidTicket', ticketId: old.id, reason: '车牌录错' });
  assert.equal(voided.tasks[0].status, 'completed');
  assert.throws(() => run(voided, { ...command, evidenceValid: false }), /依据/);
  const reopened = run(voided, command);
  const next = reopened.tickets[0];
  assert.equal(next.previousId, old.id);
  assert.equal(next.plate, '川A54321');
  assert.deepEqual(next.records, old.records);
  assert.deepEqual([next.gross, next.tare, next.net], [old.gross, old.tare, old.net]);
  assert.equal(reopened.tickets.find((ticket) => ticket.id === old.id).plate, old.plate);
  assert.equal(reopened.tickets.filter((ticket) => ticket.taskId === old.taskId && ticket.status === 'valid').length, 1);
  assert.throws(() => run(reopened, command), /有效测试单/);
  const nextVoided = run(reopened, { type: 'voidTicket', ticketId: next.id, reason: '再核对' });
  assert.throws(() => run(nextVoided, command), /已有更正重开/);
});

test('北京时间跨天需要说明，疑点交主管核查，无法确认可终止', () => {
  const evening = '2026-10-15T15:59:00.000Z'; // 北京时间 23:59
  const morning = '2026-10-15T16:01:00.000Z'; // 北京时间次日 00:01
  const first = confirm(start('inbound', evening), 'first', 32, evening);
  const taskId = first.tasks[0].id;
  assert.throws(() => confirm(first, 'second', 12, morning), /跨天/);
  assert.throws(() => run(first, { type: 'verifyDay', taskId, reason: '', doubtful: false, confirmed: true }, morning), /说明/);
  const ordinary = run(first, { type: 'verifyDay', taskId, reason: '仍为昨日原车原运输，已核对首次记录', doubtful: false, confirmed: true }, morning);
  assert.equal(confirm(ordinary, 'second', 12, morning).tasks[0].status, 'completed');
  const doubt = run(first, { type: 'verifyDay', taskId, reason: '装载情况不明', doubtful: true, confirmed: false }, morning);
  assert.throws(() => confirm(doubt, 'second', 12, morning), /疑点/);
  assert.throws(() => run(doubt, { type: 'verifyDay', taskId, reason: '确认', doubtful: false, confirmed: true }, morning), /主管/);
  const resolved = run(config(doubt, { role: 'supervisor' }), { type: 'verifyDay', taskId, reason: '核实仍属原运输', doubtful: false, confirmed: true }, morning);
  assert.equal(confirm(resolved, 'second', 12, morning).tasks[0].status, 'completed');
});

test('岗位、占用和接管入口检查，管理员只维护资料且引用和快照受保护', () => {
  const state = start();
  const occupied = run(state, { type: 'occupy', owner: 'other' });
  assert.throws(() => run(occupied, { type: 'capture' }), /占用/);
  assert.throws(() => run(occupied, { type: 'occupy', owner: null }), /接管/);
  assert.throws(() => run(occupied, { type: 'takeover', reason: '交班' }), /主管/);
  const takeover = run(config(occupied, { role: 'supervisor' }), { type: 'takeover', reason: '交班' });
  assert.equal(takeover.owner, 'self');
  const admin = config(takeover, { role: 'admin' });
  assert.throws(() => run(admin, { type: 'capture' }), /岗位/);
  assert.throws(() => run(state, { type: 'saveMaterial', name: '小麦', enabled: true }), /管理员/);
  assert.throws(() => run(admin, { type: 'deleteMaterial', id: 'material-1' }), /引用/);
  assert.throws(() => run(admin, { type: 'deleteParty', id: 'party-1' }), /引用/);
  const renamed = run(admin, { type: 'saveMaterial', id: 'material-1', name: '新名称', enabled: false });
  assert.equal(renamed.tasks[0].materialName, '酿酒高粱');
  assert.equal(renamed.tickets[0].materialName, '酿酒高粱');
  assert.throws(() => run(config(renamed, { role: 'operator' }), { type: 'createTask', input: input('inbound', '川C00001') }), /启用/);
});

test('打印未知先核实、明确失败可重试、已出纸补打须主管原因授权且沿原号', () => {
  let state = completed();
  const ticketId = state.tickets[0].id;
  state = run(config(state, { printOutcome: 'unknown' }), { type: 'previewPrint', ticketId });
  assert.equal(state.tickets[0].printStatus, 'unknown');
  assert.throws(() => run(state, { type: 'previewPrint', ticketId }), /先由主管核实/);
  assert.throws(() => run(config(state, { role: 'supervisor' }), { type: 'authorizeReprint', ticketId, reason: '重打' }), /先核实/);
  state = run(config(state, { role: 'supervisor' }), { type: 'resolvePrint', ticketId, printed: false, reason: '确认未出纸' });
  state = run(config(state, { printOutcome: 'failed' }), { type: 'previewPrint', ticketId });
  assert.equal(state.tickets[0].reprintCount, 0);
  state = run(config(state, { printOutcome: 'printed' }), { type: 'previewPrint', ticketId });
  assert.throws(() => run(state, { type: 'previewPrint', ticketId }), /授权补打/);
  assert.throws(() => run(state, { type: 'authorizeReprint', ticketId, reason: '' }), /原因/);
  state = run(state, { type: 'authorizeReprint', ticketId, reason: '原联污损' });
  state = run(config(state, { role: 'operator' }), { type: 'previewPrint', ticketId });
  assert.equal(state.tickets[0].id, ticketId);
  assert.equal(state.tickets[0].reprintCount, 1);
  assert.equal(state.tickets[0].reprintAuthorized, false);
});

test('重置恢复示例状态，仅返回本模块新数据', () => {
  assert.throws(() => run(completed(), { type: 'reset' }), /管理员/);
  const reset = run(config(completed(), { role: 'admin' }), { type: 'reset' });
  assert.deepEqual(reset, createInitialState(now));
  assert.ok(reset.tasks.every((task) => task.example));
  assert.ok(reset.tickets.some((ticket) => ticket.status === 'void'));
  const crossDay = reset.tasks.find((task) => task.id === 'TEST-T-007');
  assert.equal(crossDay.status, 'second');
  assert.ok(beijingDate(crossDay.records[0].at) < beijingDate(now));
  assert.match(crossDay.remark, /跨天待核实/);
});

test('查询按北京时间分别使用创建与出单日期，CSV 保留测试单位并正确转义', () => {
  const state = completed();
  const task = { ...state.tasks[0], createdAt: '2026-10-15T15:59:00Z', remark: '逗号,双引号"\n第二行' };
  const ticket = { ...state.tickets[0], issuedAt: '2026-10-15T16:01:00Z', remark: task.remark };
  assert.equal(beijingDate(task.createdAt), '2026-10-15');
  assert.equal(beijingDate(ticket.issuedAt), '2026-10-16');
  assert.equal(filterTasks([task], { from: '2026-10-16' }).length, 0);
  assert.equal(filterTickets([ticket], { from: '2026-10-16', to: '2026-10-16' }).length, 1);
  assert.equal(filterTickets([ticket], { to: '2026-10-15' }).length, 0);
  assert.equal(filterTasks([task], { keyword: task.plate.toLowerCase(), direction: 'inbound', materialId: task.materialId, partyId: task.partyId, status: 'completed' }).length, 1);
  assert.equal(filterTickets([ticket], { keyword: ticket.taskId, status: 'void' }).length, 0);
  for (const csv of [exportTasksCsv([task]), exportTicketsCsv([ticket])]) {
    assert.ok(csv.startsWith('\uFEFF'));
    assert.ok(csv.includes('"逗号,双引号""\n第二行"'));
    assert.ok(csv.includes('"测试"'));
    assert.ok(csv.includes('"吨"'));
  }
});

test('CSV 汇总遵循筛选结果且只计有效单，作废与更正重开不重复计重', () => {
  const state = config(completed(), { role: 'supervisor' });
  const original = state.tickets[0];
  const reopened = run(run(state, { type: 'voidTicket', ticketId: original.id, reason: '信息修正' }), { type: 'reopenTicket', ticketId: original.id, input: input(), reason: '信息修正', evidenceValid: true });
  const selected = filterTickets(reopened.tickets, { keyword: original.taskId });
  assert.equal(selected.length, 2);
  const summary = exportSummaryCsv(selected);
  assert.ok(summary.includes('"1","20","吨"'));
  assert.equal(summary.split('\r\n').length, 2);
  assert.equal(exportSummaryCsv(filterTickets(selected, { status: 'void' })).split('\r\n').length, 1);
});

test('首次一次确认原子建任务，两方向及请求幂等保持原重量，失败不留待首次任务', () => {
  for (const direction of ['inbound', 'outbound']) {
    const initial = createInitialState(now);
    const command = { type: 'firstWeigh', input: input(direction), checked: true, requestId: 'first-request' };
    assert.throws(() => run(initial, command), /重新采集/);
    const collected = run(config(initial, { weight: direction === 'inbound' ? 32 : 12 }), { type: 'capture' });
    assert.throws(() => run(collected, { ...command, checked: false }), /人工核对/);
    assert.throws(() => run(collected, { ...command, input: { ...input(direction), materialId: '' } }), /物料/);
    assert.equal(initial.tasks.length, 7);
    const first = run(collected, command);
    const task = first.tasks[0];
    assert.equal(task.status, 'second');
    assert.equal(first.tasks.length, initial.tasks.length + 1);
    assert.equal(task.records.length, 1);
    assert.equal(task.records[0].kind, direction === 'inbound' ? 'gross' : 'tare');
    assert.equal(first.firstRequests['first-request'], task.id);
    assert.equal(run(first, command).tasks.length, first.tasks.length);
    assert.equal(confirm(first, 'second', direction === 'inbound' ? 12 : 32).tickets[0].net, 20);
    // 页面未先清草稿时，切换车牌也必须取得新请求编号，避免返回上一辆车。
    const withDraftRequest = { ...first, draftRequestId: command.requestId };
    const nextDraft = run(withDraftRequest, { type: 'draft', patch: { plate: '川B54321' } });
    assert.notEqual(nextDraft.draftRequestId, command.requestId);
    const nextTask = run(run(nextDraft, { type: 'capture' }), { type: 'firstWeigh', input: nextDraft.draft, checked: true, requestId: nextDraft.draftRequestId });
    assert.equal(nextTask.tasks[0].plate, '川B54321');
    assert.notEqual(nextTask.tasks[0].id, task.id);
  }
});

test('首次结果不明成功返回原任务，未知失败无任务，查询后保留草稿再采集', () => {
  for (const submission of ['unknown-success', 'unknown-failure']) {
    let state = run(createInitialState(now), { type: 'draft', patch: { ...input(), driverPhone: '13800000123' } });
    const requestId = state.draftRequestId;
    state = run(config(state, { submission }), { type: 'capture' });
    const command = { type: 'firstWeigh', input: state.draft, checked: true, requestId };
    const unknown = run(state, command);
    assert.equal(unknown.tasks.length, state.tasks.length + (submission === 'unknown-success' ? 1 : 0));
    assert.equal(unknown.pending.requestId, requestId);
    assert.equal(unknown.pending.plate, input().plate);
    assert.throws(() => run(unknown, command), /先查询/);
    assert.throws(() => run(unknown, { type: 'clearDraft' }), /先查询/);
    assert.throws(() => run(unknown, { type: 'draft', patch: { plate: '川B54321' } }), /先查询/);
    const resolved = run(unknown, { type: 'resolveSubmission' });
    assert.equal(resolved.draft.driverPhone, '13800000123');
    assert.equal(resolved.pending, undefined);
    if (submission === 'unknown-success') {
      assert.equal(run(resolved, command).tasks[0].id, resolved.tasks[0].id);
    } else {
      assert.equal(unknown.pending.taskId, '');
      assert.equal(resolved.firstRequests[requestId], undefined);
      assert.throws(() => run(resolved, command), /重新采集/);
      const retried = run(run(resolved, { type: 'capture' }), command);
      assert.equal(retried.tasks.length, state.tasks.length + 1);
    }
  }
});

test('资料编码唯一只读、同名允许、分类引用保护，多身份和默认联系人校验', () => {
  let admin = config(createInitialState(now), { role: 'admin' });
  admin = run(admin, { type: 'saveMaterial', name: '酿酒高粱', enabled: true, categoryId: 'category-1', spec: '二级' });
  const material = admin.materials[admin.materials.length - 1];
  admin = run(admin, { type: 'saveMaterial', id: material.id, name: '酿酒高粱', enabled: false, spec: '二级改名', code: '不能改' });
  assert.equal(admin.materials.find((item) => item.id === material.id).code, material.code);
  assert.equal(new Set(admin.materials.map((item) => item.code)).size, admin.materials.length);
  assert.throws(() => run(admin, { type: 'deleteCategory', id: 'category-1' }), /引用/);
  admin = run(admin, { type: 'saveParty', name: '示例粮食供应商', enabled: true, nature: 'individual', roles: ['supplier', 'customer', 'carrier'], contacts: [{ id: 'new-contact', name: '张三', phone: '13800000123', purpose: '收货', remark: '', isDefault: true }] });
  const party = admin.parties[admin.parties.length - 1];
  assert.equal(party.nature, 'individual');
  assert.deepEqual(party.roles, ['supplier', 'customer', 'carrier']);
  assert.equal(party.contacts[0].isDefault, true);
  assert.equal(new Set(admin.parties.map((item) => item.code)).size, admin.parties.length);
  assert.throws(() => run(admin, { type: 'saveParty', id: party.id, name: party.name, enabled: true, contacts: [{ ...party.contacts[0], isDefault: false }] }), /默认联系人/);
  assert.throws(() => run(admin, { type: 'saveParty', id: party.id, name: party.name, enabled: true, roles: [] }), /至少/);
  assert.throws(() => run(admin, { type: 'saveParty', id: 'party-1', name: '改身份', enabled: true, roles: ['customer'] }), /未结束/);
  assert.throws(() => run(admin, { type: 'saveParty', id: 'party-3', name: '改承运', enabled: true, roles: ['customer'] }), /承运商身份/);
  assert.throws(() => run(admin, { type: 'deleteParty', id: 'party-3' }), /引用/);
});

test('承运、联系人、司机和材料规格快照贯通任务及更正重开，改资料不改历史', () => {
  const taskInput = { ...input(), carrierId: 'party-3', contactName: '业务张', contactPhone: '13800000111', driver: '司机李', driverPhone: '13800000222' };
  let state = run(run(createInitialState(now), { type: 'capture' }), { type: 'firstWeigh', input: taskInput, checked: true, requestId: 'snapshot-request' });
  const task = structuredClone(state.tasks[0]);
  assert.equal(task.carrierName, '示例运输合作社');
  assert.equal(task.materialSpec, '一级');
  state = config(state, { role: 'admin' });
  state = run(state, { type: 'saveParty', id: 'party-3', name: '承运新名', enabled: false });
  state = run(state, { type: 'saveMaterial', id: 'material-1', name: '物料新名', spec: '新规格', enabled: false });
  assert.equal(state.tasks[0].carrierName, task.carrierName);
  state = confirm(config(state, { role: 'operator' }), 'second', 12);
  const original = structuredClone(state.tickets[0]);
  for (const key of ['carrierName', 'carrierCode', 'contactName', 'contactPhone', 'driverPhone', 'materialSpec', 'materialCode', 'partyCode']) assert.equal(original[key], task[key]);
  for (const csv of [exportTasksCsv([state.tasks[0]]), exportTicketsCsv([original])]) {
    for (const value of ['M0001', '一级', 'B0001', '示例运输合作社', 'B0003', '业务张', '13800000111', '司机李', '13800000222']) assert.ok(csv.includes(`"${value}"`), `导出应包含历史快照 ${value}`);
    assert.ok(!csv.includes('承运新名'));
    assert.ok(!csv.includes('新规格'));
  }
  const snapshotSummary = exportSummaryCsv([original]);
  assert.ok(snapshotSummary.includes('"酿酒高粱","M0001","一级"'));
  state = run(config(state, { role: 'supervisor' }), { type: 'voidTicket', ticketId: original.id, reason: '修正司机' });
  state = run(state, { type: 'reopenTicket', ticketId: original.id, input: { ...taskInput, driverPhone: '13800000333' }, reason: '司机号码录错', evidenceValid: true });
  assert.equal(state.tickets[0].driverPhone, '13800000333');
  assert.equal(state.tickets[0].materialSpec, task.materialSpec);
  assert.equal(state.tickets[0].carrierName, task.carrierName);
  assert.equal(state.tickets.find((item) => item.id === original.id).driverPhone, '13800000222');
  assert.deepEqual(state.tickets[0].records, original.records);
});

test('旧缓存升级保留称重和历史，不以当前资料反填旧快照；草稿及请求去重可序列化恢复', () => {
  const legacy = legacyKgState(createInitialState(now));
  legacy.version = 1;
  delete legacy.categories; delete legacy.draft; delete legacy.draftRequestId; delete legacy.firstRequests;
  legacy.materials = legacy.materials.map(({ id, name, enabled }) => ({ id, name, enabled }));
  legacy.parties = legacy.parties.slice(0, 2).map(({ id, name, enabled, roles }) => ({ id, name, enabled, kind: roles[0] }));
  const addedFields = ['materialCode', 'materialSpec', 'materialCategoryName', 'partyCode', 'partyNature', 'partyPhone', 'partyAddress', 'carrierId', 'carrierName', 'carrierCode', 'contactName', 'contactPhone', 'driverPhone'];
  for (const item of [...legacy.tasks, ...legacy.tickets]) for (const key of addedFields) delete item[key];
  for (const item of [...legacy.tasks, ...legacy.tickets]) for (const record of item.records) delete record.capture;
  const before = structuredClone(legacy);
  const upgraded = migrateWeighingState(legacy, now);
  assert.deepEqual(legacy, before);
  assert.equal(upgraded.version, 3);
  assert.deepEqual(upgraded.parties[0].roles, ['supplier']);
  assert.ok(upgraded.materials.every((item) => item.code));
  assert.equal(upgraded.tasks[0].materialCode, '');
  assert.equal(upgraded.tasks[0].carrierName, '');
  assert.equal(upgraded.tasks[1].records[0].weight, 32);
  assert.equal(upgraded.tasks[1].records[0].sourceWeightKg, 32000);
  assert.deepEqual(upgraded.tickets[0].history, legacy.tickets[0].history);
  assert.equal(upgraded.tickets[0].gross, 32);
  assert.equal(upgraded.tickets[0].sourceKg.gross, 32000);
  assert.ok(upgraded.tasks.every((task) => task.records.every((record) => !record.capture)));
  let draft = run(upgraded, { type: 'draft', patch: { ...input(), contactName: '保留草稿' } });
  const requestId = draft.draftRequestId;
  draft = migrateWeighingState(JSON.parse(JSON.stringify(draft)), now);
  assert.equal(draft.draft.contactName, '保留草稿');
  assert.equal(draft.draftRequestId, requestId);
  draft = run(run(draft, { type: 'capture' }), { type: 'firstWeigh', input: draft.draft, checked: true, requestId });
  const restored = migrateWeighingState(JSON.parse(JSON.stringify(draft)), now);
  assert.equal(restored.firstRequests[requestId], draft.tasks[0].id);
  const next = run(restored, { type: 'clearDraft' });
  assert.equal(next.draft.plate, '');
  assert.notEqual(next.draftRequestId, requestId);
  assert.equal(next.sampleReady, false);
});

test('模拟抓拍绑定采集时间、重量、视角和当前车牌，不能确认其他车辆的图片', () => {
  const capturedAt = '2026-10-16T03:01:00.000Z';
  const confirmedAt = '2026-10-16T03:02:00.000Z';
  const draft = run(createInitialState(now), { type: 'draft', patch: input() });
  const collected = run(config(draft, { cameraPreset: 'rear', weight: 32 }), { type: 'capture', plate: '川A12345' }, capturedAt);
  assert.deepEqual({ preset: collected.capture.preset, imageSrc: collected.capture.imageSrc, cameraLabel: collected.capture.cameraLabel, at: collected.capture.at, weight: collected.capture.weight, plate: collected.capture.plate }, { preset: 'rear', imageSrc: '/demo-camera/truck-rear.png', cameraLabel: '车尾视角', at: capturedAt, weight: 32, plate: '川A12345' });
  const command = { type: 'firstWeigh', input: input(), checked: true, requestId: draft.draftRequestId };
  const confirmed = run(collected, command, confirmedAt);
  const record = confirmed.tasks[0].records[0];
  assert.equal(record.capture.id, collected.capture.id);
  assert.equal(record.capture.at, capturedAt);
  assert.equal(record.at, confirmedAt);
  assert.equal(record.capture.plate, confirmed.tasks[0].plate);
  assert.equal(record.capture.weight, record.weight);
  assert.equal(confirmed.capture, undefined);
  assert.equal(confirmed.sampleReady, false);
  const wrongPlate = run(config(draft, { weight: 32 }), { type: 'capture', plate: '川B54321' });
  assert.throws(() => run(wrongPlate, command), /其他车牌/);
  assert.throws(() => run({ ...collected, capture: undefined }, command), /抓拍缺失/);
  assert.throws(() => run({ ...collected, capture: { ...collected.capture, weight: 123 } }, command), /重量不一致/);
  const selected = run(createInitialState(now), { type: 'capture', taskId: 'TEST-T-001', plate: '川B54321' });
  assert.equal(selected.capture.plate, '川A·示例01');
  const unbound = run(createInitialState(now), { type: 'capture' });
  assert.equal(unbound.capture.plate, undefined);
  const legacy = run(unbound, { type: 'confirm', taskId: 'TEST-T-006', stage: 'first', checked: true });
  assert.equal(legacy.tasks.find((task) => task.id === 'TEST-T-006').records[0].capture.plate, '川A·示例06');
});

test('换图、重新采集和重称不覆盖旧图，作废重开保留原记录图片', () => {
  let state = confirm(start(), 'first', 32);
  const taskId = state.tasks[0].id;
  const firstCapture = structuredClone(state.tasks[0].records[0].capture);
  state = config(state, { role: 'supervisor', cameraPreset: 'rear', weight: 31 });
  assert.equal(state.capture, undefined);
  state = run(state, { type: 'capture', taskId });
  const replacementCapture = structuredClone(state.capture);
  state = run(state, { type: 'reweigh', taskId, reason: '复核同装载条件', sameLoad: true, checked: true });
  assert.deepEqual(state.tasks[0].records[0].capture, firstCapture);
  assert.equal(state.tasks[0].records[1].capture.imageSrc, '/demo-camera/truck-rear.png');
  assert.equal(state.tasks[0].records[1].capture.id, replacementCapture.id);
  assert.notEqual(firstCapture.id, replacementCapture.id);
  state = confirm(state, 'second', 12);
  const ticket = structuredClone(state.tickets[0]);
  assert.deepEqual(ticket.records[0].capture, firstCapture);
  state = run(state, { type: 'voidTicket', ticketId: ticket.id, reason: '更正车牌' });
  state = run(state, { type: 'reopenTicket', ticketId: ticket.id, input: input('inbound', '川C98765'), reason: '更正车牌', evidenceValid: true });
  assert.equal(state.tickets[0].plate, '川C98765');
  assert.deepEqual(state.tickets[0].records, ticket.records);
  assert.equal(state.tickets[0].records[0].capture.plate, '川A12345');
});

test('配置、草稿、接管与未知失败清当前采集；缓存恢复保历史图且无图历史不反填', () => {
  const captured = run(createInitialState(now), { type: 'capture' });
  assert.equal(run(captured, { type: 'config', patch: { cameraPreset: 'rear' } }).capture, undefined);
  assert.equal(run(captured, { type: 'draft', patch: { driver: '新司机' } }).capture, undefined);
  assert.equal(run(captured, { type: 'occupy', owner: 'other' }).capture, undefined);
  const supervisor = run(config(captured, { role: 'supervisor' }), { type: 'capture' });
  assert.equal(run(supervisor, { type: 'takeover', reason: '交接班' }).capture, undefined);
  const failure = run(run(config(createInitialState(now), { submission: 'unknown-failure' }), { type: 'capture', plate: input().plate }), { type: 'firstWeigh', input: input(), checked: true, requestId: 'unknown-image' });
  assert.equal(failure.capture, undefined);
  const restored = migrateWeighingState(JSON.parse(JSON.stringify(captured)), now);
  assert.equal(restored.capture, undefined);
  assert.equal(restored.sampleReady, false);
  assert.deepEqual(restored.tasks[1].records[0].capture, captured.tasks[1].records[0].capture);
  for (const version of [1, 2]) {
    const noImages = legacyKgState(captured); noImages.version = version;
    for (const task of noImages.tasks) for (const record of task.records) delete record.capture;
    for (const ticket of noImages.tickets) for (const record of ticket.records) delete record.capture;
    const migrated = migrateWeighingState(noImages, now);
    assert.ok(migrated.tasks.every((task) => task.records.every((record) => !record.capture)));
    assert.ok(migrated.tickets.every((ticket) => ticket.records.every((record) => !record.capture)));
  }
});

const completeWithFinalWeights = (direction, finalWeights, submission = 'normal') => {
  const first = confirm(start(direction), 'first', direction === 'inbound' ? 30 : 10);
  const ready = run(config(first, { weight: direction === 'inbound' ? 10 : 30, submission }), { type: 'capture', taskId: first.tasks[0].id });
  return run(ready, { type: 'confirm', taskId: ready.tasks[0].id, stage: 'second', checked: true, finalWeights, actor: '测试操作员甲' });
};

test('两方向毛皮独立扣减保留测量与证据，原因选填且记录身份、时间和前后值，确认幂等', () => {
  for (const direction of ['inbound', 'outbound']) {
    const state = completeWithFinalWeights(direction, { gross: 29.8, tare: 9.9 });
    const ticket = state.tickets[0];
    assert.deepEqual([ticket.gross, ticket.tare, ticket.net], [29.8, 9.9, 19.9]);
    assert.deepEqual(ticket.adjustment.original, { gross: 30, tare: 10, net: 20 });
    assert.deepEqual(ticket.adjustment.before, ticket.adjustment.original);
    assert.deepEqual(ticket.adjustment.after, { gross: 29.8, tare: 9.9, net: 19.9 });
    assert.equal(ticket.adjustment.actor, '测试操作员甲'); assert.equal(ticket.adjustment.at, now);
    assert.equal(ticket.adjustment.role, 'operator'); assert.equal(ticket.adjustment.reason, undefined);
    assert.deepEqual(ticket.records, state.tasks[0].records);
    assert.deepEqual(ticket.records.map(r => r.weight), direction === 'inbound' ? [30, 10] : [10, 30]);
    assert.ok(ticket.records.every(r => r.capture.weight === r.weight));
    const repeated = run(state, { type: 'confirm', taskId: ticket.taskId, stage: 'second', checked: true, finalWeights: { gross: 29.1, tare: 10 } });
    assert.deepEqual(repeated.tickets, state.tickets);
  }
  assert.equal(completeWithFinalWeights('inbound', { gross: 30, tare: 9.9 }).tickets[0].net, 20.1);
});

test('非法扣减与原始异常不能出单，失败不修改状态；保留小数输入精度', () => {
  const first = confirm(start(), 'first', 30);
  const ready = run(config(first, { weight: 10 }), { type: 'capture' });
  const before = structuredClone(ready);
  for (const finalWeights of [
    { gross: -0.001, tare: 10 }, { gross: NaN, tare: 10 },
    { gross: Infinity, tare: 10 }, { gross: 30, tare: 0 },
    { gross: 30.001, tare: 10 }, { gross: 10, tare: 10 }, { gross: 20, tare: 10.001 }, { gross: 29.9999, tare: 10 },
  ]) assert.throws(() => run(ready, { type: 'confirm', taskId: ready.tasks[0].id, stage: 'second', checked: true, finalWeights }));
  assert.deepEqual(ready, before);
  const bad = run(config(first, { weight: 31 }), { type: 'capture' });
  assert.throws(() => run(bad, { type: 'confirm', taskId: bad.tasks[0].id, stage: 'second', checked: true, finalWeights: { gross: 30, tare: 10 } }), /原始净重/);
  const decimalFirst = confirm(start(), 'first', 30.001);
  const decimalReady = run(config(decimalFirst, { weight: 10.001 }), { type: 'capture' });
  const decimal = run(decimalReady, { type: 'confirm', taskId: decimalReady.tasks[0].id, stage: 'second', checked: true, finalWeights: { gross: 30.001, tare: 10.001 } }).tickets[0];
  assert.deepEqual([decimal.gross, decimal.tare, decimal.net], [30.001, 10.001, 20]);
});

test('未知提交恢复保持原扣减，失败结果仍须重采', () => {
  for (const submission of ['unknown-success', 'unknown-failure']) {
    const pending = completeWithFinalWeights('inbound', { gross: 29.8, tare: 9.9 }, submission);
    assert.ok(pending.pending);
    const resolved = run(pending, { type: 'resolveSubmission' });
    const tickets = resolved.tickets.filter(t => t.taskId === resolved.tasks[0].id);
    assert.equal(tickets.length, submission === 'unknown-success' ? 1 : 0);
    if (tickets.length) assert.equal(tickets[0].net, 19.9);
    else assert.equal(resolved.sampleReady, false);
  }
});

test('主管重开保留旧扣减与证据，原始列表不增行，独立日期与双汇总不重复计重', () => {
  let state = completeWithFinalWeights('inbound', { gross: 29.8, tare: 9.9 });
  const source = structuredClone(state.tickets[0]); const oldId = source.id;
  assert.throws(() => run(state, { type: 'reopenTicket', ticketId: oldId, input: input(), reason: '修正', evidenceValid: true }), /主管/);
  state = config(state, { role: 'supervisor' });
  state = run(state, { type: 'voidTicket', ticketId: oldId, reason: '扣减填错' });
  assert.equal(exportOriginalSummaryCsv(filterOriginalWeighings(state.tasks, state.tickets, { keyword: source.taskId })).split('\r\n').length, 1);
  const nextDay = '2026-10-17T03:00:00.000Z';
  state = run(state, { type: 'reopenTicket', ticketId: oldId, input: input(), reason: '核实后重开', evidenceValid: true, finalWeights: { gross: 29.7, tare: 9.9 }, actor: '主管乙' }, nextDay);
  const current = state.tickets[0]; assert.equal(current.net, 19.8);
  assert.deepEqual(current.adjustment.before, source.adjustment.after);
  assert.equal(current.adjustment.actor, '主管乙'); assert.equal(current.adjustment.at, nextDay);
  assert.deepEqual(current.records, source.records);
  assert.deepEqual(state.tickets.find(t => t.id === oldId).adjustment, source.adjustment);
  const rows = filterOriginalWeighings(state.tasks, state.tickets, { keyword: source.taskId });
  assert.equal(rows.length, 1); assert.equal(rows[0].original.net, 20); assert.equal(rows[0].validTicketId, current.id);
  assert.match(exportOriginalSummaryCsv(rows), /原始净重合计/); assert.match(exportOriginalSummaryCsv(rows), /"1","20","吨"/);
  assert.match(exportSummaryCsv(state.tickets.filter(t => t.taskId === source.taskId)), /"1","19.8","吨"/);
  assert.equal(filterOriginalWeighings(state.tasks, state.tickets, { keyword: source.taskId, from: '2026-10-16', to: '2026-10-16' }).length, 1);
  assert.equal(filterOriginalWeighings(state.tasks, state.tickets, { keyword: source.taskId, from: '2026-10-17' }).length, 0);
  assert.deepEqual(filterTickets(state.tickets, { keyword: source.taskId, from: '2026-10-17' }).map(t => t.id), [current.id]);
  assert.match(exportOriginalWeighingsCsv(rows), /"30","10","20"/);
  assert.match(exportTicketsCsv([current]), /最终毛重/); assert.match(exportTicketsCsv([current]), /"29\.7","9\.9","19\.8"/);
});

test('零扣减旧缓存继续可查且不伪造审计，新扣减快照序列化恢复', () => {
  const legacy = legacyKgState(createInitialState(now));
  const migrated = migrateWeighingState(JSON.parse(JSON.stringify(legacy)), now);
  assert.equal(migrated.version, 3); assert.ok(migrated.tickets.every(t => !t.adjustment));
  assert.equal(migrated.tickets[0].unit, '吨'); assert.equal(migrated.materials[0].unit, '吨');
  assert.equal(migrated.tickets[0].gross, 32); assert.equal(migrated.tickets[0].sourceKg.gross, 32000);
  assert.equal(migrateWeighingState(JSON.parse(JSON.stringify(migrated)), now).tickets[0].gross, 32);
  const rows = filterOriginalWeighings(migrated.tasks, migrated.tickets, {});
  assert.equal(rows.length, 2); assert.ok(rows.every(row => row.original.net === 20));
  const adjusted = completeWithFinalWeights('outbound', { gross: 29.8, tare: 9.9 });
  const restored = migrateWeighingState(JSON.parse(JSON.stringify(adjusted)), now);
  assert.deepEqual(restored.tickets, adjusted.tickets); assert.deepEqual(restored.tasks, adjusted.tasks);
  const oldAdjusted = legacyKgState(completeWithFinalWeights('inbound', { gross: 29.8, tare: 9.9 }));
  const migratedAdjustment = migrateWeighingState(JSON.parse(JSON.stringify(oldAdjusted)), now);
  assert.deepEqual([migratedAdjustment.tickets[0].gross, migratedAdjustment.tickets[0].tare, migratedAdjustment.tickets[0].net], [29.8, 9.9, 19.9]);
  assert.deepEqual([migratedAdjustment.tickets[0].adjustment.grossDeduction, migratedAdjustment.tickets[0].adjustment.tareDeduction], [0.2, 0.1]);
  assert.deepEqual(migratedAdjustment.tickets[0].adjustment.original.sourceKg, { gross: 30000, tare: 10000, net: 20000 });
  assert.equal(migrateWeighingState(JSON.parse(JSON.stringify(migratedAdjustment)), now).tickets[0].gross, 29.8);
  assert.equal(formatWeightValue(32), '32');
  assert.equal(formatWeightValue(5.25), '5.25');
  assert.equal(formatWeightValue(5.001), '5.001');
  assert.equal(formatWeightValue(0), '0');
  assert.equal(formatWeight(5), '5 吨');
  assert.equal(formatWeightInputValue(5.25), '5.25');
  assert.equal(formatWeightInputValue(5), '5');
  const roundedToZero = migrateWeighingState(legacyKgState(createInitialState(now)), now);
  const tinySource = { gross: 10000, tare: 9999.6, net: 0.4 };
  const roundedOriginal = { ...weightValues(10, 10), sourceKg: tinySource };
  assert.equal(needsLegacyWeightReview(roundedOriginal), true);
  assert.equal(needsLegacyWeightReview({ ...weightValues(9.999, 10), sourceKg: { gross: 9999.6, tare: 10000, net: -0.4 } }), true);
  assert.equal(roundedToZero.version, 3);
  const oldUnitV3 = structuredClone(createInitialState(now));
  oldUnitV3.materials.forEach(item => { item.unit = 't'; });
  oldUnitV3.tickets.forEach(item => { item.unit = 't'; });
  const normalizedUnits = migrateWeighingState(oldUnitV3, now);
  assert.ok(normalizedUnits.materials.every(item => item.unit === '吨'));
  assert.ok(normalizedUnits.tickets.every(item => item.unit === '吨'));
});
