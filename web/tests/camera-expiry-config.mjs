import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const base = 'http://127.0.0.1:4198';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4198', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 60000);
try {
 for (let i=0;i<80;i++) { try { if ((await fetch(`${base}/tests/camera-expiry-config.html`)).ok) break; } catch {} await delay(100); }
 browser = await chromium.launch({headless:true}); const page = await browser.newPage(); page.on("pageerror", e => console.log("page error", e.message)); let value='5'; let writes=0;
 const row=()=>({id:23, configName:'车牌事件有效期（分钟）', configKey:'weighing.camera-event-max-age-minutes', configValue:value, configType:'SYSTEM', valueType:'NUMBER', status:1, isBuiltin:1, remark:'1 至 10080 分钟', createTime:new Date().toISOString(), updateTime:new Date().toISOString()});
 await page.route('**/api/**', route => { const path=new URL(route.request().url()).pathname; const method=route.request().method(); if (!path.startsWith('/api/')) return route.continue(); const data=x=>route.fulfill({json:{code:200,data:x}});
  if(path==='/api/weighing/capabilities') return data({permissionCodes:['system:config']});
  if(path==='/api/system/config/page') return data({records:[row()],total:1,page:1,pageSize:10});
  if(path==='/api/system/config/23' && method==='GET') return data(row());
  if(path==='/api/system/config/23' && method==='PUT') { value=JSON.parse(route.request().postData()).configValue; writes++; return data(null); }
  return data([]);
 });
 const response=await page.goto(`${base}/tests/camera-expiry-config.html`); console.log('page status',response.status()); page.on('console', x => console.log('console', x.text()));
 const line=page.getByRole('row').filter({hasText:'车牌事件有效期'}); await line.waitFor({timeout:8000}).catch(async e => { console.log('page text:', await page.locator('body').innerText()); throw e; });
 await line.getByRole('button',{name:'编辑',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'编辑配置'}); await dialog.waitFor();
 const input=dialog.getByLabel('配置值',{exact:false}); await input.fill('0'); await dialog.getByRole('button',{name:'保存',exact:true}).click();
 await dialog.getByText('请输入 1 至 10080 的整数分钟').waitFor(); assert.equal(writes,0);
 await input.fill('10'); await dialog.getByRole('button',{name:'保存',exact:true}).click(); await page.waitForFunction(()=>!document.querySelector('[role="dialog"]'));
 assert.equal(writes,1); assert.equal(value,'10'); console.log('camera expiry configuration browser passed');
} finally { clearTimeout(deadline); await browser?.close(); vite.kill('SIGTERM'); }
