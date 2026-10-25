import { useCallback, useEffect, useState } from 'react';
import { Activity, AlertTriangle, CheckCircle2, CircleHelp, Play, Printer, RefreshCw, RotateCw, Square } from 'lucide-react';
import { agentMaintenanceApi, type CupsQueue, type DeviceSettings, type MaintenanceConfig } from '@/api/agent-maintenance';
import { weighingApi } from '@/api/weighing';
import { PageHeader } from '@/components/common/page-header';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { SectionTabs } from '@/components/common/section-tabs';
import { DataTableCard } from '@/components/common/data-table-card';
import { TableToolbar } from '@/components/common/table-toolbar';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { MaintenanceBackupPanel } from './agent-maintenance/backup-panel';
import { MaintenanceConfigPanel } from './agent-maintenance/config-panel';
import { formatDateTime } from '@/lib/datetime';
import { formatWeightValue } from '@/lib/weighing/weights';
import { getErrorMessage } from '@/lib/api-error';

type ServiceStatus = Record<string, unknown>;

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
      queue: String(active.queue ?? ''), formLengthMm: numberOr(active.formLengthMm, 99),
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
  const [notice, setNotice] = useState('');
  const [recordsLoaded, setRecordsLoaded] = useState(false);
  const [tab, setTab] = useState<'overview' | 'printer' | 'instrument' | 'backups' | 'records'>('overview');
  const deviceOffline = !status || status?.agentProcessOnline === false || status?.maintenanceAvailable === false;
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
        setConfigData(data); setSettings(settingsFromConfig(data));
        try {
          const read = await agentMaintenanceApi.queues();
          setQueues(read.queues ?? []); setQueuesError(read.error || '');
        } catch (failure) {
          setQueues([]); setQueuesError(getErrorMessage(failure, 'Agent 打印队列读取失败'));
        }
      }
    } catch (failure) { setError(getErrorMessage(failure, 'Agent 维护状态读取失败')); }
    finally { setInitialLoaded(true); }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  async function runAction(action: string, extra: Record<string, unknown> = {}) {
    setBusy(true); setError(''); setNotice('');
    const actionLabel = actionLabels[action] ?? action;
    try {
      let preflight: Awaited<ReturnType<typeof agentMaintenanceApi.jobs>> | undefined;
      if (action !== 'start-agent') {
        preflight = await agentMaintenanceApi.jobs();
        if (preflight.unknown) {
          if (!window.confirm(`无法读取未结束打印作业。继续“${actionLabel}”后，请现场核实打印结果。是否继续？`)) { setNotice('操作已取消，设备状态未改变。'); return; }
        } else if (preflight.jobs?.length) {
          const items = preflight.jobs.map((job) => `${job.id}（${job.queue}）`).join('、');
          if (!window.confirm(`发现未结束作业：${items}。继续后仍需现场核实纸面结果。是否继续“${actionLabel}”？`)) { setNotice('操作已取消，设备状态未改变。'); return; }
        }
      }
      const result = await agentMaintenanceApi.action({ action, ...extra, ...(preflight ? { previewToken: preflight.previewToken, disposition: preflight.unknown ? 'unknown-confirmed' : 'keep', jobIds: [] } : {}) });
      setNotice(result.needsDisposition ? 'Agent 发现未结束打印作业，尚未执行。请先核实打印后再操作。' : result.output || '维护操作已完成；请检查设备实际状态及纸面结果。');
      await refresh();
    } catch (failure) {
      setError(`${getErrorMessage(failure, '维护操作失败')}。请刷新状态核实设备执行结果。`);
      try { setStatus(await agentMaintenanceApi.status()); } catch { setStatus(null); }
    } finally { setBusy(false); }
  }

  async function applySettings() {
    if (!settings || !configData) return;
    setBusy(true); setError(''); setNotice('');
    try {
      const result = await agentMaintenanceApi.apply(settings, configData.effective?.configVersion ?? 0);
      if (result.status === 'applied') {
        setNotice('配置已保存并应用，设备重载结果已核实。已有打印作业仍须现场核实。');
      } else {
        setError(`配置未生效（${result.status}）：${result.message || '请核对设备实际状态'}。维护连接保持，可修正后重试。`);
      }
      await refresh();
    } catch (failure) {
      setError(`${getErrorMessage(failure, '配置应用失败')}。结果未知时请先读取实际状态核实，不要重复盲写。`);
      try { setConfigData(await agentMaintenanceApi.config()); } catch { /* Keep entered values for correction. */ }
    } finally { setBusy(false); }
  }

  async function loadRecords() {
    setBusy(true); setError('');
    try {
      const [logText, auditData] = await Promise.all([
        agentMaintenanceApi.logs(),
        agentMaintenanceApi.audit(new URLSearchParams({ offset: '0', limit: '100' })),
      ]);
      setLogs(logText); setAudit(auditData.records ?? []); setRecordsLoaded(true);
    } catch (failure) { setError(getErrorMessage(failure, '维护记录读取失败')); }
    finally { setBusy(false); }
  }

  async function printSample() {
    if (!window.confirm('将提交一份固定校准样张，不会重打业务单据。是否继续？')) return;
    setBusy(true); setError(''); setNotice('');
    try {
      const result = await agentMaintenanceApi.testPrint();
      setNotice(result.message || '校准样张请求已提交，请核对纸面效果。');
      await refresh();
    } catch (failure) {
      setError(`${getErrorMessage(failure, '测试打印失败')}。结果可能未知，请先核实是否出纸。`);
    } finally { setBusy(false); }
  }

  const statusJobs = Array.isArray(status?.jobs) ? status.jobs as { id: string; queue?: string }[] : [];
  const scale = (status?.scale && typeof status.scale === 'object' ? status.scale : {}) as ServiceStatus;
  const serviceCards = [
    { label: '业务接口', value: status?.maintenance, detail: '本厂业务服务', icon: Activity },
    { label: '设备执行', value: status?.agentKnown ? status.agent : '未知', detail: '重量采集与打印执行', icon: Play },
    { label: 'CUPS 服务', value: status?.cupsKnown ? status.cups : '未知', detail: '本机打印服务', icon: Printer },
    { label: '打印队列', value: status?.queueKnown ? status.queueState : '未知', detail: status?.queueKnown ? stateValue(status.queue) : '暂未读取到队列', icon: Printer },
  ];
  const weight = typeof scale.weight === 'string' && scale.weight.trim() !== '' || typeof scale.weight === 'number' ? Number(scale.weight) : NaN;
  const weightAvailable = !deviceOffline && status?.agent === '运行中' && scale.known === true && scale.deviceOk === true && scale.stale === false && Number.isFinite(weight);
  const lastError = typeof status?.lastError === 'string' ? status.lastError : '';
  const statusIncomplete = !status || status.agentKnown !== true || status.cupsKnown !== true || status.queueKnown !== true;
  const DiagnosisIcon = lastError ? AlertTriangle : statusIncomplete ? CircleHelp : CheckCircle2;
  const fault = typeof scale.fault === 'string' ? scale.fault : '';
  const serialHint = /权限|permission/i.test(fault) ? fault : '';
  const feedback = <>
    {error && <p role="alert" className="rounded-control border border-error-border bg-error-background p-space-3 text-sm text-error">{error}</p>}
    {notice && <p role="status" className="rounded-control border border-info-border bg-info-background p-space-3 text-sm text-text-primary">{notice}</p>}
  </>;

  const overview = <div className="space-y-space-4">
    <ContentCard bodyClassName="p-0">
      <div className="grid sm:grid-cols-2 xl:grid-cols-4">
        {serviceCards.map(({ label, value, detail, icon: Icon }) => <div key={label} className="min-w-0 space-y-space-3 border-b border-border p-space-4 last:border-b-0 sm:[&:nth-last-child(-n+2)]:border-b-0 sm:odd:border-r xl:border-b-0 xl:[&:not(:last-child)]:border-r">
          <div className="flex items-center gap-space-2 text-sm font-medium"><Icon className="h-4 w-4 text-text-tertiary" aria-hidden />{label}</div>
          <StatusValue value={value} />
          <p className="break-all text-body-secondary text-text-tertiary">{detail}</p>
        </div>)}
      </div>
    </ContentCard>
    <div className="grid items-start gap-space-4 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
      <ContentCard title="现场诊断" description="查看异常和未结束作业，再执行相应维护操作。">
        <div className="flex items-start gap-space-3">
          <DiagnosisIcon className={`mt-0.5 h-5 w-5 shrink-0 ${lastError ? 'text-warning' : statusIncomplete ? 'text-text-tertiary' : 'text-success'}`} aria-hidden />
          <div className="min-w-0"><p className="break-words text-sm font-medium">{!status ? '尚未读取设备状态' : lastError || (statusIncomplete ? '部分运行状态尚未确认' : '未报告设备错误')}</p><p className="mt-space-1 text-body-secondary text-text-tertiary">{deviceOffline ? '请检查 Agent 主机上的服务运行状态，恢复连接后刷新。' : '状态来自最近一次检查；需要时刷新以核实现场变化。'}</p></div>
        </div>
        {serialHint && <p className="mt-space-4 break-words rounded-control border border-error-border bg-error-background p-space-3 text-sm text-error">{serialHint}</p>}
        <div className="mt-space-5 border-t border-border pt-space-4">
          <div className="flex flex-wrap items-center justify-between gap-space-2"><h3 className="text-sm font-medium">未结束打印作业</h3><StatusTag tone={status?.jobsKnown !== true || statusJobs.length ? 'warning' : 'neutral'}>{status?.jobsKnown !== true ? '状态未知' : statusJobs.length ? `${statusJobs.length} 项待核实` : '未发现'}</StatusTag></div>
          {status?.jobsKnown !== true ? <p className="mt-space-2 text-body-secondary text-text-secondary">未能确认作业状态，请先检查队列和现场纸面结果。</p> : statusJobs.length ? <ul className="mt-space-3 divide-y divide-border">{statusJobs.map(job => <li key={job.id} className="flex flex-wrap justify-between gap-space-2 py-space-2 text-sm"><span className="break-all font-medium">{job.id}</span><span className="break-all text-text-secondary">{job.queue ?? '队列未知'}，纸面结果需核实</span></li>)}</ul> : <p className="mt-space-2 text-body-secondary text-text-tertiary">本次检查未发现未结束的打印作业。</p>}
        </div>
      </ContentCard>
      <ContentCard title="设备执行控制" description="控制重量采集与打印执行，Agent 维护连接保留。" className="min-w-0">
        <DetailItem label="当前执行状态" value={<StatusValue value={status?.agentKnown ? status.agent : '未知'} />} />
        {canManage ? <>
          <div className="mt-space-4 flex flex-wrap gap-space-2">
            <Button variant="primary" disabled={busy || deviceOffline || status?.agent === '运行中'} onClick={() => void runAction('start-agent')}><Play className="h-4 w-4" aria-hidden />启动设备</Button>
            <Button disabled={busy || deviceOffline || status?.agent !== '运行中'} onClick={() => void runAction('stop-agent')}><Square className="h-4 w-4" aria-hidden />停止设备</Button>
            <Button disabled={busy || deviceOffline} onClick={() => void runAction('restart-agent')}><RotateCw className="h-4 w-4" aria-hidden />重载设备</Button>
          </div>
          <p className="mt-space-3 text-body-secondary text-text-tertiary">{deviceOffline ? 'Agent 维护连接不可用，请在其主机恢复服务后刷新。' : '停止设备后暂停采集和打印，可在此重新启动。'}</p>
        </> : <p className="mt-space-4 text-body-secondary text-text-tertiary">当前为只读维护权限；设备控制需由维护管理员操作。</p>}
      </ContentCard>
    </div>
  </div>;

  const instrumentStatus = <ContentCard title="称重仪表" description="查看仪表上报数据；维护页不直接操作串口。" className="min-w-0">
    <div className="rounded-control bg-neutral-background p-space-4">
      <div className="flex flex-wrap items-center justify-between gap-space-2"><p className="text-body-secondary text-text-secondary">最近检查重量</p><StatusValue value={scale.state} /></div>
      <p className="mt-space-1 text-2xl font-semibold tabular-nums">{weightAvailable ? formatWeightValue(weight) : '—'}<span className="ml-space-2 text-sm font-normal text-text-secondary">吨</span></p>
      <p className="mt-space-2 text-body-secondary text-text-tertiary">{weightAvailable ? '仅供维护诊断，以称重工作台的当次采集为准。' : '暂无可用重量，请检查设备执行状态和仪表连接。'}</p>
    </div>
    <div className="mt-space-4 grid grid-cols-2 gap-space-4"><DetailItem label="串口" value={stateValue(scale.port)} /><DetailItem label="波特率" value={stateValue(scale.baud)} /></div>
    <p className={`mt-space-4 break-words border-t border-border pt-space-3 text-body-secondary ${scale.fault ? 'text-warning' : 'text-text-tertiary'}`}>{String(scale.fault || '暂无仪表故障信息')}</p>
  </ContentCard>;

  const printerStatus = <ContentCard title="打印队列与校准" description="检查队列后打印样张，核对纸张尺寸和位置。" className="min-w-0">
    <div className="grid grid-cols-2 gap-space-4">
      <DetailItem label="目标队列" value={status?.queueKnown ? stateValue(status.queue) : '未知'} />
      <DetailItem label="队列状态" value={<StatusValue value={status?.queueKnown ? status.queueState : '未知'} />} />
      <DetailItem label="CUPS 服务" value={<StatusValue value={status?.cupsKnown ? status.cups : '未知'} />} />
      <DetailItem label="未结束作业" value={status?.jobsKnown === true ? `${statusJobs.length} 项` : '状态未知'} />
    </div>
    {canManage && <div className="mt-space-5 flex flex-wrap gap-space-2 border-t border-border pt-space-4">
      <Button variant="primary" disabled={busy || deviceOffline} onClick={() => void printSample()}><Printer className="h-4 w-4" aria-hidden />打印校准样张</Button>
      <Button disabled={busy || deviceOffline} onClick={() => void runAction('enable-queue')}>恢复目标队列</Button>
      <Button disabled={busy || deviceOffline} onClick={() => void runAction('restart-cups')}>重启 CUPS</Button>
    </div>}
    <p className="mt-space-3 text-body-secondary text-text-tertiary">样张不属于业务单据。打印结果未知时先核实是否出纸；取消作业不代表此前没有出纸。</p>
  </ContentCard>;

  const configuration = !canManage ? <ContentCard title="只读维护权限">
    <DetailItem label="Agent 连接的业务服务地址" value={typeof status?.server === 'string' && status.server ? status.server : '未知'} className="[&>div]:break-all" />
    <p className="mt-space-2 text-body-secondary text-text-secondary">该地址从 Agent 主机视角解释：同机可用回环地址，分机须为业务服务的局域网 IP 或域名。修改地址需在设备主机使用本机配置工具。</p>
    <p className="mt-space-3 text-sm text-text-secondary">当前账号可以查看运行状态，配置和维护操作需要 agent:maintenance:manage。</p>
  </ContentCard>
    : deviceOffline ? <ContentCard title="Agent 维护暂不可用"><p className="text-sm text-text-secondary">配置保存在 Agent 主机，当前无法读取或修改。请检查该主机的 Agent 运行状态和版本后刷新。</p></ContentCard>
    : !configData || !settings ? <ContentCard title="配置尚未读取"><p className="text-sm text-text-secondary">请刷新状态重试；读取成功后可编辑设备参数。</p></ContentCard>
    : <MaintenanceConfigPanel section={tab === 'instrument' ? 'serial' : 'print'} config={configData} settings={settings} setSettings={value => setSettings(current => typeof value === 'function' ? value(current ?? settings) : value)} queues={queues} queuesError={queuesError} serialHint={serialHint} busy={busy} feedback={feedback} onApply={() => void applySettings()} />;

  const records = <div className="space-y-space-4">
    <DataTableCard toolbar={<TableToolbar title="维护记录" description="最近 100 条维护操作，按时间核对操作目标及执行结果。" actions={<Button disabled={busy} onClick={() => void loadRecords()}><RefreshCw className="h-4 w-4" aria-hidden />刷新记录</Button>} />}>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[640px] text-left text-sm"><caption className="sr-only">设备维护操作记录</caption>
          <thead className="bg-neutral-background text-text-secondary"><tr className="border-b border-border">{['时间', '操作', '目标', '结果'].map(label => <th key={label} scope="col" className="px-space-4 py-space-3 font-medium">{label}</th>)}</tr></thead>
          <tbody>{audit.map((row, index) => <tr key={String(row.id ?? index)} className="border-b border-border last:border-b-0 hover:bg-neutral-background">
            <td className="whitespace-nowrap px-space-4 py-space-3 tabular-nums text-text-secondary">{formatDateTime(typeof row.at === 'string' ? row.at : undefined)}</td>
            <td className="px-space-4 py-space-3 font-medium">{actionLabels[String(row.action)] ?? String(row.action ?? '—')}</td>
            <td className="max-w-xs break-words px-space-4 py-space-3 text-text-secondary">{String(row.target || '—')}</td>
            <td className="max-w-sm break-words px-space-4 py-space-3">{String(row.result || '—')}</td>
          </tr>)}</tbody>
        </table>
      </div>
      {!audit.length && <div className="px-space-4 py-space-8 text-center text-sm text-text-tertiary">{busy ? '正在读取维护记录…' : recordsLoaded ? '暂无维护记录' : '尚未读取记录，请点击刷新记录。'}</div>}
    </DataTableCard>
    <ContentCard title="运行日志" description="最近的设备运行输出，用于排查连接与执行问题。" extra={<Button disabled={busy} onClick={async () => { setBusy(true); try { setLogs(await agentMaintenanceApi.logs()); } catch (failure) { setLogs(getErrorMessage(failure, '日志读取失败')); } finally { setBusy(false); } }}>读取最近日志</Button>} className="min-w-0">
      <pre aria-label="运行日志内容" className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-control border border-border bg-neutral-background p-space-4 text-body-secondary leading-relaxed text-text-secondary">{logs || '本次未读取到日志内容。'}</pre>
    </ContentCard>
  </div>;

  return <div className="min-w-0 space-y-space-4">
    <PageHeader title="设备维护" description="查看现场设备、调整串口与打印参数，并核实实际生效结果。" actions={<Button disabled={busy} onClick={() => void refresh()}><RefreshCw className={`h-4 w-4 ${busy ? 'motion-safe:animate-spin' : ''}`} aria-hidden />刷新状态</Button>} />
    {!initialLoaded && <ContentCard><p role="status" className="text-sm text-text-secondary">正在检查 Agent 维护状态…</p></ContentCard>}
    {initialLoaded && maintenanceEnabled === false && <ContentCard title="设备维护未启用"><p>当前业务服务未启用设备维护，请联系管理员检查本机维护配置。</p></ContentCard>}
    {initialLoaded && maintenanceEnabled === true && !canView && <ContentCard title="无维护查看权限"><p>当前账号没有设备维护查看权限。请联系本厂管理员分配 agent:maintenance:view。</p></ContentCard>}
    {((tab !== 'printer' && tab !== 'instrument') || !canManage || deviceOffline || !configData) && feedback}
    {canView && <>
      <div className="flex flex-wrap items-center justify-between gap-space-2 text-body-secondary text-text-tertiary"><p>最近检查：<span className="tabular-nums">{formatDateTime(typeof status?.checkedAt === 'string' ? status.checkedAt : undefined)}</span></p><span>{canManage ? '设备维护管理' : '只读维护'}</span></div>
      <SectionTabs label="设备维护页面" value={tab} onValueChange={value => { setTab(value as typeof tab); if (value === 'records' && !recordsLoaded && !busy) void loadRecords(); }} items={[
        { value: 'overview', label: '运行总览', content: tab === 'overview' ? overview : null },
        { value: 'printer', label: '打印机管理', content: tab === 'printer' ? <div className="grid items-start gap-space-4 xl:grid-cols-2">{printerStatus}{configuration}</div> : null },
        { value: 'instrument', label: '仪表管理', content: tab === 'instrument' ? <div className="grid items-start gap-space-4 xl:grid-cols-2">{instrumentStatus}{configuration}</div> : null },
        { value: 'backups', label: '配置备份', content: tab === 'backups' ? <MaintenanceBackupPanel canManage={canManage} onApplied={refresh} /> : null },
        { value: 'records', label: '维护记录', content: tab === 'records' ? records : null },
      ]} />
    </>}
  </div>;
}
