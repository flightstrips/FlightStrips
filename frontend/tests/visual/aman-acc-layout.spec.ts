import {expect, test} from "@playwright/test";

for (const viewport of [{width: 1380, height: 768}, {width: 1440, height: 960}, {width: 1920, height: 1080}, {width: 2560, height: 1080}]) {
  test(`preserves holding widths while ACC timelines expand at ${viewport.width}x${viewport.height}`, async ({page}) => {
    await page.setViewportSize(viewport);
    await page.goto("/visual-tests/aman-gap-preview.html?acc-layout");
    await page.getByRole("button", {name: "ACC", exact: true}).click();
    await page.getByRole("button", {name: "MAESTRO", exact: true}).click();
    const menu = page.getByRole("menu", {name: "MAESTRO"});
    for (const family of ["MONAK", "TIDVU", "ERNOV"]) {
      await menu.getByRole("menuitemcheckbox", {name: family}).click();
    }

    const workspace = page.getByRole("main", {name: "Arrival management workspace"});
    const board = page.getByRole("region", {name: "AMAN presentation"});
    const header = page.locator('header[aria-label="MAESTRO settings"]');
    const utc = header.locator('[title^="UTC:"]');
    const timeline = page.getByTestId("aman-timeline-grid");
    const holdings = page.getByRole("region", {name: /^TMT holding information for /});
    const sequences = page.getByRole("region", {name: "Holding sequence"});
    const rulers = page.getByTestId(/^fmp-timeline-/);
    const baselineHolding = (await holdings.first().boundingBox())!;
    const baselineTimeline = (await timeline.boundingBox())!;
    const baselineRuler0 = (await rulers.nth(0).boundingBox())!;
    const baselineRuler1 = (await rulers.nth(1).boundingBox())!;
    const workspaceBox = (await workspace.boundingBox())!;
    await expect(holdings).toHaveCount(3);

    for (const [family, remaining] of [["MONAK", 2], ["TIDVU", 1]] as const) {
      await menu.getByRole("menuitemcheckbox", {name: family}).click();
      await expect(holdings).toHaveCount(remaining);
      await expect(sequences).toHaveCount(remaining);
      const currentTimeline = (await timeline.boundingBox())!;
      const currentBoard = (await board.boundingBox())!;
      const currentHeader = (await header.boundingBox())!;
      const currentUTC = (await utc.boundingBox())!;
      const currentRuler0 = (await rulers.nth(0).boundingBox())!;
      const currentRuler1 = (await rulers.nth(1).boundingBox())!;
      expect(Math.abs(currentTimeline.width - baselineTimeline.width - (3 - remaining) * (baselineHolding.width + 4))).toBeLessThan(1);
      expect(currentRuler1.x - currentRuler0.x).toBeGreaterThan(baselineRuler1.x - baselineRuler0.x);
      expect(Math.abs(currentBoard.x + currentBoard.width - (currentTimeline.x + currentTimeline.width))).toBeLessThan(1);
      expect(Math.abs(currentHeader.x + currentHeader.width - (currentTimeline.x + currentTimeline.width))).toBeLessThan(1);
      expect(Math.abs(currentTimeline.x + currentTimeline.width - (currentUTC.x + currentUTC.width) - 4)).toBeLessThan(1);
      for (let index = 0; index < remaining; index++) {
        const graph = (await holdings.nth(index).boundingBox())!;
        const sequence = (await sequences.nth(index).boundingBox())!;
        expect(Math.abs(graph.width - baselineHolding.width)).toBeLessThan(1);
        expect(Math.abs(sequence.width - baselineHolding.width)).toBeLessThan(1);
        expect(Math.abs(graph.x - sequence.x)).toBeLessThan(1);
      }
      const rightmost = (await holdings.last().boundingBox())!;
      expect(Math.abs(workspaceBox.x + workspaceBox.width - 16 - (rightmost.x + rightmost.width))).toBeLessThan(1);
    }
    await menu.getByRole("menuitemcheckbox", {name: "MONAK"}).click();
    await menu.getByRole("menuitemcheckbox", {name: "TIDVU"}).click();
    await expect(holdings).toHaveCount(3);
    expect(Math.abs((await timeline.boundingBox())!.width - baselineTimeline.width)).toBeLessThan(1);
    await menu.getByRole("menuitemcheckbox", {name: "MAESTRO", exact: true}).click();
    await expect(holdings).toHaveCount(5);
    expect(Math.abs((await timeline.boundingBox())!.width - baselineTimeline.width)).toBeLessThan(1);
  });
}
