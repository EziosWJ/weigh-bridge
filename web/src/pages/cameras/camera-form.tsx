import { useState } from "react";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import type { CameraInput, CameraRecord } from "@/types/camera";

type CameraFormProps = {
  camera?: CameraRecord | null;
  open: boolean;
  loading: boolean;
  error: string;
  onCancel: () => void;
  onSubmit: (input: CameraInput) => void | Promise<void>;
};

/** Create/edit one camera. A blank password keeps the stored secret. */
export function CameraFormDialog({ camera, open, loading, error, onCancel, onSubmit }: CameraFormProps) {
  const [name, setName] = useState(camera?.name ?? "");
  const [sourceUrl, setSourceUrl] = useState(camera?.sourceUrl ?? "");
  const [username, setUsername] = useState(camera?.username ?? "");
  const [password, setPassword] = useState("");
  const [clearPassword, setClearPassword] = useState(false);
  const [captureEnabled, setCaptureEnabled] = useState(camera?.captureEnabled ?? false);
  const [enabled, setEnabled] = useState(camera?.enabled ?? true);
  const [localError, setLocalError] = useState("");

  async function submit() {
    const trimmedName = name.trim();
    const trimmedSource = sourceUrl.trim();
    if (!trimmedName) { setLocalError("请填写摄像头名称。"); return; }
    if (!trimmedSource) { setLocalError("请填写完整源地址，例如 rtsp://192.168.1.20:554/stream1。"); return; }
    if (password && !username.trim()) { setLocalError("填写密码时必须同时填写用户名。"); return; }
    setLocalError("");
    const input: CameraInput = {
      name: trimmedName,
      sourceUrl: trimmedSource,
      username: username.trim(),
      captureEnabled,
      enabled,
    };
    if (clearPassword) input.passwordAction = "clear";
    else if (password) { input.password = password; input.passwordAction = "set"; }
    else input.passwordAction = "keep";
    if (camera) input.version = camera.version;
    await onSubmit(input);
  }

  return <FormDialog open={open} title={camera ? "编辑摄像头" : "新增摄像头"} loading={loading} submitText={camera ? "保存摄像头" : "新增摄像头"} onCancel={onCancel} onSubmit={submit}>
    <div className="space-y-4">
      <Field label="名称" htmlFor="camera-name" required>
        <Input id="camera-name" value={name} maxLength={100} autoFocus onChange={event => setName(event.target.value)} placeholder="例如：西门入口" />
      </Field>
      <Field label="完整源地址" htmlFor="camera-source" required hint="包含主机、端口和流路径；不要把用户名密码写进地址。">
        <Input id="camera-source" value={sourceUrl} onChange={event => setSourceUrl(event.target.value)} placeholder="rtsp://192.168.1.20:554/stream1" autoComplete="off" spellCheck={false} />
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="用户名" htmlFor="camera-username">
          <Input id="camera-username" value={username} maxLength={200} autoComplete="off" onChange={event => setUsername(event.target.value)} />
        </Field>
        <Field label="密码" htmlFor="camera-password" hint={camera?.hasPassword ? "已保存密码；留空保持原值。" : "未保存密码；留空表示无密码。"}>
          <Input id="camera-password" type="password" value={password} disabled={clearPassword} autoComplete="new-password" onChange={event => setPassword(event.target.value)} placeholder={camera?.hasPassword ? "留空保持原密码" : "无密码可留空"} />
        </Field>
      </div>
      {camera?.hasPassword && <label className="flex items-center gap-2 text-sm"><Checkbox checked={clearPassword} onChange={event => { setClearPassword(event.target.checked); if (event.target.checked) setPassword(""); }} />清除已保存密码</label>}
      <div className="space-y-2 rounded-control border border-border bg-neutral-background p-3">
        <label className="flex items-start gap-2 text-sm"><Checkbox className="mt-0.5" checked={captureEnabled} onChange={event => setCaptureEnabled(event.target.checked)} />用于称重抓拍</label>
        <label className="flex items-start gap-2 text-sm"><Checkbox className="mt-0.5" checked={enabled} onChange={event => setEnabled(event.target.checked)} />启用摄像头</label>
      </div>
      {(localError || error) && <p role="alert" className="text-sm text-error">{localError || error}</p>}
    </div>
  </FormDialog>;
}

export function CameraDeleteDialog({ camera, recognitionSource, recognitionActive, remainingCaptureCount, loading, error, onCancel, onConfirm }: { camera: CameraRecord | null; recognitionSource: boolean; recognitionActive: boolean; remainingCaptureCount: number; loading: boolean; error: string; onCancel: () => void; onConfirm: () => void }) {
  return <ConfirmDialog open={!!camera} danger title="删除摄像头" confirmText="确认删除" loading={loading} onCancel={onCancel} onConfirm={onConfirm}
    description={camera ? <>
      <span className="block break-words font-medium">{camera.name} · {camera.enabled ? camera.captureEnabled ? "称重抓拍及预览" : "仅预览" : "已停用"}{recognitionSource && " · 车牌识别源"}</span>
      <span className="mt-2 block">{camera.enabled ? camera.captureEnabled
        ? `删除后从预览和称重抓拍中移除。剩余 ${remainingCaptureCount} 台称重抓拍摄像头。`
        : "删除后从视频预览中移除。"
        : "删除后移除此摄像头配置。"}{recognitionSource && (recognitionActive ? "将清除识别源并停用车牌识别。" : "将清除已选识别源。")}</span>
      {error && <span role="alert" className="mt-2 block text-error">{error}</span>}
    </> : ""} />;

}

export function CameraErrorBanner({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-control border border-error-border bg-error-background px-4 py-3 text-sm text-error"><span>{message}</span><Button size="sm" variant="secondary" onClick={onRetry}>重试</Button></div>;
}
