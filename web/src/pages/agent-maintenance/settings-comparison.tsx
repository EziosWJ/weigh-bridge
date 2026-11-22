import type { DeviceSettings } from '@/api/agent-maintenance';

import { fields, setting } from './settings-fields';

export function SettingsComparison({ current, next, currentLabel = '当前生效值', nextLabel, changesOnly = false }: {
  current?: DeviceSettings;
  next: DeviceSettings;
  currentLabel?: string;
  nextLabel: string;
  changesOnly?: boolean;
}) {
  const visibleFields = changesOnly
    ? fields.filter(field => setting(current, field.group, field.key) !== setting(next, field.group, field.key))
    : fields;
  if (!visibleFields.length) return <p className="text-sm text-text-secondary">参数相同。</p>;
  return <div className="overflow-x-auto rounded-control border border-border"><table className="w-full min-w-[300px] table-fixed text-left text-sm">
    <caption className="sr-only">设备参数对比</caption>
    <thead className="bg-neutral-background text-text-secondary"><tr className="border-b border-border">{['参数', currentLabel, nextLabel].map(label => <th key={label} scope="col" className="px-space-3 py-space-2 font-medium">{label}</th>)}</tr></thead>
    <tbody>{visibleFields.map(field => {
      const before = setting(current, field.group, field.key);
      const saved = setting(next, field.group, field.key);
      return <tr key={field.key} className="border-b border-border last:border-b-0"><th scope="row" className="px-space-3 py-space-2 font-medium">{field.label}</th><td className="px-space-3 py-space-2 tabular-nums [overflow-wrap:anywhere]">{before}</td><td className={`px-space-3 py-space-2 tabular-nums [overflow-wrap:anywhere] ${before !== saved ? 'bg-warning-background font-medium text-warning' : ''}`}>{saved}{before !== saved && <span className="ml-space-2 text-xs">（不同）</span>}</td></tr>;
    })}</tbody>
  </table></div>;
}
