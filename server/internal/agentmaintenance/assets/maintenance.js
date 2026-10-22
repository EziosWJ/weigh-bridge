const $ = (id) => document.getElementById(id);
const esc = (value) => String(value ?? '').replace(/[&<>"']/g, (c) => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
}[c]));
const pages = {
  overview: ['运行总览', '查看现场服务状态，定位连接与打印异常。'],
  printing: ['打印与队列', '检查本机队列与未结束作业，按影响范围执行维护操作。'],
  devices: ['现场设备', '查看仪表实时状态与摄像头接入能力。'],
  config: ['连接、仪表与打印配置', '核对运行参数，保存草稿并明确应用。'],
  records: ['维护记录与日志', '按时间和操作查询本机维护记录，查看运行日志。'],
};
const fields = [
  ['serverUrl', 'Server 地址', 'url', null, null, '例如：https://server.example'],
  ['agentId', 'Agent 编号', 'text', null, null, '应与 Server 登记的 Agent 身份一致'],
  ['scaleId', '地磅编号', 'number', 1, null, '应与 Server 的地磅映射一致'],
  ['agentKey', '连接密钥（留空保持原值）', 'password', null, null, '密钥不回显，仅在需要替换时填写'],
  ['queue', '已有 CUPS 队列', 'text', null, null, '选择已经安装的本机打印队列'],
  ['printWidthMm', '打印宽度 (mm)', 'number', 100, 210, '缩放前基准宽度 100～210 mm；缩放后宽度加右移不超过 210 mm'],
  ['printScale', '整体缩放 (倍)', 'number', 0.5, 1.5, '0.5～1.5 倍；文字、表格及行高统一缩放，偏移和表长不缩放'],
  ['offsetXmm', '向右偏移 (mm)', 'number', 0, 30, '允许 0～30 mm'],
  ['offsetYmm', '向下偏移 (mm)', 'number', 0, 30, '允许 0～30 mm'],
  ['formLengthMm', '连续纸表长 (mm)', 'number', 85, 300, '允许 85～300 mm，支持小数；量相邻横向撕纸线间距，不直接按 99 mm 推定'],
  ['serialEnabled', '启用柯力 D12 仪表', 'checkbox', null, null, '关闭时保持原打印能力；启用后由打印 Agent 独占串口'],
  ['serialPort', '串口路径', 'text', null, null, '例如 /dev/ttyS0；不得与诊断工具同时占用'],
  ['serialBaud', '仪表波特率', 'number', 1200, 115200, '必须与仪表 bt 一致；现场为 9600'],
  ['serialProtocol', '仪表协议', 'select', null, null, '当前仅支持 D12 tF=05 连续输出', ['keli-d12-tf05']],
  ['serialWeightUnit', '现场源单位', 'select', null, null, '按现场核对结果选择；本机已确认 t，不得依据单位位自动换算', ['t', 'kg']],
  ['serialStaleMs', '无有效帧故障阈值 (ms)', 'number', 200, 2999, '须早于 Server 的 3000 ms 采集保鲜门限'],
];
let cfg = {};
let configData;
function displayField(key, value) {
  const field = fields.find(([name]) => name === key);
  return field?.[2] === 'checkbox' ? (value ? '启用' : '停用') : String(value ?? '');
}
function scaleSummary(c) {
  return c.serialEnabled ? `D12 ${c.serialProtocol}，${c.serialPort}，${c.serialBaud} 8N1，源单位 ${c.serialWeightUnit}，陈旧阈值 ${c.serialStaleMs} ms` : '称重仪表未启用（仅打印）';
}

let lastState;
let auditOffset = 0;
let auditTotal = 0;
let auditRows = 0;
let busy = false;
let authenticated = false;
let hasPrevious = false;
let formDirty = false;
let refreshVersion = 0;
let auditVersion = 0;
let jobsVersion = 0;
let jobsCache = null;
let jobsError = '';
let currentPage = 'overview';

