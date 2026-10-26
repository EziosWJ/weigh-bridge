import { useEffect, useState } from 'react';
import { getErrorMessage } from '@/lib/api-error';

export interface OverviewResource<T> {
  data: T | null;
  loading: boolean;
  error: string;
  checkedAt: string;
}

// Independent reads: failures discard old results; late replies cannot replace a newer refresh.
export function useOverviewResource<T>(load: (() => Promise<T>) | null, revision: number): OverviewResource<T> {
  const [state, setState] = useState<OverviewResource<T>>({ data: null, loading: !!load, error: '', checkedAt: '' });
  useEffect(() => {
    let active = true;
    setState({ data: null, loading: !!load, error: '', checkedAt: '' });
    if (load) void load().then(data => {
      if (active) setState({ data, loading: false, error: '', checkedAt: new Date().toISOString() });
    }).catch(failure => {
      if (active) setState({ data: null, loading: false, error: getErrorMessage(failure, '读取失败，请刷新重试'), checkedAt: '' });
    });
    return () => { active = false; };
  }, [load, revision]);
  return state;
}
