import { useCallback, useEffect, useState } from 'react';
import { agentMaintenanceApi, type BackupRecord, type EffectiveConfig } from '@/api/agent-maintenance';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { DataTable } from '@/components/common/data-table';
import { DataTableCard } from '@/components/common/data-table-card';
import { TableActions } from '@/components/common/table-actions';
import { TableToolbar } from '@/components/common/table-toolbar';
import { Pagination } from '@/components/common/pagination';
import { FormDialog } from '@/components/common/form-dialog';
import { DetailDialog } from '@/components/common/detail-dialog';
import { Field } from '@/components/common/field';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/common/toast-store';
import { Input } from '@/components/ui/input';
import { formatDateTime } from '@/lib/datetime';
import { getErrorMessage } from '@/lib/api-error';
import { fields, setting } from './settings-fields';
import { SettingsComparison } from './settings-comparison';
import { applyResultLabel, appliedVersion } from './apply-result';
import type { DataTableColumn } from '@/types';

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
  const [loadError, setLoadError] = useState('');
  const [error, setError] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState('');
  const [notes, setNotes] = useState('');
  const [selected, setSelected] = useState<BackupRecord>();
  const [restoring, setRestoring] = useState(false);

  const load = useCallback(async () => {
    setLoaded(false);
    setLoadError('');
    try {
      const result = await agentMaintenanceApi.backups(page);
      setRecords(result.records); setTotal(result.total); setEffective(result.effective); setLive(result.live);
      setLoaded(true);
    } catch (failure) { setLive(false); setLoadError(getErrorMessage(failure, '配置备份读取失败')); }
  }, [page]);
  useEffect(() => { void load(); }, [load]);

  async function create() {
    if (!effective || !live) return;
    setBusy(true); setError('');
    try {
      await agentMaintenanceApi.createBackup({ name: name.trim(), notes: notes.trim(), baseVersion: effective.configVersion, digest: effective.digest });
      setCreateOpen(false); setName(''); setNotes(''); toast.success('备份已创建，保存的是 Agent 已核实生效的参数。');
      if (page !== 1) setPage(1); else await load();
    } catch (failure) { setError(getErrorMessage(failure, '创建备份失败，请刷新列表核实后再操作')); }
    finally { setBusy(false); }
  }
  async function remove(record: BackupRecord) {
    if (!window.confirm(`删除备份“${backupName(record)}”？删除后无法再选择此备份恢复。`)) return;
    setBusy(true); setError('');
    try {
      await agentMaintenanceApi.deleteBackup(record.id); toast.success('备份已删除。');
      if (records.length === 1 && page > 1) setPage(page - 1); else await load();
    } catch (failure) { setError(getErrorMessage(failure, '删除备份失败')); }
    finally { setBusy(false); }
  }
  async function restore() {
    if (!selected || !effective || !live) return;
    setBusy(true); setError('');
    try {
      const result = await agentMaintenanceApi.restore(selected.id, effective.configVersion);
      if (result.status === 'applied') {
        setSelected(undefined); setRestoring(false);
        toast.success({ title: '配置已恢复并核实', description: appliedVersion(result), duration: 5000 });
        await onApplied();
      } else {
        setError(`${applyResultLabel(result.status)}${result.message ? `：${result.message}` : ''}`);
        if (result.status === 'unknown') {
          setSelected(undefined); setRestoring(false); setLive(false);
          return;
        }
      }
      await load();
    } catch (failure) {
      setError(`${getErrorMessage(failure, '恢复失败')}。结果未知时请刷新核实，不要重复恢复。`);
      setSelected(undefined); setRestoring(false); setLive(false);
    } finally { setBusy(false); }
  }
  const feedback = <>
    {error && <p role="alert" className="rounded-control border border-error-border bg-error-background p-space-3 text-sm text-error">{error}</p>}
  </>;
  const comparison = selected && <SettingsComparison current={effective} next={selected.settings}
    currentLabel={live ? '当前生效值' : '最近上报值'} nextLabel="备份值" />;
  const columns: DataTableColumn<BackupRecord>[] = [
    { title: '备份名称／备注', key: 'name', render: (_, record) => <>
      <p className="truncate font-medium" title={backupName(record)}>{backupName(record)}</p>
      {record.notes && <p className="truncate text-body-secondary text-text-tertiary" title={record.notes}>{record.notes}</p>}
    </> },
    { title: '生效版本', key: 'version', width: 88, nowrap: true, render: (_, record) => <span className="tabular-nums">v{record.configVersion}</span> },
    { title: '创建时间', key: 'time', width: 172, nowrap: true, render: (_, record) => <span className="tabular-nums">{formatDateTime(record.receivedAt)}</span> },
    { title: '创建人', key: 'createdBy', width: 110, render: (_, record) => record.createdBy ? `用户 #${record.createdBy}` : '历史系统记录' },
    { title: '来源', key: 'source', width: 116, render: (_, record) => record.source === 'manual' ? '手动备份' : record.source === 'restore' ? '历史恢复记录' : '历史自动备份' },
    { title: '操作', key: 'actions', width: canManage ? 205 : 90, nowrap: true, render: (_, record) => <TableActions>
      <Button size="sm" disabled={busy} onClick={() => { setSelected(record); setRestoring(false); }}>查看</Button>
      {canManage && <>
        <Button size="sm" variant="ghost" disabled={busy || !live || !effective} onClick={() => { setError(''); setSelected(record); setRestoring(true); }}>恢复</Button>
        <Button size="sm" variant="ghost" className="text-error hover:bg-error-background hover:text-error" disabled={busy} onClick={() => void remove(record)}>删除</Button>
      </>}
    </TableActions> },
  ];
  return <div className="space-y-space-4">
    {feedback}
    <ContentCard title={live ? '当前生效配置' : '最近上报配置'} extra={<Button disabled={busy || (!loaded && !loadError)} onClick={() => { setError(''); void load(); }}>刷新备份与状态</Button>}>
      {effective ? <>
        <div className="grid gap-space-4 sm:grid-cols-3"><DetailItem label="生效版本" value={`v${effective.configVersion}`} /><DetailItem label="最近生效时间" value={formatDateTime(effective.appliedAt)} /><DetailItem label="仪表／打印" value={live ? `${state(effective.serialState)}／${state(effective.printerState)}` : '离线／未确认'} /></div>
        <div className="mt-space-4 grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4">{fields.map(field => <DetailItem key={field.key} label={field.label} value={setting(effective, field.group, field.key)} />)}</div>
      </> : <p className="text-sm text-text-secondary">尚无已核实生效参数。</p>}
      {canManage && <div className="mt-space-4 border-t border-border pt-space-4"><Button variant="primary" disabled={busy || !loaded || !live || !effective?.appliedAt} onClick={() => setCreateOpen(true)}>创建备份</Button></div>}
    </ContentCard>
    <DataTableCard toolbar={<TableToolbar title="配置备份列表" />}>
      <DataTable ariaLabel="设备配置备份列表" columns={columns} dataSource={records} rowKey="id" loading={!loaded && !loadError}
        error={loadError} minWidth={canManage ? 860 : 745} compact stickyHeader fixedEdges
        empty={<p className="p-space-5 text-center text-sm text-text-tertiary">{canManage ? '暂无配置备份，请在参数核实后手动创建。' : '暂无配置备份。'}</p>} />
      <Pagination page={page} pageSize={10} total={total} disabled={busy || !loaded} onPageChange={setPage} />
    </DataTableCard>
    <FormDialog open={createOpen} title="创建设备配置备份" submitText="创建备份" loading={busy} submitDisabled={!name.trim() || !live || !loaded} onCancel={() => setCreateOpen(false)} onSubmit={create}>
      {error && feedback}
      <div className="space-y-space-4"><Field label="备份名称" htmlFor="backup-name"><Input id="backup-name" value={name} maxLength={100} onChange={event => setName(event.target.value)} /></Field><Field label="备份备注" htmlFor="backup-notes"><Input id="backup-notes" value={notes} maxLength={500} onChange={event => setNotes(event.target.value)} /></Field></div>
    </FormDialog>
    {restoring ? <FormDialog open={!!selected} title="恢复设备配置备份" description={`将恢复“${selected ? backupName(selected) : ''}”，并重新应用仪表与打印参数。`} submitText="确认恢复" loading={busy} submitDisabled={!live || !loaded} onCancel={() => setSelected(undefined)} onSubmit={restore}>
      {error && feedback}
      {selected && <SettingsComparison current={effective} next={selected.settings} nextLabel="恢复后" changesOnly />}
      <details className="mt-4"><summary className="mb-2 cursor-pointer text-sm text-primary">全部参数</summary>{comparison}</details>
    </FormDialog> : <DetailDialog open={!!selected} title="配置备份详情" description={selected ? `${backupName(selected)} · ${selected.notes || '无备注'}` : undefined} onCancel={() => setSelected(undefined)}>{comparison}</DetailDialog>}
  </div>;
}