function badge(value) {
  const cls = /运行中|已启用|已连接/.test(value) ? 'ok' : /暂停|停止|断开/.test(value) ? 'bad' : '';
  return `<span class="badge ${cls}">${esc(value)}</span>`;
}
function updateControls() {
  for (const control of document.querySelectorAll('#private button, #private input, #private select')) control.disabled = busy || control.dataset.unavailable === 'true';
  $('restoreConfig').disabled = busy || !hasPrevious;
  $('cancelJobsButton').disabled = busy || !document.querySelector('#jobs input:checked');
  $('auditPrev').disabled = busy || auditOffset === 0;
  $('auditNext').disabled = busy || auditOffset + auditRows >= auditTotal;
}
function setBusy(value) { busy = value; updateControls(); }
function showMessage(message, failed = false) {
  $('operationMessage').textContent = message;
  $('operationMessage').className = `status-message ${failed ? 'bad' : 'ok'}`;
}
function showPage() {
  const target = location.hash.slice(1);
  currentPage = Object.hasOwn(pages, target) ? target : 'overview';
  for (const panel of document.querySelectorAll('[data-page]')) panel.hidden = panel.dataset.page !== currentPage;
  for (const link of document.querySelectorAll('[data-nav]')) {
    if (link.dataset.nav === currentPage) link.setAttribute('aria-current', 'page');
    else link.removeAttribute('aria-current');
  }
  [$('pageTitle').textContent, $('pageDescription').textContent] = pages[currentPage];
  document.title = `${pages[currentPage][0]} - 现场 Agent 维护`;
}
async function visitPage() {
  showPage();
  if (!authenticated) return;
  try {
    if (currentPage === 'printing') await Promise.all([refresh(), loadJobs()]);
    if (currentPage === 'config' && !formDirty) await loadConfig();
    if (currentPage === 'records') await loadAudit(0);
  } catch (error) { showMessage(error.message, true); }
}
async function refreshPage() {
  await refresh();
  if (!authenticated || busy) return;
  try {
    if (currentPage === 'config' && !formDirty) await loadConfig();
    if (currentPage === 'printing') await loadJobs();
  } catch (error) { showMessage(error.message, true); }
}
function setAuthenticated(value) {
  authenticated = value;
  $('auth').classList.toggle('hidden', value);
  $('private').classList.toggle('hidden', !value);
  $('logoutButton').classList.toggle('hidden', !value);
  $('sessionState').textContent = value ? '维护会话已验证' : '只读访问';
}
async function api(path, options = {}, asText = false) {
  const response = await fetch(path, { ...options, headers: { 'Content-Type': 'application/json', ...options.headers } });
  if (response.status === 401) setAuthenticated(false);
  if (!response.ok) throw Error(await response.text());
  return asText ? response.text() : response.json();
}
function renderServices(state) {
  const rows = [
    ['打印 Agent', state.agentKnown ? state.agent : '未知', `<button data-unavailable="${state.agent === '运行中'}" onclick="startAgent()">启动打印 Agent</button><button onclick="guarded('restart-agent')">重启打印 Agent</button>`],
    ['CUPS 打印服务', state.cupsKnown ? state.cups : '未知', '<button class="danger" onclick="guarded(\'restart-cups\')">重启 CUPS</button>'],
  ];
  $('services').innerHTML = `<table><thead><tr><th>服务</th><th>当前状态</th><th>操作</th></tr></thead><tbody>${rows.map(([name, state, actions]) => `<tr><td>${name}</td><td>${badge(state)}</td><td><div class="cell-actions">${actions}</div></td></tr>`).join('')}</tbody></table>`;
}
function renderQueues(state) {
  const queues = state.queueDetails || [];
  $('queueCount').textContent = state.queueKnown ? `共 ${queues.length} 个队列` : '队列状态未知';
  $('queues').innerHTML = !state.queueKnown ? '<p class="bad">无法读取打印队列，请检查 CUPS 服务后刷新。</p>'
    : `<table><thead><tr><th>队列</th><th>状态</th><th>用途</th><th>操作</th></tr></thead><tbody>${queues.map((queue) => `<tr><td>${esc(queue.name)}</td><td>${badge(queue.state)}</td><td>${queue.name === state.queue ? '<span class="badge">当前打印目标</span>' : '其他本机队列'}</td><td>${queue.name === state.queue ? `<button data-unavailable="${queue.state === '已启用'}" onclick="guarded('enable-queue')">恢复目标队列</button>` : '<span class="muted">只读查看</span>'}</td></tr>`).join('') || '<tr><td colspan="4" class="empty">本机尚无打印队列，请先在系统中安装打印机。</td></tr>'}</tbody></table>`;
}
function renderScale(state) {
  const scale = state.scale || {};
  const live = scale.known && (scale.state === '稳定' || scale.state === '动态');
  const bad = /故障|过期|未运行/.test(scale.state || '');
  $('scaleBadge').textContent = scale.state || '未知';
  $('scaleBadge').className = `badge ${live ? 'ok' : bad ? 'bad' : ''}`;
  if (!scale.known) {
    $('scaleSummary').textContent = '暂无仪表状态';
    $('scaleWeight').innerHTML = '<strong>暂无仪表状态</strong><span>等待打印 Agent 写入状态</span>';
    $('scaleMeta').textContent = '';
    $('scaleFault').textContent = scale.fault || '';
    return;
  }
  const stamp = (value) => value && !String(value).startsWith('0001-') ? new Date(value).toLocaleString() : '尚无';
  $('scaleSummary').textContent = scale.enabled ? `源单位 ${scale.unit || '未知'}；最后有效帧：${stamp(scale.lastValidAt)}` : '称重仪表未启用（仅打印）';
  $('scaleWeight').innerHTML = `<strong>${esc(scale.weight || '—')} t</strong><span>${scale.stable ? '仪表稳定' : scale.instrumentStable ? '仪表动态' : '当前不可采集'}</span>`;
  const flags = [`文档单位位 ${scale.unitFlag || '未知'}`, scale.negative ? '负值' : '', scale.anomaly ? '异常位' : '', scale.tareIssue ? `皮重：${scale.tareIssue}` : ''].filter(Boolean).join('；');
  $('scaleMeta').textContent = `串口 ${scale.port}，${scale.baud} 8N1，${scale.protocol}；源读数 ${scale.sourceWeight || '—'}，皮重 ${scale.sourceTare || '—'}，${scale.weightType === 'net' ? '净重' : '毛重'}；校验 ${scale.checksum === 'ok' ? '通过' : '最近失败'}（连续 ${scale.checksumFailures || 0} 帧）；有效 ${scale.framesValid || 0} 帧，无效 ${scale.framesInvalid || 0} 帧，垃圾字节 ${scale.discardedBytes || 0}。${flags ? ' ' + flags + '。' : ''}`;
  $('scaleFault').textContent = scale.fault || '';
}

