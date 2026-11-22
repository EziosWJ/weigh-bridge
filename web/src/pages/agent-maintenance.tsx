import { useCallback, useEffect, useState } from 'react';
import { Activity, AlertTriangle, CheckCircle2, CircleHelp, Gauge, Play, Settings2, Printer, RefreshCw, RotateCw, Square } from 'lucide-react';
import { agentMaintenanceApi, type CupsQueue, type DeviceSettings, type MaintenanceConfig } from '@/api/agent-maintenance';
import { overviewApi, type OverviewDevice } from '@/api/overview';
import { FormDialog } from '@/components/common/form-dialog';
import { HelpHint } from '@/components/common/help-hint';
import { weighingApi } from '@/api/weighing';
import { PageHeader } from '@/components/common/page-header';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { SectionTabs } from '@/components/common/section-tabs';
import { DataTable } from '@/components/common/data-table';
import { DataTableCard } from '@/components/common/data-table-card';
import { TableToolbar } from '@/components/common/table-toolbar';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/common/toast-store';
import { MaintenanceBackupPanel } from './agent-maintenance/backup-panel';
import { MaintenanceConfigPanel } from './agent-maintenance/config-panel';
import { SettingsComparison } from './agent-maintenance/settings-comparison';
import { applyResultLabel, appliedVersion } from './agent-maintenance/apply-result';
import { formatDateTime } from '@/lib/datetime';
import { formatWeightValue } from '@/lib/weighing/weights';
import { getErrorMessage } from '@/lib/api-error';
import type { DataTableColumn } from '@/types';

type ServiceStatus = Record<string, unknown>;
type PrintJobs = Awaited<ReturnType<typeof agentMaintenanceApi.jobs>>;

async function readMaintenanceStatus(): Promise<ServiceStatus> {
  const state = await agentMaintenanceApi.status();
  return { ...state, checkedAt: typeof state.checkedAt === 'string' ? state.checkedAt : new Date().toISOString() };
}

function stateValue(value: unknown) {
  if (value === true) return '启用';
  if (value === false) return '停用';
  return (typeof value === 'string' && value) || (typeof value === 'number' && Number.isFinite(value)) ? String(value) : '未知';
}

function StatusValue({ value }: { value: unknown }) {
  const label = stateValue(value);
  const tone = /未知|不可读取|未配置/.test(label) ? 'neutral' : /故障|离线|失败|不可用/.test(label) ? 'error' : /^(运行中|已启用|已连接|稳定|就绪|已配置)$/.test(label) ? 'success' : 'warning';
  return <StatusTag tone={tone}>{label}</StatusTag>;
}

function numberOr(value: unknown, fallback: number) {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
}

function settingsFromConfig(config: MaintenanceConfig): DeviceSettings {
  const effective = config.effective;
  if (effective) return { serial: effective.serial, print: effective.print };
  const active = config.active ?? config.config ?? {};
  return {
    serial: { port: String(active.serialPort ?? ''), baud: numberOr(active.serialBaud, 9600) },
    print: {
      queue: String(active.queue ?? ''), formLengthMm: numberOr(active.formLengthMm, 93),
      printWidthMm: numberOr(active.printWidthMm, 175), printScale: numberOr(active.printScale, 1),
      offsetXmm: numberOr(active.offsetXmm, 0), offsetYmm: numberOr(active.offsetYmm, 0),
    },
  };
}

const actionLabels: Record<string, string> = {
  'start-agent': '启动设备', 'stop-agent': '停止设备', 'restart-agent': '重载设备',
  'restart-cups': '重启 CUPS', 'enable-queue': '恢复目标队列', 'test-print': '打印校准样张',
};

