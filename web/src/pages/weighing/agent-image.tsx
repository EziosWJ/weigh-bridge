import { useEffect, useState, type ImgHTMLAttributes } from 'react';
import { http } from '@/lib/http';

/** Device evidence is protected by the same user permission as weighing reads. */
export function AgentImage({ src, ...props }: ImgHTMLAttributes<HTMLImageElement>) {
  const [resolved, setResolved] = useState<{ src: string; url: string } | null>(null);
  const protectedImage = src?.startsWith('/api/weighing/agent-images/');
  useEffect(() => {
    if (!src || !protectedImage) return;
    const controller = new AbortController(); let objectUrl = '';
    void http.blob(src, { signal: controller.signal }).then(blob => {
      if (controller.signal.aborted) return;
      objectUrl = URL.createObjectURL(blob); setResolved({ src, url: objectUrl });
    }).catch(() => { if (!controller.signal.aborted) setResolved({src,url:''}); });
    return () => { controller.abort(); if (objectUrl) URL.revokeObjectURL(objectUrl); };
  }, [src, protectedImage]);
  if (protectedImage && resolved?.src !== src) return <span role="status">正在读取抓拍…</span>;
  if (protectedImage && !resolved?.url) return <span role="alert">抓拍图片读取失败</span>;
  return <img {...props} src={protectedImage ? resolved?.url : src} />;
}
