import { useCallback, useEffect, useMemo, useState } from 'react';
import { Activity, FileClock, Printer, RefreshCw, Settings2 } from 'lucide-react';
import { agentMaintenanceApi, type MaintenanceConfig } from '@/api/agent-maintenance';
import { weighingApi } from '@/api/weighing';
import { PageHeader } from '@/components/common/page-header';
import { ContentCard } from '@/components/common/content-card';
import { FormSection } from '@/components/common/form-section';
import { Field } from '@/components/common/field';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { getErrorMessage } from '@/lib/api-error';

type ServiceStatus = Record<string, unknown>;
type ConfigField = { key: string; label: string; kind?: 'number' | 'boolean' | 'select' | 'secret'; min?: number; max?: number; options?: string[]; help?: string };
const fields: ConfigField[] = [
  { key: 'serverUrl', label: '业务服务地址', help: 'Agent 连接的本机业务服务地址' },
  { key: 'agentId', label: 'Agent 编号' },
  { key: 'scaleId', label: '地磅编号', kind: 'number', min: 1 },
  { key: 'agentKey', label: '连接密钥', kind: 'secret', help: '已设置的密钥不会回显；留空表示保持原值' },
  { key: 'queue', label: 'CUPS 打印队列' },
  { key: 'printWidthMm', label: '打印宽度（mm）', kind: 'number', min: 100, max: 210 },
  { key: 'printScale', label: '整体缩放（倍）', kind: 'number', min: 0.5, max: 1.5 },
  { key: 'offsetXmm', label: '向右偏移（mm）', kind: 'number', min: 0, max: 30 },
  { key: 'offsetYmm', label: '向下偏移（mm）', kind: 'number', min: 0, max: 30 },
  { key: 'formLengthMm', label: '连续纸表长（mm）', kind: 'number', min: 85, max: 300 },
  { key: 'serialEnabled', label: '启用柯力 D12 仪表', kind: 'boolean' },
  { key: 'serialPort', label: '串口路径' },
  { key: 'serialBaud', label: '仪表波特率', kind: 'number', min: 1200, max: 115200 },
  { key: 'serialProtocol', label: '仪表协议', kind: 'select', options: ['keli-d12-tf05'] },
  { key: 'serialWeightUnit', label: '现场源单位', kind: 'select', options: ['t', 'kg'] },
  { key: 'serialStaleMs', label: '无有效帧故障阈值（ms）', kind: 'number', min: 200, max: 2999 },
  { key: 'cameraFrontUrl', label: '车前抓拍地址', kind: 'secret', help: 'HTTP 快照或 RTSP 地址；地址不回显，留空保持已保存值' },
  { key: 'cameraRearUrl', label: '车后抓拍地址', kind: 'secret', help: 'HTTP 快照或 RTSP 地址；地址不回显，留空保持已保存值' },
  { key: 'plateRecognitionUrl', label: '车牌识别服务地址', kind: 'secret', help: '留空保持已保存值；不提供独立连通性测试' },
  { key: 'cameraTimeoutMs', label: '抓拍超时（ms）', kind: 'number', min: 200, max: 10000 },
];
const secretConfigKeys = ['agentKey', 'cameraFrontUrl', 'cameraRearUrl', 'plateRecognitionUrl'];
function safeDraft(config: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(config).map(([key, value]) => [key, secretConfigKeys.includes(key) ? '' : value]));
}

function stateValue(value: unknown) {
  if (value === true || value === 1) return '启用';
  if (value === false || value === 0) return '停用';
  return typeof value === 'string' && value ? value : '未知';
}

