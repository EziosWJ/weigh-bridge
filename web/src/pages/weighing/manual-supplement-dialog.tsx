import { useState } from 'react';
import { Link } from 'react-router-dom';
import { FormDialog } from '@/components/common/form-dialog';
import { Field } from '@/components/common/field';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { workflowApi, type CommandResult } from '@/api/weighing-workflow';
import { createId } from '@/lib/id';
import { isApiError } from '@/lib/api-error';
import { weightInMilliTonnes } from '@/lib/weighing/weights';
import { emptyTaskInput, supplementReasonLabels, type NetAdjustmentsInput, type TaskInput } from '@/types/weighing';
import { BusinessDirectionControl, BusinessRequiredFields } from './shared';
import { NetAdjustmentFields } from './deduction-fields';

type Pending = { requestId: string; body: Record<string, unknown> };
const storageKey = (userId: number) => `weighbridge-manual-supplement-${userId}`;
function readPending(userId: number): Pending | null {
  try { return JSON.parse(sessionStorage.getItem(storageKey(userId)) || 'null') as Pending | null; } catch { return null; }
}
function beijingValue() { return new Date(Date.now() + 8 * 3600000).toISOString().slice(0, 16); }
function toISO(value: string) { return new Date(`${value}${value.length === 16 ? ':00' : ''}+08:00`).toISOString(); }

