import { useState } from 'react';
import { Link } from 'react-router-dom';
import { DetailDialog } from '@/components/common/detail-dialog';
import { FormDialog } from '@/components/common/form-dialog';
import { HelpHint } from '@/components/common/help-hint';
import { Field } from '@/components/common/field';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { workflowApi, type CommandResult } from '@/api/weighing-workflow';
import { createId } from '@/lib/id';
import { isApiError } from '@/lib/api-error';
import { plateError, provinces } from '@/lib/weighing/plate';
import { useWeighingStore } from '@/store/weighing-store';
import { formatWeight, resolveNetAdjustments, weightInMilliTonnes } from '@/lib/weighing/weights';
import { beijingTime } from '@/lib/weighing/query';
import { emptyTaskInput, supplementReasonLabels, type NetAdjustmentsInput, type TaskInput } from '@/types/weighing';
import { BusinessDirectionControl, BusinessRequiredFields } from './shared';
import { NetAdjustmentFields } from './deduction-fields';
import { TicketPrintActions } from './ticket-print-actions';

type Pending = { requestId: string; body: Record<string, unknown> };
const storageKey = (userId: number) => `weighbridge-manual-supplement-${userId}`;
function readPending(userId: number): Pending | null {
  try { return JSON.parse(sessionStorage.getItem(storageKey(userId)) || 'null') as Pending | null; } catch { return null; }
}
function toISO(value: string) { return new Date(`${value}${value.length === 16 ? ':00' : ''}+08:00`).toISOString(); }

