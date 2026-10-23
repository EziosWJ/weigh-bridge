import { useCallback, useEffect, useState, type SetStateAction } from 'react';
import { Activity, AlertTriangle, Camera, CheckCircle2, CircleHelp, Play, Printer, RefreshCw, RotateCw, Square } from 'lucide-react';
import { agentMaintenanceApi, type MaintenanceConfig, type CameraTestResult } from '@/api/agent-maintenance';
import { weighingApi } from '@/api/weighing';
import { PageHeader } from '@/components/common/page-header';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { SectionTabs } from '@/components/common/section-tabs';
import { DataTableCard } from '@/components/common/data-table-card';
import { TableToolbar } from '@/components/common/table-toolbar';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { MaintenanceConfigPanel } from './agent-maintenance/config-panel';
import { safeDraft } from './agent-maintenance/config-draft';
import { formatDateTime } from '@/lib/datetime';
import { formatWeightValue } from '@/lib/weighing/weights';
import { useCameraConfiguration } from '@/lib/weighing/camera-preview';
import { CameraLive } from '@/pages/weighing/camera-live';
import { getErrorMessage } from '@/lib/api-error';

type ServiceStatus = Record<string, unknown>;
function CameraDiagnostics({ scaleId }: { scaleId: number }) {
  const preview = useCameraConfiguration(scaleId, true);
  const [view, setView] = useState<'front' | 'rear'>('front');
  const [result, setResult] = useState<CameraTestResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  return <div className="space-y-3">
    <p className="text-sm text-text-secondary">维护测试 · 使用当前生效配置，图片不进入正式称重记录。测试期间暂停新的称重采集和确认。</p>
    <div className="flex flex-wrap gap-2">{(['front', 'rear'] as const).map(value => <Button key={value} size="sm" variant={view === value ? 'primary' : 'secondary'} onClick={() => setView(value)}>{value === 'front' ? '车前实时预览' : '车后实时预览'}</Button>)}</div>
    <CameraLive key={`${view}:${preview.revision ?? ''}`} url={preview.ready ? preview[view] ?? '' : ''} title={`维护测试 · ${view === 'front' ? '车前' : '车后'}`} message={preview.message || (preview.mode === 'legacy' ? '旧抓拍配置尚未切换至 MediaMTX' : undefined)} />
    <Button disabled={busy} onClick={async () => { setBusy(true); setError(''); setResult(null); try { setResult(await agentMaintenanceApi.testCamera()); } catch (failure) { setError(getErrorMessage(failure, '测试抓拍失败')); } finally { setBusy(false); } }}>{busy ? '正在测试抓拍…' : '测试前后抓拍'}</Button>
    {error && <p role="alert" className="text-sm text-error">{error}</p>}
    {result && <div className="grid gap-3 sm:grid-cols-2">{result.views.map(item => <figure key={item.view}><figcaption className="mb-2 text-sm">维护测试 · {item.view === 'front' ? '车前' : '车后'} · {item.success ? '抓拍成功' : item.error}</figcaption>{item.success && item.jpeg && <img src={`data:image/jpeg;base64,${item.jpeg}`} alt={`维护测试${item.view === 'front' ? '车前' : '车后'}抓拍，不作为称重证据`} className="max-w-full rounded-control border border-border" />}<p className="mt-1 text-xs text-text-tertiary">{result.at}</p></figure>)}</div>}
  </div>;
}

function stateValue(value: unknown) {
  if (value === true) return '启用';
  if (value === false) return '停用';
  return (typeof value === 'string' && value) || (typeof value === 'number' && Number.isFinite(value)) ? String(value) : '未知';
}

function StatusValue({ value }: { value: unknown }) {
  const label = stateValue(value);
  const tone = /未知|不可读取/.test(label) ? 'neutral' : /故障|离线|失败/.test(label) ? 'error' : /^(运行中|已启用|已连接|稳定)$/.test(label) ? 'success' : 'warning';
  return <StatusTag tone={tone}>{label}</StatusTag>;
}

