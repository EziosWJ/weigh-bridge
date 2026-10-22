import { useCallback, useEffect, useState } from 'react';
import { siteConfigurationApi, type SiteConfiguration, type SiteConfigurationInput } from '@/api/site-configuration';
import { weighingApi } from '@/api/weighing';
import { PageHeader } from '@/components/common/page-header';
import { SearchFilterBar } from '@/components/common/search-filter-bar';
import { ContentCard } from '@/components/common/content-card';
import { DataTableCard } from '@/components/common/data-table-card';
import { DataTable } from '@/components/common/data-table';
import { TableToolbar } from '@/components/common/table-toolbar';
import { TableActions } from '@/components/common/table-actions';
import { Pagination } from '@/components/common/pagination';
import { FormDialog } from '@/components/common/form-dialog';
import { Field } from '@/components/common/field';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { getErrorMessage } from '@/lib/api-error';
import type { DataTableColumn } from '@/types';
import './list.css';
import { nodeApi, type PendingConfiguration } from '@/api/node';

const empty: SiteConfigurationInput = {code: '', name: '', scaleCode: '', scaleName: '', agentId: '', enabled: 1, version: 0};
export function SiteConfigurationPage() {
 const [nodeRole,setNodeRole]=useState<string>();
 const [pending,setPending]=useState<PendingConfiguration|null>(null);
 const [rows, setRows] = useState<SiteConfiguration[]>([]);
 const [serverUrl, setServerUrl] = useState('');
 const [codes, setCodes] = useState<string[]>([]);
 const [loading, setLoading] = useState(true);
 const [busy, setBusy] = useState(false);
 const [loadError, setLoadError] = useState('');
 const [error, setError] = useState('');
 const [notice, setNotice] = useState('');
 const [keyword, setKeyword] = useState('');
 const [status, setStatus] = useState('');
 const [page, setPage] = useState(1);
 const [editing, setEditing] = useState<(SiteConfigurationInput & {id?: number}) | null>(null);
 const [originalAgent, setOriginalAgent] = useState('');
 const refresh = useCallback(async () => {
  setLoading(true); setLoadError('');
  try {
   const [data, settings, caps] = await Promise.all([siteConfigurationApi.list(), siteConfigurationApi.settings(), weighingApi.capabilities()]);
   setRows(data); setServerUrl(settings.serverUrl); setCodes(caps.permissionCodes);setNodeRole(caps.nodeRole);
   if(caps.nodeRole==='site'&&caps.permissionCodes.includes('weighing:site:manage'))setPending(await nodeApi.pendingConfiguration());
  } catch (e) { setLoadError(getErrorMessage(e, '厂区配置加载失败')); }
  finally { setLoading(false); }
 }, []);
 useEffect(() => { void refresh(); }, [refresh]);
 const canManage = nodeRole!=='site'&&codes.includes('weighing:site:manage');
 const canExport = codes.includes('weighing:site:export');
 async function run(action: () => Promise<void>) {
  setBusy(true); setError(''); setNotice('');
  try { await action(); } catch (e) { setError(getErrorMessage(e, '操作失败')); }
  finally { setBusy(false); }
 }
 async function download(row: SiteConfiguration) {
  await run(async () => {
   const connection = await siteConfigurationApi.connection(row.id);
   const url = URL.createObjectURL(new Blob([JSON.stringify(connection, null, 2)], {type: 'application/json'}));
   const link = document.createElement('a'); link.href = url; link.download = `${row.code}-agent.json`; link.click();
   setTimeout(() => URL.revokeObjectURL(url), 1000);
   setNotice(`${row.name}配置已下载。真实 Agent 在本地维护台导入并应用；mock 使用 --config 文件路径启动。`);
  });
 }
 const filtered = rows.filter(row => (!keyword || `${row.code} ${row.name} ${row.scaleName} ${row.agentId}`.toLowerCase().includes(keyword.toLowerCase())) && (status === '' || String(row.enabled) === status));
 const columns: DataTableColumn<SiteConfiguration>[] = [
  {title: '厂区 / 编号', key: 'site', width: 200, render: (_, row) => <div><span className="font-medium">{row.name}</span><p className="mt-1 text-text-tertiary">{row.code}</p></div>},
  {title: '地磅', key: 'scale', width: 180, render: (_, row) => <><span>{row.scaleName}</span><p className="text-xs text-text-tertiary">{row.scaleCode}</p></>},
  {title: 'Agent 编号', key: 'agent', width: 180, render: (_, row) => row.agentId || '待配置'},
  {title: '状态', key: 'enabled', width: 88, render: (_, row) => <StatusTag tone={row.enabled ? 'success' : 'neutral'}>{row.pendingRevision&&row.pendingRevision>(row.appliedRevision??0)?'待现场应用':row.enabled ? '启用' : '停用'}</StatusTag>},
  {title: '操作', key: 'actions', width: 228, nowrap: true, render: (_, row) => <TableActions>
   {canManage && <Button variant="ghost" size="sm" disabled={busy} onClick={() => { setError(''); setOriginalAgent(row.agentId); setEditing({...row}); }}>编辑</Button>}
   {canExport && <Button variant="ghost" size="sm" disabled={busy || !row.agentId} onClick={() => void download(row)}>下载启动配置</Button>}
  </TableActions>},
 ];
 return <div className="weighing-list-page">
  <PageHeader title="厂区配置" description="维护厂区、地磅及对应 Agent，下载各现场使用的启动配置。" actions={<Button onClick={() => void refresh()} disabled={busy || loading}>刷新</Button>} />
  {nodeRole==='site'&&pending&&pending.revision>pending.appliedRevision&&<ContentCard className="mb-space-4" title="待应用的中心配置"><p>{pending.configuration.name} · {pending.configuration.scaleName} · {pending.configuration.enabled?'启用':'停用'} · {pending.configuration.serverUrl}</p><Button disabled={busy} onClick={()=>void run(async()=>{await nodeApi.applyConfiguration(pending.revision);await refresh();setNotice('配置已在本厂应用，等待同步中心。');})}>在本厂明确应用</Button></ContentCard>}
  {!nodeRole&&<ContentCard className="mb-space-4" bodyClassName="!p-space-4">
   <form className="flex flex-wrap items-end gap-space-3" onSubmit={e => { e.preventDefault(); void run(async () => { await siteConfigurationApi.saveSettings(serverUrl); setNotice('现场连接地址已保存，下载配置时会使用此地址。'); }); }}>
    <label htmlFor="site-server-url" className="self-center text-sm font-medium">现场连接地址</label><div className="min-w-0 flex-1"><Input id="site-server-url" type="url" required placeholder="http://192.168.1.100:8900" value={serverUrl} disabled={!canManage || busy || loading} onChange={e => setServerUrl(e.target.value)}  title="填写工控机可访问的服务器地址，下载配置时自动带上" /></div>
    <Button type="submit" disabled={!canManage || busy || loading}>保存地址</Button>
   </form>
  </ContentCard>}
  {error && !editing && <p role="alert" className="mb-4 text-sm text-error">{error}</p>}
  {notice && <p role="status" className="mb-4 text-sm text-text-secondary">{notice}</p>}
  <SearchFilterBar compact>
   <Field label="查找厂区" htmlFor="site-keyword"><Input id="site-keyword" placeholder="厂区、地磅或 Agent" value={keyword} onChange={e => {setKeyword(e.target.value); setPage(1);}} /></Field>
   <Field label="状态" htmlFor="site-status"><Select id="site-status" value={status} onChange={e => {setStatus(e.target.value); setPage(1);}}><option value="">全部</option><option value="1">启用</option><option value="0">停用</option></Select></Field>
  </SearchFilterBar>
  <DataTableCard className="weighing-list-table">
   <TableToolbar className="!py-space-3" title="厂区列表" description="每厂一台地磅。业务权限在用户管理中单独授权。" actions={canManage && <Button variant="primary" disabled={busy} onClick={() => {setOriginalAgent(''); setError(''); setEditing({...empty});}}>新增厂区</Button>} />
   <DataTable compact stickyHeader fixedEdges ariaLabel="厂区配置列表" className="weighing-list-scroll" columns={columns} dataSource={filtered.slice((page - 1) * 10, page * 10)} rowKey="id" loading={loading} error={loadError} minWidth={876} />
   <Pagination page={page} pageSize={10} total={filtered.length} onPageChange={setPage} />
  </DataTableCard>
  <FormDialog open={!!editing} title={editing?.id ? '编辑厂区' : '新增厂区'} description="已有编号保持不变；停用前须结束任务、核实打印并停止 Agent。" loading={busy} onCancel={() => setEditing(null)} onSubmit={() => run(async () => { if (!editing) return; await siteConfigurationApi.save(editing.id, editing); setEditing(null); await refresh(); setNotice('厂区配置已保存；用户业务范围请在用户管理中授权。'); })}>
   {editing && <div className="grid gap-3 sm:grid-cols-2">{([
    ['code', '厂区编号'], ['name', '厂区名称'], ['scaleCode', '地磅编号'], ['scaleName', '地磅名称'], ['agentId', 'Agent 编号'], ['serverUrl','本厂 Server 地址'],
   ] as const).map(([key, label]) => <Field key={key} label={label} htmlFor={`site-${key}`} required={key!=='serverUrl'||nodeRole==='central'}><Input id={`site-${key}`} required={key!=='serverUrl'||nodeRole==='central'} maxLength={key === 'serverUrl' ? 500 : key === 'name' || key === 'scaleName' ? 100 : key === 'agentId' ? 80 : 40} pattern={key === 'serverUrl' || key === 'name' || key === 'scaleName' ? undefined : '[A-Za-z0-9_.-]+'} readOnly={!!editing.id && (key === 'code' || key === 'scaleCode' || (key === 'agentId' && !!originalAgent))} value={editing[key]??''} onChange={e => setEditing({...editing, [key]: e.target.value})} /></Field>)}
    <Field label="状态" htmlFor="site-enabled"><Select id="site-enabled" value={String(editing.enabled)} onChange={e => setEditing({...editing, enabled: Number(e.target.value)})}><option value="1">启用</option><option value="0">停用</option></Select></Field>
   </div>}
   {error && <p role="alert" className="text-sm text-error">{error}</p>}
  </FormDialog>
 </div>;
}
