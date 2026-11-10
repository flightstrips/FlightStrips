import { expect, test, type Locator, type Page } from '@playwright/test';

test.use({ hasTouch: true });

async function openEfb(page: Page) {
  await page.route(/^https?:\/\/[^/]+\/api\//, async (route) => {
    const url = route.request().url();
    const body = url.includes('/api/efb/me')
      ? { live_mode: true, online_callsign: 'SAS123' }
      : url.includes('/api/efb/flight')
        ? {
          callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE',
          stand: 'A12', stand_state: 'ASSIGNED', runway: '22R', sid: 'NEXEN2A',
          tobt: '1200', tsat: '1205', ctot: '1210',
          pdc_state: 'CONFIRMED', pdc_clearance_text: Array.from({ length: 30 }, (_, i) => `CLEARANCE LINE ${i + 1}`).join('\n'),
          atis: { code: 'C', text: Array.from({ length: 30 }, (_, i) => `ATIS LINE ${i + 1}`), stale: false },
          capabilities: { pdc: true, tobt_update: true, stand_reassignment: false },
        }
        : {};
    await route.fulfill({ json: body });
  });
  await page.goto('/visual-tests/efb-preview.html');
  await expect(page.getByText('SAS123', { exact: true })).toBeVisible();
}

async function swipeUp(page: Page, scroller: Locator) {
  const box = (await scroller.boundingBox())!;
  const session = await page.context().newCDPSession(page);
  const x = box.x + box.width / 2;
  const y = box.y + box.height * 0.75;
  await session.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y, id: 1 }] });
  for (const offset of [20, 40, 60, 80]) {
    await session.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y: y - offset, id: 1 }] });
    await page.waitForTimeout(20);
  }
  await session.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await expect.poll(() => scroller.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await session.detach();
}

for (const viewport of [
  { width: 1024, height: 768 },
  { width: 768, height: 1024 },
  { width: 1024, height: 600 },
  { width: 390, height: 700 },
]) {
  test(`dashboard fits and responds to resizing at ${viewport.width}x${viewport.height}`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await openEfb(page);
    const dashboard = page.locator('[data-efb-dashboard]');
    for (const height of [viewport.height, viewport.height - 100]) {
      await page.setViewportSize({ width: viewport.width, height });
      await expect.poll(() => dashboard.evaluate((element) => element.getBoundingClientRect().height)).toBe(height);
      const bounds = await dashboard.evaluate((element) => ({
        width: element.getBoundingClientRect().width,
        tiles: Array.from(element.querySelectorAll('[data-efb-tile]'), (tile) => {
          const rect = tile.getBoundingClientRect();
          return { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom };
        }),
        documentWidth: document.documentElement.scrollWidth,
        documentHeight: document.documentElement.scrollHeight,
      }));
      expect(bounds.width).toBe(viewport.width);
      expect(bounds.documentWidth).toBe(viewport.width);
      expect(bounds.documentHeight).toBe(height);
      expect(bounds.tiles).toHaveLength(6);
      for (const tile of bounds.tiles) {
        expect(tile.left).toBeGreaterThanOrEqual(0);
        expect(tile.top).toBeGreaterThanOrEqual(0);
        expect(tile.right).toBeLessThanOrEqual(viewport.width);
        expect(tile.bottom).toBeLessThanOrEqual(height);
      }
      for (const name of ['TOBT', 'TSAT', 'CTOT']) {
        const fields = page.getByRole('group', { name, exact: true }).locator(':scope > div');
        for (const field of [fields.first(), fields.nth(1)]) {
          const fits = await field.evaluate((element) => element.scrollWidth <= element.clientWidth);
          expect(fits, `${name} label and value fit their tile`).toBe(true);
        }
      }
    }
  });
}

test('touch swipes scroll long ATIS text from beginning to end', async ({ page, browserName }) => {
  test.skip(browserName !== 'chromium', 'Native touch input is available through Chromium CDP');
  await page.setViewportSize({ width: 768, height: 1024 });
  await openEfb(page);
  await page.locator('[data-efb-tile="R2"] .efb-clickspot').first().click();
  const scroller = page.getByRole('dialog', { name: 'ATIS' }).locator('.efb-scroll');
  await expect(scroller).toHaveCSS('touch-action', 'manipulation');
  const initial = await scroller.evaluate((element) => ({
    top: element.scrollTop, height: element.scrollHeight, clientHeight: element.clientHeight,
    first: element.firstElementChild!.getBoundingClientRect().top - element.getBoundingClientRect().top,
  }));
  expect(initial.height).toBeGreaterThan(initial.clientHeight);
  expect(initial.top).toBe(0);
  expect(initial.first).toBeGreaterThanOrEqual(0);
  await swipeUp(page, scroller);
  await scroller.evaluate((element) => { element.scrollTop = element.scrollHeight; });
  await expect.poll(() => scroller.evaluate((element) => element.scrollTop + element.clientHeight)).toBeGreaterThanOrEqual(initial.height - 1);
});

test('touch swipes scroll clearance and tile information', async ({ page, browserName }) => {
  test.skip(browserName !== 'chromium', 'Native touch input is available through Chromium CDP');
  await page.setViewportSize({ width: 768, height: 768 });
  await openEfb(page);
  await page.getByRole('button', { name: 'PDC COMPLETE' }).click();
  const clearance = page.getByRole('dialog', { name: 'PDC clearance' }).locator('.efb-scroll');
  await expect(clearance).toBeVisible();
  await swipeUp(page, clearance);
  await page.getByRole('button', { name: 'CLOSE', exact: true }).click();
  await page.getByRole('button', { name: 'Stand information' }).click();
  const information = page.getByRole('dialog', { name: 'Stand information' }).locator('.efb-scroll');
  await expect(information).toBeVisible();
  await swipeUp(page, information);
});

test('chart controls fit a narrow viewport', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 700 });
  await openEfb(page);
  await page.getByAltText('Charts', { exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'EKCH' });
  await expect(dialog).toBeVisible();
  const bounds = await dialog.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom };
  });
  expect(bounds.left).toBeGreaterThanOrEqual(0);
  expect(bounds.right).toBeLessThanOrEqual(390);
  expect(bounds.top).toBeGreaterThanOrEqual(0);
  expect(bounds.bottom).toBeLessThanOrEqual(700);
  await expect(dialog.getByRole('button', { name: 'CLICK TO CLOSE' })).toBeVisible();
});
