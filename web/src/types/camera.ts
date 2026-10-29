/** Server-owned camera and MediaMTX contract shared by the management page and workbench. */
export type PasswordAction = "keep" | "set" | "clear";
export type StreamState = "pending" | "synced" | "error" | "removing" | "removed";
export type VideoSyncState = "pending" | "synced" | "error";

export interface CameraStream {
  path: string;
  playbackUrl: string;
  desiredVersion: number;
  syncedVersion: number;
  state: StreamState;
  lastError?: string;
  updatedAt: string;
}

export interface CameraRecord {
  id: number;
  publicId: string;
  name: string;
  sourceUrl: string;
  username: string;
  hasPassword: boolean;
  captureEnabled: boolean;
  enabled: boolean;
  version: number;
  stream: CameraStream;
  createTime: string;
  updateTime: string;
}

export interface CameraInput {
  name: string;
  sourceUrl: string;
  username?: string;
  password?: string;
  passwordAction?: PasswordAction;
  captureEnabled?: boolean;
  enabled?: boolean;
  version?: number;
}

export interface CaptureSettings {
  configVersion: number;
  captureTimeoutMs: number;
  recognitionCameraId?: number | null;
  recognitionUrl?: string;
}

export interface CaptureSettingsInput {
  configVersion: number;
  captureTimeoutMs?: number;
  recognitionCameraId?: number | null;
  recognitionUrl?: string;
  clearRecognitionCamera?: boolean;
}

export interface VideoSettings {
  apiUrl: string;
  apiUsername: string;
  hasApiPassword: boolean;
  whepBaseUrl: string;
  version: number;
  syncState: VideoSyncState;
  syncError?: string;
  syncedVersion: number;
  updateTime: string;
}

export interface VideoSettingsInput {
  apiUrl: string;
  apiUsername?: string;
  apiPassword?: string;
  passwordAction?: PasswordAction;
  whepBaseUrl: string;
  version?: number;
}

export interface CameraList extends CaptureSettings {
  cameras: CameraRecord[];
  video: VideoSettings;
}

export interface CameraTestResult {
  success: boolean;
  code?: string;
  message?: string;
  /** Base64 JPEG. Diagnostic only; never a formal weighing image. */
  jpeg?: string;
  width?: number;
  height?: number;
  at: string;
}

export interface CameraSyncResult {
  configVersion: number;
  videoVersion: number;
  state: VideoSyncState;
  error?: string;
  streams: CameraStream[];
  at: string;
}

export const streamStateLabels: Record<StreamState, string> = {
  pending: "待同步",
  synced: "已同步",
  error: "同步失败",
  removing: "待移除",
  removed: "已移除",
};

export interface CameraEventBinding {
  cameraId: number;
  serialNumber: string;
  enabled: boolean;
  timeoutSeconds: number;
  lastHeartbeatAt?: string | null;
  deviceTime: string;
  deviceIp: string;
}
export interface CameraEventDiagnostic {
  id: number; operator: string; eventId: string; deviceTime: string; receivedAt: string; sha256: string; diagnostic: unknown;
}
export interface CameraEvents {
  binding: CameraEventBinding; state: "unconfigured" | "unknown" | "offline" | "online"; events: CameraEventDiagnostic[];
}