export function ManualSupplementDialog({ open, onClose, userId, allowAdjust }: { open: boolean; onClose: () => void; userId: number; allowAdjust: boolean }) {
  const [business, setBusiness] = useState<TaskInput>(emptyTaskInput);
  const [first, setFirst] = useState(''); const [second, setSecond] = useState('');
  const [firstAt, setFirstAt] = useState(beijingValue); const [secondAt, setSecondAt] = useState(beijingValue);
  const [reason, setReason] = useState(''); const [adjustments, setAdjustments] = useState<NetAdjustmentsInput>([]);
  const [adjustmentError, setAdjustmentError] = useState(''); const [error, setError] = useState('');
  const [pending, setPending] = useState<Pending | null>(() => readPending(userId));
  const [checked, setChecked] = useState(false); const [busy, setBusy] = useState(false); const [result, setResult] = useState<CommandResult | null>(null);
  const update = (patch: Partial<TaskInput>) => setBusiness(current => ({ ...current, ...patch }));
  let basis: { gross: number; tare: number; net: number } | undefined;
  try {
    const f = weightInMilliTonnes(Number(first)) / 1000; const s = weightInMilliTonnes(Number(second)) / 1000;
    const gross = business.direction === 'inbound' ? f : s; const tare = business.direction === 'inbound' ? s : f;
    if (f > 0 && s > 0 && gross > tare) basis = { gross, tare, net: (weightInMilliTonnes(gross) - weightInMilliTonnes(tare)) / 1000 };
  } catch { /* Validation is shown on submission. */ }
  function clearPending() { sessionStorage.removeItem(storageKey(userId)); setPending(null); }
  async function send(request: Pending) {
    setBusy(true); setError('');
    try {
      const response = await workflowApi.post<CommandResult>('/manual-supplements', request.body);
      clearPending(); setResult(response);
    } catch (failure) {
      if (isApiError(failure) && ['business', 'forbidden', 'unauthorized'].includes(failure.type)) clearPending();
      setError(failure instanceof Error ? failure.message : '提交结果未知，请先查询原请求');
    } finally { setBusy(false); }
  }
  async function submit() {
    if (pending) return;
    if (!basis || !business.plate || !business.materialId || !business.partyId || !reason || adjustmentError) { setError('请核对必填资料、重量及扣减。毛重须大于皮重。'); return; }
    let firstISO: string; let secondISO: string;
    try { firstISO = toISO(firstAt); secondISO = toISO(secondAt); } catch { setError('请填写有效的实际称重时间'); return; }
    if (firstISO > secondISO || new Date(secondISO).getTime() > Date.now()) { setError('首磅不能晚于回磅，回磅不能晚于当前时间'); return; }
    const requestId = createId();
    const netAdjustments = adjustments.filter(item => item.mode === 'percent' ? !!item.rateBp : !!item.amount).map(item => ({ mode: item.mode, ...(item.mode === 'percent' ? { rateBp: item.rateBp } : { amount: String(item.amount) }), category: item.category, remark: item.remark ?? '' }));
    const body = { requestId, business: { ...business, materialId: Number(business.materialId), partyId: Number(business.partyId), carrierId: business.carrierId ? Number(business.carrierId) : null }, firstWeight: first, secondWeight: second, firstWeighedAt: firstISO, secondWeighedAt: secondISO, reason, netAdjustments };
    const request = { requestId, body };
    try { sessionStorage.setItem(storageKey(userId), JSON.stringify(request)); } catch { setError('无法保存待核实请求，不能提交'); return; }
    setPending(request); await send(request);
  }
  async function check() {
    if (!pending) return;
    setBusy(true); setError('');
    try {
      const op = await workflowApi.operation(pending.requestId);
      if (op.status === 'committed' && op.result) { clearPending(); setResult(op.result); }
      else if (op.status === 'rejected') { clearPending(); setError(op.error || '原请求已拒绝，请修改后重新提交'); }
      else { setChecked(true); setError('尚未查到原请求。可再次查询，或使用原请求重试；不要新建补单。'); }
    } catch (failure) { setError(failure instanceof Error ? failure.message : '查询失败'); }
    finally { setBusy(false); }
  }
  return <FormDialog open={open} title="人工补单" description="两次称重均已完成且系统没有称重记录时，核对现场记录后补单。" onCancel={onClose} onSubmit={submit} submitText="确认出单" submitDisabled={busy || !!pending || !!result} loading={busy}>
    {result ? <div className="space-y-3"><p>人工补单已出单。</p><Link className="text-primary underline" to={`/weighing/tickets/${result.ticketId}`}>查看单据</Link></div> : <div className="space-y-4">
      {pending && <div className="space-y-2 rounded-control border border-warning p-3"><p>上次提交结果待核实，请先查询原请求。</p><Button type="button" onClick={() => void check()} disabled={busy}>查询原请求</Button><Button type="button" onClick={() => void send(pending)} disabled={busy || !checked}>重试原请求</Button></div>}
      <BusinessDirectionControl value={business} update={update} />
      <BusinessRequiredFields value={business} update={update} hidePlate={false} />
      <Field label={`首磅重量（吨，${business.direction === 'inbound' ? '毛重' : '皮重'}）`} required><Input type="number" min="0.001" step="0.001" value={first} onChange={e => setFirst(e.target.value)} /></Field>
      <Field label="首磅实际时间（北京时间）" required><Input type="datetime-local" value={firstAt} onChange={e => setFirstAt(e.target.value)} /></Field>
      <Field label={`回磅重量（吨，${business.direction === 'inbound' ? '皮重' : '毛重'}）`} required><Input type="number" min="0.001" step="0.001" value={second} onChange={e => setSecond(e.target.value)} /></Field>
      <Field label="回磅实际时间（北京时间）" required><Input type="datetime-local" value={secondAt} onChange={e => setSecondAt(e.target.value)} /></Field>
      {basis && <p>调整前净重：{basis.net.toFixed(3)} 吨</p>}
      {basis && allowAdjust && <details><summary>有扣减时填写</summary><NetAdjustmentFields basis={basis} value={adjustments} onChange={setAdjustments} onValidationChange={setAdjustmentError} /></details>}
      <Field label="补单原因" required><Select value={reason} onChange={e => setReason(e.target.value)}><option value="">请选择</option>{Object.entries(supplementReasonLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</Select></Field>
      {error && <p role="alert" className="text-error">{error}</p>}
    </div>}
  </FormDialog>;
}
