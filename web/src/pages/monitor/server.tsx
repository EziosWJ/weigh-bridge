import { Activity, CircuitBoard, HardDrive, ListOrdered, Network, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { getCurrentUser } from "@/api/auth";
import { getMonitorHistory, getMonitorSnapshot } from "@/api/monitoring";
import { ContentCard } from "@/components/common/content-card";
import { DetailItem } from "@/components/common/detail-item";
import { DetailDialog } from "@/components/common/detail-dialog";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { getErrorMessage, isApiError } from "@/lib/api-error";
import type { FilesystemMetrics, MonitorHistory, MonitorResource, MonitorSnapshot } from "@/types/monitoring";
import { ResourceSection, ResourceTable } from "./resource-status";
import { TrendChart, type TrendSeries } from "./trend-chart";

const percent = (value: number | null | undefined) => value == null ? "不可用" : `${value.toFixed(1)}%`;
const number = (value: number | null | undefined, unit = "") => value == null ? "不可用" : `${value.toFixed(1)}${unit}`;
const bytes = (value: number | null | undefined) => {
  if (value == null) return "不可用";
  const unit = value >= 1024 ** 3 ? "GiB" : "MiB";
  return `${(value / (unit === "GiB" ? 1024 ** 3 : 1024 ** 2)).toFixed(2)} ${unit}`;
};
const rate = (value: number | null) => value == null ? "采集中 / 不可用" : `${(value / 1024 ** 2).toFixed(2)} MiB/s`;
const monitoringDisabled = (reason: unknown) => isApiError(reason) && reason.status === 503 && typeof reason.data === "object" && reason.data !== null && "reason" in reason.data && reason.data.reason === "monitoring_disabled";
const resources: { value: MonitorResource; label: string }[] = [{ value: "cpu", label: "CPU" }, { value: "memory", label: "内存" }, { value: "filesystem", label: "文件系统" }, { value: "disk", label: "块设备 I/O" }, { value: "network", label: "网卡" }, { value: "gpu", label: "GPU" }];
const detailTabs = [
  { value: "filesystems", label: "文件系统", icon: HardDrive },
  { value: "disks", label: "块设备 I/O", icon: Activity },
  { value: "network", label: "网卡", icon: Network },
  { value: "gpu", label: "GPU", icon: CircuitBoard },
  { value: "processes", label: "进程", icon: ListOrdered },
] as const;
type DetailTab = (typeof detailTabs)[number]["value"];

const seriesByResource: Record<MonitorResource, TrendSeries[]> = {
  cpu: [{ key: "usagePercent", label: "CPU 利用率", color: "#1677ff" }],
  memory: [{ key: "usagePercent", label: "内存占用率", color: "#1677ff" }],
  filesystem: [{ key: "usagePercent", label: "容量占用率", color: "#1677ff" }],
  disk: [{ key: "readBytesPerSecond", label: "读取", color: "#1677ff" }, { key: "writeBytesPerSecond", label: "写入", color: "#16a34a" }],
  network: [{ key: "receiveBytesPerSecond", label: "接收", color: "#1677ff" }, { key: "transmitBytesPerSecond", label: "发送", color: "#16a34a" }],
  gpu: [{ key: "usagePercent", label: "GPU 利用率", color: "#1677ff" }, { key: "memoryUsagePercent", label: "显存占用率", color: "#16a34a" }],
};



// Capacity overview: omit memory/runtime mounts and collapse aliases of the
// same storage source. Raw mount samples remain available for diagnosis.
function storageFilesystems(items: FilesystemMetrics[]) {
  const temporaryTypes = new Set(["tmpfs", "devtmpfs", "ramfs", "rootfs", "overlay", "squashfs", "iso9660", "udf", "fuse.portal"]);
  const runtimePaths = ["/dev", "/proc", "/sys", "/usr/lib/wsl", "/mnt/wsl/docker-desktop", "/mnt/wsl/docker-desktop-bind-mounts", "/mnt/wslg", "/Docker/host"];
  const candidates = items.filter((item) => item.mountpoint === "/" || (!temporaryTypes.has(item.type) && !runtimePaths.some((path) => item.mountpoint === path || item.mountpoint.startsWith(`${path}/`))));
  candidates.sort((a, b) => a.mountpoint.length - b.mountpoint.length || a.mountpoint.localeCompare(b.mountpoint));
  const seen = new Set<string>();
  return candidates.filter((item) => {
    const key = `${item.type}:${item.device || item.id}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

function UsageBar({ label, value, compact = false, className = "" }: { label: string; value: number | null; compact?: boolean; className?: string }) {
  return <div role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={value ?? undefined} aria-valuetext={value == null ? "采集中" : percent(value)} className={`${compact ? "h-1" : "h-2"} overflow-hidden rounded-full bg-neutral-background ${className}`}>
    {value != null && <div className={`h-full rounded-full ${value >= 90 ? "bg-warning" : "bg-primary"}`} style={{ width: `${Math.max(0, Math.min(100, value))}%` }} />}
  </div>;
}

export function ServerMonitoringPage() {
  const [access, setAccess] = useState<"checking" | "allowed" | "denied" | "disabled" | "error">("checking");
  const [snapshot, setSnapshot] = useState<MonitorSnapshot | null>(null);
  const [error, setError] = useState("");
  const [resource, setResource] = useState<MonitorResource>("cpu");
  const [device, setDevice] = useState("");
  const [showAll, setShowAll] = useState(false);
  const [showAllMounts, setShowAllMounts] = useState(false);
  const [detailTab, setDetailTab] = useState<DetailTab>("filesystems");
  const [history, setHistory] = useState<{ key: string; data: MonitorHistory | null; error: string } | null>(null);
  const [refresh, setRefresh] = useState(0);
  const [now, setNow] = useState(() => Date.now());
  const [ipInterface, setIpInterface] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    void getCurrentUser().then((user) => {
      if (active) setAccess(user.roles.some((role) => role.roleCode === "ADMIN") ? "allowed" : "denied");
    }).catch((reason: unknown) => {
      if (!active) return;
      setAccess("error");
      setError(getErrorMessage(reason, "用户权限检查失败"));
    });
    return () => { active = false; };
  }, [refresh]);

  useEffect(() => {
    if (access !== "allowed") return;
    const controller = new AbortController();
    let pending = false;
    const load = async () => {
      if (pending) return;
      pending = true;
      try {
        const data = await getMonitorSnapshot(controller.signal);
        if (!controller.signal.aborted) { setSnapshot(data); setError(""); }
      } catch (reason) {
        if (controller.signal.aborted) return;
        if (isApiError(reason) && reason.type === "forbidden") { setAccess("denied"); setSnapshot(null); }
        else if (monitoringDisabled(reason)) { setAccess("disabled"); setSnapshot(null); setHistory(null); setIpInterface(null); }
        else setError(getErrorMessage(reason, "监控加载失败"));
      } finally { pending = false; }
    };
    void load();
    const timer = window.setInterval(() => { void load(); }, 5000);
    const clock = window.setInterval(() => setNow(Date.now()), 1000);
    return () => { window.clearInterval(timer); window.clearInterval(clock); controller.abort(); };
  }, [access, refresh]);

  const allFilesystems = snapshot?.filesystems.data ?? [];
  const filesystems = showAllMounts ? allFilesystems : storageFilesystems(allFilesystems);
  const interfaces = (snapshot?.network.data ?? []).filter((item) => showAll || (!item.loopback && !item.id.startsWith("veth")));
  const devices = resource === "cpu" ? [{ id: "", label: "整机总体" }, ...(snapshot?.cpu.data?.perCore ?? []).map((_, index) => ({ id: String(index), label: `逻辑核 ${index}` }))]
    : resource === "memory" ? []
    : resource === "filesystem" ? filesystems.map((item) => ({ id: item.id, label: `${item.mountpoint}（${item.device}）` }))
    : resource === "disk" ? (snapshot?.disks.data ?? []).map((item) => ({ id: item.id, label: item.id }))
    : resource === "network" ? interfaces.map((item) => ({ id: item.id, label: item.id }))
    : (snapshot?.gpu.data ?? []).map((item) => ({ id: item.id, label: `${item.id} · ${item.name}` }));
  const selectedDevice = devices.some((item) => item.id === device) ? device : devices[0]?.id ?? "";
  const historyKey = `${resource}:${selectedDevice}`;
  const canQueryHistory = resource === "cpu" || resource === "memory" || Boolean(selectedDevice);

  useEffect(() => {
    if (access !== "allowed" || !canQueryHistory) return;
    const controller = new AbortController();
    let pending = false;
    const load = async () => {
      if (pending) return;
      pending = true;
      try {
        const data = await getMonitorHistory(resource, selectedDevice, controller.signal);
        if (!controller.signal.aborted) setHistory({ key: historyKey, data, error: "" });
      } catch (reason) {
        if (controller.signal.aborted) return;
        if (isApiError(reason) && reason.type === "forbidden") { setAccess("denied"); setSnapshot(null); }
        else if (monitoringDisabled(reason)) { setAccess("disabled"); setSnapshot(null); setHistory(null); setIpInterface(null); }
        else setHistory((previous) => ({ key: historyKey, data: previous?.key === historyKey ? previous.data : null, error: getErrorMessage(reason, "趋势加载失败") }));
      } finally { pending = false; }
    };
    void load();
    const timer = window.setInterval(() => { void load(); }, 5000);
    return () => { window.clearInterval(timer); controller.abort(); };
  }, [access, resource, selectedDevice, historyKey, canQueryHistory, refresh]);

  const currentHistory = history?.key === historyKey ? history : null;
  const trendPoints = (currentHistory?.data?.points ?? []).map((point) => ({ time: point.collectedAt, values: Object.fromEntries(Object.entries(point.values).map(([key, value]) => [key, value != null && (resource === "disk" || resource === "network") && key.endsWith("BytesPerSecond") ? value / 1024 ** 2 : value])) }));
  const warningTarget = (warning: MonitorSnapshot["warnings"][number]) => {
    const label = resources.find((item) => item.value === warning.resource)?.label ?? warning.resource;
    if (!warning.device) return label;
    const filesystem = warning.resource === "filesystem" ? snapshot?.filesystems.data?.find((item) => item.id === warning.device) : null;
    const gpu = warning.resource === "gpu" ? snapshot?.gpu.data?.find((item) => item.id === warning.device) : null;
    return `${label} · ${filesystem ? `${filesystem.mountpoint}（${filesystem.device}）` : gpu ? `${gpu.id}（${gpu.name}）` : warning.device}`;
  };
  const cpu = snapshot?.cpu.data;
  const memory = snapshot?.memory.data;
  const host = snapshot?.host.data;
  const selectedInterface = snapshot?.network.data?.find((item) => item.id === ipInterface);
  const header = <PageHeader title="服务器监控" description={`${host?.os || "宿主机资源"} · 每5秒刷新 · 最近30分钟历史`} actions={<Button variant="secondary" size="sm" onClick={() => setRefresh((value) => value + 1)}><RefreshCw className="h-4 w-4" />刷新</Button>} />;
  if (access === "checking") return <div>{header}<p role="status">正在检查 ADMIN 权限…</p></div>;
  if (access === "denied") return <div>{header}<EmptyState title="无权访问服务器监控" description="仅启用的 ADMIN 管理员可以查看宿主机和进程信息。" /></div>;
  if (access === "disabled") return <div>{header}<EmptyState title="服务器监控模块未启用" description="当前服务器未开启资源监控。" /></div>;
  if (access === "error") return <div>{header}<EmptyState title="权限检查失败" description={error} actionText="重试" onAction={() => setRefresh((value) => value + 1)} /></div>;
  return <div className="space-y-6">{header}
    {error && <p role="alert" className="rounded-admin border border-warning-border bg-warning-background p-4 text-sm text-warning">{error}；保留已有数据，各区域鲜度按有效采集时间判断。</p>}
    {!snapshot ? <p role="status">监控数据加载中…</p> : <>
      <ResourceSection title="主机信息" state={snapshot.host} now={now}><div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4"><DetailItem label="主机名" value={host?.hostname} /><DetailItem label="操作系统" value={host?.os} /><DetailItem label="内核" value={host?.kernel} /><DetailItem label="运行时长" value={host ? `${Math.floor(host.uptimeSeconds / 86400)} 天 ${Math.floor(host.uptimeSeconds % 86400 / 3600)} 小时` : null} /></div><p className="mt-4 break-all text-xs text-text-tertiary">样本时间：{Date.parse(snapshot.sampledAt) > 0 ? new Date(snapshot.sampledAt).toLocaleString() : "采集中"}</p></ResourceSection>
      {(snapshot.warnings ?? []).length > 0 && <div role="alert" className="rounded-admin border border-warning-border bg-warning-background p-4 text-sm text-warning"><h2 className="font-semibold">资源异常提示</h2>{(snapshot.warnings ?? []).map((warning) => <p key={`${warning.resource}:${warning.device}`} className="mt-2"><span className="font-semibold">{warningTarget(warning)}</span>：{warning.message} · {new Date(warning.since).toLocaleString()}</p>)}</div>}
      <div className="grid grid-cols-1 items-stretch gap-6 lg:grid-cols-2">
        <ResourceSection title="CPU" state={snapshot.cpu} now={now} className="flex h-full flex-col" bodyClassName="flex flex-1 flex-col">
          {cpu && <>
            <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)] items-center gap-5">
              <div>
                <p className="text-sm text-text-secondary">总体利用率</p>
                <p className="mt-1 text-3xl font-semibold tabular-nums text-text-primary">{cpu.usagePercent == null ? <span className="text-xl">采集中</span> : percent(cpu.usagePercent)}</p>
                <p className="mt-1 text-xs text-text-tertiary">整机总算力为100%</p>
              </div>
              <div className="border-l border-border pl-4">
                <p className="mb-2 text-xs text-text-secondary">系统负载{cpu.load1 == null ? "（当前平台不支持）" : ""}</p>
                <div className="grid grid-cols-3 gap-2">
                  {[{ label: "1 分钟", value: cpu.load1 }, { label: "5 分钟", value: cpu.load5 }, { label: "15 分钟", value: cpu.load15 }].map((item) => <div key={item.label}><p className="text-lg font-semibold tabular-nums">{number(item.value)}</p><p className="mt-1 whitespace-nowrap text-xs text-text-tertiary">{item.label}</p></div>)}
                </div>
              </div>
            </div>
            <UsageBar label="总体 CPU 利用率" value={cpu.usagePercent} className="mt-4" />
            <div className="mt-5 border-t border-border pt-4">
              <p className="break-words text-sm font-medium text-text-secondary">{cpu.model || "CPU 型号不可用"}</p>
              <p className="mt-1 text-xs text-text-tertiary">{cpu.physicalCores > 0 ? `${cpu.physicalCores} 物理核` : "物理核数不可用"} / {cpu.logicalCores} 逻辑核</p>
            </div>
            <div className="my-4 grid grid-cols-2 gap-x-5 gap-y-3 sm:grid-cols-4">
              {(cpu.perCore ?? []).map((value, index) => <div key={index} className="min-w-0">
                <div className="mb-1.5 flex items-baseline justify-between gap-1"><span className="whitespace-nowrap text-xs text-text-tertiary">逻辑核 {index}</span><span className="whitespace-nowrap text-xs font-medium tabular-nums text-text-primary">{value == null ? "采集中" : percent(value)}</span></div>
                <UsageBar label={`逻辑核 ${index} 利用率`} value={value} compact />
              </div>)}
            </div>
            <p className="mt-auto border-t border-border pt-3 text-xs leading-5 text-text-tertiary">逐核以该逻辑核的算力为100%；系统负载展示原始值。</p>
          </>}
        </ResourceSection>
        <ResourceSection title="内存与 Swap" state={snapshot.memory} now={now} className="flex h-full flex-col" bodyClassName="flex flex-1 flex-col">
          {memory && <>
            <div className="flex items-start justify-between gap-4">
              <div><p className="text-sm text-text-secondary">内存占用率</p><p className="mt-1 text-3xl font-semibold tabular-nums text-text-primary">{percent(memory.usagePercent)}</p><p className="mt-1 text-xs text-text-tertiary">物理内存</p></div>
              <div className="text-right"><p className="text-xs text-text-secondary">可用内存</p><p className="mt-2 whitespace-nowrap text-xl font-semibold tabular-nums text-text-primary">{bytes(memory.availableBytes)}</p></div>
            </div>
            <UsageBar label="内存占用率" value={memory.usagePercent} className="mt-4" />
            <dl className="mt-5 grid grid-cols-3 divide-x divide-border border-y border-border py-4">
              {[{ label: "内存总量", value: memory.totalBytes }, { label: "已用内存", value: memory.usedBytes }, { label: "可用内存", value: memory.availableBytes }].map((item, index) => <div key={item.label} className={index === 0 ? "pr-2" : "px-2 sm:px-3"}><dt className="text-xs text-text-tertiary">{item.label}</dt><dd className="mt-2 whitespace-nowrap text-sm font-semibold tabular-nums text-text-primary">{bytes(item.value)}</dd></div>)}
            </dl>
            <div className="my-5">
              <div className="mb-3 flex items-baseline justify-between gap-3"><h3 className="text-sm font-medium text-text-secondary">Swap</h3><span className="text-sm font-semibold tabular-nums">{memory.swapUnsupported ? "当前平台不支持此指标" : memory.swapTotalBytes === 0 ? "未配置 Swap" : percent(memory.swapUsagePercent)}</span></div>
              {memory.swapTotalBytes > 0 && <><UsageBar label="Swap 占用率" value={memory.swapUsagePercent} /><div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-text-tertiary"><span>已用 <span className="tabular-nums text-text-secondary">{bytes(memory.swapUsedBytes)}</span></span><span>总量 <span className="tabular-nums text-text-secondary">{bytes(memory.swapTotalBytes)}</span></span></div></>}
            </div>
            <p className="mt-auto border-t border-border pt-3 text-xs leading-5 text-text-tertiary">内存已用 = 总量 − 可用量；占用率 = 已用 ÷ 总量。</p>
          </>}
        </ResourceSection>
      </div>
      <ContentCard title="最近30分钟趋势" className="shadow-none"><div className="mb-4 flex flex-wrap gap-3"><Select aria-label="趋势资源" className="w-40" value={resource} onChange={(event) => { setResource(event.target.value as MonitorResource); setDevice(""); }}>{resources.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</Select>{devices.length > 0 && <Select aria-label="趋势设备" className="w-64" value={selectedDevice} onChange={(event) => setDevice(event.target.value)}>{devices.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}</Select>}{resource === "filesystem" && <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={showAllMounts} onChange={(event) => setShowAllMounts(event.target.checked)} />显示全部挂载点</label>}{resource === "network" && <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={showAll} onChange={(event) => setShowAll(event.target.checked)} />显示全部接口</label>}</div>{currentHistory?.error && <p role="alert" className="mb-3 text-sm text-warning">{currentHistory.error}</p>}{canQueryHistory ? <><TrendChart points={trendPoints} series={seriesByResource[resource]} unit={resource === "disk" || resource === "network" ? "MiB/s" : "%"} />{resource === "disk" && <div className="mt-5"><TrendChart points={trendPoints} series={[{ key: "readIops", label: "读取 IOPS", color: "#1677ff" }, { key: "writeIops", label: "写入 IOPS", color: "#16a34a" }]} unit="次/秒" /></div>}{resource === "gpu" && <div className="mt-5 grid grid-cols-1 gap-5 sm:grid-cols-2"><TrendChart points={trendPoints} series={[{ key: "temperatureCelsius", label: "温度", color: "#1677ff" }]} unit="°C" /><TrendChart points={trendPoints} series={[{ key: "powerWatts", label: "功耗", color: "#1677ff" }]} unit="W" /></div>}</> : <p className="text-sm text-text-tertiary">当前资源没有可选择的设备，暂无历史数据。</p>}</ContentCard>
      <section aria-label="设备与进程" className="min-w-0 overflow-hidden rounded-admin border border-border bg-surface">
        <div role="tablist" aria-label="监控详情" className="flex gap-1 overflow-x-auto border-b border-border px-3">
          {detailTabs.map((tab, index) => {
            const Icon = tab.icon;
            const state = snapshot[tab.value];
            const expired = state.stale || Boolean(state.collectedAt && now - Date.parse(state.collectedAt) > 15000);
            const attention = expired || state.status === "error" || state.status === "unsupported" || state.partial;
            return <button key={tab.value} type="button" role="tab" id={`monitor-tab-${tab.value}`} aria-controls={`monitor-panel-${tab.value}`} aria-selected={detailTab === tab.value} tabIndex={detailTab === tab.value ? 0 : -1} title={attention ? `${tab.label}：数据过期、不支持或采集不完整，请查看详情` : tab.label}
              className={`relative flex shrink-0 items-center gap-2 whitespace-nowrap px-4 py-4 text-sm font-medium outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary ${detailTab === tab.value ? "text-primary" : "text-text-secondary hover:bg-neutral-background hover:text-text-primary"}`}
              onClick={() => setDetailTab(tab.value)}
              onKeyDown={(event) => {
                let next = index;
                if (event.key === "ArrowRight") next = (index + 1) % detailTabs.length;
                else if (event.key === "ArrowLeft") next = (index + detailTabs.length - 1) % detailTabs.length;
                else if (event.key === "Home") next = 0;
                else if (event.key === "End") next = detailTabs.length - 1;
                else return;
                event.preventDefault();
                setDetailTab(detailTabs[next].value);
                event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus();
              }}>
              <Icon aria-hidden="true" className="h-4 w-4" />{tab.label}
              {attention && <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-warning" />}
              {detailTab === tab.value && <span aria-hidden="true" className="absolute inset-x-4 bottom-0 h-0.5 rounded-t-full bg-primary" />}
            </button>;
          })}
        </div>
        <div role="tabpanel" id="monitor-panel-filesystems" aria-labelledby="monitor-tab-filesystems" hidden={detailTab !== "filesystems"} tabIndex={0} className="outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary">
          <ResourceSection title="文件系统容量" state={snapshot.filesystems} now={now} className="rounded-none border-0"><div className="mb-4 flex flex-wrap items-center justify-between gap-3"><p className="text-sm text-text-secondary">{showAllMounts ? `全部挂载点 ${allFilesystems.length} 项` : `持久存储 ${filesystems.length} 项`}<span className="ml-2 text-xs text-text-tertiary">{showAllMounts ? "包含重复挂载及临时文件系统" : "已隐藏临时、运行时挂载，并合并同源重复项"}</span></p><label className="flex shrink-0 items-center gap-2 text-sm"><input type="checkbox" checked={showAllMounts} onChange={(event) => setShowAllMounts(event.target.checked)} />显示全部挂载点</label></div><ResourceTable headers={["挂载点", "设备 / 类型", "总量", "已用", "普通用户可用", "占用率", "说明"]} rows={filesystems.map((item) => [item.mountpoint, `${item.device} / ${item.type}`, bytes(item.totalBytes), bytes(item.usedBytes), bytes(item.availableBytes), percent(item.usagePercent), item.message || "—"])} /><p className="mt-3 text-xs text-text-tertiary">占用率 = 已用 ÷（已用 + 普通用户可用）。</p></ResourceSection>
        </div>
        <div role="tabpanel" id="monitor-panel-disks" aria-labelledby="monitor-tab-disks" hidden={detailTab !== "disks"} tabIndex={0} className="outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary">
          <ResourceSection title="块设备 I/O" state={snapshot.disks} now={now} className="rounded-none border-0"><ResourceTable headers={["设备", "读取", "写入", "读取 IOPS", "写入 IOPS", "说明"]} rows={(snapshot.disks.data ?? []).map((item) => [item.id, rate(item.readBytesPerSecond), rate(item.writeBytesPerSecond), number(item.readIops), number(item.writeIops), item.message || "—"])} /><p className="mt-3 text-xs text-text-tertiary">新设备、首样本与计数重置会重新建立速率基线；速率不可用时不会填零。</p></ResourceSection>
        </div>
        <div role="tabpanel" id="monitor-panel-network" aria-labelledby="monitor-tab-network" hidden={detailTab !== "network"} tabIndex={0} className="outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary">
          <ResourceSection title="逐网卡流量" state={snapshot.network} now={now} className="rounded-none border-0"><label className="mb-4 flex items-center gap-2 text-sm"><input type="checkbox" checked={showAll} onChange={(event) => setShowAll(event.target.checked)} />显示全部接口（默认隐藏回环和 veth）</label><ResourceTable headers={["网卡", "链路", "链路速率", "接收 / 发送", "累计接收 / 发送", "收 / 发带宽占用率", "收 / 发错误", "收 / 发丢包", "说明", "操作"]} rows={interfaces.map((item) => [item.id, item.state, number(item.speedBitsPerSecond == null ? null : item.speedBitsPerSecond / 1e6, " Mbit/s"), `${rate(item.receiveBytesPerSecond)} / ${rate(item.transmitBytesPerSecond)}`, `${bytes(item.receiveBytes)} / ${bytes(item.transmitBytes)}`, `${percent(item.receiveUsagePercent)} / ${percent(item.transmitUsagePercent)}`, `${item.receiveErrors} / ${item.transmitErrors}`, `${item.receiveDropped} / ${item.transmitDropped}`, item.message || "—", <Button key={item.id} variant="ghost" size="sm" onClick={() => setIpInterface(item.id)}>查看 IP</Button>])} /><p className="mt-3 text-xs text-text-tertiary">1 MiB = 1,048,576 字节；收发分别按 bit/s 与链路速率计算占用率。链路速率未知时占用率不可用。</p></ResourceSection>
        </div>
        <div role="tabpanel" id="monitor-panel-gpu" aria-labelledby="monitor-tab-gpu" hidden={detailTab !== "gpu"} tabIndex={0} className="outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary">
          <ResourceSection title="NVIDIA GPU" state={snapshot.gpu} now={now} className="rounded-none border-0"><ResourceTable headers={["GPU", "型号", "利用率", "显存已用 / 总量", "温度", "功耗", "说明"]} rows={(snapshot.gpu.data ?? []).map((item) => [item.id, item.name, percent(item.usagePercent), `${bytes(item.memoryUsedBytes)} / ${bytes(item.memoryTotalBytes)}`, number(item.temperatureCelsius, " °C"), number(item.powerWatts, " W"), item.message || "—"])} /><p className="mt-3 text-xs text-text-tertiary">字段不可用表示设备不支持或当前无法读取，请参照采集说明。</p></ResourceSection>
        </div>
        <div role="tabpanel" id="monitor-panel-processes" aria-labelledby="monitor-tab-processes" hidden={detailTab !== "processes"} tabIndex={0} className="outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary">
          <ResourceSection title="进程 Top 10" state={snapshot.processes} now={now} className="rounded-none border-0"><div className="grid grid-cols-1 gap-6 xl:grid-cols-2"><div><h3 className="mb-3 text-sm font-semibold">CPU 占用</h3><ResourceTable headers={["进程名", "PID", "CPU 占用"]} rows={(snapshot.processes.data?.cpu ?? []).slice(0, 10).map((item) => [item.name, item.pid, percent(item.cpuPercent)])} /></div><div><h3 className="mb-3 text-sm font-semibold">常驻物理内存</h3><ResourceTable headers={["进程名", "PID", "RSS"]} rows={(snapshot.processes.data?.memory ?? []).slice(0, 10).map((item) => [item.name, item.pid, bytes(item.memoryBytes)])} /></div></div><p className="mt-3 text-xs text-text-tertiary">进程 CPU 以整机总算力为100%；内存按常驻物理内存量排序。</p></ResourceSection>
        </div>
      </section>
      <ContentCard title="固定异常规则" className="shadow-none"><p className="text-sm leading-7 text-text-secondary">CPU、内存 ≥90% 持续60秒后提示，&lt;85% 持续30秒后恢复。文件系统 ≥90% 立即提示，&lt;85% 恢复。采样中断中止持续时间判断，恢复后重新计时。GPU 和网络展示状态与错误，高利用率或高流量本身不判为异常。</p></ContentCard>
    </>}
    <DetailDialog open={ipInterface !== null} title="网卡 IP 地址" description={ipInterface} onCancel={() => setIpInterface(null)} trapFocus contentClassName="max-w-xl">
      {!selectedInterface ? <p role="status">接口已不可用</p> : snapshot && <ResourceSection title="地址信息" state={snapshot.network} now={now}>
        {selectedInterface.addressError ? <p role="alert" className="text-sm text-warning">{selectedInterface.addressError}</p>
          : selectedInterface.addresses == null ? <p role="status">IP 地址不可用：采集端未提供地址信息</p>
          : selectedInterface.addresses.length === 0 ? <p role="status">未分配 IP</p>
          : <ul className="space-y-3">{selectedInterface.addresses.map((address) => <li key={`${address.family}:${address.ip}/${address.prefixLength}`} className="flex flex-wrap gap-3 text-sm"><span className="text-text-secondary">{address.family}</span><span className="break-all font-mono">{address.ip}/{address.prefixLength}</span></li>)}</ul>}
      </ResourceSection>}
    </DetailDialog>
  </div>;
}
