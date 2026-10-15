import { readFileSync } from 'node:fs';
import { test, expect } from '@playwright/test';

const index = JSON.parse(readFileSync(new URL('../../storybook-static/index.json', import.meta.url), 'utf8')) as {
  entries: Record<string, { id: string; type: string; title: string; name: string }>;
};

for (const story of Object.values(index.entries).filter(entry => entry.type === 'story')) {
  test(`${story.title}: ${story.name}`, async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto(`/iframe.html?id=${story.id}&viewMode=story`);
    await expect(page.locator('#storybook-root')).toBeVisible();
    await expect(page.locator('#storybook-root > *').first()).toBeVisible();
    await expect(page.locator('body')).not.toHaveClass(/sb-show-errordisplay/);
    expect(errors).toEqual([]);
  });
}

test('review guide renders without the application MDX pipeline', async ({ page }) => {
  await page.goto('/iframe.html?id=start-here--docs&viewMode=docs');
  await expect(page.getByRole('heading', { name: 'FlightStrips component catalogue' })).toBeVisible();
});

test('reviewer can open and dismiss a confirmation dialog', async ({ page }) => {
  await page.goto('/iframe.html?id=shared-controls-examples--confirmation-dialog&viewMode=story&runPlayFunction=false');
  await page.getByRole('button', { name: 'Open confirmation' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await expect(page.getByRole('button', { name: 'Open confirmation' })).toBeFocused();
});
