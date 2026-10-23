import { useEffect, useRef, useState, type ReactNode } from 'react';
import { PlateCloseup, PlateEvidenceInfo } from './plate-evidence';
import { Camera, Maximize2, VideoOff, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { ContentCard } from '@/components/common/content-card';
import { Dialog, DialogOverlay, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogClose, DialogBody } from '@/components/ui/dialog';
import { AgentImage } from './agent-image';
import { useWeighingStore } from '@/store/weighing-store';
import { beijingTime } from '@/lib/weighing/query';
import { formatWeight } from '@/lib/weighing/weights';
import type { CaptureSnapshot } from '@/types/weighing';

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
      <DialogBody><AgentImage src={src} alt={alt} className="max-h-[65dvh] w-full object-contain" />{children && <div className="mt-3">{children}</div>}</DialogBody>
    </DialogContent>
  </Dialog>;
}

export function CapturePhoto({ capture, onReady, onPreviewChange, hideWeight = false, compact = false }: { capture: CaptureSnapshot; onReady?: (ready: boolean) => void; onPreviewChange?: (open: boolean) => void; hideWeight?: boolean; compact?: boolean }) {
  const [open, setOpen] = useState(false);
  const [view, setView] = useState('front');
  const image = capture.images?.find(image => image.view === view);
  const imageSrc = image?.url ?? capture.imageSrc;
  const cameraLabel = image ? (image.view === 'front' ? '车头视角' : '车尾视角') : capture.cameraLabel;
  const [failed, setFailed] = useState(false);
  const preview = (next: boolean) => { setOpen(next); onPreviewChange?.(next); };
  const metadata = <div className="space-y-1 text-xs text-text-secondary"><p>关联人工确认车牌：{capture.plate || '待确认时关联'}</p><p>{!hideWeight && <>{formatWeight(capture.weight)} · </>}{beijingTime(capture.at)}</p></div>;
  return <div className={compact ? 'grid grid-cols-[120px_minmax(0,1fr)] items-center gap-3' : 'space-y-2'}>
    <button type="button" className="relative block w-full overflow-hidden rounded-control border border-border bg-neutral-background text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary" aria-label={`放大抓拍 ${cameraLabel}`} onClick={() => preview(true)}>
      <AgentImage src={imageSrc} alt={`${cameraLabel}抓拍`} className="aspect-video w-full object-contain" onLoad={() => { setFailed(false); onReady?.(true); }} onError={() => { setFailed(true); onReady?.(false); }} />
      {!compact && <span className="absolute left-2 top-2 rounded bg-black/70 px-2 py-1 text-xs text-white">抓拍图片 · {cameraLabel}</span>}
      <span className="absolute bottom-2 right-2 rounded bg-black/70 px-2 py-1 text-xs text-white">点击放大</span>
    </button>
    <div className="min-w-0 space-y-2">
    {failed && <p role="alert" className="text-sm text-error">抓拍图片加载失败，请重新加载或重新采集。</p>}
    {capture.images && <div className="flex gap-2" role="group" aria-label="抓拍视角">{capture.images.map(image => <Button key={image.id} size="sm" variant="ghost" aria-pressed={view === image.view} onClick={() => { if (view !== image.view) { setView(image.view); onReady?.(false); } }}>{image.view === 'front' ? '车头' : '车尾'}</Button>)}</div>}
    {metadata}
    {!compact && <PlateEvidenceInfo evidence={capture.plateEvidence} />}
    </div>
    <CameraImagePreview open={open} title="抓拍图片预览" description="称重当次保存的图片；车牌经操作员核对后关联。" src={imageSrc} alt={`${cameraLabel}抓拍大图`} onClose={() => preview(false)}>{metadata}<PlateEvidenceInfo evidence={capture.plateEvidence} /></CameraImagePreview>
  </div>;
}

export type CameraSize = 'small' | 'standard' | 'large';

