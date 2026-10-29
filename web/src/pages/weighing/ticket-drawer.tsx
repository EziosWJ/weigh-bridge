import { useDeferredValue, useState } from 'react';
import { CalendarDays, FileText, RefreshCw, RotateCcw, Search, X } from 'lucide-react';
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
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { netAdjustmentText } from '@/lib/weighing/weights';
import { useWeighingPage, useWeighingSummary } from '@/hooks/use-weighing-page';
import { beijingDate } from '@/lib/weighing/query';
import { useWeighingStore } from '@/store/weighing-store';
import type { DataTableColumn } from '@/types';
import { directionLabels, printStatusLabels, type WeighingFilter } from '@/types/weighing';
import { ListDate } from './shared';

const linkClass = 'text-primary hover:underline';

const ticketColumns: DataTableColumn<WireTicket>[] = [
  {
    title: '出单时间 ↓',
    key: 'issuedAt',
    width: 136,
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
    render: (_, row) => <div className="tabular-nums">{row.finalGross} / {row.finalTare} / <strong>{row.finalNet}</strong>{Number(row.netAdjustment ?? 0) !== 0 && <div className="text-text-tertiary">净重调整 {netAdjustmentText(Number(row.netAdjustment))}</div>}</div>,
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
  siteName?: string;
  onClose: () => void;
};

export function TicketDrawer({ siteName, onClose }: TicketDrawerProps) {
  const { materials, parties } = useWeighingStore(state => state.data);
  const [filter, setFilter] = useState<WeighingFilter>({});
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
  const hasFilters = Boolean(filter.keyword?.trim() || filter.status || filter.direction || filter.materialId || filter.partyId || filter.from || filter.to);

  const changeFilter = (next: WeighingFilter) => {
    setFilter(next);
    setPage(1);
  };
  const updateFilter = (patch: Partial<WeighingFilter>) => changeFilter({ ...filter, ...patch });
  const setDateRange = (from?: string, to?: string) => updateFilter({ from, to });

  return <Dialog open onOpenChange={next => { if (!next) onClose(); }} trapFocus restoreFocus lockScroll>
    <DialogOverlay />
    <DialogContent className="ticket-drawer">
      <DialogHeader className="ticket-drawer-header">
        <DialogTitle className="flex items-center gap-2 text-lg"><FileText className="h-4 w-4" aria-hidden />过磅单列表</DialogTitle>
        <DialogClose aria-label="关闭过磅单列表"><X className="h-4 w-4" aria-hidden /></DialogClose>
      </DialogHeader>

      <div className="ticket-drawer-body">
        <section className="ticket-drawer-filter-panel" aria-label="过磅单筛选">
          <div className="ticket-drawer-filter-grid">
            <div className="ticket-drawer-search">
              <Search className="h-4 w-4" aria-hidden />
              <Input aria-label="车牌或编号" placeholder="车牌 / 单号 / 任务号" value={filter.keyword ?? ''} onChange={event => updateFilter({ keyword: event.target.value })} />
            </div>
            <Select aria-label="筛选状态" value={filter.status ?? ''} onChange={event => updateFilter({ status: event.target.value })}>
              <option value="">全部状态</option>
              <option value="valid">有效</option>
              <option value="void">作废</option>
            </Select>
            <Select aria-label="筛选方向" value={filter.direction ?? ''} onChange={event => updateFilter({ direction: event.target.value })}>
              <option value="">全部方向</option>
              <option value="inbound">入厂卸货</option>
              <option value="outbound">出厂装货</option>
            </Select>
            <Select aria-label="筛选物料" value={filter.materialId ?? ''} onChange={event => updateFilter({ materialId: event.target.value })}>
              <option value="">全部物料</option>
              {materials.map(material => <option key={material.id} value={material.id}>{material.name} {material.spec}</option>)}
            </Select>
            <Select aria-label="筛选供收方" value={filter.partyId ?? ''} onChange={event => updateFilter({ partyId: event.target.value })}>
              <option value="">全部供收方</option>
              {parties.map(party => <option key={party.id} value={party.id}>{party.name}</option>)}
            </Select>

            <div className="ticket-drawer-datebar">
              <span className="ticket-drawer-date-title"><CalendarDays className="h-4 w-4" aria-hidden />出单日期</span>
              <div className="ticket-drawer-date-presets" role="group" aria-label="出单日期快捷筛选">
                <Button size="sm" variant={datePreset === 'today' ? 'primary' : 'secondary'} aria-pressed={datePreset === 'today'} onClick={() => setDateRange(today, today)}>今天</Button>
                <Button size="sm" variant={datePreset === 'week' ? 'primary' : 'secondary'} aria-pressed={datePreset === 'week'} onClick={() => setDateRange(weekAgo, today)}>近 7 天</Button>
                <Button size="sm" variant={datePreset === 'all' ? 'primary' : 'secondary'} aria-pressed={datePreset === 'all'} onClick={() => setDateRange(undefined, undefined)}>全部</Button>
              </div>
              <div className="ticket-drawer-date-range">
                <Input type="date" aria-label="出单开始日期" value={filter.from ?? ''} onChange={event => updateFilter({ from: event.target.value || undefined })} />
                <span className="ticket-drawer-date-sep" aria-hidden>至</span>
                <Input type="date" aria-label="出单结束日期" value={filter.to ?? ''} onChange={event => updateFilter({ to: event.target.value || undefined })} />
              </div>
            </div>
          </div>
        </section>

        <div className="ticket-drawer-listbar">
          <div className="ticket-drawer-summary">
            {siteName && <span className="ticket-drawer-scope">当前厂区 · {siteName}</span>}
            <span>共 <strong>{result.total}</strong> 张 · 有效 <strong>{summary.result?.taskCount ?? '—'}</strong> 张 · 最终净重 <strong>{summary.result?.net ?? '—'}</strong> 吨</span>
          </div>
          <div className="ticket-drawer-list-actions">
            <Button size="sm" variant="ghost" disabled={!hasFilters} onClick={() => changeFilter({})}><RotateCcw className="h-4 w-4" aria-hidden />清空筛选</Button>
            <Button size="sm" variant="secondary" disabled={result.loading} onClick={result.refresh}>
              <RefreshCw className={result.loading ? 'h-4 w-4 motion-safe:animate-spin' : 'h-4 w-4'} aria-hidden />刷新
            </Button>
          </div>
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
          minWidth={1176}
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
