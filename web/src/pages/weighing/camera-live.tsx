import { useEffect, useRef, useState } from 'react';
import { Maximize2, Minimize2, VideoOff } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useCameraPreview } from './use-camera-preview';
import './workbench.css';

const statusLabels = {
  connecting: '正在连接实时画面', live: '实时画面',
  paused: '页面隐藏，实时画面已暂停',
  reconnecting: '画面中断，正在重连', unsupported: '当前浏览器不支持实时视频',
};

export function CameraLive({ url, title, message: configuredMessage }: { url: string; title: string; message?: string }) {
  const { videoRef, status, reconnect } = useCameraPreview(url);
  const [mode, setMode] = useState<'normal' | 'native' | 'window'>('normal');
  const frameRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (mode === 'normal') return;
    const frame = frameRef.current;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    frame?.querySelector<HTMLButtonElement>('.camera-live-exit')?.focus();
    const onFullscreenChange = () => {
      if (mode === 'native' && document.fullscreenElement !== frame) setMode('normal');
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && mode === 'window') { event.preventDefault(); setMode('normal'); }
      if (event.key === 'Tab') {
        const buttons = [...(frame?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])];
        if (!buttons.length) return;
        const first = buttons[0], last = buttons[buttons.length - 1];
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
      }
    };
    document.addEventListener('fullscreenchange', onFullscreenChange);
    document.addEventListener('keydown', onKeyDown);
    return () => { document.body.style.overflow = previousOverflow; document.removeEventListener('fullscreenchange', onFullscreenChange); document.removeEventListener('keydown', onKeyDown); requestAnimationFrame(() => frame?.querySelector<HTMLButtonElement>('.camera-live-enlarge')?.focus()); };
  }, [mode]);
  async function enlarge() {
    const frame = frameRef.current;
    if (!frame) return;
    try {
      if (!frame.requestFullscreen) throw new Error('fullscreen unavailable');
      await frame.requestFullscreen();
      setMode('native');
    } catch { setMode('window'); }
  }
  async function close() {
    if (mode === 'native' && document.fullscreenElement === frameRef.current) {
      try { await document.exitFullscreen(); } catch { /* 恢复窗口显示。 */ }
    }
    setMode('normal');
  }
  const message = url ? statusLabels[status] : configuredMessage || '当前视角未配置实时视频';
  const live = !!url && status === 'live';
  return <>
    <div ref={frameRef} className={`workbench-camera-feed camera-live-feed relative${mode !== 'normal' ? ' is-expanded' : ''}${mode === 'window' ? ' is-window-expanded' : ''}`} role={mode !== 'normal' ? 'dialog' : undefined} aria-modal={mode !== 'normal' ? true : undefined} aria-label={mode !== 'normal' ? `${title}实时画面` : undefined} data-camera-state={url ? status : 'unconfigured'}>
      <video ref={videoRef} autoPlay muted playsInline aria-label={`${title}实时视频`} className={live ? '' : 'invisible'} />
      <span role="status" className="camera-live-status"><span className={`connection-dot ${live ? 'is-online' : ''}`} />{message} · {title}</span>
      {!live && <div className="camera-placeholder camera-live-placeholder"><VideoOff className="h-12 w-12" aria-hidden /><strong>{message}</strong><span>实时预览不代替本次称重抓拍。</span>{url && status !== 'unsupported' && <Button variant="secondary" size="sm" onClick={reconnect}>重新连接</Button>}</div>}
      {mode === 'normal' ? <Button className="camera-live-enlarge" size="sm" variant="ghost" aria-label="全屏查看实时画面" disabled={!live} onClick={() => void enlarge()}><Maximize2 className="h-4 w-4" aria-hidden />全屏</Button> : <Button className="camera-live-exit" size="sm" variant="secondary" aria-label="退出实时画面全屏" onClick={() => void close()}><Minimize2 className="h-4 w-4" aria-hidden />退出全屏</Button>}
    </div>
  </>;
}
