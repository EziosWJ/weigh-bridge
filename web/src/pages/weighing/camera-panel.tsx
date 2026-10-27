import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Camera, RefreshCw, VideoOff, X } from 'lucide-react';
import { PlateCloseup, PlateEvidenceInfo } from './plate-evidence';
import { Button } from '@/components/ui/button';
import { ContentCard } from '@/components/common/content-card';
import { Dialog, DialogOverlay, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogClose, DialogBody } from '@/components/ui/dialog';
import { AgentImage } from './agent-image';
import { useWeighingStore } from '@/store/weighing-store';
import { beijingTime } from '@/lib/weighing/query';
import { formatWeight } from '@/lib/weighing/weights';
import { getErrorMessage } from '@/lib/api-error';
import { cameraApi } from '@/api/camera';
import type { CameraRecord, CameraStream } from '@/types/camera';
import type { CaptureSnapshot, DeviceImage } from '@/types/weighing';
import { CameraLive } from './camera-live';

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

/** Real camera name with a clear historical label for old Agent front/rear rows. */
function imageLabel(image: DeviceImage, capture: CaptureSnapshot) {
  if (image.cameraName) return image.cameraName;
  if (image.view === 'front') return '车头视角（历史）';
  if (image.view === 'rear') return '车尾视角（历史）';
  return capture.cameraLabel || '抓拍图片';
}

function imageStatusText(status?: string) {
  if (status === 'disabled') return '未配置抓拍摄像头';
  if (status === 'capture_failed') return '本次未取得有效图片';
  if (status === 'upload_failed') return '历史上传失败';
  return '抓拍完整';
}

