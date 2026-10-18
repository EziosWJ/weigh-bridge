import { WorkbenchLink } from "@/components/common/workbench-link";
import { useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { ConfirmDialog } from '@/components/common/confirm-dialog';
import { ContentCard } from '@/components/common/content-card';
import { SectionTabs } from '@/components/common/section-tabs';
import { Field } from '@/components/common/field';
import { PageHeader } from '@/components/common/page-header';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { cameraPresets, getCameraPreset } from '@/lib/weighing/camera';
import { formatWeightInputValue, formatWeightValue } from '@/lib/weighing/weights';
import { useWeighingStore } from '@/store/weighing-store';
import { roleLabels, type DemoConfig, type WeighingCommand } from '@/types/weighing';
import { RecognitionSettings } from './recognition-settings';
import { DemoBanner } from './shared';

const switches: { key: 'stable' | 'timedOut' | 'deviceError' | 'online' | 'evidence'; label: string }[] = [
  { key: 'stable', label: '模拟重量稳定' },
  { key: 'timedOut', label: '模拟数据超时' },
  { key: 'deviceError', label: '模拟设备异常' },
  { key: 'online', label: '模拟在线' },
  { key: 'evidence', label: '模拟证据可用' },
];

function SimulatedWeightInput({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  const [display, setDisplay] = useState(() => formatWeightInputValue(value));
  const pendingValue = useRef<number | null>(null);
  useEffect(() => {
    if (pendingValue.current !== null && Object.is(pendingValue.current, value)) {
      pendingValue.current = null;
      return;
    }
    setDisplay(formatWeightInputValue(value));
  }, [value]);
  return <Input id="demo-weight" type="number" inputMode="decimal" step="0.001" value={display} onChange={event => {
    const next = event.target.value === '' ? Number.NaN : event.target.valueAsNumber;
    setDisplay(event.target.value);
    pendingValue.current = next;
    onChange(next);
  }} onBlur={() => setDisplay(formatWeightInputValue(value))} />;
}

export function DemoPanel() {
  const { data, dispatch } = useWeighingStore();
  const [feedback, setFeedback] = useState('');
  const [reason, setReason] = useState('');
  const [resetOpen, setResetOpen] = useState(false);
  const run = async (command: WeighingCommand, success: string) => {
    try {
      await dispatch(command);
      setFeedback(success);
      return true;
    } catch (error) {
      setFeedback(error instanceof Error ? error.message : '操作未完成，请检查演示条件');
      return false;
    }
  };
  const configure = (patch: Partial<DemoConfig>) => run({ type: 'config', patch }, '演示配置已更新，请重新采集并核对重量。');
  const camera = getCameraPreset(data.config.cameraPreset);

  return (
    <ContentCard title="模拟设备与场景" description="仅影响当前浏览器的称重测试。切换岗位不会改变真实登录账号或权限。">
        <SectionTabs label="称重演示设置" items={[
          { value: 'settings', label: '模拟配置', content: <div className="space-y-5">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Field label="演示岗位" htmlFor="demo-role">
              <Select id="demo-role" value={data.config.role} onChange={(event) => configure({ role: event.target.value as DemoConfig['role'] })}>
                {Object.entries(roleLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </Select>
            </Field>
            <Field label="模拟重量（吨）" htmlFor="demo-weight" help="步长 0.001 吨；允许输入异常值以验证确认拦截。">
              <SimulatedWeightInput value={data.config.weight} onChange={weight => configure({ weight })} />
              <p className="mt-1 text-xs text-text-tertiary">当前显示：{formatWeightValue(data.config.weight)} 吨</p>
            </Field>
            <Field label="模拟提交结果" htmlFor="demo-submission">
              <Select id="demo-submission" value={data.config.submission} onChange={(event) => configure({ submission: event.target.value as DemoConfig['submission'] })}>
                <option value="normal">正常返回</option>
                <option value="unknown-success">实际成功，结果不明</option>
                <option value="unknown-failure">实际失败，结果不明</option>
              </Select>
            </Field>
            <Field label="模拟打印结果" htmlFor="demo-print">
              <Select id="demo-print" value={data.config.printOutcome} onChange={(event) => configure({ printOutcome: event.target.value as DemoConfig['printOutcome'] })}>
                <option value="printed">已模拟出纸</option>
                <option value="failed">明确失败且未出纸</option>
                <option value="unknown">结果未知，需核实</option>
              </Select>
            </Field>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {switches.map(({ key, label }) => (
              <div key={key} className="flex items-center justify-between gap-3 rounded-control border border-border p-3">
                <span id={`demo-${key}-label`} className="text-sm">{label}</span>
                <Switch aria-labelledby={`demo-${key}-label`} checked={data.config[key]} onCheckedChange={(checked) => configure({ [key]: checked })} />
              </div>
            ))}
          </div>
          <div className="grid items-start gap-4 sm:grid-cols-2">
            <Field label="模拟摄像头画面" htmlFor="demo-camera" help="两幅为独立示例，不是同一车辆的多机位画面。">
              <Select id="demo-camera" aria-label="模拟摄像头画面" value={camera.id} onChange={(event) => configure({ cameraPreset: event.target.value as DemoConfig['cameraPreset'] })}>
                {cameraPresets.map((preset) => <option key={preset.id} value={preset.id}>{preset.title}</option>)}
              </Select>
              <p className="mt-2 text-sm text-text-tertiary">摄像头未接入，画面来自 AI 生成示例，非实时、非现场抓拍。“模拟证据可用”控制是否可生成模拟截图；修改配置后原采集失效。</p>
            </Field>
            <figure className="max-w-sm space-y-2">
              <img src={camera.src} alt={`${camera.title}模拟摄像头示例，非现场抓拍`} className="aspect-video w-full rounded-control border border-border object-cover" />
              <figcaption className="text-sm text-text-tertiary">{camera.title} · 模拟画面预览</figcaption>
            </figure>
          </div>
          <RecognitionSettings />
          <div className="border-t border-border pt-4">
            <p className="mb-3 text-sm">现场操作占用：<strong>{data.owner === 'self' ? '当前演示会话' : data.owner === 'other' ? '其他演示会话' : '未占用'}</strong></p>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" onClick={() => run({ type: 'occupy', owner: 'self' }, '当前演示会话已占用入口。')}>当前会话占用</Button>
              <Button size="sm" onClick={() => run({ type: 'occupy', owner: 'other' }, '已模拟其他会话占用。')}>模拟其他会话占用</Button>
              <Button size="sm" onClick={() => run({ type: 'occupy', owner: null }, '已释放演示占用。')}>释放占用</Button>
            </div>
            {data.config.role !== 'admin' && data.owner === 'other' && (
              <div className="mt-3 flex flex-wrap items-start gap-2">
                <div className="min-w-64 flex-1"><Field label="操作员接管原因" htmlFor="takeover-reason" required><Input id="takeover-reason" value={reason} onChange={(event) => setReason(event.target.value)} /></Field></div>
                <Button className="mt-6" disabled={!reason.trim()} onClick={() => run({ type: 'takeover', reason }, '操作员已接管，记录已保留。')}>接管入口</Button>
              </div>
            )}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
            <p className="text-sm text-text-tertiary">浏览器自动保存；重置恢复本模块示例，不清理登录或其他模块数据。</p>
            {data.config.role === 'admin' ? <Button variant="danger" size="sm" onClick={() => setResetOpen(true)}>重置称重测试数据</Button> : <span className="text-sm text-text-tertiary">切换管理员岗位可重置</span>}
          </div>
          {feedback && <p role="status" className="text-sm text-text-secondary">{feedback}</p>}
          </div> },
          { value: 'history', label: `演示操作历史（${data.history.length}）`, content: <ol className="max-h-64 space-y-2 overflow-y-auto text-sm">
            {data.history.slice().reverse().map((entry, index) => <li key={`${entry.at}-${index}`} className="border-b border-border pb-2"><time>{new Date(entry.at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })}</time> · {roleLabels[entry.role]} · {entry.action}{entry.reason ? ` · ${entry.reason}` : ''}</li>)}
            {!data.history.length && <li className="text-text-tertiary">暂无演示操作记录</li>}
          </ol> },
        ]} />
      <ConfirmDialog open={resetOpen} title="重置称重测试数据？" description="本模块新增的任务、称重、测试单和演示历史将恢复为预置示例。现有登录和其他模块数据不受影响。" danger confirmText="确认重置" onCancel={() => setResetOpen(false)} onConfirm={async () => { if (await run({ type: 'reset' }, '已恢复称重示例数据。')) setResetOpen(false); }} />
    </ContentCard>
  );
}

export function DemoSettingsPage() {
  const [params] = useSearchParams();
  const requestedReturn = params.get('returnTo');
  const returnTo = requestedReturn?.startsWith('/weighing/') ? requestedReturn : '/weighing/workbench';
  return <div className="space-y-6">
    <PageHeader title="演示设置" description="模拟重量、岗位与异常场景集中设置。返回作业后草稿保留，需重新采集和人工核对。" actions={(returnTo.split('?')[0] === '/weighing/workbench' ? <WorkbenchLink className="text-sm text-primary hover:underline" to={returnTo}>返回称重作业</WorkbenchLink> : <Link className="text-sm text-primary hover:underline" to={returnTo}>返回称重作业</Link>)} />
    <DemoBanner />
    <DemoPanel />
  </div>;
}
