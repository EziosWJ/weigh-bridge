import { useCallback, useEffect, useRef, useState } from "react";
import { Plus, RefreshCw, Search, Trash2, Video } from "lucide-react";
import { cameraApi } from "@/api/camera";
import { weighingApi } from "@/api/weighing";
import { ContentCard } from "@/components/common/content-card";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { Field } from "@/components/common/field";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { TableActions } from "@/components/common/table-actions";
import { TableToolbar } from "@/components/common/table-toolbar";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import { CameraEventDialog } from "./camera-events";
import { CameraDeleteDialog, CameraErrorBanner, CameraFormDialog } from "./camera-form";
import { CameraPreviewDialog, CameraTestDialog, TestSummary, type TestRecord } from "./camera-dialogs";
import { streamStateLabels, type CameraInput, type CameraList, type CameraRecord, type VideoSyncState } from "@/types/camera";
import type { DataTableColumn } from "@/types";

const TEST_STORAGE_KEY = "weighbridge-camera-test-results-v1";

function readTests(): Record<string, TestRecord> {
  try {
    const parsed = JSON.parse(sessionStorage.getItem(TEST_STORAGE_KEY) || "{}") as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    const result: Record<string, TestRecord> = {};
    for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
      const row = value as Partial<TestRecord> | null;
      if (row && typeof row.at === "string" && typeof row.success === "boolean") result[key] = row as TestRecord;
    }
    return result;
  } catch { return {}; }
}

function syncTone(state: VideoSyncState) {
  return state === "synced" ? "success" : state === "error" ? "error" : "warning";
}

