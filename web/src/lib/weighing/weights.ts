import type { FinalWeightInput, NetAdjustmentInput, WeighRecord, WeightDeduction, WeightValues } from '../../types/weighing';

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
    throw new Error('原始净重必须大于零。请返回核对运输方向和两次称重，待设备提供正确重量后重新采集；首次重量有误时须重称，不能通过修改最终重量或交换毛皮重出单。');
  }
  weightInMilliTonnes(original.gross);
  weightInMilliTonnes(original.tare);
  const gross = weightInMilliTonnes(input.gross);
  const tare = weightInMilliTonnes(input.tare);
  if (gross <= 0 || tare <= 0) throw new Error('最终毛重和皮重必须大于零');
  const net = gross - tare;
  if (gross <= tare || tare <= 0 || net <= 0) throw new Error('最终毛重必须大于最终皮重，最终皮重和净重必须大于零');
  return { gross: tonnes(gross), tare: tonnes(tare), net: tonnes(net) };
}

export function weightDeduction(original: WeightValues, final: WeightValues, reason?: string): WeightDeduction {
  const grossDeduction = tonnes(weightInMilliTonnes(original.gross) - weightInMilliTonnes(final.gross));
  const tareDeduction = tonnes(weightInMilliTonnes(original.tare) - weightInMilliTonnes(final.tare));
  return { grossDeduction, tareDeduction, ...(reason?.trim() ? { reason: reason.trim() } : {}) };
}

export function formatAdjustment(value: number): string {
  return `${formatWeight(value)}（${value < 0 ? '上调' : value > 0 ? '下调' : '未调整'}）`;
}

export interface ResolvedNetAdjustment { amount: number; rateBp?: number }

function roundRatio(numerator: number, denominator: number): number {
  const half = denominator / 2;
  return numerator >= 0 ? Math.floor((numerator + half) / denominator) : -Math.floor((-numerator + half) / denominator);
}

/** Resolves the signed net adjustment in tonnes from the pre-adjustment net basis. */
export function resolveNetAdjustment(basis: WeightValues, input: NetAdjustmentInput, allowUpward = false): ResolvedNetAdjustment {
  if (![basis.gross, basis.tare, basis.net].every(Number.isFinite) || basis.tare <= 0 || basis.net <= 0) {
    throw new Error('原始净重必须大于零。请返回核对运输方向和两次称重，待设备提供正确重量后重新采集；首次重量有误时须重称，不能通过调整净重出单。');
  }
  const grossMilli = weightInMilliTonnes(basis.gross);
  const tareMilli = weightInMilliTonnes(basis.tare);
  const netMilli = grossMilli - tareMilli;
  if (netMilli <= 0) throw new Error('原始毛重必须大于原始皮重');
  let amountMilli = 0;
  let rateBp: number | undefined;
  if (input.mode === 'percent') {
    if (!Number.isInteger(input.rateBp) || Math.abs(input.rateBp) > 10000) throw new Error('调整比例最多两位小数，且绝对值不超过 100%');
    rateBp = input.rateBp;
    if (!allowUpward && rateBp < 0) throw new Error('正常出单只能扣减净重；提高净重请在更正重开中办理');
    if (rateBp !== 0) {
      amountMilli = roundRatio(netMilli * rateBp, 10000);
      if (amountMilli === 0) throw new Error('调整比例折算不足 0.001 吨，请改为按吨录入');
    }
  } else {
    amountMilli = weightInMilliTonnes(input.amount);
    if (!allowUpward && amountMilli < 0) throw new Error('正常出单只能扣减净重；提高净重请在更正重开中办理');
  }
  const finalNetMilli = netMilli - amountMilli;
  if (finalNetMilli <= 0 || finalNetMilli >= grossMilli) throw new Error('调整后净重必须大于零且小于最终毛重');
  if (amountMilli !== 0 && !input.category.trim()) throw new Error('请选择净重调整备注');
  return { amount: tonnes(amountMilli), ...(rateBp === undefined ? {} : { rateBp }) };
}

export function netAdjustedWeights(basis: WeightValues, input: NetAdjustmentInput, allowUpward = false): WeightValues {
  const resolved = resolveNetAdjustment(basis, input, allowUpward);
  return { gross: basis.gross, tare: basis.tare, net: tonnes(weightInMilliTonnes(basis.net) - weightInMilliTonnes(resolved.amount)) };
}

export function netAdjustmentText(amount: number): string {
  if (!Number.isFinite(amount) || amount === 0) return '未调整';
  return `${formatWeight(Math.abs(amount))}（${amount > 0 ? '扣减' : '上调'}）`;
}
