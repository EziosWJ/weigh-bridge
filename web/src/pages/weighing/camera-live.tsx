import { useEffect, useRef, useState } from 'react';
import { Maximize2, VideoOff, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogOverlay, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogClose, DialogBody } from '@/components/ui/dialog';
import { useCameraPreview } from './use-camera-preview';
import './workbench.css';

const statusLabels = {
  connecting: '正在连接实时画面', live: '实时画面',
  paused: '页面隐藏，实时画面已暂停',
  reconnecting: '画面中断，正在重连', unsupported: '当前浏览器不支持实时视频',
};

function EnlargedVideo({ stream, title }: { stream: MediaStream | null; title: string }) {
  const ref = useRef<HTMLVideoElement>(null);
  useEffect(() => {
    const video = ref.current;
    if (!video) return;
    video.srcObject = stream;
    if (stream) void video.play().catch(() => {});
    return () => { video.pause(); video.srcObject = null; };
  }, [stream]);
  return <video ref={ref} autoPlay muted playsInline aria-label={`${title}实时大图`} className="max-h-[65dvh] w-full object-contain" />;
}

export function CameraLive({ url, title, message: configuredMessage }: { url: string; title: string; message?: string }) {
  const { videoRef, stream, status, reconnect } = useCameraPreview(url);
  const [open, setOpen] = useState(false);
  const closeRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() => closeRef.current?.querySelector('button')?.focus());
    return () => cancelAnimationFrame(frame);
  }, [open]);
  const message = url ? statusLabels[status] : configuredMessage || '当前视角未配置实时视频';
  const live = !!url && status === 'live';
  return <>
    <div className="workbench-camera-feed camera-live-feed relative" data-camera-state={url ? status : 'unconfigured'}>
      <video ref={videoRef} autoPlay muted playsInline aria-label={`${title}实时视频`} className={live ? '' : 'invisible'} />
      <span role="status" className="camera-live-status"><span className={`connection-dot ${live ? 'is-online' : ''}`} />{message} · {title}</span>
      {!live && <div className="camera-placeholder camera-live-placeholder"><VideoOff className="h-12 w-12" aria-hidden /><strong>{message}</strong><span>实时预览不代替本次称重抓拍。</span>{url && status !== 'unsupported' && <Button variant="secondary" size="sm" onClick={reconnect}>重新连接</Button>}</div>}
      <Button className="camera-live-enlarge" size="sm" variant="ghost" aria-label="放大实时画面" disabled={!live} onClick={() => setOpen(true)}><Maximize2 className="h-4 w-4" aria-hidden />放大</Button>
    </div>
    <Dialog open={open} onOpenChange={setOpen} trapFocus restoreFocus lockScroll>
      <DialogOverlay />
      <DialogContent className="flex max-w-[1200px] flex-col">
        <DialogHeader><div><DialogTitle>{title}实时画面</DialogTitle><DialogDescription>实时监看；正式称重以本次采集保存的抓拍为依据。</DialogDescription></div><div ref={closeRef}><DialogClose aria-label="关闭实时画面"><X className="h-4 w-4" aria-hidden /></DialogClose></div></DialogHeader>
        <DialogBody><EnlargedVideo stream={stream} title={title} /><p role="status" className="mt-2 text-sm text-text-secondary">{message}</p></DialogBody>
      </DialogContent>
    </Dialog>
  </>;
}