/** 0..N independent cameras owned by Server; no fixed front/rear meaning. */
export function CamerasPage() {
  const [permissions, setPermissions] = useState<string[] | null>(null);
  const [permissionError, setPermissionError] = useState("");
  const [list, setList] = useState<CameraList | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [tests, setTests] = useState<Record<string, TestRecord>>(() => readTests());
  const listRef = useRef<CameraList | null>(null);

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<CameraRecord | null>(null);
  const [formBusy, setFormBusy] = useState(false);
  const [formError, setFormError] = useState("");
  const [deleting, setDeleting] = useState<CameraRecord | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const [testing, setTesting] = useState<CameraRecord | null>(null);
  const [eventCamera, setEventCamera] = useState<CameraRecord | null>(null);
  const [previewing, setPreviewing] = useState<CameraRecord | null>(null);

  const canView = permissions?.includes("weighing:camera:view") ?? false;
  const canManage = permissions?.includes("weighing:camera:manage") ?? false;

  useEffect(() => {
    let active = true;
    void weighingApi.capabilities().then(value => { if (active) setPermissions(value.permissionCodes); })
      .catch(failure => { if (active) { setPermissions([]); setPermissionError(getErrorMessage(failure, "无法读取账号权限")); } });
    return () => { active = false; };
  }, []);

  const applyList = useCallback((next: CameraList, announce: boolean) => {
    const previous = listRef.current;
    if (announce && previous?.recognitionCameraId && !next.recognitionCameraId) {
      setNotice("识别源已停用：原选定的摄像头已删除、停用或不再用于称重抓拍。请重新选择识别源或保持识别关闭。");
    }
    listRef.current = next;
    setList(next);
  }, []);

  const load = useCallback(async (announce = false) => {
    setLoading(true);
    try {
      const next = await cameraApi.list();
      applyList(next, announce);
      setError("");
    } catch (failure) {
      setError(getErrorMessage(failure, "摄像头配置加载失败"));
    } finally { setLoading(false); }
  }, [applyList]);

  useEffect(() => { if (canView) void load(false); }, [canView, load]);

  function recordTest(camera: CameraRecord, record: TestRecord) {
    setTests(current => {
      const next = { ...current, [camera.id]: record };
      try { sessionStorage.setItem(TEST_STORAGE_KEY, JSON.stringify(next)); } catch { /* Per-viewer diagnostic only. */ }
      return next;
    });
  }

  async function saveCamera(input: CameraInput) {
    setFormBusy(true); setFormError("");
    try {
      if (editing) await cameraApi.update(editing.id, input);
      else await cameraApi.create(input);
      setFormOpen(false); setEditing(null);
      await load(true);
      toast.success(editing ? "摄像头已保存，后续抓拍使用新配置。" : "摄像头已新增，可测试抓拍或查看预览。");
    } catch (failure) {
      setFormError(getErrorMessage(failure, "摄像头保存失败"));
    } finally { setFormBusy(false); }
  }

  async function deleteCamera() {
    if (!deleting) return;
    setDeleteBusy(true); setDeleteError("");
    try {
      await cameraApi.remove(deleting.id, deleting.version);
      setDeleting(null);
      await load(true);
      toast.success("摄像头已删除；历史图片保留当时名称与来源。");
    } catch (failure) {
      setDeleteError(getErrorMessage(failure, "删除摄像头失败"));
    } finally { setDeleteBusy(false); }
  }

  async function syncVideo() {
    const current = listRef.current;
    if (!current) throw new Error("摄像头配置尚未加载");
    const result = await cameraApi.sync(current.configVersion, current.video.version);
    await load(false);
    if (result.state !== "synced") throw new Error(result.error || "视频源同步未完成，请稍后重试");
  }

  const captureCameras = list?.cameras.filter(camera => camera.captureEnabled && camera.enabled) ?? [];
  const recognitionCamera = list?.cameras.find(camera => camera.id === list.recognitionCameraId);
  const columns: DataTableColumn<CameraRecord>[] = [
    { title: "名称 / 标识", key: "name", width: 180, render: (_, row) => <div><strong className="font-medium">{row.name}</strong><div className="mt-1 break-all text-xs text-text-tertiary">{row.publicId}</div></div> },
    { title: "用途与状态", key: "purpose", width: 150, render: (_, row) => <div className="flex flex-wrap gap-1"><StatusTag tone={row.enabled ? "success" : "neutral"}>{row.enabled ? "已启用" : "已停用"}</StatusTag><StatusTag tone={row.captureEnabled ? "info" : "neutral"}>{row.captureEnabled ? "称重抓拍" : "仅预览"}</StatusTag></div> },
    { title: "源地址 / 凭据", key: "source", width: 280, render: (_, row) => <div className="space-y-1 text-xs"><span className="break-all text-text-secondary">{row.sourceUrl || "未填写源地址"}</span><span className="block text-text-tertiary">{row.username ? `用户名 ${row.username} · ` : "无用户名 · "}{row.hasPassword ? "已保存密码" : "未保存密码"}</span></div> },
    { title: "视频映射", key: "stream", width: 200, render: (_, row) => <div className="space-y-1"><StatusTag tone={row.stream.state === "synced" ? "success" : row.stream.state === "error" ? "error" : "warning"}>{streamStateLabels[row.stream.state]}</StatusTag>{row.stream.playbackUrl && <span className="block break-all text-xs text-text-tertiary">预览地址已生成</span>}{row.stream.lastError && <span className="block break-all text-xs text-error">{row.stream.lastError}</span>}</div> },
    { title: "最近测试", key: "test", width: 190, render: (_, row) => <TestSummary record={tests[row.id]} /> },
    { title: "配置版本", key: "version", width: 90, align: "center", render: (_, row) => <span className="tabular-nums">{row.version}</span> },
    { title: "操作", key: "actions", width: canManage ? 240 : 90, nowrap: true, render: (_, row) => <TableActions>
      {canManage && <Button size="sm" variant="ghost" onClick={() => setEventCamera(row)}>上报</Button>}
      <Button size="sm" variant="ghost" onClick={() => setPreviewing(row)}><Video className="h-4 w-4" aria-hidden />预览</Button>
      {canManage && <Button size="sm" variant="ghost" onClick={() => setTesting(row)}><Search className="h-4 w-4" aria-hidden />测试</Button>}
      {canManage && <Button size="sm" variant="ghost" onClick={() => { setEditing(row); setFormError(""); setFormOpen(true); }}>编辑</Button>}
      {canManage && <Button size="sm" variant="ghost" className="text-error" onClick={() => { setDeleteError(""); setDeleting(row); }}><Trash2 className="h-4 w-4" aria-hidden />删除</Button>}
    </TableActions> },
  ];

  if (permissions === null) return <><PageHeader title="摄像头管理" /><p role="status" className="text-sm text-text-secondary">{permissionError || "正在读取账号权限…"}</p></>;
  if (!canView) return <><PageHeader title="摄像头管理" />{permissionError ? <p role="alert" className="rounded-control border border-error-border bg-error-background px-4 py-3 text-sm text-error">无法读取账号权限：{permissionError}。请刷新页面重试。</p> : <EmptyState title="无摄像头查看权限" description="当前账号没有 weighing:camera:view 权限，请联系本厂管理员分配。正式称重采集由 Server 执行，不受此页面权限影响。" />}</>;

  return <div className="space-y-5">
    <PageHeader title="摄像头管理" description="Server 直接连接摄像头预览、测试与正式抓拍；数量与名称可变，支持 0、1 或多台，不使用车头/车尾固定含义。"
      actions={<><Button onClick={() => void load(false)} disabled={loading}><RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} aria-hidden />刷新</Button>{canManage && <Button variant="primary" onClick={() => { setEditing(null); setFormError(""); setFormOpen(true); }}><Plus className="h-4 w-4" aria-hidden />新增摄像头</Button>}</>} />

    {!canManage && <p role="status" className="rounded-control border border-border bg-neutral-background px-4 py-2 text-sm text-text-secondary">当前账号仅可查看摄像头配置和实时预览；测试、编辑、删除与同步需要 weighing:camera:manage 权限。</p>}
    {notice && <p role="status" className="rounded-control border border-warning-border bg-warning-background px-4 py-2 text-sm text-warning">{notice}</p>}
    {error && <CameraErrorBanner message={error} onRetry={() => { void load(false); }} />}

    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <ContentCard bodyClassName="p-4"><p className="text-xs text-text-tertiary">摄像头数量</p><p className="mt-1 text-2xl font-semibold tabular-nums">{list?.cameras.length ?? "—"}<span className="ml-1 text-sm font-normal text-text-secondary">台</span></p><p className="mt-1 text-xs text-text-tertiary">{list?.cameras.filter(camera => camera.enabled).length ?? 0} 台启用 · {captureCameras.length} 台用于称重抓拍</p></ContentCard>
      <ContentCard bodyClassName="p-4"><p className="text-xs text-text-tertiary">抓拍配置版本</p><p className="mt-1 text-2xl font-semibold tabular-nums">{list?.configVersion ?? "—"}</p><p className="mt-1 text-xs text-text-tertiary">保存摄像头或抓拍设置后递增；样本确认时核对冻结版本。</p></ContentCard>
      <ContentCard bodyClassName="p-4"><p className="text-xs text-text-tertiary">抓拍超时</p><p className="mt-1 text-2xl font-semibold tabular-nums">{list ? (list.captureTimeoutMs / 1000).toFixed(1) : "—"}<span className="ml-1 text-sm font-normal text-text-secondary">秒</span></p><p className="mt-1 text-xs text-text-tertiary">整次并行取图的统一截止时间，不逐台累加。</p></ContentCard>
      <ContentCard bodyClassName="p-4"><p className="text-xs text-text-tertiary">车牌识别</p><p className="mt-1 truncate text-lg font-semibold">{list?.recognitionUrl ? recognitionCamera?.name || "已配置" : "未启用"}</p><p className="mt-1 text-xs text-text-tertiary">{list?.recognitionUrl ? "复用指定抓拍摄像头的本次图片，失败不阻断称重。" : "识别接口地址留空即停用；不额外抓图。"}</p></ContentCard>
    </div>

    <DataTableCard toolbar={<TableToolbar title="摄像头列表" description="测试抓拍与实时预览分别反馈；最近测试结果仅用于本浏览器诊断记录。" />}>
      <DataTable ariaLabel="摄像头列表" columns={columns} dataSource={list?.cameras ?? []} rowKey="id" loading={loading && !list} error={error && !list ? error : undefined} minWidth={1080}
        empty={<EmptyState title="尚未配置摄像头" description={canManage ? "新增摄像头后即可预览和测试；正式称重抓拍使用标记为称重抓拍的摄像头。" : "当前没有摄像头配置；请联系管理员在摄像头管理中新增。"} actionText={canManage ? "新增摄像头" : undefined} onAction={canManage ? () => { setEditing(null); setFormError(""); setFormOpen(true); } : undefined} />} />
    </DataTableCard>

    {list && <div className="grid gap-5 xl:grid-cols-2">
      <CaptureSettingsCard list={list} cameras={captureCameras} canManage={canManage} onSaved={next => { applyList({ ...list, ...next, cameras: list.cameras }, false); }} />
      <VideoSettingsCard list={list} canManage={canManage} onSaved={video => applyList({ ...list, video }, false)} onSync={syncVideo} />
    </div>}
    {list && <p className="text-xs text-text-tertiary">视频映射同步与直连抓拍分别反馈：摄像头保存成功不会因 MediaMTX 离线回滚；预览不可用不代表抓拍不可用。测试抓拍、预览或同步都不会创建正式称重样本。{list.video.updateTime && !list.video.updateTime.startsWith('0001') ? ` 视频配置更新于 ${formatDateTime(list.video.updateTime)}。` : ''}</p>}

    {formOpen && <CameraFormDialog key={editing?.id ?? "new"} camera={editing} open loading={formBusy} error={formError} onCancel={() => { setFormOpen(false); setEditing(null); }} onSubmit={saveCamera} />}
    <CameraDeleteDialog camera={deleting} loading={deleteBusy} error={deleteError} onCancel={() => setDeleting(null)} onConfirm={() => void deleteCamera()} />
    {eventCamera && <CameraEventDialog camera={eventCamera} canManage={canManage} onClose={() => setEventCamera(null)} />}
    {testing && <CameraTestDialog key={`test-${testing.id}`} camera={testing} onClose={() => { setTesting(null); void load(false); }} onResult={record => recordTest(testing, record)} />}
    {previewing && <CameraPreviewDialog key={`preview-${previewing.id}`} camera={previewing} canManage={canManage} onClose={() => setPreviewing(null)} onSync={async () => { await syncVideo(); toast.success("视频源已同步。"); }} />}
  </div>;
}

