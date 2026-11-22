import { hasAdjustmentInput } from '@/lib/weighing/weights';
import { useEffect, useMemo, useState } from 'react';
import { WorkbenchLink } from '@/components/common/workbench-link';
import { DetailItem } from '@/components/common/detail-item';
import { HelpHint } from '@/components/common/help-hint';
import { Field } from '@/components/common/field';
import { FormDialog } from '@/components/common/form-dialog';
import { ConfirmDialog } from '@/components/common/confirm-dialog';
import { toast } from '@/components/common/toast-store';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useTransportDirections } from '@/hooks/use-transport-directions';
import { beijingTime } from '@/lib/weighing/query';
import { formatWeight, weightInMilliTonnes, resolveNetAdjustments } from '@/lib/weighing/weights';
import { useWeighingStore } from '@/store/weighing-store';
import { activeWeighRecord, type NetAdjustmentsInput, type TransportTask } from '@/types/weighing';
import { NetAdjustmentFields } from './deduction-fields';

/** Business dates are Beijing wall-clock values in inputs and storage. */
function fromBeijingInputValue(value: string): string {
  return new Date(`${value}${value.length === 16 ? ':00' : ''}+08:00`).toISOString();
}
function currentFirst(task: TransportTask) {
  return activeWeighRecord(task.records, 'first');
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
    {estimatedNet !== undefined && <DetailItem label="预计出单净重" value={<strong className="tabular-nums text-text-primary">{formatWeight(estimatedNet)}</strong>} />}
  </div>;
}

