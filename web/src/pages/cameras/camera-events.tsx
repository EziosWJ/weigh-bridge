import { useEffect, useState } from "react";
import { cameraApi } from "@/api/camera";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { CameraEvents, CameraRecord } from "@/types/camera";

export function CameraEventDialog({ camera, canManage, onClose }: { camera: CameraRecord; canManage: boolean; onClose: () => void }) {
  const [data, setData] = useState<CameraEvents | null>(null);
  const [serial, setSerial] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [timeout, setTimeout] = useState("180");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function load() {
    try {
      const result = await cameraApi.events(camera.id);
      setData(result); setSerial(result.binding.serialNumber); setEnabled(result.binding.enabled); setTimeout(String(result.binding.timeoutSeconds)); setError("");
    } catch (failure) { setData(null); setError(getErrorMessage(failure, "上报状态读取失败")); }
  }
  useEffect(() => { void load(); }, [camera.id]); // eslint-disable-line react-hooks/exhaustive-deps
  async function save() {
    const seconds = Number(timeout);
    if (!Number.isInteger(seconds) || seconds < 10 || seconds > 86400 || (enabled && !serial.trim())) { setError("请填写序列号和 10–86400 秒的离线阈值。"); return; }
    setBusy(true);
    try { await cameraApi.saveEvents(camera.id, { serialNumber: serial.trim(), enabled, timeoutSeconds: seconds }); await load(); }
    catch (failure) { setError(getErrorMessage(failure, "上报配置保存失败")); }
    finally { setBusy(false); }
  }
  const labels = { unconfigured: "未配置", unknown: "未知", offline: "离线", online: "近期收到心跳" };
  return <FormDialog open title={`${camera.name} · 设备上报`} description="仅供可信设备网络访问；事件诊断不参与正式称重。" loading={busy} submitText={canManage && data ? "保存" : "关闭"} onCancel={onClose} onSubmit={canManage && data ? save : onClose}>
    {error && <p role="alert" className="text-sm text-error">{error}</p>}
    {data && <div className="space-y-4">
      <Field label="设备序列号" htmlFor="event-serial" help="与厂家设备序列号精确匹配；每台设备只能绑定一台摄像头。"><Input id="event-serial" value={serial} maxLength={200} disabled={!canManage} onChange={e => setSerial(e.target.value)} /></Field>
      <Field label="离线阈值（秒）" htmlFor="event-timeout" help="根据实际心跳周期调整，建议约为周期的三倍。"><Input id="event-timeout" type="number" value={timeout} min={10} max={86400} disabled={!canManage} onChange={e => setTimeout(e.target.value)} /></Field>
      <label className="flex gap-2 text-sm"><Checkbox checked={enabled} disabled={!canManage} onChange={e => setEnabled(e.target.checked)} />启用主动上报</label>
      <p className="text-sm">状态：{labels[data.state]}；最近心跳：{data.binding.lastHeartbeatAt ? formatDateTime(data.binding.lastHeartbeatAt) : "无"}</p>
      <p className="text-xs">设备时间：{data.binding.deviceTime || "无"}；自报 IP：{data.binding.deviceIp || "无"}（仅供诊断）</p>
      <Button size="sm" onClick={() => void load()}>刷新状态</Button>
      <div className="max-h-60 overflow-auto text-xs"><strong>最近诊断（最多 100 条）</strong>{data.events?.map(event => <div key={event.id} className="mt-2 break-all border-t border-border pt-2">{formatDateTime(event.receivedAt)} · {event.operator} · 事件号 {event.eventId}<pre className="whitespace-pre-wrap">{JSON.stringify(event.diagnostic, null, 2)}</pre></div>)}</div>
    </div>}
  </FormDialog>;
}
