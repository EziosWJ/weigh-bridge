import assert from 'node:assert/strict';
import { chromium } from 'playwright';

const server = process.argv[2];
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage();
  page.setDefaultTimeout(12_000);
  await page.goto(`${server}/fixture`);
  await page.waitForFunction(() => typeof window.start === 'function');
  for (const view of ['front', 'rear']) {
    await page.evaluate(view => window.start(view), view);
    await page.waitForFunction(view => window.activeView === view && document.querySelector('video').videoWidth === 320 && document.querySelector('video').getVideoPlaybackQuality().totalVideoFrames > window.startFrames + 3, view);
    const frames = await page.evaluate(() => document.querySelector('video').getVideoPlaybackQuality().totalVideoFrames);
    await page.waitForTimeout(500);
    assert.ok(await page.evaluate(() => document.querySelector('video').getVideoPlaybackQuality().totalVideoFrames) > frames);
  }
  console.log('真实 MediaMTX v1.21.2：前后直连视频解码通过。');
} finally {
  await browser.close();
}
