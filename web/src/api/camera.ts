import { http } from "@/lib/http";
import type {
  CameraInput,
  CameraEvents,
  CameraEventBinding,
  CameraList,
  CameraRecord,
  CameraSyncResult,
  CameraTestResult,
  CaptureSettings,
  CaptureSettingsInput,
  VideoSettings,
  VideoSettingsInput,
} from "@/types/camera";

const base = "/api/weighing";

/** Camera maintenance API. Read endpoints require weighing:camera:view. */
export const cameraApi = {
  events: (id: number) => http.get<CameraEvents>(`${base}/cameras/${id}/events`),
  saveEvents: (id: number, binding: Pick<CameraEventBinding, "serialNumber" | "enabled" | "timeoutSeconds">) => http.put(`${base}/cameras/${id}/events`, binding),
  list: () => http.get<CameraList>(`${base}/cameras`),
  create: (input: CameraInput) => http.post<CameraRecord>(`${base}/cameras`, input),
  update: (id: number, input: CameraInput) => http.put<CameraRecord>(`${base}/cameras/${id}`, input),
  remove: (id: number, version: number) => http.delete(`${base}/cameras/${id}`, { version }),
  settings: () => http.get<CaptureSettings>(`${base}/cameras/settings`),
  saveSettings: (input: CaptureSettingsInput) => http.put<CaptureSettings>(`${base}/cameras/settings`, input),
  video: () => http.get<VideoSettings>(`${base}/cameras/video`),
  saveVideo: (input: VideoSettingsInput) => http.put<VideoSettings>(`${base}/cameras/video`, input),
  sync: (configVersion?: number, videoVersion?: number) =>
    http.post<CameraSyncResult>(`${base}/cameras/sync`, { configVersion, videoVersion }),
  test: (id: number) => http.post<CameraTestResult>(`${base}/cameras/${id}/test`),
  preview: (id: number) => http.get<CameraRecord["stream"]>(`${base}/cameras/${id}/preview`),
};
