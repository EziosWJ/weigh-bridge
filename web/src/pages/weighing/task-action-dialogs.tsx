import { useEffect, useMemo, useState } from 'react';
import { DetailItem } from '@/components/common/detail-item';
import { Field } from '@/components/common/field';
import { FormDialog } from '@/components/common/form-dialog';
import { toast } from '@/components/common/toast-store';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useTransportDirections } from '@/hooks/use-transport-directions';
import { beijingTime } from '@/lib/weighing/query';
import { formatWeight, weightInMilliTonnes, resolveNetAdjustments } from '@/lib/weighing/weights';
import { useWeighingStore } from '@/store/weighing-store';
import type { NetAdjustmentsInput, TransportTask } from '@/types/weighing';
import { NetAdjustmentFields } from './deduction-fields';

/** Business dates are Beijing wall-clock values in inputs and storage. */
function beijingInputValue(iso: string): string {
  return new Date(new Date(iso).getTime() + 8 * 3600_000).toISOString().slice(0, 16);
}
function fromBeijingInputValue(value: string): string {
  return new Date(`${value}${value.length === 16 ? ':00' : ''}+08:00`).toISOString();
}
function currentFirst(task: TransportTask) {
  return [...task.records].reverse().find(record => record.stage === 'first');
}

function ActionSummary({ task, first, secondWeight, returnedAt, estimatedNet }: {
  task: TransportTask;
  first?: ReturnType<typeof currentFirst>;
  secondWeight?: number;
  returnedAt?: string;
  estimatedNet?: number;
}) {
  const directionLabels = useTransportDirections();
  return <div className="grid gap-space-4 rounded-control border border-border bg-neutral-background p-space-3 sm:grid-cols-2">
    <DetailItem label="车牌" value={task.plate} />
    <DetailItem label="运输任务" value={task.taskNo ?? task.id} />
    <DetailItem label="方向" value={directionLabels[task.direction]} />
    <DetailItem label={`首磅重量${first ? `（${first.kind === 'gross' ? '毛重' : '皮重'}）` : ''} / 时间`} value={first ? `${formatWeight(first.weight)} · ${beijingTime(first.capturedAt ?? first.at)}` : '首磅依据缺失'} />
    {secondWeight !== undefined && <DetailItem label={`补录回磅重量（${task.direction === 'inbound' ? '皮重' : '毛重'}）`} value={formatWeight(secondWeight)} />}
    {returnedAt && <DetailItem label="实际回磅时间" value={beijingTime(returnedAt)} />}
    {estimatedNet !== undefined && <DetailItem label="预计净重" value={<strong className="tabular-nums text-text-primary">{formatWeight(estimatedNet)}</strong>} />}
  </div>;
}

