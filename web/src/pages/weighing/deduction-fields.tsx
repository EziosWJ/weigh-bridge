import { useState } from 'react';
import { DetailItem } from '@/components/common/detail-item';
import { HelpHint } from '@/components/common/help-hint';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { DICT_CODES, WEIGHING_DEDUCTION_CATEGORY_OPTIONS } from '@/constants/dicts';
import { useDictOptions } from '@/hooks/use-dict-options';
import {
  adjustedWeights,
  formatAdjustment,
  formatWeight,
  formatWeightInputValue,
  netAdjustmentText,
  originalWeights,
  resolveNetAdjustments,
  weightInMilliTonnes,
} from '@/lib/weighing/weights';
import { beijingTime } from '@/lib/weighing/query';
import {
  roleLabels,
  type FinalWeightInput,
  type NetAdjustmentInput,
  type NetAdjustmentMode,
  type NetAdjustmentsInput,
  type WeighingTicket,
  type WeightValues,
} from '@/types/weighing';

function DecimalField({ id, label, value, step, min, onChange, hint }: {
  id: string;
  label: string;
  value: number;
  step: string;
  min?: string;
  onChange: (value: number) => void;
  hint?: string;
}) {
  const [display, setDisplay] = useState(() => (Number.isFinite(value) ? formatWeightInputValue(value) : ''));
  return <div className="space-y-1.5">
    <div className="flex items-center gap-1"><label htmlFor={id} className="block text-form-label font-medium text-text-primary">{label}</label>{hint && <HelpHint label={`${label}说明`} align="right" above>{hint}</HelpHint>}</div>
    <Input id={id} type="number" inputMode="decimal" min={min} step={step} value={display} onChange={event => {
      setDisplay(event.target.value);
      onChange(event.target.value === '' ? Number.NaN : event.target.valueAsNumber);
    }} onBlur={() => {
      if (!Number.isFinite(value)) return;
      try { weightInMilliTonnes(value); setDisplay(formatWeightInputValue(value)); } catch { /* Keep invalid input visible for correction. */ }
    }} />
  </div>;
}

function formatRate(rateBp: number): string {
  return `${new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 }).format(rateBp / 100)}%`;
}

