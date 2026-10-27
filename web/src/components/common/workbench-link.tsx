import { useState, type AnchorHTMLAttributes } from 'react';
import { openWorkbench, workbenchWindow, MANAGEMENT_PATH } from '@/lib/workbench-window';
import { localSiteCode } from '@/lib/local-site';
import { useWeighingStore } from '@/store/weighing-store';

export function WorkbenchLink({to,children,...props}:Omit<AnchorHTMLAttributes<HTMLAnchorElement>,'href'>&{to:string}){
 const [blocked,setBlocked]=useState(false);
 const url=new URL(to,window.location.origin);
 const taskId=url.searchParams.get('task');
 const task=useWeighingStore(s=>s.data.tasks.find(t=>t.id===taskId));
 const capabilities=useWeighingStore(s=>s.capabilities);
 const siteCode=capabilities?localSiteCode(capabilities):undefined;
 const scales=capabilities?.scales.filter(s=>s.siteCode===siteCode&&s.enabled===1)??[];
 if(!url.searchParams.has('scale')&&task?.scaleCode)url.searchParams.set('scale',String(task.scaleCode));
 if(!url.searchParams.has('scale')&&scales.length===1&&scales[0].canOperate)url.searchParams.set('scale',scales[0].code);
 const scaleCode=url.searchParams.get('scale');
 const href=scaleCode?url.pathname+url.search:MANAGEMENT_PATH;
 const target=scaleCode?workbenchWindow(scaleCode):undefined;
 return <><a {...props} href={href} target={target} onClick={event=>{
  if(!scaleCode||event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
  event.preventDefault();setBlocked(!openWorkbench(href));
 }}>{children}</a>{blocked&&<span role="alert" className="text-sm text-warning">浏览器阻止了新窗口。<a className="text-primary underline" href={href} target={target}>直接打开工作台</a></span>}</>;
}
