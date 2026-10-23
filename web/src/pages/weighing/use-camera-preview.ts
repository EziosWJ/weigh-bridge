import { useEffect, useRef, useState } from 'react';
import MediaMTXWebRTCReader from '@/lib/vendor/mediamtx/reader.js';

type PreviewStatus = 'connecting' | 'live' | 'reconnecting' | 'paused' | 'unsupported';

// Own exactly one reader per selected view. Decoded frames stay in the browser;
// React receives only the current stream and infrequent connection state changes.
export function useCameraPreview(url: string) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [stream, setStream] = useState<MediaStream | null>(null);
  const [status, setStatus] = useState<PreviewStatus>('connecting');
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !url) return;
    if (typeof RTCPeerConnection === 'undefined') {
      setStatus('unsupported');
      return;
    }
    let disposed = false;
    let reader: MediaMTXWebRTCReader | null = null;
    let currentStream: MediaStream | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let attempts = 0;
    let generation = 0;
    let lastProgress = performance.now();
    let lastFrames = 0;
    let playing = false;
    const presentedFrames = () => {
      const quality = video.getVideoPlaybackQuality();
      return quality.totalVideoFrames - quality.droppedVideoFrames;
    };

    const release = () => {
      generation++;
      reader?.close();
      reader = null;
      currentStream?.getTracks().forEach(track => track.stop());
      currentStream = null;
      video.pause();
      video.srcObject = null;
      setStream(null);
    };
    const reconnect = () => {
      if (disposed || retryTimer !== undefined) return;
      release();
      playing = false;
      setStatus('reconnecting');
      retryTimer = setTimeout(start, Math.min(1000 * 2 ** Math.min(attempts++, 5), 30_000));
    };
    const start = () => {
      retryTimer = undefined;
      if (disposed) return;
      if (document.hidden) { setStatus('paused'); return; }
      const activeGeneration = ++generation;
      lastProgress = performance.now();
      playing = false;
      setStatus(attempts ? 'reconnecting' : 'connecting');
      reader = new MediaMTXWebRTCReader({
        url,
        onError: () => { if (!disposed && activeGeneration === generation) reconnect(); },
        onTrack: event => {
          if (disposed || activeGeneration !== generation || event.track.kind !== 'video') return;
          currentStream = event.streams[0] ?? new MediaStream([event.track]);
          lastFrames = presentedFrames();
          video.srcObject = currentStream;
          setStream(currentStream);
          // Muted autoplay is normally allowed. If interrupted, the watchdog
          // exposes the failure and the explicit reconnect button stays usable.
          void video.play().catch(() => {});
        },
      });
    };
    const visibilityChanged = () => {
      if (document.hidden) {
        clearTimeout(retryTimer);
        retryTimer = undefined;
        release();
        playing = false;
        setStatus('paused');
      } else start();
    };
    document.addEventListener('visibilitychange', visibilityChanged);
    start();
    const watchdog = setInterval(() => {
      if (disposed || document.hidden || retryTimer !== undefined) return;
      const frames = presentedFrames();
      // Counters can reset when srcObject changes during a reconnect.
      if (currentStream && video.readyState >= 2 && frames !== lastFrames) {
        lastFrames = frames;
        lastProgress = performance.now();
        attempts = 0;
        if (!playing) { playing = true; setStatus('live'); }
      }
      // A peer connection can remain connected while the camera has frozen.
      if (performance.now() - lastProgress > (playing ? 10_000 : 15_000)) reconnect();
    }, 1000);
    return () => {
      disposed = true;
      document.removeEventListener('visibilitychange', visibilityChanged);
      clearInterval(watchdog);
      clearTimeout(retryTimer);
      release();
    };
  }, [url, revision]);

  return { videoRef, stream, status, reconnect: () => setRevision(value => value + 1) };
}
