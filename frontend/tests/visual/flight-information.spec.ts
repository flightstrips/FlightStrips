import {expect, test} from "@playwright/test";
import {flightInformationDetail} from "../../src/visual-tests/flight-information-fixture";

const detailURL = "**/api/aman/airports/EKCH/flights/SAS1778/detail";

for (const width of [1920, 1280, 1024]) {
  test(`keeps the holding row proportional to the Figma strip at ${width}px`, async ({page}, testInfo) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.setViewportSize({width, height: 1080});
    await page.route(detailURL, async (route) => {
      expect(route.request().headers().authorization).toBe("Bearer visual-preview");
      await route.fulfill({json: flightInformationDetail});
    });
    await page.goto("/visual-tests/flight-information-preview.html?api");
    const summary = page.getByLabel("Flight information summary");
    const holding = page.getByLabel("Holding information");
    await expect(page.getByLabel("Holding EAT", {exact: true})).toHaveText("21:31:30");
    await page.evaluate(() => document.fonts.ready);

    await expect(page.getByLabel("Holding fix", {exact: true})).toHaveText("ROSBI");
    await expect(page.getByLabel("Expected holding duration")).toHaveText("11:30");
    await expect(holding).toContainText("UTC");
    await expect(page.getByLabel("Initial runway ETA")).toHaveText("21:27:37");
    await expect(page.getByLabel("Current runway ETA")).toHaveText("21:29:03");
    await expect(page.getByLabel("Current runway STA")).toHaveText("21:27:37");
    await expect(page.getByLabel("Current total delay")).toHaveText("−1′26″");
    await expect(page.getByLabel("Current accumulated delay")).toHaveText("−1′26″");
    for (const field of ["Initial STA-FF", "Current STA-FF", "Initial runway STA", "Initial accumulated delay"]) {
      await expect(page.getByLabel(field, {exact: true})).toBeEmpty();
    }
    await expect(page.getByRole("button", {name: "Technical evidence"})).toHaveAttribute("aria-expanded", "false");

    const rowBox = (await holding.boundingBox())!;
    const timingBox = (await page.getByLabel("Current runway ETA").boundingBox())!;
    // A holding row should occupy one strip row, including when the strip
    // scales down. A fixed 44px row fails this at laptop viewport widths.
    expect(Math.abs(rowBox.height - timingBox.height)).toBeLessThan(1.5);
    const holdingFont = await holding.evaluate((element) => getComputedStyle(element).fontSize);
    await expect(summary.getByText("ETA-FF", {exact: true})).toHaveCSS("font-size", holdingFont);
    expect(rowBox.y).toBeGreaterThanOrEqual(timingBox.y + timingBox.height);
    const closePanel = (await page.getByTestId("flight-detail-close-panel").boundingBox())!;
    const closeButton = (await page.getByRole("button", {name: "Close flight detail"}).boundingBox())!;
    const stripTop = (await summary.getByText("Trajectory", {exact: true}).boundingBox())!.y;
    await expect(page.getByRole("button", {name: "Close flight detail"})).toHaveCSS("outline-style", "none");
    await expect(page.getByRole("button", {name: "Close flight detail"})).toHaveCSS("border-width", "0px");
    // The action column spans the strip and holding row, with the original
    // Figma insets. No unused holding-row background extends beneath Close.
    expect(closePanel.y).toBeCloseTo(stripTop, 0);
    expect(closePanel.y + closePanel.height).toBeCloseTo(rowBox.y + rowBox.height, 0);
    expect(rowBox.x + rowBox.width).toBeCloseTo(closePanel.x, 0);
    expect(closePanel.width / (await summary.boundingBox())!.width).toBeCloseTo(80 / 1658, 3);
    expect((closeButton.x - closePanel.x) / closePanel.width).toBeCloseTo(6 / 80, 2);
    expect((closePanel.y + closePanel.height - closeButton.y - closeButton.height) / closePanel.width).toBeCloseTo(13 / 80, 2);
    expect(await summary.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
    for (const field of ["Current runway ETA", "Current runway STA", "Current total delay", "Holding EAT", "Expected holding duration"]) {
      const overflow = await page.getByLabel(field, {exact: true}).evaluate((element) => {
        const text = document.createRange();
        text.selectNodeContents(element);
        const textBox = text.getBoundingClientRect();
        const box = element.getBoundingClientRect();
        return Math.max(box.left - textBox.left, textBox.right - box.right);
      });
      expect(overflow, `${field} must fit its cell`).toBeLessThan(1);
    }
    expect(await summary.locator("img").evaluateAll((images) => images.every((image) => image.complete && image.naturalWidth > 0))).toBe(true);
    await expect(summary).toBeInViewport({ratio: 1});
    const currentETA = (await page.getByLabel("Current runway ETA").boundingBox())!;
    const currentSTA = (await page.getByLabel("Current runway STA").boundingBox())!;
    const initialSTA = (await page.getByLabel("Initial STA-FF").boundingBox())!;
    const screenshot = await page.screenshot({animations: "disabled"});
    const seamPixels = await page.evaluate(async ({base64, points}) => {
      const image = new Image();
      image.src = `data:image/png;base64,${base64}`;
      await image.decode();
      const canvas = document.createElement("canvas");
      canvas.width = image.width;
      canvas.height = image.height;
      const context = canvas.getContext("2d")!;
      context.drawImage(image, 0, 0);
      return points.map(([x, y]) => [...context.getImageData(Math.floor(x), Math.floor(y), 1, 1).data].slice(0, 3));
    }, {base64: screenshot.toString("base64"), points: [
      [currentSTA.x + 5, currentSTA.y],
      [initialSTA.x + 5, initialSTA.y],
      [currentETA.x, currentETA.y + 3],
    ]});
    // Undivided fields share a continuous cream surface, even where their
    // CSS grid edges fall between device pixels. Dark seams are not dividers.
    expect(seamPixels).toEqual([[239, 241, 233], [239, 241, 233], [239, 241, 233]]);
    await summary.screenshot({path: testInfo.outputPath(`flight-information-${width}.png`), animations: "disabled"});
    await testInfo.attach(`Flight information at ${width}px`, {path: testInfo.outputPath(`flight-information-${width}.png`), contentType: "image/png"});
    expect(errors).toEqual([]);
  });
}

