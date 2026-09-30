import { expect, test } from "@playwright/test";

const STRIP_TYPES = [
  "pre-clearance",
  "cleared-summary",
  "compact-clearance",
  "pushback",
  "startup",
  "apron-departure",
  "tower-departure",
  "final-arrival",
  "apron-arrival",
  "compact-flight",
  "control-zone",
  "tactical-memaid",
  "tactical-crossing",
  "tactical-start",
  "tactical-land",
  "message",
] as const;

test.beforeEach(async ({ page }) => {
  // Keep the fixture's 13:25 TOBT / 13:31 TSAT in the overdue state captured by the snapshots.
  await page.clock.setFixedTime(new Date("2026-08-16T14:00:00Z"));
});

test("tactical SI cells stay square at different viewport shapes", async ({ page }) => {
  for (const viewport of [{ width: 1280, height: 720 }, { width: 1920, height: 1080 }, { width: 2560, height: 1080 }]) {
    await page.setViewportSize(viewport);
    for (const stripType of ["tactical-memaid", "tactical-crossing", "tactical-start", "tactical-land"]) {
      await page.goto(`/strip-gallery?shot=${stripType}`);
      const si = page.getByTestId(`strip-fixture-${stripType}`).locator(".bg-white");
      await expect(si).toBeVisible();
      const box = await si.boundingBox();
      expect(box).not.toBeNull();
      expect(Math.abs(box!.width - box!.height), `${stripType} at ${viewport.width}×${viewport.height}`).toBeLessThan(1);
    }
  }
});

test("every strip type keeps its content inside the framed height", async ({ page, browserName }) => {
  expect(page.viewportSize()).toEqual({ width: 1920, height: 1080 });

  for (const stripType of STRIP_TYPES) {
    await page.goto(`/strip-gallery?shot=${stripType}`);
    const bay = page.locator(`[data-shot="${stripType}"]`);
    const fixture = page.getByTestId(`strip-fixture-${stripType}`);
    await expect(bay).toBeVisible();
    await expect(fixture).toBeVisible();

    const bayBox = await bay.boundingBox();
    expect(bayBox).not.toBeNull();
    expect(bayBox!.width).toBeGreaterThan(300);
    expect(bayBox!.height).toBeGreaterThan(100);
    await expect(bay).toHaveAttribute("data-layout-source", /.+ · .+/);

    const geometry = await fixture.evaluate((fixtureElement) => {
      const face = fixtureElement.firstElementChild as HTMLElement | null;
      if (!face) return { faceHeight: 0, overflow: ["missing strip face"], escapedText: [], miscenteredText: [] };

      const faceRect = face.getBoundingClientRect();
      const overflow = face.scrollHeight > face.clientHeight + 1 ? ["strip face"] : [];

      const escapedText: string[] = [];
      const walker = document.createTreeWalker(face, NodeFilter.SHOW_TEXT);
      let node = walker.nextNode();
      while (node) {
        const text = node.textContent?.trim();
        if (text) {
          const range = document.createRange();
          range.selectNodeContents(node);
          const rect = range.getBoundingClientRect();
          if (rect.width > 0 && rect.height > 0
            && (rect.top < faceRect.top - 1 || rect.bottom > faceRect.bottom + 1)) {
            escapedText.push(text);
          }
        }
        node = walker.nextNode();
      }

      const miscenteredText = [...face.querySelectorAll<HTMLElement>("*")].flatMap((element) => {
        const style = getComputedStyle(element);
        if (style.display !== "flex" || style.alignItems !== "center" || element.children.length !== 1
          || parseFloat(style.paddingTop) !== 0 || parseFloat(style.paddingBottom) !== 0) return [];
        const child = element.firstElementChild;
        if (!(child instanceof HTMLElement) || child.tagName !== "SPAN") return [];
        const text = child.textContent?.trim();
        if (!text) return [];
        const cellRect = element.getBoundingClientRect();
        const textRect = child.getBoundingClientRect();
        const centerOffset = Math.abs((textRect.top + textRect.bottom - cellRect.top - cellRect.bottom) / 2);
        return centerOffset > 2 ? [`${text}: ${centerOffset.toFixed(2)}px`] : [];
      });

      return { faceHeight: faceRect.height, overflow, escapedText, miscenteredText };
    });

    expect(geometry.faceHeight, `${stripType} should have a visible strip height`).toBeGreaterThan(15);
    expect(geometry.overflow, `${stripType} has vertically overflowing cells`).toEqual([]);
    expect(geometry.escapedText, `${stripType} has text outside its frame`).toEqual([]);
    expect(geometry.miscenteredText, `${stripType} has text that is not vertically centered`).toEqual([]);

    if (stripType === "apron-departure") {
      await expect(fixture.getByText("B738/M", { exact: true })).toBeVisible();
      await expect(fixture.getByText("PH-PJK", { exact: true })).toBeVisible();
    } else if (process.platform === "win32" && browserName === "firefox" && stripType.startsWith("tactical-")) {
      // Tactical pixel baselines run in both browsers on Linux and in Chromium on Windows.
      if (stripType === "tactical-start" || stripType === "tactical-land") {
        await expect(fixture.getByRole("button", { name: "Start tactical timer" })).toBeVisible();
      }
    } else {
      await expect(bay).toHaveScreenshot(`${stripType}.png`, {
        animations: "disabled",
        caret: "hide",
        // Allow a few pixels of platform-specific font antialiasing noise while
        // keeping the comparison strict enough to catch layout regressions.
        maxDiffPixels: 5,
      });
    }
  }
});

test("the final-arrival runway and TWY rows retain their two-thirds/one-third split", async ({ page }) => {
  await page.goto("/strip-gallery?shot=final-arrival");

  const runwayRow = page.getByTestId("final-arrival-runway-row");
  const twyRow = page.getByTestId("final-arrival-twy-row");
  const twyLabel = page.getByTestId("final-arrival-twy-label");
  await expect(runwayRow).toContainText("04L");
  await expect(twyRow).toContainText("TWY");
  await expect(runwayRow).toHaveCSS("border-bottom-width", "0px");
  await expect(twyRow).toHaveCSS("border-top-width", "0px");

  const [runwayBox, twyBox, twyLabelBox] = await Promise.all([
    runwayRow.boundingBox(),
    twyRow.boundingBox(),
    twyLabel.boundingBox(),
  ]);
  expect(runwayBox).not.toBeNull();
  expect(twyBox).not.toBeNull();
  expect(twyLabelBox).not.toBeNull();
  expect(runwayBox!.height / twyBox!.height).toBeCloseTo(2, 1);
  expect(twyBox!.y).toBeCloseTo(runwayBox!.y + runwayBox!.height, 0);
  const twyCenterOffset = (twyLabelBox!.y + twyLabelBox!.height / 2) - (twyBox!.y + twyBox!.height / 2);
  expect(twyCenterOffset).toBeCloseTo(-1, 0);
});

test("the apron-arrival strip shows the aircraft registration", async ({ page }) => {
  await page.goto("/strip-gallery?shot=apron-arrival");

  await expect(page.getByTestId("strip-fixture-apron-arrival")).toContainText("PH-PJK");
});
