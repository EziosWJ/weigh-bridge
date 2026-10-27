import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Field } from '@/components/common/field';
import { FormDialog } from '@/components/common/form-dialog';
import { Input } from '@/components/ui/input';
import { useWeighingStore } from '@/store/weighing-store';
import { toast } from '@/components/common/toast-store';
import { printStatusLabels } from '@/types/weighing';

/** 所有入口共用现有打印命令；打开页面和刷新状态均不发起打印。 */
export function TicketPrintActions({ ticketId }: { ticketId: string }) {
  const { data, capabilities, busy, dispatch, loadTicket } = useWeighingStore();
  const ticket = data.tickets.find(item => item.id === ticketId);
  const canPrint = !!capabilities?.sites.some(s=>s.id===ticket?.siteId&&s.canOperate)&&capabilities.permissionCodes.includes('weighing:print');
  const canReprint = capabilities?.permissionCodes.includes('weighing:reprint') ?? false;
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
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
      if (useWeighingStore.getState().data.pending) {
        setOpen(true);
        toast.warning({ title: '打印发起结果待核实', description: '请先查询原请求结果，避免重复出纸。', duration: 6000 });
        return;
      }
      setOpen(false); setReason('');
      setNotice('打印任务已登记，请查看打印结果。');
      toast.success({ title: '打印任务已登记', description: '请查看打印结果；未知时先核实，避免重复出纸。' });
    } catch (failure) {
      const message = failure instanceof Error ? failure.message : '打印发起失败';
      setError(message);
      toast.error({ title: '打印发起失败', description: message, duration: 5000 });
    }
  }
  async function resolvePending() {
    setError('');
    try {
      await dispatch({ type: 'resolveSubmission' });
      if (useWeighingStore.getState().data.pending) {
        toast.warning({ title: '结果仍未确认', description: '请继续查询原请求，勿重复发起打印。', duration: 6000 });
        return;
      }
      setOpen(false);
      setNotice('已查询到原打印结果，请按当前状态继续办理。');
      toast.success({ title: '已查询到原打印结果', description: '请按当前打印状态继续办理。' });
    } catch (failure) {
      const message = failure instanceof Error ? failure.message : '查询打印结果失败';
      setError(message);
      toast.error({ title: '查询打印结果失败', description: message, duration: 5000 });
    }
  }
  if (!ticket || !canPrint) return null;
  const unknown = ticket.printStatus === 'unknown';
  const reprint = ticket.printStatus !== 'unprinted';
  const printPending = pending && data.pending?.operation === 'print' && data.pending.taskId === ticket.taskId;
  return <div className="space-y-2">
    <div className="flex flex-wrap items-center gap-2">
      {ticket.status === 'valid' && <Button variant="primary" disabled={busy || (pending && !printPending) || (reprint && !canReprint)} onClick={() => {
        if (printPending || reprint) { setOpen(true); return; }
        void print('initial');
      }}>{printPending ? '查询打印发起结果' : reprint ? '重新打印' : '打印'}</Button>}
      <span className="text-sm text-text-secondary">{printStatusLabels[ticket.printStatus]} · 重新打印 {ticket.reprintCount} 次</span>
    </div>
    {notice && <p role="status" className="text-sm text-success">{notice}</p>}
    {error && !open && <p role="alert" className="text-sm text-error">{error}</p>}
    <FormDialog open={open} title={printPending ? '查询打印发起结果' : '重新打印'} onCancel={() => setOpen(false)} onSubmit={() => setOpen(false)} submitText="关闭">
      <div className="space-y-4">
        {printPending ? <><p className="text-warning">打印发起结果未知，请先查询原请求，避免重复出纸。</p><Button disabled={busy} onClick={() => void resolvePending()}>查询原请求结果</Button></> : <>
          {unknown && <p className="text-warning">上次打印结果未知，重新打印可能重复出纸。</p>}
          {reprint && canReprint && <><Field label="重新打印原因" htmlFor="ticket-reprint-reason"><Input id="ticket-reprint-reason" placeholder="请填写重新打印原因" value={reason} onChange={event => setReason(event.target.value)} /></Field><Button disabled={busy || pending || !reason.trim()} onClick={() => void print('reprint')}>确认重新打印</Button></>}
        </>}
        {error && <p role="alert" className="text-error">{error}</p>}
      </div>
    </FormDialog>
  </div>;
}
