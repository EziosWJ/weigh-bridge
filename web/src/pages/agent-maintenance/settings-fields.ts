import type { DeviceSettings } from '@/api/agent-maintenance';

export const fields: { label: string; group: 'serial' | 'print'; key: string }[] = [
  { label: '串口路径', group: 'serial', key: 'port' }, { label: '仪表波特率', group: 'serial', key: 'baud' },
  { label: 'CUPS 打印队列', group: 'print', key: 'queue' }, { label: '单据节长（mm）', group: 'print', key: 'formLengthMm' },
  { label: '水平偏移（mm）', group: 'print', key: 'offsetXmm' }, { label: '垂直偏移（mm）', group: 'print', key: 'offsetYmm' },
  { label: '内容缩放（倍）', group: 'print', key: 'printScale' }, { label: '基准打印宽度（mm）', group: 'print', key: 'printWidthMm' },
];
export function setting(settings: DeviceSettings | undefined, group: 'serial' | 'print', key: string) {
  const values = settings?.[group] as Record<string, string | number> | undefined;
  const value = values?.[key];
  return value === '' || value === undefined ? '—' : String(value);
}
