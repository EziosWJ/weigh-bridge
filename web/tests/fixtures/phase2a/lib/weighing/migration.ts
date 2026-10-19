import { emptyTaskInput } from '../../types/weighing';
import type { BusinessInfo, Material, Party, WeighingState } from '../../types/weighing';
import type { SimulatedCapture, TestTicket, TransportTask, WeighRecord, WeightAdjustment, WeightValues } from '../../types/weighing';
import { createInitialState } from './seed';
import { kilogramsToTonnes, weightValues } from './weights';

/** 仅升级本模块旧测试缓存；历史称重与快照不从当前基础资料反填。 */
export function migrateWeighingState(value: unknown, now = new Date().toISOString()): WeighingState {
  const initial = createInitialState(now);
  if (!value || typeof value !== 'object') return initial;
  const data = value as Record<string, unknown>;
  if (![1, 2, 3].includes(Number(data.version)) || !Array.isArray(data.tasks) || !Array.isArray(data.tickets) || !Array.isArray(data.materials) || !Array.isArray(data.parties) || !data.config) return initial;
  const convertLegacyKg = data.version !== 3;
  const saved = structuredClone(data) as unknown as WeighingState;
  const usedCodes = new Set<string>();
  const code = (existing: string | undefined, prefix: string, index: number) => {
    let result = existing || `${prefix}${String(index + 1).padStart(4, '0')}`;
    let suffix = index + 1;
    while (usedCodes.has(result)) result = `${prefix}${String(++suffix).padStart(4, '0')}`;
    usedCodes.add(result); return result;
  };
  const materials: Material[] = saved.materials.map((item, index) => ({ ...item, code: code(item.code, 'M', index), categoryId: item.categoryId ?? '', spec: item.spec ?? '', unit: '吨', remark: item.remark ?? '' }));
  const parties: Party[] = saved.parties.map((item, index) => {
    const legacy = item as Party & { kind?: 'supplier' | 'customer' | 'carrier' };
    const { kind: _kind, ...rest } = legacy; void _kind;
    return { ...rest, code: code(item.code, 'B', index), shortName: item.shortName ?? '', nature: item.nature ?? 'enterprise', roles: item.roles ?? (legacy.kind ? [legacy.kind] : ['supplier']), phone: item.phone ?? '', address: item.address ?? '', creditCode: item.creditCode ?? '', remark: item.remark ?? '', contacts: item.contacts ?? [] };
  });
  const snapshot = <T extends BusinessInfo>(record: T): T => ({
    ...record, materialCode: record.materialCode ?? '', materialSpec: record.materialSpec ?? '', materialCategoryName: record.materialCategoryName ?? '',
    partyCode: record.partyCode ?? '', partyPhone: record.partyPhone ?? '', partyAddress: record.partyAddress ?? '',
    carrierId: record.carrierId ?? '', carrierName: record.carrierName ?? '', carrierCode: record.carrierCode ?? '',
    contactName: record.contactName ?? '', contactPhone: record.contactPhone ?? '', driverPhone: record.driverPhone ?? '',
  });
  const convertValues = (values: WeightValues): WeightValues => {
    const current = weightValues(kilogramsToTonnes(values.gross), kilogramsToTonnes(values.tare));
    return { ...current, sourceKg: values.sourceKg ?? { gross: values.gross, tare: values.tare, net: values.net } };
  };
  const convertCapture = (capture: SimulatedCapture): SimulatedCapture => ({
    ...capture, weight: kilogramsToTonnes(capture.weight), sourceWeightKg: capture.sourceWeightKg ?? capture.weight,
  });
  const convertRecord = (record: WeighRecord): WeighRecord => ({
    ...record, weight: kilogramsToTonnes(record.weight), sourceWeightKg: record.sourceWeightKg ?? record.weight,
    ...(record.capture ? { capture: convertCapture(record.capture) } : {}),
  });
  const convertAdjustment = (adjustment: WeightAdjustment): WeightAdjustment => {
    const original = convertValues(adjustment.original);
    const before = convertValues(adjustment.before);
    const after = convertValues(adjustment.after);
    return { ...adjustment, ...weightDeductionValues(original, after, adjustment.reason), original, before, after };
  };
  const migratedTasks: TransportTask[] = saved.tasks.map(task => ({
    ...snapshot(task), records: convertLegacyKg ? task.records.map(convertRecord) : task.records,
  }));
  const migratedTickets: TestTicket[] = saved.tickets.map(ticket => {
    if (!convertLegacyKg) return { ...snapshot(ticket), unit: '吨' };
    const original = weightValues(kilogramsToTonnes(ticket.gross), kilogramsToTonnes(ticket.tare));
    return {
      ...snapshot(ticket), ...original,
      sourceKg: ticket.sourceKg ?? { gross: ticket.gross, tare: ticket.tare, net: ticket.net },
      unit: '吨', records: ticket.records.map(convertRecord),
      ...(ticket.adjustment ? { adjustment: convertAdjustment(ticket.adjustment) } : {}),
    };
  });
  const config = convertLegacyKg
    ? { ...initial.config, ...saved.config, weight: kilogramsToTonnes(saved.config.weight), sourceWeightKg: saved.config.weight }
    : { ...initial.config, ...saved.config };
  const allIds = [...saved.tasks, ...saved.tickets, ...materials, ...parties].map((item) => Number(item.id.match(/(\d+)$/)?.[1] ?? 0));
  return { ...saved, version: 3, categories: saved.categories ?? [], materials, parties,
    tasks: migratedTasks, tickets: migratedTickets,
    draft: { ...emptyTaskInput(), ...saved.draft }, draftRequestId: saved.draftRequestId || `FIRST-${now}-${saved.sequence ?? 100}`,
    firstRequests: saved.firstRequests ?? {}, sequence: Math.max(saved.sequence ?? 100, ...allIds),
    config, owner: saved.owner ?? null, history: saved.history ?? [],
    sampleReady: false, capture: undefined,
  };
}

function weightDeductionValues(original: WeightValues, after: WeightValues, reason?: string) {
  return {
    grossDeduction: Math.round((original.gross - after.gross) * 1000) / 1000,
    tareDeduction: Math.round((original.tare - after.tare) * 1000) / 1000,
    ...(reason ? { reason } : {}),
  };
}