const actionLabels: Record<string, string> = {
  'start-agent': '启动设备', 'stop-agent': '停止设备', 'restart-agent': '重载设备',
  'restart-cups': '重启 CUPS', 'enable-queue': '恢复目标队列',
  apply: '应用配置', restore: '恢复上一份配置', 'test-print': '打印校准样张',
  'test-camera': '摄像头测试',
};

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
  const [diagnostics, setDiagnostics] = useState(false);
  const [recordsLoaded, setRecordsLoaded] = useState(false);
  const [tab, setTab] = useState<'overview' | 'devices' | 'config' | 'records'>('overview');
  const deviceOffline = !status || status?.agentProcessOnline === false || status?.maintenanceAvailable === false;
  const canManage = maintenanceEnabled === true && permissionCodes.includes('agent:maintenance:manage');
  const canView = canManage || (maintenanceEnabled === true && permissionCodes.includes('agent:maintenance:view'));

  const refresh = useCallback(async (reloadDraft = false) => {
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
        setConfigData(data); setDraft(current => reloadDraft || !Object.keys(current).length ? safeDraft(data.config) : current);
      }
    } catch (failure) { setError(getErrorMessage(failure, 'Agent 维护状态读取失败')); }
    finally { setInitialLoaded(true); }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  async function runAction(action: string, extra: Record<string, unknown> = {}) {
    setBusy(true); setError(''); setNotice('');
    const actionLabel = actionLabels[action] ?? action;
    let savedCurrentDraft = false;
    try {
      if (action === 'apply') {
        await agentMaintenanceApi.saveConfig(draft);
        savedCurrentDraft = true;
      }
      let preflight: Awaited<ReturnType<typeof agentMaintenanceApi.jobs>> | undefined;
      if (action !== 'start-agent' && action !== 'apply') {
        preflight = await agentMaintenanceApi.jobs();
        if (preflight.unknown) {
          if (!window.confirm(`无法读取未结束打印作业。继续“${actionLabel}”后，请现场核实打印结果。是否继续？`)) { setNotice('操作已取消，设备状态未改变。'); return; }
        } else if (preflight.jobs?.length) {
          const items = preflight.jobs.map((job) => `${job.id}（${job.queue}）`).join('、');
          if (action === 'apply' || action === 'restore') {
            setNotice(`发现未结束作业：${items}。配置草稿已保留，完成现场核实后再应用。`); return;
          }
          if (!window.confirm(`发现未结束作业：${items}。继续后仍需现场核实纸面结果。是否继续“${actionLabel}”？`)) { setNotice('操作已取消，设备状态未改变。'); return; }
        }
      }
      const result = await agentMaintenanceApi.action({ action, ...extra, ...(preflight ? { previewToken: preflight.previewToken, disposition: preflight.unknown ? 'unknown-confirmed' : 'keep', jobIds: [] } : {}) });
      setNotice(result.needsDisposition ? '当前表单已保存；Agent 发现未结束打印作业，尚未应用。请先核实打印，再应用配置。' : result.output || '维护操作已完成；请检查设备实际状态及纸面结果。');
      await refresh(action === 'apply' || action === 'restore');
    } catch (failure) {
      setError(`${savedCurrentDraft ? '当前表单已保存为草稿；' : ''}${getErrorMessage(failure, '维护操作失败')}。请刷新状态核实设备执行结果。`);
      try { setStatus(await agentMaintenanceApi.status()); } catch { setStatus(null); }
      if (savedCurrentDraft) {
        try { setConfigData(await agentMaintenanceApi.config()); } catch { /* Keep the entered values when status cannot be reread. */ }
      }
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
      setLogs(logText); setAudit(auditData.records ?? []); setRecordsLoaded(true);
    } catch (failure) { setError(getErrorMessage(failure, '维护记录读取失败')); }
    finally { setBusy(false); }
  }

  async function refreshStatus() {
    setBusy(true);
    try { await refresh(); } finally { setBusy(false); }
  }

  function editDraft(update: SetStateAction<Record<string, unknown>>) {
    setNotice('');
    setDraft(update);
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
  const camera = (status?.camera && typeof status.camera === 'object' ? status.camera : {}) as ServiceStatus;
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

  const devices = <div className="space-y-space-4">
    <div className="grid items-start gap-space-4 xl:grid-cols-2">
      <ContentCard title="称重仪表" description="查看仪表上报数据；维护页不直接操作串口。" className="min-w-0">
        <div className="rounded-control bg-neutral-background p-space-4">
          <div className="flex flex-wrap items-center justify-between gap-space-2"><p className="text-body-secondary text-text-secondary">最近检查重量</p><StatusValue value={scale.state} /></div>
          <p className="mt-space-1 text-2xl font-semibold tabular-nums">{weightAvailable ? formatWeightValue(weight) : '—'}<span className="ml-space-2 text-sm font-normal text-text-secondary">吨</span></p>
          <p className="mt-space-2 text-body-secondary text-text-tertiary">{weightAvailable ? '仅供维护诊断，以称重工作台的当次采集为准。' : '暂无可用重量，请检查设备执行状态和仪表连接。'}</p>
        </div>
        <div className="mt-space-4 grid grid-cols-2 gap-space-4"><DetailItem label="串口" value={stateValue(scale.port)} /><DetailItem label="波特率" value={stateValue(scale.baud)} /></div>
        <p className={`mt-space-4 break-words border-t border-border pt-space-3 text-body-secondary ${scale.fault ? 'text-warning' : 'text-text-tertiary'}`}>{String(scale.fault || '暂无仪表故障信息')}</p>
      </ContentCard>
      <ContentCard title="打印队列与校准" description="检查队列后打印样张，核对纸张尺寸和位置。" className="min-w-0">
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
      </ContentCard>
    </div>
    <ContentCard title="摄像头" description="配置状态与连接状态分别核实；测试使用当前已生效参数。" extra={<Camera className="h-5 w-5 shrink-0 text-text-tertiary" aria-hidden />}>
      <div className="grid gap-space-4 sm:grid-cols-2">
        {(['front', 'rear'] as const).map(view => <div key={view} className="flex items-center justify-between gap-space-3 rounded-control border border-border p-space-3"><span className="text-sm font-medium">{view === 'front' ? '车前抓拍' : '车后抓拍'}</span><StatusTag tone={camera[`${view}Configured`] ? 'info' : 'neutral'}>{camera[`${view}Configured`] === true ? '已配置' : camera[`${view}Configured`] === false ? '未配置' : '未知'}</StatusTag></div>)}
      </div>
      <p className="mt-space-3 text-body-secondary text-text-secondary">{String(camera.state ?? '尚未读取摄像头状态，请刷新后核实。')}</p>
      {canManage && <div className="mt-space-4 border-t border-border pt-space-4">
        <Button disabled={busy || deviceOffline} aria-expanded={diagnostics} aria-controls="maintenance-camera-diagnostics" onClick={() => setDiagnostics(value => !value)}><Camera className="h-4 w-4" aria-hidden />{diagnostics ? '关闭摄像头测试' : '打开摄像头测试'}</Button>
        {diagnostics && !deviceOffline && <div id="maintenance-camera-diagnostics" className="mt-space-4"><CameraDiagnostics scaleId={Number(configData?.active.scaleId ?? status?.scaleId ?? 1)} /></div>}
      </div>}
    </ContentCard>
  </div>;

  const configuration = !canManage ? <ContentCard title="只读维护权限"><p className="text-sm text-text-secondary">当前账号可以查看运行状态，配置和维护操作需要 agent:maintenance:manage。</p></ContentCard>
    : deviceOffline ? <ContentCard title="Agent 维护暂不可用"><p className="text-sm text-text-secondary">配置保存在 Agent 主机，当前无法读取或修改。请检查该主机的 Agent 运行状态和版本后刷新；页面启动设备只对支持维护的在线 Agent 生效。</p></ContentCard>
    : !configData ? <ContentCard title="配置尚未读取"><p className="text-sm text-text-secondary">请刷新状态重试；读取成功后可编辑设备参数。</p></ContentCard>
    : <MaintenanceConfigPanel config={configData} draft={draft} setDraft={editDraft} busy={busy} feedback={feedback} onSave={() => void saveDraft()} onApply={() => void runAction('apply')} onRestore={() => void runAction('restore')} />;

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
    <PageHeader title="设备维护" description="查看现场设备、调整运行参数并排查维护问题。" actions={<Button disabled={busy} onClick={() => void refreshStatus()}><RefreshCw className={`h-4 w-4 ${busy ? 'motion-safe:animate-spin' : ''}`} aria-hidden />刷新状态</Button>} />
    {!initialLoaded && <ContentCard><p role="status" className="text-sm text-text-secondary">正在检查 Agent 维护状态…</p></ContentCard>}
    {initialLoaded && maintenanceEnabled === false && <ContentCard title="设备维护未启用"><p>当前业务服务未启用设备维护，请联系管理员检查本机维护配置。</p></ContentCard>}
    {initialLoaded && maintenanceEnabled === true && !canView && <ContentCard title="无维护查看权限"><p>当前账号没有设备维护查看权限。请联系本厂管理员分配 agent:maintenance:view。</p></ContentCard>}
    {(tab !== 'config' || !canManage || deviceOffline || !configData) && feedback}
    {canView && <>
      <div className="flex flex-wrap items-center justify-between gap-space-2 text-body-secondary text-text-tertiary"><p>最近检查：<span className="tabular-nums">{formatDateTime(typeof status?.checkedAt === 'string' ? status.checkedAt : undefined)}</span></p><span>{canManage ? '设备维护管理' : '只读维护'}</span></div>
      <SectionTabs label="设备维护页面" value={tab} onValueChange={value => { setTab(value as typeof tab); if (value === 'records' && !recordsLoaded && !busy) void loadRecords(); }} items={[
        { value: 'overview', label: '运行总览', content: tab === 'overview' ? overview : null },
        { value: 'devices', label: '设备与打印', content: tab === 'devices' ? devices : null },
        { value: 'config', label: '配置', content: tab === 'config' ? configuration : null },
        { value: 'records', label: '维护记录', content: tab === 'records' ? records : null },
      ]} />
    </>}
  </div>;
}
