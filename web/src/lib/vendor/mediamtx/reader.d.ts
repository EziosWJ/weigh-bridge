export default class MediaMTXWebRTCReader {
  constructor(options: {
    url: string;
    token?: string;
    onError: (error: string) => void;
    onTrack: (event: RTCTrackEvent) => void;
  });
  close(): void;
}