test("refreshes EAT from HTTP detail and clears it without substituting another clock", async ({page}, testInfo) => {
  const response = structuredClone(flightInformationDetail);
  await page.route(detailURL, (route) => route.fulfill({json: response}));
  await page.goto("/visual-tests/flight-information-preview.html?api");
  const eat = page.getByLabel("Holding EAT", {exact: true});
  await expect(eat).toHaveText("21:31:30");

  response.holding_plan!.approach_release_time = "2026-10-21T01:34:45+02:00";
  await page.evaluate(() => window.dispatchEvent(new Event("aman-preview-refresh")));
  await expect(eat).toHaveText("23:34:45");
  response.holding_plan = null;
  await page.evaluate(() => window.dispatchEvent(new Event("aman-preview-refresh")));
  await expect(eat).toBeEmpty();
  await expect(page.getByLabel("Expected holding duration")).toBeEmpty();
  await expect(page.getByLabel("Holding information")).not.toContainText("UTC");
  await expect(page.getByLabel("Holding fix", {exact: true})).toHaveText("ROSBI");
  await page.getByLabel("Flight information summary").screenshot({path: testInfo.outputPath("flight-information-no-eat.png"), animations: "disabled"});
});

test("opens real technical evidence and supports keyboard closing and reopening", async ({page}) => {
  await page.route(detailURL, (route) => route.fulfill({json: flightInformationDetail}));
  await page.goto("/visual-tests/flight-information-preview.html?api");
  const close = page.getByRole("button", {name: "Close flight detail"});
  await expect(page.getByLabel("Holding EAT", {exact: true})).toHaveText("21:31:30");
  await expect(page.getByRole("dialog")).toBeFocused();
  await expect(close).toHaveCSS("outline-style", "none");
  await expect(close).toHaveCSS("text-decoration-line", "none");
  await page.keyboard.press("Tab");
  await expect(page.getByLabel("STAR waypoint estimates")).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", {name: "Technical evidence"})).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(close).toBeFocused();
  await expect(close).toHaveCSS("text-decoration-line", "underline");
  const technical = page.getByRole("button", {name: "Technical evidence"});
  await technical.click();
  await expect(technical).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByRole("heading", {name: "Expected holding & approach release"})).toBeVisible();
  await expect(page.getByLabel("Holding EAT", {exact: true})).toBeVisible();
  await technical.click();
  await expect(technical).toHaveAttribute("aria-expanded", "false");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", {name: "Open flight information"}).click();
  await expect(page.getByLabel("Holding EAT", {exact: true})).toHaveText("21:31:30");
  await close.click();
  await expect(page.getByRole("button", {name: "Open flight information"})).toBeFocused();
});
