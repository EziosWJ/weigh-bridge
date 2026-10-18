import { useDeferredValue, useState } from 'react';
import { Link } from 'react-router-dom';
import { DataTable } from '@/components/common/data-table';
import { DataTableCard } from '@/components/common/data-table-card';
import { PageHeader } from '@/components/common/page-header';
import { Pagination } from '@/components/common/pagination';
import { StatusTag } from '@/components/common/status-tag';
import { TableActions } from '@/components/common/table-actions';
import { TableToolbar } from '@/components/common/table-toolbar';
import { Button } from '@/components/ui/button';
import { beijingTime, downloadCsv, exportOriginalSummaryCsv, exportOriginalWeighingsCsv, filterOriginalWeighings, type OriginalWeighingRow } from '@/lib/weighing/query';
import { useWeighingStore } from '@/store/weighing-store';
import type { DataTableColumn } from '@/types';
import { directionLabels, type WeighingFilter } from '@/types/weighing';
import { formatWeight, needsLegacyWeightReview } from '@/lib/weighing/weights';
import { DemoBanner, WeighingFilters } from './shared';

export function WeighingOriginalsPage() {
  const data = useWeighingStore(state => state.data);
  const [filter, setFilter] = useState<WeighingFilter>({});
  const deferredFilter = useDeferredValue(filter);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const rows = filterOriginalWeighings(data.tasks, data.tickets, deferredFilter);
  const valid = rows.filter(row => row.validTicketId);
  const currentPage = Math.min(page, Math.max(1, Math.ceil(rows.length / pageSize)));
  const linkClass = 'text-primary hover:underline';
  const columns: DataTableColumn<OriginalWeighingRow>[] = [
    { title: '任务号 / 车牌', key: 'task', width: 230, render: (_, row) => <div><Link className={linkClass} to={`/weighing/tasks/${encodeURIComponent(row.id)}`}>{row.id}</Link><div className="mt-1 font-medium">{row.plate}</div></div> },
    { title: '方向 / 物料规格', key: 'business', width: 210, render: (_, row) => <div>{directionLabels[row.direction]} · {row.materialName}<div className="text-text-tertiary">{row.materialSpec || '-'} · {row.materialCode || '-'}</div></div> },
    { title: '供收方', key: 'party', width: 180, render: (_, row) => row.partyName },
    { title: '原始毛 / 皮 / 净重（吨）', key: 'weight', width: 290, render: (_, row) => <span className="tabular-nums">{formatWeight(row.original.gross)} / {formatWeight(row.original.tare)} / <strong>{formatWeight(row.original.net)}</strong>{needsLegacyWeightReview(row.original) && <span className="mt-1 block text-warning">换算后需核对来源重量</span>}</span> },
    { title: '二次确认时间（北京）', key: 'date', width: 190, render: (_, row) => beijingTime(row.confirmedAt) },
    { title: '关联单据', key: 'status', width: 180, render: (_, row) => row.validTicketId ? <Link className={linkClass} to={`/weighing/tickets/${encodeURIComponent(row.validTicketId)}`}>{row.validTicketId}</Link> : <StatusTag tone="neutral">无有效单据</StatusTag> },
    { title: '操作', key: 'actions', width: 130, nowrap: true, render: (_, row) => <TableActions><Link className={linkClass} to={`/weighing/tasks/${encodeURIComponent(row.id)}`}>原始依据与历史</Link></TableActions> },
  ];
  return <>
    <PageHeader title="原始称重" description="每个完成两次称重的运输任务一行；按北京时间二次称重确认日期筛选。" />
    <DemoBanner />
    <WeighingFilters value={filter} onChange={value => { setFilter(value); setPage(1); }} dateLabel="二次称重确认" hideStatus />
    <DataTableCard>
      <TableToolbar title="原始称重记录" description={`筛选 ${rows.length} 趟，关联有效单 ${valid.length} 趟，原始净重合计 ${formatWeight(valid.reduce((sum, row) => sum + row.original.net, 0))}（仅供测试核对）。`} actions={data.config.role !== 'admin' && <><Button size="sm" onClick={() => downloadCsv('测试原始称重明细.csv', exportOriginalWeighingsCsv(rows))}>导出明细 CSV</Button><Button size="sm" onClick={() => downloadCsv('测试原始净重汇总.csv', exportOriginalSummaryCsv(rows))}>导出净重汇总 CSV</Button></>} />
      <DataTable columns={columns} dataSource={rows.slice((currentPage - 1) * pageSize, currentPage * pageSize)} rowKey="id" minWidth={1440} />
      <Pagination page={currentPage} pageSize={pageSize} total={rows.length} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    </DataTableCard>
  </>;
}
