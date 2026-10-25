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

  return <FormDialog open={open} title={camera ? "编辑摄像头" : "新增摄像头"} loading={loading} submitText={camera ? "保存摄像头" : "新增摄像头"} onCancel={onCancel} onSubmit={submit}
    description="源地址与凭据保存在 Server；保存后立即用于后续抓拍，无需 Agent 重载。历史图片保留当时名称和来源。">
    <div className="space-y-4">
      <Field label="名称" htmlFor="camera-name" required help="用于工作台预览、核对与历史图片显示，可随时修改。">
        <Input id="camera-name" value={name} maxLength={100} autoFocus onChange={event => setName(event.target.value)} placeholder="例如：西门入口" />
      </Field>
      <Field label="完整源地址" htmlFor="camera-source" required help="包含主机、端口和流路径；不要把用户名密码写进地址。">
        <Input id="camera-source" value={sourceUrl} onChange={event => setSourceUrl(event.target.value)} placeholder="rtsp://192.168.1.20:554/stream1" autoComplete="off" spellCheck={false} />
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="用户名" htmlFor="camera-username" help="摄像头未启用鉴权时留空。">
          <Input id="camera-username" value={username} maxLength={200} autoComplete="off" onChange={event => setUsername(event.target.value)} />
        </Field>
        <Field label="密码" htmlFor="camera-password" help={camera?.hasPassword ? "已保存密码；留空保持原值。" : "未保存密码；留空表示无密码。"}>
          <Input id="camera-password" type="password" value={password} disabled={clearPassword} autoComplete="new-password" onChange={event => setPassword(event.target.value)} placeholder={camera?.hasPassword ? "留空保持原密码" : "无密码可留空"} />
        </Field>
      </div>
      {camera?.hasPassword && <label className="flex items-center gap-2 text-sm"><Checkbox checked={clearPassword} onChange={event => { setClearPassword(event.target.checked); if (event.target.checked) setPassword(""); }} />清除已保存密码（仅明确勾选时清除）</label>}
      <div className="space-y-2 rounded-control border border-border bg-neutral-background p-3">
        <label className="flex items-start gap-2 text-sm"><Checkbox className="mt-0.5" checked={captureEnabled} onChange={event => setCaptureEnabled(event.target.checked)} /><span>用于称重抓拍<br /><span className="text-text-tertiary">未勾选的摄像头仅用于实时预览，不参与正式称重证据。</span></span></label>
        <label className="flex items-start gap-2 text-sm"><Checkbox className="mt-0.5" checked={enabled} onChange={event => setEnabled(event.target.checked)} /><span>启用该摄像头<br /><span className="text-text-tertiary">停用后停止预览和抓拍，历史图片仍可查询。</span></span></label>
      </div>
      {(localError || error) && <p role="alert" className="text-sm text-error">{localError || error}</p>}
    </div>
  </FormDialog>;
}

export function CameraDeleteDialog({ camera, loading, error, onCancel, onConfirm }: { camera: CameraRecord | null; loading: boolean; error: string; onCancel: () => void; onConfirm: () => void }) {
  return <ConfirmDialog open={!!camera} danger title="删除摄像头" confirmText="确认删除" loading={loading} onCancel={onCancel} onConfirm={onConfirm}
    description={camera ? <>删除“{camera.name}”后不再参与预览和抓拍；历史称重图片保留当时名称与来源。若该摄像头是识别源，识别选择会同时清除并停用识别。<br />当前配置版本：{camera.version}。{error && <span role="alert" className="mt-2 block text-error">{error}</span>}</> : ""} />;
}

export function CameraErrorBanner({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-control border border-error-border bg-error-background px-4 py-3 text-sm text-error"><span>{message}</span><Button size="sm" variant="secondary" onClick={onRetry}>重试</Button></div>;
}
