export const WORKBENCH_PATH = '/weighing/workbench';
export const WORKBENCH_WINDOW = 'weighbridge-workbench';
export const MANAGEMENT_PATH = '/weighing/tasks';
export function workbenchWindow(scaleId: string | number) { return `${WORKBENCH_WINDOW}-${scaleId}`; }

/** Each physical scale has an independent reusable window. */
export function openWorkbench(to = WORKBENCH_PATH): boolean {
 const destination=new URL(to,window.location.origin);
 if(destination.origin!==window.location.origin||destination.pathname!==WORKBENCH_PATH)return false;
 const scaleId=destination.searchParams.get('scaleId');
 if(!scaleId||!/^[1-9]\d*$/.test(scaleId))return false;
 const workbench=window.open('',workbenchWindow(scaleId));
 if(!workbench)return false;
 try{
  if(workbench.location.pathname!==WORKBENCH_PATH||new URLSearchParams(workbench.location.search).get('scaleId')!==scaleId)workbench.location.href=destination.href;
  else if(destination.searchParams.has('task'))workbench.postMessage({type:'weighbridge-locate-task',taskId:destination.searchParams.get('task')},window.location.origin);
  workbench.focus();
 }catch{workbench.location.href=destination.href;}
 return true;
}
