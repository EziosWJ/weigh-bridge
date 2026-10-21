import { useState, type AnchorHTMLAttributes } from 'react';
import { openWorkbench, workbenchWindow, SITES_PATH } from '@/lib/workbench-window';
import { useWeighingStore } from '@/store/weighing-store';

export function WorkbenchLink({to,children,...props}:Omit<AnchorHTMLAttributes<HTMLAnchorElement>,'href'>&{to:string}){
 const [blocked,setBlocked]=useState(false);
 const url=new URL(to,window.location.origin);
 const taskId=url.searchParams.get('task');
 const task=useWeighingStore(s=>s.data.tasks.find(t=>t.id===taskId));
 if(!url.searchParams.has('scaleId')&&task?.scaleId)url.searchParams.set('scaleId',String(task.scaleId));
 const scaleId=url.searchParams.get('scaleId');
 const href=scaleId?url.pathname+url.search:SITES_PATH;
 const target=scaleId?workbenchWindow(scaleId):undefined;
 return <><a {...props} href={href} target={target} onClick={event=>{
  if(!scaleId||event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
  event.preventDefault();setBlocked(!openWorkbench(href));
 }}>{children}</a>{blocked&&<span role="alert" className="text-sm text-warning">浏览器阻止了新窗口。<a className="text-primary underline" href={href} target={target}>直接打开工作台</a></span>}</>;
}
