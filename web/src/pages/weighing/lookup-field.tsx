import { useCallback } from 'react';
import { RecordPicker } from '@/components/common/record-picker';

export interface LookupOption { id: string; name: string; code: string; details: string[]; tags: string[]; disabled?: boolean }
interface LocalRecord { id: number; source: LookupOption }
const describe = (record: LocalRecord) => record.source;
const warning = (record: LocalRecord) => record.source.disabled ? '资料已停用，原业务快照保留；请选择启用资料。' : '';

// 使用服务端稳定 ID；每次候选查询及选中核对都读取最新资料。
export function WeighingLookup({ label, value, options, loadOptions, onChange, showDetails = true }: { label: string; value: string; options: LookupOption[]; loadOptions: () => Promise<LookupOption[]>; onChange: (id: string) => void; showDetails?: boolean }) {
  const loadPage = useCallback(async (keyword: string, page: number, size: number) => {
    const records = (await loadOptions()).map(source => ({ id: Number(source.id), source }));
    const words = keyword.toLocaleLowerCase().split(/\s+/).filter(Boolean);
    const filtered = records.filter(r => words.every(word => [r.source.name, r.source.code, ...r.source.details, ...r.source.tags].join(' ').toLocaleLowerCase().includes(word)));
    return { records: filtered.slice((page - 1) * size, page * size), total: filtered.length, page, pageSize: size };
  }, [loadOptions]);
  const loadRecord = useCallback(async (id: number) => {
    const source = (await loadOptions()).find(option => Number(option.id) === id);
    const record = source ? { id, source } : undefined;
    if (!record) throw new Error('资料已更新，请重新选择');
    return record;
  }, [loadOptions]);
  const selected = options.find(option => option.id === value);
  return <div className="space-y-1"><RecordPicker label={label} value={value ? Number(value) : 0} fallback={options.find(o => o.id === value)?.name} placeholder="输入名称、简称或编码搜索" loadPage={loadPage} loadRecord={loadRecord} describe={describe} warning={warning} onChange={record => onChange(record?.source.id ?? '')} />{showDetails && selected && <p className="text-sm text-text-secondary" aria-label={`${label}已选资料`}>{[selected.code, ...selected.details.filter(detail => detail !== selected.code), ...selected.tags].filter(Boolean).join(' · ')}</p>}</div>;
}
