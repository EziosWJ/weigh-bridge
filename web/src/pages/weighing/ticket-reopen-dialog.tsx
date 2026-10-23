import { Link, useNavigate } from 'react-router-dom';
import { useEffect, useRef, useState } from 'react';
import { ConfirmDialog } from '@/components/common/confirm-dialog';
import { FormDialog } from '@/components/common/form-dialog';
import { Field } from '@/components/common/field';
import { Input } from '@/components/ui/input';
import { plateError } from '@/lib/weighing/plate';
import { adjustedWeights, formatWeight, originalWeights } from '@/lib/weighing/weights';
import { useWeighingStore } from '@/store/weighing-store';
import { emptyTaskInput, type FinalWeightInput, type TaskInput } from '@/types/weighing';
import { BusinessFields } from './shared';
import { WeightDeductionFields } from './deduction-fields';

const linkClass = 'text-primary hover:underline';

export function TicketReopenDialog({ ticketId, onClose }: { ticketId: string | null; onClose: () => void }) {
  const navigate = useNavigate();
  const data = useWeighingStore(state => state.data);
  const dispatch = useWeighingStore(state => state.dispatch);
  const loadTask = useWeighingStore(state => state.loadTask);
  const loadTicket = useWeighingStore(state => state.loadTicket);
  const [loading, setLoading] = useState(false);
  const [ready, setReady] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [input, setInput] = useState<TaskInput>(emptyTaskInput);
  const [reason, setReason] = useState('');
  const [finalWeights, setFinalWeights] = useState<FinalWeightInput>({ gross: 0, tare: 0 });
  const [error, setError] = useState('');
  const basis = useRef(data);
  const lastLoaded = useRef<string | null>(null);

  useEffect(() => {
    if (!ticketId) {
      lastLoaded.current = null;
      setReady(false);
      return;
    }
    if (lastLoaded.current === ticketId) return;
    lastLoaded.current = ticketId;
    let cancelled = false;
    setLoading(true);
    setReady(false);
    setError('');
    setConfirming(false);
    setSubmitting(false);
    setReason('');
    void (async () => {
      const store = useWeighingStore.getState();
      let ticket = store.data.tickets.find(item => item.id === ticketId);
      if (!ticket) {
        await store.loadTicket(ticketId);
        ticket = useWeighingStore.getState().data.tickets.find(item => item.id === ticketId);
      }
      if (!ticket) throw new Error('未找到该单据，请刷新后重试。');
      await useWeighingStore.getState().loadTask(ticket.taskId);
      if (cancelled) return;
      const current = useWeighingStore.getState().data;
      const loadedTicket = current.tickets.find(item => item.id === ticketId) ?? ticket;
      const task = current.tasks.find(item => item.id === loadedTicket.taskId);
      const original = task ? originalWeights(task.records) : undefined;
      const changed = !!task && JSON.stringify(task.records) !== JSON.stringify(loadedTicket.records);
      setInput({
        plate: loadedTicket.plate,
        direction: loadedTicket.direction,
        materialId: loadedTicket.materialId,
        partyId: loadedTicket.partyId,
        carrierId: loadedTicket.carrierId ?? '',
        contactName: loadedTicket.contactName ?? '',
        contactPhone: loadedTicket.contactPhone ?? '',
        driver: loadedTicket.driver,
        driverPhone: loadedTicket.driverPhone ?? '',
        remark: loadedTicket.remark,
      });
      setFinalWeights({
        gross: changed && original ? original.gross : loadedTicket.gross,
        tare: changed && original ? original.tare : loadedTicket.tare,
        reason: '',
      });
      basis.current = current;
    })().catch(failure => {
      if (!cancelled) {
        lastLoaded.current = null;
        setError(failure instanceof Error ? failure.message : '重开资料加载失败');
      }
    }).finally(() => {
      if (!cancelled) {
        setLoading(false);
        setReady(true);
      }
    });
    return () => { cancelled = true; };
  }, [ticketId, loadTask, loadTicket]);

  const ticket = ticketId ? data.tickets.find(item => item.id === ticketId) : undefined;
  const task = ticket ? data.tasks.find(item => item.id === ticket.taskId) : undefined;
  const currentOriginal = task ? originalWeights(task.records) : undefined;
  const successors = ticket ? data.tickets.filter(item => item.previousId === ticket.id) : [];
  const canReopen = !!ticket && ticket.status === 'void' && successors.length === 0 && !data.tickets.some(item => item.taskId === ticket.taskId && item.status === 'valid');

  function requestConfirmation() {
    if (!ready || loading || submitting) return;
    if (!ticket || !task) {
      setError('重开资料仍在加载，请稍候。');
      return;
    }
    if (!canReopen) {
      setError('该单据已有更正单，或同任务已有有效单据，请沿关联记录办理。');
      return;
    }
    if (input.plate !== ticket.plate && plateError(input.plate)) {
      setError(plateError(input.plate));
      return;
    }
    if (!reason.trim()) {
      setError('请填写操作原因 / 核实说明。');
      return;
    }
    if (currentOriginal) {
      try {
        adjustedWeights(currentOriginal, finalWeights);
      } catch (failure) {
        setError(failure instanceof Error ? failure.message : '请核对最终毛重和皮重。');
        return;
      }
    }
    setError('');
    setConfirming(true);
  }

  async function submitConfirmed() {
    if (submitting) return;
    if (!ticket || !task || !canReopen) {
      setConfirming(false);
      setError('该单据已有更正单，或同任务已有有效单据，请沿关联记录办理。');
      return;
    }
    setSubmitting(true);
    try {
      // The two-basis confirmation is captured by the second dialog. Only a
      // confirmed save may submit basisValid=true; cancelling never sends.
      await dispatch({
        type: 'reopenTicket',
        ticketId: ticket.id,
        input,
        reason: reason.trim(),
        evidenceValid: true,
        finalWeights,
      }, basis.current);
      const latest = useWeighingStore.getState().data;
      setConfirming(false);
      onClose();
      if (latest.pending) {
        navigate(`/weighing/workbench?task=${encodeURIComponent(ticket.taskId)}`);
        return;
      }
      const reopened = latest.tickets.find(item => item.previousId === ticket.id);
      if (reopened) navigate(`/weighing/tickets/${encodeURIComponent(reopened.id)}`);
    } catch (failure) {
      const pending = useWeighingStore.getState().data.pending;
      setConfirming(false);
      setError(pending
        ? '提交结果未知，请前往工作台查询原结果，避免重复提交。'
        : failure instanceof Error ? failure.message : '重开失败');
    } finally {
      setSubmitting(false);
    }
  }

  return <>
    <FormDialog
      open={!!ticketId}
      title="关联更正重开"
      loading={submitting}
      submitDisabled={!ready || loading || !ticket || !task || !canReopen}
      submitText="确认保存"
      onCancel={onClose}
      onSubmit={requestConfirmation}
      closeOnEscape={!confirming}
      trapFocus={!confirming}
    >
      <div className="space-y-space-4">
        {(!ready || loading) && ticketId && <p role="status" className="text-text-secondary">正在加载原单与称重依据…</p>}
        {ready && !loading && !ticket && <p role="alert" className="text-error">{error || '未找到该单据，请刷新后重试。'}</p>}
        {ticket && ready && !loading && <>
          <p>对照原始称重录入更正后的最终毛皮重，系统计算调整差额；原单及采集依据保留，并关联原单，同任务最多一张有效单据。</p>
          {!canReopen && <p role="alert" className="text-error">该单据已有更正单，或同任务已有有效单据，请沿关联记录办理。</p>}
          <BusinessFields value={input} onChange={setInput} lockDirection />
          <p className="text-sm">上版最终毛／皮／净重：{formatWeight(ticket.gross)} / {formatWeight(ticket.tare)} / {formatWeight(ticket.net)}</p>
          <Link className={linkClass} to={`/weighing/tasks/${encodeURIComponent(ticket.taskId)}`}>需要实际单磅重称：前往原任务采集，完成后返回本单重开</Link>
          {currentOriginal && <WeightDeductionFields original={currentOriginal} value={finalWeights} onChange={setFinalWeights} />}
        </>}
        <Field label="操作原因 / 核实说明" htmlFor="ticket-operation-reason" required><Input id="ticket-operation-reason" required value={reason} onChange={event => setReason(event.target.value)} /></Field>
        {error && <p role="alert" className="text-error">{error}</p>}
      </div>
    </FormDialog>
    <ConfirmDialog
      open={confirming}
      title="确认两磅依据有效"
      description={<><p>已核验本次采用的两磅依据仍有效（包含单磅重称时沿用的另一磅）。</p><p className="mt-2">确认后将按当前资料生成关联更正的新单据；原单及原始称重依据保留。</p></>}
      confirmText="确认保存"
      cancelText="返回修改"
      loading={submitting}
      onConfirm={() => void submitConfirmed()}
      onCancel={() => { if (!submitting) setConfirming(false); }}
    />
  </>;
}
