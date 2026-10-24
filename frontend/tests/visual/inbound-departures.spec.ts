import {expect, test} from "@playwright/test";

for (const viewport of [{width: 1440, height: 960}, {width: 1920, height: 1080}]) {
  test(`shows the CTOT tab and scrolls the full list at ${viewport.width}px`, async ({page}) => {
    await page.setViewportSize(viewport);
    await page.goto("/visual-tests/inbound-departures-preview.html");
    const tab = page.getByRole("tab", {name: "CTOT (40)"});
    await expect(tab).toBeVisible();
    await tab.click();
    const panel = page.getByRole("tabpanel", {name: "CTOT (40)"});
    await expect(panel.getByRole("rowheader", {name: "FBT08F"})).toBeVisible();
    await expect(panel.getByRole("row").filter({hasText: "FBT08F"})).toContainText("−18m");
    const bounds = await panel.boundingBox();
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport.width);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height);
    await page.screenshot({path: test.info().outputPath("ctot-tab.png")});
    await panel.getByRole("rowheader", {name: "SAS39"}).scrollIntoViewIfNeeded();
    await expect(panel.getByRole("rowheader", {name: "SAS39"})).toBeVisible();
    await tab.focus();
    await page.keyboard.press("ArrowLeft");
    await expect(page.getByRole("tab", {name: "Excluded (0)"})).toBeFocused();
  });
}
