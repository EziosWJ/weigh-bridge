import { useState, type AnchorHTMLAttributes } from 'react';
import { openWorkbench, workbenchWindow, MANAGEMENT_PATH } from '@/lib/workbench-window';
import { localSiteId } from '@/lib/local-site';
import { useWeighingStore } from '@/store/weighing-store';

export function WorkbenchLink({to,children,...props}:Omit<AnchorHTMLAttributes<HTMLAnchorElement>,'href'>&{to:string}){
 const [blocked,setBlocked]=useState(false);
 const url=new URL(to,window.location.origin);
 const taskId=url.searchParams.get('task');
 const task=useWeighingStore(s=>s.data.tasks.find(t=>t.id===taskId));
 const capabilities=useWeighingStore(s=>s.capabilities);
 const siteId=capabilities?localSiteId(capabilities):undefined;
 const scales=capabilities?.scales.filter(s=>s.siteId===siteId&&s.enabled===1)??[];
 if(!url.searchParams.has('scaleId')&&task?.scaleId)url.searchParams.set('scaleId',String(task.scaleId));
 if(!url.searchParams.has('scaleId')&&scales.length===1&&scales[0].canOperate)url.searchParams.set('scaleId',String(scales[0].id));
 const scaleId=url.searchParams.get('scaleId');
 const href=scaleId?url.pathname+url.search:MANAGEMENT_PATH;
 const target=scaleId?workbenchWindow(scaleId):undefined;
 return <><a {...props} href={href} target={target} onClick={event=>{
  if(!scaleId||event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
  event.preventDefault();setBlocked(!openWorkbench(href));
 }}>{children}</a>{blocked&&<span role="alert" className="text-sm text-warning">浏览器阻止了新窗口。<a className="text-primary underline" href={href} target={target}>直接打开工作台</a></span>}</>;
}
