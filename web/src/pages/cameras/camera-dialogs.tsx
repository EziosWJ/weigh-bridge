import { useEffect, useRef, useState } from "react";
import { AlertTriangle, CheckCircle2, RefreshCw, VideoOff, X } from "lucide-react";
import { cameraApi } from "@/api/camera";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogOverlay, DialogTitle } from "@/components/ui/dialog";
import { HelpHint } from "@/components/common/help-hint";
import { StatusTag } from "@/components/common/status-tag";
import { getErrorMessage } from "@/lib/api-error";
import { beijingTime } from "@/lib/weighing/query";
import { CameraLive } from "@/pages/weighing/camera-live";
import { streamStateLabels, type CameraRecord, type CameraStream, type CameraTestResult } from "@/types/camera";

export type TestRecord = { at: string; cameraVersion?: number; success: boolean; code?: string; message?: string };

/** Diagnostic capture. The image never becomes a weighing sample or evidence row. */
export function CameraTestDialog({ camera, onClose, onResult }: { camera: CameraRecord; onClose: () => void; onResult: (record: TestRecord) => void }) {
  const [result, setResult] = useState<CameraTestResult | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  const resultRef = useRef(onResult);
  resultRef.current = onResult;

  useEffect(() => {
    let active = true;
    setLoading(true); setResult(null); setError("");
    void cameraApi.test(camera.id).then(value => {
      if (!active) return;
      setResult(value);
      resultRef.current({ at: value.at || new Date().toISOString(), cameraVersion: value.cameraVersion, success: value.success, code: value.code, message: value.message });
    }).catch(failure => {
      if (!active) return;
      const message = getErrorMessage(failure, "测试抓拍失败");
      setError(message);
      resultRef.current({ at: new Date().toISOString(), success: false, message });
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [camera.id, attempt]);

  return <Dialog open onOpenChange={next => { if (!next) onClose(); }} trapFocus restoreFocus lockScroll>
    <DialogOverlay />
    <DialogContent className="flex max-w-[900px] flex-col">
      <DialogHeader><div className="flex items-center gap-1"><DialogTitle>测试抓拍 · {camera.name}</DialogTitle><HelpHint label="测试抓拍说明">测试图片仅用于维护诊断。</HelpHint></div><DialogClose aria-label="关闭测试抓拍"><X className="h-4 w-4" aria-hidden /></DialogClose></DialogHeader>
      <DialogBody className="space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <StatusTag tone={loading ? "warning" : result?.success ? "success" : "error"}>{loading ? "正在取图" : result?.success ? "测试成功" : "测试失败"}</StatusTag>
          {result && <span className="text-sm text-text-secondary">{beijingTime(result.at)} · {result.cameraVersion === undefined ? "配置版本未确认" : `配置版本 v${result.cameraVersion}`}</span>}
        </div>
        {!loading && result?.success && result.jpeg && <figure className="space-y-2"><img className="max-h-[55dvh] w-full rounded-control border border-border bg-neutral-background object-contain" alt="摄像头测试抓拍图片" src={`data:image/jpeg;base64,${result.jpeg}`} /><figcaption className="text-xs text-text-tertiary">测试图片 {result.width && result.height ? `${result.width}×${result.height}` : ""}</figcaption></figure>}
        {!loading && result?.success && !result.jpeg && <p role="status" className="text-sm text-text-secondary">测试成功，但服务端未返回预览图片。</p>}
        {!loading && (error || (result && !result.success)) && <div role="alert" className="flex items-start gap-2 rounded-control border border-error-border bg-error-background p-3 text-sm text-error"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />{error || `${result?.code ? `${result.code}：` : ""}${result?.message || "未能取得有效图片"}`}</div>}
      </DialogBody>
      <DialogFooter><Button onClick={onClose}>关闭</Button><Button variant="primary" disabled={loading} onClick={() => setAttempt(value => value + 1)}><RefreshCw className="h-4 w-4" aria-hidden />重新测试</Button></DialogFooter>
    </DialogContent>
  </Dialog>;
}

function streamMessage(camera: CameraRecord, stream: CameraStream | null, loading: boolean, error: string) {
  if (error) return error;
  if (loading && !stream) return "正在读取视频预览地址…";
  if (!camera.enabled) return "该摄像头已停用，不能预览；启用后可重新读取。";
  if (!stream) return "尚未取得视频预览地址。";
  if (stream.state === "synced" && stream.playbackUrl) return "";
  if (stream.state === "error") return `视频映射同步失败${stream.lastError ? `：${stream.lastError}` : ""}`;
  if (stream.state === "removing" || stream.state === "removed") return "视频映射待移除；当前不能预览。";
  return `视频映射待同步${stream.lastError ? `：${stream.lastError}` : "，同步完成后可预览"}`;
}

/** Live preview only. A failure here never blocks direct capture or tests. */
export function CameraPreviewDialog({ camera, onTest, onClose, onSync }: { camera: CameraRecord; onTest?: () => void; onClose: () => void; onSync?: () => Promise<void> }) {
  const [stream, setStream] = useState<CameraStream | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [syncing, setSyncing] = useState(false);
  useEffect(() => {
    let active = true;
    setLoading(true); setError(""); setStream(null);
    void cameraApi.preview(camera.id).then(value => { if (active) setStream(value); })
      .catch(failure => { if (active) setError(getErrorMessage(failure, "无法读取视频预览")); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [camera.id, attempt]);
  const message = streamMessage(camera, stream, loading, error);
  const playbackURL = !error && camera.enabled && stream?.state === "synced" ? stream.playbackUrl : "";
  async function sync() {
    if (!onSync) return;
    setSyncing(true); setError("");
    try { await onSync(); setAttempt(value => value + 1); }
    catch (failure) { setError(getErrorMessage(failure, "同步视频源失败")); }
    finally { setSyncing(false); }
  }
  return <Dialog open onOpenChange={next => { if (!next) onClose(); }} trapFocus restoreFocus lockScroll>
    <DialogOverlay />
    <DialogContent className="flex max-w-[1100px] flex-col">
      <DialogHeader><DialogTitle>{camera.name} · 实时预览</DialogTitle><DialogClose aria-label="关闭实时预览"><X className="h-4 w-4" aria-hidden /></DialogClose></DialogHeader>
      <DialogBody className="space-y-3">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <StatusTag tone={loading ? "neutral" : playbackURL ? "neutral" : stream?.state === "error" ? "error" : "warning"}>{loading ? "读取中" : !camera.enabled ? "已停用" : playbackURL ? "映射已同步" : stream ? streamStateLabels[stream.state] : "未知"}</StatusTag>
          {stream && <span className="text-text-secondary">已同步 v{stream.syncedVersion} / 目标 v{stream.desiredVersion}</span>}
        </div>
        {playbackURL ? <CameraLive key={`${camera.id}:${stream?.syncedVersion}`} url={playbackURL} title={camera.name} /> : <div className="space-y-3 rounded-control border border-border bg-neutral-background p-6 text-center text-sm text-text-secondary"><VideoOff className="mx-auto h-10 w-10 text-text-tertiary" aria-hidden /><p role={error || stream?.state === "error" ? "alert" : undefined} className="font-medium text-text-primary">{message || "视频预览不可用"}</p></div>}
      </DialogBody>
      <DialogFooter><Button onClick={onClose}>关闭</Button>{onTest && <Button disabled={syncing} onClick={onTest}>测试抓拍</Button>}{onSync && <Button disabled={syncing || loading} onClick={() => void sync()}>{syncing ? "正在同步…" : "同步视频源"}</Button>}<Button variant="primary" disabled={loading} onClick={() => setAttempt(value => value + 1)}><RefreshCw className="h-4 w-4" aria-hidden />重新读取预览</Button></DialogFooter>
    </DialogContent>
  </Dialog>;
}

export function TestSummary({ record, version }: { record?: TestRecord; version: number }) {
  if (!record) return <span className="text-text-tertiary">未测试</span>;
  return <span className="inline-flex flex-wrap items-center gap-2"><span className={record.success ? "inline-flex items-center gap-1 text-success" : "inline-flex items-center gap-1 text-error"}>{record.success ? <CheckCircle2 className="h-4 w-4" aria-hidden /> : <AlertTriangle className="h-4 w-4" aria-hidden />}{record.success ? "成功" : "失败"}</span><span className="text-xs text-text-tertiary">{beijingTime(record.at)} · {record.cameraVersion === undefined ? "版本未知" : `v${record.cameraVersion}`}{record.cameraVersion !== undefined && record.cameraVersion !== version && "（旧配置）"}</span>{!record.success && (record.message || record.code) && <span className="max-w-[220px] truncate text-xs text-error" title={record.message || record.code}>{record.message || record.code}</span>}</span>;
}
