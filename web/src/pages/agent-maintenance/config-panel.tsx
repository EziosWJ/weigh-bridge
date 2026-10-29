import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { Check } from 'lucide-react';
import type { CupsQueue, DeviceSettings, MaintenanceConfig } from '@/api/agent-maintenance';
import { Field } from '@/components/common/field';
import { FormSection } from '@/components/common/form-section';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';

const baudOptions = [1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200];

export function MaintenanceConfigPanel({ section, config, settings, setSettings, queues, queuesError, serialHint, busy, feedback, onApply }: {
  section: 'serial' | 'print';
  config: MaintenanceConfig;
  settings: DeviceSettings;
  setSettings: Dispatch<SetStateAction<DeviceSettings>>;
  queues: CupsQueue[];
  queuesError?: string;
  serialHint?: string;
  busy: boolean;
  feedback: ReactNode;
  onApply: () => void;
}) {
  const effective = config.effective;
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
    {section === 'serial' && serialHint && <div role="alert" className="rounded-control border border-error-border bg-error-background p-space-3 text-sm text-error">{serialHint}</div>}

    {section === 'serial' && <div id="maintenance-config-serial" className="scroll-mt-20">
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
    </div>}

    {section === 'print' && <div id="maintenance-config-print" className="scroll-mt-20">
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
        <Field label="垂直偏移（mm）" htmlFor="maintenance-offset-y" help="正数向下、负数向上；上移不能超过内容居中留白，超出会拒绝。不作为累计走纸误差的替代修正。">
          <Input id="maintenance-offset-y" type="number" min={-30} max={30} disabled={busy}
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
    </div>}

    <div className="sticky bottom-0 z-10 space-y-space-3 rounded-admin border border-border bg-surface p-space-4 shadow-admin">
      {feedback}
      <div className="flex flex-wrap items-center justify-between gap-space-3">
        <p className="text-body-secondary text-text-secondary" aria-live="polite">{busy ? '正在校验、保存、重载并核实设备，请稍候…' : '保存会一并应用仪表与打印参数；需要留存时请到“配置备份”手动创建备份。'}</p>
        <div className="flex flex-wrap gap-space-2">
          <Button variant="primary" disabled={busy} onClick={onApply}><Check className="h-4 w-4" aria-hidden />保存并应用</Button>
        </div>
      </div>
    </div>
  </div>;
}
