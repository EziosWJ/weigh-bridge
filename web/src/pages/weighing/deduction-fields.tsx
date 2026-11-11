import { useEffect, useState } from 'react';
import { Plus } from 'lucide-react';
import { DetailItem } from '@/components/common/detail-item';
import { HelpHint } from '@/components/common/help-hint';
import { Button } from '@/components/ui/button';
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
  resolveNetAdjustment,
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
  const [display, setDisplay] = useState(() => (Number.isFinite(value) && value !== 0 ? formatWeightInputValue(value) : ''));
  return <div className="space-y-1.5">
    <div className="flex items-center gap-1"><label htmlFor={id} className="block text-form-label font-medium text-text-primary">{label}</label>{hint && <HelpHint label={`${label}说明`} align="right" above>{hint}</HelpHint>}</div>
    <Input id={id} type="number" inputMode="decimal" min={min} step={step} value={display} onChange={event => {
      setDisplay(event.target.value);
      onChange(event.target.value === '' ? 0 : event.target.valueAsNumber);
    }} onBlur={() => {
      if (display === '' || !Number.isFinite(value)) return;
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
    onChange(event.target.value === '' ? 0 : event.target.valueAsNumber);
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

const adjustmentRow = (category = ''): NetAdjustmentInput => ({
  mode: 'amount', amount: 0, rateBp: 0, category, remark: '',
});

export function NetAdjustmentFields({ basis, value, onChange, showError = true, onValidationChange }: {
  basis: WeightValues;
  value: NetAdjustmentsInput;
  onChange: (value: NetAdjustmentsInput) => void;
  showError?: boolean;
  onValidationChange?: (error: string) => void;
}) {
  const { options: categoryOptions } = useDictOptions(DICT_CODES.WEIGHING_DEDUCTION_CATEGORY, {
    fallback: WEIGHING_DEDUCTION_CATEGORY_OPTIONS,
  });
  const [removed, setRemoved] = useState<{ index: number; row: NetAdjustmentInput } | null>(null);
  const [revision, setRevision] = useState(0);
  const rows = value.length > 0 ? value : [adjustmentRow()];
  let resolved: ReturnType<typeof resolveNetAdjustments> | undefined;
  let error = '';
  try {
    resolved = resolveNetAdjustments(basis, rows);
    const unavailable = rows.find(row => (row.mode === 'amount' ? row.amount : row.rateBp) !== 0 && !categoryOptions.some(option => option.value === row.category));
    if (unavailable) throw new Error('扣减原因已停用，请重新选择');
  } catch (failure) { error = failure instanceof Error ? failure.message : '请核对扣减量'; resolved = undefined; }
  useEffect(() => { onValidationChange?.(error); }, [error, onValidationChange]);
  const amount = resolved?.amount ?? 0;
  const setRow = (index: number, row: NetAdjustmentInput) => onChange(rows.map((item, i) => i === index ? row : item));
  const setMode = (index: number, mode: NetAdjustmentMode) => setRow(index, { ...rows[index], mode });
  const addRow = () => {
    onChange([...rows, adjustmentRow()]);
  };
  const removeRow = (index: number) => {
    setRemoved({ index, row: rows[index] });
    onChange(rows.filter((_, i) => i !== index));
    setRevision(current => current + 1);
  };
  const undo = () => {
    if (!removed) return;
    const restored = value.length ? [...value] : [];
    restored.splice(Math.min(removed.index, restored.length), 0, removed.row);
    onChange(restored);
    setRemoved(null);
    setRevision(current => current + 1);
  };
  return <fieldset className="min-w-0 space-y-3">
    <legend className="text-sm font-semibold">核对重量并录入扣减 <HelpHint label="净重调整规则">空值或零不扣减。各比例项均以调整前净重为基数，各自四舍五入到 0.001 吨后相加。调整后净重须大于零；比例折算不足 0.001 吨时请改按吨录入。</HelpHint></legend>
    <div className="grid gap-3 sm:grid-cols-3">
      <DetailItem label="原始毛重" value={<strong className="text-xl tabular-nums">{formatWeight(basis.gross)}</strong>} />
      <DetailItem label="原始皮重" value={<strong className="text-xl tabular-nums">{formatWeight(basis.tare)}</strong>} />
      <DetailItem label="调整前净重" value={<strong className="text-xl tabular-nums">{formatWeight(basis.net)}</strong>} />
    </div>
    <div className="space-y-3">
      {rows.map((row, index) => {
        let itemAmount: number | undefined;
        let itemError = '';
        try { itemAmount = resolveNetAdjustment(basis, row).amount; } catch (failure) { itemError = failure instanceof Error ? failure.message : '请核对扣减'; }
        const unavailable = row.category && !categoryOptions.some(option => option.value === row.category);
        if (unavailable && (row.mode === 'percent' ? row.rateBp : row.amount) !== 0) itemError = '扣减原因已停用，请重新选择';
        return <div key={`${index}-${revision}`} className="grid min-w-0 gap-3 rounded-control border border-border p-3 sm:grid-cols-2 lg:grid-cols-4">
          <div className="space-y-1.5">
            <label htmlFor={`net-adjustment-category-${index}`} className="block text-form-label font-medium text-text-primary">第 {index + 1} 项扣减原因</label>
            <Select id={`net-adjustment-category-${index}`} value={row.category} onChange={event => setRow(index, { ...row, category: event.target.value, remark: '' })}>
              <option value="">请选择扣减原因</option>
              {unavailable && <option value={row.category}>原原因已停用，请重新选择</option>}
              {categoryOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </Select>
          </div>
          <div className="space-y-1.5">
            <span className="block text-form-label font-medium text-text-primary">录入方式</span>
            <div role="radiogroup" aria-label={`第 ${index + 1} 项录入方式`} className="flex h-control-md gap-2">
              {([['amount', '按吨'], ['percent', '按比例']] as const).map(([mode, label]) => <label key={mode} className="flex-1 cursor-pointer">
                <input className="peer sr-only" type="radio" name={`net-adjustment-mode-${index}`} value={mode} checked={row.mode === mode} onChange={() => setMode(index, mode)} />
                <span className="flex h-full items-center justify-center rounded-control border border-border bg-surface px-3 text-sm text-text-primary peer-checked:border-primary peer-checked:bg-info-background peer-checked:text-info peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-primary">{label}</span>
              </label>)}
            </div>
          </div>
          {row.mode === 'amount' ? <DecimalField key={`amount-${index}`} id={`net-adjustment-amount-${index}`} label="扣减重量（吨）" value={row.amount} step="0.001" min="0" onChange={amount => setRow(index, { ...row, amount })} /> : <DecimalField key={`percent-${index}`} id={`net-adjustment-rate-${index}`} label="扣减比例（%）" value={row.rateBp / 100} step="0.01" min="0" onChange={rate => setRow(index, { ...row, rateBp: Math.round(rate * 100 * 1e8) / 1e8 })} hint="最多两位小数；按调整前净重计算，四舍五入到 0.001 吨" />}
          <div className="flex items-end justify-between gap-2"><span className="text-sm text-text-secondary tabular-nums">{itemError ? <span role="alert" className="text-error">{itemError}</span> : itemAmount ? <>{row.mode === 'percent' && `${formatWeight(basis.net)} × ${formatRate(row.rateBp)} = `}扣减 <strong className="text-text-primary">{formatWeight(itemAmount)}</strong></> : '不扣减'}</span><Button size="sm" variant="secondary" onClick={() => removeRow(index)}>删除该项</Button></div>
          {row.category === 'OTHER' && <div className="space-y-1.5 sm:col-span-2 lg:col-span-4"><label htmlFor={`net-adjustment-remark-${index}`} className="block text-form-label font-medium text-text-primary">补充说明（选填）</label><Input id={`net-adjustment-remark-${index}`} placeholder="请输入其他扣减原因" value={row.remark ?? ''} onChange={event => setRow(index, { ...row, remark: event.target.value })} /></div>}
        </div>;
      })}
      <div className="flex items-center gap-3"><Button type="button" size="md" variant="secondary" onClick={addRow}><Plus className="h-4 w-4" aria-hidden />添加扣减项</Button>{removed && <Button type="button" size="md" variant="secondary" onClick={undo}>撤销删除</Button>}</div>
    </div>
    <div className="rounded-control border border-border border-l-[3px] border-l-primary bg-neutral-background p-3" aria-label="扣减合计">
      {resolved ? <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 tabular-nums"><span>调整前净重 <strong>{formatWeight(basis.net)}</strong></span><span aria-hidden>−</span><span>扣减合计 <strong>{formatWeight(amount)}</strong></span><span aria-hidden>=</span><span>出单净重 <strong className="text-3xl text-primary">{formatWeight(Math.round((basis.net - amount) * 1000) / 1000)}</strong></span>{amount === 0 && <span className="text-sm text-text-secondary">未扣减</span>}</div> : <span className="text-text-tertiary">请核对扣减项后计算出单净重</span>}
    </div>
    {showError && error && <p role="alert" className="text-sm text-error">{error}</p>}
  </fieldset>;
}
