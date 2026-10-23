import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { Check, Save } from 'lucide-react';
import type { MaintenanceConfig } from '@/api/agent-maintenance';
import { ContentCard } from '@/components/common/content-card';
import { DetailItem } from '@/components/common/detail-item';
import { Field } from '@/components/common/field';
import { FormSection } from '@/components/common/form-section';
import { StatusTag } from '@/components/common/status-tag';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { safeDraft } from './config-draft';

type ConfigField = { key: string; label: string; kind?: 'number' | 'boolean' | 'select' | 'secret'; min?: number; max?: number; options?: string[]; help?: string };
const connectionFields: ConfigField[] = [
  { key: 'serverUrl', label: '业务服务地址', help: '填写设备 Agent 可访问的本厂业务服务地址' },
  { key: 'agentId', label: 'Agent 编号' },
  { key: 'scaleId', label: '地磅编号', kind: 'number', min: 1 },
  { key: 'agentKey', label: '连接密钥', kind: 'secret', help: '已设置的密钥不会回显；留空表示保持原值' },
];
const serialFields: ConfigField[] = [
  { key: 'serialEnabled', label: '启用柯力 D12 仪表', kind: 'boolean' },
  { key: 'serialPort', label: '串口路径', help: '例如 /dev/ttyUSB0，使用 Agent 主机上的路径' },
  { key: 'serialBaud', label: '仪表波特率', kind: 'number', min: 1200, max: 115200 },
  { key: 'serialProtocol', label: '仪表协议', kind: 'select', options: ['keli-d12-tf05'] },
  { key: 'serialWeightUnit', label: '现场源单位', kind: 'select', options: ['t', 'kg'] },
  { key: 'serialStaleMs', label: '无有效帧故障阈值（ms）', kind: 'number', min: 200, max: 2999 },
];
const printFields: ConfigField[] = [
  { key: 'queue', label: 'CUPS 打印队列' },
  { key: 'printWidthMm', label: '打印宽度（mm）', kind: 'number', min: 100, max: 210 },
  { key: 'printScale', label: '整体缩放（倍）', kind: 'number', min: 0.5, max: 1.5 },
  { key: 'formLengthMm', label: '连续纸表长（mm）', kind: 'number', min: 85, max: 300 },
  { key: 'offsetXmm', label: '向右偏移（mm）', kind: 'number', min: 0, max: 30 },
  { key: 'offsetYmm', label: '向下偏移（mm）', kind: 'number', min: 0, max: 30 },
];
const cameraFields: ConfigField[] = [
  { key: 'cameraEnabled', label: '前后摄像头', kind: 'boolean', help: '停用后不能进行新的正式称重采集' },
  { key: 'cameraTimeoutMs', label: '抓拍超时（ms）', kind: 'number', min: 200, max: 10000 },
  { key: 'cameraRtspBase', label: 'RTSP 服务入口', help: '例如 rtsp://192.168.1.10:8554；不要填写密码或流名称' },
  { key: 'cameraWhepBase', label: '实时预览服务入口', help: '例如 http://192.168.1.10:8889；也可使用 HTTPS 入口' },
  { key: 'cameraFrontPath', label: '车前流名称', help: '例如 front，与 MediaMTX 中配置的名称一致' },
  { key: 'cameraRearPath', label: '车后流名称', help: '例如 rear，与车前流名称不同' },
];
const recognitionField: ConfigField = { key: 'plateRecognitionUrl', label: '车牌识别服务地址', kind: 'secret', help: '留空保持已保存值；不提供独立连通性测试' };
export function MaintenanceConfigPanel({ config, draft, setDraft, busy, feedback, onSave, onApply, onRestore }: {
  config: MaintenanceConfig;
  draft: Record<string, unknown>;
  setDraft: Dispatch<SetStateAction<Record<string, unknown>>>;
  busy: boolean;
  feedback: ReactNode;
  onSave: () => void;
  onApply: () => void;
  onRestore: () => void;
}) {
  const savedDraft = safeDraft(config.config);
  const dirty = Object.entries(draft).some(([key, value]) => value !== savedDraft[key]);

  function renderField(field: ConfigField) {
    const id = `maintenance-${field.key}`;
    const value = draft[field.key];
    return <Field key={field.key} label={field.label} htmlFor={id} help={field.help}>
      {field.kind === 'boolean' ? <Select id={id} value={String(Boolean(value))} disabled={busy} onChange={event => setDraft(current => ({ ...current, [field.key]: event.target.value === 'true' }))}>
        <option value="true">启用</option><option value="false">停用</option>
      </Select> : field.kind === 'select' ? <Select id={id} value={String(value ?? field.options?.[0] ?? '')} disabled={busy} onChange={event => setDraft(current => ({ ...current, [field.key]: event.target.value }))}>
        {field.options?.map(option => <option key={option} value={option}>{option === 't' ? '吨（t）' : option === 'kg' ? '千克（kg）' : option}</option>)}
      </Select> : <Input id={id} type={field.kind === 'number' ? 'number' : field.kind === 'secret' ? 'password' : 'text'} min={field.min} max={field.max}
        step={field.key === 'printScale' || field.key === 'formLengthMm' ? 'any' : undefined}
        autoComplete={field.kind === 'secret' ? 'new-password' : 'off'} disabled={busy} value={String(value ?? '')}
        onChange={event => setDraft(current => ({ ...current, [field.key]: field.kind === 'number' ? Number(event.target.value) : event.target.value }))} />}
    </Field>;
  }

  return <div className="space-y-space-4">
    <ContentCard title="配置状态" extra={<div className="shrink-0"><StatusTag tone={dirty || !config.saved ? 'warning' : 'success'}>{dirty ? '表单有未保存修改' : config.saved ? '没有待应用草稿' : '存在未应用草稿'}</StatusTag></div>}>
      <div className="grid gap-space-4 sm:grid-cols-2">
        <DetailItem label="当前生效情况" value={config.runtimeState ?? '运行配置状态未知'} />
        <DetailItem label="连接密钥" value={`${config.hasAgentKey ? '已设置（不回显）' : '未设置'}${config.keyChanged ? config.hasActiveAgentKey ? '，密钥待替换' : '，待首次应用' : ''}`} />
      </div>
      <p className="mt-space-4 border-t border-border pt-space-3 text-body-secondary text-text-secondary">保存草稿不会改变设备。保存并应用后将重载设备，需重新采集重量；已有打印作业仍须现场核实。</p>
    </ContentCard>
    <div className="flex flex-wrap items-center gap-x-space-4 gap-y-space-2 text-sm" aria-label="配置分组">
      <span className="text-text-tertiary">快速定位</span>
      {[['connection', '服务连接'], ['serial', '称重仪表'], ['print', '打印与校准'], ['camera', '摄像头与识别']].map(([id, label]) => <a key={id} href={`#maintenance-config-${id}`} className="rounded text-primary underline-offset-4 hover:underline">{label}</a>)}
    </div>
    <div id="maintenance-config-connection" className="scroll-mt-20">
      <FormSection title="服务连接" description="设备身份与业务服务连接；局域网分机部署时使用可访问的业务服务地址。">{connectionFields.map(renderField)}</FormSection>
    </div>
    <div id="maintenance-config-serial" className="scroll-mt-20">
      <FormSection title="称重仪表" description="参数需与现场仪表一致，串口由 Agent 独占。">{serialFields.map(renderField)}</FormSection>
    </div>
    <div id="maintenance-config-print" className="scroll-mt-20">
      <FormSection title="打印与校准" description="调整纸张尺寸及偏移后，保存并应用，再到「设备与打印」打印校准样张。">{printFields.map(renderField)}</FormSection>
    </div>
    <div id="maintenance-config-camera" className="scroll-mt-20">
      <FormSection title="摄像头与识别" description="前后流名称同时用于实时预览和称重抓拍；维护测试使用已生效配置。">
        {draft.cameraMode !== 'mediamtx' ? <div className="space-y-space-3 md:col-span-2">
          <p className="text-sm text-text-secondary">当前沿用旧抓拍配置，地址不回显。切换后仍须保存并应用。</p>
          <Button disabled={busy} onClick={() => setDraft(current => ({ ...current, cameraMode: 'mediamtx', cameraEnabled: true, cameraRtspBase: 'rtsp://127.0.0.1:8554', cameraWhepBase: `${location.protocol}//${location.hostname}:8889`, cameraFrontPath: 'front', cameraRearPath: 'rear', cameraTimeoutMs: 6000 }))}>切换为 MediaMTX 配置</Button>
        </div> : <>
          {cameraFields.map(renderField)}
          <details className="min-w-0 rounded-control border border-border bg-neutral-background p-space-3 text-body-secondary md:col-span-2">
            <summary className="cursor-pointer font-medium">查看草稿中的抓拍与预览地址</summary>
            <div className="mt-space-3 space-y-space-2 break-all text-text-secondary">
              {(['front', 'rear'] as const).map(view => {
                const path = String(draft[view === 'front' ? 'cameraFrontPath' : 'cameraRearPath'] ?? '');
                const name = view === 'front' ? '车前' : '车后';
                return <div key={view}><p>{name}抓拍：{String(draft.cameraRtspBase ?? '').replace(/\/$/, '')}/{path}</p><p>{name}预览：{String(draft.cameraWhepBase ?? '').replace(/\/$/, '')}/{path}/whep</p></div>;
              })}
              <p>以上为草稿地址；凭据由服务管理，不在页面显示。</p>
            </div>
          </details>
        </>}
        <div className="border-t border-border pt-space-4 md:col-span-2">{renderField(recognitionField)}</div>
      </FormSection>
    </div>
    <div className="sticky bottom-0 z-10 space-y-space-3 rounded-admin border border-border bg-surface p-space-4 shadow-admin">
      {feedback}
      <div className="flex flex-wrap items-center justify-between gap-space-3">
        <p className="text-body-secondary text-text-secondary" aria-live="polite">{busy ? '正在处理，请稍候…' : dirty ? '当前修改尚未保存' : '保存草稿后，需应用才会生效'}</p>
        <div className="flex flex-wrap gap-space-2">
          <Button disabled={busy || !config.hasPrevious} onClick={onRestore}>恢复上一份配置</Button>
          <Button disabled={busy} onClick={onSave}><Save className="h-4 w-4" aria-hidden />保存草稿</Button>
          <Button variant="primary" disabled={busy} onClick={onApply}><Check className="h-4 w-4" aria-hidden />保存并应用配置</Button>
        </div>
      </div>
    </div>
  </div>;
}
