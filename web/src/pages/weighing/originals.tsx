import { useTransportDirections } from '@/hooks/use-transport-directions';
import { useDeferredValue, useState } from 'react';
import { Link } from 'react-router-dom';
import { DataTable } from '@/components/common/data-table';
import { DataTableCard } from '@/components/common/data-table-card';
import { PageHeader } from '@/components/common/page-header';
import { Pagination } from '@/components/common/pagination';
import { StatusTag } from '@/components/common/status-tag';
import { TableActions } from '@/components/common/table-actions';
import { TableToolbar } from '@/components/common/table-toolbar';
import { toast } from '@/components/common/toast-store';
import { Button } from '@/components/ui/button';
import { useWeighingStore } from '@/store/weighing-store';
import { useWeighingPage, useWeighingSummary, downloadWeighingCsv } from '@/hooks/use-weighing-page';
import type { WireOriginal } from '@/api/weighing-workflow';
import type { DataTableColumn } from '@/types';
import { type WeighingFilter } from '@/types/weighing';
import { DeviceBanner, WeighingFilters, ListDate } from './shared';

export function WeighingOriginalsPage() {
  const directionLabels = useTransportDirections();
  const capabilities = useWeighingStore(state => state.capabilities);
  const [filter, setFilter] = useState<WeighingFilter>({});
  const deferred = useDeferredValue(filter);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const result = useWeighingPage<WireOriginal>('originals', deferred, page, pageSize);
  const summary = useWeighingSummary('originals', deferred, result.revision);
  const canExport = capabilities?.permissionCodes.includes('weighing:export');
  const download = (kind: string) => void downloadWeighingCsv(kind, filter).then(() => toast.success("原始称重 CSV 已导出")).catch(error => toast.error({ title: "导出失败", description: error.message }));
  const linkClass = 'text-primary hover:underline';
  const columns: DataTableColumn<WireOriginal>[] = [
    { title: '任务号 / 车牌', key: 'task', width: 220, render: (_, row) => <div><Link className={linkClass} to={`/weighing/tasks/${row.id}`}>{row.taskNo}</Link><div className="mt-1 font-medium">{row.business.plate}</div></div> },
    {title:"厂区",key:"site",width:100,render:(_,row)=>row.siteCode||"—"},
    { title: '方向 / 物料规格', key: 'business', width: 180, render: (_, row) => <div>{directionLabels[row.business.direction]} · {row.business.materialName}<div className="text-text-tertiary">{row.business.materialSpec || '—'} · {row.business.materialCode}</div></div> },
    { title: '供收方', key: 'party', width: 160, render: (_, row) => row.business.partyName },
    { title: '原始毛 / 皮 / 净重（吨）', key: 'weight', width: 220, render: (_, row) => <span className="tabular-nums">{row.originalGross} / {row.originalTare} / <strong>{row.originalNet}</strong></span> },
    { title: '二次确认时间', key: 'date', width: 144, render: (_, row) => <ListDate value={row.secondConfirmedAt} /> },
    { title: '关联单据', key: 'ticket', width: 144, render: (_, row) => row.currentTicketId ? <Link className={linkClass} to={`/weighing/tickets/${row.currentTicketId}`}>查看当前有效单</Link> : <StatusTag tone="neutral">无有效单据</StatusTag> },
    { title: '操作', key: 'actions', width: 148, nowrap: true, render: (_, row) => <TableActions><Link className={linkClass} to={`/weighing/tasks/${row.id}`}>原始依据与历史</Link></TableActions> },
  ];
  return <div className="weighing-list-page"><PageHeader title="原始称重" description="" /><DeviceBanner compact />
    <WeighingFilters value={filter} onChange={value => { setFilter(value); setPage(1); }} dateLabel="二次称重确认" hideStatus />
    {summary.error && <p role="alert" className="text-error">{summary.error}</p>}
    <DataTableCard className="weighing-list-table"><TableToolbar className="!py-space-3" title="原始称重记录" description={`筛选 ${result.total} 趟；关联有效单 ${summary.result?.taskCount ?? '—'} 趟，原始净重合计 ${summary.result?.net ?? '—'} 吨`} actions={<><Button size="sm" onClick={result.refresh}>刷新</Button>{canExport && <><Button size="sm" onClick={() => download('originals')}>导出明细 CSV</Button><Button size="sm" onClick={() => download('originals-summary')}>导出净重汇总 CSV</Button></>}</>} />
      <DataTable compact stickyHeader fixedEdges ariaLabel="原始称重列表" className="weighing-list-scroll" columns={columns} dataSource={result.records} loading={result.loading} error={result.error} rowKey="id" minWidth={1316} />
      <Pagination page={page} pageSize={pageSize} total={result.total} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    </DataTableCard></div>;
}