export function AgentMaintenancePage() {
  const [maintenanceEnabled, setMaintenanceEnabled] = useState<boolean | null>(null);
  const [status, setStatus] = useState<ServiceStatus | null>(null);
  const [configData, setConfigData] = useState<MaintenanceConfig | null>(null);
  const [draft, setDraft] = useState<Record<string, unknown>>({});
  const [permissionCodes, setPermissionCodes] = useState<string[]>([]);
  const [logs, setLogs] = useState('尚未读取运行日志。');
  const [audit, setAudit] = useState<Record<string, unknown>[]>([]);
  const [busy, setBusy] = useState(false);
  const [initialLoaded, setInitialLoaded] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [tab, setTab] = useState<'overview' | 'devices' | 'config' | 'records'>('overview');
  const canManage = maintenanceEnabled === true && permissionCodes.includes('agent:maintenance:manage');
  const canView = canManage || (maintenanceEnabled === true && permissionCodes.includes('agent:maintenance:view'));

  const refresh = useCallback(async () => {
    setError('');
    try {
      const caps = await weighingApi.capabilities();
      setPermissionCodes(caps.permissionCodes);
      setMaintenanceEnabled(caps.maintenanceEnabled === true);
      if (!caps.maintenanceEnabled) { setStatus(null); return; }
      if (!caps.permissionCodes.includes('agent:maintenance:view') && !caps.permissionCodes.includes('agent:maintenance:manage')) { setStatus(null); return; }
      const state = await agentMaintenanceApi.status(); setStatus(state);
      if (caps.permissionCodes.includes('agent:maintenance:manage')) {
        const data = await agentMaintenanceApi.config();
        setConfigData(data); setDraft(safeDraft(data.config));
      }
    } catch (failure) { setError(getErrorMessage(failure, '本机维护状态读取失败')); }
    finally { setInitialLoaded(true); }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  async function runAction(action: string, extra: Record<string, unknown> = {}) {
    setBusy(true); setError(''); setNotice('');
    try {
      let preflight: Awaited<ReturnType<typeof agentMaintenanceApi.jobs>> | undefined;
      if (action !== 'start-agent') {
        preflight = await agentMaintenanceApi.jobs();
        if (preflight.unknown) {
          if (!window.confirm(`无法读取未结束打印作业。继续“${action}”后，请现场核实打印结果。是否继续？`)) { setNotice('操作已取消，设备状态未改变。'); return; }
        } else if (preflight.jobs?.length) {
          const items = preflight.jobs.map((job) => `${job.id}（${job.queue}）`).join('、');
          if (action === 'apply' || action === 'restore') {
            setNotice(`发现未结束作业：${items}。配置草稿已保留，完成现场核实后再应用。`); return;
          }
          if (!window.confirm(`发现未结束作业：${items}。继续后仍需现场核实纸面结果。是否继续“${action}”？`)) { setNotice('操作已取消，设备状态未改变。'); return; }
        }
      }
      const result = await agentMaintenanceApi.action({ action, ...extra, ...(preflight ? { previewToken: preflight.previewToken, disposition: preflight.unknown ? 'unknown-confirmed' : 'keep', jobIds: [] } : {}) });
      setNotice(result.output || '维护操作已完成；请检查设备实际状态及纸面结果。');
      await refresh();
    } catch (failure) {
      setError(`${getErrorMessage(failure, '维护操作失败')}。结果可能未知，已尝试读取本机状态；核实设备状态后再决定是否重试。`);
      try { setStatus(await agentMaintenanceApi.status()); } catch { setStatus(null); }
    } finally { setBusy(false); }
  }

  async function saveDraft() {
    setBusy(true); setError(''); setNotice('');
    try {
      await agentMaintenanceApi.saveConfig(draft);
      setNotice('配置草稿已保存，当前运行参数未改变。');
      const data = await agentMaintenanceApi.config(); setConfigData(data); setDraft(safeDraft(data.config));
    } catch (failure) { setError(`${getErrorMessage(failure, '配置保存失败')}。若连接中断，请重新读取草稿核实保存结果。`); }
    finally { setBusy(false); }
  }

  async function loadRecords() {
    setBusy(true); setError('');
    try {
      const [logText, auditData] = await Promise.all([
        agentMaintenanceApi.logs(),
        agentMaintenanceApi.audit(new URLSearchParams({ offset: '0', limit: '100' })),
      ]);
      setLogs(logText); setAudit(auditData.records ?? []);
    } catch (failure) { setError(getErrorMessage(failure, '维护记录读取失败')); }
    finally { setBusy(false); }
  }

  const serviceCards = useMemo(() => [
    ['业务服务', status?.maintenance], ['打印 Agent', status?.agentKnown ? status?.agent : '未知'],
    ['CUPS 服务', status?.cupsKnown ? status?.cups : '未知'], ['打印队列', status?.queueKnown ? `${status?.queue ?? '未知'}（${status?.queueState ?? '未知'}）` : '未知'],
  ], [status]);
  const statusJobs = Array.isArray(status?.jobs) ? status.jobs as { id: string; queue?: string }[] : [];
  const camera = (status?.camera && typeof status.camera === 'object' ? status.camera : {}) as Record<string, unknown>;

  if (!initialLoaded) return <p role="status">正在检查本机维护状态…</p>;
  return <div className="space-y-space-4">
    <PageHeader title="设备维护" description="" actions={<><StatusTag tone="info">本机设备维护</StatusTag><Button variant="secondary" disabled={busy} onClick={() => void refresh()}><RefreshCw className="mr-2 h-4 w-4" aria-hidden />刷新状态</Button></>} />
    {maintenanceEnabled === false && <ContentCard title="设备维护未启用"><p>当前业务服务未启用设备维护，请联系管理员检查本机维护配置。</p></ContentCard>}
    {maintenanceEnabled === true && !canView && <ContentCard title="无维护查看权限"><p>当前账号没有设备维护查看权限。请联系本厂管理员分配 agent:maintenance:view。</p></ContentCard>}
    {canView && <>
      <nav aria-label="设备维护页面" className="flex flex-wrap gap-2 border-b border-border pb-3">
        {([['overview', '运行总览', Activity], ['devices', '设备与打印', Printer], ['config', '配置', Settings2], ['records', '维护记录', FileClock]] as const).map(([key, label, Icon]) => <Button key={key} variant={tab === key ? 'primary' : 'secondary'} onClick={() => { setTab(key); if (key === 'records') void loadRecords(); }}>{<Icon className="mr-2 h-4 w-4" aria-hidden />}{label}</Button>)}
      </nav>
      {error && <p className="rounded-control border border-error/30 bg-error/5 p-3 text-sm text-error" role="alert">{error}</p>}
      {notice && <p className="rounded-control border border-border bg-neutral-background p-3 text-sm" role="status">{notice}</p>}
      {tab === 'overview' && <>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{serviceCards.map(([label, value]) => <ContentCard key={String(label)} title={String(label)}><StatusTag tone={/运行中|已启用|已连接/.test(String(value ?? '')) ? 'success' : 'warning'}>{stateValue(value)}</StatusTag></ContentCard>)}</div>
        <ContentCard title="本机诊断" description={`检查时间：${String(status?.checkedAt ?? '未知')}`}>
          <p>{String(status?.lastError ?? '未报告本机错误')}</p>
          <p className="mt-2 text-sm text-text-secondary">未结束打印作业：{status?.jobsKnown === false ? '状态未知' : statusJobs.length ? statusJobs.map((job) => `${job.id}（${job.queue ?? '队列未知'}，纸面结果需核实）`).join('、') : '未发现'}</p>
          {canManage && <div className="mt-4 flex flex-wrap gap-2"><Button disabled={busy || status?.agent === '运行中'} onClick={() => void runAction('start-agent')}>启动打印 Agent</Button><Button variant="secondary" disabled={busy || status?.agent !== '运行中'} onClick={() => void runAction('stop-agent')}>停止打印 Agent</Button><Button variant="secondary" disabled={busy} onClick={() => void runAction('restart-agent')}>重启打印 Agent</Button><Button variant="secondary" disabled={busy} onClick={() => void runAction('restart-cups')}>重启 CUPS</Button></div>}
        </ContentCard>
      </>}
      {tab === 'devices' && <>
        <ContentCard title="称重仪表" description="串口由打印 Agent 独占；维护页仅查看状态，不直接驱动设备。"><p>{stateValue((status?.scale as Record<string, unknown> | undefined)?.state)} · 重量 {stateValue((status?.scale as Record<string, unknown> | undefined)?.weight)} t</p><p className="mt-2 text-sm text-text-secondary">{String((status?.scale as Record<string, unknown> | undefined)?.fault ?? '暂无仪表故障信息')}</p></ContentCard>
        <ContentCard title="打印队列与校准" description="取消作业不代表此前没有出纸；操作后核实设备状态和纸面结果。"><p>目标队列：{stateValue(status?.queue)} · {stateValue(status?.queueState)}</p><div className="mt-3 flex flex-wrap gap-2">{canManage && <><Button variant="secondary" disabled={busy} onClick={() => void runAction('enable-queue')}>恢复目标队列</Button><Button disabled={busy} onClick={async () => { if (window.confirm('将提交一份固定校准样张，不会重打业务单据。是否继续？')) { setBusy(true); try { const result = await agentMaintenanceApi.testPrint(); setNotice(result.message || '校准样张请求已提交，请核对纸面效果。'); await refresh(); } catch (failure) { setError(`${getErrorMessage(failure, '测试打印失败')}。结果可能未知，请先核实是否出纸。`); } finally { setBusy(false); } } }}>打印校准样张</Button></>}</div></ContentCard>
        <ContentCard title="摄像头" description="此页仅显示配置状态；抓拍只会由正式称重采集流程按请求执行。"><p>车前抓拍地址：{camera.frontConfigured ? '已配置（地址不回显）' : '未配置'} · 车后抓拍地址：{camera.rearConfigured ? '已配置（地址不回显）' : '未配置'} · 车牌识别服务：{camera.recognitionConfigured ? '已配置（地址不回显）' : '未配置'}</p><p className="mt-2 text-sm text-text-secondary">{String(camera.state ?? '不提供视频预览或维护页测试抓拍，避免生成业务之外的现场图片。')}</p></ContentCard>
      </>}
      {tab === 'config' && <>
        {!canManage && <ContentCard title="只读维护权限"><p>当前账号可以查看运行状态，配置和维护操作需要 agent:maintenance:manage。</p></ContentCard>}
        {canManage && <>
          <FormSection title="连接与设备参数" description="保存仅形成草稿。应用前请确认无未决打印作业；保存和应用结果不明时，先重新读取核实。">
            {fields.map((field) => {
              const value = draft[field.key];
              const id = `maintenance-${field.key}`;
              if (field.kind === 'boolean') return <Field key={field.key} label={field.label} htmlFor={id}><Select id={id} value={String(Boolean(value))} disabled={busy} onChange={(event) => setDraft((current) => ({ ...current, [field.key]: event.target.value === 'true' }))}><option value="true">启用</option><option value="false">停用</option></Select></Field>;
              if (field.kind === 'select') return <Field key={field.key} label={field.label} htmlFor={id}><Select id={id} value={String(value ?? field.options?.[0] ?? '')} disabled={busy} onChange={(event) => setDraft((current) => ({ ...current, [field.key]: event.target.value }))}>{field.options?.map((option) => <option key={option}>{option}</option>)}</Select></Field>;
              return <Field key={field.key} label={field.label} htmlFor={id} help={field.help}><Input id={id} type={field.kind === 'number' ? 'number' : field.kind === 'secret' ? 'password' : 'text'} min={field.min} max={field.max} step={field.key === 'printScale' || field.key === 'formLengthMm' ? 'any' : undefined} autoComplete="new-password" disabled={busy} value={field.kind === 'secret' ? '' : String(value ?? '')} onChange={(event) => setDraft((current) => ({ ...current, [field.key]: field.kind === 'number' ? Number(event.target.value) : event.target.value }))} /></Field>;
            })}
          </FormSection>
          <div className="flex flex-wrap gap-2"><Button disabled={busy} onClick={() => void saveDraft()}>保存草稿</Button><Button variant="secondary" disabled={busy || !configData} onClick={() => void runAction('apply')}>应用配置并重启 Agent</Button><Button variant="secondary" disabled={busy || !configData?.hasPrevious} onClick={() => void runAction('restore')}>恢复上一份配置</Button></div>
          <ContentCard title="配置状态"><p>{configData?.saved ? '没有待应用草稿' : '存在未应用草稿'} · Agent 密钥{configData?.hasAgentKey ? '已设置（不回显）' : '未设置'}{configData?.keyChanged ? ' · 密钥待替换（不回显）' : ''}</p><p className="mt-2 text-sm text-text-secondary">{String(configData?.runtimeState ?? '运行配置状态未知')}</p></ContentCard>
        </>}
      </>}
      {tab === 'records' && <>
        <ContentCard title="维护记录" extra={<Button variant="secondary" disabled={busy} onClick={() => void loadRecords()}>刷新记录</Button>}><div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="border-b border-border"><th className="p-2">时间</th><th className="p-2">操作</th><th className="p-2">目标</th><th className="p-2">结果</th></tr></thead><tbody>{audit.map((row, index) => <tr key={String(row.id ?? index)} className="border-b border-border"><td className="p-2">{String(row.at ?? '')}</td><td className="p-2">{String(row.action ?? '')}</td><td className="p-2">{String(row.target ?? '')}</td><td className="p-2">{String(row.result ?? '')}</td></tr>)}</tbody></table>{!audit.length && <p className="p-3 text-text-tertiary">暂无已读取的维护记录</p>}</div></ContentCard>
        <ContentCard title="运行日志" extra={<Button variant="secondary" disabled={busy} onClick={async () => { setBusy(true); try { setLogs(await agentMaintenanceApi.logs()); } catch (failure) { setLogs(getErrorMessage(failure, '日志读取失败')); } finally { setBusy(false); } }}>读取最近日志</Button>}><pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words text-xs">{logs}</pre></ContentCard>
      </>}
    </>}
  </div>;
}
