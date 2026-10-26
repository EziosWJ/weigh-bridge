import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdir, readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';

const webDir = fileURLToPath(new URL('..', import.meta.url));
const outputDir = fileURLToPath(new URL('../../.task/ticket-sheet/', import.meta.url));
const origin = 'http://127.0.0.1:4197';
const sheet = {
  template: 'weighing-compact-v2', widthMm: 241, heightMm: 93, layers: 5,
  title: '新厂过磅单', marks: [], rows: [
    [{ label: '单号', value: 'GB-NEW-9a6735f61d3b803e078773473802c348' }],
    [{ label: '车牌', value: '晋A9T026' }, { label: '业务', value: '入厂卸货' }],
    [{ label: '供货方', value: '联调供货方' }, { label: '物料', value: '联调煤炭 / 一级' }],
    [{ label: '毛重（吨）', value: '30.125' }, { label: '皮重（吨）', value: '10' }, { label: '净重（吨）', value: '20.125' }],
    [{ label: '首次称重', value: '2026-10-26 10:45:14' }, { label: '二次称重', value: '2026-10-27 10:45:19' }],
    [{ label: '操作员', value: '管理员' }, { label: '司机签字', value: '________________' }],
  ],
};
let browser;
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4197', '--strictPort'], { cwd: webDir, stdio: ['ignore', 'pipe', 'pipe'] });
let logs = '';
vite.stdout.on('data', chunk => { logs += chunk; });
vite.stderr.on('data', chunk => { logs += chunk; });
try {
  for (let i = 0; ; i++) {
    if (vite.exitCode !== null || i === 80) throw new Error(`Vite failed: ${logs}`);
    try { if ((await fetch(origin)).ok) break; } catch { /* Starting. */ }
    await delay(125);
  }
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) });
  const page = await browser.newPage({ viewport: { width: 1200, height: 650 } });
  page.setDefaultTimeout(10000);
  page.on('pageerror', error => console.error('Browser error:', error.message));
  page.on('console', message => { if (message.type() === 'error') console.error('Browser console:', message.text()); });
  await page.route('**/src/main.tsx', route => route.fulfill({ contentType: 'text/javascript', body: `
    import React from '/node_modules/.vite/deps/react.js';
    import ReactDOM from '/node_modules/.vite/deps/react-dom_client.js';
    import { TicketSheet } from '/src/pages/weighing/ticket-sheet.tsx';
    document.body.style.margin = '0';
    ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(TicketSheet, {sheet: ${JSON.stringify(sheet)}}));
  ` }));
  await page.goto(origin);
  const paper = page.getByRole('article', { name: '精简过磅单' });
  await paper.waitFor();
  await page.evaluate(() => document.fonts.ready);
  const dimensions = await paper.evaluate(el => {
    const rect = el.getBoundingClientRect();
    const content = el.querySelector('.ticket-sheet-content').getBoundingClientRect();
    const first = el.querySelector('.ticket-sheet-row');
    const net = el.querySelector('.ticket-sheet-weights .ticket-sheet-field:last-child strong');
    return { width: rect.width, height: rect.height, centerX: content.x + content.width / 2 - rect.x - rect.width / 2,
      centerY: content.y + content.height / 2 - rect.y - rect.height / 2,
      contentHeight: content.height, numberHeight: first.getBoundingClientRect().height,
      numberLineHeight: parseFloat(getComputedStyle(first).lineHeight), netSize: parseFloat(getComputedStyle(net).fontSize),
      signatureHeight: el.querySelector('.ticket-sheet-signature').getBoundingClientRect().height,
      fieldsOverflow: [...el.querySelectorAll('.ticket-sheet-field')].some(field => field.scrollWidth > field.clientWidth),
    };
  });
  assert.ok(Math.abs(dimensions.width - 241 * 96 / 25.4) < 1);
  assert.ok(Math.abs(dimensions.height - 93 * 96 / 25.4) < 1);
  assert.ok(Math.abs(dimensions.centerX) < 1 && Math.abs(dimensions.centerY) < 1, JSON.stringify(dimensions));
  assert.ok(dimensions.contentHeight < dimensions.height - 8 * 96 / 25.4);
  assert.ok(dimensions.numberHeight < dimensions.numberLineHeight * 2, 'Full ticket number wrapped');
  assert.ok(dimensions.netSize >= 26 && dimensions.signatureHeight >= 13 * 96 / 25.4 - 1);
  assert.equal(dimensions.fieldsOverflow, false);
  assert.equal(await page.locator('.ticket-sheet-row').count(), 6);
  await mkdir(outputDir, { recursive: true });
  await paper.screenshot({ path: `${outputDir}preview.png` });
  await page.pdf({ path: `${outputDir}preview.pdf`, preferCSSPageSize: true });
  const pdf = (await readFile(`${outputDir}preview.pdf`)).toString('latin1');
  const media = pdf.match(/\/MediaBox\s*\[\s*0\s+0\s+([\d.]+)\s+([\d.]+)\s*\]/);
  assert.ok(media, 'PDF has no page dimensions');
  assert.ok(Math.abs(Number(media[1]) - 241 * 72 / 25.4) < 1);
  assert.ok(Math.abs(Number(media[2]) - 93 * 72 / 25.4) < 1);
  assert.equal([...pdf.matchAll(/\/Type\s*\/Page\b/g)].length, 1, 'Ticket spans multiple sections');
  await page.setViewportSize({ width: 390, height: 650 });
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
  assert.ok(await page.locator('.ticket-sheet-scroll').evaluate(el => el.scrollWidth > el.clientWidth));
  console.log(`Ticket preview, centering, full number, one-section PDF and narrow viewport passed. Artifacts: ${outputDir}`);
} finally {
  await browser?.close();
  vite.kill('SIGTERM');
  await Promise.race([new Promise(resolve => vite.once('exit', resolve)), delay(3000)]);
  if (vite.exitCode === null) vite.kill('SIGKILL');
}
