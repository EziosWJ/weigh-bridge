import type { TestTicket, TransportTask, WeighingFilter, WeightValues } from '../../types/weighing';
import { directionLabels, taskStatusLabels } from '../../types/weighing';
import { originalWeights } from './weights';
import { formatWeightValue } from './weights';

export interface OriginalWeighingRow extends TransportTask { original: WeightValues; confirmedAt: string; validTicketId?: string }

export function filterOriginalWeighings(tasks: TransportTask[], tickets: TestTicket[], filter: WeighingFilter): OriginalWeighingRow[] {
  return tasks.flatMap(task => {
    const second = [...task.records].reverse().find(record => record.stage === 'second');
    const original = originalWeights(task.records);
    if (!second || !original || !matches(task, filter, beijingDate(second.at))) return [];
    return [{ ...task, original, confirmedAt: second.at, validTicketId: tickets.find(ticket => ticket.taskId === task.id && ticket.status === 'valid')?.id }];
  });
}

/** 演示日期始终按北京时间，与浏览器本地时区无关。 */
export function beijingDate(value: string): string {
  return new Date(new Date(value).getTime() + 8 * 3600_000).toISOString().slice(0, 10);
}
export function beijingTime(value: string): string {
  return new Date(new Date(value).getTime() + 8 * 3600_000).toISOString().slice(0, 19).replace('T', ' ');
}
function matches(row: TransportTask | TestTicket, filter: WeighingFilter, date: string): boolean {
  const keyword = filter.keyword?.trim().toUpperCase();
  return (!keyword || `${row.id} ${row.plate} ${'taskId' in row ? row.taskId : ''}`.toUpperCase().includes(keyword))
    && (!filter.direction || row.direction === filter.direction)
    && (!filter.materialId || row.materialId === filter.materialId)
    && (!filter.partyId || row.partyId === filter.partyId)
    && (!filter.status || row.status === filter.status)
    && (!filter.from || date >= filter.from)
    && (!filter.to || date <= filter.to);
}
export function filterTasks(tasks: TransportTask[], filter: WeighingFilter): TransportTask[] {
  return tasks.filter(row => matches(row, filter, beijingDate(row.createdAt)));
}
export function filterTickets(tickets: TestTicket[], filter: WeighingFilter): TestTicket[] {
  return tickets.filter(row => matches(row, filter, beijingDate(row.issuedAt)));
}
function csv(rows: (string | number)[][]): string {
  return '\uFEFF' + rows.map(row => row.map(cell => `"${String(cell).replace(/"/g, '""')}"`).join(',')).join('\r\n');
}
export function exportTasksCsv(tasks: TransportTask[]): string {
  return csv([
    ['性质', '任务号', '车牌', '方向', '物料', '供收方', '状态', '创建时间（北京时间）', '首次重量', '二次重量', '单位', '物料编码', '规格／等级', '供收方编码', '承运方', '承运方编码', '本趟联系人', '联系电话', '司机', '司机电话', '备注'],
    ...tasks.map(t => ['测试', t.id, t.plate, directionLabels[t.direction], t.materialName, t.partyName, taskStatusLabels[t.status], beijingTime(t.createdAt), formatWeightValueOrBlank([...t.records].reverse().find(r => r.stage === 'first')?.weight), formatWeightValueOrBlank([...t.records].reverse().find(r => r.stage === 'second')?.weight), '吨', t.materialCode ?? '', t.materialSpec ?? '', t.partyCode ?? '', t.carrierName ?? '', t.carrierCode ?? '', t.contactName ?? '', t.contactPhone ?? '', t.driver, t.driverPhone ?? '', t.remark]),
  ]);
}
export function exportTicketsCsv(tickets: TestTicket[]): string {
  return csv([
    ['性质', '测试单号', '任务号', '关联原单', '车牌', '方向', '物料', '供收方', '状态', '最终毛重', '最终皮重', '最终净重', '单位', '出单时间（北京时间）', '补打次数', '物料编码', '规格／等级', '供收方编码', '承运方', '承运方编码', '本趟联系人', '联系电话', '司机', '司机电话', '备注'],
    ...tickets.map(t => ['测试', t.id, t.taskId, t.previousId ?? '', t.plate, directionLabels[t.direction], t.materialName, t.partyName, t.status === 'valid' ? '有效' : '作废', formatWeightValue(t.gross), formatWeightValue(t.tare), formatWeightValue(t.net), '吨', beijingTime(t.issuedAt), t.reprintCount, t.materialCode ?? '', t.materialSpec ?? '', t.partyCode ?? '', t.carrierName ?? '', t.carrierCode ?? '', t.contactName ?? '', t.contactPhone ?? '', t.driver, t.driverPhone ?? '', t.remark]),
  ]);
}
type SummaryRow = Pick<TestTicket, 'direction' | 'materialId' | 'partyId' | 'materialName' | 'partyName' | 'materialSpec' | 'materialCode' | 'net'>;
function summaryCsv(rows: SummaryRow[], label: string): string {
  const groups = new Map<string, { direction: string; material: string; party: string; spec: string; code: string; count: number; net: number }>();
  for (const t of rows) {
    const key = JSON.stringify([t.direction, t.materialId, t.partyId]);
    const group = groups.get(key) ?? { direction: directionLabels[t.direction], material: t.materialName, party: t.partyName, spec: t.materialSpec ?? '', code: t.materialCode ?? '', count: 0, net: 0 };
    group.count++; group.net += t.net;
    groups.set(key, group);
  }
  return csv([['性质', '方向', '物料（历史名称）', '物料编码', '规格／等级', '供收方（历史名称）', '有效业务数', `${label}净重合计`, '单位'], ...[...groups.values()].map(g => ['测试汇总', g.direction, g.material, g.code, g.spec, g.party, g.count, formatWeightValue(g.net), '吨'])]);
}
export function exportSummaryCsv(tickets: TestTicket[]): string {
  return summaryCsv(tickets.filter(ticket => ticket.status === 'valid'), '最终');
}
export function exportOriginalSummaryCsv(rows: OriginalWeighingRow[]): string {
  return summaryCsv(rows.filter(row => row.validTicketId).map(row => ({ ...row, net: row.original.net })), '原始');
}
export function exportOriginalWeighingsCsv(rows: OriginalWeighingRow[]): string {
  return csv([
    ['性质', '任务号', '车牌', '方向', '物料', '物料编码', '规格／等级', '供收方', '原始毛重', '原始皮重', '原始净重', '单位', '二次称重确认时间（北京时间）', '当前有效测试单'],
    ...rows.map(row => ['测试', row.id, row.plate, directionLabels[row.direction], row.materialName, row.materialCode ?? '', row.materialSpec ?? '', row.partyName, formatWeightValue(row.original.gross), formatWeightValue(row.original.tare), formatWeightValue(row.original.net), '吨', beijingTime(row.confirmedAt), row.validTicketId ?? '无有效单据']),
  ]);
}
function formatWeightValueOrBlank(value: number | undefined): string { return value === undefined ? '' : formatWeightValue(value); }
export function downloadCsv(filename: string, content: string): void {
  const url = URL.createObjectURL(new Blob([content], { type: 'text/csv;charset=utf-8' }));
  const link = document.createElement('a'); link.href = url; link.download = filename;
  document.body.appendChild(link); link.click(); link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