export function WeightAdjustmentDetails({ ticket }: { ticket: WeighingTicket }) {
  const snapshot = ticket.adjustment;
  const original = snapshot?.original ?? originalWeights(ticket.records);
  const amount = snapshot?.netAdjustment ?? ticket.netAdjustment ?? 0;
  const mode = snapshot?.netAdjustmentMode ?? ticket.netAdjustmentMode ?? 'amount';
  const rateBp = snapshot?.netAdjustmentRateBp ?? ticket.netAdjustmentRateBp;
  const categoryName = snapshot?.netAdjustmentCategoryName ?? ticket.netAdjustmentCategoryName;
  const items = ticket.netAdjustmentItems;
  const grossDeduction = snapshot?.grossDeduction ?? 0;
  const tareDeduction = snapshot?.tareDeduction ?? 0;
  const manualReturn = ticket.records.some(record => record.source === 'manual') || ticket.grossSource === 'manual' || ticket.tareSource === 'manual';
  const adjusted = amount !== 0 || grossDeduction !== 0 || tareDeduction !== 0;
  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-3">
      {original && <><DetailItem label="原始毛重" value={formatWeight(original.gross)} /><DetailItem label="原始皮重" value={formatWeight(original.tare)} /><DetailItem label="原始净重" value={formatWeight(original.net)} /></>}
      <DetailItem label="净重调整合计" value={netAdjustmentText(amount)} />
      {items && items.length > 1 && <DetailItem label="调整方式" value={`共 ${items.length} 项（见明细）`} />}
      {(!items || items.length <= 1) && <DetailItem label="调整方式" value={mode === 'percent' && rateBp !== undefined ? `按比例 ${formatRate(rateBp)}` : '按吨'} />}
      <DetailItem label="调整备注" value={items && items.length > 1 ? [categoryName, snapshot?.reason].filter(Boolean).filter((item, index, all) => all.indexOf(item) === index).join('：') || '未填写' : [categoryName, snapshot?.reason].filter(Boolean).filter((item, index, all) => all.indexOf(item) === index).join('：') || '未填写'} />
      {(grossDeduction !== 0 || tareDeduction !== 0) && <><DetailItem label="毛重更正差额" value={formatAdjustment(grossDeduction)} /><DetailItem label="皮重更正差额" value={formatAdjustment(tareDeduction)} /></>}
    </div>
    {items && items.length > 0 && <div className="space-y-1.5">
      <p className="text-sm font-semibold">净重调整明细</p>
      <table className="w-full text-sm">
        <thead className="text-text-tertiary"><tr className="border-b border-border text-left">
          <th className="py-1.5 pr-3 font-medium">序号</th><th className="py-1.5 pr-3 font-medium">方式</th><th className="py-1.5 pr-3 font-medium">折算量（吨）</th><th className="py-1.5 pr-3 font-medium">类别</th><th className="py-1.5 font-medium">备注</th>
        </tr></thead>
        <tbody className="tabular-nums">
          {items.map((item, index) => <tr key={index} className="border-b border-border last:border-b-0">
            <td className="py-1.5 pr-3">{index + 1}</td>
            <td className="py-1.5 pr-3">{item.mode === 'percent' ? `按比例 ${item.rateBp !== undefined ? formatRate(item.rateBp) : ''}` : '按吨'}</td>
            <td className="py-1.5 pr-3">{netAdjustmentText(item.amount)}</td>
            <td className="py-1.5 pr-3">{item.categoryName || item.category || '—'}</td>
            <td className="py-1.5">{item.remark || '—'}</td>
          </tr>)}
        </tbody>
      </table>
    </div>}
    <DetailItem label="重量来源" value={manualReturn && !adjusted ? '含人工补录回磅（非设备采集）' : adjusted || snapshot?.source === 'manual' ? '人工调整（原始采集依据保留）' : '沿用采集依据'} />
    {snapshot ? <div className="grid gap-3 border-t border-border pt-3 sm:grid-cols-2">
      <DetailItem label="操作人／记录身份" value={`${snapshot.actor} · ${snapshot.role ? roleLabels[snapshot.role] : '账号操作'}`} />
      <DetailItem label="操作时间" value={beijingTime(snapshot.at)} />
      <DetailItem label="本次操作前毛／皮／净重" value={`${formatWeight(snapshot.before.gross)} / ${formatWeight(snapshot.before.tare)} / ${formatWeight(snapshot.before.net)}`} />
      <DetailItem label="本次操作后毛／皮／净重" value={`${formatWeight(snapshot.after.gross)} / ${formatWeight(snapshot.after.tare)} / ${formatWeight(snapshot.after.net)}`} />
    </div> : <p className="text-sm text-text-tertiary">历史测试单未记录净重调整，按未调整展示；不补造操作人或操作时间。</p>}
  </div>;
}

function FinalWeightField({ id, label, value, onChange }: {
  id: string; label: string; value: number; onChange: (value: number) => void;
}) {
  const [display, setDisplay] = useState(() => formatWeightInputValue(value));
  return <div className="space-y-1.5"><label htmlFor={id} className="block text-form-label font-medium text-text-primary">{label}</label><Input id={id} type="number" inputMode="decimal" min="0.001" step="0.001" value={display} onChange={event => {
    setDisplay(event.target.value);
    onChange(event.target.value === '' ? Number.NaN : event.target.valueAsNumber);
  }} onBlur={() => {
    if (!Number.isFinite(value)) return;
    try { weightInMilliTonnes(value); setDisplay(formatWeightInputValue(value)); } catch { /* Keep invalid input visible for correction. */ }
  }} /></div>;
}

