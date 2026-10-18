import { useEffect, useRef, useState, type ReactNode } from 'react';
import { PlateCloseup, PlateEvidenceInfo } from './plate-evidence';
import { Camera, Maximize2, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { ContentCard } from '@/components/common/content-card';
import { Dialog, DialogOverlay, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogClose, DialogBody } from '@/components/ui/dialog';
import { getCameraPreset } from '@/lib/weighing/camera';
import { beijingTime } from '@/lib/weighing/query';
import { formatWeight } from '@/lib/weighing/weights';
import type { SimulatedCapture } from '@/types/weighing';

function CameraImagePreview({ open, title, description, src, alt, onClose, children }: { open: boolean; title: string; description: string; src: string; alt: string; onClose: () => void; children?: ReactNode }) {
  const controls = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    // 在 Dialog 保存原焦点后移入图片窗口，关闭时返回原放大按钮。
    const frame = requestAnimationFrame(() => controls.current?.querySelector('button')?.focus());
    return () => cancelAnimationFrame(frame);
  }, [open]);
  return <Dialog open={open} onOpenChange={next => { if (!next) onClose(); }} trapFocus restoreFocus lockScroll>
    <DialogOverlay />
    <DialogContent className="flex max-w-[1200px] flex-col">
      <DialogHeader><div><DialogTitle>{title}</DialogTitle><DialogDescription>{description}</DialogDescription></div><div ref={controls}><DialogClose aria-label="关闭图片预览"><X className="h-4 w-4" aria-hidden /></DialogClose></div></DialogHeader>
      <DialogBody><img src={src} alt={alt} className="max-h-[65dvh] w-full object-contain" />{children && <div className="mt-3">{children}</div>}</DialogBody>
    </DialogContent>
  </Dialog>;
}

export function CapturePhoto({ capture, onReady, onPreviewChange, hideWeight = false, compact = false }: { capture: SimulatedCapture; onReady?: (ready: boolean) => void; onPreviewChange?: (open: boolean) => void; hideWeight?: boolean; compact?: boolean }) {
  const [open, setOpen] = useState(false);
  const [failed, setFailed] = useState(false);
  const preview = (next: boolean) => { setOpen(next); onPreviewChange?.(next); };
  const metadata = <div className="space-y-1 text-xs text-text-secondary"><p>关联人工确认车牌：{capture.plate || '待确认时关联'}</p><p>{!hideWeight && <>{formatWeight(capture.weight)} · </>}{beijingTime(capture.at)}（北京时间）</p></div>;
  return <div className={compact ? 'grid grid-cols-[120px_minmax(0,1fr)] items-center gap-3' : 'space-y-2'}>
    <button type="button" className="relative block w-full overflow-hidden rounded-control border border-border bg-neutral-background text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary" aria-label={`放大模拟抓拍 ${capture.cameraLabel}`} onClick={() => preview(true)}>
      <img src={capture.imageSrc} alt={`${capture.cameraLabel}模拟抓拍`} className="aspect-video w-full object-contain" onLoad={() => { setFailed(false); onReady?.(true); }} onError={() => { setFailed(true); onReady?.(false); }} />
      {!compact && <span className="absolute left-2 top-2 rounded bg-black/70 px-2 py-1 text-xs text-white">模拟图片 · {capture.cameraLabel}</span>}
      <span className="absolute bottom-2 right-2 rounded bg-black/70 px-2 py-1 text-xs text-white">点击放大</span>
    </button>
    {failed && <p role="alert" className="text-sm text-error">模拟图片加载失败，请重新加载或重新采集。</p>}
    {metadata}
    {!compact && <PlateEvidenceInfo evidence={capture.plateEvidence} />}
    <CameraImagePreview open={open} title="模拟图片预览" description="内置示例，非现场抓拍；车牌经操作员核对后关联。" src={capture.imageSrc} alt={`${capture.cameraLabel}模拟抓拍大图`} onClose={() => preview(false)}>{metadata}<PlateEvidenceInfo evidence={capture.plateEvidence} /></CameraImagePreview>
  </div>;
}

export type CameraSize = 'small' | 'standard' | 'large';

export function SimulatedCameraPanel({ scenePlate, preset, capture, saved = false, size, onSizeChange }: { scenePlate?: string; preset?: string; capture?: SimulatedCapture; saved?: boolean; size: CameraSize; onSizeChange: (size: CameraSize) => void }) {
  const camera = getCameraPreset(preset);
  const [open, setOpen] = useState(false);
  const [failed, setFailed] = useState(false);
  return <ContentCard className="weighing-camera shadow-none" bodyClassName="p-0">
    <div className="workbench-camera-toolbar flex flex-wrap items-center justify-between gap-2 px-4 py-3">
      <h2 className="flex items-center gap-2 text-base font-semibold"><Camera className="h-4 w-4 text-text-tertiary" aria-hidden />现场画面</h2>
      <div className="flex items-center gap-1" role="group" aria-label="摄像头画面大小">
        {([['small', '小窗'], ['standard', '标准'], ['large', '大窗']] as const).map(([value, label]) => <Button key={value} size="sm" variant="ghost" aria-pressed={size === value} className={size === value ? 'bg-neutral-background text-primary' : ''} onClick={() => onSizeChange(value)}>{label}</Button>)}
        <Button size="sm" variant="ghost" aria-label="放大摄像头画面" onClick={() => setOpen(true)}><Maximize2 className="h-4 w-4" aria-hidden /><span className="sr-only">放大</span></Button>
      </div>
    </div>
    <button type="button" aria-label="查看摄像头大图" onClick={() => setOpen(true)} className="workbench-camera-feed relative block w-full bg-neutral-background focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary focus-visible:outline-offset-2">
      <img src={camera.src} alt={`${camera.title}模拟预览`} className="aspect-video w-full object-contain" onLoad={() => setFailed(false)} onError={() => setFailed(true)} />
      <span className="absolute left-3 top-3 rounded bg-black/70 px-2 py-1 text-xs text-white">模拟画面 · {camera.title}</span>
      <span className="absolute bottom-3 right-3 flex items-center gap-1 rounded bg-black/70 px-2 py-1 text-xs text-white"><Maximize2 className="h-3 w-3" aria-hidden />点击放大</span>
    </button>
    <div className="workbench-camera-evidence space-y-3 p-4">
      {failed && <p role="alert" className="text-sm text-error">模拟画面加载失败，请重新加载。</p>}
      <div className="workbench-camera-plate">{scenePlate ? <PlateCloseup plate={scenePlate} /> : <div className="rounded-control border border-border bg-neutral-background p-3 text-sm text-text-secondary">等待车辆到磅，请核对现场车牌。</div>}<p className="mt-2 text-xs text-text-tertiary">静态示例，非实时视频 · 摄像头未接入</p></div>
      <div className="workbench-camera-capture border-t border-border pt-3"><h3 className="mb-3 text-sm font-medium">{saved ? '已保存模拟抓拍' : '本次模拟抓拍'}</h3>{capture ? <CapturePhoto key={capture.id} capture={capture} compact /> : <div className="flex items-center gap-3 py-2 text-sm text-text-tertiary"><Camera className="h-5 w-5 shrink-0" aria-hidden /><p>采集重量时同步抓拍，图片与本次重量一起保留。</p></div>}</div>
    </div>
    <CameraImagePreview open={open} title="摄像头画面" description="静态模拟画面，非实时视频；摄像头未接入。" src={camera.src} alt={`${camera.title}模拟预览大图`} onClose={() => setOpen(false)}>{scenePlate && <PlateCloseup plate={scenePlate} />}</CameraImagePreview>
  </ContentCard>;
}
