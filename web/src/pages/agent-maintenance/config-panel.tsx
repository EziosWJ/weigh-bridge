import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { Check, RotateCcw } from 'lucide-react';
import type { CupsQueue, DeviceSettings, MaintenanceConfig } from '@/api/agent-maintenance';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { Field } from '@/components/common/field';
import { FormSection } from '@/components/common/form-section';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { formatDateTime } from '@/lib/datetime';

const baudOptions = [1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200];

function stateLabel(value: string | undefined) {
  switch (value) {
    case 'running': return '运行中';
    case 'degraded': return '降级运行';
    case 'unavailable': return '不可用';
    case 'configured': return '已配置';
    case 'fault': return '故障';
    case 'ready': return '就绪';
    case 'not_configured': return '未配置';
    default: return value || '未知';
  }
}

function tone(value: string | undefined) {
  if (value === 'running' || value === 'ready' || value === 'configured') return 'success' as const;
  if (value === 'fault' || value === 'unavailable') return 'error' as const;
  return 'warning' as const;
}

export function MaintenanceConfigPanel({ config, settings, setSettings, queues, queuesError, serialHint, busy, feedback, onApply, onRestore }: {
  config: MaintenanceConfig;
  settings: DeviceSettings;
  setSettings: Dispatch<SetStateAction<DeviceSettings>>;
  queues: CupsQueue[];
  queuesError?: string;
  serialHint?: string;
  busy: boolean;
  feedback: ReactNode;
  onApply: () => void;
  onRestore: () => void;
}) {
  const effective = config.effective;
  const backup = config.backup;
  const queueOptions = queues.map(queue => queue.name);
  if (settings.print.queue && !queueOptions.includes(settings.print.queue)) queueOptions.unshift(settings.print.queue);
  const latest = effective?.print.queue ? queues.find(queue => queue.name === effective.print.queue) : undefined;

  function updateSerial(key: keyof DeviceSettings['serial'], value: string | number) {
    setSettings(current => ({ ...current, serial: { ...current.serial, [key]: value } }));
  }
  function updatePrint(key: keyof DeviceSettings['print'], value: string | number) {
    setSettings(current => ({ ...current, print: { ...current.print, [key]: value } }));
  }

  return <div className="space-y-space-4">
    <ContentCard title="当前生效配置" extra={<StatusTag tone={tone(effective?.deviceState)}>{stateLabel(effective?.deviceState)}</StatusTag>}>
      <div className="grid gap-space-4 sm:grid-cols-3">
        <DetailItem label="生效版本" value={effective ? `v${effective.configVersion}` : '尚未上报'} />
        <DetailItem label="最近生效时间" value={formatDateTime(effective?.appliedAt)} />
        <DetailItem label="仪表／打印" value={`${stateLabel(effective?.serialState)}／${stateLabel(effective?.printerState)}`} />
      </div>
      <p className="mt-space-4 border-t border-border pt-space-3 text-body-secondary text-text-secondary">此区域只读，来自 Agent 实际成功生效的参数；修改下方表单并“保存并应用”才会改变设备。</p>
    </ContentCard>

    {serialHint && <div role="alert" className="rounded-control border border-error-border bg-error-background p-space-3 text-sm text-error">{serialHint}</div>}

    <div className="flex flex-wrap items-center gap-x-space-4 gap-y-space-2 text-sm" aria-label="配置分组">
      <span className="text-text-tertiary">快速定位</span>
      {[['serial', '称重仪表'], ['print', '打印与校准']].map(([id, label]) => <a key={id} href={`#maintenance-config-${id}`} className="rounded text-primary underline-offset-4 hover:underline">{label}</a>)}
    </div>

    <div id="maintenance-config-serial" className="scroll-mt-20">
      <FormSection title="称重仪表" description="固定 D12 tF=05、8N1、吨制与 1 秒数据新鲜度；只填 Agent 主机实际串口与仪表波特率。">
        <Field label="串口路径" htmlFor="maintenance-serial-port" help="例如 /dev/ttyUSB0，使用 Agent 主机上的实际路径；留空表示尚未配置。">
          <Input id="maintenance-serial-port" disabled={busy} value={settings.serial.port}
            onChange={event => updateSerial('port', event.target.value)} autoComplete="off" />
        </Field>
        <Field label="仪表波特率" htmlFor="maintenance-serial-baud">
          <Select id="maintenance-serial-baud" disabled={busy} value={String(settings.serial.baud)}
            onChange={event => updateSerial('baud', Number(event.target.value))}>
            {baudOptions.map(value => <option key={value} value={value}>{value}</option>)}
          </Select>
        </Field>
      </FormSection>
    </div>

    <div id="maintenance-config-print" className="scroll-mt-20">
      <FormSection title="打印与校准" description="先节长，再位置，必要时缩放；保存并应用后打印校准样张核对纸面。">
        <Field label="CUPS 打印队列" htmlFor="maintenance-queue" help={queuesError || '队列由 Agent 主机枚举；不是 Server 主机的队列。'}>
          <Select id="maintenance-queue" disabled={busy || !queueOptions.length} value={settings.print.queue}
            onChange={event => updatePrint('queue', event.target.value)}>
            {!queueOptions.length && <option value="">{queuesError ? '队列不可读取' : '尚未读取到队列'}</option>}
            {queueOptions.map(name => <option key={name} value={name}>{name}{name === latest?.name ? '（当前）' : ''}{queues.find(queue => queue.name === name)?.isDefault ? '（默认）' : ''}</option>)}
          </Select>
        </Field>
        <Field label="单据节长（mm）" htmlFor="maintenance-form-length" help="相邻横向撕纸线节距，支持小数；用于逐单走纸校准。">
          <Input id="maintenance-form-length" type="number" min={85} max={300} step="any" disabled={busy}
            value={settings.print.formLengthMm} onChange={event => updatePrint('formLengthMm', Number(event.target.value))} />
        </Field>
        <Field label="水平偏移（mm）" htmlFor="maintenance-offset-x" help="正方向向右；单张内的位置调整。">
          <Input id="maintenance-offset-x" type="number" min={0} max={30} disabled={busy}
            value={settings.print.offsetXmm} onChange={event => updatePrint('offsetXmm', Number(event.target.value))} />
        </Field>
        <Field label="垂直偏移（mm）" htmlFor="maintenance-offset-y" help="正方向向下；不作为累计走纸误差的替代修正。">
          <Input id="maintenance-offset-y" type="number" min={0} max={30} disabled={busy}
            value={settings.print.offsetYmm} onChange={event => updatePrint('offsetYmm', Number(event.target.value))} />
        </Field>
        <Field label="内容缩放（倍）" htmlFor="maintenance-print-scale" help="先节长、再位置，必要时才缩放。">
          <Input id="maintenance-print-scale" type="number" min={0.5} max={1.5} step="any" disabled={busy}
            value={settings.print.printScale} onChange={event => updatePrint('printScale', Number(event.target.value))} />
        </Field>
        <Field label="基准打印宽度（mm）" htmlFor="maintenance-print-width" help="高级校准，维持现有已满意的版面。">
          <Input id="maintenance-print-width" type="number" min={100} max={210} disabled={busy}
            value={settings.print.printWidthMm} onChange={event => updatePrint('printWidthMm', Number(event.target.value))} />
        </Field>
      </FormSection>
    </div>

    <ContentCard title="成功配置备份" description="Agent 自动上报成功生效参数；此份副本只读，只有明确发起恢复才会使用。">
      {backup ? <div className="grid gap-space-4 sm:grid-cols-3">
        <DetailItem label="备份版本" value={`v${backup.configVersion}`} />
        <DetailItem label="生效时间" value={formatDateTime(backup.appliedAt)} />
        <DetailItem label="备份时间" value={formatDateTime(backup.receivedAt)} />
      </div> : <p className="text-sm text-text-secondary">{config.backupError ? `备份读取失败：${config.backupError}` : '尚无成功配置备份；Agent 上报后自动保存。'}</p>}
    </ContentCard>

    <div className="sticky bottom-0 z-10 space-y-space-3 rounded-admin border border-border bg-surface p-space-4 shadow-admin">
      {feedback}
      <div className="flex flex-wrap items-center justify-between gap-space-3">
        <p className="text-body-secondary text-text-secondary" aria-live="polite">{busy ? '正在校验、保存、重载并核实设备，请稍候…' : '一次提交会完成校验、持久化、重载、核实与失败回退。'}</p>
        <div className="flex flex-wrap gap-space-2">
          <Button disabled={busy || !backup} onClick={onRestore}><RotateCcw className="h-4 w-4" aria-hidden />恢复此备份</Button>
          <Button variant="primary" disabled={busy} onClick={onApply}><Check className="h-4 w-4" aria-hidden />保存并应用</Button>
        </div>
      </div>
    </div>
  </div>;
}
