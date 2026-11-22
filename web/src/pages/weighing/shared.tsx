import { HelpHint } from '@/components/common/help-hint';
import { WorkbenchLink } from "@/components/common/workbench-link";
import { useId, useRef, useState, type ReactNode } from 'react';
import { ArrowRight, Info, Plus } from 'lucide-react';
import { Field } from '@/components/common/field';
import { StatusTag } from '@/components/common/status-tag';
import { SearchFilterBar } from '@/components/common/search-filter-bar';
import { SectionTabs } from '@/components/common/section-tabs';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { useWeighingStore } from '@/store/weighing-store';
import { useTransportDirections } from '@/hooks/use-transport-directions';
import { beijingTime } from '@/lib/weighing/query';
import type { HistoryEntry, Party, PartyKind, TaskInput, TripCandidate, WeighingFilter } from '@/types/weighing';
import { roleLabels } from '@/types/weighing';
import { PlateInput } from './plate-input';
import { WeighingLookup, type LookupOption } from './lookup-field';
import { isAvailableParty, supplementarySummary, tripSupplierProblem, useBusinessOptions } from './business-options';
import { TemporaryPartyDialog } from './temporary-party-dialog';
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

/** 运输方向控件：工作台简单行、标准表单字段和带说明的资料弹窗三种排布。 */
export function BusinessDirectionControl({ value, update, lockDirection = false, variant = 'field', showHelp = true }: {
  value: TaskInput;
  update: (patch: Partial<TaskInput>) => void;
  lockDirection?: boolean;
  variant?: 'field' | 'simple' | 'panel';
  showHelp?: boolean;
}) {
  const inbound = value.direction === 'inbound';
  const directionGroup = useId();
  const directionLabels = useTransportDirections();
  const first = inbound ? '毛重' : '皮重';
  const second = inbound ? '皮重' : '毛重';
  const title = `${directionLabels[value.direction]} · ${inbound ? '先毛后皮' : '先皮后毛'}`;
  const detail = inbound
    ? '车辆进厂后先称车辆和货物的总重量（毛重），卸货完成后再称空车重量（皮重）。'
    : '车辆进厂后先称空车重量（皮重），装货完成后再称车辆和货物的总重量（毛重）。';
  const note = '首磅确认后建立运输任务；回磅确认后生成正式过磅单。净重 = 毛重 − 皮重。';
  const select = <Select id="business-direction" aria-label="运输方向" className={variant === 'panel' ? 'w-auto min-w-[140px]' : undefined} value={value.direction} disabled={lockDirection} onChange={e => update({ direction: e.target.value as TaskInput['direction'], partyId: '', contactName: '', contactPhone: '' })}><option value="inbound">{directionLabels.inbound}</option><option value="outbound">{directionLabels.outbound}</option></Select>;
  if (variant === 'simple') return <fieldset className="workbench-direction">
    <legend className="block text-sm font-medium">运输方向<span className="ml-1 text-error">*</span></legend>
    <div className="workbench-direction-options">
      {(['inbound', 'outbound'] as const).map(direction => <label key={direction} className={`workbench-direction-option${value.direction === direction ? ' is-selected' : ''}`}>
        <input type="radio" name={directionGroup} value={direction} checked={value.direction === direction} disabled={lockDirection} onChange={() => update({ direction, partyId: '', contactName: '', contactPhone: '' })} />
        <span>{directionLabels[direction]}</span>
      </label>)}
    </div>
  </fieldset>;
  if (variant === 'panel') return <div className="business-direction-row md:col-span-2">
    <div className="business-direction-control">
      <label htmlFor="business-direction" className="shrink-0 text-sm font-medium">运输方向<span className="ml-1 text-error">*</span></label>
      {select}
      <span className="business-direction-chip">{inbound ? '先毛后皮' : '先皮后毛'}</span>
      {lockDirection && <span className="business-direction-lock">方向已锁定</span>}
    </div>
    <div className="business-direction-help" role="note">
      <Info className="h-4 w-4" aria-hidden />
      <div>
        <strong>{title}</strong>
        <p>{lockDirection ? '首次称重后方向不可变更；回磅沿用原方向继续办理。' : detail}</p>
        <div className="business-direction-steps" aria-label="称重顺序">
          <span className="business-direction-step"><b>首磅</b>{first}</span>
          <ArrowRight className="h-3.5 w-3.5 business-direction-arrow" aria-hidden />
          <span className="business-direction-step is-second"><b>回磅</b>{second}</span>
        </div>
        <p>{note}</p>
      </div>
    </div>
  </div>;
  return <Field label="运输方向" htmlFor="business-direction" required help={showHelp ? (lockDirection ? '首次称重后方向锁定；回磅沿用原方向，净重 = 毛重 − 皮重。' : `${directionLabels.inbound}先称毛重、卸货后称皮重；${directionLabels.outbound}先称皮重、装货后称毛重。首磅建立任务，回磅确认后出单；净重 = 毛重 − 皮重。`) : undefined}>{select}</Field>;
}

