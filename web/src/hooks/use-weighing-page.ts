import { useCallback, useEffect, useMemo, useState } from 'react';
import { http } from '@/lib/http';
import type { ApiPageResult, ApiQueryParams } from '@/types/api';
import type { WeighingFilter } from '@/types/weighing';

export interface WeighingSummary { taskCount: number; gross: string; tare: string; net: string; rows: unknown[] }
export function weighingQuery(kind: string, filter: WeighingFilter): ApiQueryParams {
  return { ...filter, status: kind === 'tasks' && filter.status === 'second' ? 'awaiting_second' : filter.status };
}
export function useWeighingPage<T>(kind: string, filter: WeighingFilter, page: number, pageSize: number) {
  const [result, setResult] = useState<ApiPageResult<T>>({ records: [], total: 0, page: 1, pageSize });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  const query = useMemo(() => ({ ...weighingQuery(kind, filter), page, pageSize }), [kind, filter, page, pageSize]);
  const refresh = useCallback(() => setRevision(value => value + 1), []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError('');
    void http.get<ApiPageResult<T>>(`/api/weighing/${kind}`, { query, signal: controller.signal }).then(setResult).catch(failure => {
      if (!controller.signal.aborted) setError(failure instanceof Error ? failure.message : '服务端查询失败');
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [kind, query, revision]);
  return { ...result, loading, error, refresh, revision };
}
export function useWeighingSummary(kind: 'originals' | 'tickets', filter: WeighingFilter, revision = 0) {
  const [result, setResult] = useState<WeighingSummary | null>(null);
  const [error, setError] = useState('');
  const query = useMemo(() => weighingQuery(kind, filter), [kind, filter]);
  useEffect(() => {
    const controller = new AbortController(); setResult(null); setError('');
    void http.get<WeighingSummary>(`/api/weighing/${kind}/summary`, { query, signal: controller.signal }).then(setResult).catch(failure => { if (!controller.signal.aborted) setError(failure instanceof Error ? failure.message : '汇总查询失败'); });
    return () => controller.abort();
  }, [kind, query, revision]);
  return { result, error };
}
export async function downloadWeighingCsv(kind: string, filter: WeighingFilter) {
  const path = kind.endsWith('-summary') ? `${kind.replace('-summary', '')}/summary/export` : `${kind}/export`;
  const blob = await http.blob(`/api/weighing/${path}`, { query: weighingQuery(kind.split('-')[0], filter) });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a'); anchor.href = url; anchor.download = `${kind}.csv`; anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
