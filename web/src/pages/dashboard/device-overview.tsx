import { Camera, Gauge, Printer, Video, type LucideIcon } from 'lucide-react';
import type { OverviewDevice } from '@/api/overview';
import type { CameraList } from '@/types/camera';
import { ContentCard } from '@/components/common/content-card';
import { StatusTag } from '@/components/common/status-tag';
import { beijingTime } from '@/lib/weighing/query';
import type { OverviewResource } from './use-overview-resource';
import './device-overview.css';

type Props = {
  device: OverviewResource<OverviewDevice>;
  printer: OverviewResource<Record<string, unknown>>;
  cameras: OverviewResource<CameraList>;
  canRead: boolean;
  canMaintain: boolean;
  canViewCamera: boolean;
  scaleEnabled?: number;
};
type State = { label: string; detail?: string; tone: 'success' | 'warning' | 'error' | 'neutral' };
const unknown: State = { label: '未知', tone: 'neutral' };
const text = (value: unknown) => typeof value === 'string' ? value : '';

export function DeviceOverview({ device, printer, cameras, canRead, canMaintain, canViewCamera, scaleEnabled }: Props) {
  const state = device.data;
  const maintenance = printer.data;
  const offline = maintenance?.agentProcessOnline === false || maintenance?.maintenanceAvailable === false;
  let instrument: State = device.loading ? { label: '读取中', tone: 'neutral' } : { ...unknown, detail: device.error };
  if (state) instrument = !state.online ? { label: '离线或数据过期', tone: 'error', detail: '暂不能采集重量' }
    : !state.deviceOk ? { label: '采集故障', tone: 'error', detail: state.error || '暂不能采集重量' }
    : { label: '采集中', tone: 'success', detail: state.stable ? '最近上报重量稳定' : '最近上报重量未稳定' };
  if (scaleEnabled === 0) instrument = { label: '已停用', tone: 'warning', detail: '本厂地磅已停用' };
  let print: State = printer.loading ? { label: '读取中', tone: 'neutral' } : { ...unknown, detail: printer.error };
  if (maintenance) {
    if (offline) print = { label: '维护连接不可用', tone: 'error', detail: text(maintenance.lastError) || '请检查现场 Agent' };
    else if (maintenance.agentKnown === true && maintenance.agent !== '运行中') print = { label: text(maintenance.agent) || '未知', tone: 'warning', detail: '设备执行未运行' };
    else if (maintenance.cupsKnown === true && maintenance.cups !== '运行中') print = { label: '打印服务异常', tone: 'error', detail: text(maintenance.cups) };
    else if (maintenance.queueKnown === true && /停|故障|失败|不可用/.test(text(maintenance.queueState))) print = { label: '队列异常', tone: 'error', detail: text(maintenance.queueState) };
    else if (maintenance.agentKnown === true && maintenance.cupsKnown === true && maintenance.queueKnown === true) print = { label: text(maintenance.queueState) || '未知', tone: 'neutral', detail: text(maintenance.queue) };
  }
  const captureCameras = cameras.data?.cameras.filter(camera => camera.enabled && camera.captureEnabled);
  const captureRequired = state?.captureRequired;
  const captureMissing = captureRequired === true && captureCameras?.length === 0;
  const video = cameras.data?.video;
  const syncError = video?.syncState === 'error' || cameras.data?.cameras.some(camera => camera.enabled && camera.stream.state === 'error');
  const jobs = maintenance?.jobsKnown === false;
  const rows: { title: string; state: State }[] = [];
  if (canRead) rows.push({ title: '地磅仪表', state: instrument });
  if (canMaintain) rows.push({ title: '打印设备', state: print });
  if (canRead || canViewCamera) rows.push({ title: '称重抓拍', state: {
    label: captureMissing ? '未配置抓拍摄像头' : captureRequired === undefined ? '策略未知' : captureRequired ? '强制抓拍开启' : '强制抓拍关闭',
    tone: captureMissing ? 'error' : 'neutral',
    detail: cameras.error || (canViewCamera ? captureCameras ? `${captureCameras.length} 台参与抓拍，连通性未检测` : cameras.loading ? '正在读取摄像头配置' : '摄像头状态未知' : '采集时核实抓拍结果'),
  } });
  if (canViewCamera && video) rows.push({ title: '视频预览同步', state: {
    label: !video.whepBaseUrl ? '未配置' : syncError ? '同步失败' : video.syncState === 'synced' ? '映射已同步' : '待同步',
    tone: syncError ? 'warning' : 'neutral',
    detail: syncError ? '预览同步失败，抓拍独立核实' : '摄像头连通性未检测',
  } });

  const icons: Record<string, LucideIcon> = { '地磅仪表': Gauge, '打印设备': Printer, '称重抓拍': Camera, '视频预览同步': Video };
  const readAt = [device.checkedAt, printer.checkedAt, cameras.checkedAt].filter(Boolean).sort().at(-1);
  return <ContentCard title="设备状态摘要" className="dashboard-devices min-w-0 overflow-hidden" bodyClassName="!p-0 flex flex-1 flex-col">
    <div className="dashboard-device-grid">
      {rows.map(row => {
        const Icon = icons[row.title];
        return <div key={row.title} className="dashboard-device-item min-w-0 space-y-space-2 p-space-4">
          <div className="flex items-center gap-space-2 text-sm text-text-secondary"><Icon className="h-4 w-4 shrink-0 text-text-tertiary" aria-hidden /><span>{row.title}</span></div>
          <StatusTag tone={row.state.tone}>{row.state.label}</StatusTag>
          {row.state.detail && <p className="break-words text-sm leading-5 text-text-tertiary">{row.state.detail}</p>}
          {row.title === '打印设备' && jobs && !offline && <p className="text-sm leading-5 text-warning">作业状态待核实</p>}
        </div>;
      })}
    </div>
    <footer className="flex flex-wrap items-center justify-between gap-x-space-4 gap-y-space-2 border-t border-border px-space-4 py-space-3">
      <span className="text-xs text-text-tertiary" title={readAt ? `最近读取：${beijingTime(readAt)}；点击页面刷新更新状态` : '尚未取得设备状态'}>最近读取 {readAt ? beijingTime(readAt).slice(11) : '—'}</span>
    </footer>
  </ContentCard>;
}