function CaptureSettingsCard({ list, cameras, canManage, onSaved }: { list: CameraList; cameras: CameraRecord[]; canManage: boolean; onSaved: (next: { configVersion: number; captureTimeoutMs: number; recognitionCameraId: number | null; recognitionUrl: string }) => void }) {
  const [timeout, setTimeoutMs] = useState(String(list.captureTimeoutMs));
  const [recognitionUrl, setRecognitionUrl] = useState(list.recognitionUrl ?? "");
  const [recognitionId, setRecognitionId] = useState(list.recognitionCameraId ? String(list.recognitionCameraId) : "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  // Reflect server-side clearing (deleted/disabled recognition source) without
  // discarding edits made while the page stays open.
  useEffect(() => {
    setRecognitionId(list.recognitionCameraId ? String(list.recognitionCameraId) : "");
    setRecognitionUrl(list.recognitionUrl ?? "");
    setTimeoutMs(String(list.captureTimeoutMs));
  }, [list.recognitionCameraId, list.recognitionUrl, list.captureTimeoutMs]);

  async function save() {
    const timeoutMs = Number(timeout);
    if (!Number.isFinite(timeoutMs) || timeoutMs < 500 || timeoutMs > 60_000) { setError("抓拍超时须在 500 到 60000 毫秒之间。"); return; }
    setBusy(true); setError("");
    try {
      const saved = await cameraApi.saveSettings({
        configVersion: list.configVersion,
        captureTimeoutMs: timeoutMs,
        recognitionUrl: recognitionUrl.trim(),
        recognitionCameraId: recognitionId ? Number(recognitionId) : undefined,
        clearRecognitionCamera: !recognitionId && !!list.recognitionCameraId,
      });
      onSaved({ configVersion: saved.configVersion, captureTimeoutMs: saved.captureTimeoutMs, recognitionCameraId: saved.recognitionCameraId ?? null, recognitionUrl: saved.recognitionUrl ?? "" });
      toast.success({ title: "抓拍设置已保存", description: "新配置立即用于后续采集；进行中的未确认样本需重新采集。", duration: 5000 });
    } catch (failure) { setError(getErrorMessage(failure, "抓拍设置保存失败")); }
    finally { setBusy(false); }
  }

  return <ContentCard title="抓拍设置" description="统一超时、识别接口与识别源；超时为整次并行取图的统一截止时间。" extra={<StatusTag tone="info">{`配置版本 ${list.configVersion}`}</StatusTag>}>
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="抓拍超时（毫秒）" htmlFor="camera-timeout" help="默认 6000；500–60000 毫秒。多台摄像头并行取图共用一个截止时间。">
          <Input id="camera-timeout" type="number" min={500} max={60000} step={100} value={timeout} disabled={!canManage} onChange={event => setTimeoutMs(event.target.value)} />
        </Field>
        <Field label="识别源摄像头" htmlFor="camera-recognition-source" help="识别复用该摄像头本次抓拍图片，不额外抓图；留空即停用识别源。">
          <Select id="camera-recognition-source" value={recognitionId} disabled={!canManage} onChange={event => setRecognitionId(event.target.value)}>
            <option value="">不指定（识别停用）</option>
            {cameras.map(camera => <option key={camera.id} value={camera.id}>{camera.name}</option>)}
          </Select>
        </Field>
      </div>
      <Field label="识别接口地址" htmlFor="camera-recognition-url" help="可选外部车牌识别接口；留空即停用。识别失败不阻断其他条件满足的称重，候选仍须人工核对。">
        <Input id="camera-recognition-url" value={recognitionUrl} disabled={!canManage} placeholder="https://recognizer.example/api/plate" onChange={event => setRecognitionUrl(event.target.value)} autoComplete="off" spellCheck={false} />
      </Field>
      <p className="text-xs text-text-tertiary">删除识别源或取消其“称重抓拍”用途时，Server 会清除识别源选择并停用识别，不会静默改选其他摄像头。</p>
      {error && <p role="alert" className="text-sm text-error">{error}</p>}
      {canManage && <Button variant="primary" disabled={busy} onClick={() => void save()}>{busy ? "正在保存…" : "保存抓拍设置"}</Button>}
    </div>
  </ContentCard>;
}

function VideoSettingsCard({ list, canManage, onSaved, onSync }: { list: CameraList; canManage: boolean; onSaved: (video: CameraList["video"]) => void; onSync: () => Promise<void> }) {
  const video = list.video;
  const [apiUrl, setApiUrl] = useState(video.apiUrl);
  const [apiUsername, setApiUsername] = useState(video.apiUsername);
  const [apiPassword, setApiPassword] = useState("");
  const [clearPassword, setClearPassword] = useState(false);
  const [whepBaseUrl, setWhepBaseUrl] = useState(video.whepBaseUrl);
  const [busy, setBusy] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    setApiUrl(video.apiUrl);
    setApiUsername(video.apiUsername);
    setWhepBaseUrl(video.whepBaseUrl);
  }, [video.version, video.apiUrl, video.apiUsername, video.whepBaseUrl]);

  async function save() {
    setBusy(true); setError("");
    try {
      const saved = await cameraApi.saveVideo({
        apiUrl: apiUrl.trim(),
        apiUsername: apiUsername.trim(),
        whepBaseUrl: whepBaseUrl.trim(),
        version: video.version,
        ...(clearPassword ? { passwordAction: "clear" as const } : apiPassword ? { apiPassword, passwordAction: "set" as const } : { passwordAction: "keep" as const }),
      });
      onSaved(saved);
      setApiPassword(""); setClearPassword(false);
      toast.success({ title: "视频服务设置已保存", description: "远端同步结果单独显示；MediaMTX 离线不会回滚本次保存。", duration: 5000 });
    } catch (failure) { setError(getErrorMessage(failure, "视频服务设置保存失败")); }
    finally { setBusy(false); }
  }

  async function sync() {
    setSyncing(true); setError("");
    try { await onSync(); toast.success("视频源已同步。"); }
    catch (failure) { setError(`同步视频源失败：${getErrorMessage(failure, "请稍后重试")}`); }
    finally { setSyncing(false); }
  }

  return <ContentCard title="视频预览服务" description="MediaMTX 管理接口供 Server 同步托管路径；WHEP 入口由浏览器直接访问。" extra={<StatusTag tone={syncTone(video.syncState)}>{video.syncState === "synced" ? "已同步" : video.syncState === "error" ? "同步失败" : "待同步"}</StatusTag>}>
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="MediaMTX 管理接口" htmlFor="camera-api-url" help="Server 可达地址，可含路径前缀；不接受查询参数或内嵌凭据。">
          <Input id="camera-api-url" value={apiUrl} disabled={!canManage} placeholder="http://mediamtx.local:9997" onChange={event => setApiUrl(event.target.value)} autoComplete="off" spellCheck={false} />
        </Field>
        <Field label="公开 WHEP 入口" htmlFor="camera-whep-url" help="浏览器可达的 WebRTC/WHEP 基础地址；每台播放地址由系统生成。">
          <Input id="camera-whep-url" value={whepBaseUrl} disabled={!canManage} placeholder="http://mediamtx.local:8889" onChange={event => setWhepBaseUrl(event.target.value)} autoComplete="off" spellCheck={false} />
        </Field>
        <Field label="管理接口用户名" htmlFor="camera-api-username" help="管理凭据只保存在 Server，不下发浏览器。">
          <Input id="camera-api-username" value={apiUsername} disabled={!canManage} autoComplete="off" onChange={event => setApiUsername(event.target.value)} />
        </Field>
        <Field label="管理接口密码" htmlFor="camera-api-password" help={video.hasApiPassword ? "已保存密码；留空保持原值。" : "未保存密码。"}>
          <Input id="camera-api-password" type="password" value={apiPassword} disabled={!canManage || clearPassword} autoComplete="new-password" placeholder={video.hasApiPassword ? "留空保持原密码" : "无密码可留空"} onChange={event => setApiPassword(event.target.value)} />
        </Field>
      </div>
      {video.hasApiPassword && <label className="flex items-center gap-2 text-sm"><Checkbox checked={clearPassword} disabled={!canManage} onChange={event => { setClearPassword(event.target.checked); if (event.target.checked) setApiPassword(""); }} />清除已保存的管理接口密码（仅明确勾选时清除）</label>}
      <div className="space-y-1 rounded-control border border-border bg-neutral-background p-3 text-xs text-text-secondary">
        <p>视频配置版本 {video.version} · 已同步版本 {video.syncedVersion}</p>
        {video.syncError && <p role="alert" className="text-error">同步错误：{video.syncError}</p>}
        <p>同步失败或待恢复独立显示；相机保存成功不因远端离线回滚，直连抓拍仍可使用。预览映射未同步时明确显示不可用，不用旧画面冒充新配置。</p>
      </div>
      {error && <p role="alert" className="text-sm text-error">{error}</p>}
      {canManage && <div className="flex flex-wrap gap-2"><Button variant="primary" disabled={busy} onClick={() => void save()}>{busy ? "正在保存…" : "保存视频设置"}</Button><Button disabled={syncing || busy} onClick={() => void sync()}>{syncing ? "正在同步…" : "同步视频源"}</Button></div>}
    </div>
  </ContentCard>;
}