export function GrossTareCorrectionFields({ original, value, onChange, showError = true }: {
  original: WeightValues;
  value: FinalWeightInput;
  onChange: (value: FinalWeightInput) => void;
  showError?: boolean;
}) {
  let final: WeightValues | undefined;
  let error = '';
  try { final = adjustedWeights(original, value); } catch (failure) { error = failure instanceof Error ? failure.message : '请核对最终毛重和皮重'; }
  return <fieldset className="min-w-0 space-y-3">
    <legend className="text-sm font-semibold">毛重／皮重更正（异常纠错）</legend>
    <p className="text-xs leading-relaxed text-text-tertiary">仅在原始毛重或皮重确有错误时使用；日常净重扣减请使用下方录入。原始采集依据保留。</p>
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="min-w-0 space-y-3 rounded-control border border-border p-3">
        <DetailItem label="原始毛重" value={<span className="font-semibold tabular-nums">{formatWeight(original.gross)}</span>} />
        <FinalWeightField id="correction-gross" label="更正后毛重（吨）" value={value.gross} onChange={gross => onChange({ ...value, gross })} />
      </div>
      <div className="min-w-0 space-y-3 rounded-control border border-border p-3">
        <DetailItem label="原始皮重" value={<span className="font-semibold tabular-nums">{formatWeight(original.tare)}</span>} />
        <FinalWeightField id="correction-tare" label="更正后皮重（吨）" value={value.tare} onChange={tare => onChange({ ...value, tare })} />
      </div>
    </div>
    {final && <DetailItem label="更正后净重（扣减前）" value={<span className="tabular-nums">{formatWeight(final.net)}</span>} />}
    {(['gross', 'tare'] as const).filter(key => Number.isFinite(value[key]) && value[key] > original[key]).map(key => <p key={key} role="status" className="text-sm text-warning">更正后{key === 'gross' ? '毛重' : '皮重'}高于原始值，核对后可继续办理，原始依据保留。</p>)}
    {showError && error && <p role="alert" className="text-sm text-error">{error}</p>}
  </fieldset>;
}

const emptyAdjustmentRow: NetAdjustmentInput = { mode: 'amount', amount: 0, rateBp: 0, category: '', remark: '' };

