import { useDeferredValue, useState } from 'react';
import { FileText, RefreshCw, X } from 'lucide-react';
import type { WireTicket } from '@/api/weighing-workflow';
import { DataTable } from '@/components/common/data-table';
import { EmptyState } from '@/components/common/empty-state';
import { Pagination } from '@/components/common/pagination';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from '@/components/ui/dialog';
import { useWeighingPage, useWeighingSummary } from '@/hooks/use-weighing-page';
import { beijingDate } from '@/lib/weighing/query';
import type { DataTableColumn } from '@/types';
import { directionLabels, printStatusLabels, type WeighingFilter } from '@/types/weighing';
import { ListDate, WeighingFilters } from './shared';

const linkClass = 'text-primary hover:underline';

const ticketColumns: DataTableColumn<WireTicket>[] = [
  {
    title: '出单时间',
    key: 'issuedAt',
    width: 128,
    render: (_, row) => <ListDate value={row.issuedAt} />,
  },
  {
    title: '单号 / 车牌',
    key: 'ticket',
    width: 190,
    render: (_, row) => <div>
      <a className={linkClass} href={`/weighing/tickets/${encodeURIComponent(String(row.id))}`} target="weighbridge-management" rel="noreferrer">{row.ticketNo}</a>
      <div className="mt-1 font-medium">{row.business.plate}</div>
    </div>,
  },
  {
    title: '方向 / 物料',
    key: 'business',
    width: 230,
    render: (_, row) => <div>
      {directionLabels[row.business.direction]} · {row.business.materialName}
      <div className="text-text-tertiary">{row.business.materialSpec || '—'} · {row.business.materialCode || '—'}</div>
    </div>,
  },
  {
    title: '供收方',
    key: 'party',
    width: 170,
    render: (_, row) => row.business.partyName,
  },
  {
    title: '最终毛 / 皮 / 净重（吨）',
    key: 'weight',
    width: 210,
    render: (_, row) => <span className="tabular-nums">{row.finalGross} / {row.finalTare} / <strong>{row.finalNet}</strong></span>,
  },
  {
    title: '状态 / 打印',
    key: 'status',
    width: 170,
    render: (_, row) => <div>
      <StatusTag tone={row.status === 'valid' ? 'success' : 'neutral'}>{row.status === 'valid' ? '有效' : '作废'}</StatusTag>
      <div className="mt-1 text-text-tertiary">{printStatusLabels[row.printStatus ?? 'unprinted']} · 补打 {row.reprintCount ?? 0} 次</div>
    </div>,
  },
  {
    title: '操作',
    key: 'actions',
    width: 70,
    nowrap: true,
    render: (_, row) => <a className={linkClass} href={`/weighing/tickets/${encodeURIComponent(String(row.id))}`} target="weighbridge-management" rel="noreferrer" aria-label={`查看单据 ${row.ticketNo}`}>详情</a>,
  },
];

type TicketDrawerProps = {
  siteId: number;
  siteName?: string;
  onClose: () => void;
};

export function TicketDrawer({ siteId, siteName, onClose }: TicketDrawerProps) {
  const [filter, setFilter] = useState<WeighingFilter>({ siteId: String(siteId) });
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const deferred = useDeferredValue(filter);
  const result = useWeighingPage<WireTicket>('tickets', deferred, page, pageSize);
  const summary = useWeighingSummary('tickets', deferred, result.revision);
  const today = beijingDate(new Date().toISOString());
  const weekAgo = beijingDate(new Date(Date.now() - 6 * 24 * 60 * 60 * 1000).toISOString());
  const datePreset = filter.from === today && filter.to === today
    ? 'today'
    : filter.from === weekAgo && filter.to === today
      ? 'week'
      : !filter.from && !filter.to
        ? 'all'
        : 'custom';

  const changeFilter = (next: WeighingFilter) => {
    setFilter({ ...next, siteId: String(siteId) });
    setPage(1);
  };
  const setDateRange = (from?: string, to?: string) => {
    changeFilter({ ...filter, from, to });
  };

  return <Dialog open onOpenChange={next => { if (!next) onClose(); }} trapFocus restoreFocus lockScroll>
    <DialogOverlay />
    <DialogContent className="ticket-drawer">
      <DialogHeader className="ticket-drawer-header">
        <div className="min-w-0">
          <DialogTitle className="flex items-center gap-2"><FileText className="h-4 w-4" aria-hidden />过磅单列表</DialogTitle>
          <DialogDescription>按出单时间倒序 · 当前厂区{siteName ? `：${siteName}` : ''}</DialogDescription>
        </div>
        <DialogClose aria-label="关闭过磅单列表"><X className="h-4 w-4" aria-hidden /></DialogClose>
      </DialogHeader>

      <div className="ticket-drawer-body">
        <div className="ticket-drawer-presets" aria-label="出单日期快捷筛选">
          <span>出单日期</span>
          <Button size="sm" variant={datePreset === 'today' ? 'primary' : 'secondary'} aria-pressed={datePreset === 'today'} onClick={() => setDateRange(today, today)}>今天</Button>
          <Button size="sm" variant={datePreset === 'week' ? 'primary' : 'secondary'} aria-pressed={datePreset === 'week'} onClick={() => setDateRange(weekAgo, today)}>近 7 天</Button>
          <Button size="sm" variant={datePreset === 'all' ? 'primary' : 'secondary'} aria-pressed={datePreset === 'all'} onClick={() => setDateRange(undefined, undefined)}>全部日期</Button>
        </div>

        <WeighingFilters className="ticket-drawer-filters" value={filter} onChange={changeFilter} ticket dateLabel="出单" />

        <div className="ticket-drawer-summary">
          <div>
            共 <strong>{result.total}</strong> 张 · 有效 <strong>{summary.result?.taskCount ?? '—'}</strong> 张 · 最终净重 <strong>{summary.result?.net ?? '—'}</strong> 吨
          </div>
          <Button size="sm" variant="secondary" disabled={result.loading} onClick={result.refresh}>
            <RefreshCw className="h-4 w-4" aria-hidden />刷新
          </Button>
        </div>
        {summary.error && <p role="alert" className="text-sm text-error">{summary.error}</p>}

        <DataTable
          compact
          stickyHeader
          fixedEdges
          ariaLabel="过磅单列表"
          className="ticket-drawer-table"
          columns={ticketColumns}
          dataSource={result.records}
          loading={result.loading}
          error={result.error}
          rowKey="id"
          minWidth={1168}
          empty={<EmptyState title="暂无过磅单" description="当前筛选条件下没有符合条件的过磅单。" />}
        />

        <Pagination
          className="ticket-drawer-pagination"
          page={page}
          pageSize={pageSize}
          total={result.total}
          pageSizeOptions={[10, 20, 50]}
          onPageChange={setPage}
          onPageSizeChange={size => { setPageSize(size); setPage(1); }}
        />
      </div>
    </DialogContent>
  </Dialog>;
}
