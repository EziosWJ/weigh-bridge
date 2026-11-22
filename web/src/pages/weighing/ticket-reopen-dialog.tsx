import { Link, useNavigate } from 'react-router-dom';
import { useEffect, useRef, useState } from 'react';
import { WorkbenchLink } from '@/components/common/workbench-link';
import { ConfirmDialog } from '@/components/common/confirm-dialog';
import { FormDialog } from '@/components/common/form-dialog';
import { toast } from '@/components/common/toast-store';
import { DetailItem } from '@/components/common/detail-item';
import { Field } from '@/components/common/field';
import { Input } from '@/components/ui/input';
import { plateError } from '@/lib/weighing/plate';
import { adjustedWeights, formatWeight, netAdjustmentText, originalWeights, resolveNetAdjustments, weightInMilliTonnes } from '@/lib/weighing/weights';
import { useWeighingStore } from '@/store/weighing-store';
import { activeWeighRecord, emptyTaskInput, type FinalWeightInput, type NetAdjustmentsInput, type TaskInput, type WeightValues } from '@/types/weighing';
import { BusinessFields } from './shared';
import { GrossTareCorrectionFields, NetAdjustmentFields } from './deduction-fields';

const linkClass = 'text-primary hover:underline';
const correctionItems = [
  ['business', '更正业务资料'],
  ['weights', '更正毛重或皮重'],
  ['deduction', '修改净重扣减'],
  ['retain', '沿用当前依据'],
] as const;
type CorrectionItem = typeof correctionItems[number][0];

const businessLabels: Partial<Record<keyof TaskInput, string>> = {
  plate: '车牌', materialId: '物料', partyId: '供收方', carrierId: '承运方',
  contactName: '联系人', contactPhone: '联系电话', driver: '司机', driverPhone: '司机电话', remark: '备注',
};

