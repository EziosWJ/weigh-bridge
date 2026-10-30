import { useState } from 'react';
import { DetailItem } from '@/components/common/detail-item';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { DICT_CODES, WEIGHING_DEDUCTION_CATEGORY_OPTIONS } from '@/constants/dicts';
import { useDictOptions } from '@/hooks/use-dict-options';
import {
  adjustedWeights,
  formatAdjustment,
  formatWeight,
  formatWeightInputValue,
  netAdjustedWeights,
  netAdjustmentText,
  originalWeights,
  weightInMilliTonnes,
} from '@/lib/weighing/weights';
import { beijingTime } from '@/lib/weighing/query';
import {
  roleLabels,
  type FinalWeightInput,
  type NetAdjustmentInput,
  type NetAdjustmentMode,
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
    <label htmlFor={id} className="block text-form-label font-medium text-text-primary">{label}</label>
    <Input id={id} type="number" inputMode="decimal" min={min} step={step} value={display} onChange={event => {
      setDisplay(event.target.value);
      onChange(event.target.value === '' ? Number.NaN : event.target.valueAsNumber);
    }} onBlur={() => {
      if (!Number.isFinite(value)) return;
      try { weightInMilliTonnes(value); setDisplay(formatWeightInputValue(value)); } catch { /* Keep invalid input visible for correction. */ }
    }} />
    {hint && <p className="text-xs text-text-tertiary">{hint}</p>}
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
  const grossDeduction = snapshot?.grossDeduction ?? 0;
  const tareDeduction = snapshot?.tareDeduction ?? 0;
  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-3">
      {original && <><DetailItem label="原始毛重" value={formatWeight(original.gross)} /><DetailItem label="原始皮重" value={formatWeight(original.tare)} /><DetailItem label="原始净重" value={formatWeight(original.net)} /></>}
      <DetailItem label="净重调整" value={netAdjustmentText(amount)} />
      <DetailItem label="调整方式" value={mode === 'percent' && rateBp !== undefined ? `按比例 ${formatRate(rateBp)}` : '按吨'} />
      <DetailItem label="调整类别" value={categoryName || '未填写'} />
      {(grossDeduction !== 0 || tareDeduction !== 0) && <><DetailItem label="毛重更正差额" value={formatAdjustment(grossDeduction)} /><DetailItem label="皮重更正差额" value={formatAdjustment(tareDeduction)} /></>}
      <DetailItem label="调整备注" value={snapshot?.reason || ticket.adjustment?.reason || '未填写'} />
    </div>
    <DetailItem label="重量来源" value={snapshot?.source === 'manual' || amount !== 0 || grossDeduction !== 0 || tareDeduction !== 0 ? '人工调整（原始采集依据保留）' : '沿用采集依据'} />
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

export function NetAdjustmentFields({ basis, value, onChange, allowUpward = false, showError = true }: {
  basis: WeightValues;
  value: NetAdjustmentInput;
  onChange: (value: NetAdjustmentInput) => void;
  allowUpward?: boolean;
  showError?: boolean;
}) {
  const { options: categoryOptions } = useDictOptions(DICT_CODES.WEIGHING_DEDUCTION_CATEGORY, {
    fallback: WEIGHING_DEDUCTION_CATEGORY_OPTIONS,
  });
  let final: WeightValues | undefined;
  let error = '';
  try { final = netAdjustedWeights(basis, value, allowUpward); } catch (failure) { error = failure instanceof Error ? failure.message : '请核对净重调整'; }
  const amount = final ? final.net - basis.net : 0;
  const setMode = (mode: NetAdjustmentMode) => onChange({ ...value, mode, amount: mode === 'amount' ? (Number.isFinite(value.amount) ? value.amount : 0) : value.amount, rateBp: mode === 'percent' ? (Number.isFinite(value.rateBp) ? value.rateBp : 0) : value.rateBp });
  return <fieldset className="min-w-0 space-y-3">
    <legend className="text-sm font-semibold">核对原始重量并录入净重调整</legend>
    <p className="text-xs leading-relaxed text-text-tertiary">毛重、皮重沿用设备原始值；正常出单只能扣减净重，可按吨或按比例录入。负数表示上调，仅更正重开可用。</p>
    <div className="grid gap-3 sm:grid-cols-3">
      <DetailItem label="原始毛重" value={<span className="font-semibold tabular-nums">{formatWeight(basis.gross)}</span>} />
      <DetailItem label="原始皮重" value={<span className="font-semibold tabular-nums">{formatWeight(basis.tare)}</span>} />
      <DetailItem label="调整前净重" value={<span className="font-semibold tabular-nums">{formatWeight(basis.net)}</span>} />
    </div>
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="space-y-1.5">
        <label htmlFor="net-adjustment-mode" className="block text-form-label font-medium text-text-primary">录入方式</label>
        <Select id="net-adjustment-mode" value={value.mode} onChange={event => setMode(event.target.value as NetAdjustmentMode)}>
          <option value="amount">按吨</option>
          <option value="percent">按比例</option>
        </Select>
      </div>
      {value.mode === 'amount' ? <DecimalField key="amount" id="net-adjustment-amount" label="净重调整量（吨）" value={value.amount} step="0.001" min={allowUpward ? undefined : '0'} onChange={amount => onChange({ ...value, amount })} hint="正数扣减；负数上调（仅更正重开）" /> : <DecimalField key="percent" id="net-adjustment-rate" label="净重调整比例（%）" value={value.rateBp / 100} step="0.01" min={allowUpward ? undefined : '0'} onChange={rate => onChange({ ...value, rateBp: Number.isFinite(rate) ? Math.round(rate * 100) : Number.NaN })} hint="最多两位小数；按调整前净重计算，四舍五入到 0.001 吨" />}
    </div>
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="space-y-1.5">
        <label htmlFor="net-adjustment-category" className="block text-form-label font-medium text-text-primary">调整类别（扣减或上调时必填）</label>
        <Select id="net-adjustment-category" value={value.category} onChange={event => onChange({ ...value, category: event.target.value })}>
          <option value="">请选择类别</option>
          {categoryOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
        </Select>
      </div>
      <div className="space-y-1.5">
        <label htmlFor="net-adjustment-remark" className="block text-form-label font-medium text-text-primary">调整备注（选填）</label>
        <Input id="net-adjustment-remark" placeholder="补充说明，可不填" value={value.remark ?? ''} onChange={event => onChange({ ...value, remark: event.target.value })} />
      </div>
    </div>
    <div className="grid gap-4 rounded-control bg-neutral-background p-3 sm:grid-cols-2">
      <DetailItem label="系统计算：净重调整量" value={final ? netAdjustmentText(amount) : '待核对调整量'} />
      <DetailItem label="调整后净重" value={final ? <strong className="text-2xl tabular-nums text-primary">{formatWeight(final.net)}</strong> : <span className="text-text-tertiary">待核对调整量</span>} />
    </div>
    {amount < 0 && <p role="status" className="text-sm text-warning">当前为向上调整净重 {formatWeight(-amount)}；仅更正重开允许，原始依据保留。</p>}
    {showError && error && <p role="alert" className="text-sm text-error">{error}</p>}
    <p className="text-xs leading-relaxed text-text-tertiary">调整后净重须大于零且小于最终毛重。按比例时以调整前净重为基数，四舍五入到 0.001 吨；折算不足 0.001 吨请改按吨录入。</p>
  </fieldset>;
}
