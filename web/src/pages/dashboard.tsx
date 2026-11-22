import { useTransportDirections } from '@/hooks/use-transport-directions';
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
import type { DataTableColumn } from '@/types';
import { DeviceOverview } from './dashboard/device-overview';
import { useOverviewResource } from './dashboard/use-overview-resource';

const linkClass = 'text-sm text-primary hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary';

function waitingDuration(firstConfirmedAt: string, checkedAt: string | undefined) {
  const elapsed = Date.parse(checkedAt ?? '') - Date.parse(firstConfirmedAt);
  if (!Number.isFinite(elapsed) || elapsed < 0) return '—';
  const minutes = Math.floor(elapsed / 60000);
  if (minutes < 1) return '不足 1 分钟';
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  if (days > 0) return `${days} 天 ${hours} 小时`;
  return hours > 0 ? `${hours} 小时 ${minutes % 60} 分钟` : `${minutes} 分钟`;
}

export function DashboardPage() {
  const directionLabels = useTransportDirections();
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
    { title: '车牌', key: 'plate', width: 110, nowrap: true, render: (_, row) => <Link className={linkClass} to={`/weighing/tasks/${row.id}`}>{row.business.plate}</Link> },
    { title: '方向', key: 'direction', render: (_, row) => directionLabels[row.business.direction] },
    { title: '物料', key: 'material', render: (_, row) => [row.business.materialName, row.business.materialSpec].filter(Boolean).join(' / ') },
    { title: '首磅时间 / 已等待', key: 'first', width: 230, render: (_, row) => {
      const confirmedAt = beijingTime(row.firstConfirmedAt);
      return <div className="whitespace-nowrap tabular-nums">
        <div>{confirmedAt}</div>
        <div className="flex items-center gap-2 text-xs text-text-secondary"><span>{waitingDuration(row.firstConfirmedAt, data?.checkedAt)}</span>
          {date && Number.isFinite(Date.parse(row.firstConfirmedAt)) && confirmedAt.slice(0, 10) < date && <StatusTag>隔日</StatusTag>}
        </div>
      </div>;
    } },
  ];
  const ticketColumns: DataTableColumn<BusinessOverview['recentTickets'][number]>[] = [
    { title: '单号', key: 'ticketNo', width: 184, render: (_, row) => <Link className={linkClass} to={`/weighing/tickets/${row.id}`}>{row.ticketNo}</Link> },
    { title: '车牌', key: 'plate', render: (_, row) => row.business.plate },
    { title: '方向', key: 'direction', render: (_, row) => directionLabels[row.business.direction] },
    { title: '物料', key: 'material', render: (_, row) => [row.business.materialName, row.business.materialSpec].filter(Boolean).join(' / ') },
    { title: '最终净重（吨）', key: 'net', align: 'right', render: (_, row) => <span className="tabular-nums">{formatWeightValue(Number(row.finalNet))}</span> },
    { title: '出单时间', key: 'issuedAt', width: 176, nowrap: true, render: (_, row) => <span className="tabular-nums">{beijingTime(row.issuedAt)}</span> },
  ];
  const cards = [
    { label: '今日有效单', value: data?.validTickets, unit: '张', detail: '' },
    { label: `今日${directionLabels.inbound}净重`, value: data ? formatWeightValue(Number(data.inbound.net)) : undefined, unit: '吨', detail: data ? `${data.inbound.taskCount} 趟` : '— 趟' },
    { label: `今日${directionLabels.outbound}净重`, value: data ? formatWeightValue(Number(data.outbound.net)) : undefined, unit: '吨', detail: data ? `${data.outbound.taskCount} 趟` : '— 趟' },
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
          {cards.map(card => <ContentCard key={card.label} bodyClassName="p-space-4"><p className="text-sm text-text-secondary">{card.label}</p><p className="mt-2 flex items-baseline gap-2"><span className="text-2xl font-semibold tabular-nums">{card.value ?? '—'}</span><span className="text-sm text-text-tertiary">{card.unit}</span></p>{card.detail && <p className="mt-2 text-sm text-text-tertiary">{card.detail}</p>}</ContentCard>)}
        </div>
        {business.error && <p role="alert" className="text-sm text-error">业务概览读取失败：{business.error} <button className={linkClass} onClick={refresh}>重试</button></p>}
      </>}
      <div className={`grid items-stretch gap-space-6 ${canRead ? 'xl:grid-cols-[minmax(0,2fr)_minmax(300px,1fr)]' : ''}`}>
        {canRead && <ContentCard title="待回磅任务" extra={<Link className={`${linkClass} whitespace-nowrap`} to="/weighing/tasks">查看全部</Link>} bodyClassName="!p-0" className="min-w-0 overflow-hidden">
          <DataTable columns={taskColumns} dataSource={data?.awaitingTasks ?? []} rowKey="id" minWidth={550} compact stickyHeader loading={business.loading} error={business.error} empty={<EmptyState title="暂无待回磅任务" />} ariaLabel="待回磅任务" />
        </ContentCard>}
        {(canRead || canMaintain || canViewCamera) && <DeviceOverview device={device} printer={printer} cameras={cameras} canRead={canRead} canMaintain={canMaintain} canViewCamera={canViewCamera} scaleEnabled={scale?.enabled} />}
      </div>
      {canRead && <ContentCard title="最近有效单据" extra={<Link className={`${linkClass} whitespace-nowrap`} to="/weighing/tickets">查看全部</Link>} bodyClassName="!p-0" className="min-w-0 overflow-hidden">
        <DataTable columns={ticketColumns} dataSource={data?.recentTickets ?? []} rowKey="id" minWidth={760} compact stickyHeader loading={business.loading} error={business.error} empty={<EmptyState title="暂无有效单据" />} ariaLabel="最近有效单据" />
      </ContentCard>}
      {!canRead && !canMaintain && !canViewCamera && <ContentCard><EmptyState title="暂无可查看的概览内容" /></ContentCard>}
    </div>}
  </>;
}
