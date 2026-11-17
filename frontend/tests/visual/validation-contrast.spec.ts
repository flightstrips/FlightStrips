import { expect, test } from "@playwright/test";

for (const scenario of ["closed", "pdc"]) {
  for (const validation of ["active", "inactive"]) {
    for (const manual of [true, false]) {
      test(`dark callsigns without a request: ${scenario}, validation=${validation}, manual=${manual}`, async ({ page }) => {
        await page.goto(`/visual-tests/validation-contrast.html?scenario=${scenario}&request=none&validation=${validation}&manual=${manual}`);
        for (const name of ["arrival", "pushback", "half"]) {
          const fixture = page.getByTestId(name);
          const callsign = fixture.getByText("SAS123", { exact: true });
          await expect(callsign).toBeVisible();
          const samples = await callsign.evaluate((element) => {
            const row = element.parentElement!;
            const frequency = row.parentElement!.querySelector<HTMLElement>(":scope > div:last-child > span");
            const animations = row.getAnimations();
            const sample = (time: number) => {
              for (const animation of animations) {
                animation.pause();
                animation.currentTime = time;
              }
              return { color: getComputedStyle(element).color, frequency: frequency ? getComputedStyle(frequency).color : null };
            };
            return { count: animations.length, gray: sample(750), normal: sample(250) };
          });
          const animated = validation === "active" && name !== "half";
          expect(samples.count).toBe(animated ? 1 : 0);
          expect(samples.gray.color).toBe(animated ? "rgb(0, 0, 0)" : "rgb(255, 255, 255)");
          expect(samples.normal.color).toBe("rgb(255, 255, 255)");
          if (name !== "half") {
            expect(samples.normal.frequency).toBe("rgb(255, 255, 255)");
            expect(samples.gray.frequency).toBe(samples.normal.frequency);
          }
        }
      });
    }
  }

  test(`removing a request preserves dark manual callsign contrast: ${scenario}`, async ({ page }) => {
    await page.goto(`/visual-tests/validation-contrast.html?scenario=${scenario}&validation=inactive`);
    await page.getByRole("button", { name: "Remove request", exact: true }).click();
    for (const name of ["arrival", "pushback", "half"]) {
      await expect(page.getByTestId(name).getByText("SAS123", { exact: true })).toHaveCSS("color", "rgb(255, 255, 255)");
    }
  });
}

for (const scenario of ["closed", "pdc", "selected", "light", "inactive"]) {
  test(`callsign validation contrast preserves both phases and the frequency row: ${scenario}`, async ({ page }) => {
    await page.goto(`/visual-tests/validation-contrast.html?scenario=${scenario}`);
    for (const name of ["arrival", "pushback", "apronTaxi", "cleared", "delivery", "final", "ground", "taxiDep"]) {
      // Summary strips have a separate seven-second PDC acknowledgement flash.
      if (scenario === "pdc" && (name === "cleared" || name === "delivery")) continue;
      const fixture = page.getByTestId(name);
      const callsign = fixture.getByText("SAS123", { exact: true });
      await expect(callsign).toBeVisible();
      const samples = await callsign.evaluate((element) => {
        const row = element.parentElement!;
        const column = row.parentElement!;
        const frequency = row.tagName === "BUTTON" ? null : column.querySelector<HTMLElement>(":scope > div:last-child > span");
        const animations = row.getAnimations();
        const sample = (time: number) => {
          for (const animation of animations) {
            animation.pause();
            animation.currentTime = time;
          }
          return {
            color: getComputedStyle(element).color,
            background: getComputedStyle(row).backgroundColor,
            frequencyColor: frequency ? getComputedStyle(frequency).color : null,
            frequencyBackground: frequency ? getComputedStyle(column).backgroundColor : null,
          };
        };
        return { count: animations.length, gray: sample(750), normal: sample(250) };
      });
      if (scenario === "inactive") {
        expect(samples.count).toBe(0);
        expect(samples.gray.color).toBe("rgb(255, 255, 255)");
      } else {
        expect(samples.count).toBe(1);
        expect(samples.gray.background).toBe("rgb(167, 167, 167)");
        expect(samples.gray.color).toBe("rgb(0, 0, 0)");
        const manualColor = ["arrival", "pushback", "cleared", "delivery"].includes(name) ? "rgb(33, 50, 106)" : "rgb(0, 0, 0)";
        const expectedNormal = scenario === "selected" ? "rgb(0, 0, 0)"
          : scenario === "light" ? manualColor : "rgb(255, 255, 255)";
        expect(samples.normal.color).toBe(expectedNormal);
        if (scenario === "selected") expect(samples.normal.background).toBe("rgb(255, 0, 245)");
      }
      expect(samples.gray.frequencyColor).toBe(samples.normal.frequencyColor);
      expect(samples.gray.frequencyBackground).toBe(samples.normal.frequencyBackground);
    }
  });
}

test("yellow unexpected-change cells keep dark text on closed-runway strips", async ({ page }) => {
  await page.clock.install();
  await page.goto("/visual-tests/validation-contrast.html?scenario=warnings");
  for (const rootColor of ["rgb(255, 255, 255)", "rgb(0, 0, 0)"]) {
    await page.clock.runFor(1100);
    await expect(page.getByTestId("arrival").locator(":scope > div").first()).toHaveCSS("color", rootColor);
    for (const name of ["arrival", "pushback", "apronTaxi", "cleared", "delivery", "taxiDep", "half"]) {
      const fixture = page.getByTestId(name);
      await expect(fixture.getByText("SAS123", { exact: true })).toBeVisible();
      const labels = [name === "pushback" ? "K3" : "A18"];
      if (["arrival", "pushback", "apronTaxi"].includes(name)) labels.push("04L");
      if (["apronTaxi", "taxiDep"].includes(name)) labels.push("K3");
      for (const label of labels) {
        const warning = fixture.getByText(label, { exact: true });
        await expect(warning).toHaveCSS("color", "rgb(0, 0, 0)");
        expect(await warning.evaluate((element) => {
          const root = element.closest("[data-testid]");
          for (let cell: HTMLElement | null = element; cell && cell !== root; cell = cell.parentElement) {
            if (getComputedStyle(cell).backgroundColor === "rgb(255, 215, 0)") {
              return true;
            }
          }
          return false;
        }), `${name}/${label} has a yellow warning background`).toBe(true);
      }
    }
  }
});
