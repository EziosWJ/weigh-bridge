import { useState, type AnchorHTMLAttributes } from 'react';
import { openWorkbench, WORKBENCH_WINDOW } from '@/lib/workbench-window';

export function WorkbenchLink({ to, children, ...props }: Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href'> & { to: string }) {
  const [blocked, setBlocked] = useState(false);
  return <><a {...props} href={to} target={WORKBENCH_WINDOW} onClick={event => {
    if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    event.preventDefault(); setBlocked(!openWorkbench(to));
  }}>{children}</a>{blocked && <span role="alert" className="text-sm text-warning">浏览器阻止了新窗口。<a className="text-primary underline" href={to} target={WORKBENCH_WINDOW}>直接打开工作台</a></span>}</>;
}