function PartyRoleLookup({ label, role, value, options, loadOptions, loadParties, onChange, showDetails = true, allowTemporary = false }: {
  label: string;
  role: PartyKind;
  value: string;
  options: LookupOption[];
  loadOptions: () => Promise<LookupOption[]>;
  loadParties: () => Promise<Party[]>;
  onChange: (id: string) => void;
  showDetails?: boolean;
  allowTemporary?: boolean;
}) {
  const selected = useWeighingStore(s => s.data.parties.find(item => item.id === value));
  const [editor, setEditor] = useState<{ current: Party | null; initialName: string } | null>(null);
  const heading = useRef<HTMLDivElement>(null);
  const temporary = !!selected?.temporaryState;
  const editable = temporary && isAvailableParty(selected);
  return <div className={showDetails ? 'min-w-0 space-y-1' : 'workbench-compact-field'}>
    <div className="workbench-party-field-heading" ref={heading}>
      <span>{label}{role === 'carrier' ? '（选填）' : <span className="text-error">*</span>}</span>
      {allowTemporary && <Button className="workbench-register-party" size="sm" variant="secondary"
        onClick={() => setEditor({ current: null, initialName: '' })}>
        <Plus className="h-4 w-4 shrink-0" aria-hidden />登记临时{label}
      </Button>}
    </div>
    <WeighingLookup label={label} value={value} options={options} loadOptions={loadOptions}
      placeholder="名称、简称或编码"
      emptyAction={allowTemporary ? { label: `登记为临时${label}`, onClick: initialName => setEditor({ current: null, initialName }) } : undefined}
      showRemarkColumn={role !== 'carrier'} showDetails={showDetails} onChange={onChange} />
    {temporary && (!showDetails || (allowTemporary && editable)) && <div className="flex flex-wrap items-center gap-2">
      {!showDetails && <StatusTag>临时</StatusTag>}
      {allowTemporary && editable && <Button size="sm" variant="ghost" aria-label={`更正临时${label}`}
        onClick={() => setEditor({ current: selected, initialName: '' })}>更正资料</Button>}
    </div>}
    {allowTemporary && editor && <TemporaryPartyDialog current={editor.current} initialName={editor.initialName}
      label={label} role={role} loadParties={loadParties} onSelect={saved => {
        if (saved.id !== value && saved.roles.includes(role)) onChange(saved.id);
        if (saved.id === value && !saved.roles.includes(role)) onChange('');
      }} onCancel={() => {
        setEditor(null);
        requestAnimationFrame(() => heading.current?.querySelector('button')?.focus({ preventScroll: true }));
      }} />}
  </div>;
}

