import { useEffect, useState } from 'react';
import { buildApiUrl } from '@/lib/http';
import { useAuthStore } from '@/store/auth-store';

export type CameraPreview = {
  mode: 'legacy' | 'mediamtx'; enabled?: boolean; ready?: boolean;
  front?: string; rear?: string; revision?: string;
  message?: string;
};

// Read the active mapping through the existing business API. Video itself
// connects directly to the configured MediaMTX address.
export function useCameraConfiguration(scaleId: number, diagnostic = false) {
  const token = useAuthStore(state => state.token);
  const [result, setResult] = useState<{ scale: number; token: string | null; data: CameraPreview } | null>(null);
  useEffect(() => {
    if (!Number.isSafeInteger(scaleId) || scaleId < 1) return;
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    let controller: AbortController;
    const poll = async () => {
      controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 4000);
      try {
        const response = await fetch(buildApiUrl(diagnostic ? '/api/system/agent-maintenance/camera-preview' : `/api/weighing/scales/${scaleId}/camera-preview`), {
          headers: token ? { Authorization: token } : {}, signal: controller.signal, cache: 'no-store',
        });
        if (!response.ok) throw new Error(response.status === 403 || response.status === 401 ? '无权读取摄像头配置' : '无法读取摄像头配置');
        const body = await response.json() as { code: number; data: CameraPreview };
        if (body.code !== 200 || !body.data || !['legacy', 'mediamtx'].includes(body.data.mode)) throw new Error('摄像头配置响应无效');
        if (!disposed) setResult({ scale: scaleId, token, data: body.data });
      } catch (error) {
        if (!disposed) setResult({ scale: scaleId, token, data: { mode: 'mediamtx', ready: false, message: error instanceof Error ? error.message : '无法读取摄像头配置' } });
      } finally {
        clearTimeout(timeout);
        if (!disposed) timer = setTimeout(() => void poll(), 2000);
      }
    };
    void poll();
    return () => { disposed = true; clearTimeout(timer); controller?.abort(); };
  }, [scaleId, token, diagnostic]);
  return result?.scale === scaleId && result.token === token ? result.data : { mode: 'mediamtx' as const, ready: false, message: '正在读取摄像头配置' };
}
