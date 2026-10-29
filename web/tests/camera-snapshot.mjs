import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
const base = 'http://127.0.0.1:4197';
const vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4197', '--strictPort'], { cwd: new URL('..', import.meta.url), stdio: 'ignore' });
let browser;
const deadline = setTimeout(() => { vite.kill('SIGTERM'); void browser?.close(); }, 60_000);
try {
  for (let i=0;i<80;i++) { try { if ((await fetch(`${base}/tests/camera-snapshot.html`)).ok) break; } catch {} await delay(100); }
  browser = await chromium.launch({ headless: true }); const page = await browser.newPage(); const seen = [];
  await page.route('**/api/weighing/**', route => {
    const path = new URL(route.request().url()).pathname; seen.push([path,route.request().headers().authorization]);
    if (path.endsWith('/events')) return route.fulfill({ json: { code: 200, data: { binding: { serialNumber:'test-camera', enabled:true, timeoutSeconds:180, deviceTime:'', deviceIp:'' }, state:'unknown', events:[], snapshots:[{ id:7, plate:'晋A12345', serialNumber:'test-camera', eventId:'42', deviceTime:'2026-10-29T12:00:00+08:00', receivedAt:'2026-10-29T04:00:00Z' }] } } });
    if (path.endsWith('/capture') || path.endsWith('/background')) return route.fulfill({ contentType:'image/jpeg', body:Buffer.from('/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wAALCAABAAEBAREA/8QAFAABAAAAAAAAAAAAAAAAAAAACf/EABQQAQAAAAAAAAAAAAAAAAAAAAD/2gAIAQEAAD8AVN//2Q==','base64') });
    return route.abort();
  });
  await page.goto(`${base}/tests/camera-snapshot.html`);
  await page.getByText(/晋A12345/).waitFor();
  assert.equal(seen.filter(([p])=>p.includes('/snapshots/')).length,0); assert.ok(seen.every(([,token])=>token==='test-token'));
  console.log('camera plate records passed');
} finally { clearTimeout(deadline); await browser?.close(); vite.kill('SIGTERM'); }
