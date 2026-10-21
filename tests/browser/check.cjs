const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const base = process.env.CORE5G_URL || 'http://127.0.0.1:8085';
async function main() {
 const out = path.resolve('test-results/browser'); await fs.mkdir(out, { recursive: true });
 const browser = await chromium.launch({ headless: true }); const errors = [], results = [];
 try {
  for (const [name, width, height] of [['desktop', 1440, 1080], ['tablet', 768, 1024], ['mobile', 390, 844], ['small-mobile', 320, 740]]) {
   const page = await browser.newPage({ viewport: { width, height }, isMobile: width < 700, hasTouch: width < 700 });
   page.on('pageerror', error => errors.push(`${name}: ${error.message}`));
   await page.goto(base, { waitUntil: 'networkidle' });
   assert.equal(await page.locator('html').getAttribute('lang'), 'en');
   await page.getByText(/services available/, { exact: false }).first().waitFor();
   const titles = { overview: 'Overview', equipment: 'UE devices', sessions: 'PDU sessions', traffic: 'Virtual traffic', observability: 'Observability' };
   for (const view of ['overview', 'equipment', 'sessions', 'traffic', 'observability']) {
    await page.evaluate(view => { location.hash = view; }, view);
    await page.waitForTimeout(120);
    assert.equal(await page.locator('h1').textContent(), titles[view]);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
    assert.ok(overflow < 2, `${name}/${view} root overflow ${overflow}`);
    assert.equal(await page.locator('.view:visible').count(), 1);
    await page.screenshot({ path: path.join(out, `${name}-${view}.png`), fullPage: true });
   }
   await page.getByRole('button', { name: /Register a UE/ }).click();
   await page.getByRole('dialog').waitFor();
   await page.getByLabel('Synthetic SUPI', { exact: true }).fill('imsi-001010000000002');
   await page.getByRole('button', { name: /Register device/ }).click();
   await page.getByRole('dialog').waitFor({ state: 'hidden' });
   await page.locator('#toast').filter({ hasText: 'Device registered.' }).waitFor();
   if (width < 761) {
    await page.getByRole('button', { name: 'Show navigation' }).click();
    await page.locator('nav').getByRole('link', { name: /UE devices/ }).click();
    await page.waitForFunction(() => !document.getElementById('sidebar').classList.contains('open'));
    assert.equal(await page.locator('#sidebar.open').count(), 0);
   }
   await page.evaluate(() => { location.hash = 'sessions'; });
   await page.locator('.skip').focus(); await page.keyboard.press('Enter');
   assert.ok(page.url().endsWith('#sessions'), 'Skip link changed the current route');
   results.push(`${name}: five views, no root overflow, registration dialog, skip link and responsive navigation passed`);
   await page.close();
  }
  assert.deepEqual(errors, []);
  await fs.writeFile(path.join(out, 'results.json'), JSON.stringify({ results, errors }, null, 2));
  console.log(JSON.stringify({ results, errors, screenshots: out }, null, 2));
 } finally { await browser.close(); }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