export function ManualReturnDialog({ task, onClose, onDone }: {
  task: TransportTask | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const dispatch = useWeighingStore(state => state.dispatch);
  const [weight, setWeight] = useState('');
  const [returnedAt, setReturnedAt] = useState(() => beijingInputValue(new Date().toISOString()));
  const [reason, setReason] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [netAdjustments, setNetAdjustments] = useState<NetAdjustmentsInput>([]);
  const first = useMemo(() => task ? currentFirst(task) : undefined, [task]);

  useEffect(() => { if (task) { setWeight(''); setReturnedAt(beijingInputValue(new Date().toISOString())); setReason(''); setConfirmed(false); setError(''); setNetAdjustments([]); } }, [task]);

  let entered: number | undefined;
  let net: number | undefined;
  let weightError = '';
  try {
    if (weight.trim() === '') throw new Error('请填写回磅重量');
    const value = weightInMilliTonnes(Number(weight)) / 1000;
    if (value <= 0) throw new Error('回磅重量必须大于零');
    entered = value;
    if (first) {
      const calculatedGross = task?.direction === 'outbound' ? value : first.weight;
      const calculatedTare = task?.direction === 'outbound' ? first.weight : value;
      if (calculatedGross <= calculatedTare) throw new Error('原始毛重必须大于原始皮重，请核对运输方向与重量，不能自动交换毛皮重');
      net = (weightInMilliTonnes(calculatedGross) - weightInMilliTonnes(calculatedTare)) / 1000;
    }
  } catch (failure) {
    weightError = failure instanceof Error ? failure.message : '请核对回磅重量';
    net = undefined;
  }

  let adjustmentBasis: { gross: number; tare: number; net: number } | undefined;
  let adjustmentError = '';
  // Inbound: gross is the first weighing, tare is the manual entry; outbound
  // reverses them. net above is already gross minus tare.
  if (net !== undefined && first && task) {
    const gross = task.direction === 'outbound' ? entered! : first.weight;
    const tare = task.direction === 'outbound' ? first.weight : entered!;
    adjustmentBasis = { gross, tare, net };
    try { resolveNetAdjustments(adjustmentBasis, netAdjustments); } catch (failure) { adjustmentError = failure instanceof Error ? failure.message : '请核对净重调整'; }
  }

  let returnedIso = '';
  let timeError = '';
  try {
    returnedIso = fromBeijingInputValue(returnedAt);
    if (first) {
      const actual = new Date(returnedIso);
      const firstAt = new Date(first.capturedAt ?? first.at);
      if (actual.getTime() < firstAt.getTime()) throw new Error('实际回磅时间不能早于有效首磅时间');
      if (actual.getTime() > Date.now()) throw new Error('实际回磅时间不能晚于当前时间');
    }
  } catch (failure) {
    timeError = failure instanceof Error ? failure.message : '请填写有效的实际回磅时间';
  }

  const ready = !!task && !!first && !!entered && !weightError && !timeError && !adjustmentError && reason.trim() !== '' && confirmed;

  async function submit() {
    if (!task || !ready) { setError(weightError || timeError || (reason.trim() === '' ? '请填写补录原因' : '请核对并明确确认人工补录')); return; }
    setLoading(true); setError('');
    try {
      await dispatch({ type: 'manualReturn', taskId: task.id, weight: String(entered), returnedAt: returnedIso, reason: reason.trim(), netAdjustments });
      if (useWeighingStore.getState().data.pending) {
        toast.warning({ title: '提交结果待核实', description: '请先查询原请求，再继续办理。', duration: 6000 });
      } else {
        toast.success('人工补录回磅已完成，原任务已出单');
      }
      onDone();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : '补录失败');
    } finally { setLoading(false); }
  }

  return <FormDialog open={!!task} title="人工补录回磅" description="人工录入回磅重量完成原任务并出单；不要求上传照片，系统也不会保存照片依据。" onCancel={onClose} onSubmit={submit} submitDisabled={!ready || loading} submitText="确认人工补录" loading={loading}>
    {task && <div className="space-y-space-4">
      <ActionSummary task={task} first={first} secondWeight={entered} returnedAt={returnedIso || undefined} estimatedNet={net} />
      <div className="rounded-control border border-border px-space-3 py-space-2 text-sm"><span className="text-text-tertiary">系统补录时间：</span>提交时由服务器记录，与实际回磅时间分开保存。</div>
      <p className="text-sm text-text-secondary">补录重量标记为“人工补录”，不是设备采集值；系统不伪造设备报文、稳定状态或抓拍结果。</p>
      <Field label="回磅重量（吨）" htmlFor="manual-return-weight" required error={weightError} help="精确到 0.001 吨；按原运输方向确定毛重和皮重。">
        <Input id="manual-return-weight" type="number" inputMode="decimal" min="0.001" step="0.001" value={weight} onChange={event => setWeight(event.target.value)} />
      </Field>
      <Field label="实际回磅时间（北京时间）" htmlFor="manual-return-time" required error={timeError} help="不早于有效首磅时间，不晚于当前时间；与系统补录时间分别保存。">
        <Input id="manual-return-time" type="datetime-local" step="1" value={returnedAt} onChange={event => setReturnedAt(event.target.value)} />
      </Field>
      {adjustmentBasis && <NetAdjustmentFields basis={adjustmentBasis} value={netAdjustments} onChange={setNetAdjustments} showError={false} />}
      <Field label="补录原因" htmlFor="manual-return-reason" required>
        <Textarea id="manual-return-reason" rows={3} value={reason} onChange={event => setReason(event.target.value)} placeholder="例如：车辆已回磅离开，现场已拍下地磅读数，经核对后补录" />
      </Field>
      <label className="flex items-start gap-space-2 text-sm">
        <input type="checkbox" className="mt-0.5" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />
        <span>我已核对车牌、首磅依据、实际回磅重量和时间，确认由我人工补录；此操作将完成原任务并生成正式单据。</span>
      </label>
      {error && <p role="alert" className="text-error">{error}</p>}
    </div>}
  </FormDialog>;
}

export function TerminateTaskDialog({ task, onClose, onDone }: {
  task: TransportTask | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const dispatch = useWeighingStore(state => state.dispatch);
  const [reason, setReason] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const first = useMemo(() => task ? currentFirst(task) : undefined, [task]);

  useEffect(() => { if (task) { setReason(''); setConfirmed(false); setError(''); } }, [task]);

  async function submit() {
    if (!task || reason.trim() === '' || !confirmed) { setError(reason.trim() === '' ? '请填写终止原因' : '请确认终止不可恢复'); return; }
    setLoading(true); setError('');
    try {
      await dispatch({ type: 'terminate', taskId: task.id, reason: reason.trim() });
      if (useWeighingStore.getState().data.pending) {
        toast.warning({ title: '提交结果待核实', description: '请先查询原请求，再继续办理。', duration: 6000 });
      } else {
        toast.success('运输任务已终止，车牌已释放');
      }
      onDone();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : '终止失败');
    } finally { setLoading(false); }
  }

  return <FormDialog open={!!task} title="终止运输任务" description="终止后不可恢复；已有称重记录保留，不生成正式单据，车牌可办理新任务。" onCancel={onClose} onSubmit={submit} submitDisabled={reason.trim() === '' || !confirmed || loading} submitText="确认终止" loading={loading}>
    {task && <div className="space-y-space-4">
      <ActionSummary task={task} first={first} />
      <p className="text-error">终止后该任务不能恢复使用，也不能再按普通回磅出单；已有首磅记录和操作历史继续保留。</p>
      <Field label="终止原因" htmlFor="terminate-task-reason" required>
        <Textarea id="terminate-task-reason" rows={3} value={reason} onChange={event => setReason(event.target.value)} placeholder="例如：车辆已离开，现场确认不再回磅" />
      </Field>
      <label className="flex items-start gap-space-2 text-sm">
        <input type="checkbox" className="mt-0.5" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />
        <span>我已核对车牌和首磅依据，确认终止该任务且不可恢复。</span>
      </label>
      {error && <p role="alert" className="text-error">{error}</p>}
    </div>}
  </FormDialog>;
}
