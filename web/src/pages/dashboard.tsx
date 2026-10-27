import { useCallback, useState } from 'react';
import { Link } from 'react-router-dom';
import { RefreshCw } from 'lucide-react';
import { overviewApi, type BusinessOverview } from '@/api/overview';
import { weighingApi } from '@/api/weighing';
import { cameraApi } from '@/api/camera';
import { agentMaintenanceApi } from '@/api/agent-maintenance';
import { ContentCard } from '@/components/common/content-card';
import { DataTable } from '@/components/common/data-table';
import { EmptyState } from '@/components/common/empty-state';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { beijingTime } from '@/lib/weighing/query';
import { formatWeightValue } from '@/lib/weighing/weights';
import { directionLabels } from '@/types/weighing';
import type { DataTableColumn } from '@/types';
import { DeviceOverview } from './dashboard/device-overview';
import { useOverviewResource } from './dashboard/use-overview-resource';

const linkClass = 'text-sm text-primary hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary';

export function DashboardPage() {
  const [revision, setRevision] = useState(0);
  const caps = useOverviewResource(weighingApi.capabilities, revision);
  const permissions = caps.data?.permissionCodes ?? [];
  const canRead = permissions.includes('weighing:read');
  const canMaintain = caps.data?.maintenanceEnabled === true && permissions.some(code => ['agent:maintenance:view', 'agent:maintenance:manage'].includes(code));
  const canViewCamera = permissions.includes('weighing:camera:view');
  const business = useOverviewResource(canRead ? overviewApi.business : null, revision);
  const scale = caps.data?.scales[0];
  const scaleCode = scale?.code;
  const loadDevice = useCallback(() => scaleCode ? overviewApi.device(scaleCode) : Promise.reject(new Error('未找到可查询的本厂地磅')), [scaleCode]);
  const device = useOverviewResource(canRead ? loadDevice : null, revision);
  const printer = useOverviewResource(canMaintain ? agentMaintenanceApi.status : null, revision);
  const cameras = useOverviewResource(canViewCamera ? cameraApi.list : null, revision);
  const refresh = () => setRevision(value => value + 1);
  const busy = caps.loading || business.loading || device.loading || printer.loading || cameras.loading;
  const data = business.data;
  const date = data?.date;

  const taskColumns: DataTableColumn<BusinessOverview['awaitingTasks'][number]>[] = [
    { title: '车牌', key: 'plate', render: (_, row) => <Link className={linkClass} to={`/weighing/tasks/${row.id}`}>{row.business.plate}</Link> },
    { title: '方向', key: 'direction', render: (_, row) => directionLabels[row.business.direction] },
    { title: '物料', key: 'material', render: (_, row) => [row.business.materialName, row.business.materialSpec].filter(Boolean).join(' / ') },
    { title: '首次称重时间', key: 'first', width: 230, render: (_, row) => <div className="flex items-center gap-2 whitespace-nowrap"><span className="tabular-nums">{beijingTime(row.firstConfirmedAt)}</span>{date && beijingTime(row.firstConfirmedAt).slice(0, 10) < date && <span className="shrink-0"><StatusTag>隔日</StatusTag></span>}</div> },
    { title: '操作', key: 'action', render: (_, row) => <Link className={linkClass} to={`/weighing/tasks/${row.id}`}>查看任务</Link> },
  ];
  const ticketColumns: DataTableColumn<BusinessOverview['recentTickets'][number]>[] = [
    { title: '单号', key: 'ticketNo', render: (_, row) => <Link className={linkClass} to={`/weighing/tickets/${row.id}`}>{row.ticketNo}</Link> },
    { title: '车牌', key: 'plate', render: (_, row) => row.business.plate },
    { title: '方向', key: 'direction', render: (_, row) => directionLabels[row.business.direction] },
    { title: '物料', key: 'material', render: (_, row) => [row.business.materialName, row.business.materialSpec].filter(Boolean).join(' / ') },
    { title: '最终净重（吨）', key: 'net', align: 'right', render: (_, row) => <span className="tabular-nums">{formatWeightValue(Number(row.finalNet))}</span> },
    { title: '出单时间', key: 'issuedAt', render: (_, row) => <span className="tabular-nums">{beijingTime(row.issuedAt)}</span> },
    { title: '操作', key: 'action', render: (_, row) => <Link className={linkClass} to={`/weighing/tickets/${row.id}`}>查看单据</Link> },
  ];
  const cards = [
    { label: '今日有效单', value: data?.validTickets, unit: '张', detail: '今日出单且当前有效' },
    { label: '今日入厂卸货净重', value: data ? formatWeightValue(Number(data.inbound.net)) : undefined, unit: '吨', detail: data ? `${data.inbound.taskCount} 趟` : '— 趟' },
    { label: '今日出厂装货净重', value: data ? formatWeightValue(Number(data.outbound.net)) : undefined, unit: '吨', detail: data ? `${data.outbound.taskCount} 趟` : '— 趟' },
    { label: '当前待回磅', value: data?.awaitingCount, unit: '趟', detail: data ? `其中隔日 ${data.overnightCount} 趟` : '其中隔日 — 趟' },
  ];

  return <>
    <h1 className="sr-only">系统概览</h1>
    <div className="mb-space-4 flex justify-end"><Button onClick={refresh} disabled={busy}><RefreshCw className="mr-2 h-4 w-4" aria-hidden />{busy ? '刷新中…' : '刷新'}</Button></div>
    {caps.error && <ContentCard><p role="alert" className="text-error">权限与本厂信息读取失败：{caps.error}</p></ContentCard>}
    {caps.loading && !caps.data && <p role="status" className="text-text-tertiary">正在读取本厂信息…</p>}
    {caps.data && <div className="space-y-space-6">
      {canRead && <>
        <div className="grid gap-space-4 sm:grid-cols-2 xl:grid-cols-4" aria-busy={business.loading}>
          {cards.map(card => <ContentCard key={card.label} bodyClassName="p-space-4"><p className="text-sm text-text-secondary">{card.label}</p><p className="mt-2 flex items-baseline gap-2"><span className="text-2xl font-semibold tabular-nums">{card.value ?? '—'}</span><span className="text-sm text-text-tertiary">{card.unit}</span></p><p className="mt-2 text-sm text-text-tertiary">{card.detail}</p></ContentCard>)}
        </div>
        {business.error && <p role="alert" className="text-sm text-error">业务概览读取失败：{business.error} <button className={linkClass} onClick={refresh}>重试</button></p>}
      </>}
      <div className={`grid items-stretch gap-space-6 ${canRead ? 'xl:grid-cols-[minmax(0,2fr)_minmax(300px,1fr)]' : ''}`}>
        {canRead && <ContentCard title="待回磅任务" description="全部日期中最早的 5 趟，按首次称重时间排列。" extra={<Link className={`${linkClass} whitespace-nowrap`} to="/weighing/tasks">查看全部</Link>} bodyClassName="!p-0" className="min-w-0 overflow-hidden">
          <DataTable columns={taskColumns} dataSource={data?.awaitingTasks ?? []} rowKey="id" minWidth={680} compact loading={business.loading} error={business.error} empty={<EmptyState title="暂无待回磅任务" description="首次称重确认后，待二次称重的任务会显示在这里。" />} ariaLabel="待回磅任务" />
        </ContentCard>}
        {(canRead || canMaintain || canViewCamera) && <DeviceOverview device={device} printer={printer} cameras={cameras} canRead={canRead} canMaintain={canMaintain} canViewCamera={canViewCamera} scaleEnabled={scale?.enabled} />}
      </div>
      {canRead && <ContentCard title="最近有效单据" description="最近 5 张当前有效的单据，包含历史日期。" extra={<Link className={`${linkClass} whitespace-nowrap`} to="/weighing/tickets">查看全部</Link>} bodyClassName="!p-0" className="min-w-0 overflow-hidden">
        <DataTable columns={ticketColumns} dataSource={data?.recentTickets ?? []} rowKey="id" minWidth={960} compact loading={business.loading} error={business.error} empty={<EmptyState title="暂无有效单据" description="完成二次称重并生成有效过磅单后显示。" />} ariaLabel="最近有效单据" />
      </ContentCard>}
      {!canRead && !canMaintain && !canViewCamera && <ContentCard><EmptyState title="暂无可查看的概览内容" description="当前账号没有称重查询、设备维护或摄像头查看权限。" /></ContentCard>}
    </div>}
  </>;
}
