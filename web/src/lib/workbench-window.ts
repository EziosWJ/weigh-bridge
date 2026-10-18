export const WORKBENCH_PATH = '/weighing/workbench';
export const WORKBENCH_WINDOW = 'weighbridge-workbench';

/** 仅用户点击时打开；空 URL 复用窗口且不重载正在办理的车辆。 */
export function openWorkbench(to = WORKBENCH_PATH): boolean {
  const destination = new URL(to, window.location.origin);
  if (destination.origin !== window.location.origin || destination.pathname !== WORKBENCH_PATH) return false;
  const workbench = window.open('', WORKBENCH_WINDOW);
  if (!workbench) return false;
  try {
    if (workbench.location.pathname !== WORKBENCH_PATH || (destination.search && workbench.location.search !== destination.search)) workbench.location.href = destination.href;
    if (destination.searchParams.has('task')) workbench.postMessage({ type: 'weighbridge-locate-task', taskId: destination.searchParams.get('task') }, window.location.origin);
    workbench.focus();
  } catch { workbench.location.href = destination.href; }
  return true;
}