/** 首磅现场必填项：物料和按方向限定的供收方。 */
export function BusinessRequiredFields({ value, update, hidePlate = true, compact = false, allowTemporary = false, trip, plateValidation }: {
  value: TaskInput;
  update: (patch: Partial<TaskInput>) => void;
  hidePlate?: boolean;
  compact?: boolean;
  allowTemporary?: boolean;
  trip?: TripCandidate;
  plateValidation?: string;
}) {
  const legacyPlate = useRef(value.plate);
  const { materials, parties, materialOptions, partyOptions, loadMaterials, loadPartyOptions, loadParties } = useBusinessOptions(value, compact);
  const partyLabel = value.direction === 'inbound' ? '供货方' : '收货方';
  const material = <WeighingLookup label="物料" value={value.materialId} options={materialOptions} loadOptions={loadMaterials}
    placeholder="名称、规格或编码" showDetails={!compact} onChange={materialId => update({ materialId })} />;
  const materialSpec = materials.find(item => item.id === value.materialId)?.spec;
  const tripSupplier = trip ? parties.find(item => item.code === trip.partyCode) : undefined;
  const tripProblem = trip ? tripSupplierProblem(tripSupplier) : '';
  const party = trip ? <div className="workbench-compact-field" role="group" aria-label="粮食流转单供货方">
    <span>供货方</span>
    <div className="workbench-trip-supplier">
      <strong className="min-w-0 break-words">{tripSupplier?.name || trip.partyName}</strong>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-text-secondary">
        <StatusTag tone="info">来自粮食流转单</StatusTag>{trip.partyCode && <HelpHint label="供货方编码">{trip.partyCode}</HelpHint>}
      </div>
    </div>
    {tripProblem && <p role="alert" className="text-sm text-warning">{tripProblem}</p>}
  </div> : <PartyRoleLookup key={partyLabel} label={partyLabel} role={value.direction === 'inbound' ? 'supplier' : 'customer'}
    value={value.partyId} options={partyOptions} loadOptions={loadPartyOptions} loadParties={loadParties} showDetails={!compact}
    allowTemporary={allowTemporary} onChange={partyId => {
      const selected = useWeighingStore.getState().data.parties.find(item => item.id === partyId);
      const contact = selected?.contacts.find(c => c.isDefault);
      update({ partyId, contactName: contact?.name ?? '', contactPhone: contact ? contact.phone : selected?.phone ?? '' });
    }} />;
  if (compact) return <>
    {!hidePlate && <div className="workbench-compact-field"><span>车牌<span className="text-error">*</span></span><PlateInput id="business-plate" value={value.plate} legacyValue={legacyPlate.current} validationError={plateValidation} onChange={plate => update({ plate })} /></div>}
    <div className="workbench-compact-field"><span>物料<span className="text-error">*</span></span>{material}{materialSpec && <span className="text-xs text-text-secondary">{materialSpec}</span>}</div>
    {party}
  </>;
  return <>
    {!hidePlate && <Field label="车牌" htmlFor="business-plate" required><PlateInput id="business-plate" value={value.plate} legacyValue={legacyPlate.current} validationError={plateValidation} onChange={plate => update({ plate })} /></Field>}
    <Field label="物料" required>{material}</Field>
    {party}
  </>;
}

