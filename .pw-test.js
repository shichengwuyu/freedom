const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errors = [];
  page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
  page.on('pageerror', e => errors.push('PAGEERROR: ' + e.message));

  try {
    await page.goto('http://127.0.0.1:3100/novel', { waitUntil: 'networkidle', timeout: 60000 });
  } catch (e) {
    console.log('goto failed:', e.message);
  }
  await page.waitForTimeout(3000);

  // 整体截图
  await page.screenshot({ path: 'F:/trae/wifi/infinite-canvas-main/.pw-novel-overview.png', fullPage: false });
  console.log('overview screenshot done');

  // 尝试找"新建项目"或项目相关 UI
  const hasNewProject = await page.getByText('新建项目', { exact: false }).count();
  console.log('新建项目 button count:', hasNewProject);

  console.log('CONSOLE ERRORS:', errors.length);
  errors.slice(0, 10).forEach(e => console.log('  -', e));

  await browser.close();
})();