export function ManualSupplementDialog({ open, onClose, userId, allowAdjust }: { open: boolean; onClose: () => void; userId: number; allowAdjust: boolean }) {
  const [business, setBusiness] = useState<TaskInput>(emptyTaskInput);
  const [first, setFirst] = useState(''); const [second, setSecond] = useState('');
  const [firstAt, setFirstAt] = useState(''); const [secondAt, setSecondAt] = useState('');
  const [reason, setReason] = useState(''); const [adjustments, setAdjustments] = useState<NetAdjustmentsInput>([]);
  const [adjustmentError, setAdjustmentError] = useState(''); const [error, setError] = useState('');
  const [pending, setPending] = useState<Pending | null>(() => readPending(userId));
  const [checked, setChecked] = useState(false); const [busy, setBusy] = useState(false); const [result, setResult] = useState<CommandResult | null>(null);
  const ticketId = result?.ticketId;
  const ticket = useWeighingStore(state => state.data.tickets.find(item => item.id === String(ticketId)));
  const loadTicket = useWeighingStore(state => state.loadTicket);
  const [ticketError, setTicketError] = useState('');
  const [plateValidation, setPlateValidation] = useState('');
  const [timeError, setTimeError] = useState('');
  const [weightError, setWeightError] = useState('');
  const inbound = business.direction === 'inbound';
  const update = (patch: Partial<TaskInput>) => {
    if (patch.plate !== undefined) setPlateValidation('');
    if (patch.direction) setWeightError('');
    setBusiness(current => ({ ...current, ...patch }));
  };
  let basis: { gross: number; tare: number; net: number } | undefined;
  try {
    const firstMilli = weightInMilliTonnes(Number(first));
    const secondMilli = weightInMilliTonnes(Number(second));
    const grossMilli = inbound ? firstMilli : secondMilli;
    const tareMilli = inbound ? secondMilli : firstMilli;
    if (tareMilli > 0 && grossMilli > tareMilli) basis = {
      gross: grossMilli / 1000, tare: tareMilli / 1000, net: (grossMilli - tareMilli) / 1000,
    };
  } catch { /* Validation is shown on submission. */ }
  let finalNet: number | undefined;
  if (basis && !adjustmentError) {
    try {
      const { amount } = resolveNetAdjustments(basis, adjustments);
      finalNet = (weightInMilliTonnes(basis.net) - weightInMilliTonnes(amount)) / 1000;
    } catch { /* 编辑区展示扣减校验。 */ }
  }

  async function readTicket(ticketId: number) {
    setTicketError('');
    try { await loadTicket(ticketId); }
    catch { setTicketError('单据读取失败，请重新查询。'); }
  }

  function showResult(response: CommandResult) {
    clearPending();
    setResult(response);
    if (response.ticketId) void readTicket(response.ticketId);
  }

  function clearPending() { sessionStorage.removeItem(storageKey(userId)); setPending(null); }
  async function send(request: Pending) {
    setBusy(true); setError('');
    try {
      const response = await workflowApi.post<CommandResult>('/manual-supplements', request.body);
      showResult(response);
    } catch (failure) {
      if (isApiError(failure) && ['business', 'forbidden', 'unauthorized'].includes(failure.type)) clearPending();
      setError(failure instanceof Error ? failure.message : '提交结果未知，请先查询原请求');
    } finally { setBusy(false); }
  }
  async function submit() {
    if (pending || result) return;
    setError('');
    setTimeError('');
    setWeightError('');
    const invalidPlate = plateError(business.plate);
    if (invalidPlate) {
      document.getElementById(provinces.includes(business.plate[0]) ? 'business-plate' : 'business-plate-province')?.focus();
      setPlateValidation(invalidPlate);
      return;
    }
    if (!business.materialId || !business.partyId || !reason) { setError('请选择物料、供收方和补单原因。'); return; }
    if (!basis) { setWeightError('请填写有效重量，毛重须大于皮重。'); return; }
    if (finalNet === undefined) return;
    let firstISO: string; let secondISO: string;
    try { firstISO = toISO(firstAt); secondISO = toISO(secondAt); } catch { setTimeError('请填写有效的实际称重时间。'); return; }
    if (firstISO > secondISO || new Date(secondISO).getTime() > Date.now()) { setTimeError('首磅不能晚于回磅，回磅不能晚于当前时间。'); return; }
    const requestId = createId();
    const netAdjustments = adjustments.filter(item => item.mode === 'percent' ? !!item.rateBp : !!item.amount).map(item => ({ mode: item.mode, ...(item.mode === 'percent' ? { rateBp: item.rateBp } : { amount: String(item.amount) }), category: item.category, remark: item.remark ?? '' }));
    const body = { requestId, business: { ...business, materialId: Number(business.materialId), partyId: Number(business.partyId), carrierId: business.carrierId ? Number(business.carrierId) : null }, firstWeight: first, secondWeight: second, firstWeighedAt: firstISO, secondWeighedAt: secondISO, reason, netAdjustments };
    const request = { requestId, body };
    try { sessionStorage.setItem(storageKey(userId), JSON.stringify(request)); } catch { setError('无法保存待核实请求，不能提交'); return; }
    setChecked(false); setPending(request); await send(request);
  }
  async function check() {
    if (!pending) return;
    setBusy(true); setError('');
    try {
      const op = await workflowApi.operation(pending.requestId);
      if (op.status === 'committed' && op.result) showResult(op.result);
      else if (op.status === 'rejected') { clearPending(); setError(op.error || '原请求已拒绝，请修改后重新提交'); }
      else { setChecked(true); setError('尚未查到原请求。可再次查询，或使用原请求重试；不要新建补单。'); }
    } catch (failure) { setError(failure instanceof Error ? failure.message : '查询失败'); }
    finally { setBusy(false); }
  }
  if (result) return <DetailDialog open={open} title="人工补单已出单" contentClassName="workbench-modal workbench-completion-dialog max-w-[520px]" trapFocus onCancel={onClose}>
    <div className="workbench-completion-summary">
      <div><p className="review-fact-label">车牌</p><p className="workbench-completion-plate">{ticket?.plate || business.plate || '—'}</p></div>
      <div><p className="review-fact-label">最终净重</p><p className="workbench-completion-weight">{ticket ? formatWeight(ticket.net) : '—'}</p></div>
    </div>
    <dl className="workbench-completion-details">
      {ticketId && <div><dt>单据号</dt><dd><Link target="weighbridge-management" className="text-primary hover:underline" to={`/weighing/tickets/${ticketId}`}>{ticket?.ticketNo || '查看单据'}</Link></dd></div>}
      {ticket && <div><dt>出单时间</dt><dd>{beijingTime(ticket.issuedAt)}</dd></div>}
    </dl>
    {ticket && <div className="workbench-completion-print"><TicketPrintActions ticketId={ticket.id} /></div>}
    {ticketError && <div className="mt-3 space-y-2"><p role="alert" className="text-sm text-error">{ticketError}</p>{ticketId && <Button onClick={() => void readTicket(ticketId)}>重新查询单据</Button>}</div>}
  </DetailDialog>;

  return <FormDialog open={open} title="人工补单" contentClassName="workbench-modal manual-supplement-dialog workbench-form-labels max-w-[700px]" onCancel={onClose} onSubmit={submit}
    submitText="确认出单" submitDisabled={busy || !!pending} loading={busy}>
    <div className="space-y-4">
      {pending && <div className="space-y-2 rounded-control border border-warning p-3"><p>上次提交结果待核实，请先查询原请求。</p><div className="flex flex-wrap gap-2"><Button onClick={() => void check()} disabled={busy}>查询原请求</Button><Button onClick={() => void send(pending)} disabled={busy || !checked}>重试原请求</Button></div></div>}
      <fieldset disabled={busy || !!pending} className="space-y-4">
        <section className="space-y-2">
          <div className="flex items-center gap-1"><h3 className="workbench-dialog-section-title">运输资料</h3><HelpHint label="人工补单适用规则">两次称重均已完成且系统没有称重记录时，核对现场记录后补单。</HelpHint></div>
          <div className="manual-supplement-grid">
            <BusinessDirectionControl value={business} update={update} showHelp={false} />
            <BusinessRequiredFields value={business} update={update} compact hidePlate={false} plateValidation={plateValidation} />
          </div>
        </section>
        <section className="space-y-3 border-t border-border pt-3" aria-label="出单核对">
          <h3 className="workbench-dialog-section-title">重量与时间</h3>
          <div className="manual-supplement-grid">
            <Field label={`首磅${inbound ? '毛重' : '皮重'}（吨）`} htmlFor="manual-first-weight" required><Input id="manual-first-weight" type="number" min="0.001" step="0.001" value={first} onChange={e => { setFirst(e.target.value); setWeightError(''); }} /></Field>
            <Field label="首磅实际时间（北京时间）" htmlFor="manual-first-at" required><Input id="manual-first-at" type="datetime-local" value={firstAt} onChange={e => { setFirstAt(e.target.value); setTimeError(''); }} /></Field>
            <Field label={`回磅${inbound ? '皮重' : '毛重'}（吨）`} htmlFor="manual-second-weight" required><Input id="manual-second-weight" type="number" min="0.001" step="0.001" value={second} onChange={e => { setSecond(e.target.value); setWeightError(''); }} /></Field>
            <Field label="回磅实际时间（北京时间）" htmlFor="manual-second-at" required><Input id="manual-second-at" type="datetime-local" value={secondAt} onChange={e => { setSecondAt(e.target.value); setTimeError(''); }} /></Field>
          </div>
          {weightError && <p role="alert" className="text-sm text-error">{weightError}</p>}
          {timeError && <p role="alert" className="text-sm text-error">{timeError}</p>}
          <div className="space-y-3 border-t border-border pt-3">
            {basis && allowAdjust && <details><summary className="workbench-dialog-section-title cursor-pointer">有扣减时填写</summary><div className="mt-2"><NetAdjustmentFields basis={basis} value={adjustments} onChange={setAdjustments} onValidationChange={setAdjustmentError} showWeightSummary={false} /></div></details>}
            <Field label="补单原因" htmlFor="manual-supplement-reason" required><Select id="manual-supplement-reason" value={reason} onChange={e => setReason(e.target.value)}><option value="">请选择</option>{Object.entries(supplementReasonLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</Select></Field>
            <p className="manual-supplement-total"><span className="review-fact-label">出单净重</span><strong>{finalNet === undefined ? '—' : formatWeight(finalNet)}</strong></p>
          </div>
        </section>
      </fieldset>
      {error && <p role="alert" className="text-sm text-error">{error}</p>}
    </div>
  </FormDialog>;
}
