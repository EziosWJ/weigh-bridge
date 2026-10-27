import { WorkbenchLink } from "@/components/common/workbench-link";
import { useCallback, useMemo, useRef, type ReactNode } from 'react';
import { ArrowRight, Info } from 'lucide-react';
import { Field } from '@/components/common/field';
import { SearchFilterBar } from '@/components/common/search-filter-bar';
import { SectionTabs } from '@/components/common/section-tabs';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { weighingApi } from '@/api/weighing';
import { useAuthStore } from '@/store/auth-store';
import { useWeighingStore } from '@/store/weighing-store';
import { beijingTime } from '@/lib/weighing/query';
import type { HistoryEntry, TaskInput, WeighingFilter } from '@/types/weighing';
import { roleLabels, partyRoleLabels, partyNatureLabels } from '@/types/weighing';
import { PlateInput } from './plate-input';
import { WeighingLookup } from './lookup-field';
import './list.css';

export function ListDate({ value }: { value: string }) {
  const formatted = beijingTime(value);
  return <time dateTime={value} className="whitespace-nowrap tabular-nums"><span className="block">{formatted.slice(0, 10)}</span><span className="block text-text-tertiary">{formatted.slice(11)}</span></time>;
}

export function DeviceBanner({ compact = false }: { compact?: boolean }) {
  const { data, storageError } = useWeighingStore();
  if (compact && !data.pending && !storageError) return null;
  return <aside className="mb-4 rounded-control border border-border bg-neutral-background px-4 py-2 text-sm text-text-secondary" aria-label="设备连接提示">
    {!compact && <div>设备数据由 Agent 提供 · 服务端保存 · 称重须人工确认</div>}
    {data.pending && <p className="mt-2 text-warning">上次称重提交结果待核实。<WorkbenchLink className="ml-1 text-primary underline" to={`/weighing/workbench?task=${encodeURIComponent(data.pending.taskId)}`}>前往工作台查询原结果</WorkbenchLink></p>}
    {storageError && <p role="alert" className="mt-2 text-error">{storageError}</p>}
  </aside>;
}

