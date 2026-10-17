import { useState } from 'react';
import { DetailItem } from '@/components/common/detail-item';
import { Field } from '@/components/common/field';
import { Input } from '@/components/ui/input';
import { adjustedWeights, formatWeight, formatWeightInputValue, weightDeduction, weightInMilliTonnes } from '@/lib/weighing/weights';
import { originalWeights } from '@/lib/weighing/weights';
import { beijingTime } from '@/lib/weighing/query';
import { roleLabels, type FinalWeightInput, type TestTicket, type WeightValues } from '@/types/weighing';

function FinalWeightField({ id, label, max, value, onChange }: {
  id: string; label: string; max: number; value: number; onChange: (value: number) => void;
}) {
  const [display, setDisplay] = useState(() => formatWeightInputValue(value));
  return <Field label={label} htmlFor={id}><Input id={id} type="number" inputMode="decimal" min="0.001" max={max} step="0.001" value={display} onChange={event => {
    setDisplay(event.target.value);
    onChange(event.target.value === '' ? Number.NaN : event.target.valueAsNumber);
  }} onBlur={() => {
    if (!Number.isFinite(value)) return;
    try { weightInMilliTonnes(value); setDisplay(formatWeightInputValue(value)); } catch { /* Keep invalid input visible for correction. */ }
  }} /></Field>;
}

export function WeightAdjustmentDetails({ ticket }: { ticket: TestTicket }) {
  const snapshot = ticket.adjustment;
  const original = snapshot?.original ?? originalWeights(ticket.records);
  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-3">{original && <><DetailItem label="原始毛重" value={formatWeight(original.gross)} /><DetailItem label="原始皮重" value={formatWeight(original.tare)} /><DetailItem label="原始净重" value={formatWeight(original.net)} /></>}<DetailItem label="毛重扣减量" value={formatWeight(snapshot?.grossDeduction ?? 0)} /><DetailItem label="皮重扣减量" value={formatWeight(snapshot?.tareDeduction ?? 0)} /><DetailItem label="扣减原因" value={snapshot?.reason || '未填写'} /></div>
    {snapshot ? <div className="grid gap-3 border-t border-border pt-3 sm:grid-cols-2"><DetailItem label="操作人／演示岗位" value={`${snapshot.actor} · ${roleLabels[snapshot.role]}`} /><DetailItem label="操作时间（北京）" value={beijingTime(snapshot.at)} /><DetailItem label="本次操作前毛／皮／净重" value={`${formatWeight(snapshot.before.gross)} / ${formatWeight(snapshot.before.tare)} / ${formatWeight(snapshot.before.net)}`} /><DetailItem label="本次操作后毛／皮／净重" value={`${formatWeight(snapshot.after.gross)} / ${formatWeight(snapshot.after.tare)} / ${formatWeight(snapshot.after.net)}`} /></div> : <p className="text-sm text-text-tertiary">历史测试单未记录人工扣减，按零扣减展示；不补造操作人或操作时间。</p>}
  </div>;
}

export function WeightDeductionFields({ original, value, onChange }: {
  original: WeightValues; value: FinalWeightInput; onChange: (value: FinalWeightInput) => void;
}) {
  let final: WeightValues | undefined;
  let error = '';
  try { final = adjustedWeights(original, value); } catch (failure) { error = failure instanceof Error ? failure.message : '请核对最终重量'; }
  const deductions = final ? weightDeduction(original, final) : undefined;
  return <fieldset className="space-y-4 rounded-control border border-border p-4">
    <legend className="px-1 text-sm font-semibold">核对原始重量并录入最终重量</legend>
    <div className="grid gap-3 sm:grid-cols-3"><DetailItem label="原始毛重" value={formatWeight(original.gross)} /><DetailItem label="原始皮重" value={formatWeight(original.tare)} /><DetailItem label="原始净重" value={formatWeight(original.net)} /></div>
    <div className="grid gap-3 sm:grid-cols-2">
      <FinalWeightField id="final-gross" label="最终毛重（吨）" max={original.gross} value={value.gross} onChange={gross => onChange({ ...value, gross })} />
      <FinalWeightField id="final-tare" label="最终皮重（吨）" max={original.tare} value={value.tare} onChange={tare => onChange({ ...value, tare })} />
    </div>
    <div className="grid gap-3 sm:grid-cols-2"><DetailItem label="系统计算：毛重扣减量" value={deductions ? formatWeight(deductions.grossDeduction) : '—'} /><DetailItem label="系统计算：皮重扣减量" value={deductions ? formatWeight(deductions.tareDeduction) : '—'} /></div>
    <Field label="扣减原因（选填）" htmlFor="deduction-reason"><Input id="deduction-reason" value={value.reason ?? ''} onChange={event => onChange({ ...value, reason: event.target.value })} /></Field>
    {error ? <p role="alert" className="text-sm text-error">{error}</p> : final && <div className="grid gap-3 border-t border-border pt-3 sm:grid-cols-3"><DetailItem label="最终毛重" value={formatWeight(final.gross)} /><DetailItem label="最终皮重" value={formatWeight(final.tare)} /><DetailItem label="最终净重" value={<strong>{formatWeight(final.net)}</strong>} /></div>}
    <p className="text-xs text-text-tertiary">对照原始重量填写扣减后的毛重和皮重。最终值只能等于或小于原值，扣减量由系统计算；扣减皮重可能使最终净重增加。</p>
  </fieldset>;
}
