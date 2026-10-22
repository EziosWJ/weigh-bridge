import { WorkbenchLink } from "@/components/common/workbench-link";
import { useDeferredValue, useState, type ReactNode } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { PageHeader } from '@/components/common/page-header';
import { DataTableCard } from '@/components/common/data-table-card';
import { DataTable } from '@/components/common/data-table';
import { TableToolbar } from '@/components/common/table-toolbar';
import { TableActions } from '@/components/common/table-actions';
import { Pagination } from '@/components/common/pagination';
import { SearchFilterBar } from '@/components/common/search-filter-bar';
import { FormDialog } from '@/components/common/form-dialog';
import { ConfirmDialog } from '@/components/common/confirm-dialog';
import { StatusTag } from '@/components/common/status-tag';
import { Field } from '@/components/common/field';
import { FormSection } from '@/components/common/form-section';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { useWeighingMasterStore, useWeighingMasterData } from '@/store/weighing-master-store';
import { useAuthStore } from '@/store/auth-store';
import { partyRoleLabels, partyNatureLabels, type Material, type Party, type PartyKind, type PartyNature, type MaterialCategory } from '@/types/weighing';
import type { DataTableColumn } from '@/types';
import './list.css';


const linkClass = 'inline-flex h-control-md items-center rounded-control border border-border bg-surface px-4 text-sm text-text-secondary hover:text-primary';
function ReturnLink() {
  const [params] = useSearchParams();
  const target = params.get('returnTo');
  return target?.startsWith('/weighing/workbench') ? <WorkbenchLink className={linkClass} to={target}>返回称重工作台</WorkbenchLink> : null;
}
function RoleHint() {
  return <p className="mb-4 text-sm text-text-secondary">资料为只读。需要维护时请联系管理员授权。</p>;
}


function MasterBanner() {
  return <aside className="mb-4 rounded-control border border-border bg-neutral-background px-4 py-2 text-sm text-text-secondary">服务端保存 · 使用真实账号权限 · 设备数据由 Agent 提供</aside>;
}
function MasterBoundary({ children }: { children: ReactNode }) {
  const store = useWeighingMasterData();
  const token = useAuthStore(s => s.token);
  if (store.error) return <><PageHeader title="资料加载失败" /><p role="alert" className="mb-4 text-error">{store.error}</p><Button onClick={() => void store.load()}>重新加载</Button></>;
  if (store.loadedToken !== token) return <p role="status" className="p-4 text-text-secondary">正在加载服务端资料…</p>;
  if (!store.capabilities?.permissionCodes.includes('weighing:master:read')) return <PageHeader title="暂无资料查询权限" description="请联系管理员授权。" />;
  return children;
}
export function WeighingMaterialsPage(props: { parties?: boolean }) { return <MasterBoundary><MaterialsContent {...props} /></MasterBoundary>; }
export function WeighingMaterialEditPage() { return <MasterBoundary><MaterialEditContent /></MasterBoundary>; }
export function WeighingPartyEditPage() { return <MasterBoundary><PartyEditContent /></MasterBoundary>; }
export function WeighingCategoriesPage() { return <MasterBoundary><CategoriesContent /></MasterBoundary>; }

