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
