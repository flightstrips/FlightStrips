import {expect, test} from "@playwright/test";

for (const viewport of [{width: 1440, height: 960}, {width: 1920, height: 1080}]) {
  test(`shows the CTOT tab and scrolls the full list at ${viewport.width}px`, async ({page}) => {
    await page.setViewportSize(viewport);
    await page.goto("/visual-tests/inbound-departures-preview.html");
    const tab = page.getByRole("tab", {name: "CTOT (40)"});
    const nonCompliance = page.getByRole("region", {name: "CDM non-compliance"});
    await expect(tab).toBeVisible();
    await expect(nonCompliance).toBeHidden();
    await tab.click();
    const panel = page.getByRole("tabpanel", {name: "CTOT (40)"});
    const list = panel.getByRole("region", {name: "Inbound aircraft with CTOT"});
    await expect(list.getByRole("rowheader", {name: "FBT08F"})).toBeVisible();
    await expect(list.getByRole("row").filter({hasText: "FBT08F"})).toContainText("−18m");
    const nonComplianceList = panel.getByRole("region", {name: "CDM non-compliance"});
    await expect(nonComplianceList.getByRole("rowheader", {name: "FBT08F"})).toBeVisible();
    const bounds = await panel.boundingBox();
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport.width);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height);
    await page.screenshot({path: test.info().outputPath("ctot-tab.png")});
    await list.getByRole("rowheader", {name: "SAS39"}).scrollIntoViewIfNeeded();
    await expect(list.getByRole("rowheader", {name: "SAS39"})).toBeVisible();
    await nonComplianceList.getByRole("button", {name: "Remove FBT08F"}).click();
    await tab.focus();
    await page.keyboard.press("ArrowLeft");
    await expect(page.getByRole("tab", {name: "Excluded (0)"})).toBeFocused();
    await expect(nonCompliance).toBeHidden();
    await tab.click();
    await expect(nonComplianceList.getByRole("rowheader", {name: "FBT08F"})).toHaveCount(0);
  });
}
