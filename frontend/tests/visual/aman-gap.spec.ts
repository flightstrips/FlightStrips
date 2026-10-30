import {expect, test} from "@playwright/test";

test.beforeEach(async ({page}) => {
  await page.clock.setFixedTime(new Date("2026-07-22T10:00:00.000Z"));
});

for (const viewport of [{width: 1440, height: 960}, {width: 1920, height: 1080}]) {
  test(`matches Figma's paired STAR gap markers at ${viewport.width}px`, async ({page}) => {
    await page.setViewportSize(viewport);
    await page.goto("/visual-tests/aman-gap-preview.html");
    const timelines = page.getByTestId(/^fmp-timeline-/);
    await expect(timelines).toHaveCount(3);
    const tops: number[] = [];
    for (const timeline of await timelines.all()) {
      const axis = timeline.getByRole("img", {name: /^UTC timeline/});
      const gap = timeline.getByRole("button", {name: /^GAP ARRIVAL-22/});
      const bars = gap.locator("span");
      await expect(gap).toBeInViewport({ratio: 1});
      await expect(bars).toHaveCount(2);
      const axisBox = (await axis.boundingBox())!;
      const gapBox = (await gap.boundingBox())!;
      const left = (await bars.nth(0).boundingBox())!;
      const right = (await bars.nth(1).boundingBox())!;
      expect(axisBox.x - (left.x + left.width)).toBeCloseTo(3, 1);
      expect(right.x - (axisBox.x + axisBox.width)).toBeCloseTo(3, 1);
      for (const bar of await bars.all()) {
        await expect(bar).toHaveCSS("background-color", "rgb(156, 0, 0)");
        await expect(bar).toHaveCSS("width", "6px");
      }
      // Four minutes of the 90-minute timeline; future times run upward.
      expect(gapBox.height / axisBox.height).toBeCloseTo(4 / 90, 3);
      expect((gapBox.y - axisBox.y) / axisBox.height).toBeCloseTo(1 - 22 / 90, 3);
      tops.push(gapBox.y);
    }
    expect(Math.max(...tops) - Math.min(...tops)).toBeLessThan(1);
    await page.screenshot({path: test.info().outputPath("aman-gap-review.png"), fullPage: true});
  });
}

test("clips a gap at the horizon and preserves sub-minute duration", async ({page}) => {
  await page.goto("/visual-tests/aman-gap-preview.html?gap=clipped");
  await page.getByRole("scrollbar", {name: "Timeline scroll position"}).press("Home");
  const timeline = page.getByTestId("fmp-timeline-1");
  const axis = (await timeline.getByRole("img", {name: /^UTC timeline/}).boundingBox())!;
  const clipped = (await timeline.locator("[data-gap-id]").boundingBox())!;
  expect(clipped.y).toBeCloseTo(axis.y, 1);
  expect(clipped.height / axis.height).toBeCloseTo(2 / 90, 3);
  await page.goto("/visual-tests/aman-gap-preview.html?gap=short");
  const short = (await page.getByTestId("fmp-timeline-1").locator("[data-gap-id]").boundingBox())!;
  expect(short.height / axis.height).toBeCloseTo(0.1 / 90, 4);
});

test("keeps the gap until removal is confirmed and sends the correct command", async ({page}) => {
  await page.goto("/visual-tests/aman-gap-preview.html");
  const gaps = page.getByRole("button", {name: /^GAP ARRIVAL-22/});
  await gaps.first().focus();
  await expect(gaps.first()).toHaveCSS("outline-style", "solid");
  await gaps.first().press("Enter");
  const dialog = page.getByRole("dialog", {name: "Remove GAP"});
  await expect(dialog).toContainText("Inserted spacing");
  await dialog.getByRole("button", {name: "NO", exact: true}).click();
  await expect(gaps).toHaveCount(3);
  await expect(page.locator("#preview-commands")).toBeEmpty();
  await gaps.first().click();
  await dialog.getByRole("button", {name: "YES", exact: true}).click();
  await expect(page.locator("#preview-commands")).toContainText('"type":"aman.remove_gap"');
  await expect(page.locator("#preview-commands")).toContainText('"gap_id":"review-gap"');
  await expect(page.locator("#preview-commands")).toContainText('"runway_group_id":"ARRIVAL-22"');
  // A command is not a server-confirmed replacement.
  await expect(gaps).toHaveCount(3);
});

test("shows the same gap markers in a read-only session", async ({page}) => {
  await page.goto("/visual-tests/aman-gap-preview.html?readOnly");
  const gaps = page.getByRole("button", {name: /^GAP ARRIVAL-22/});
  await expect(gaps).toHaveCount(3);
  for (const gap of await gaps.all()) {
    await expect(gap).toBeVisible();
    await expect(gap).toBeDisabled();
    await expect(gap.locator("span").first()).toHaveCSS("background-color", "rgb(156, 0, 0)");
  }
});