function MaterialsContent({ parties = false }: { parties?: boolean }) {
  const { data, dispatch, capabilities, saving, load } = useWeighingMasterStore();
  const [params] = useSearchParams();
  const [keyword, setKeyword] = useState('');
  const deferred = useDeferredValue(keyword).trim().toLocaleLowerCase();
  const [enabled, setEnabled] = useState('');
  const [classification, setClassification] = useState('');
  const [nature, setNature] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [deleting, setDeleting] = useState<Material | Party | null>(null);
  const [error, setError] = useState('');
  const admin = capabilities?.nodeRole!=='site' && (capabilities?.permissionCodes.includes('weighing:master:write') ?? false);
  const name = parties ? '往来主体' : '物料';
  const base = parties ? '/weighing/parties' : '/weighing/materials';
  const suffix = params.get('returnTo') ? `?${params.toString()}` : '';
  const rows: (Material | Party)[] = (parties ? data.parties : data.materials).filter(r => {
    const p = r as Party, m = r as Material;
    const text = [r.name, r.code, parties ? p.shortName : m.spec, parties ? p.phone : ''].join(' ').toLocaleLowerCase();
    return (!deferred || text.includes(deferred)) && (!enabled || String(r.enabled) === enabled)
      && (!classification || (parties ? p.roles.includes(classification as PartyKind) : m.categoryId === classification))
      && (!parties || !nature || p.nature === nature);
  });
  const currentPage = Math.min(page, Math.max(1, Math.ceil(rows.length / pageSize)));
  const columns: DataTableColumn<Material | Party>[] = [
    { title: `${name}名称 / 编码`, key: 'name', width: 220, render: (_, r) => <div><Link className="font-medium text-primary hover:underline" to={`${base}/${r.id}/edit${suffix}`}>{r.name}</Link><p className="mt-1 text-xs text-text-tertiary">{r.code}{parties && (r as Party).shortName ? ` · ${(r as Party).shortName}` : ''}</p></div> },
    ...(parties ? [
      { title: '性质 / 业务身份', key: 'roles', width: 180, render: (_: unknown, r: Material | Party) => <div>{partyNatureLabels[(r as Party).nature]}<p className="mt-1 text-text-secondary">{(r as Party).roles.map(role => partyRoleLabels[role]).join('、')}</p></div> },
      { title: '联系信息', key: 'phone', width: 176, render: (_: unknown, r: Material | Party) => <div>{(r as Party).phone || '—'}<p className="text-text-tertiary">{(r as Party).contacts.find(c => c.isDefault)?.name || '未设默认联系人'}</p></div> },
      { title: '常用业务地址', key: 'address', width: 220, render: (_: unknown, r: Material | Party) => (r as Party).address || '—' },
    ] : [
      { title: '分类', key: 'category', width: 128, render: (_: unknown, r: Material | Party) => data.categories.find(c => c.id === (r as Material).categoryId)?.name || '未分类' },
      { title: '规格 / 等级', key: 'spec', width: 144, render: (_: unknown, r: Material | Party) => (r as Material).spec || '—' },
      { title: '称重单位', key: 'unit', width: 88, render: () => '吨' },
    ]),
    { title: '状态', key: 'enabled', width: 88, render: (_, r) => <StatusTag tone={r.enabled ? 'success' : 'neutral'}>{r.enabled ? '启用' : '停用'}</StatusTag> },
    { title: '备注', dataIndex: 'remark', width: 180 },
    { title: '操作', key: 'actions', width: 144, nowrap: true, render: (_, r) => <TableActions><Link className="text-primary hover:underline" to={`${base}/${r.id}/edit${suffix}`}>{admin ? '编辑' : '查看'}</Link>{admin && <Button size="sm" variant="ghost" onClick={() => { setError(''); setDeleting(r); }}>删除</Button>}</TableActions> },
  ];
  return <div className="weighing-list-page">
    <PageHeader title={`${name}资料`} description={parties ? '企业与个人分别建档，可兼具供货商、客户和承运商身份。' : '按可独立称重、查询汇总的规格建档，历史业务保留当时资料。'} actions={<><ReturnLink />{parties&&capabilities?.nodeRole&&capabilities.nodeRole!=='standalone'&&<Link className={linkClass} to="/weighing/temporary-parties">临时主体登记与确认</Link>}{admin && <Link className={linkClass} to={`${base}/new${suffix}`}>新增{name}</Link>}</>} />
    {!admin && <RoleHint />}
    <SearchFilterBar compact actions={<Button onClick={() => { setKeyword(''); setEnabled(''); setClassification(''); setNature(''); setPage(1); }}>清空筛选</Button>}>
      <Input aria-label="资料搜索" placeholder={parties ? '名称 / 简称 / 编码 / 电话' : '名称 / 编码 / 规格'} value={keyword} onChange={e => { setKeyword(e.target.value); setPage(1); }} />
      <Select aria-label="资料状态" value={enabled} onChange={e => { setEnabled(e.target.value); setPage(1); }}><option value="">全部状态</option><option value="true">启用</option><option value="false">停用</option></Select>
      <Select aria-label={parties ? '业务身份筛选' : '物料分类筛选'} value={classification} onChange={e => { setClassification(e.target.value); setPage(1); }}><option value="">{parties ? '全部业务身份' : '全部分类'}</option>{parties ? Object.entries(partyRoleLabels).map(([v, l]) => <option key={v} value={v}>{l}</option>) : data.categories.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</Select>
      {parties && <Select aria-label="主体性质筛选" value={nature} onChange={e => { setNature(e.target.value); setPage(1); }}><option value="">全部主体性质</option><option value="enterprise">企业</option><option value="individual">个人</option></Select>}
    </SearchFilterBar>
    <DataTableCard className="weighing-list-table"><TableToolbar className="!py-space-3" title={`${name}列表`} description={`共 ${rows.length} 条；已引用资料可停用，不能删除。`} actions={<Button disabled={saving} onClick={() => void load()}>刷新</Button>} /><DataTable compact stickyHeader fixedEdges ariaLabel={`${name}列表`} className="weighing-list-scroll" columns={columns} dataSource={rows.slice((currentPage - 1) * pageSize, currentPage * pageSize)} rowKey="id" minWidth={parties ? 1208 : 992} /><Pagination page={currentPage} pageSize={pageSize} total={rows.length} onPageChange={setPage} onPageSizeChange={v => { setPageSize(v); setPage(1); }} /></DataTableCard>
    <ConfirmDialog open={!!deleting} title={`删除${name}`} description={<><span>确认删除“{deleting?.name}”？引用过的资料请改为停用。</span>{error && <p role="alert" className="mt-2 text-error">{error}</p>}</>} onCancel={() => setDeleting(null)} onConfirm={async () => { if (!deleting) return; try { await dispatch({ type: parties ? 'deleteParty' : 'deleteMaterial', id: deleting.id, version: deleting.version }); setDeleting(null); } catch (e) { setError(e instanceof Error ? e.message : '删除失败'); } }} />
  </div>;
}

function MaterialEditContent() {
  const { id } = useParams();
  const { data, dispatch, capabilities, saving } = useWeighingMasterStore();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const current = data.materials.find(m => m.id === id);
  const [baseVersion] = useState(current?.version);
  const [form, setForm] = useState({ name: current?.name ?? '', categoryId: current?.categoryId ?? '', spec: current?.spec ?? '', enabled: current?.enabled ?? true, remark: current?.remark ?? '' });
  const [error, setError] = useState('');
  const admin = capabilities?.nodeRole!=='site' && (capabilities?.permissionCodes.includes('weighing:master:write') ?? false);
  const back = `/weighing/materials${params.toString() ? `?${params}` : ''}`;
  if (id && !current) return <PageHeader title="物料不存在" actions={<Link to={back}>返回资料列表</Link>} />;
  return <>
    <PageHeader title={`${id ? admin ? '编辑' : '查看' : '新增'}物料`} description="一种可独立称重和汇总的规格对应一条资料。" actions={<Link className={linkClass} to={back}>返回物料列表</Link>} /><MasterBanner />
    <form className="max-w-5xl space-y-5" onSubmit={async e => { e.preventDefault(); try { await dispatch({ type: 'saveMaterial', id, version: baseVersion, ...form }); navigate(back); } catch (failure) { setError(failure instanceof Error ? failure.message : '保存失败'); } }}>
      <fieldset disabled={!admin || saving}><FormSection title="物料信息"><div className="grid gap-x-5 md:col-span-2 md:grid-cols-2">
        <Field label="物料编码" help="自动生成且唯一，保存后不可修改"><Input value={current?.code ?? '保存时自动生成'} readOnly /></Field>
        <Field label="物料名称" htmlFor="material-name" required><Input id="material-name" required value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} /></Field>
        <Field label="分类" htmlFor="material-category"><Select id="material-category" value={form.categoryId} onChange={e => setForm({ ...form, categoryId: e.target.value })}><option value="">未分类</option>{data.categories.filter(c => c.enabled || c.id === form.categoryId).map(c => <option key={c.id} value={c.id}>{c.name}{!c.enabled ? '（停用）' : ''}</option>)}</Select></Field>
        <Field label="规格／等级" htmlFor="material-spec"><Input id="material-spec" placeholder="例如：一级、二级" value={form.spec} onChange={e => setForm({ ...form, spec: e.target.value })} /></Field>
        <Field label="称重单位"><Input value="吨" readOnly /></Field>
        <Field label="状态" htmlFor="material-enabled"><Select id="material-enabled" value={String(form.enabled)} onChange={e => setForm({ ...form, enabled: e.target.value === 'true' })}><option value="true">启用</option><option value="false">停用</option></Select></Field>
        <Field label="备注" htmlFor="material-remark"><Textarea id="material-remark" value={form.remark} onChange={e => setForm({ ...form, remark: e.target.value })} /></Field>
      </div></FormSection></fieldset>
      {data.materials.some(m => m.id !== id && m.name === form.name.trim()) && <p className="text-sm text-warning">已有同名物料，请用规格和编码区分。</p>}
      {error && <p role="alert" className="text-sm text-error">{error}</p>}
      {admin && <Button type="submit" variant="primary" disabled={saving}>保存物料</Button>}
    </form>
  </>;
}

