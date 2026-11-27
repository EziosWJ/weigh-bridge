import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { PageHeader } from '@/components/common/page-header';
import { DataTable } from '@/components/common/data-table';
import { DataTableCard } from '@/components/common/data-table-card';
import { Pagination } from '@/components/common/pagination';
import { SearchFilterBar } from '@/components/common/search-filter-bar';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { mobileHandoffApi, type WireMobileHandoff } from '@/api/weighing-workflow';
import { beijingTime } from '@/lib/weighing/query';
import type { DataTableColumn } from '@/types';

const actionLabels = { first: '首磅', second: '回磅' } as const;

function statusTone(status: WireMobileHandoff['status']) {
  if (status === 'completed') return 'success' as const;
  if (status === 'failed') return 'error' as const;
  return 'warning' as const;
}

function statusLabel(status: WireMobileHandoff['status']) {
  if (status === 'completed') return '已完成';
  if (status === 'failed') return '失败';
  return '处理中';
}

function weight(value?: string) {
  return value || '—';
}

function sourceLabel(source?: string) {
  if (source === 'serial_agent') return '串口扫码枪';
  if (source === 'operator') return '手工扫码';
  return source || '—';
}

function resultText(item: WireMobileHandoff) {
  if (item.result?.ticketNo) return item.result.ticketNo;
  if (item.result?.taskNo) return `任务 ${item.result.taskNo}`;
  if (item.error) return item.error;
  return '—';
}

const columns: DataTableColumn<WireMobileHandoff>[] = [
  { title: '时间', key: 'time', width: 154, nowrap: true, render: (_, row) => beijingTime(row.completedAt || row.claimedAt) },
  { title: '动作', key: 'action', width: 76, render: (_, row) => actionLabels[row.action] || row.action },
  { title: '车牌', key: 'plate', width: 108, render: (_, row) => <span className="font-medium">{row.plate}</span> },
  { title: '手机账号', key: 'mobileAccount', width: 120, render: (_, row) => row.mobileAccount || '—' },
  {
    title: '来源', key: 'source', width: 136,
    render: (_, row) => <div>{sourceLabel(row.source)}{row.device ? <div className="text-xs text-text-tertiary">{row.device}</div> : null}</div>,
  },
  { title: '状态', key: 'status', width: 92, render: (_, row) => <StatusTag tone={statusTone(row.status)}>{statusLabel(row.status)}</StatusTag> },
  {
    title: '任务 / 磅单', key: 'result', width: 202,
    render: (_, row) => row.ticketId
      ? <Link className="text-primary hover:underline" to={`/weighing/tickets/${row.ticketId}`}>{row.result?.ticketNo || '查看磅单'}</Link>
      : row.taskId
        ? <Link className="text-primary hover:underline" to={`/weighing/tasks/${row.taskId}`}>{row.result?.taskNo || '查看任务'}</Link>
        : resultText(row),
  },
  { title: '首磅 / 回磅 / 净重（吨）', key: 'weights', width: 194, nowrap: true, render: (_, row) => `${weight(row.result?.firstWeight)} / ${weight(row.result?.secondWeight)} / ${weight(row.result?.net)}` },
  { title: '抓拍', key: 'capture', width: 90, render: (_, row) => row.result?.captureStatus || '—' },
];

type HandoffFilter = {
  keyword?: string;
  action?: string;
  status?: string;
  source?: string;
  from?: string;
  to?: string;
};

export function MobileHandoffsPage() {
  const [items, setItems] = useState<WireMobileHandoff[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [draft, setDraft] = useState<HandoffFilter>({});
  const [filter, setFilter] = useState<HandoffFilter>({});

  const load = useCallback(async (showLoading: boolean) => {
    if (showLoading) setLoading(true);
    try {
      const result = await mobileHandoffApi.list({ page, pageSize, ...filter });
      setItems(result.records);
      setTotal(result.total);
      setError('');
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : '扫码记录加载失败');
    } finally {
      if (showLoading) setLoading(false);
    }
  }, [page, pageSize, filter]);

  useEffect(() => {
    void load(true);
    const timer = window.setInterval(() => void load(false), 3000);
    return () => window.clearInterval(timer);
  }, [load]);

  const applyFilter = () => {
    setFilter({ ...draft });
    setPage(1);
  };
  const clearFilter = () => {
    setDraft({});
    setFilter({});
    setPage(1);
  };

  return (
    <div className="space-y-space-5">
      <PageHeader title="手机扫码" />
      <SearchFilterBar
        compact
        actions={
          <>
            <Button size="sm" onClick={applyFilter}>查询</Button>
            <Button size="sm" variant="secondary" onClick={clearFilter}>清空筛选</Button>
          </>
        }
      >
        <Input
          aria-label="车牌、手机账号、任务号或磅单号"
          placeholder="车牌 / 手机账号 / 任务号 / 磅单号"
          value={draft.keyword ?? ''}
          onChange={event => setDraft({ ...draft, keyword: event.target.value })}
          onKeyDown={event => { if (event.key === 'Enter') applyFilter(); }}
        />
        <Select aria-label="筛选动作" value={draft.action ?? ''} onChange={event => setDraft({ ...draft, action: event.target.value })}>
          <option value="">全部动作</option>
          <option value="first">首磅</option>
          <option value="second">回磅</option>
        </Select>
        <Select aria-label="筛选状态" value={draft.status ?? ''} onChange={event => setDraft({ ...draft, status: event.target.value })}>
          <option value="">全部状态</option>
          <option value="claimed">处理中</option>
          <option value="completed">已完成</option>
          <option value="failed">失败</option>
        </Select>
        <Select aria-label="筛选来源" value={draft.source ?? ''} onChange={event => setDraft({ ...draft, source: event.target.value })}>
          <option value="">全部来源</option>
          <option value="serial_agent">串口扫码枪</option>
          <option value="operator">手工扫码</option>
        </Select>
        <label className="flex min-w-0 items-center gap-space-2 text-sm text-text-secondary">
          <span className="shrink-0">从</span>
          <Input className="min-w-0" type="date" aria-label="扫码开始日期" value={draft.from ?? ''} onChange={event => setDraft({ ...draft, from: event.target.value })} />
        </label>
        <label className="flex min-w-0 items-center gap-space-2 text-sm text-text-secondary">
          <span className="shrink-0">至</span>
          <Input className="min-w-0" type="date" aria-label="扫码结束日期" value={draft.to ?? ''} onChange={event => setDraft({ ...draft, to: event.target.value })} />
        </label>
      </SearchFilterBar>
      <DataTableCard>
        <DataTable
          compact
          stickyHeader
          fixedEdges
          ariaLabel="手机扫码记录"
          columns={columns}
          dataSource={items}
          loading={loading}
          error={error}
          rowKey="id"
          minWidth={1100}
        />
        <Pagination
          page={page}
          pageSize={pageSize}
          total={total}
          onPageChange={setPage}
          onPageSizeChange={size => { setPageSize(size); setPage(1); }}
        />
      </DataTableCard>
    </div>
  );
}