export function BusinessFields({ value, onChange, lockDirection = false, hidePlate = false, compact = false, onPatch, help, tab, onTabChange }: { onPatch?: (patch: Partial<TaskInput>) => void; value: TaskInput; onChange: (value: TaskInput) => void; lockDirection?: boolean; hidePlate?: boolean; compact?: boolean; help?: ReactNode; tab?: string; onTabChange?: (value: string) => void }) {
  const legacyPlate = useRef(value.plate);
  const { materials, parties } = useWeighingStore(s => s.data);
  const update = (patch: Partial<TaskInput>) => onPatch ? onPatch(patch) : onChange({ ...value, ...patch });
  const materialOptions = useMemo(() => materials.filter(m => m.enabled || m.id === value.materialId).map(m => ({ id: m.id, name: m.name, code: m.code, details: [m.spec || '未填写规格', m.code], tags: ['吨'], disabled: !m.enabled })), [materials, value.materialId]);
  const partyOptions = useMemo(() => parties.filter(p => (p.enabled || p.id === value.partyId) && p.roles.includes(value.direction === 'inbound' ? 'supplier' : 'customer')).map(p => ({ id: p.id, name: p.name, code: p.code, details: [p.shortName, p.phone, p.code].filter(Boolean), tags: [partyNatureLabels[p.nature], ...p.roles.map(r => partyRoleLabels[r])], disabled: !p.enabled })), [parties, value.partyId, value.direction]);
  const carrierOptions = useMemo(() => parties.filter(p => (p.enabled || p.id === value.carrierId) && p.roles.includes('carrier')).map(p => ({ id: p.id, name: p.name, code: p.code, details: [p.shortName, p.phone, p.code].filter(Boolean), tags: [partyNatureLabels[p.nature], '承运商'], disabled: !p.enabled })), [parties, value.carrierId]);
  const loadMaterials = useCallback(async () => {
    const token = useAuthStore.getState().token;
    const materials = await weighingApi.materials();
    if (token !== useAuthStore.getState().token) throw new Error('登录状态已变化，请重新打开选择框');
    useWeighingStore.setState(s => ({ data: { ...s.data, materials } }));
    return materials.filter(m => m.enabled || m.id === value.materialId).map(m => ({ id: m.id, name: m.name, code: m.code, details: [m.spec || '未填写规格', m.code], tags: ['吨'], disabled: !m.enabled }));
  }, [value.materialId]);
  const loadParties = useCallback(async () => {
    const token = useAuthStore.getState().token;
    const parties = await weighingApi.parties();
    if (token !== useAuthStore.getState().token) throw new Error('登录状态已变化，请重新打开选择框');
    useWeighingStore.setState(s => ({ data: { ...s.data, parties } }));
    return parties;
  }, []);
  const loadPartyOptions = useCallback(async () => (await loadParties())
    .filter(p => (p.enabled || p.id === value.partyId) && p.roles.includes(value.direction === 'inbound' ? 'supplier' : 'customer'))
    .map(p => ({ id: p.id, name: p.name, code: p.code, details: [p.shortName, p.phone, p.code].filter(Boolean), tags: [partyNatureLabels[p.nature], ...p.roles.map(r => partyRoleLabels[r])], disabled: !p.enabled })), [loadParties, value.partyId, value.direction]);
  const loadCarrierOptions = useCallback(async () => (await loadParties())
    .filter(p => (p.enabled || p.id === value.carrierId) && p.roles.includes('carrier'))
    .map(p => ({ id: p.id, name: p.name, code: p.code, details: [p.shortName, p.phone, p.code].filter(Boolean), tags: [partyNatureLabels[p.nature], '承运商'], disabled: !p.enabled })), [loadParties, value.carrierId]);
  const party = parties.find(p => p.id === value.partyId);
  const carrier = parties.find(p => p.id === value.carrierId);
  const supplementSummary = [carrier && `承运：${carrier.name}`, value.contactName && `联系人：${value.contactName}`, value.driver && `司机：${value.driver}`, value.driverPhone && `司机电话：${value.driverPhone}`, value.remark && `备注：${value.remark}`].filter(Boolean).join(' · ') || '按需填写承运方、联系人、司机或备注';
  const supplementary = <>
    <Field label="承运方（选填）" help="可与供收方相同，须具有承运商身份"><WeighingLookup label="承运方" value={value.carrierId ?? ''} options={carrierOptions} loadOptions={loadCarrierOptions} onChange={carrierId => update({ carrierId })} /></Field>
    {!!party?.contacts.length && <Field label="选择主体联系人" htmlFor="business-contact"><Select id="business-contact" value={party.contacts.find(c => c.name === value.contactName && c.phone === value.contactPhone)?.id ?? ''} onChange={e => { const contact = party.contacts.find(c => c.id === e.target.value); if (contact) update({ contactName: contact.name, contactPhone: contact.phone }); }}><option value="">手工填写本趟联系信息</option>{party.contacts.map(c => <option key={c.id} value={c.id}>{c.name} · {c.phone || '无电话'}{c.isDefault ? '（默认）' : ''}</option>)}</Select></Field>}
    <Field label="本趟联系人（选填）" htmlFor="business-contact-name"><Input id="business-contact-name" value={value.contactName ?? ''} onChange={e => update({ contactName: e.target.value })} /></Field>
    <Field label="联系手机／电话（选填）" htmlFor="business-contact-phone"><Input id="business-contact-phone" type="tel" value={value.contactPhone ?? ''} onChange={e => update({ contactPhone: e.target.value })} /></Field>
    <Field label="司机姓名（选填）" htmlFor="business-driver"><Input id="business-driver" value={value.driver} onChange={e => update({ driver: e.target.value })} /></Field>
    <Field label="司机电话（选填）" htmlFor="business-driver-phone"><Input id="business-driver-phone" type="tel" value={value.driverPhone ?? ''} onChange={e => update({ driverPhone: e.target.value })} /></Field>
    <Field label="备注（选填）" htmlFor="business-remark"><Textarea id="business-remark" value={value.remark} onChange={e => update({ remark: e.target.value })} rows={2} /></Field>
  </>;
  const directionInbound = value.direction === 'inbound';
  const directionFirst = directionInbound ? '毛重' : '皮重';
  const directionSecond = directionInbound ? '皮重' : '毛重';
  const directionTitle = directionInbound ? '入厂卸货 · 先毛后皮' : '出厂装货 · 先皮后毛';
  const directionDetail = directionInbound
    ? '车辆进厂后先称车辆和货物的总重量（毛重），卸货完成后再称空车重量（皮重）。'
    : '车辆进厂后先称空车重量（皮重），装货完成后再称车辆和货物的总重量（毛重）。';
  const directionNote = '首磅确认后建立运输任务；回磅确认后生成正式过磅单。净重 = 毛重 − 皮重。';
  const basic = <>
    {!hidePlate && <Field label="车牌" htmlFor="business-plate" required><PlateInput id="business-plate" value={value.plate} legacyValue={legacyPlate.current} onChange={plate => update({ plate })} /></Field>}
    {compact ? <div className="business-direction-row md:col-span-2">
      <div className="business-direction-control">
        <label htmlFor="business-direction" className="shrink-0 text-sm font-medium">运输方向<span className="ml-1 text-error">*</span></label>
        <Select id="business-direction" className="w-auto min-w-[140px]" value={value.direction} disabled={lockDirection} onChange={e => update({ direction: e.target.value as TaskInput['direction'], partyId: '', contactName: '', contactPhone: '' })}><option value="inbound">入厂卸货</option><option value="outbound">出厂装货</option></Select>
        <span className="business-direction-chip">{directionInbound ? '先毛后皮' : '先皮后毛'}</span>
        {lockDirection && <span className="business-direction-lock">方向已锁定</span>}
      </div>
      <div className="business-direction-help" role="note">
        <Info className="h-4 w-4" aria-hidden />
        <div>
          <strong>{directionTitle}</strong>
          <p>{lockDirection ? '首次称重后方向不可变更；回磅沿用原方向继续办理。' : directionDetail}</p>
          <div className="business-direction-steps" aria-label="称重顺序">
            <span className="business-direction-step"><b>首磅</b>{directionFirst}</span>
            <ArrowRight className="h-3.5 w-3.5 business-direction-arrow" aria-hidden />
            <span className="business-direction-step is-second"><b>回磅</b>{directionSecond}</span>
          </div>
          <p>{directionNote}</p>
        </div>
      </div>
    </div> : <Field label="运输方向" htmlFor="business-direction" required help={lockDirection ? '首次称重后方向锁定；回磅沿用原方向，净重 = 毛重 − 皮重。' : '入厂卸货先称毛重、卸货后称皮重；出厂装货先称皮重、装货后称毛重。首磅建立任务，回磅确认后出单；净重 = 毛重 − 皮重。'}><Select id="business-direction" value={value.direction} disabled={lockDirection} onChange={e => update({ direction: e.target.value as TaskInput['direction'], partyId: '', contactName: '', contactPhone: '' })}><option value="inbound">入厂卸货</option><option value="outbound">出厂装货</option></Select></Field>}
    <Field label="物料" required><WeighingLookup label="物料" value={value.materialId} options={materialOptions} loadOptions={loadMaterials} onChange={materialId => update({ materialId })} /></Field>
    <div className="min-w-0">
    <Field label={value.direction === 'inbound' ? '供货方' : '收货方'} required><WeighingLookup label={value.direction === 'inbound' ? '供货方' : '收货方'} value={value.partyId} options={partyOptions} loadOptions={loadPartyOptions} onChange={partyId => { const p = useWeighingStore.getState().data.parties.find(p => p.id === partyId); const contact = p?.contacts.find(c => c.isDefault); update({ partyId, contactName: contact?.name ?? '', contactPhone: contact ? contact.phone : p?.phone ?? '' }); }} /></Field>
    </div>
  </>;
  if (!compact) return <div className="grid gap-x-4 md:grid-cols-2">{basic}{supplementary}</div>;
  return <SectionTabs label="本趟运输资料" value={tab} onValueChange={onTabChange} items={[
    { value: 'basic', label: '基本信息', content: <div className="grid gap-x-4 md:grid-cols-2">{basic}</div> },
    { value: 'supplementary', label: '补充运输信息（选填）', content: <>
      <p aria-label="补充运输信息摘要" className="mb-4 break-words text-xs text-text-secondary">{supplementSummary}</p>
      <div className="grid gap-x-4 md:grid-cols-2">{supplementary}</div>
    </> },
    ...(help ? [{ value: 'help', label: '找不到资料？', content: help }] : []),
  ]} />;
}