function PartyEditContent() {
  const { id } = useParams();
  const { data, dispatch, capabilities, saving } = useWeighingMasterStore();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const current = data.parties.find(p => p.id === id);
  const [baseVersion] = useState(current?.version);
  const [form, setForm] = useState({ name: current?.name ?? '', shortName: current?.shortName ?? '', nature: current?.nature ?? 'enterprise' as PartyNature, roles: current?.roles ?? ['supplier'] as PartyKind[], phone: current?.phone ?? '', address: current?.address ?? '', creditCode: current?.creditCode ?? '', enabled: current?.enabled ?? true, remark: current?.remark ?? '', contacts: current?.contacts ?? [] });
  const [error, setError] = useState('');
  const admin = capabilities?.nodeRole!=='site' && (capabilities?.permissionCodes.includes('weighing:master:write') ?? false);
  const back = `/weighing/parties${params.toString() ? `?${params}` : ''}`;
  if (id && !current) return <PageHeader title="往来主体不存在" actions={<Link to={back}>返回资料列表</Link>} />;
  return <>
    <PageHeader title={`${id ? admin ? '编辑' : '查看' : '新增'}往来主体`} description="同一企业或个人可兼具多种业务身份；司机属于本趟运输，不自动建档。" actions={<Link className={linkClass} to={back}>返回主体列表</Link>} /><MasterBanner />
    <form className="max-w-5xl space-y-5" onSubmit={async e => { e.preventDefault(); try { await dispatch({ type: 'saveParty', id, version: baseVersion, ...form }); navigate(back); } catch (failure) { setError(failure instanceof Error ? failure.message : '保存失败'); } }}>
      <fieldset disabled={!admin || saving} className="space-y-5"><FormSection title="主体信息"><div className="grid gap-x-5 md:col-span-2 md:grid-cols-2">
        <Field label="主体编码" help="自动生成且唯一，保存后不可修改"><Input value={current?.code ?? '保存时自动生成'} readOnly /></Field>
        <Field label="名称" htmlFor="party-name" required><Input id="party-name" required value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} /></Field>
        <Field label="简称" htmlFor="party-short"><Input id="party-short" value={form.shortName} onChange={e => setForm({ ...form, shortName: e.target.value })} /></Field>
        <Field label="主体性质" htmlFor="party-nature" required><Select id="party-nature" value={form.nature} onChange={e => setForm({ ...form, nature: e.target.value as PartyNature })}><option value="enterprise">企业</option><option value="individual">个人</option></Select></Field>
        <Field label="业务身份" required help="至少选择一项，可同时供货、收货及承运"><div className="flex flex-wrap gap-4 py-2">{Object.entries(partyRoleLabels).map(([role, label]) => <label key={role} className="inline-flex items-center gap-2"><input type="checkbox" checked={form.roles.includes(role as PartyKind)} onChange={e => setForm({ ...form, roles: e.target.checked ? [...form.roles, role as PartyKind] : form.roles.filter(r => r !== role) })} />{label}</label>)}</div></Field>
        <Field label="主体联系电话" htmlFor="party-phone"><Input id="party-phone" type="tel" value={form.phone} onChange={e => setForm({ ...form, phone: e.target.value })} /></Field>
        {form.nature === 'enterprise' && <Field label="统一社会信用代码（选填）" htmlFor="party-credit"><Input id="party-credit" value={form.creditCode} onChange={e => setForm({ ...form, creditCode: e.target.value })} /></Field>}
        <Field label="常用业务地址" htmlFor="party-address"><Input id="party-address" value={form.address} onChange={e => setForm({ ...form, address: e.target.value })} /></Field>
        <Field label="状态" htmlFor="party-enabled"><Select id="party-enabled" value={String(form.enabled)} onChange={e => setForm({ ...form, enabled: e.target.value === 'true' })}><option value="true">启用</option><option value="false">停用</option></Select></Field>
        <Field label="备注" htmlFor="party-remark"><Textarea id="party-remark" value={form.remark} onChange={e => setForm({ ...form, remark: e.target.value })} /></Field>
      </div></FormSection>
      <FormSection title="主体联系人" description="可以有多个联系人，并指定一位默认联系人带入本趟业务。">
        <div className="space-y-5 md:col-span-2">{form.contacts.map((contact, index) => <div key={contact.id} className="border-b border-border pb-4">
          <div className="mb-3 flex items-center justify-between"><label className="flex items-center gap-2 text-sm"><input type="radio" name="default-contact" checked={contact.isDefault} onChange={() => setForm({ ...form, contacts: form.contacts.map(c => ({ ...c, isDefault: c.id === contact.id })) })} />默认联系人</label><Button size="sm" onClick={() => { const contacts = form.contacts.filter(c => c.id !== contact.id); if (contacts.length && !contacts.some(c => c.isDefault)) contacts[0] = { ...contacts[0], isDefault: true }; setForm({ ...form, contacts }); }}>移除联系人</Button></div>
          <div className="grid gap-x-4 md:grid-cols-2">{([['name', '姓名'], ['phone', '电话'], ['purpose', '职务／用途'], ['remark', '联系人备注']] as const).map(([key, label]) => <Field key={key} label={`${label}（联系人${index + 1}）`} htmlFor={`contact-${contact.id}-${key}`} required={key === 'name'}><Input id={`contact-${contact.id}-${key}`} type={key === 'phone' ? 'tel' : 'text'} required={key === 'name'} value={contact[key]} onChange={e => setForm({ ...form, contacts: form.contacts.map(c => c.id === contact.id ? { ...c, [key]: e.target.value } : c) })} /></Field>)}</div>
        </div>)}{admin && <Button onClick={() => setForm({ ...form, contacts: [...form.contacts, { id: crypto.randomUUID(), name: '', phone: '', purpose: '', remark: '', isDefault: form.contacts.length === 0 }] })}>添加联系人</Button>}{!form.contacts.length && <p className="text-sm text-text-tertiary">暂无联系人，可先保存主体资料。</p>}</div>
      </FormSection></fieldset>
      {data.parties.some(p => p.id !== id && p.name === form.name.trim()) && <p className="text-sm text-warning">存在同名主体，允许保存。请通过编码、电话和性质核对，避免重复建档。</p>}
      {error && <p role="alert" className="text-sm text-error">{error}</p>}
      {admin && <Button type="submit" variant="primary" disabled={saving}>保存往来主体</Button>}
    </form>
  </>;
}

