import { useCallback, useEffect, useState } from 'react';
import { agentMaintenanceApi, type BackupRecord, type DeviceSettings, type EffectiveConfig } from '@/api/agent-maintenance';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { DataTableCard } from '@/components/common/data-table-card';
import { TableToolbar } from '@/components/common/table-toolbar';
import { Pagination } from '@/components/common/pagination';
import { FormDialog } from '@/components/common/form-dialog';
import { DetailDialog } from '@/components/common/detail-dialog';
import { Field } from '@/components/common/field';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { formatDateTime } from '@/lib/datetime';
import { getErrorMessage } from '@/lib/api-error';

const fields: { label: string; group: 'serial' | 'print'; key: string }[] = [
  { label: '串口路径', group: 'serial', key: 'port' }, { label: '仪表波特率', group: 'serial', key: 'baud' },
  { label: 'CUPS 打印队列', group: 'print', key: 'queue' }, { label: '单据节长（mm）', group: 'print', key: 'formLengthMm' },
  { label: '水平偏移（mm）', group: 'print', key: 'offsetXmm' }, { label: '垂直偏移（mm）', group: 'print', key: 'offsetYmm' },
  { label: '内容缩放（倍）', group: 'print', key: 'printScale' }, { label: '基准打印宽度（mm）', group: 'print', key: 'printWidthMm' },
];
function setting(settings: DeviceSettings | undefined, group: 'serial' | 'print', key: string) {
  const values = settings?.[group] as Record<string, string | number> | undefined;
  const value = values?.[key];
  return value === '' || value === undefined ? '—' : String(value);
}
function state(value?: string) {
  return ({ running: '运行中', degraded: '降级运行', unavailable: '不可用', configured: '已配置', fault: '故障', ready: '就绪', not_configured: '未配置' } as Record<string, string>)[value ?? ''] ?? '未知';
}
function backupName(record: BackupRecord) { return record.name || `历史备份 #${record.id}`; }

