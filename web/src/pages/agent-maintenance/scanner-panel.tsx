import { useEffect, useState } from 'react';
import { Check, RefreshCw, ScanLine, Usb } from 'lucide-react';
import type { SerialScannerConfig, SerialScannerSnapshot } from '@/api/agent-maintenance';
import { Field } from '@/components/common/field';
import { FormSection } from '@/components/common/form-section';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { formatDateTime } from '@/lib/datetime';

export type ScannerSettings = Omit<SerialScannerConfig, 'version' | 'updateTime'>;

const baudOptions = [1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200];

function settingsFrom(config: SerialScannerConfig): ScannerSettings {
  return {
    enabled: config.enabled, device: config.device, baudRate: config.baudRate,
    dataBits: config.dataBits, stopBits: config.stopBits, parity: config.parity,
    delimiter: config.delimiter, trimPrefix: config.trimPrefix, trimSuffix: config.trimSuffix,
    retryCount: config.retryCount, retryIntervalMs: config.retryIntervalMs,
  };
}

export function MaintenanceScannerPanel({ snapshot, loading, busy, error, canManage, onReload, onSave }: {
  snapshot: SerialScannerSnapshot | null;
  loading: boolean;
  busy: boolean;
  error: string;
  canManage: boolean;
  onReload: () => void;
  onSave: (settings: ScannerSettings) => void;
}) {
  const [draft, setDraft] = useState<ScannerSettings | null>(null);
  const version = snapshot?.config.version;

  useEffect(() => {
    if (snapshot) setDraft(settingsFrom(snapshot.config));
  }, [version, snapshot]);

  function update<K extends keyof ScannerSettings>(key: K, value: ScannerSettings[K]) {
    setDraft(current => current ? { ...current, [key]: value } : current);
  }

  function save() {
    if (!draft || !canManage || busy) return;
    if (!window.confirm(`保存后将立即下发到扫码枪 Agent，并重新打开 ${draft.device || '串口'}。是否继续？`)) return;
    onSave(draft);
  }

  const status = snapshot?.status;
  const online = snapshot?.online === true;
  const connected = online && status?.connected === true;

  return <div className="space-y-space-4">
    {error && <p role="alert" className="rounded-control border border-error-border bg-error-background p-space-3 text-sm text-error">{error}</p>}

    <div className="grid gap-space-4 xl:grid-cols-2">
      <ContentCard title="扫码枪运行状态" extra={<Button disabled={loading || busy} onClick={onReload}><RefreshCw className={`h-4 w-4 ${loading ? 'motion-safe:animate-spin' : ''}`} aria-hidden />刷新</Button>}>
        {loading && !snapshot ? <p role="status" className="text-sm text-text-secondary">正在读取扫码枪状态…</p> : !snapshot ? <p className="text-sm text-text-tertiary">暂无扫码枪状态。</p> : <>
          <div className="grid grid-cols-2 gap-space-4">
            <DetailItem label="Agent 状态" value={<StatusTag tone={online ? 'success' : 'error'}>{online ? '在线' : '离线'}</StatusTag>} />
            <DetailItem label="串口连接" value={<StatusTag tone={connected ? 'success' : 'warning'}>{connected ? '已连接' : '未连接'}</StatusTag>} />
            <DetailItem label="串口设备" value={status?.device || snapshot.config.device || '—'} />
            <DetailItem label="配置版本" value={`v${snapshot.config.version}`} />
            <DetailItem label="Agent 应用版本" value={`v${status?.configVersion ?? 0}`} />
            <DetailItem label="最近心跳" value={formatDateTime(status?.heartbeatAt)} />
            <DetailItem label="Agent 标识" value={status?.agentId || '—'} />
            <DetailItem label="本次扫码" value={`${status?.scans ?? 0} 次`} />
            <DetailItem label="本次失败" value={`${status?.failures ?? 0} 次`} />
          </div>
          <div className="mt-space-4 border-t border-border pt-space-4">
            <DetailItem label="最近扫码" value={status?.lastScanAt ? `${formatDateTime(status.lastScanAt)} · ${status.lastScanResult || '—'}` : '尚未扫码'} />
            {status?.lastError && <p role="alert" className="mt-space-3 break-words text-sm text-warning">{status.lastError}</p>}
            {online && status && status.configVersion !== snapshot.config.version && <p className="mt-space-3 text-sm text-warning">Agent 尚未应用最新配置，将在下次心跳后重新连接串口。</p>}
          </div>
          <p className="mt-space-4 text-body-secondary text-text-tertiary">
            串口扫码 Agent 在厂端部署机后台运行，直接读取串口并调用内部扫码接口。本页只做配置和监控，不需要打开扫码页面，也不使用用户账号登录。
          </p>
        </>}
      </ContentCard>

      <ContentCard title="扫码枪配置" extra={<span className="text-xs text-text-tertiary">生效版本 v{snapshot?.config.version ?? 0}</span>}>
        {!canManage ? <p className="text-sm text-text-secondary">当前账号没有扫码枪配置权限。请联系管理员分配 agent:maintenance:manage。</p>
          : !draft ? <p className="text-sm text-text-secondary">请先刷新状态。</p>
          : <div className="space-y-space-4">
            <FormSection title="串口参数">
              <Field label="启用扫码枪" htmlFor="scanner-enabled" hint="停用后 Agent 会保持在线但关闭串口。">
                <label className="flex h-control-md items-center gap-space-2 text-sm">
                  <input id="scanner-enabled" type="checkbox" className="h-4 w-4" checked={draft.enabled} disabled={busy}
                    onChange={event => update('enabled', event.target.checked)} />
                  启用后台串口扫码
                </label>
              </Field>
              <Field label="串口设备" htmlFor="scanner-device" hint="Linux 部署机实际路径，例如 /dev/ttyUSB0。">
                <Input id="scanner-device" value={draft.device} disabled={busy} autoComplete="off"
                  onChange={event => update('device', event.target.value)} />
              </Field>
              <Field label="波特率" htmlFor="scanner-baud">
                <Select id="scanner-baud" value={String(draft.baudRate)} disabled={busy}
                  onChange={event => update('baudRate', Number(event.target.value))}>
                  {baudOptions.map(value => <option key={value} value={value}>{value}</option>)}
                </Select>
              </Field>
              <div className="grid grid-cols-2 gap-space-3">
                <Field label="数据位" htmlFor="scanner-data-bits">
                  <Select id="scanner-data-bits" value={String(draft.dataBits)} disabled={busy}
                    onChange={event => update('dataBits', Number(event.target.value))}>
                    {[5, 6, 7, 8].map(value => <option key={value} value={value}>{value}</option>)}
                  </Select>
                </Field>
                <Field label="停止位" htmlFor="scanner-stop-bits">
                  <Select id="scanner-stop-bits" value={String(draft.stopBits)} disabled={busy}
                    onChange={event => update('stopBits', Number(event.target.value))}>
                    <option value="1">1</option>
                    <option value="2">2</option>
                  </Select>
                </Field>
              </div>
              <Field label="校验位" htmlFor="scanner-parity">
                <Select id="scanner-parity" value={draft.parity} disabled={busy}
                  onChange={event => update('parity', event.target.value as ScannerSettings['parity'])}>
                  <option value="none">无（none）</option>
                  <option value="even">偶校验（even）</option>
                  <option value="odd">奇校验（odd）</option>
                </Select>
              </Field>
            </FormSection>

            <FormSection title="扫码组帧">
              <Field label="结束符" htmlFor="scanner-delimiter" hint="支持 \r \n \t \\ 转义；扫码枪通常输出回车换行。">
                <Input id="scanner-delimiter" value={draft.delimiter} disabled={busy} autoComplete="off"
                  onChange={event => update('delimiter', event.target.value)} />
              </Field>
              <div className="grid grid-cols-2 gap-space-3">
                <Field label="前置字符" htmlFor="scanner-trim-prefix" hint="扫码枪固定前缀，通常留空。">
                  <Input id="scanner-trim-prefix" value={draft.trimPrefix} disabled={busy} autoComplete="off"
                    onChange={event => update('trimPrefix', event.target.value)} />
                </Field>
                <Field label="后置字符" htmlFor="scanner-trim-suffix" hint="扫码枪固定后缀，通常留空。">
                  <Input id="scanner-trim-suffix" value={draft.trimSuffix} disabled={busy} autoComplete="off"
                    onChange={event => update('trimSuffix', event.target.value)} />
                </Field>
              </div>
            </FormSection>

            <FormSection title="失败重试">
              <div className="grid grid-cols-2 gap-space-3">
                <Field label="重试次数" htmlFor="scanner-retry-count" hint="仅网络超时或服务不可用时重试；二维码业务拒绝不重试。">
                  <Input id="scanner-retry-count" type="number" min={0} max={10} value={draft.retryCount} disabled={busy}
                    onChange={event => update('retryCount', Number(event.target.value))} />
                </Field>
                <Field label="重试间隔（ms）" htmlFor="scanner-retry-interval" hint="100～10000 ms。">
                  <Input id="scanner-retry-interval" type="number" min={100} max={10000} value={draft.retryIntervalMs} disabled={busy}
                    onChange={event => update('retryIntervalMs', Number(event.target.value))} />
                </Field>
              </div>
            </FormSection>

            <div className="flex flex-wrap items-center justify-end gap-space-3 border-t border-border pt-space-4">
              <span className="mr-auto text-xs text-text-secondary">保存后 Agent 会自动重新连接串口。</span>
              <Button variant="primary" disabled={busy} onClick={save}><Check className="h-4 w-4" aria-hidden />保存并下发</Button>
            </div>
          </div>}
      </ContentCard>
    </div>

    <div className="flex items-center gap-space-2 text-body-secondary text-text-tertiary">
      <ScanLine className="h-4 w-4" aria-hidden />
      <span>Agent 只读取扫码枪串口；手机二维码内容不会写入日志全文。</span>
      <Usb className="ml-space-2 h-4 w-4" aria-hidden />
      <span>串口参数变更后无需重启 API 服务。扫码明细请在「手机扫码」页查看。</span>
    </div>
  </div>;
}
