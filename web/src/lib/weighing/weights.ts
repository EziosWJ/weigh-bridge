import type { FinalWeightInput, WeighRecord, WeightDeduction, WeightValues } from '../../types/weighing';

export const weightInMilliTonnes = (value: number) => {
  if (!Number.isFinite(value)) throw new Error('重量必须是有效吨数');
  const scaled = value * 1000;
  const rounded = Math.round(scaled);
  if (!Number.isSafeInteger(rounded) || Math.abs(scaled - rounded) > 1e-7) throw new Error('重量最多保留三位小数（吨）');
  return rounded;
};
const tonnes = (value: number) => value / 1000;

export function formatWeightValue(value: number): string {
  if (!Number.isFinite(value)) return '—';
  return new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 3 }).format(value);
}

/** Formats an editable numeric field without grouping separators and insignificant zeroes. */
export function formatWeightInputValue(value: number): string {
  return Number.isFinite(value) ? value.toFixed(3).replace(/\.?0+$/, '') : '';
}

export function formatWeight(value: number): string {
  return `${formatWeightValue(value)} 吨`;
}

export function needsLegacyWeightReview(values: WeightValues | undefined): boolean {
  if (!values?.sourceKg) return false;
  const source = values.sourceKg;
  return (source.gross > 0 && values.gross <= 0)
    || (source.tare > 0 && values.tare <= 0)
    || (source.net > 0 && values.net <= 0)
    || (source.gross > 0 && source.tare > 0 && source.gross <= source.tare)
    || source.net <= 0;
}

export function needsAdjustmentReview(original: WeightValues | undefined, final: WeightValues): boolean {
  if (needsLegacyWeightReview(original) || needsLegacyWeightReview(final)) return true;
  const sourceOriginal = original?.sourceKg;
  const sourceFinal = final.sourceKg;
  return Boolean(sourceOriginal && sourceFinal && (sourceFinal.gross > sourceOriginal.gross || sourceFinal.tare > sourceOriginal.tare));
}

/** Converts legacy instrument kg values to business tonnes at 0.001 吨 precision. */
export function kilogramsToTonnes(value: number): number {
  if (!Number.isFinite(value)) return value;
  return tonnes(Math.round(value));
}

export function originalWeights(records: WeighRecord[]): WeightValues | undefined {
  const grossRecord = [...records].reverse().find(record => record.kind === 'gross');
  const tareRecord = [...records].reverse().find(record => record.kind === 'tare');
  if (!grossRecord || !tareRecord) return undefined;
  const result = weightValues(grossRecord.weight, tareRecord.weight);
  if (grossRecord.sourceWeightKg !== undefined || tareRecord.sourceWeightKg !== undefined) {
    result.sourceKg = {
      gross: grossRecord.sourceWeightKg ?? grossRecord.weight * 1000,
      tare: tareRecord.sourceWeightKg ?? tareRecord.weight * 1000,
      net: (grossRecord.sourceWeightKg ?? grossRecord.weight * 1000) - (tareRecord.sourceWeightKg ?? tareRecord.weight * 1000),
    };
  }
  return result;
}

export function weightValues(gross: number, tare: number): WeightValues {
  return { gross, tare, net: tonnes(weightInMilliTonnes(gross) - weightInMilliTonnes(tare)) };
}

export function adjustedWeights(original: WeightValues, input: FinalWeightInput): WeightValues {
  if (![original.gross, original.tare, original.net].every(Number.isFinite) || original.tare <= 0 || original.net <= 0) {
    throw new Error('原始净重必须大于零，请核对运输方向和两次称重，不能交换毛皮重');
  }
  const originalGross = weightInMilliTonnes(original.gross);
  const originalTare = weightInMilliTonnes(original.tare);
  const gross = weightInMilliTonnes(input.gross);
  const tare = weightInMilliTonnes(input.tare);
  if (gross <= 0 || tare <= 0) throw new Error('最终毛重和皮重必须大于零');
  if (gross > originalGross || tare > originalTare) throw new Error('最终重量不能大于对应原始重量');
  const net = gross - tare;
  if (gross <= tare || tare <= 0 || net <= 0) throw new Error('最终毛重必须大于最终皮重，最终皮重和净重必须大于零');
  return { gross: tonnes(gross), tare: tonnes(tare), net: tonnes(net) };
}

export function weightDeduction(original: WeightValues, final: WeightValues, reason?: string): WeightDeduction {
  const grossDeduction = tonnes(weightInMilliTonnes(original.gross) - weightInMilliTonnes(final.gross));
  const tareDeduction = tonnes(weightInMilliTonnes(original.tare) - weightInMilliTonnes(final.tare));
  return { grossDeduction, tareDeduction, ...(reason?.trim() ? { reason: reason.trim() } : {}) };
}
