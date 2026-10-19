import type { BusinessInfo, FinalWeightInput, HistoryEntry, TaskInput, TestTicket, TransportTask, WeighRecord, WeighingCommand, WeighingState, WeightValues } from '../../types/weighing';
import { emptyTaskInput, roleLabels } from '../../types/weighing';
import { createInitialState } from './seed';
const siteConfigKeys = ['weight', 'stable', 'timedOut', 'deviceError', 'online', 'evidence', 'cameraPreset', 'scenePlate', 'recognitionCandidate', 'recognitionScenario', 'submission', 'printOutcome'] as const;
import { getCameraPreset } from './camera';
import { adjustedWeights, originalWeights, weightDeduction, weightInMilliTonnes } from './weights';

const beijingDay = (at: string) => new Date(Date.parse(at) + 8 * 3600_000).toISOString().slice(0, 10);
const active = (task: TransportTask) => task.status === 'first' || task.status === 'second';
const latest = (task: TransportTask, stage: WeighRecord['stage']) => {
  const records = task.records.filter((record) => record.stage === stage);
  return records[records.length - 1];
};

/** 所有模拟业务修改共用此入口，返回新状态，失败不改写原状态。 */
export function applyCommand(previous: WeighingState, command: WeighingCommand, now = new Date().toISOString()): WeighingState {
  const state = structuredClone(previous);
  const role = state.config.role;
  const invalidateSample = () => { state.sampleReady = false; delete state.capture; };
  function fail(message: string): never { throw new Error(message); }
  const requireReason = (reason: string) => reason.trim() || fail('请填写操作原因或核实说明');
  const requireDaily = () => { if (role === 'admin') fail('请切换操作员或主管演示岗位办理日常业务'); };
  const requireAdmin = () => { if (role !== 'admin') fail('维护测试资料需要管理员演示岗位'); };
  const requireSettled = () => { if (state.pending) fail('上次提交结果不明，请先查询提交结果'); };
  const requireSite = () => {
    requireDaily(); requireSettled();
    if (state.owner === 'other') fail('现场正由其他演示会话占用，请操作员接管后操作');
    state.owner = 'self';
  };
  const log = (action: string, reason?: string): HistoryEntry => ({ at: now, role, actor: command.actor?.trim() || `演示${roleLabels[role]}`, action, ...(reason ? { reason } : {}) });
  const id = (prefix: string) => `TEST-${prefix}-${++state.sequence}`;
  const findTask = (taskId: string) => state.tasks.find((task) => task.id === taskId) ?? fail('测试任务不存在');
  const findTicket = (ticketId: string) => state.tickets.find((ticket) => ticket.id === ticketId) ?? fail('测试单据不存在');
  const requireActive = (task: TransportTask) => { if (!active(task)) fail('已完成或已终止的任务不能继续称重或更正'); };
  const checkPlate = (plate: string, exceptId?: string) => {
    if (state.tasks.some((task) => task.id !== exceptId && active(task) && task.plate === plate)) fail('该车牌已有未结束任务，请先办理原任务');
  };
  const businessInfo = (input: TaskInput, existing?: BusinessInfo): BusinessInfo => {
    const plate = input.plate.trim().toUpperCase();
    if (!plate) fail('请填写车牌');
    if (input.direction !== 'inbound' && input.direction !== 'outbound') fail('请选择运输方向');
    const material = state.materials.find((entry) => entry.id === input.materialId);
    const party = state.parties.find((entry) => entry.id === input.partyId);
    if (!material || (!material.enabled && material.id !== existing?.materialId)) fail('请选择已启用的物料');
    if (!party || (!party.enabled && party.id !== existing?.partyId)) fail('请选择已启用的往来主体');
    if (!party.roles.includes(input.direction === 'inbound' ? 'supplier' : 'customer')) fail('入厂卸货请选择供方，出厂装货请选择收方');
    const carrierId = input.carrierId?.trim() ?? existing?.carrierId ?? '';
    const carrier = carrierId ? state.parties.find((entry) => entry.id === carrierId) : undefined;
    if (carrierId && (!carrier || (!carrier.enabled && carrier.id !== existing?.carrierId) || !carrier.roles.includes('carrier'))) fail('请选择已启用且具有承运商身份的主体');
    const sameMaterial = existing?.materialId === material.id;
    const sameParty = existing?.partyId === party.id;
    const sameCarrier = existing?.carrierId === carrierId;
    return { ...input, plate, driver: input.driver.trim(), remark: input.remark.trim(),
      materialName: sameMaterial ? existing.materialName : material.name,
      materialCode: sameMaterial ? existing.materialCode ?? '' : material.code,
      materialSpec: sameMaterial ? existing.materialSpec ?? '' : material.spec,
      materialCategoryName: sameMaterial ? existing.materialCategoryName ?? '' : state.categories.find((item) => item.id === material.categoryId)?.name ?? '',
      partyName: sameParty ? existing.partyName : party.name, partyCode: sameParty ? existing.partyCode ?? '' : party.code,
      partyNature: sameParty ? existing.partyNature : party.nature,
      partyPhone: sameParty ? existing.partyPhone ?? '' : party.phone, partyAddress: sameParty ? existing.partyAddress ?? '' : party.address,
      carrierId, carrierName: sameCarrier ? existing?.carrierName ?? '' : carrier?.name ?? '', carrierCode: sameCarrier ? existing?.carrierCode ?? '' : carrier?.code ?? '',
      contactName: input.contactName?.trim() ?? existing?.contactName ?? '', contactPhone: input.contactPhone?.trim() ?? existing?.contactPhone ?? '',
      driverPhone: input.driverPhone?.trim() ?? existing?.driverPhone ?? '',
    };
  };
  const requireConditions = () => {
    const config = state.config;
    if (!config.online) fail('演示已断网，暂停称重确认');
    if (config.deviceError) fail('模拟设备异常，请恢复后重新采集');
    if (!config.stable) fail('模拟重量失稳，请恢复稳定后重新采集');
    if (config.timedOut) fail('模拟数据已超时，请重新采集');
    if (!config.evidence) fail('模拟证据失败，请恢复后重新采集；摄像头未真实接入');
    if (!Number.isFinite(config.weight) || config.weight <= 0) fail('模拟重量必须为大于零的有效数字');
    weightInMilliTonnes(config.weight);
  };
  const requireSample = (checked: boolean, plate?: string) => {
    requireConditions();
    if (!state.sampleReady) fail('请重新采集模拟重量');
    if (!state.capture?.imageSrc || state.capture.weight !== state.config.weight) fail('本次模拟抓拍缺失或与重量不一致，请重新采集');
    if (plate && state.capture.plate && state.capture.plate !== plate.trim().toUpperCase()) fail('本次模拟抓拍关联的是其他车牌，请为当前车辆重新采集');
    if (!checked) fail('请先人工核对车牌、业务信息及模拟称重依据');
  };
  const checkDay = (task: TransportTask) => {
    if (task.crossDayDoubt) fail('原运输存在疑点，请操作员核实车辆与任务，无法确认时终止');
  };
  const makeRecord = (task: TransportTask, stage: WeighRecord['stage'], reason?: string): WeighRecord => {
    const capture = state.capture;
    if (!capture) fail('本次模拟抓拍缺失，请重新采集');
    if (capture.plate && capture.plate !== task.plate) fail('本次模拟抓拍关联的是其他车牌，请为当前车辆重新采集');
    return { id: id('W'), stage, kind: (task.direction === 'inbound') === (stage === 'first') ? 'gross' : 'tare',
      weight: state.config.weight, at: now, actor: command.actor?.trim() || `演示${roleLabels[role]}`, role, evidence: 'simulated', capture: { ...structuredClone(capture), plate: task.plate }, ...(reason ? { reason } : {}),
    };
  };
  const adjustment = (original: WeightValues, input: FinalWeightInput, actor?: string, before = original) => {
    const after = adjustedWeights(original, input);
    return {
    ...weightDeduction(original, after, input.reason?.trim() || (after.gross !== original.gross || after.tare !== original.tare ? '磅精度调整' : undefined)),
    source: after.gross !== original.gross || after.tare !== original.tare ? 'manual' as const : 'collected' as const,
    actor: actor?.trim() || `演示${roleLabels[role]}`, role, at: now,
    original: { ...original }, before: { ...before }, after,
  }; };
  const issueTicket = (task: TransportTask, input: FinalWeightInput, actor?: string): TestTicket => {
    if (state.tickets.some((ticket) => ticket.taskId === task.id && ticket.status === 'valid')) fail('该任务已有有效测试单');
    const original = originalWeights(task.records) ?? fail('缺少原始毛重或皮重');
    const snapshot = adjustment(original, input, actor);
    const { id: taskId, createdAt: _createdAt, status: _status, records: _records, history: _history, verifiedDate: _verifiedDate, crossDayDoubt: _crossDayDoubt, example: _example, ...info } = task;
    void _createdAt; void _status; void _records; void _history; void _verifiedDate; void _crossDayDoubt; void _example;
    return { ...info, id: id('P'), taskId, issuedAt: now,
      status: 'valid', ...snapshot.after, adjustment: snapshot, unit: '吨', records: structuredClone(task.records), history: [log('生成测试过磅单并保存扣减快照', snapshot.reason)], printStatus: 'unprinted', reprintCount: 0, reprintAuthorized: false };
  };

  switch (command.type) {
    case 'reset': requireAdmin(); return { ...createInitialState(now), resetAt: now };
    case 'config': {
      // 配置面板是所有演示使用者的场景控制，不修改真实账号权限。
      if (siteConfigKeys.some(key => key in command.patch && !Object.is(state.config[key], command.patch[key]))) invalidateSample();
      state.config = { ...state.config, ...command.patch };
      return state;
    }
    case 'capture': {
      requireSite(); requireConditions(); state.sampleReady = true;
      const preset = getCameraPreset(state.config.cameraPreset);
      const plate = (command.taskId ? findTask(command.taskId).plate : command.plate ?? state.draft.plate).trim().toUpperCase();
      state.capture = { id: id('C'), preset: preset.id, imageSrc: preset.src, cameraLabel: preset.title, at: now, weight: state.config.weight, ...(plate ? { plate } : {}), ...((command.plateEvidence ?? (state.draftPlateEvidence?.confirmedPlate === plate ? state.draftPlateEvidence : undefined)) ? { plateEvidence: structuredClone(command.plateEvidence ?? state.draftPlateEvidence) } : {}) };
      state.history.push(log('采集模拟重量与抓拍图片', preset.title)); return state;
    }
    case 'draft': {
      requireSettled();
      const plateChanged = command.patch.plate !== undefined && command.patch.plate.trim().toUpperCase() !== state.draft.plate.trim().toUpperCase();
      if (plateChanged && state.firstRequests[state.draftRequestId]) state.draftRequestId = id('FIRST');
      state.draft = { ...state.draft, ...command.patch };
      // 同车允许先采集再补业务资料；改车牌必须重新采集现场依据。
      if (plateChanged) { invalidateSample(); delete state.draftPlateEvidence; }
      return state;
    }
    case 'verifyPlate':
      requireSettled(); state.draftPlateEvidence = structuredClone(command.evidence); return state;
    case 'clearDraft':
      requireSettled(); state.draft = emptyTaskInput(); delete state.draftPlateEvidence; state.draftRequestId = id('FIRST'); invalidateSample(); return state;
    case 'firstWeigh': {
      requireSite();
      if (!command.requestId.trim()) fail('首次称重请求编号不能为空');
      if (state.firstRequests[command.requestId]) return state;
      requireSample(command.checked, command.input.plate);
      const info = businessInfo(command.input); checkPlate(info.plate);
      const committed = state.config.submission !== 'unknown-failure';
      let taskId = '';
      if (committed) {
        const task: TransportTask = { ...info, id: id('T'), createdAt: now, status: 'second', records: [], history: [log('创建任务并确认首次测试称重')] };
        task.records.push(makeRecord(task, 'first')); taskId = task.id;
        state.tasks.unshift(task); state.firstRequests[command.requestId] = taskId;
      }
      state.draft = { ...command.input }; invalidateSample();
      if (committed) delete state.draftPlateEvidence;
      if (state.config.submission !== 'normal') {
        state.pending = { taskId, stage: 'first', committed, requestId: command.requestId, plate: info.plate };
        state.history.push(log('模拟首次称重提交结果不明，等待查询'));
      }
      return state;
    }
    case 'createTask': {
      requireSite();
      const info = businessInfo(command.input); checkPlate(info.plate);
      state.tasks.unshift({ ...info, id: id('T'), createdAt: now, status: 'first', records: [], history: [log('创建测试任务')] });
      invalidateSample(); return state;
    }
    case 'confirm': {
      requireSite();
      const task = findTask(command.taskId);
      if (task.status === 'terminated') fail('已终止任务不可恢复或继续称重');
      // 已确认的同一阶段直接返回原结果，不依赖再次采集，更不重复出单。
      if (latest(task, command.stage)) return state;
      requireActive(task);
      if (task.status !== command.stage) fail('当前任务状态不允许此阶段确认');
      if (command.stage === 'second') checkDay(task);
      requireSample(command.checked, task.plate);
      const record = makeRecord(task, command.stage);
      if (command.stage === 'second') {
        const candidate = [...task.records, record];
        const original = originalWeights(candidate) ?? fail('缺少原始称重依据');
        adjustedWeights(original, command.finalWeights ?? { gross: original.gross, tare: original.tare });
      }
      const committed = state.config.submission !== 'unknown-failure';
      if (committed) {
        task.records.push(record);
        if (command.stage === 'second') {
          const original = originalWeights(task.records) ?? fail('缺少原始称重依据');
          const first = latest(task, 'first');
          if (first && beijingDay(first.at) !== beijingDay(now)) task.history.push(log(`隔日续称，沿用首次记录 ${first.id}（${first.at}），本次记录 ${record.id}（${now}）`));
          state.tickets.unshift(issueTicket(task, command.finalWeights ?? { gross: original.gross, tare: original.tare }, command.actor)); task.status = 'completed';
        } else task.status = 'second';
        task.history.push(log(`确认测试${command.stage === 'first' ? '首次' : '二次'}称重`));
      }
      invalidateSample();
      if (committed) delete state.draftPlateEvidence;
      if (state.config.submission !== 'normal') {
        state.pending = { taskId: task.id, stage: command.stage, committed };
        state.history.push(log('模拟提交结果不明，等待查询'));
      }
      return state;
    }
    case 'startPrint': fail('打印发起由服务端处理'); return state;
    case 'retrySubmission': fail('原请求重试由服务端处理'); return state;
    case 'resolveSubmission': {
      requireDaily();
      if (!state.config.online) fail('请恢复连接后再查询提交结果');
      if (!state.pending) fail('没有待核实的提交');
      const pending = state.pending;
      const entry = log(pending.committed ? '查询提交结果：已成功，沿用原称重结果' : '查询提交结果：未成功，请重新核对和采集');
      if (pending.taskId) findTask(pending.taskId).history.push(entry);
      else state.history.push(entry);
      delete state.pending; invalidateSample(); state.config.submission = 'normal';
      return state;
    }
    case 'correctTask': {
      requireDaily(); requireSite();
      const reason = requireReason(command.reason); const task = findTask(command.taskId); requireActive(task);
      if (task.records.length && command.input.direction !== task.direction) fail('首次称重后运输方向已锁定');
      const info = businessInfo(command.input, task); checkPlate(info.plate, task.id);
      const summary = (value: BusinessInfo) => `${value.plate} / ${value.direction === 'inbound' ? '入厂卸货' : '出厂装货'} / ${value.materialName} ${value.materialSpec || ''}(${value.materialCode || ''}) / ${value.partyName}(${value.partyCode || ''}) / 承运：${value.carrierName || '未填写'}(${value.carrierCode || ''}) / 联系人：${value.contactName || '未填写'} ${value.contactPhone || ''} / 司机：${value.driver || '未填写'} ${value.driverPhone || ''} / 备注：${value.remark || '无'}`;
      task.history.push(log(`更正任务信息：${summary(task)} → ${summary(info)}`, reason));
      Object.assign(task, info); invalidateSample(); return state;
    }
    case 'reweigh': {
      requireDaily(); requireSite();
      const task = findTask(command.taskId);
      const stage = command.stage ?? 'first';
      if (command.captureId && task.records.some(record => record.capture?.id === command.captureId)) return state;
      if (task.status === 'terminated') fail('已终止任务不可重称');
      if (state.tickets.some(ticket => ticket.taskId === task.id && ticket.status === 'valid')) fail('请先作废有效单再办理单磅重称');
      const reason = requireReason(command.reason);
      const original = latest(task, stage) ?? fail('目标称重尚未确认，请在工作台采集确认');
      const other = latest(task, stage === 'first' ? 'second' : 'first');
      if (!command.sameLoad) fail('装载状态已改变，不能替代原称重结果');
      if (other && !command.otherValid) fail('请确认沿用的另一磅仍有效');
      checkDay(task); requireSample(command.checked, task.plate);
      if (command.captureId && command.captureId !== state.capture?.id) fail('采集依据已变化，请重新核对');
      const record = makeRecord(task, stage, reason); record.replaces = original.id;
      if (other) record.retainedRecordId = other.id;
      const committed = state.config.submission !== 'unknown-failure';
      if (committed) {
        task.records.push(record);
        task.history.push(log(`单磅重称 ${record.id} 替代 ${original.id}：${original.weight} → ${record.weight} 吨${other ? `，沿用另一磅 ${other.id}` : ''}`, reason));
      }
      if (state.config.submission !== 'normal') state.pending = { taskId: task.id, stage, committed, operation: 'reweigh' };
      invalidateSample(); return state;
    }
    case 'terminate': {
      requireDaily(); requireSite();
      const task = findTask(command.taskId); requireActive(task);
      task.history.push(log('终止测试任务，不可恢复', requireReason(command.reason))); task.status = 'terminated';
      invalidateSample(); return state;
    }
    case 'verifyDay': {
      requireSite();
      const task = findTask(command.taskId); requireActive(task);
      const reason = command.reason.trim();
      if (task.status !== 'second') fail('请先完成首次称重再办理续称核实');
      if (task.crossDayDoubt || command.doubtful) {
        if (command.confirmed) requireDaily();
        task.crossDayDoubt = !command.confirmed;
      }
      if (command.confirmed) {
        task.crossDayDoubt = false; task.verifiedDate = beijingDay(now);
      } else delete task.verifiedDate;
      task.history.push(log(command.confirmed ? '核实仍属原运输，可继续测试称重' : '原运输有疑点，待操作员核实或终止', reason));
      invalidateSample(); return state;
    }
    case 'voidTicket': {
      requireDaily(); requireSettled();
      const ticket = findTicket(command.ticketId);
      if (ticket.status !== 'valid') fail('此测试单已经作废');
      const entry = log('作废测试单据，任务保持完成', requireReason(command.reason));
      ticket.status = 'void'; ticket.reprintAuthorized = false; ticket.history.push(entry); findTask(ticket.taskId).history.push(entry);
      return state;
    }
    case 'reopenTicket': {
      requireDaily(); requireSettled();
      const old = findTicket(command.ticketId); const reason = requireReason(command.reason);
      const successor = state.tickets.find(ticket => ticket.previousId === old.id);
      if (successor?.status === 'valid') return state;
      if (old.status !== 'void') fail('请先作废原测试单，再关联更正重开');
      if (!command.evidenceValid) fail('需核验原称重依据仍有效；无效依据不能更正重开');
      if (state.tickets.some((ticket) => ticket.taskId === old.taskId && ticket.status === 'valid')) fail('该任务已有有效测试单，不能重复重开');
      if (state.tickets.some((ticket) => ticket.previousId === old.id)) fail('该测试单已有更正重开记录，请沿最新单据处理');
      if (command.input.direction !== old.direction) fail('更正重开不能改变原称重方向和重量语义');
      const info = businessInfo(command.input, old);
      const task = findTask(old.taskId);
      if (task.status !== 'completed') fail('仅可重开已完成任务');
      const original = originalWeights(task.records) ?? fail('缺少原始称重依据，不能更正重开');
      const snapshot = adjustment(original, command.finalWeights ?? { gross: old.gross, tare: old.tare }, command.actor, { gross: old.gross, tare: old.tare, net: old.net });
      const newTicket: TestTicket = { ...structuredClone(old), ...info, id: id('P'), issuedAt: now, status: 'valid', previousId: old.id,
        ...snapshot.after, adjustment: snapshot, records: structuredClone(task.records),
        history: [log(`关联原测试单 ${old.id} 更正重开`, reason)], printStatus: 'unprinted', reprintCount: 0, reprintAuthorized: false, example: false };
      if (state.config.submission === 'unknown-failure') {
        state.pending = { taskId: task.id, stage: 'second', committed: false, operation: 'reopen' };
        return state;
      }
      old.history.push(log(`关联更正重开为 ${newTicket.id}`, reason));
      findTask(old.taskId).history.push(log(`更正重开测试单 ${newTicket.id}`, reason));
      Object.assign(task, info);
      state.tickets.unshift(newTicket);
      if (state.config.submission === 'unknown-success') state.pending = { taskId: task.id, stage: 'second', committed: true, operation: 'reopen' };
      return state;
    }
    case 'previewPrint': {
      requireDaily(); requireSettled(); const ticket = findTicket(command.ticketId);
      if (ticket.status !== 'valid') fail('作废测试单只能查看历史，不能办理打印');
      if (ticket.printStatus === 'unknown') fail('上次打印结果未知，请先由操作员核实，不得自动重打');
      if (ticket.printStatus === 'printed' && !ticket.reprintAuthorized) fail('已打印测试单需要操作员填写原因后补打');
      const reprint = ticket.reprintAuthorized;
      if (reprint) ticket.reprintCount += 1;
      ticket.reprintAuthorized = false; ticket.printStatus = state.config.printOutcome;
      ticket.history.push(log(reprint ? `测试补打预览，第 ${ticket.reprintCount} 次` : '测试首次打印预览／明确失败重试'));
      return state;
    }
    case 'resolvePrint': {
      requireDaily(); const ticket = findTicket(command.ticketId);
      if (ticket.printStatus !== 'unknown') fail('此测试单没有待核实的打印结果');
      const reason = requireReason(command.reason);
      ticket.printStatus = command.printed ? 'printed' : 'failed';
      ticket.history.push(log(command.printed ? '核实已出纸或不能排除已出纸，后续需授权补打' : '核实明确失败且未出纸，可演示重试', reason));
      return state;
    }
    case 'authorizeReprint': {
      requireDaily(); const ticket = findTicket(command.ticketId);
      if (ticket.status !== 'valid') fail('作废测试单不能授权补打');
      if (ticket.printStatus === 'unknown') fail('请先核实未知打印结果，再办理补打授权');
      if (ticket.printStatus !== 'printed') fail('首次打印或明确未出纸的失败重试无需补打授权');
      ticket.history.push(log('授权测试单补打', requireReason(command.reason))); ticket.reprintAuthorized = true; return state;
    }
    case 'occupy':
      requireDaily(); requireSettled();
      if (state.owner === 'other' && command.owner !== 'other') fail('其他会话已占用现场，请操作员接管');
      state.owner = command.owner; invalidateSample(); state.history.push(log(`演示操作占用：${command.owner ?? '空闲'}`)); return state;
    case 'takeover':
      requireDaily(); requireSettled();
      state.history.push(log('操作员接管现场演示会话', requireReason(command.reason))); state.owner = 'self'; invalidateSample(); return state;
    case 'saveCategory': {
      requireAdmin(); const name = command.name.trim(); if (!name) fail('请填写分类名称');
      if (state.categories.some((item) => item.id !== command.id && item.name === name)) fail('分类名称已经存在');
      if (command.id) Object.assign(state.categories.find((item) => item.id === command.id) ?? fail('分类不存在'), { name, enabled: command.enabled });
      else state.categories.push({ id: id('C'), name, enabled: command.enabled });
      state.history.push(log('保存测试物料分类', name)); return state;
    }
    case 'deleteCategory':
      requireAdmin();
      if (!state.categories.some((item) => item.id === command.id)) fail('分类不存在');
      if (state.materials.some((item) => item.categoryId === command.id)) fail('分类已被引用，不能删除，可改为停用');
      state.categories = state.categories.filter((item) => item.id !== command.id); state.history.push(log('删除未引用分类')); return state;
    case 'saveMaterial': {
      requireAdmin(); const name = command.name.trim(); if (!name) fail('请填写物料名称');
      const existing = command.id ? state.materials.find((item) => item.id === command.id) ?? fail('物料不存在') : undefined;
      const categoryId = command.categoryId ?? existing?.categoryId ?? '';
      const category = state.categories.find((item) => item.id === categoryId);
      if (categoryId && (!category || (!category.enabled && categoryId !== existing?.categoryId))) fail('请选择已启用的分类');
      const values = { name, enabled: command.enabled, categoryId, spec: command.spec?.trim() ?? existing?.spec ?? '', remark: command.remark?.trim() ?? existing?.remark ?? '' };
      if (existing) Object.assign(existing, values);
      else { const itemId = id('M'); state.materials.push({ id: itemId, code: itemId.replace('TEST-', ''), ...values, unit: '吨' }); }
      state.history.push(log('保存测试物料', name)); return state;
    }
    case 'saveParty': {
      requireAdmin(); const name = command.name.trim(); if (!name) fail('请填写往来主体名称');
      const existing = command.id ? state.parties.find((item) => item.id === command.id) ?? fail('往来主体不存在') : undefined;
      const roles = [...new Set(command.roles ?? (command.kind ? [command.kind] : existing?.roles ?? []))];
      if (!roles.length || roles.some((item) => !['supplier', 'customer', 'carrier'].includes(item))) fail('请至少选择一项业务身份');
      const nature = command.nature ?? existing?.nature ?? 'enterprise';
      if (nature !== 'enterprise' && nature !== 'individual') fail('请选择企业或个人主体性质');
      const contacts = structuredClone(command.contacts ?? existing?.contacts ?? []);
      if (contacts.some((contact) => !contact.name.trim())) fail('请填写联系人姓名');
      if (contacts.length && contacts.filter((contact) => contact.isDefault).length !== 1) fail('存在联系人时必须指定且仅指定一个默认联系人');
      if (new Set(contacts.map((contact) => contact.id)).size !== contacts.length) fail('联系人编号不能重复');
      if (existing) for (const task of state.tasks.filter(active)) {
        if (task.partyId === existing.id && !roles.includes(task.direction === 'inbound' ? 'supplier' : 'customer')) fail('未结束任务仍需该供货商／客户身份，不能移除');
        if (task.carrierId === existing.id && !roles.includes('carrier')) fail('未结束任务仍需该承运商身份，不能移除');
      }
      const values = { name, enabled: command.enabled, nature, roles, contacts,
        shortName: command.shortName?.trim() ?? existing?.shortName ?? '', phone: command.phone?.trim() ?? existing?.phone ?? '',
        address: command.address?.trim() ?? existing?.address ?? '', creditCode: command.creditCode?.trim() ?? existing?.creditCode ?? '', remark: command.remark?.trim() ?? existing?.remark ?? '' };
      if (existing) Object.assign(existing, values);
      else { const itemId = id('B'); state.parties.push({ id: itemId, code: itemId.replace('TEST-', ''), ...values }); }
      state.history.push(log('保存测试往来主体', name)); return state;
    }
    case 'deleteMaterial':
      requireAdmin();
      if (!state.materials.some((item) => item.id === command.id)) fail('物料不存在');
      if (state.tasks.some((task) => task.materialId === command.id) || state.tickets.some((ticket) => ticket.materialId === command.id)) fail('物料已被引用，不能删除，可改为停用');
      state.materials = state.materials.filter((item) => item.id !== command.id); state.history.push(log('删除未引用测试物料')); return state;
    case 'deleteParty':
      requireAdmin();
      if (!state.parties.some((item) => item.id === command.id)) fail('往来主体不存在');
      if (state.tasks.some((task) => task.partyId === command.id || task.carrierId === command.id) || state.tickets.some((ticket) => ticket.partyId === command.id || ticket.carrierId === command.id)) fail('往来主体已被引用，不能删除，可改为停用');
      state.parties = state.parties.filter((item) => item.id !== command.id); state.history.push(log('删除未引用测试往来主体')); return state;
  }
}