export function CapturePhoto({ capture, onReady, onPreviewChange, hideWeight = false, compact = false, gallery = false }: { capture: CaptureSnapshot; onReady?: (ready: boolean) => void; onPreviewChange?: (open: boolean) => void; hideWeight?: boolean; compact?: boolean; gallery?: boolean }) {
  const images = useMemo(() => capture.images ?? [], [capture.images]);
  const [activeId, setActiveId] = useState(images[0]?.id ?? '');
  useEffect(() => { if (!images.some(image => image.id === activeId)) setActiveId(images[0]?.id ?? ''); }, [capture.id, images, activeId]);
  const active = images.find(image => image.id === activeId) ?? images[0];
  const imageSrc = active?.url || capture.imageSrc || '';
  const cameraLabel = active ? imageLabel(active, capture) : capture.cameraLabel || '抓拍图片';
  const [open, setOpen] = useState(false);
  const [loaded, setLoaded] = useState<Record<string, boolean>>({});
  const [failed, setFailed] = useState<Record<string, boolean>>({});
  const preview = (next: boolean) => { setOpen(next); onPreviewChange?.(next); };
  const required = capture.captureRequired !== false && capture.imageStatus !== 'disabled';
  const keys = images.length ? images.map(image => image.id) : imageSrc ? ['legacy'] : [];
  const allLoaded = keys.length > 0 && keys.every(key => loaded[key]);
  const anyFailed = keys.some(key => failed[key]);
  useEffect(() => {
    // Optional capture may confirm without an image; required capture waits for
    // every acquired image so the operator reviews all cameras, not just one.
    if (!required) onReady?.(true);
    else onReady?.(!!imageSrc && allLoaded && !anyFailed);
  }, [required, imageSrc, allLoaded, anyFailed, onReady]);
  const mark = (key: string, ok: boolean) => {
    if (ok) { setLoaded(value => value[key] ? value : { ...value, [key]: true }); setFailed(value => { if (!value[key]) return value; const next = { ...value }; delete next[key]; return next; }); }
    else setFailed(value => value[key] ? value : { ...value, [key]: true });
  };
  const policy = <p className="text-xs text-text-secondary">当时强制抓拍：{capture.captureRequired ? '开启' : '关闭'} · {imageStatusText(capture.imageStatus)}{capture.imageError ? ` · ${capture.imageError}` : ''}{capture.cameraConfigVersion ? ` · 配置版本 ${capture.cameraConfigVersion}` : ''}</p>;
  const failures = capture.failures?.length ? <ul role="alert" className="space-y-1 text-xs text-error">{capture.failures.map(failure => <li key={`${failure.cameraId}-${failure.code}`}>{failure.cameraName || `摄像头 ${failure.cameraId}`}：{failure.message}</li>)}</ul> : null;
  const sourceLabel = capture.source === 'server' ? '图片来源：Server 直连抓拍' : capture.source === 'simulated' ? '图片来源：历史模拟数据' : capture.source === 'agent' ? '图片来源：历史 Agent 上传' : '';
  const metadata = <div className="space-y-1 text-xs text-text-secondary"><p>关联人工确认车牌：{capture.plate || '待确认时关联'}</p>{images.length === 1 && <p>摄像头：{cameraLabel}</p>}<p>{!hideWeight && <>{formatWeight(capture.weight)} · </>}{beijingTime(capture.at)}</p>{sourceLabel && <p>{sourceLabel}</p>}</div>;
  if (compact || gallery) {
    const photos = images.length ? images : imageSrc ? [{ id: 'legacy', url: imageSrc, cameraName: cameraLabel, capturedAt: capture.at }] : [];
    return <div className="workbench-capture-content">
      {photos.length > 0 ? <div className="workbench-capture-gallery" role="group" aria-label={gallery ? '本次抓拍图片' : '抓拍摄像头'}>{photos.map(image => <div className="workbench-capture-photo" key={image.id}>
        <button type="button" className="workbench-capture-thumbnail" aria-label={`放大抓拍 ${imageLabel(image, capture)}`} onClick={() => { setActiveId(image.id); preview(true); }}>
          <AgentImage src={image.url} alt={`${imageLabel(image, capture)}抓拍`} className="aspect-video w-full object-contain" onLoadState={ok => mark(image.id, ok)} />
          <span>点击放大</span>
        </button>
        <p className="text-sm">{imageLabel(image, capture)}</p>
        <p className="text-xs text-text-tertiary">{beijingTime(image.capturedAt || capture.at)}</p>
        {failed[image.id] && <p role="alert" className="text-sm text-error">图片加载失败，请重新加载或重新采集。</p>}
      </div>)}</div> : <p className={capture.captureRequired === false ? 'text-sm text-warning' : 'text-sm text-error'} role={capture.captureRequired === false ? undefined : 'alert'}>{capture.captureRequired === false ? '本次无抓拍图片，请核对现场车辆和重量。' : '缺少本次抓拍图片，请重新采集。'}</p>}
      {failures}
      {capture.imageError && <p role="alert" className="text-sm text-error">{capture.imageError}</p>}
      {sourceLabel && capture.source !== 'server' && <p className="text-xs text-text-secondary">{sourceLabel}</p>}
      {photos.length > 0 && <CameraImagePreview open={open} title="抓拍图片预览" description="称重当次保存的图片；请核对车牌与车辆上磅情况。" src={imageSrc} alt={`${cameraLabel}抓拍大图`} onClose={() => preview(false)}>{failures}{metadata}<PlateEvidenceInfo evidence={capture.plateEvidence} /></CameraImagePreview>}
    </div>;
  }
  if (!imageSrc) return <div className="space-y-2 text-sm text-text-secondary">{policy}{failures}
    {capture.captureRequired === false ? <p>本次无抓拍图片，请核对现场车辆和重量。</p> : <p role="alert" className="text-error">缺少本次抓拍图片，请返回重新采集。</p>}{metadata}</div>;
  return <div className="space-y-2">
    <button type="button" className="relative block w-full overflow-hidden rounded-control border border-border bg-neutral-background text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary" aria-label={`放大抓拍 ${cameraLabel}`} onClick={() => preview(true)}>
      <AgentImage key={imageSrc} src={imageSrc} alt={`${cameraLabel}抓拍`} className="aspect-video w-full object-contain" onLoadState={ok => mark(active?.id ?? 'legacy', ok)} />
      <span className="absolute left-2 top-2 max-w-[70%] truncate rounded bg-black/70 px-2 py-1 text-xs text-white">抓拍图片 · {cameraLabel}</span>
      <span className="absolute bottom-2 right-2 rounded bg-black/70 px-2 py-1 text-xs text-white">点击放大</span>
    </button>
    <div className="min-w-0 space-y-2">
    {policy}
    {failures}
    {images.length > 1 && <div className="flex flex-wrap gap-2" role="group" aria-label="抓拍摄像头">{images.map(image => <Button key={image.id} size="sm" variant="ghost" aria-pressed={active?.id === image.id} onClick={() => setActiveId(image.id)}>{imageLabel(image, capture)}{failed[image.id] ? ' · 加载失败' : ''}</Button>)}</div>}
    <div className="flex flex-wrap gap-2" aria-hidden={images.length > 1}>
      {images.length > 1 && images.map(image => <span key={`thumb-${image.id}`} className="hidden"><AgentImage src={image.url} alt="" onLoadState={ok => mark(image.id, ok)} /></span>)}
    </div>
    {anyFailed && <p role="alert" className="text-sm text-error">有抓拍图片加载失败；请重新加载或重新采集后再确认。</p>}
    {metadata}
    <PlateEvidenceInfo evidence={capture.plateEvidence} />
    </div>
    <CameraImagePreview open={open} title="抓拍图片预览" description="称重当次由 Server 保存的图片；车牌经操作员核对后关联。" src={imageSrc} alt={`${cameraLabel}抓拍大图`} onClose={() => preview(false)}>{failures}{metadata}<PlateEvidenceInfo evidence={capture.plateEvidence} /></CameraImagePreview>
  </div>;
}