export function TicketReopenDialog({ ticketId, onClose }: { ticketId: string | null; onClose: () => void }) {
  const navigate = useNavigate();
  const data = useWeighingStore(state => state.data);
  const permissionCodes = useWeighingStore(state => state.capabilities?.permissionCodes);
  const dispatch = useWeighingStore(state => state.dispatch);
  const loadTask = useWeighingStore(state => state.loadTask);
  const loadTicket = useWeighingStore(state => state.loadTicket);
  const [loading, setLoading] = useState(false);
  const [ready, setReady] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [confirmClose, setConfirmClose] = useState(false);
  const [fieldError, setFieldError] = useState('');
  const [input, setInput] = useState<TaskInput>(emptyTaskInput);
  const [selected, setSelected] = useState<CorrectionItem[]>([]);
  const [reason, setReason] = useState('');
  const [finalWeights, setFinalWeights] = useState<FinalWeightInput>({ gross: 0, tare: 0 });
  const [netAdjustments, setNetAdjustments] = useState<NetAdjustmentsInput>([]);
  const [error, setError] = useState('');
  const basis = useRef(data);
  const initial = useRef({ input, finalWeights, netAdjustments });
  const [basisChanged, setBasisChanged] = useState(false);

  useEffect(() => {
    if (!ticketId) {
      setReady(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setReady(false);
    setError('');
    setConfirming(false);
    setSubmitting(false);
    setReason('');
    setFieldError('');
    setSelected([]);
    setConfirmClose(false);
    setBasisChanged(false);
    void (async () => {
      const store = useWeighingStore.getState();
      await loadTicket(ticketId);
      const ticket = useWeighingStore.getState().data.tickets.find(item => item.id === ticketId);
      if (!ticket) throw new Error('未找到该单据，请刷新后重试。');
      await store.loadTask(ticket.taskId);
      if (cancelled) return;
      const current = useWeighingStore.getState().data;
      const loadedTicket = current.tickets.find(item => item.id === ticketId) ?? ticket;
      const task = current.tasks.find(item => item.id === loadedTicket.taskId);
      const original = task ? originalWeights(task.records) : undefined;
      const changed = !!task && (['first', 'second'] as const).some(stage =>
        activeWeighRecord(task.records, stage)?.id !== activeWeighRecord(loadedTicket.records, stage)?.id);
      const initialInput: TaskInput = {
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
      };
      const initialWeights = {
        gross: changed && original ? original.gross : loadedTicket.gross,
        tare: changed && original ? original.tare : loadedTicket.tare,
        reason: '',
      };
      const loadedItems = loadedTicket.netAdjustmentItems?.length
        ? loadedTicket.netAdjustmentItems
        // Historical single-item tickets keep the fact in the legacy columns;
        // carry it over as one row so a reopen can edit it without inventing
        // additional detail.
        : (loadedTicket.netAdjustment ?? 0) !== 0
          ? [{ mode: loadedTicket.netAdjustmentMode ?? 'amount', amount: loadedTicket.netAdjustment ?? 0, rateBp: loadedTicket.netAdjustmentRateBp, category: loadedTicket.netAdjustmentCategory, categoryName: loadedTicket.netAdjustmentCategoryName, remark: loadedTicket.adjustment?.reason }]
          : [];
      const initialAdjustments = loadedItems.map(item => ({
        mode: item.mode,
        amount: item.mode === 'amount' ? (item.amount ?? 0) : 0,
        rateBp: item.mode === 'percent' ? (item.rateBp ?? 0) : 0,
        category: item.category ?? '',
        remark: item.remark ?? '',
      }));
      initial.current = { input: initialInput, finalWeights: initialWeights, netAdjustments: initialAdjustments };
      setInput(initialInput);
      setFinalWeights(initialWeights);
      setNetAdjustments(initialAdjustments);
      setBasisChanged(changed);
      basis.current = current;
      setReady(true);
    })().catch(failure => {
      if (!cancelled) {
          setError(failure instanceof Error ? failure.message : '重开资料加载失败');
      }
    }).finally(() => {
      if (!cancelled) {
        setLoading(false);
      }
    });
    return () => { cancelled = true; };
  }, [ticketId, loadTask, loadTicket]);

  const ticket = ticketId ? data.tickets.find(item => item.id === ticketId) : undefined;
  const task = ticket ? data.tasks.find(item => item.id === ticket.taskId) : undefined;
  const currentOriginal = task ? originalWeights(task.records) : undefined;
  let correctedBasis: WeightValues | undefined;
  let projectedNet: number | undefined;
  let adjustment = 0;
  let calculationError = '';
  if (currentOriginal) {
    try {
      correctedBasis = adjustedWeights(currentOriginal, finalWeights);
      adjustment = resolveNetAdjustments(correctedBasis, netAdjustments).amount;
      projectedNet = (weightInMilliTonnes(correctedBasis.net) - weightInMilliTonnes(adjustment)) / 1000;
    } catch (failure) {
      calculationError = failure instanceof Error ? failure.message : '请核对更正重量与净重扣减';
    }
  }
  const allowedItems = correctionItems.filter(([key]) => key === 'retain'
    || (key === 'business' ? permissionCodes?.includes('weighing:correct') : permissionCodes?.includes('weighing:adjust')));
  const changedBusiness = ticket ? (Object.keys(businessLabels) as (keyof TaskInput)[])
    .filter(key => (input[key] ?? '') !== (initial.current.input[key] ?? '')) : [];

  function businessValue(key: keyof TaskInput, after: boolean) {
    if (!ticket) return '—';
    const value = (after ? input : initial.current.input)[key];
    if (!value) return '—';
    if (key === 'materialId') {
      const material = data.materials.find(item => item.id === value);
      if (after) return material ? [material.name, material.spec].filter(Boolean).join(' / ') : value;
      return [ticket.materialName, ticket.materialSpec].filter(Boolean).join(' / ') || value;
    }
    if (key === 'partyId' || key === 'carrierId') {
      const party = data.parties.find(item => item.id === value);
      return after ? party?.name || value : (key === 'partyId' ? ticket.partyName : ticket.carrierName) || value;
    }
    return value;
  }

  function toggleItem(item: CorrectionItem) {
    if (item === 'retain' || (item === 'business' && selected.includes(item))) setInput(initial.current.input);
    if (item === 'retain' || (item === 'weights' && selected.includes(item))) setFinalWeights(initial.current.finalWeights);
    if (item === 'retain' || (item === 'deduction' && selected.includes(item))) {
      setNetAdjustments(initial.current.netAdjustments);
      setFieldError('');
    }
    setSelected(current => current.includes(item) ? current.filter(key => key !== item)
      : item === 'retain' ? [item] : [...current.filter(key => key !== 'retain'), item]);
    setError('');
  }
  const successors = ticket ? data.tickets.filter(item => item.previousId === ticket.id) : [];
  const canReopen = !!ticket && ticket.status === 'void' && successors.length === 0 && !data.tickets.some(item => item.taskId === ticket.taskId && item.status === 'valid');

  function requestConfirmation() {
    if (!ready || loading || submitting || data.pending || selected.length === 0) return;
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
    if (fieldError) {
      setError(fieldError);
      return;
    }
    if (!reason.trim()) {
      setError('请填写更正原因');
      return;
    }
    if (calculationError || projectedNet === undefined) {
      setError(calculationError || '缺少有效的两磅依据');
      return;
    }
    setError('');
    setConfirming(true);
  }

  async function submitConfirmed() {
    if (submitting) return;
    if (!ticket || !task || !canReopen || data.pending || projectedNet === undefined || fieldError) {
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
        netAdjustments,
      }, basis.current);
      const latest = useWeighingStore.getState().data;
      setConfirming(false);
      onClose();
      if (latest.pending) {
        toast.warning({ title: '提交结果待核实', description: '请先在工作台查询原请求，再继续办理。', duration: 6000 });
        navigate(`/weighing/workbench?task=${encodeURIComponent(ticket.taskId)}`);
        return;
      }
      const reopened = latest.tickets.find(item => item.previousId === ticket.id);
      if (reopened) {
        toast.success('更正单据已生成');
        navigate(`/weighing/tickets/${encodeURIComponent(reopened.id)}`);
      }
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
      submitDisabled={!ready || loading || !ticket || !task || !canReopen || !!data.pending || selected.length === 0}
      submitText="核对更正"
      onCancel={() => { if (reason || changedBusiness.length || finalWeights !== initial.current.finalWeights || netAdjustments !== initial.current.netAdjustments) setConfirmClose(true); else onClose(); }}
      onSubmit={requestConfirmation}
      closeOnEscape={!confirming && !confirmClose}
      closeOnOverlayClick={!confirming && !confirmClose}
      trapFocus={!confirming && !confirmClose}
    >
      <div className="space-y-space-4">
        {(loading || (!ready && !error)) && ticketId && <p role="status" className="text-text-secondary">正在加载原单与称重依据…</p>}
        {ticket && ready && !loading && <>
          <div className="grid gap-space-3 rounded-control border border-border bg-neutral-background p-space-3 sm:grid-cols-3">
            <DetailItem label="原单号" value={ticket.ticketNo ?? ticket.id} />
            <DetailItem label="车牌" value={ticket.plate} />
            <DetailItem label="原单最终净重" value={formatWeight(ticket.net)} />
          </div>
          {!canReopen && <p role="alert" className="text-error">该单据已有更正单，或同任务已有有效单据，请查看关联记录。</p>}
          {canReopen && <fieldset className="space-y-2">
            <legend className="text-form-label font-medium">更正事项</legend>
            <div className="flex flex-wrap gap-x-4 gap-y-2">
              {allowedItems.map(([key, label]) => <label key={key} className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={selected.includes(key)} onChange={() => toggleItem(key)} />{label}
              </label>)}
              {permissionCodes?.includes('weighing:reweigh') && <Link className={linkClass} to={`/weighing/tasks/${encodeURIComponent(ticket.taskId)}`}>前往原任务单磅重称</Link>}
            </div>
          </fieldset>}
          {basisChanged && <p role="status" className="text-text-secondary">两磅依据已有替代记录，本次使用当前有效称重。</p>}
          {selected.includes('business') && <BusinessFields value={input} onChange={setInput} lockDirection />}
          {currentOriginal && selected.includes('weights') && <GrossTareCorrectionFields original={currentOriginal} value={finalWeights} onChange={setFinalWeights} />}
          {correctedBasis && selected.includes('deduction') && <NetAdjustmentFields basis={correctedBasis} value={netAdjustments} onChange={setNetAdjustments} onValidationChange={setFieldError} />}
          {!selected.includes('deduction') && projectedNet !== undefined && !fieldError && <DetailItem label="预计出单净重" value={<strong className="tabular-nums">{formatWeight(projectedNet)}</strong>} />}
          {calculationError && !selected.includes('weights') && (!selected.includes('deduction') || !correctedBasis) && <p role="alert" className="text-error">{calculationError}</p>}
        </>}
        {ready && !loading && canReopen && <Field label="更正原因" htmlFor="reopen-reason" required>
          <Input id="reopen-reason" required value={reason} onChange={event => setReason(event.target.value)} />
        </Field>}
        {data.pending && ticket && data.pending.taskId === ticket.taskId
          ? <p role="alert" className="text-warning">提交结果待核实。<WorkbenchLink className={linkClass} to={`/weighing/workbench?task=${encodeURIComponent(ticket.taskId)}`}>查询原请求</WorkbenchLink></p>
          : error && <p role="alert" className="text-error">{error}</p>}
      </div>
    </FormDialog>
    <ConfirmDialog open={confirmClose} title="放弃本次更正？" confirmText="放弃并关闭" cancelText="返回修改" onConfirm={() => { setConfirmClose(false); onClose(); }} onCancel={() => setConfirmClose(false)} />
    <FormDialog
      open={confirming}
      title="核对并确认更正重开"
      submitText="确认两磅有效并出单"
      cancelText="返回修改"
      loading={submitting}
      onSubmit={submitConfirmed}
      onCancel={() => { if (!submitting) setConfirming(false); }}
    >{ticket && correctedBasis && projectedNet !== undefined && <div className="mt-2 space-y-3">
        <p className="break-words">{ticket.ticketNo ?? ticket.id} · {ticket.plate}</p>
        <table className="w-full table-fixed text-left text-sm">
          <thead><tr><th scope="col">核对项</th><th scope="col">旧值</th><th scope="col">新值</th></tr></thead>
          <tbody className="break-words tabular-nums">
            {changedBusiness.map(key => <tr key={key}><th scope="row">{businessLabels[key]}</th><td>{businessValue(key, false)}</td><td>{businessValue(key, true)}</td></tr>)}
            <tr><th scope="row">最终毛重</th><td>{formatWeight(ticket.gross)}</td><td>{formatWeight(correctedBasis.gross)}</td></tr>
            <tr><th scope="row">最终皮重</th><td>{formatWeight(ticket.tare)}</td><td>{formatWeight(correctedBasis.tare)}</td></tr>
            <tr><th scope="row">净重扣减</th><td>{netAdjustmentText(ticket.netAdjustment ?? 0)}</td><td>{netAdjustmentText(adjustment)}</td></tr>
            <tr><th scope="row">最终净重</th><td>{formatWeight(ticket.net)}</td><td><strong>{formatWeight(projectedNet)}</strong></td></tr>
          </tbody>
        </table>
        <p>确认两磅依据有效后，将生成关联原单的新单据。</p>
      </div>}
    </FormDialog>
  </>;
}
