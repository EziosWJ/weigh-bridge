import { useCallback, useMemo } from 'react';
import { RecordPicker } from '@/components/common/record-picker';

export interface LookupOption { id: string; name: string; code: string; details: string[]; tags: string[]; disabled?: boolean }
interface LocalRecord { id: number; source: LookupOption }
const describe = (record: LocalRecord) => record.source;
const warning = (record: LocalRecord) => record.source.disabled ? '资料已停用，原业务快照保留；请选择启用资料。' : '';

// 适配现有数字ID选择组件，业务资料仍使用原字符串ID；不发起网络请求。
export function WeighingLookup({ label, value, options, onChange }: { label: string; value: string; options: LookupOption[]; onChange: (id: string) => void }) {
  const records = useMemo(() => options.map((source, index) => ({ id: index + 1, source })), [options]);
  const loadPage = useCallback(async (keyword: string, page: number, size: number) => {
    const words = keyword.toLocaleLowerCase().split(/\s+/).filter(Boolean);
    const filtered = records.filter(r => words.every(word => [r.source.name, r.source.code, ...r.source.details, ...r.source.tags].join(' ').toLocaleLowerCase().includes(word)));
    return { records: filtered.slice((page - 1) * size, page * size), total: filtered.length, page, pageSize: size };
  }, [records]);
  const loadRecord = useCallback(async (id: number) => {
    const record = records.find(r => r.id === id);
    if (!record) throw new Error('资料已更新，请重新选择');
    return record;
  }, [records]);
  const selected = options.find(option => option.id === value);
  return <div className="space-y-1"><RecordPicker label={label} value={records.find(r => r.source.id === value)?.id ?? 0} fallback={options.find(o => o.id === value)?.name} placeholder="输入名称、简称或编码搜索" loadPage={loadPage} loadRecord={loadRecord} describe={describe} warning={warning} onChange={record => onChange(record?.source.id ?? '')} />{selected && <p className="text-sm text-text-secondary" aria-label={`${label}已选资料`}>{[selected.code, ...selected.details.filter(detail => detail !== selected.code), ...selected.tags].filter(Boolean).join(' · ')}</p>}</div>;
}