export function NetAdjustmentFields({ basis, value, onChange, allowUpward = false, showError = true }: {
  basis: WeightValues;
  value: NetAdjustmentsInput;
  onChange: (value: NetAdjustmentsInput) => void;
  allowUpward?: boolean;
  showError?: boolean;
}) {
  const { options: categoryOptions } = useDictOptions(DICT_CODES.WEIGHING_DEDUCTION_CATEGORY, {
    fallback: WEIGHING_DEDUCTION_CATEGORY_OPTIONS,
  });
  const rows = value.length > 0 ? value : [emptyAdjustmentRow];
  let resolved: ReturnType<typeof resolveNetAdjustments> | undefined;
  let error = '';
  let final: WeightValues | undefined;
  try {
    resolved = resolveNetAdjustments(basis, rows, allowUpward);
    final = { gross: basis.gross, tare: basis.tare, net: Math.round((basis.net - resolved.amount) * 1000) / 1000 };
  } catch (failure) { error = failure instanceof Error ? failure.message : '请核对净重调整'; }
  const amount = resolved?.amount ?? 0;
  const setRow = (index: number, row: NetAdjustmentInput) => onChange(rows.map((item, i) => i === index ? row : item));
  const setMode = (index: number, mode: NetAdjustmentMode) => setRow(index, { ...rows[index], mode, amount: mode === 'amount' ? (Number.isFinite(rows[index].amount) ? rows[index].amount : 0) : rows[index].amount, rateBp: mode === 'percent' ? (Number.isFinite(rows[index].rateBp) ? rows[index].rateBp : 0) : rows[index].rateBp });
  return <fieldset className="min-w-0 space-y-3">
    <legend className="text-sm font-semibold">核对原始重量并录入净重调整 <HelpHint label="净重调整规则">毛重、皮重沿用设备原始值；正常出单只能扣减净重，可按吨或按比例录入，可添加多项（如按比例扣水分、按吨扣杂质）。多项时各比例项均按调整前净重计算、各自四舍五入到 0.001 吨后相加。负数表示上调，仅更正重开可用。调整后净重须大于零；比例折算不足 0.001 吨时请改按吨录入。</HelpHint></legend>
    <div className="grid gap-3 sm:grid-cols-3">
      <DetailItem
        className="rounded-control border border-border border-l-[3px] border-l-info bg-neutral-background px-3 py-2.5"
        label="原始毛重"
        value={<strong className="text-2xl font-semibold tabular-nums text-text-primary">{formatWeight(basis.gross)}</strong>}
      />
      <DetailItem
        className="rounded-control border border-border border-l-[3px] border-l-text-secondary bg-neutral-background px-3 py-2.5"
        label="原始皮重"
        value={<strong className="text-2xl font-semibold tabular-nums text-text-primary">{formatWeight(basis.tare)}</strong>}
      />
      <DetailItem
        className="rounded-control border border-border border-l-[3px] border-l-success bg-neutral-background px-3 py-2.5"
        label="调整前净重"
        value={<strong className="text-2xl font-semibold tabular-nums text-success">{formatWeight(basis.net)}</strong>}
      />
    </div>
    <div className="space-y-3">
      {rows.map((row, index) => <div key={index} className="grid gap-3 rounded-control border border-border p-3 sm:grid-cols-2 lg:grid-cols-4">
        <div className="space-y-1.5">
          <span className="block text-form-label font-medium text-text-primary">第 {index + 1} 项录入方式</span>
          <div role="radiogroup" aria-label={`第 ${index + 1} 项录入方式`} className="flex h-control-md gap-2">
            {([['amount', '按吨'], ['percent', '按比例']] as const).map(([mode, label]) => <label key={mode} className="flex-1 cursor-pointer">
              <input className="peer sr-only" type="radio" name={`net-adjustment-mode-${index}`} value={mode} checked={row.mode === mode} onChange={() => setMode(index, mode)} />
              <span className="flex h-full items-center justify-center rounded-control border border-border bg-surface px-3 text-sm text-text-primary peer-checked:border-primary peer-checked:bg-info-background peer-checked:text-info peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-primary">{label}</span>
            </label>)}
          </div>
          {rows.length > 1 && <button type="button" className="text-sm text-error underline-offset-2 hover:underline" onClick={() => onChange(rows.filter((_, i) => i !== index))}>删除该项</button>}
        </div>
        {row.mode === 'amount' ? <DecimalField key={`amount-${index}`} id={`net-adjustment-amount-${index}`} label="净重调整量（吨）" value={row.amount} step="0.001" min={allowUpward ? undefined : '0'} onChange={amount => setRow(index, { ...row, amount })} hint="正数扣减；负数上调（仅更正重开）" /> : <DecimalField key={`percent-${index}`} id={`net-adjustment-rate-${index}`} label="净重调整比例（%）" value={row.rateBp / 100} step="0.01" min={allowUpward ? undefined : '0'} onChange={rate => setRow(index, { ...row, rateBp: Number.isFinite(rate) ? Math.round(rate * 100) : Number.NaN })} hint="最多两位小数；按调整前净重计算，四舍五入到 0.001 吨" />}
        <div className="space-y-1.5">
          <label htmlFor={`net-adjustment-category-${index}`} className="block text-form-label font-medium text-text-primary">调整类别（扣减或上调时必填）</label>
          <Select id={`net-adjustment-category-${index}`} value={row.category} onChange={event => setRow(index, { ...row, category: event.target.value, remark: '' })}>
            <option value="">请选择调整原因</option>
            {categoryOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
          </Select>
        </div>
        {row.category === 'OTHER' && <div className="space-y-1.5">
          <label htmlFor={`net-adjustment-remark-${index}`} className="block text-form-label font-medium text-text-primary">自定义备注（选填）</label>
          <Input id={`net-adjustment-remark-${index}`} placeholder="请输入其他调整原因" value={row.remark ?? ''} onChange={event => setRow(index, { ...row, remark: event.target.value })} />
        </div>}
      </div>)}
      <button type="button" className="text-sm text-info underline-offset-2 hover:underline" onClick={() => onChange([...rows, emptyAdjustmentRow])}>+ 添加调整项</button>
    </div>
    <div className="grid gap-4 rounded-control border border-border border-l-[3px] border-l-primary bg-neutral-background p-3 sm:grid-cols-2">
      <DetailItem label="系统计算：净重调整量合计" value={final ? <strong className="text-2xl tabular-nums">{netAdjustmentText(amount)}</strong> : '待核对调整量'} />
      <DetailItem label="调整后净重" value={final ? <strong className="text-3xl tabular-nums text-primary">{formatWeight(final.net)}</strong> : <span className="text-text-tertiary">待核对调整量</span>} />
    </div>
    {amount < 0 && <p role="status" className="text-sm text-warning">当前为向上调整净重 {formatWeight(-amount)}；仅更正重开允许，原始依据保留。</p>}
    {showError && error && <p role="alert" className="text-sm text-error">{error}</p>}
  </fieldset>;
}