export function MaintenanceBackupPanel({ canManage, onApplied }: { canManage: boolean; onApplied: () => Promise<void> }) {
  const [records, setRecords] = useState<BackupRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [effective, setEffective] = useState<EffectiveConfig>();
  const [live, setLive] = useState(false);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState('');
  const [notes, setNotes] = useState('');
  const [selected, setSelected] = useState<BackupRecord>();
  const [restoring, setRestoring] = useState(false);

  const load = useCallback(async () => {
    setLoaded(false);
    try {
      const result = await agentMaintenanceApi.backups(page);
      setRecords(result.records); setTotal(result.total); setEffective(result.effective); setLive(result.live);
      setLoaded(true);
    } catch (failure) { setLive(false); setError(getErrorMessage(failure, '配置备份读取失败')); }
  }, [page]);
  useEffect(() => { void load(); }, [load]);

  async function create() {
    if (!effective || !live) return;
    setBusy(true); setError(''); setNotice('');
    try {
      await agentMaintenanceApi.createBackup({ name: name.trim(), notes: notes.trim(), baseVersion: effective.configVersion, digest: effective.digest });
      setCreateOpen(false); setName(''); setNotes(''); setNotice('备份已创建，保存的是 Agent 已核实生效的参数。');
      if (page !== 1) setPage(1); else await load();
    } catch (failure) { setError(getErrorMessage(failure, '创建备份失败，请刷新列表核实后再操作')); }
    finally { setBusy(false); }
  }
  async function remove(record: BackupRecord) {
    if (!window.confirm(`删除备份“${backupName(record)}”？当前设备参数不受影响。`)) return;
    setBusy(true); setError(''); setNotice('');
    try {
      await agentMaintenanceApi.deleteBackup(record.id); setNotice('备份已删除。');
      if (records.length === 1 && page > 1) setPage(page - 1); else await load();
    } catch (failure) { setError(getErrorMessage(failure, '删除备份失败')); }
    finally { setBusy(false); }
  }
  async function restore() {
    if (!selected || !effective || !live) return;
    setBusy(true); setError(''); setNotice('');
    try {
      const result = await agentMaintenanceApi.restore(selected.id, effective.configVersion);
      if (result.status === 'applied') {
        setSelected(undefined); setRestoring(false);
        setNotice('已明确发起恢复并核对设备结果；未新增备份，连接地址与运行状态未被覆盖。');
        await onApplied();
      } else setError(`恢复未完成（${result.status}）：${result.message || '请核对设备实际状态'}。`);
      await load();
    } catch (failure) {
      setError(`${getErrorMessage(failure, '恢复失败')}。结果未知时请刷新核实，不要重复恢复。`);
      setLive(false);
    } finally { setBusy(false); }
  }
  const feedback = <>
    {error && <p role="alert" className="rounded-control border border-error-border bg-error-background p-space-3 text-sm text-error">{error}</p>}
    {notice && <p role="status" className="rounded-control border border-info-border bg-info-background p-space-3 text-sm">{notice}</p>}
  </>;
  const comparison = selected && <div className="overflow-x-auto"><table className="w-full min-w-[480px] text-left text-sm">
    <caption className="sr-only">备份与当前生效参数对比</caption>
    <thead><tr className="border-b border-border">{['参数', live ? '当前生效值' : '最近上报值', '备份值'].map(label => <th key={label} className="px-space-3 py-space-2 font-medium">{label}</th>)}</tr></thead>
    <tbody>{fields.map(field => {
      const current = setting(effective, field.group, field.key);
      const saved = setting(selected.settings, field.group, field.key);
      return <tr key={field.key} className="border-b border-border"><td className="px-space-3 py-space-2">{field.label}</td><td className="break-all px-space-3 py-space-2">{current}</td><td className={`break-all px-space-3 py-space-2 ${current !== saved ? 'font-medium text-warning' : ''}`}>{saved}{current !== saved && <span className="ml-space-2 text-xs">（不同）</span>}</td></tr>;
    })}</tbody>
  </table></div>;
  return <div className="space-y-space-4">
    {feedback}
    <ContentCard title="当前生效配置" description={live ? '来自 Agent 当前连接的生效状态；保存应用不会自动创建备份。' : 'Agent 离线或状态未确认，仅展示最近上报信息，不能创建或恢复备份。'} extra={<Button disabled={busy} onClick={() => { setError(''); void load(); }}>刷新备份与状态</Button>}>
      {effective ? <>
        <div className="grid gap-space-4 sm:grid-cols-3"><DetailItem label="生效版本" value={`v${effective.configVersion}`} /><DetailItem label="最近生效时间" value={formatDateTime(effective.appliedAt)} /><DetailItem label="仪表／打印" value={live ? `${state(effective.serialState)}／${state(effective.printerState)}` : '离线／未确认'} /></div>
        <div className="mt-space-4 grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">{fields.map(field => <DetailItem key={field.key} label={field.label} value={setting(effective, field.group, field.key)} />)}</div>
      </> : <p className="text-sm text-text-secondary">尚无已核实生效参数。</p>}
      {canManage && <div className="mt-space-4 border-t border-border pt-space-4"><Button variant="primary" disabled={busy || !loaded || !live || !effective?.appliedAt} onClick={() => setCreateOpen(true)}>创建备份</Button></div>}
    </ContentCard>
    <DataTableCard toolbar={<TableToolbar title="配置备份列表" description="手动保留仪表与打印参数；恢复不会自动新增备份。历史自动备份继续可用。" />}>
      <div className="overflow-x-auto"><table className="w-full min-w-[900px] text-left text-sm">
        <caption className="sr-only">设备配置备份列表</caption>
        <thead className="bg-neutral-background"><tr>{['备份名称／备注', '来源生效版本', '创建时间', '创建人', '来源', '操作'].map(label => <th key={label} className="px-space-4 py-space-3 font-medium">{label}</th>)}</tr></thead>
        <tbody>{records.map(record => <tr key={record.id} className="border-t border-border">
          <td className="max-w-xs break-words px-space-4 py-space-3"><p>{backupName(record)}</p><p className="text-body-secondary text-text-tertiary">{record.notes || '—'}</p></td>
          <td className="px-space-4 py-space-3">v{record.configVersion}</td><td className="whitespace-nowrap px-space-4 py-space-3">{formatDateTime(record.receivedAt)}</td><td className="px-space-4 py-space-3">{record.createdBy ? `用户 #${record.createdBy}` : '历史系统记录'}</td><td className="px-space-4 py-space-3">{record.source === 'manual' ? '手动备份' : record.source === 'restore' ? '历史恢复记录' : '历史自动备份'}</td>
          <td className="px-space-4 py-space-3"><div className="flex gap-space-2"><Button size="sm" disabled={busy || !loaded} onClick={() => { setSelected(record); setRestoring(false); }}>查看</Button>{canManage && <><Button size="sm" disabled={busy || !loaded || !live || !effective} onClick={() => { setSelected(record); setRestoring(true); }}>恢复</Button><Button size="sm" disabled={busy || !loaded} onClick={() => void remove(record)}>删除</Button></>}</div></td>
        </tr>)}</tbody>
      </table></div>
      {!records.length && <p className="p-space-5 text-center text-sm text-text-tertiary">{!loaded ? (error ? '读取失败，请刷新重试。' : '正在读取备份…') : '暂无配置备份，请在参数核实后手动创建。'}</p>}
      <Pagination page={page} pageSize={10} total={total} disabled={busy || !loaded} onPageChange={setPage} />
    </DataTableCard>
    <FormDialog open={createOpen} title="创建设备配置备份" description="仅保存 Agent 已生效的仪表与打印参数，不包含尚未应用的表单修改。" submitText="创建备份" loading={busy} submitDisabled={!name.trim() || !live || !loaded} onCancel={() => setCreateOpen(false)} onSubmit={create}>
      {error && feedback}
      <div className="space-y-space-4"><Field label="备份名称" htmlFor="backup-name"><Input id="backup-name" value={name} maxLength={100} onChange={event => setName(event.target.value)} /></Field><Field label="备份备注" htmlFor="backup-notes"><Input id="backup-notes" value={notes} maxLength={500} onChange={event => setNotes(event.target.value)} /></Field></div>
    </FormDialog>
    {restoring ? <FormDialog open={!!selected} title="恢复设备配置备份" description={`将恢复“${selected ? backupName(selected) : ''}”，同时覆盖仪表与打印参数；不会恢复连接地址、运行状态或重放打印。`} submitText="确认恢复" loading={busy} submitDisabled={!live || !loaded} onCancel={() => setSelected(undefined)} onSubmit={restore}>
      {error && feedback}{comparison}
    </FormDialog> : <DetailDialog open={!!selected} title="配置备份详情" description={selected ? `${backupName(selected)} · ${selected.notes || '无备注'}` : undefined} onCancel={() => setSelected(undefined)}>{comparison}</DetailDialog>}
  </div>;
}
