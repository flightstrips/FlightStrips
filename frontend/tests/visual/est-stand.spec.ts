import { expect, test } from "@playwright/test";

test("SAT assignments stay hidden while physical stand occupants remain visible", async ({ page }, testInfo) => {
  await page.goto("/visual-tests/est-preview.html");

  const assignedInbound = page.getByTestId("assigned-inbound");
  const parkedArrival = page.getByTestId("parked-arrival");
  const reservedDeparture = page.getByTestId("reserved-departure");

  await expect(assignedInbound.getByRole("button", { name: "A18" })).toHaveCSS("background-color", "rgb(217, 217, 217)");
  await expect(assignedInbound).not.toContainText("SAS401");
  await expect(assignedInbound).not.toContainText(/CONFIRMED|AUTOMATIC|ETA|EXP/);

  const parkedStand = parkedArrival.getByRole("button");
  await expect(parkedStand).toHaveCSS("background-color", "rgb(255, 242, 142)");
  await expect(parkedStand).toContainText("A19");
  await expect(parkedStand).toContainText("NAX782");
  await expect(parkedStand).toContainText("A320");
  await expect(parkedStand).not.toContainText("22L");

  await expect(reservedDeparture.getByRole("button", { name: "A20" })).toHaveCSS("background-color", "rgb(217, 217, 217)");
  await expect(reservedDeparture).not.toContainText("SAS900");
  await expect(reservedDeparture).not.toContainText(/RESERVED|AUTOMATIC|EXP/);

  await testInfo.attach("est-arrival-visibility", {
    body: await page.getByTestId("est-arrival-visual").screenshot(),
    contentType: "image/png",
  });
});