export function WeighingFilters({ value, onChange, ticket = false, dateLabel, hideStatus = false, className }: { value: WeighingFilter; onChange: (value: WeighingFilter) => void; ticket?: boolean; dateLabel?: string; hideStatus?: boolean; className?: string }) {
  const { materials, parties } = useWeighingStore(s => s.data);
  const change = (patch: Partial<WeighingFilter>) => onChange({ ...value, ...patch });
  return <SearchFilterBar className={className} compact actions={<Button variant="secondary" onClick={() => onChange({})}>清空筛选</Button>}>
    <Input aria-label="车牌或编号" placeholder={ticket ? '车牌 / 单号 / 任务号' : '车牌 / 任务号'} value={value.keyword ?? ''} onChange={e => change({ keyword: e.target.value })} />
    <Select aria-label="筛选方向" value={value.direction ?? ''} onChange={e => change({ direction: e.target.value })}><option value="">全部方向</option><option value="inbound">入厂卸货</option><option value="outbound">出厂装货</option></Select>
    <Select aria-label="筛选物料" value={value.materialId ?? ''} onChange={e => change({ materialId: e.target.value })}><option value="">全部物料</option>{materials.map(m => <option key={m.id} value={m.id}>{m.name} {m.spec}</option>)}</Select>
    <Select aria-label="筛选供收方" value={value.partyId ?? ''} onChange={e => change({ partyId: e.target.value })}><option value="">全部供收方</option>{parties.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</Select>
    {!hideStatus && <Select aria-label="筛选状态" value={value.status ?? ''} onChange={e => change({ status: e.target.value })}><option value="">全部状态</option>{(ticket ? [['valid', '有效'], ['void', '作废']] : [['second', '待二次称重'], ['completed', '已完成'], ['terminated', '已终止']]).map(([v, l]) => <option key={v} value={v}>{l}</option>)}</Select>}
    <label className="flex min-w-0 items-center gap-space-2 text-sm text-text-secondary"><span className="shrink-0">从</span><Input id="filter-date-from" className="min-w-0" type="date" aria-label={`${dateLabel ?? (ticket ? '出单' : '创建')}起始日期`} value={value.from ?? ''} onChange={e => change({ from: e.target.value })} /></label>
    <label className="flex min-w-0 items-center gap-space-2 text-sm text-text-secondary"><span className="shrink-0">至</span><Input id="filter-date-to" className="min-w-0" type="date" aria-label={`${dateLabel ?? (ticket ? '出单' : '创建')}结束日期`} value={value.to ?? ''} onChange={e => change({ to: e.target.value })} /></label>
  </SearchFilterBar>;
}

export function HistoryList({ entries }: { entries: HistoryEntry[] }) {
  return <ol className="space-y-2 text-sm">{entries.map((entry, i) => <li key={i} className="border-b border-border pb-2 last:border-0"><span className="tabular-nums text-text-tertiary">{beijingTime(entry.at)}</span><span className="mx-2">{entry.actor || '历史未记录操作人'} · {entry.role ? roleLabels[entry.role] : '账号操作'} · {entry.action}</span>{entry.reason && <p className="mt-1 break-words text-text-secondary">{entry.reason}</p>}</li>)}{!entries.length && <li className="text-text-tertiary">暂无操作记录</li>}</ol>;
}
