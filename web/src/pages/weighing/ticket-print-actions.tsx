import { useEffect, useId, useRef, useState } from 'react';
import { workflowApi } from '@/api/weighing-workflow';
import { Button } from '@/components/ui/button';
import { Field } from '@/components/common/field';
import { Input } from '@/components/ui/input';
import { useWeighingStore } from '@/store/weighing-store';
import { printStatusLabels } from '@/types/weighing';

type Attempt = { id: number; status: string };

async function latestAttempt(ticketId: string) {
  const attempts = await workflowApi.get<Attempt[]>(`/tickets/${ticketId}/print-attempts`);
  const current = useWeighingStore.getState().data.tickets.find(item => item.id === ticketId);
  return attempts.find(attempt => String(attempt.id) === current?.lastPrintAttemptId);
}

/** 状态查询只读，补打由操作员单独发起。 */
export function TicketPrintActions({ ticketId, autoPrint = false }: { ticketId: string; autoPrint?: boolean }) {
  return <TicketPrintControls key={ticketId} ticketId={ticketId} autoPrint={autoPrint} />;
}

function TicketPrintControls({ ticketId, autoPrint }: { ticketId: string; autoPrint: boolean }) {
  const { data, capabilities, busy, dispatch, loadPreview } = useWeighingStore();
  const ticket = data.tickets.find(item => item.id === ticketId);
  const canPrint = !!capabilities?.sites.some(site => site.canOperate) && capabilities.permissionCodes.includes('weighing:print');
  const canReprint = capabilities?.permissionCodes.includes('weighing:reprint') ?? false;
  const panelId = useId();
  const autoStarted = useRef(false);
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');
  const [printing, setPrinting] = useState(false);
  const [checking, setChecking] = useState(false);
  const [submittedAttempt, setSubmittedAttempt] = useState<string>();
  const [checkedAttempt, setCheckedAttempt] = useState<string>();
  const [verified, setVerified] = useState(false);
  const pending = !!data.pending;
  const printPending = data.pending?.operation === 'print' && data.pending.taskId === ticket?.taskId &&
    (!data.pending.ticketId || data.pending.ticketId === ticketId);
  const unknown = ticket?.printStatus === 'unknown';
  const attemptId = ticket?.lastPrintAttemptId;
  const submitted = unknown && !!attemptId && submittedAttempt === attemptId;
  const paperCheckReady = unknown && !!attemptId && checkedAttempt === attemptId;
  const canOperate = canPrint && ticket?.status === 'valid';

  useEffect(() => {
    if (!canOperate || !unknown || open || pending || printing) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        if (active) await loadPreview(ticketId);
        if (active && useWeighingStore.getState().data.tickets.find(item => item.id === ticketId)?.printStatus === 'unknown') {
          const attempt = await latestAttempt(ticketId);
          if (active) setSubmittedAttempt(attempt?.status === 'initiated' ? String(attempt.id) : undefined);
        }
      } catch { /* 状态查询按钮可显示具体错误。 */ }
      if (active) timer = setTimeout(poll, 1500);
    }
    void poll();
    return () => { active = false; clearTimeout(timer); };
  }, [canOperate, unknown, open, pending, printing, ticketId, loadPreview]);

  async function print(intent: 'initial' | 'reprint') {
    setError('');
    setPrinting(true);
    try {
      await dispatch({ type: 'startPrint', ticketId, intent, reason: intent === 'reprint' ? reason.trim() : '' });
      const current = useWeighingStore.getState().data.tickets.find(item => item.id === ticketId);
      setSubmittedAttempt(current?.lastPrintAttemptId);
      setCheckedAttempt(undefined);
      setVerified(false);
      setOpen(false);
      setReason('');
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : '打印发起失败');
    } finally { setPrinting(false); }
  }

  useEffect(() => {
    if (!autoPrint || autoStarted.current || !canOperate || ticket?.printStatus !== 'unprinted' || pending || busy) return;
    autoStarted.current = true;
    void print('initial');
    // 仅由工作台本次明确成功的回磅触发。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoPrint, ticketId, ticket?.printStatus, canOperate, pending, busy]);

  async function refresh() {
    setOpen(true);
    setError('');
    setChecking(true);
    setCheckedAttempt(undefined);
    setVerified(false);
    setReason('');
    try {
      if (printPending) await dispatch({ type: 'resolveSubmission' });
      else await loadPreview(ticketId);
      const current = useWeighingStore.getState().data.tickets.find(item => item.id === ticketId);
      if (current?.printStatus === 'unknown') {
        const attempt = await latestAttempt(ticketId);
        if (!attempt) throw new Error('未查到当前打印尝试，请重新查询。');
        setSubmittedAttempt(attempt.status === 'initiated' ? String(attempt.id) : undefined);
        setCheckedAttempt(String(attempt.id));
      }
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : '查询打印结果失败');
    } finally { setChecking(false); }
  }

  if (!ticket) return null;
  const unavailable = busy || printing || checking || (pending && !printPending);
  const unprinted = ticket.printStatus === 'unprinted';
  const label = printPending ? '查询打印发起结果'
    : unprinted ? '打印本单' : unknown && !submitted ? '核实打印结果' : '查看打印状态';
  const status = printPending ? '打印提交待核实' : submitted ? '已提交 · 待设备回执' : printStatusLabels[ticket.printStatus];
  const canOfferReprint = canOperate && !pending && !unprinted && canReprint;
  const canConfirmReprint = canOfferReprint && (!unknown || (paperCheckReady && verified));
  const reasonId = `${panelId}-reason`;

  return <div className="space-y-2">
    <div className="flex flex-wrap items-center gap-2">
      {canOperate && <Button variant={unprinted && !printPending ? 'primary' : 'secondary'} disabled={unavailable}
        aria-expanded={!unprinted || printPending ? open : undefined}
        aria-controls={open ? panelId : undefined}
        onClick={() => {
          if (unprinted && !printPending) { void print('initial'); return; }
          void refresh();
        }}>{label}</Button>}
      <span role="status" className="text-sm text-text-secondary">{printing ? '正在提交打印任务' : checking ? '正在查询打印结果' : status}{ticket.reprintCount > 0 && ` · 补打 ${ticket.reprintCount} 次`}</span>
    </div>
    {open && canOperate && <div id={panelId} className="space-y-2 rounded-control border border-border p-3">
      {!!paperCheckReady && <p className="text-sm text-warning">请核对现场纸票与单号。补打可能重复出纸。</p>}
      {paperCheckReady && canOfferReprint && <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={verified} disabled={unavailable} onChange={event => setVerified(event.target.checked)} />
        已核实现场出纸情况
      </label>}
      {canConfirmReprint && <Field label="补打原因" htmlFor={reasonId} required>
        <Input id={reasonId} disabled={unavailable} value={reason} onChange={event => setReason(event.target.value)} />
      </Field>}
      <div className="flex flex-wrap gap-2">
        {canConfirmReprint && <Button disabled={unavailable || !reason.trim()} onClick={() => void print('reprint')}>确认补打</Button>}
        <Button variant="ghost" disabled={checking || printing} onClick={() => setOpen(false)}>收起</Button>
      </div>
    </div>}
    {error && <p role="alert" className="text-sm text-error">{error}</p>}
  </div>;
}