export function DeviceCameraPanel({ scenePlate, capture, saved = false, size, onSizeChange }: { scenePlate?: string; capture?: CaptureSnapshot; saved?: boolean; size: CameraSize; onSizeChange: (size: CameraSize) => void }) {
  const device = useWeighingStore(s => s.device);
  const [view, setView] = useState<'front' | 'rear'>('front');
  const camera = { src: device?.images.find(i => i.view === view)?.url ?? capture?.images?.find(i => i.view === view)?.url ?? '', title: view === 'front' ? '车头视角' : '车尾视角' };
  const [open, setOpen] = useState(false);
  const [failed, setFailed] = useState(false);
  return <ContentCard className="weighing-camera shadow-none" bodyClassName="p-0">
    <div className="workbench-camera-toolbar flex flex-wrap items-center justify-between gap-2 px-4 py-3">
      <h2 className="flex items-center gap-2 text-base font-semibold"><Camera className="h-4 w-4 text-text-tertiary" aria-hidden />现场画面<span className="camera-source-label">最近上传图片</span></h2>
      <div className="flex items-center gap-1" role="group" aria-label="摄像头画面大小">
        {([['small', '小窗'], ['standard', '标准'], ['large', '大窗']] as const).map(([value, label]) => <Button key={value} size="sm" variant="ghost" aria-pressed={size === value} className={size === value ? 'bg-neutral-background text-primary' : ''} onClick={() => onSizeChange(value)}>{label}</Button>)}
        <Button size="sm" variant="ghost" aria-label="放大摄像头画面" disabled={!camera.src} onClick={() => setOpen(true)}><Maximize2 className="h-4 w-4" aria-hidden /><span className="sr-only">放大</span></Button>
      </div>
    </div>
    <div className="workbench-camera-views" role="group" aria-label="摄像头视角">
      {(['front', 'rear'] as const).map(value => <Button key={value} size="sm" variant="ghost" aria-pressed={view === value} onClick={() => { setView(value); setFailed(false); }}>{value === 'front' ? '车头视角' : '车尾视角'}</Button>)}
      <span className="ml-auto text-xs text-text-secondary">{device?.online ? 'Agent 已连接' : 'Agent 未连接'} · 非实时视频</span>
    </div>
    <button type="button" disabled={!camera.src} aria-label="查看摄像头大图" onClick={() => setOpen(true)} className="workbench-camera-feed relative block w-full bg-neutral-background focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary focus-visible:outline-offset-2">
      {camera.src ? <AgentImage key={camera.src} src={camera.src} alt={`${camera.title}预览`} className="aspect-video w-full object-contain" onLoad={() => setFailed(false)} onError={() => setFailed(true)} /> : <span className="camera-placeholder"><VideoOff className="h-12 w-12" aria-hidden /><strong>尚未取得现场图片</strong><span>{device?.online ? '点击“采集重量”，由 Agent 同步获取前后抓拍。' : '请先连接 Agent 并等待设备重量就绪。'}</span><span>实时视频暂未接入</span></span>}
      <span className="absolute left-3 top-3 rounded bg-black/70 px-2 py-1 text-xs text-white">最近图片 · {camera.title}</span>
      <span className="absolute bottom-3 right-3 flex items-center gap-1 rounded bg-black/70 px-2 py-1 text-xs text-white"><Maximize2 className="h-3 w-3" aria-hidden />点击放大</span>
    </button>
    <div className="workbench-camera-evidence space-y-3 p-4">
      {failed && <p role="alert" className="text-sm text-error">最近图片加载失败，请检查连接或等待 Agent 重新上传。</p>}
      <div className="workbench-camera-plate">{scenePlate ? <PlateCloseup plate={scenePlate} /> : <div className="rounded-control border border-border bg-neutral-background p-3 text-sm text-text-secondary">请核对现场车牌，图片不能替代人工确认。</div>}<p className="mt-2 text-xs text-text-tertiary"></p></div>
      <div className="workbench-camera-capture border-t border-border pt-3"><h3 className="mb-3 text-sm font-medium">{saved ? '已保存抓拍' : '本次抓拍'}</h3>{capture ? <CapturePhoto key={capture.id} capture={capture} compact /> : <div className="flex items-center gap-3 py-2 text-sm text-text-tertiary"><Camera className="h-5 w-5 shrink-0" aria-hidden /><p>采集重量时同步抓拍，图片与本次重量一起保留。</p></div>}</div>
    </div>
    <CameraImagePreview open={open && !!camera.src} title="摄像头画面" description="" src={camera.src} alt={`${camera.title}预览大图`} onClose={() => setOpen(false)}>{scenePlate && <PlateCloseup plate={scenePlate} />}</CameraImagePreview>
  </ContentCard>;
}
