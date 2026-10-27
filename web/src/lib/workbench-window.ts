export const WORKBENCH_PATH = '/weighing/workbench';
export const WORKBENCH_WINDOW = 'weighbridge-workbench';
export const MANAGEMENT_PATH = '/weighing/tasks';
export function workbenchWindow(scaleCode: string) { return `${WORKBENCH_WINDOW}-${scaleCode}`; }

/** Each physical scale has an independent reusable window. */
export function openWorkbench(to = WORKBENCH_PATH): boolean {
 const destination=new URL(to,window.location.origin);
 if(destination.origin!==window.location.origin||destination.pathname!==WORKBENCH_PATH)return false;
 const scaleCode=destination.searchParams.get('scale');
 if(!scaleCode||!/^[A-Za-z0-9_.-]{1,40}$/.test(scaleCode))return false;
 const workbench=window.open('',workbenchWindow(scaleCode));
 if(!workbench)return false;
 try{
  if(workbench.location.pathname!==WORKBENCH_PATH||new URLSearchParams(workbench.location.search).get('scale')!==scaleCode)workbench.location.href=destination.href;
  else if(destination.searchParams.has('task'))workbench.postMessage({type:'weighbridge-locate-task',taskId:destination.searchParams.get('task')},window.location.origin);
  workbench.focus();
 }catch{workbench.location.href=destination.href;}
 return true;
}