export function ManualReturnDialog({ task, onClose, onDone }: {
  task: TransportTask | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const dispatch = useWeighingStore(state => state.dispatch);
  const pending = useWeighingStore(state => state.data.pending);
  const [confirmClose, setConfirmClose] = useState(false);
  const [fieldError, setFieldError] = useState('');
  const [weight, setWeight] = useState('');
  const [returnedAt, setReturnedAt] = useState('');
  const [reason, setReason] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [netAdjustments, setNetAdjustments] = useState<NetAdjustmentsInput>([]);
  const first = useMemo(() => task ? currentFirst(task) : undefined, [task]);

  useEffect(() => { if (task) { setWeight(''); setReturnedAt(''); setReason(''); setConfirmed(false); setError(''); setFieldError(''); setNetAdjustments([]); } }, [task]);

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
  let finalNet: number | undefined;
  // Inbound: gross is the first weighing, tare is the manual entry; outbound
  // reverses them. net above is already gross minus tare.
  if (net !== undefined && first && task) {
    const gross = task.direction === 'outbound' ? entered! : first.weight;
    const tare = task.direction === 'outbound' ? first.weight : entered!;
    adjustmentBasis = { gross, tare, net };
    try {
      const { amount } = resolveNetAdjustments(adjustmentBasis, netAdjustments);
      finalNet = (weightInMilliTonnes(net) - weightInMilliTonnes(amount)) / 1000;
    } catch (failure) {
      adjustmentError = failure instanceof Error ? failure.message : '请核对净重调整';
    }
  }

  let returnedIso = '';
  let timeError = '';
  try {
    if (!returnedAt) throw new Error('请填写实际回磅时间');
    const iso = fromBeijingInputValue(returnedAt);
    if (first) {
      const actual = new Date(iso);
      const firstAt = new Date(first.capturedAt ?? first.at);
      if (actual.getTime() < firstAt.getTime()) throw new Error('实际回磅时间不能早于有效首磅时间');
      if (actual.getTime() > Date.now()) throw new Error('实际回磅时间不能晚于当前时间');
    }
    returnedIso = iso;
  } catch (failure) {
    timeError = failure instanceof RangeError ? '请填写有效的实际回磅时间' : failure instanceof Error ? failure.message : '请填写有效的实际回磅时间';
  }

  const ready = !pending && task?.status === 'second' && !!first && !!entered && !weightError && !timeError && !adjustmentError && !fieldError && reason.trim() !== '' && confirmed;

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

  const requestClose = () => {
    if (weight || returnedAt || reason || confirmed || hasAdjustmentInput(netAdjustments)) setConfirmClose(true);
    else onClose();
  };
  return <><FormDialog open={!!task} title="人工补录回磅" onCancel={requestClose} trapFocus={!confirmClose} closeOnEscape={!confirmClose} closeOnOverlayClick={!confirmClose} onSubmit={submit} submitDisabled={!ready || loading} submitText="确认补录并出单" loading={loading}>
    {task && <div className="space-y-space-4">
      <ActionSummary task={task} first={first} secondWeight={entered} returnedAt={returnedIso || undefined} estimatedNet={fieldError ? undefined : finalNet} />
      <Field label={task.direction === 'inbound' ? '补录皮重（吨）' : '补录毛重（吨）'} htmlFor="manual-return-weight" required error={weight ? weightError : undefined}>
        <Input id="manual-return-weight" type="number" inputMode="decimal" min="0.001" step="0.001" value={weight} onChange={event => setWeight(event.target.value)} />
      </Field>
      <div className="space-y-1.5">
        <div className="flex items-center gap-1">
          <label htmlFor="manual-return-time" className="text-form-label font-medium">实际回磅时间（北京时间） <span className="text-error">*</span></label>
          <HelpHint label="回磅时间规则">按现场记录填写实际时间，须介于有效首磅和当前时间之间。系统补录时间可在称重记录中查看。</HelpHint>
        </div>
        <Input id="manual-return-time" type="datetime-local" step="1" required value={returnedAt}
          aria-invalid={!!returnedAt && !!timeError} aria-describedby={returnedAt && timeError ? 'manual-return-time-error' : undefined}
          onChange={event => setReturnedAt(event.target.value)} />
        {returnedAt && timeError && <p id="manual-return-time-error" role="alert" className="text-helper text-error">{timeError}</p>}
      </div>
      {adjustmentBasis && <NetAdjustmentFields basis={adjustmentBasis} value={netAdjustments} onChange={setNetAdjustments} onValidationChange={setFieldError} />}
      <Field label="补录原因" htmlFor="manual-return-reason" required>
        <Textarea id="manual-return-reason" rows={3} value={reason} onChange={event => setReason(event.target.value)} />
      </Field>
      <label className="flex items-start gap-space-2 text-sm">
        <input type="checkbox" className="mt-0.5" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />
        <span>已核对现场称重记录</span>
      </label>
      {pending?.taskId === task.id
        ? <p role="alert" className="text-warning">提交结果待核实。<WorkbenchLink className="text-primary hover:underline" to={`/weighing/workbench?task=${encodeURIComponent(task.id)}`}>查询原请求</WorkbenchLink></p>
        : error && <p role="alert" className="text-error">{error}</p>}
    </div>}
  </FormDialog><ConfirmDialog open={confirmClose} title="放弃本次补录？" confirmText="放弃并关闭" cancelText="返回修改" onConfirm={() => { setConfirmClose(false); onClose(); }} onCancel={() => setConfirmClose(false)} /></>;
}

export function TerminateTaskDialog({ task, onClose, onDone }: {
  task: TransportTask | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const dispatch = useWeighingStore(state => state.dispatch);
  const pending = useWeighingStore(state => state.data.pending);
  const [reason, setReason] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const first = useMemo(() => task ? currentFirst(task) : undefined, [task]);

  useEffect(() => { if (task) { setReason(''); setConfirmed(false); setError(''); } }, [task]);

  async function submit() {
    if (!task || pending || reason.trim() === '' || !confirmed) { setError(reason.trim() === '' ? '请填写终止原因' : '请确认终止不可恢复'); return; }
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

  return <FormDialog open={!!task} title="终止运输任务" onCancel={onClose} onSubmit={submit} submitDisabled={!!pending || reason.trim() === '' || !confirmed || loading} submitText="确认终止" loading={loading}>
    {task && <div className="space-y-space-4">
      <ActionSummary task={task} first={first} />
      <Field label="终止原因" htmlFor="terminate-task-reason" required>
        <Textarea id="terminate-task-reason" rows={3} value={reason} onChange={event => setReason(event.target.value)} />
      </Field>
      <label className="flex items-start gap-space-2 text-sm">
        <input type="checkbox" className="mt-0.5" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />
        <span>已核对本趟任务，确认终止后不可恢复</span>
      </label>
      {pending?.taskId === task.id
        ? <p role="alert" className="text-warning">提交结果待核实。<WorkbenchLink className="text-primary hover:underline" to={`/weighing/workbench?task=${encodeURIComponent(task.id)}`}>查询原请求</WorkbenchLink></p>
        : error && <p role="alert" className="text-error">{error}</p>}
    </div>}
  </FormDialog>;
}