async function refresh() {
  const version = ++refreshVersion;
  try {
    const state = await api('/api/status');
    if (version !== refreshVersion) return;
    lastState = state;
    const stats = [['维护服务', state.maintenance], ['打印 Agent', state.agentKnown ? state.agent : '未知'], ['云端连接', state.cloud], ['CUPS 服务', state.cupsKnown ? state.cups : '未知'], ['目标队列', state.queueKnown ? `${state.queue}（${state.queueState}）` : '未知']];
    $('stats').innerHTML = stats.map(([label, value]) => `<div class="stat"><div class="muted">${label}</div><strong class="${/运行中|已启用|已连接/.test(value) ? 'ok' : 'bad'}">${esc(value)}</strong></div>`).join('');
    $('checked').textContent = `检查时间：${new Date(state.checkedAt).toLocaleString()}`;
    $('error').textContent = state.lastError === '暂无可读取错误' ? '' : state.lastError;
    $('statusJobs').textContent = !state.jobsKnown ? '未结束作业：未知／不可读取' : state.jobs?.length ? `未结束作业摘要：${state.jobs.map((job) => `${job.id}（纸面结果未知）`).join('、')}` : '未发现未结束作业';
    renderServices(state);
    renderQueues(state);
    renderScale(state);
    updateControls();
  } catch (error) {
    if (version !== refreshVersion) return;
    lastState = undefined;
    $('stats').innerHTML = ['维护服务', '打印 Agent', '云端连接', 'CUPS 服务', '目标队列'].map((label) => `<div class="stat"><div class="muted">${label}</div><strong class="bad">未知</strong></div>`).join('');
    $('checked').textContent = '检查失败，状态未知';
    $('statusJobs').textContent = '未结束作业：未知';
    $('error').textContent = error.message;
    $('services').innerHTML = '<p class="bad">服务状态不可读取，请恢复连接后刷新。</p>';
    $('queues').innerHTML = '<p class="bad">队列状态不可读取，请恢复连接后刷新。</p>';
    $('queueCount').textContent = '状态未知';
    renderScale({ scale: { known: false, state: '未知', fault: '状态不可读取，请恢复连接后刷新。' } });
  }
}
async function login() {
  try {
    await api('/api/login', { method: 'POST', body: JSON.stringify({ password: $('password').value }) });
    $('password').value = '';
    $('msg').textContent = '';
    setAuthenticated(true);
    await loadConfig();
    await visitPage();
  } catch (error) { $('msg').textContent = error.message; }
}
async function logout() {
  try { await api('/api/logout', { method: 'POST' }); location.reload(); }
  catch (error) { showMessage(error.message, true); }
}
function outputGeometry(c) {
  const scale = c.printScale || 1;
  const width = Math.round(Math.round(c.printWidthMm * 180 / 25.4) * scale);
  const height = Math.round(600 * scale);
  const feed = Math.ceil((height + Math.round(c.offsetYmm * 180 / 25.4)) / 24) * 24;
  const form = Math.round(c.formLengthMm * 360 / 25.4);
  const valid = width + Math.round(c.offsetXmm * 180 / 25.4) <= Math.round(210 * 180 / 25.4) && feed * 2 <= form;
  return `内容约 ${(width * 25.4 / 180).toFixed(2)} × ${(height * 25.4 / 180).toFixed(2)} mm，含下移及分带走纸 ${(feed * 25.4 / 180).toFixed(2)} mm，指令表长 ${(form * 25.4 / 360).toFixed(3)} mm${valid ? '' : '；超出纸宽或表长，请减小缩放或偏移'}`;
}
function calibration(c) { return `打印队列 ${c.queue}，基准宽 ${c.printWidthMm} mm，缩放 ${c.printScale || 1} 倍，右移 ${c.offsetXmm} mm，下移 ${c.offsetYmm} mm，表长 ${c.formLengthMm} mm；${outputGeometry(c)}`; }
function updateDraftGeometry() {
  const draft = { ...cfg };
  for (const [key, , type] of fields) if (type === 'number') draft[key] = Number($(`f-${key}`).value);
  $('draftGeometry').textContent = `未应用预估：${outputGeometry(draft)}`;
}
function configDifferences(data) {
  return `<table><thead><tr><th>参数</th><th>agent.env 中的配置</th><th>已保存的草稿</th></tr></thead><tbody>${fields.map(([key, name]) => key === 'agentKey' ? `<tr class="${data.keyChanged ? 'changed' : ''}"><td>连接密钥</td><td>${data.hasActiveAgentKey ? '已设置（不回显）' : '未设置'}</td><td>${data.keyChanged ? '待替换（不回显）' : data.hasAgentKey ? '已设置（不回显）' : '未设置'}</td></tr>` : `<tr class="${data.active[key] !== data.config[key] ? 'changed' : ''}"><td>${name}</td><td>${esc(displayField(key, data.active[key]))}</td><td>${esc(displayField(key, data.config[key]))}${data.active[key] !== data.config[key] ? ' <span class="badge">待应用</span>' : ''}</td></tr>`).join('')}</tbody></table>`;
}
async function loadConfig() {
  const data = await api('/api/config');
  cfg = data.config;
 $('siteIdentity').textContent = cfg.siteName ? `草稿归属：${cfg.siteName}（${cfg.siteCode}） · ${cfg.scaleName || cfg.scaleId}` : '现有配置未记录厂区名称，可导入后台下载的厂区配置。';
  configData = data;
  const queues = lastState?.queues || (await api('/api/status')).queues || [];
  const fieldHTML = ([key, name, type, min, max, help, options]) => {
    let control;
    if (key === 'queue') control = `<select id="f-${key}">${[...new Set([cfg[key], ...queues])].map((value) => `<option>${esc(value)}</option>`).join('')}</select>`;
    else if (type === 'checkbox') control = `<input id="f-${key}" type="checkbox" ${cfg[key] ? 'checked' : ''} aria-describedby="hint-${key}">`;
    else if (type === 'select') control = `<select id="f-${key}" aria-describedby="hint-${key}">${(options || []).map((value) => `<option value="${esc(value)}" ${cfg[key] === value ? 'selected' : ''}>${esc(value)}</option>`).join('')}</select>`;
    else control = `<input id="f-${key}" type="${type}" ${type === 'number' ? `step="${key === 'formLengthMm' || key === 'printScale' ? 'any' : '1'}" min="${min}" ${max === null ? '' : `max="${max}"`}` : ''} ${key === 'agentKey' ? '' : 'required'} value="${key === 'agentKey' ? '' : esc(cfg[key])}" autocomplete="${type === 'password' ? 'new-password' : 'off'}" aria-describedby="hint-${key}">`;
    if (type === 'checkbox') return `<div><label class="check" for="f-${key}"><span>${control} ${name}</span></label><p class="field-help" id="hint-${key}">${help}</p></div>`;
    return `<div><label for="f-${key}">${name}</label>${control}<p class="field-help" id="hint-${key}">${help}</p></div>`;
  };
  $('form').innerHTML = `<fieldset><legend>云端连接与身份</legend><div class="fields-grid">${fields.slice(0, 4).map(fieldHTML).join('')}</div></fieldset><fieldset><legend>打印目标与校准</legend><div class="fields-grid">${fields.slice(4, 10).map(fieldHTML).join('')}</div><p id="draftGeometry" class="muted"></p><p class="field-help">票面位置、每单走纸和撕纸停靠需分别校准。撕纸位置请按打印机说明调整；软件尺寸预估不代表实际纸面已验收。</p></fieldset><fieldset><legend>称重仪表（柯力 D12）</legend><div class="fields-grid">${fields.slice(10).map(fieldHTML).join('')}</div></fieldset>`;
  updateDraftGeometry();
  $('saved').textContent = `${data.hasAgentKey ? '连接密钥已设置。' : '连接密钥尚未设置。'}${data.saved ? '当前无未应用草稿。' : '存在未应用草稿，尚未影响运行配置。'}打印及仪表参数需要应用并重启打印 Agent。`;
  $('activeConfig').textContent = data.environmentKnown ? `配置来源：agent.env\n${data.active.siteName || "厂区名称未配置"}，${data.active.scaleName || "地磅 " + data.active.scaleId}\n${data.active.serverUrl}，${data.active.agentId}\n${calibration(data.active)}。\n仪表：${scaleSummary(data.active)}。` : `agent.env 尚未有效载入，以下为上次配置或初始值，不能视为当前运行值。${data.environmentError || '请保存并应用配置。'}`;
  $('runtimeConfig').textContent = data.runtimeState;
  $('configDiff').innerHTML = configDifferences(data);
  $('calibrationSummary').textContent = calibration(data.active);
  $('listenerSummary').textContent = `当前访问：${location.protocol === 'https:' ? 'HTTPS' : 'HTTP'} ${location.host}。默认监听所有 IPv4 网卡，仅供可信维护局域网使用。`;
  hasPrevious = data.hasPrevious;
  formDirty = false;
  $('draftState').textContent = '';
  updateControls();
}
async function save() {
  if (busy || !$('configForm').reportValidity()) return;
  setBusy(true);
  try {
    const draft = { ...cfg };
    for (const [key, , type] of fields) draft[key] = type === 'checkbox' ? $(`f-${key}`).checked : type === 'number' ? Number($(`f-${key}`).value) : $(`f-${key}`).value;
    await api('/api/config', { method: 'PUT', body: JSON.stringify(draft) });
    showMessage('草稿已保存。点击“应用配置”并确认重启打印 Agent 后生效。');
    await loadConfig();
  } catch (error) { showMessage(error.message, true); }
  finally { setBusy(false); }
}
function openDialog(title, body, actions, wide = false) {
  document.querySelector('dialog')?.close();
  const box = document.createElement('dialog');
  if (wide) box.className = 'wide';
  box.setAttribute('aria-labelledby', 'dialogTitle');
  box.innerHTML = `<div class="dialog-head"><h2 id="dialogTitle">${esc(title)}</h2><button aria-label="关闭弹窗" data-close>关闭</button></div><div class="dialog-body">${body}<p class="dialog-error bad" role="alert"></p></div><div class="dialog-actions">${actions}</div>`;
  box.querySelector('[data-close]').onclick = () => box.close();
  box.addEventListener('close', () => box.remove());
  document.body.append(box);
  box.showModal();
  return box;
}
function confirmAction(title, body, label, danger = false) {
  return new Promise((resolve) => {
    const box = openDialog(title, body, `<button data-abort>取消</button><button class="${danger ? 'danger' : 'primary'}" data-confirm>${esc(label)}</button>`);
    let accepted = false;
    box.querySelector('[data-abort]').onclick = () => box.close();
    box.querySelector('[data-confirm]').onclick = () => { accepted = true; box.close(); };
    box.addEventListener('close', () => resolve(accepted), { once: true });
  });
}
async function act(action, extra = {}) {
  if (busy) return;
  setBusy(true);
  try {
    const result = await api('/api/action', { method: 'POST', body: JSON.stringify({ action, ...extra }) });
    if (result.needsDisposition) { chooseJobs(action, result.previewToken, result.jobs, result.unknown); return; }
    showMessage(result.output || '维护操作已完成，已核对执行结果。纸面结果仍需现场核实。');
    if (action === 'apply' || action === 'restore') await loadConfig();
    await refresh();
    if (currentPage === 'printing') await loadJobs();
  } catch (error) { showMessage(error.message, true); await refresh(); }
  finally { setBusy(false); }
}
function jobTable(jobs, checkboxes = false) {
  return `<div class="table-wrap"><table><thead><tr>${checkboxes ? '<th>选择</th>' : ''}<th>作业编号</th><th>所属队列</th><th>提交信息</th></tr></thead><tbody>${jobs.map((job) => `<tr>${checkboxes ? `<td><input type="checkbox" aria-label="选择 ${esc(job.id)}" value="${esc(job.id)}"></td>` : ''}<td>${esc(job.id)}</td><td>${esc(job.queue)}</td><td>${esc(job.title || '纸面结果未知')}</td></tr>`).join('')}</tbody></table></div>`;
}
function chooseJobs(action, token, jobs = [], unknown = false, queueNames = lastState?.queues || []) {
  jobs ||= [];
  const changingConfig = action === 'apply' || action === 'restore';
  const title = { 'restart-agent': '重启打印 Agent', 'restart-cups': '重启 CUPS', 'enable-queue': '恢复目标队列', apply: '应用配置前检查', restore: '恢复配置前检查' }[action];
  const names = action === 'restart-cups' ? queueNames : [...new Set([lastState?.queue, ...(changingConfig ? [cfg.queue] : [])].filter(Boolean))];
  const box = openDialog(title, `<p>${action === 'restart-cups' ? '重启 CUPS 会影响本机全部打印队列。' : `操作目标：${esc(action === 'restart-agent' ? '打印 Agent' : names.join('、') || '当前打印配置')}。`}</p><p>涉及队列：${esc(names.join('、') || '未知／不可读取')}</p><div class="notice">保留的旧作业可能继续出纸；取消不代表此前没有出纸。</div><p>${unknown ? '当前作业不可读取，恢复后的出纸情况需现场检查。' : jobs.length ? `检查发现 ${jobs.length} 个未结束作业，请选择需要取消的作业。` : '未发现未结束作业。'}</p>${jobs.length ? jobTable(jobs, true) : ''}<p id="dialogSelection" class="muted"></p>`, `<button data-choice="abort">放弃操作</button><button data-choice="keep">${changingConfig ? '保留作业，暂不应用' : unknown ? '接受状态未知并恢复' : '保留作业并继续'}</button><button class="danger" data-choice="cancel" disabled>取消所选作业后继续</button>`, true);
  box.addEventListener('change', () => {
    const count = box.querySelectorAll('input:checked').length;
    box.querySelector('[data-choice=cancel]').disabled = unknown || !count;
    $('dialogSelection').textContent = `已选择 ${count} 个作业`;
  });
  for (const button of box.querySelectorAll('[data-choice]')) button.onclick = async () => {
    const choice = button.dataset.choice;
    if (choice === 'abort') { box.close(); return; }
    if (choice === 'keep' && changingConfig) { box.close(); showMessage('作业已保留，配置尚未应用。请在作业完成后重试。'); return; }
    const ids = [...box.querySelectorAll('input:checked')].map((input) => input.value);
    box.close();
    await act(action, { previewToken: token, disposition: unknown ? 'unknown-confirmed' : choice, jobIds: ids });
  };
}
async function guarded(action) {
  if (busy) return;
  try {
    await refresh();
    const data = await api('/api/jobs');
    const queues = (data.queuesOutput || '').split('\n').filter((line) => line.startsWith('printer ')).map((line) => line.split(/\s+/)[1]);
    chooseJobs(action, data.previewToken, data.jobs, data.unknown, queues);
  } catch (error) { showMessage(error.message, true); }
}
async function startAgent() {
  if (busy) return;
  if (await confirmAction('启动打印 Agent', '<p>使用 agent.env 中已应用的配置启动打印 Agent。启动或重连不会重新提交旧打印尝试。</p>', '确认启动')) await act('start-agent');
}
async function apply() {
  if (busy) return;
  if (formDirty) { showMessage('请先保存表单中的修改，再应用配置。', true); return; }
  if (await confirmAction('应用配置并重启打印 Agent', `<p>将已保存的草稿写入 agent.env，并重启打印 Agent。维护页面保持可用。</p><p>${esc(calibration(cfg))}</p><p>仪表：${esc(scaleSummary(cfg))}。</p><p>执行前会检查当前及新目标队列的未结束作业；有作业时需先处理。</p>`, '确认应用并重启')) await act('apply');
}
async function restore() {
  if (busy) return;
  if (await confirmAction('恢复上一份配置', '<p>恢复上一次应用前的配置并重启打印 Agent，不删除或重新提交打印尝试。执行前会检查相关队列的未结束作业。</p>', '确认恢复并重启')) await act('restore');
}
function renderJobs() {
  if (jobsError) { $('jobs').innerHTML = `<p class="bad">${esc(jobsError)}</p>`; $('jobCount').textContent = '作业状态未知'; updateControls(); return; }
  const jobs = (jobsCache || []).filter((job) => !$('jobQueue').value || job.queue === $('jobQueue').value);
  $('jobCount').textContent = `共 ${jobs.length} 个未结束作业`;
  $('jobs').innerHTML = jobs.length ? jobTable(jobs, true) : '<p class="empty">当前无未结束作业。</p>';
  updateControls();
}
async function loadJobs() {
  const version = ++jobsVersion;
  $('jobs').textContent = '正在读取未结束作业…';
  try {
    const data = await api('/api/jobs');
    if (version !== jobsVersion) return;
    if (data.unknown) throw Error('无法读取当前作业，请先查看并恢复 CUPS 服务。');
    jobsCache = data.jobs || [];
    jobsError = '';
    const selected = $('jobQueue').value;
    const queues = [...new Set([...(lastState?.queues || []), ...jobsCache.map((job) => job.queue)])];
    $('jobQueue').innerHTML = '<option value="">全部队列</option>' + queues.map((queue) => `<option>${esc(queue)}</option>`).join('');
    if (queues.includes(selected)) $('jobQueue').value = selected;
  } catch (error) {
    if (version !== jobsVersion) return;
    jobsCache = null;
    jobsError = error.message;
  }
  renderJobs();
}
async function cancelSelected() {
  if (busy) return;
  const ids = [...document.querySelectorAll('#jobs input:checked')].map((input) => input.value);
  if (!ids.length) return;
  const selected = (jobsCache || []).filter((job) => ids.includes(job.id));
  if (await confirmAction('取消选中作业', `<p>确认取消以下 ${ids.length} 个作业？取消不代表此前没有出纸，打印尝试记录会保留。</p>${jobTable(selected)}`, '确认取消所选作业', true)) await act('cancel-jobs', { jobIds: ids });
}
async function testPrint() {
  if (busy) return;
  if (!await confirmAction('打印校准样张', `<p>提交一份固定“测试／校准”样张，不重打业务单据。</p><p>${esc(configData ? calibration(configData.active) : '使用已应用配置')}</p><p>不会自动恢复暂停队列。纸面效果需要人工检查。</p>`, '确认打印一份')) return;
  setBusy(true);
  try {
    const result = await api('/api/test-print', { method: 'POST', body: '{}' });
    showMessage(result.message);
    await refresh();
    await loadJobs();
  } catch (error) { showMessage(error.message, true); }
  finally { setBusy(false); }
}
async function loadLogs() {
  $('logs').textContent = '正在读取运行日志…';
  try { $('logs').textContent = await api('/api/logs', {}, true); }
  catch (error) { $('logs').textContent = error.message; }
}
async function loadAudit(offset = 0) {
  const version = ++auditVersion;
  const query = new URLSearchParams({ from: $('afrom').value, to: $('ato').value, action: $('atype').value, result: $('ares').value, offset, limit: 100 });
  try {
    const data = await api(`/api/audit?${query}`);
    if (version !== auditVersion) return;
    auditOffset = data.offset;
    auditTotal = data.total;
    auditRows = data.records.length;
    $('audit').innerHTML = `<table><thead><tr><th>时间</th><th>来源／会话</th><th>操作</th><th>目标</th><th>结果</th><th>变更</th></tr></thead><tbody>${data.records.map((row) => `<tr><td>${esc(new Date(row.at).toLocaleString())}</td><td>${esc(row.source)}<br><span class="muted">${esc(row.session)}</span></td><td>${esc(row.action)}</td><td>${esc(row.target)}</td><td>${esc(row.result)}</td><td>${esc(row.before || row.after ? JSON.stringify({ before: row.before, after: row.after }) : '')}</td></tr>`).join('') || '<tr><td colspan="6" class="empty">未找到符合条件的维护记录。</td></tr>'}</tbody></table>`;
    $('auditCount').textContent = `共 ${data.total} 条，日期按工控机本地时区筛选；共用维护会话不代表具体人员身份。`;
    updateControls();
  } catch (error) { if (version === auditVersion) $('audit').textContent = error.message; }
}
$('configForm').addEventListener('input', () => { formDirty = true; $('draftState').textContent = '表单有尚未保存的修改'; updateDraftGeometry(); });
$('jobs').addEventListener('change', updateControls);
window.addEventListener('hashchange', visitPage);
showPage();
refresh();
api('/api/session').then(async (session) => {
  setAuthenticated(session.authenticated);
  if (session.authenticated) { await loadConfig(); await visitPage(); }
}).catch((error) => showMessage(error.message, true));
setInterval(refresh, 15000);

async function importSiteConfiguration(input) {
 const file=input.files?.[0];if(!file||busy)return;
 try {
  if(file.size>16384)throw Error('厂区配置文件过大');
  const connection=JSON.parse(await file.text());
  const preview=await api('/api/config/import',{method:'POST',body:JSON.stringify({connection})});
  const c=preview.config;
  if(!await confirmAction('导入厂区配置',`<p>厂区：${esc(c.siteName)}（${esc(c.siteCode)}）</p><p>地磅：${esc(c.scaleName)}（${esc(c.scaleCode)}）</p><p>Agent：${esc(c.agentId)}</p><p>Server：${esc(c.serverUrl)}</p><p>保留${esc(calibration(c))}。导入只保存草稿，需明确应用后生效。</p>`,'确认导入'))return;
  setBusy(true);
  await api('/api/config/import',{method:'POST',body:JSON.stringify({connection,confirm:true})});
  await loadConfig();showMessage('厂区配置已导入草稿，点击“应用配置”后生效。');
 }catch(error){showMessage(error.message,true)}finally{input.value='';setBusy(false)}
}