export type CameraSize = 'small' | 'standard' | 'large';

function previewMessage(camera: CameraRecord, stream: CameraStream | null, error: string) {
  if (error) return `预览信息读取失败：${error}`;
  if (!camera.enabled) return '摄像头已停用，不能预览；已保存的历史图片仍可查看。';
  if (!stream) return '正在读取视频预览状态…';
  if (stream.playbackUrl) return '';
  if (stream.state === 'error') return `视频映射同步失败${stream.lastError ? `：${stream.lastError}` : '，请在摄像头管理中重试同步。'}`;
  if (stream.state === 'removing') return '视频映射待移除；当前不能预览。';
  return `视频映射待同步${stream.lastError ? `：${stream.lastError}` : '，同步完成后可预览；直连抓拍不受影响。'}`;
}

export function DeviceCameraPanel({ scaleId, scenePlate, capture, saved = false, size, onSizeChange }: { scaleId: number; scenePlate?: string; capture?: CaptureSnapshot; saved?: boolean; size: CameraSize; onSizeChange: (size: CameraSize) => void }) {
  const device = useWeighingStore(state => state.device);
  const [cameras, setCameras] = useState<CameraRecord[] | null>(null);
  const [cameraError, setCameraError] = useState('');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [stream, setStream] = useState<CameraStream | null>(null);
  const [streamError, setStreamError] = useState('');
  const [streamLoading, setStreamLoading] = useState(false);
  const validScale = Number.isSafeInteger(scaleId) && scaleId > 0;

  const loadCameras = useCallback(async () => {
    if (!validScale) { setCameras([]); return; }
    try {
      const result = await cameraApi.list();
      setCameras(result.cameras);
      setCameraError('');
      setSelectedId(current => {
        if (current && result.cameras.some(camera => camera.id === current)) return current;
        return result.cameras.find(camera => camera.enabled)?.id ?? result.cameras[0]?.id ?? null;
      });
    } catch (error) {
      // Permission and availability failures must not block formal capture.
      setCameras([]);
      setSelectedId(null);
      setCameraError(getErrorMessage(error, '无法读取摄像头配置'));
    }
  }, [validScale]);

  useEffect(() => {
    void loadCameras();
    if (!validScale) return;
    const timer = setInterval(() => void loadCameras(), 20_000);
    return () => clearInterval(timer);
  }, [loadCameras, validScale]);

  const selected = cameras?.find(camera => camera.id === selectedId) ?? null;

  const loadStream = useCallback(async (id: number) => {
    setStreamLoading(true);
    try {
      const value = await cameraApi.preview(id);
      setStream(value);
      setStreamError('');
    } catch (error) {
      setStream(null);
      setStreamError(getErrorMessage(error, '无法读取视频预览'));
    } finally {
      setStreamLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!selected?.enabled) { setStream(null); setStreamError(''); return; }
    setStream(selected.stream);
    setStreamError('');
    void loadStream(selected.id);
    const timer = setInterval(() => void loadStream(selected.id), 20_000);
    return () => clearInterval(timer);
  }, [selected, loadStream]);

  const activeStream = streamError ? null : stream ?? selected?.stream ?? null;
  const playbackURL = !streamError && selected?.enabled && activeStream?.state === 'synced' ? activeStream.playbackUrl : '';
  const message = selected ? previewMessage(selected, activeStream, streamError) : '';
  // A saved sample uses its frozen policy, never the current device policy.
  const captureRequired = capture ? capture.captureRequired : device?.captureRequired;
  const imageCount = capture ? capture.images?.length || (capture.imageSrc ? 1 : 0) : 0;
  const failureCount = capture?.failures?.length ?? 0;
  const captureIssue = !!capture && (failureCount > 0 || !!capture.imageError || capture.imageStatus === 'capture_failed' || capture.imageStatus === 'upload_failed' || imageCount === 0);
  const captureStatus = !capture ? '等待采集' : imageCount === 0 ? '本次无图' : captureIssue ? `已取得 ${imageCount} 张 · 抓拍不完整` : `已取得 ${imageCount}/${imageCount} 张`;

  return <ContentCard className="weighing-camera shadow-none" bodyClassName="p-0">
    <div className="workbench-camera-toolbar flex flex-wrap items-center justify-between gap-2 px-4 py-3">
      <h2 className="flex items-center gap-2 text-base font-semibold"><Camera className="h-4 w-4 text-text-tertiary" aria-hidden />现场画面<span className="camera-source-label">{selected?.enabled ? '实时预览' : '本次抓拍'}</span></h2>
      <div className="flex items-center gap-1" role="group" aria-label="摄像头画面大小">
        {([['small', '小窗'], ['standard', '标准'], ['large', '大窗']] as const).map(([value, label]) => <Button key={value} size="sm" variant="ghost" aria-pressed={size === value} className={size === value ? 'bg-neutral-background text-primary' : ''} onClick={() => onSizeChange(value)}>{label}</Button>)}
      </div>
    </div>
    <div className="workbench-camera-views" role="group" aria-label="摄像头">
      {(cameras ?? []).map(camera => <Button key={camera.id} size="sm" variant="ghost" aria-pressed={selectedId === camera.id} onClick={() => setSelectedId(camera.id)}>{camera.name}{camera.captureEnabled ? ' · 抓拍' : ''}{camera.enabled ? '' : ' · 停用'}</Button>)}
      {cameras === null && <span role="status" className="text-xs text-text-secondary">正在读取摄像头…</span>}
      {cameras?.length === 0 && !cameraError && <span className="text-xs text-text-secondary">尚未配置摄像头</span>}
      <span className="ml-auto text-xs text-text-secondary">{cameraError ? '预览不可用' : playbackURL ? '实时监控 · 正式称重另行抓拍' : selected ? '预览待就绪 · 抓拍不受影响' : '预览与抓拍不依赖 Agent 在线'}</span>
    </div>
    {cameraError && <p role="alert" className="px-4 py-2 text-sm text-warning">{cameraError}；正式采集与已保存图片不受影响。</p>}
    {selected?.enabled && playbackURL ? <CameraLive key={`${selected.id}:${activeStream?.syncedVersion ?? ''}:${playbackURL}`} url={playbackURL} title={selected.name} /> : <div className="workbench-camera-feed relative">
      <span className="camera-placeholder"><VideoOff className="h-12 w-12" aria-hidden /><strong>{selected ? (message || '视频映射待同步') : cameras?.length ? '请选择摄像头' : cameraError ? '无法读取摄像头预览' : '尚未配置摄像头'}</strong><span>{selected ? '实时预览不代替本次称重抓拍；可在摄像头管理中重试同步或测试抓拍。' : cameraError ? '当前账号可能没有摄像头查看权限；本次称重抓拍仍由正式采集完成。' : '在摄像头管理中添加摄像头后即可预览；正式抓拍不依赖 Agent 在线。'}</span>{selected && <Button size="sm" variant="secondary" disabled={streamLoading} onClick={() => { void loadCameras(); void loadStream(selected.id); }}><RefreshCw className="h-4 w-4" aria-hidden />重新读取预览</Button>}</span>
    </div>}
    <section className="workbench-camera-evidence" aria-label={saved ? '已保存抓拍' : '本次抓拍'}>
      <header className="workbench-capture-heading">
        <h3>{saved ? '已保存抓拍' : '本次抓拍'}</h3>
        {captureRequired != null && <span className={`workbench-capture-policy${captureRequired ? '' : ' is-optional'}`}>{captureRequired ? '强制抓拍' : '非强制抓拍'}</span>}
        <span role="status" className={`workbench-capture-status${capture ? captureIssue ? ' text-warning' : ' text-success' : ''}`}>{captureStatus}</span>
      </header>
      {capture ? <CapturePhoto key={capture.id} capture={capture} compact /> : <div className="workbench-capture-empty"><Camera className="h-5 w-5 shrink-0" aria-hidden /><p>采集重量时自动抓拍，请核对现场车牌。</p></div>}
      {scenePlate && <div className="workbench-camera-plate"><PlateCloseup plate={scenePlate} /></div>}
      <p className="workbench-capture-hint">{capture ? '请核对车牌与车辆上磅情况。' : captureRequired === false ? '本厂未强制抓拍，允许无图确认；实时预览仅用于监看。' : '实时预览仅用于监看；抓拍图片随确认称重保存。'}</p>
    </section>
  </ContentCard>;
}
