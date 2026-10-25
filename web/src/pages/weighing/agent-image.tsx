import { useEffect, useRef, useState, type ImgHTMLAttributes } from 'react';
import { http } from '@/lib/http';

type AgentImageProps = ImgHTMLAttributes<HTMLImageElement> & {
  /** Reports decode/read success for both direct and bearer-protected sources. */
  onLoadState?: (ok: boolean) => void;
};

/** Device evidence is protected by the same user permission as weighing reads. */
export function AgentImage({ src, onLoadState, onLoad, onError, ...props }: AgentImageProps) {
  const [resolved, setResolved] = useState<{ src: string; url: string } | null>(null);
  const stateRef = useRef(onLoadState);
  stateRef.current = onLoadState;
  // Server-acquired images and historical Agent uploads both need the bearer token.
  const protectedImage = !!src && (src.startsWith('/api/weighing/images/') || src.startsWith('/api/weighing/agent-images/'));
  useEffect(() => {
    if (!src || !protectedImage) return;
    const controller = new AbortController(); let objectUrl = '';
    void http.blob(src, { signal: controller.signal }).then(blob => {
      if (controller.signal.aborted) return;
      objectUrl = URL.createObjectURL(blob); setResolved({ src, url: objectUrl }); stateRef.current?.(true);
    }).catch(() => { if (!controller.signal.aborted) { setResolved({ src, url: '' }); stateRef.current?.(false); } });
    return () => { controller.abort(); if (objectUrl) URL.revokeObjectURL(objectUrl); };
  }, [src, protectedImage]);
  if (protectedImage && resolved?.src !== src) return <span role="status">正在读取抓拍…</span>;
  if (protectedImage && !resolved?.url) return <span role="alert">抓拍图片读取失败</span>;
  return <img {...props} src={protectedImage ? resolved?.url : src} onLoad={event => { onLoadState?.(true); onLoad?.(event); }} onError={event => { onLoadState?.(false); onError?.(event); }} />;
}
