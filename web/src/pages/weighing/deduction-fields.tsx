import { useState } from 'react';
import { DetailItem } from '@/components/common/detail-item';
import { Input } from '@/components/ui/input';
import { adjustedWeights, formatAdjustment, formatWeight, formatWeightInputValue, weightInMilliTonnes } from '@/lib/weighing/weights';
import { originalWeights } from '@/lib/weighing/weights';
import { beijingTime } from '@/lib/weighing/query';
import { roleLabels, type FinalWeightInput, type WeighingTicket, type WeightValues } from '@/types/weighing';

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

export function WeightAdjustmentDetails({ ticket }: { ticket: WeighingTicket }) {
  const snapshot = ticket.adjustment;
  const original = snapshot?.original ?? originalWeights(ticket.records);
  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-3">{original && <><DetailItem label="原始毛重" value={formatWeight(original.gross)} /><DetailItem label="原始皮重" value={formatWeight(original.tare)} /><DetailItem label="原始净重" value={formatWeight(original.net)} /></>}<DetailItem label="毛重调整差额" value={formatAdjustment(snapshot?.grossDeduction ?? 0)} /><DetailItem label="皮重调整差额" value={formatAdjustment(snapshot?.tareDeduction ?? 0)} /><DetailItem label="调整备注" value={snapshot?.reason || '未填写'} /></div>
    <DetailItem label="重量来源" value={snapshot?.source === 'manual' || (snapshot && (snapshot.grossDeduction !== 0 || snapshot.tareDeduction !== 0)) ? '人工调整最终重量（原始采集依据保留）' : '沿用采集依据'} />
    {snapshot ? <div className="grid gap-3 border-t border-border pt-3 sm:grid-cols-2"><DetailItem label="操作人／记录身份" value={`${snapshot.actor} · ${snapshot.role ? roleLabels[snapshot.role] : '账号操作'}`} /><DetailItem label="操作时间（北京）" value={beijingTime(snapshot.at)} /><DetailItem label="本次操作前毛／皮／净重" value={`${formatWeight(snapshot.before.gross)} / ${formatWeight(snapshot.before.tare)} / ${formatWeight(snapshot.before.net)}`} /><DetailItem label="本次操作后毛／皮／净重" value={`${formatWeight(snapshot.after.gross)} / ${formatWeight(snapshot.after.tare)} / ${formatWeight(snapshot.after.net)}`} /></div> : <p className="text-sm text-text-tertiary">历史测试单未记录人工调整，按未调整展示；不补造操作人或操作时间。</p>}
  </div>;
}

function deductionDisplay(original: number, final: number): string {
  try {
    const originalUnits = weightInMilliTonnes(original);
    const finalUnits = weightInMilliTonnes(final);
    if (finalUnits <= 0) return '—';
    return formatAdjustment((originalUnits - finalUnits) / 1000);
  } catch { return '—'; }
}

export function WeightDeductionFields({ original, value, onChange, showError = true }: {
  original: WeightValues; value: FinalWeightInput; onChange: (value: FinalWeightInput) => void; showError?: boolean;
}) {
  let final: WeightValues | undefined;
  let error = '';
  try { final = adjustedWeights(original, value); } catch (failure) { error = failure instanceof Error ? failure.message : '请核对最终重量'; }
  return <fieldset className="min-w-0 space-y-3">
    <legend className="text-sm font-semibold">核对原始重量并录入最终重量</legend>
    <p className="text-xs leading-relaxed text-text-tertiary">无需调整时保留原值；可上调或下调最终重量，系统自动计算差额。单位：吨。</p>
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="min-w-0 space-y-3 rounded-control border border-border p-3">
        <DetailItem label="原始毛重" value={<span className="font-semibold tabular-nums">{formatWeight(original.gross)}</span>} />
        <FinalWeightField id="final-gross" label="最终毛重（吨）" value={value.gross} onChange={gross => onChange({ ...value, gross })} />
        <div className="border-t border-border pt-3"><DetailItem label="系统计算：毛重调整差额" value={deductionDisplay(original.gross, value.gross)} /></div>
      </div>
      <div className="min-w-0 space-y-3 rounded-control border border-border p-3">
        <DetailItem label="原始皮重" value={<span className="font-semibold tabular-nums">{formatWeight(original.tare)}</span>} />
        <FinalWeightField id="final-tare" label="最终皮重（吨）" value={value.tare} onChange={tare => onChange({ ...value, tare })} />
        <div className="border-t border-border pt-3"><DetailItem label="系统计算：皮重调整差额" value={deductionDisplay(original.tare, value.tare)} /></div>
      </div>
    </div>
    <div className="grid grid-cols-2 items-center gap-4 rounded-control bg-neutral-background p-3">
      <DetailItem label="原始净重" value={formatWeight(original.net)} />
      <DetailItem label="最终净重" value={final ? <strong className="text-2xl tabular-nums text-primary">{formatWeight(final.net)}</strong> : <span className="text-text-tertiary">待核对重量</span>} />
    </div>
    <div className="space-y-1.5"><label htmlFor="deduction-reason" className="block text-form-label font-medium text-text-primary">调整备注（选填）</label><Input id="deduction-reason" placeholder="磅精度调整（未填或清空时使用）" value={value.reason ?? ''} onChange={event => onChange({ ...value, reason: event.target.value })} /></div>
    {(['gross', 'tare'] as const).filter(key => Number.isFinite(value[key]) && value[key] > original[key]).map(key => <p key={key} role="status" className="text-sm text-warning">最终{key === 'gross' ? '毛重' : '皮重'}高于原始值 {formatWeight(Math.round((value[key] - original[key]) * 1000) / 1000)}，属于人工上调；核对后可继续办理，原始依据保留。</p>)}
    {showError && error && <p role="alert" className="text-sm text-error">{error}</p>}
    <p className="text-xs leading-relaxed text-text-tertiary">最终重量须为有限正数，精确至 0.001 吨，毛重须大于皮重。差额＝原始值－最终值；负数表示上调。</p>
  </fieldset>;
}
