import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Field } from '@/components/common/field';
import { FormDialog } from '@/components/common/form-dialog';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { useWeighingStore } from '@/store/weighing-store';
import { printStatusLabels } from '@/types/weighing';

/** 所有入口共用现有打印命令；打开页面和刷新状态均不发起打印。 */
export function TicketPrintActions({ ticketId }: { ticketId: string }) {
  const { data, capabilities, busy, dispatch, loadTicket } = useWeighingStore();
  const ticket = data.tickets.find(item => item.id === ticketId);
  const canPrint = capabilities?.permissionCodes.includes('weighing:print') ?? false;
  const canReprint = capabilities?.permissionCodes.includes('weighing:reprint') ?? false;
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
  const [feedbackReason, setFeedbackReason] = useState('');
  const [resolution, setResolution] = useState<'unknown' | 'printed' | 'failed'>('unknown');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const pending = !!data.pending;

  useEffect(() => {
    if (!canPrint || ticket?.status !== 'valid' || ticket.printStatus !== 'unknown') return;
    const timer = setInterval(() => { void loadTicket(ticketId).catch(() => {}); }, 1500);
    return () => clearInterval(timer);
  }, [canPrint, ticket?.status, ticket?.printStatus, ticketId, loadTicket]);

  async function print(intent: 'initial' | 'retry' | 'reprint') {
    setError(''); setNotice('');
    try {
      await dispatch({ type: 'startPrint', ticketId, intent, reason });
      if (useWeighingStore.getState().data.pending) { setOpen(true); return; }
      setOpen(false); setReason('');
      setNotice('打印任务已登记，请查看打印结果。');
    } catch (failure) { setError(failure instanceof Error ? failure.message : '打印发起失败'); }
  }
  async function resolve() {
    setError('');
    try {
      await dispatch({ type: 'resolvePrint', ticketId, status: resolution, printed: resolution === 'printed', reason: feedbackReason });
      if (useWeighingStore.getState().data.pending) return;
      setNotice('已保存操作员反馈。'); setFeedbackReason(''); setOpen(false);
    } catch (failure) { setError(failure instanceof Error ? failure.message : '结果核实失败'); }
  }
  if (!ticket || !canPrint) return null;
  const unknown = ticket.printStatus === 'unknown';
  const reprint = ticket.printStatus === 'printed' || unknown;
  const printPending = pending && data.pending?.operation === 'print' && data.pending.taskId === ticket.taskId;
  return <div className="space-y-2">
    <div className="flex flex-wrap items-center gap-2">
      {ticket.status === 'valid' && <Button variant="primary" disabled={busy || (pending && !printPending) || (reprint && !unknown && !canReprint)} onClick={() => {
        if (printPending || reprint) { setOpen(true); return; }
        void print(ticket.printStatus === 'failed' ? 'retry' : 'initial');
      }}>{printPending ? '查询打印发起结果' : unknown ? '核实打印 / 补打' : ticket.printStatus === 'printed' ? '补打' : ticket.printStatus === 'failed' ? '重试打印' : '打印'}</Button>}
      <span className="text-sm text-text-secondary">{printStatusLabels[ticket.printStatus]} · 补打 {ticket.reprintCount} 次</span>
    </div>
    {notice && <p role="status" className="text-sm text-success">{notice}</p>}
    {error && !open && <p role="alert" className="text-sm text-error">{error}</p>}
    <FormDialog open={open} title={printPending ? '查询打印发起结果' : unknown ? '核实打印 / 补打' : '补打单据'} onCancel={() => setOpen(false)} onSubmit={() => setOpen(false)} submitText="关闭">
      <div className="space-y-4">
        {printPending ? <><p className="text-warning">打印发起结果未知，请先查询原请求，避免重复出纸。</p><Button disabled={busy} onClick={() => void dispatch({ type: 'resolveSubmission' }).catch(failure => setError(failure.message))}>查询原请求结果</Button></> : <>
          {unknown && <><p className="text-warning">请先检查实际出纸情况。无法排除出纸时，填写原因办理补打。</p><Field label="人工核实结果" htmlFor="ticket-print-result"><Select id="ticket-print-result" value={resolution} onChange={event => setResolution(event.target.value as typeof resolution)}><option value="unknown">仍无法判断</option><option value="printed">确认已出纸</option><option value="failed">明确失败且确认未出纸</option></Select></Field><Field label="打印核实说明" htmlFor="ticket-result-reason"><Input id="ticket-result-reason" value={feedbackReason} onChange={event => setFeedbackReason(event.target.value)} /></Field><Button disabled={busy || !feedbackReason.trim()} onClick={() => void resolve()}>保存人工核实结果</Button></>}
          {reprint && canReprint && <><Field label="补打原因" htmlFor="ticket-reprint-reason"><Input id="ticket-reprint-reason" value={reason} onChange={event => setReason(event.target.value)} /></Field><Button disabled={busy || pending || !reason.trim()} onClick={() => void print('reprint')}>确认补打</Button></>}
        </>}
        {error && <p role="alert" className="text-error">{error}</p>}
      </div>
    </FormDialog>
  </div>;
}