function CategoriesContent() {
  const { data, dispatch, capabilities, saving } = useWeighingMasterStore();
  const admin = capabilities?.nodeRole!=='site' && (capabilities?.permissionCodes.includes('weighing:master:write') ?? false);
  const [editing, setEditing] = useState<{ id?: string; name: string; enabled: boolean; version?: number } | null>(null);
  const [deleting, setDeleting] = useState<MaterialCategory | null>(null);
  const [error, setError] = useState('');
  const columns: DataTableColumn<MaterialCategory>[] = [
    { title: '分类名称', dataIndex: 'name', width: 260 },
    { title: '状态', key: 'status', width: 120, render: (_, c) => <StatusTag tone={c.enabled ? 'success' : 'neutral'}>{c.enabled ? '启用' : '停用'}</StatusTag> },
    { title: '操作', key: 'action', width: 180, render: (_, c) => <TableActions><Button size="sm" variant="ghost" disabled={!admin || saving} onClick={() => { setError(''); setEditing(c); }}>编辑</Button><Button size="sm" variant="ghost" disabled={!admin || saving} onClick={() => { setError(''); setDeleting(c); }}>删除</Button></TableActions> },
  ];
  return <div className="weighing-list-page"><PageHeader title="物料分类" description="管理员维护简单分类，已引用分类可停用，不能删除。" actions={admin && <Button onClick={() => { setError(''); setEditing({ name: '', enabled: true }); }}>新增分类</Button>} /><MasterBanner />{!admin && <RoleHint />}<DataTableCard className="weighing-list-table"><TableToolbar className="!py-space-3" title="分类列表" description={`共 ${data.categories.length} 项`} /><DataTable compact stickyHeader ariaLabel="物料分类列表" className="weighing-list-scroll" columns={columns} dataSource={data.categories} rowKey="id" minWidth={600} /></DataTableCard>
    <FormDialog open={!!editing} title="维护物料分类" onCancel={() => setEditing(null)} onSubmit={async () => { if (!editing) return; try { await dispatch({ type: 'saveCategory', ...editing }); setEditing(null); } catch (failure) { setError(failure instanceof Error ? failure.message : '保存失败'); } }}>{editing && <><Field label="分类名称" htmlFor="category-name" required><Input id="category-name" required value={editing.name} onChange={e => setEditing({ ...editing, name: e.target.value })} /></Field><Field label="分类状态" htmlFor="category-enabled"><Select id="category-enabled" value={String(editing.enabled)} onChange={e => setEditing({ ...editing, enabled: e.target.value === 'true' })}><option value="true">启用</option><option value="false">停用</option></Select></Field></>}{error && <p role="alert" className="text-error">{error}</p>}</FormDialog>
    <ConfirmDialog open={!!deleting} title="删除分类" description={<><span>确认删除“{deleting?.name}”？</span>{error && <p role="alert" className="text-error">{error}</p>}</>} onCancel={() => setDeleting(null)} onConfirm={async () => { if (!deleting) return; try { await dispatch({ type: 'deleteCategory', id: deleting.id, version: deleting.version }); setDeleting(null); } catch (failure) { setError(failure instanceof Error ? failure.message : '删除失败'); } }} />
  </div>;
}
