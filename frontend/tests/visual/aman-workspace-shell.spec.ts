import {expect, test} from "@playwright/test";

const VIEWPORTS = [
  {name: "reference 3:2", width: 1440, height: 960},
  {name: "wide 16:9", width: 1920, height: 1080},
] as const;

for (const viewport of VIEWPORTS) {
  test(`keeps MAESTRO and TMT separated at ${viewport.name}`, async ({page}) => {
    await page.setViewportSize(viewport);
    await page.goto("/visual-tests/aman-workspace-shell-preview.html");

    const maestro = page.getByRole("region", {name: "MAESTRO sequence workspace"});
    const tmt = page.getByRole("complementary", {name: "TMT analysis area"});
    const [maestroBox, tmtBox] = await Promise.all([maestro.boundingBox(), tmt.boundingBox()]);

    expect(maestroBox).not.toBeNull();
    expect(tmtBox).not.toBeNull();
    expect(maestroBox!.x).toBeLessThan(tmtBox!.x);
    expect(maestroBox!.x + maestroBox!.width).toBeLessThan(tmtBox!.x);
    expect(tmtBox!.x + tmtBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(tmtBox!.width / tmtBox!.height).toBeCloseTo(3 / 4, 2);

    const settings = (await page.locator('header[aria-label="MAESTRO settings"]').boundingBox())!;
    expect(settings.height / maestroBox!.height).toBeGreaterThanOrEqual(0.125);
    expect(settings.height / maestroBox!.height).toBeLessThanOrEqual(0.145);
    await expect(page.getByTestId("timeline-reference")).toHaveCSS("width", "48px");
    await expect(page.getByTestId("target-reference")).toHaveCSS("width", "96px");
  });
}

test("gives useful extra width to MAESTRO on a wide viewport", async ({page}) => {
  await page.setViewportSize(VIEWPORTS[0]);
  await page.goto("/visual-tests/aman-workspace-shell-preview.html");
  const referenceWidth = (await page.getByRole("region", {name: "MAESTRO sequence workspace"}).boundingBox())!.width;

  await page.setViewportSize(VIEWPORTS[1]);
  const wideWidth = (await page.getByRole("region", {name: "MAESTRO sequence workspace"}).boundingBox())!.width;

  expect(wideWidth).toBeGreaterThan(referenceWidth);
});

test("shows keyboard focus and selected state without relying on color", async ({page}) => {
  await page.setViewportSize(VIEWPORTS[0]);
  await page.goto("/visual-tests/aman-workspace-shell-preview.html");
  const maestro = page.getByRole("button", {name: "Open target information preferences"});
  const all = page.getByRole("button", {name: "ALL"});

  await page.keyboard.press("Tab");
  await page.keyboard.press("Tab");
  await expect(maestro).toBeFocused();
  await expect(maestro).toHaveCSS("outline-style", "solid");
  await expect(all).toHaveAttribute("aria-pressed", "true");
  await expect(all).toContainText("✓");
  await expect(page.getByText(/READ ONLY/)).toBeVisible();
});