/** 选填补充资料；工作台折叠展示，资料表单直接展开。 */
export function BusinessSupplementaryFields({ value, update, allowTemporary = false, showHelp = true }: {
  value: TaskInput;
  update: (patch: Partial<TaskInput>) => void;
  allowTemporary?: boolean;
  showHelp?: boolean;
}) {
  const { parties, carrierOptions, loadCarrierOptions, loadParties } = useBusinessOptions(value);
  const party = parties.find(p => p.id === value.partyId);
  return <>
    <div className="space-y-1.5">
      <PartyRoleLookup label="承运方" role="carrier" value={value.carrierId ?? ''} options={carrierOptions}
        loadOptions={loadCarrierOptions} loadParties={loadParties} allowTemporary={allowTemporary} onChange={carrierId => update({ carrierId })} />
      {showHelp && <p className="text-helper text-text-tertiary">可与供收方相同，须具有承运商身份</p>}
    </div>
    {!!party?.contacts.length && <Field label="选择主体联系人" htmlFor="business-contact"><Select id="business-contact" value={party.contacts.find(c => c.name === value.contactName && c.phone === value.contactPhone)?.id ?? ''} onChange={e => { const contact = party.contacts.find(c => c.id === e.target.value); if (contact) update({ contactName: contact.name, contactPhone: contact.phone }); }}><option value="">手工填写本趟联系信息</option>{party.contacts.map(c => <option key={c.id} value={c.id}>{c.name} · {c.phone || '无电话'}{c.isDefault ? '（默认）' : ''}</option>)}</Select></Field>}
    <Field label="本趟联系人（选填）" htmlFor="business-contact-name"><Input id="business-contact-name" value={value.contactName ?? ''} onChange={e => update({ contactName: e.target.value })} /></Field>
    <Field label="联系手机／电话（选填）" htmlFor="business-contact-phone"><Input id="business-contact-phone" type="tel" value={value.contactPhone ?? ''} onChange={e => update({ contactPhone: e.target.value })} /></Field>
    <Field label="司机姓名（选填）" htmlFor="business-driver"><Input id="business-driver" value={value.driver} onChange={e => update({ driver: e.target.value })} /></Field>
    <Field label="司机电话（选填）" htmlFor="business-driver-phone"><Input id="business-driver-phone" type="tel" value={value.driverPhone ?? ''} onChange={e => update({ driverPhone: e.target.value })} /></Field>
    <Field label="备注（选填）" htmlFor="business-remark"><Textarea id="business-remark" value={value.remark} onChange={e => update({ remark: e.target.value })} rows={2} /></Field>
  </>;
}

export function BusinessFields({ value, onChange, lockDirection = false, hidePlate = false, compact = false, onPatch, help, tab, onTabChange }: { onPatch?: (patch: Partial<TaskInput>) => void; value: TaskInput; onChange: (value: TaskInput) => void; lockDirection?: boolean; hidePlate?: boolean; compact?: boolean; help?: ReactNode; tab?: string; onTabChange?: (value: string) => void }) {
  const update = (patch: Partial<TaskInput>) => onPatch ? onPatch(patch) : onChange({ ...value, ...patch });
  const { parties } = useBusinessOptions(value);
  const legacyPlate = useRef(value.plate);
  const basic = <>
    {!hidePlate && <Field label="车牌" htmlFor="business-plate" required><PlateInput id="business-plate" value={value.plate} legacyValue={legacyPlate.current} onChange={plate => update({ plate })} /></Field>}
    <BusinessDirectionControl value={value} update={update} lockDirection={lockDirection} variant={compact ? 'panel' : 'field'} />
    <BusinessRequiredFields value={value} update={update} hidePlate />
  </>;
  const supplementary = <BusinessSupplementaryFields value={value} update={update} />;
  const summary = supplementarySummary(value, parties);
  if (!compact) return <div className="grid gap-x-4 md:grid-cols-2">{basic}{supplementary}</div>;
  return <SectionTabs label="本趟运输资料" value={tab} onValueChange={onTabChange} items={[
    { value: 'basic', label: '基本信息', content: <div className="grid gap-x-4 md:grid-cols-2">{basic}</div> },
    { value: 'supplementary', label: '补充运输信息（选填）', content: <>
      <p aria-label="补充运输信息摘要" className="mb-4 break-words text-xs text-text-secondary">{summary}</p>
      <div className="grid gap-x-4 md:grid-cols-2">{supplementary}</div>
    </> },
    ...(help ? [{ value: 'help', label: '找不到资料？', content: help }] : []),
  ]} />;
}

