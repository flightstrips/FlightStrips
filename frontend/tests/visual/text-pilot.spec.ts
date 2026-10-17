import { expect, test } from "@playwright/test";

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1280, height: 720 }]) {
for (const variant of ["push", "apron-out", "apron-in", "tower-out", "tower-in", "si", "runway"]) {
  test(`${variant} composer remains usable at ${viewport.width}x${viewport.height}`, async ({ page }, testInfo) => {
    await page.setViewportSize(viewport);
    await page.goto(`/visual-tests/text-pilot-preview.html?variant=${variant}`);
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(page.getByRole("textbox", { name: "Text message" })).not.toHaveValue("");
    const bounds = await dialog.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0); expect(bounds!.y).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport.width + 1); expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height + 1);
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({ path: testInfo.outputPath(`${variant}.png`) });
    await page.getByRole("button", { name: "ERASE", exact: true }).click();
    await expect(page.getByRole("textbox")).toHaveValue("");
    await expect(page.getByRole("button", { name: "SEND", exact: true })).toBeDisabled();
    await page.getByRole("button", { name: "ESC", exact: true }).click();
    await expect(dialog).not.toBeVisible();
  });
}

}

test("push composer opens above the map and ESC retains the map", async ({ page }, testInfo) => {
  await page.goto("/visual-tests/text-pilot-preview.html?entry=push-map");
  const map = page.getByRole("dialog", { name: "Select Release Point" });
  await expect(map).toBeVisible();
  await map.getByRole("button", { name: "J4", exact: true }).click();
  const composer = page.getByRole("dialog", { name: "SAS123 text message" });
  await expect(composer).toBeVisible();
  await expect(composer.getByRole("textbox")).toHaveValue("Push and start approved, Release point J4.");
  await page.screenshot({ path: testInfo.outputPath("composer-over-map.png") });
  await composer.getByRole("button", { name: "ERASE", exact: true }).click();
  await expect(composer.getByRole("textbox")).toHaveValue("");
  await composer.getByRole("button", { name: "ESC", exact: true }).click();
  await expect(composer).not.toBeVisible();
  await expect(map).toBeVisible();
});

test("indicators match departure field and arrival rightmost cells", async ({ page }, testInfo) => {
  await page.goto("/visual-tests/text-pilot-preview.html?entry=indicators");
  await page.screenshot({ path: testInfo.outputPath("indicators.png"), fullPage: true });
  const indicators = page.getByTestId("text-pilot-indicator");
  await expect(indicators).toHaveCount(24);
  for (const indicator of await indicators.all()) {
    const geometry = await indicator.evaluate(node => {
      const square = node.getBoundingClientRect();
      const parent = node.parentElement!.getBoundingClientRect();
      const glyph = node.firstElementChild!.getBoundingClientRect();
      return { width: square.width, height: square.height, parentHeight: parent.height, right: square.right - parent.right, bottom: square.bottom - parent.bottom, glyphHeight: glyph.height };
    });
    expect(Math.abs(geometry.width - geometry.height)).toBeLessThan(1);
    expect(Math.abs(geometry.height - geometry.parentHeight / 3)).toBeLessThan(1);
    expect(Math.abs(geometry.right)).toBeLessThan(3);
    expect(Math.abs(geometry.bottom)).toBeLessThan(3);
    expect(geometry.glyphHeight).toBeLessThanOrEqual(geometry.height);
  }
  for (const [index, row] of (await page.getByTestId("indicator-departure").all()).entries()) {
    const indicator = row.getByTestId("text-pilot-indicator");
    await expect(indicator.locator("..")).toContainText([2, 3].includes(index % 5) ? "B738" : "SAS123");
    await expect(indicator).toHaveCSS("border-left-width", "1px");
  }
  for (const row of await page.getByTestId("indicator-arrival").all()) {
    const indicator = row.getByTestId("text-pilot-indicator");
    await expect(indicator).toHaveCSS("border-left-width", "0px");
    const geometry = await indicator.evaluate(node => {
      const cell = node.parentElement!;
      const strip = cell.parentElement!;
      return { right: cell.getBoundingClientRect().right - strip.getBoundingClientRect().right + parseFloat(getComputedStyle(strip).paddingRight) + parseFloat(getComputedStyle(strip).borderRightWidth), lastCell: cell === strip.lastElementChild };
    });
    expect(geometry.lastCell).toBe(true);
    expect(Math.abs(geometry.right)).toBeLessThan(3);
  }
});
