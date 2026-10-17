import { useState } from 'react';
import { ContentCard } from '@/components/common/content-card';
import { DetailDialog } from '@/components/common/detail-dialog';
import { getCameraPreset } from '@/lib/weighing/camera';
import { beijingTime } from '@/lib/weighing/query';
import { formatWeight } from '@/lib/weighing/weights';
import type { SimulatedCapture } from '@/types/weighing';

export function CapturePhoto({ capture, onReady, onPreviewChange, hideWeight = false }: { capture: SimulatedCapture; onReady?: (ready: boolean) => void; onPreviewChange?: (open: boolean) => void; hideWeight?: boolean }) {
  const [open, setOpen] = useState(false);
  const [failed, setFailed] = useState(false);
  const preview = (next: boolean) => { setOpen(next); onPreviewChange?.(next); };
  const metadata = <div className="space-y-1 text-xs text-text-secondary"><p>关联手填车牌：{capture.plate || '待确认时关联'}</p><p>{!hideWeight && <>{formatWeight(capture.weight)} · </>}{beijingTime(capture.at)}（北京时间）</p></div>;
  return <div className="space-y-2">
    <button type="button" className="relative block w-full overflow-hidden rounded-control border border-border bg-neutral-background text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary" aria-label={`放大模拟抓拍 ${capture.cameraLabel}`} onClick={() => preview(true)}>
      <img src={capture.imageSrc} alt={`${capture.cameraLabel}模拟抓拍`} className="aspect-video w-full object-cover" onLoad={() => { setFailed(false); onReady?.(true); }} onError={() => { setFailed(true); onReady?.(false); }} />
      <span className="absolute left-2 top-2 rounded bg-black/70 px-2 py-1 text-xs text-white">模拟图片 · {capture.cameraLabel}</span>
      <span className="absolute bottom-2 right-2 rounded bg-black/70 px-2 py-1 text-xs text-white">点击放大</span>
    </button>
    {failed && <p role="alert" className="text-sm text-error">模拟图片加载失败，请重新加载或重新采集。</p>}
    {metadata}
    <DetailDialog open={open} title="模拟图片预览" description="内置示例，非现场抓拍；车牌为操作员手填关联。" trapFocus onCancel={() => preview(false)}>
      <img src={capture.imageSrc} alt={`${capture.cameraLabel}模拟抓拍大图`} className="max-h-[60vh] w-full rounded-control object-contain" />
      <div className="mt-3">{metadata}</div>
    </DetailDialog>
  </div>;
}

export function SimulatedCameraPanel({ preset, capture, saved = false }: { preset?: string; capture?: SimulatedCapture; saved?: boolean }) {
  const camera = getCameraPreset(preset);
  return <ContentCard title="模拟摄像头" bodyClassName="space-y-3 p-3">
    <div className="relative overflow-hidden rounded-control border border-border bg-neutral-background">
      <img src={camera.src} alt={`${camera.title}模拟预览`} className="aspect-video w-full object-cover" />
      <span className="absolute left-2 top-2 rounded bg-black/70 px-2 py-1 text-xs text-white">模拟预览 · {camera.title}</span>
    </div>
    <p className="text-xs text-text-tertiary">静态示例，非实时视频 · 摄像头未接入</p>
    <div className="border-t border-border pt-3"><p className="mb-2 text-sm font-medium">{saved ? '已保存模拟抓拍' : '本次模拟抓拍'}</p>{capture ? <CapturePhoto key={capture.id} capture={capture} /> : <p className="text-xs text-text-tertiary">采集重量时同步生成模拟抓拍。</p>}</div>
  </ContentCard>;
}