export function WeighingFilters({ value, onChange, ticket = false, dateLabel, hideStatus = false }: { value: WeighingFilter; onChange: (value: WeighingFilter) => void; ticket?: boolean; dateLabel?: string; hideStatus?: boolean }) {
  const directionLabels = useTransportDirections();
  const { materials, parties } = useWeighingStore(s => s.data);
  const change = (patch: Partial<WeighingFilter>) => onChange({ ...value, ...patch });
  return <SearchFilterBar compact actions={<Button variant="secondary" onClick={() => onChange({})}>清空筛选</Button>}>
    <Input aria-label="车牌或编号" placeholder={ticket ? '车牌 / 单号 / 任务号' : '车牌 / 任务号'} value={value.keyword ?? ''} onChange={e => change({ keyword: e.target.value })} />
    <Select aria-label="筛选方向" value={value.direction ?? ''} onChange={e => change({ direction: e.target.value })}><option value="">全部方向</option><option value="inbound">{directionLabels.inbound}</option><option value="outbound">{directionLabels.outbound}</option></Select>
    <Select aria-label="筛选物料" value={value.materialId ?? ''} onChange={e => change({ materialId: e.target.value })}><option value="">全部物料</option>{materials.map(m => <option key={m.id} value={m.id}>{m.name} {m.spec}</option>)}</Select>
    <Select aria-label="筛选供收方" value={value.partyId ?? ''} onChange={e => change({ partyId: e.target.value })}><option value="">全部供收方</option>{parties.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</Select>
    {!hideStatus && <Select aria-label="筛选状态" value={value.status ?? ''} onChange={e => change({ status: e.target.value })}><option value="">全部状态</option>{(ticket ? [['valid', '有效'], ['void', '作废']] : [['second', '待二次称重'], ['completed', '已完成'], ['terminated', '已终止']]).map(([v, l]) => <option key={v} value={v}>{l}</option>)}</Select>}
    <label className="flex min-w-0 items-center gap-space-2 text-sm text-text-secondary"><span className="shrink-0">从</span><Input id="filter-date-from" className="min-w-0" type="date" aria-label={`${dateLabel ?? (ticket ? '出单' : '创建')}起始日期`} value={value.from ?? ''} onChange={e => change({ from: e.target.value })} /></label>
    <label className="flex min-w-0 items-center gap-space-2 text-sm text-text-secondary"><span className="shrink-0">至</span><Input id="filter-date-to" className="min-w-0" type="date" aria-label={`${dateLabel ?? (ticket ? '出单' : '创建')}结束日期`} value={value.to ?? ''} onChange={e => change({ to: e.target.value })} /></label>
  </SearchFilterBar>;
}

const historyActionLabels: Record<string, string> = {
  'first.confirm': '确认首磅',
  'second.confirm': '确认回磅',
  'task.correct': '业务纠错',
  'task.terminate': '终止任务',
  'task.manual-return': '人工补录回磅',
  'task.reweigh': '单磅重称',
  'ticket.void': '单据作废',
  'ticket.reopen': '更正重开',
  'ticket.manual-supplement': '人工补单',
};

export function HistoryList({ entries, ticketNumbers }: { entries: HistoryEntry[]; ticketNumbers?: Record<string, string | undefined> }) {
  return <ol className="space-y-2 text-sm">{entries.map((entry, i) => {
    const rejected = entry.action.endsWith('.rejected');
    const action = rejected ? entry.action.slice(0, -'.rejected'.length) : entry.action;
    const number = entry.ticketId ? ticketNumbers?.[entry.ticketId] : undefined;
    return <li key={i} className="border-b border-border pb-2 last:border-0">
      <span className="tabular-nums text-text-tertiary">{beijingTime(entry.at)}</span>
      <span className="mx-2">{[historyActionLabels[action] ?? action, rejected && "未通过", number, entry.actor || '历史未记录操作人', entry.role && roleLabels[entry.role]].filter(Boolean).join(' · ')}</span>
      {entry.reason && <p className="mt-1 break-words text-text-secondary">{entry.reason}</p>}
    </li>;
  })}{!entries.length && <li className="text-text-tertiary">暂无操作记录</li>}</ol>;
}
