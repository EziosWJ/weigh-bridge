import assert from 'node:assert/strict';
import { chromium } from 'playwright';
const [site, central] = process.argv.slice(2);
assert(site && central, 'Temporary site and central URLs are required');
const browser = await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_EXECUTABLE_PATH}:{})});
try {
 for (const [url,isCentral] of [[site,false],[central,true]]) {
  const context = await browser.newContext({viewport:{width:1366,height:900}});
  const page = await context.newPage();page.setDefaultTimeout(12000);
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`${url}/login`);
  await page.getByPlaceholder('请输入用户名',{exact:true}).fill('admin');
  await page.getByPlaceholder('请输入密码',{exact:true}).fill('admin123');
  await page.getByRole('button',{name:'登录',exact:true}).click();
  await page.waitForURL(u=>u.pathname!=='/login');
  await page.goto(`${url}/weighing/temporary-parties`);
  await page.getByRole('heading',{name:'临时往来主体',exact:true}).waitFor();
  await page.getByLabel('中心同步状态').getByText(isCentral?'中心汇总':'本厂独立运行',{exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:'补录临时主体',exact:true}).count(),isCentral?0:1);
  if(!isCentral){
   // Page reload and local authentication must work with the center hop blocked.
   await page.reload();await page.getByRole('heading',{name:'临时往来主体',exact:true}).waitFor();
   await page.getByRole('button',{name:'补录临时主体',exact:true}).click();
   const dialog=page.getByRole('dialog');await dialog.getByRole('textbox').first().fill('浏览器临时承运方');
   await dialog.getByRole('button',{name:'保存',exact:true}).click();await dialog.waitFor({state:'hidden'});
   await page.getByRole('row').filter({hasText:'浏览器临时承运方'}).waitFor();
  }
  if(isCentral){await page.goto(`${url}/weighing/tasks`);await page.getByRole('heading',{name:'运输任务',level:1,exact:true}).waitFor();assert.equal(await page.getByRole('link',{name:'返回称重工作台',exact:true}).count(),0);}
  assert.deepEqual(errors,[]);
  await context.close();
 }
 console.log('PASS: 本地刷新、实名登录、同步状态、临时主体补录及中心职责入口');
}finally{await browser.close();}