export function AgentMaintenancePage() {
  const [maintenanceEnabled, setMaintenanceEnabled] = useState<boolean | null>(null);
  const [status, setStatus] = useState<ServiceStatus | null>(null);
  const [report, setReport] = useState<OverviewDevice | null>(null);
  const [reportError, setReportError] = useState('');
  const [applyOpen, setApplyOpen] = useState(false);
  const [applyNeedsCheck, setApplyNeedsCheck] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [actionConfirmation, setActionConfirmation] = useState<{ action: string; preflight: PrintJobs }>();
  const [configData, setConfigData] = useState<MaintenanceConfig | null>(null);
  const [settings, setSettings] = useState<DeviceSettings | null>(null);
  const [queues, setQueues] = useState<CupsQueue[]>([]);
  const [queuesError, setQueuesError] = useState('');
  const [permissionCodes, setPermissionCodes] = useState<string[]>([]);
  const [logs, setLogs] = useState('尚未读取运行日志。');
  const [audit, setAudit] = useState<Record<string, unknown>[]>([]);
  const [busy, setBusy] = useState(false);
  const [initialLoaded, setInitialLoaded] = useState(false);
  const [error, setError] = useState('');
  const [recordsLoaded, setRecordsLoaded] = useState(false);
  const [recordsLoading, setRecordsLoading] = useState(false);
  const [recordsError, setRecordsError] = useState('');
  const [tab, setTab] = useState<'overview' | 'printer' | 'instrument' | 'backups' | 'records'>('overview');
  const deviceOffline = !status || status?.agentProcessOnline === false || status?.maintenanceAvailable === false;
  const canManage = maintenanceEnabled === true && permissionCodes.includes('agent:maintenance:manage');
  const canView = canManage || (maintenanceEnabled === true && permissionCodes.includes('agent:maintenance:view'));

  const refresh = useCallback(async () => {
    setRefreshing(true); setError(''); setReportError('');
    try {
      const caps = await weighingApi.capabilities();
      setPermissionCodes(caps.permissionCodes);
      setMaintenanceEnabled(caps.maintenanceEnabled === true);
      if (!caps.maintenanceEnabled) { setStatus(null); return; }
      if (!caps.permissionCodes.includes('agent:maintenance:view') && !caps.permissionCodes.includes('agent:maintenance:manage')) { setStatus(null); return; }
      const code = caps.scales[0]?.code;
      const readReport = code && caps.permissionCodes.includes('weighing:read')
        ? overviewApi.device(code).then(setReport, failure => {
          setReport(null);
          setReportError(getErrorMessage(failure, '无法读取仪表上报'));
        })
        : Promise.resolve(setReport(null));
      const [state] = await Promise.all([readMaintenanceStatus().catch(failure => {
        setStatus(null);
        throw failure;
      }), readReport]);
      setStatus(state);
      if (state.maintenanceAvailable !== false && state.agentProcessOnline !== false && caps.permissionCodes.includes('agent:maintenance:manage')) {
        const data = await agentMaintenanceApi.config();
        setConfigData(data); setSettings(settingsFromConfig(data)); setApplyNeedsCheck(false);
        try {
          const read = await agentMaintenanceApi.queues();
          setQueues(read.queues ?? []); setQueuesError(read.error || '');
        } catch (failure) {
          setQueues([]); setQueuesError(getErrorMessage(failure, 'Agent 打印队列读取失败'));
        }
      }
    } catch (failure) {
      setApplyNeedsCheck(true);
      setError(getErrorMessage(failure, 'Agent 维护状态读取失败'));
    }
    finally { setRefreshing(false); setInitialLoaded(true); }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  async function runAction(action: string, confirmed?: PrintJobs) {
    setBusy(true); setError('');
    try {
      const preflight = confirmed ?? (action === 'start-agent' ? undefined : await agentMaintenanceApi.jobs());
      if (!confirmed && preflight && (preflight.unknown || preflight.jobs?.length)) {
        setActionConfirmation({ action, preflight });
        return;
      }
      const result = await agentMaintenanceApi.action({ action, ...(preflight ? { previewToken: preflight.previewToken, disposition: preflight.unknown ? 'unknown-confirmed' : 'keep', jobIds: [] } : {}) });
      if (result.needsDisposition) {
        const jobs = result.unknown ? '未结束打印作业数量未知' : `未结束打印作业 ${result.jobs?.length ?? 0} 项`;
        toast.warning({ title: `${actionLabels[action]}待确认`, description: `${jobs}。请核实后重新发起。`, duration: 6000 });
      } else {
        toast.success(result.output || '维护操作已完成');
      }
      await refresh();
    } catch (failure) {
      setError(`${getErrorMessage(failure, '维护操作失败')}。请刷新状态核实设备执行结果。`);
      try { setStatus(await readMaintenanceStatus()); } catch { setStatus(null); }
    } finally { if (confirmed) setActionConfirmation(undefined); setBusy(false); }
  }

  async function applySettings() {
    if (!settings || !configData || applyNeedsCheck || deviceOffline) return;
    setBusy(true); setError('');
    try {
      const result = await agentMaintenanceApi.apply(settings, configData.effective?.configVersion ?? 0);
      if (result.status === 'applied') {
        toast.success({ title: '配置已保存并应用', description: `设备已核实 · ${appliedVersion(result)}`, duration: 5000 });
        if (result.effective) { setConfigData({ ...configData, effective: result.effective }); setSettings(settingsFromConfig({ ...configData, effective: result.effective })); }
        await refresh();
      } else {
        if (result.status === 'unknown') setApplyNeedsCheck(true);
        setError(`${applyResultLabel(result.status)}${result.message ? `：${result.message}` : ''}`);
        try { setStatus(await readMaintenanceStatus()); } catch { setStatus(null); }
      }
    } catch (failure) {
      setApplyNeedsCheck(true);
      setError(`${getErrorMessage(failure, '配置应用失败')}。结果未知时请先读取实际状态核实，不要重复盲写。`);
    } finally { setApplyOpen(false); setBusy(false); }
  }

  async function loadRecords() {
    setBusy(true); setRecordsLoading(true); setRecordsError('');
    try {
      const [logText, auditData] = await Promise.all([
        agentMaintenanceApi.logs(),
        agentMaintenanceApi.audit(new URLSearchParams({ offset: '0', limit: '100' })),
      ]);
      setLogs(logText); setAudit(auditData.records ?? []); setRecordsLoaded(true);
    } catch (failure) { setRecordsError(getErrorMessage(failure, '维护记录读取失败')); }
    finally { setBusy(false); setRecordsLoading(false); }
  }

  async function printSample() {
    if (!window.confirm('将向当前打印队列提交一份校准样张。是否继续？')) return;
    setBusy(true); setError('');
    try {
      const result = await agentMaintenanceApi.testPrint();
      toast.info(result.message || '校准样张请求已提交，请核对纸面效果。');
      await refresh();
    } catch (failure) {
      setError(`${getErrorMessage(failure, '测试打印失败')}。结果可能未知，请先核实是否出纸。`);
    } finally { setBusy(false); }
  }

  const jobsKnown = !deviceOffline && status?.jobsKnown === true;
  const statusJobs = jobsKnown && Array.isArray(status?.jobs) ? status.jobs as { id: string; queue?: string }[] : [];
  const scale = (status?.scale && typeof status.scale === 'object' ? status.scale : {}) as ServiceStatus;
  const instrumentState = report
    ? !report.online ? '离线或数据过期' : !report.deviceOk ? '采集故障' : report.stable ? '稳定' : '未稳定'
    : deviceOffline ? '未知' : scale.state;
  const maintenanceState = deviceOffline ? (status ? '维护连接不可用' : '未知') : '已连接';
  const reportedError = typeof status?.lastError === 'string' ? status.lastError.trim() : '';
  // Older Agents return a no-error placeholder in lastError.
  const lastError = reportedError === '暂无可读取错误' ? '' : reportedError;
  const serviceCards = [
    { label: '仪表上报', value: instrumentState, detail: reportError || report?.error, icon: Gauge },
    { label: '远程维护', value: maintenanceState, detail: deviceOffline ? lastError : undefined, icon: Settings2 },
    { label: '业务接口', value: status?.maintenance, icon: Activity },
    { label: '设备执行', value: !deviceOffline && status?.agentKnown ? status.agent : '未知', icon: Play },
    { label: 'CUPS 服务', value: !deviceOffline && status?.cupsKnown ? status.cups : '未知', icon: Printer },
    { label: '打印队列', value: !deviceOffline && status?.queueKnown ? status.queueState : '未知', detail: !deviceOffline && status?.queueKnown ? stateValue(status.queue) : undefined, icon: Printer },
  ];
  const reportedWeight = report ? report.weight : scale.weight;
  const weight = typeof reportedWeight === 'string' && reportedWeight.trim() !== '' || typeof reportedWeight === 'number' ? Number(reportedWeight) : NaN;
  const weightAvailable = Number.isFinite(weight) && (report
    ? report.online && report.deviceOk
    : !deviceOffline && status?.agent === '运行中' && scale.known === true && scale.deviceOk === true && scale.stale === false);
  const reportTime = report?.receivedAt ?? scale.lastValidAt;
  const lastReportAt = typeof reportTime === 'string' && !reportTime.startsWith('0001') ? reportTime : undefined;
  const statusIncomplete = deviceOffline || status?.agentKnown !== true || status?.cupsKnown !== true || status?.queueKnown !== true || !jobsKnown;
  const printerError = deviceOffline ? '' : lastError;
  const DiagnosisIcon = printerError ? AlertTriangle : statusIncomplete ? CircleHelp : CheckCircle2;
  const fault = typeof scale.fault === 'string' ? scale.fault : '';
  const serialHint = /权限|permission/i.test(fault) ? fault : '';
  const feedback = <>
    {error && <p role="alert" className="rounded-control border border-error-border bg-error-background p-space-3 text-sm text-error">{error}</p>}
  </>;

  const overview = <div className="space-y-space-4">
    <ContentCard bodyClassName="p-0">
      <div className="grid sm:grid-cols-2 xl:grid-cols-3">
        {serviceCards.map(({ label, value, detail, icon: Icon }) => <div key={label} className="min-w-0 space-y-space-3 border-b border-border p-space-4 last:border-b-0 sm:[&:nth-last-child(-n+2)]:border-b-0 sm:odd:border-r xl:[&:nth-child(-n+3)]:border-b xl:[&:not(:nth-child(3n))]:border-r xl:[&:nth-child(3n)]:border-r-0">
          <div className="flex items-center gap-space-2 text-sm font-medium"><Icon className="h-4 w-4 text-text-tertiary" aria-hidden />{label}</div>
          <StatusValue value={value} />
          {detail && <p className="break-all text-body-secondary text-text-tertiary">{detail}</p>}
        </div>)}
      </div>
    </ContentCard>
    <ContentCard title="设备执行控制" className="min-w-0">
      <DetailItem label="当前执行状态" value={<StatusValue value={!deviceOffline && status?.agentKnown ? status.agent : '未知'} />} />
      {canManage ? <>
        <div className="mt-space-4 flex flex-wrap gap-space-2">
          <Button variant="primary" disabled={busy || refreshing || deviceOffline || status?.agent === '运行中'} onClick={() => void runAction('start-agent')}><Play className="h-4 w-4" aria-hidden />启动设备</Button>
          <Button disabled={busy || refreshing || deviceOffline || status?.agent !== '运行中'} onClick={() => void runAction('stop-agent')}><Square className="h-4 w-4" aria-hidden />停止设备</Button>
          <Button disabled={busy || refreshing || deviceOffline} onClick={() => void runAction('restart-agent')}><RotateCw className="h-4 w-4" aria-hidden />重载设备</Button>
        </div>
        {deviceOffline && <p className="mt-space-3 text-sm text-warning">请在 Agent 主机恢复维护连接后刷新。</p>}
      </> : <p className="mt-space-4 text-body-secondary text-text-tertiary">只读维护</p>}
    </ContentCard>
  </div>;

  const printerDiagnosis = <ContentCard title="打印诊断">
    <div className="flex items-start gap-space-3">
      <DiagnosisIcon className={`mt-0.5 h-5 w-5 shrink-0 ${printerError ? 'text-warning' : statusIncomplete ? 'text-text-tertiary' : 'text-success'}`} aria-hidden />
      <div className="min-w-0"><p className="break-words text-sm font-medium">{!status ? '尚未读取设备状态' : printerError || (deviceOffline ? 'Agent 维护连接不可用' : statusIncomplete ? '部分运行状态尚未确认' : '未报告设备或打印服务错误')}</p></div>
    </div>
    <div className="mt-space-5 border-t border-border pt-space-4">
      <div className="flex flex-wrap items-center justify-between gap-space-2"><h3 className="text-sm font-medium">未结束打印作业</h3><StatusTag tone={!jobsKnown || statusJobs.length ? 'warning' : 'neutral'}>{!jobsKnown ? '状态未知' : statusJobs.length ? `${statusJobs.length} 项待核实` : '未发现'}</StatusTag></div>
      {!jobsKnown ? <p className="mt-space-2 text-body-secondary text-text-secondary">未能确认作业状态，请先检查队列和现场纸面结果。</p> : statusJobs.length ? <ul className="mt-space-3 divide-y divide-border">{statusJobs.map(job => <li key={job.id} className="flex flex-wrap justify-between gap-space-2 py-space-2 text-sm"><span className="break-all font-medium">{job.id}</span><span className="break-all text-text-secondary">{job.queue ?? '队列未知'}，纸面结果需核实</span></li>)}</ul> : null}
    </div>
  </ContentCard>;

  const instrumentStatus = <ContentCard title="称重仪表" className="min-w-0">
    <div className="rounded-control bg-neutral-background p-space-4">
      <div className="flex flex-wrap items-center justify-between gap-space-2"><p className="text-body-secondary text-text-secondary">最近检查重量</p><StatusValue value={instrumentState} /></div>
      <p className="mt-space-1 text-2xl font-semibold tabular-nums">{weightAvailable ? formatWeightValue(weight) : '—'}<span className="ml-space-2 text-sm font-normal text-text-secondary">吨</span></p>
      <DetailItem label="最近仪表上报" value={formatDateTime(lastReportAt)} />
    </div>
    <div className="mt-space-4 grid grid-cols-2 gap-space-4"><DetailItem label="串口" value={stateValue(scale.port)} /><DetailItem label="波特率" value={stateValue(scale.baud)} /></div>
    {(fault || report?.error || reportError) && <p role="alert" className="mt-space-4 break-words text-sm text-warning">{fault || report?.error || reportError}</p>}
  </ContentCard>;

  const printerStatus = <ContentCard title="打印队列与校准" extra={<HelpHint label="校准与打印说明">先节长，再位置，必要时缩放。打印结果未知时先核实是否出纸。</HelpHint>} className="min-w-0">
    <div className="grid grid-cols-2 gap-space-4">
      <DetailItem label="目标队列" value={!deviceOffline && status?.queueKnown ? stateValue(status.queue) : '未知'} />
      <DetailItem label="队列状态" value={<StatusValue value={!deviceOffline && status?.queueKnown ? status.queueState : '未知'} />} />
      <DetailItem label="CUPS 服务" value={<StatusValue value={!deviceOffline && status?.cupsKnown ? status.cups : '未知'} />} />
      <DetailItem label="未结束作业" value={jobsKnown ? `${statusJobs.length} 项` : '状态未知'} />
    </div>
    {canManage && <div className="mt-space-5 flex flex-wrap gap-space-2 border-t border-border pt-space-4">
      <Button variant="primary" disabled={busy || refreshing || deviceOffline} onClick={() => void printSample()}><Printer className="h-4 w-4" aria-hidden />打印校准样张</Button>
      <Button disabled={busy || refreshing || deviceOffline} onClick={() => void runAction('enable-queue')}>恢复目标队列</Button>
      <Button disabled={busy || refreshing || deviceOffline} onClick={() => void runAction('restart-cups')}>重启 CUPS</Button>
    </div>}
  </ContentCard>;

  const configuration = !canManage ? <ContentCard title="只读维护权限">
    <DetailItem label="Agent 连接的业务服务地址" value={typeof status?.server === 'string' && status.server ? status.server : '未知'} className="[&>div]:break-all" />
  </ContentCard>
    : deviceOffline ? <ContentCard title="Agent 维护暂不可用"><p className="text-sm text-text-secondary">请在 Agent 主机恢复维护连接后刷新。</p></ContentCard>
    : !configData || !settings ? <ContentCard title="配置尚未读取"><p className="text-sm text-text-secondary">请刷新状态重试；读取成功后可编辑设备参数。</p></ContentCard>
    : <MaintenanceConfigPanel section={tab === 'instrument' ? 'serial' : 'print'} config={configData} settings={settings} setSettings={value => setSettings(current => typeof value === 'function' ? value(current ?? settings) : value)} queues={queues} queuesError={queuesError} serialHint={serialHint} busy={busy || refreshing} applyDisabled={applyNeedsCheck} feedback={feedback} onApply={() => { setError(''); setApplyOpen(true); }} />;

  const recordColumns: DataTableColumn<Record<string, unknown>>[] = [
    { title: '时间', key: 'at', width: 172, nowrap: true, render: (_, row) => <span className="tabular-nums text-text-secondary">{formatDateTime(typeof row.at === 'string' ? row.at : undefined)}</span> },
    { title: '操作', key: 'action', width: 136, render: (_, row) => <span className="font-medium">{actionLabels[String(row.action)] ?? String(row.action ?? '—')}</span> },
    { title: '目标', key: 'target', width: 156, render: (_, row) => <span className="text-text-secondary">{String(row.target || '—')}</span> },
    { title: '结果', key: 'result', render: (_, row) => String(row.result || '—') },
  ];
  const records = <div className="space-y-space-4">
    <DataTableCard toolbar={<TableToolbar title="维护记录" actions={<Button disabled={busy} onClick={() => void loadRecords()}><RefreshCw className="h-4 w-4" aria-hidden />刷新记录</Button>} />}>
      <DataTable columns={recordColumns} dataSource={audit.map((row, index) => ({ ...row, rowKey: String(row.id ?? index) }))} rowKey="rowKey" minWidth={620} compact stickyHeader ariaLabel="设备维护操作记录"
        loading={recordsLoading} error={recordsError}
        empty={<p className="px-space-4 py-space-8 text-center text-sm text-text-tertiary">{recordsLoaded ? '暂无维护记录' : '尚未读取记录，请点击刷新记录。'}</p>} />
    </DataTableCard>
    <ContentCard title="运行日志" extra={<Button disabled={busy} onClick={async () => { setBusy(true); try { setLogs(await agentMaintenanceApi.logs()); } catch (failure) { setLogs(getErrorMessage(failure, '日志读取失败')); } finally { setBusy(false); } }}>读取最近日志</Button>} className="min-w-0">
      <pre aria-label="运行日志内容" className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-control border border-border bg-neutral-background p-space-4 text-body-secondary leading-relaxed text-text-secondary">{logs || '本次未读取到日志内容。'}</pre>
    </ContentCard>
  </div>;

  return <div className="min-w-0 space-y-space-4">
    <PageHeader title="设备维护" actions={<Button disabled={busy || refreshing} onClick={() => void refresh()}><RefreshCw className={`h-4 w-4 ${refreshing ? 'motion-safe:animate-spin' : ''}`} aria-hidden />刷新状态</Button>} />
    {settings && configData && <FormDialog open={applyOpen} title="核对设备参数" description="确认后重新应用仪表与打印参数。"
      submitText="确认应用" loading={busy} submitDisabled={applyNeedsCheck || refreshing || deviceOffline} onCancel={() => setApplyOpen(false)} onSubmit={applySettings}>
      <SettingsComparison current={settingsFromConfig(configData)} next={settings} nextLabel="应用后" changesOnly />
    </FormDialog>}
    {actionConfirmation && <FormDialog open title={actionLabels[actionConfirmation.action]} submitText={`确认${actionLabels[actionConfirmation.action]}`}
      loading={busy} onCancel={() => setActionConfirmation(undefined)}
      onSubmit={() => runAction(actionConfirmation.action, actionConfirmation.preflight)}>
      <div className="space-y-2 text-sm">
        <p>{actionConfirmation.preflight.unknown ? '未结束打印作业数量未知' : `未结束打印作业 ${actionConfirmation.preflight.jobs?.length ?? 0} 项`}</p>
        {!actionConfirmation.preflight.unknown && <ul>{actionConfirmation.preflight.jobs?.map(job =>
          <li key={job.id} className="break-words">{job.id} · {job.queue}{job.title && ` · ${job.title}`}</li>)}</ul>}
        {actionConfirmation.action === 'stop-agent' && <p>将暂停重量采集和设备打印执行。</p>}
        {actionConfirmation.action === 'restart-agent' && <p>重载期间会短暂中断重量采集和设备打印执行。</p>}
        <p>继续后请到打印机核实作业出纸结果。</p>
      </div>
    </FormDialog>}
    {!initialLoaded && <ContentCard><p role="status" className="text-sm text-text-secondary">正在检查 Agent 维护状态…</p></ContentCard>}
    {initialLoaded && maintenanceEnabled === false && <ContentCard title="设备维护未启用"><p>当前业务服务未启用设备维护，请联系管理员检查本机维护配置。</p></ContentCard>}
    {initialLoaded && maintenanceEnabled === true && !canView && <ContentCard title="无维护查看权限"><p>当前账号没有设备维护查看权限。请联系本厂管理员分配 agent:maintenance:view。</p></ContentCard>}
    {((tab !== 'printer' && tab !== 'instrument') || !canManage || deviceOffline || !configData) && feedback}
    {canView && <>
      <div className="flex flex-wrap items-center justify-between gap-space-2 text-body-secondary text-text-tertiary"><p>最近检查：<span className="tabular-nums">{formatDateTime(typeof status?.checkedAt === 'string' ? status.checkedAt : undefined)}</span></p><span>{canManage ? '设备维护管理' : '只读维护'}</span></div>
      <SectionTabs label="设备维护页面" value={tab} onValueChange={value => { setTab(value as typeof tab); if (value === 'records' && !recordsLoaded && !busy) void loadRecords(); }} items={[
        { value: 'overview', label: '运行总览', content: tab === 'overview' ? overview : null },
        { value: 'printer', label: '打印机管理', content: tab === 'printer' ? <div className="grid items-start gap-space-4 xl:grid-cols-2"><div className="min-w-0 space-y-space-4">{printerDiagnosis}{printerStatus}</div>{configuration}</div> : null },
        { value: 'instrument', label: '仪表管理', content: tab === 'instrument' ? <div className="grid items-start gap-space-4 xl:grid-cols-2">{instrumentStatus}{configuration}</div> : null },
        { value: 'backups', label: '配置备份', content: tab === 'backups' ? <MaintenanceBackupPanel canManage={canManage} onApplied={refresh} /> : null },
        { value: 'records', label: '维护记录', content: tab === 'records' ? records : null },
      ]} />
    </>}
  </div>;
}
