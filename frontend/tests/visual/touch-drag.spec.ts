import { expect, test, type CDPSession, type Page } from "@playwright/test";

test.use({ hasTouch: true });

async function dispatchTouch(
  page: Page,
  browserName: string,
  session: CDPSession | null,
  type: "touchStart" | "touchMove" | "touchEnd",
  x?: number,
  y?: number,
) {
  if (browserName === "chromium") {
    await session!.send("Input.dispatchTouchEvent", {
      type,
      touchPoints: x == null || y == null ? [] : [{ x, y, id: 1, radiusX: 4, radiusY: 4, force: 1 }],
    });
    return;
  }

  await page.evaluate(({ eventType, clientX, clientY }) => {
    interface PointerTestWindow extends Window {
      __pointerTarget?: Element;
      __pointerPoint?: { x: number; y: number };
    }
    const pointerWindow = window as PointerTestWindow;
    if (eventType === "touchStart") {
      pointerWindow.__pointerTarget = document.elementFromPoint(clientX!, clientY!) ?? undefined;
    }
    if (clientX != null && clientY != null) {
      pointerWindow.__pointerPoint = { x: clientX, y: clientY };
    }
    const target = pointerWindow.__pointerTarget;
    const point = pointerWindow.__pointerPoint;
    if (!target || !point) throw new Error("Pointer target was not initialized");

    const touchEventName = eventType === "touchStart"
      ? "touchstart"
      : eventType === "touchMove"
        ? "touchmove"
        : "touchend";
    const touch = new Touch({
      identifier: 1,
      target,
      clientX: point.x,
      clientY: point.y,
      screenX: point.x,
      screenY: point.y,
      pageX: point.x,
      pageY: point.y,
      radiusX: 4,
      radiusY: 4,
      force: 1,
    });
    target.dispatchEvent(new TouchEvent(touchEventName, {
      touches: eventType === "touchEnd" ? [] : [touch],
      targetTouches: eventType === "touchEnd" ? [] : [touch],
      changedTouches: [touch],
      bubbles: true,
      cancelable: true,
    }));
    if (eventType === "touchEnd") {
      delete pointerWindow.__pointerTarget;
      delete pointerWindow.__pointerPoint;
    }
  }, { eventType: type, clientX: x, clientY: y });
}

test("a strip follows a touch gesture across production bay contexts", async ({ page, browserName }) => {
  await page.goto("/visual-tests/touch-drag-preview.html");

  const source = page.getByTestId("source-strip");
  const targetBay = page.locator('[data-strip-scroll-container="true"]').nth(1);
  const sourceBox = await source.boundingBox();
  const targetBox = await targetBay.boundingBox();
  expect(sourceBox).not.toBeNull();
  expect(targetBox).not.toBeNull();
  const sourceWrapper = source.locator("..");
  await expect(sourceWrapper).toHaveCSS("touch-action", "auto");
  await expect(sourceWrapper).toHaveAttribute("role", "button");
  await expect(sourceWrapper).toHaveAttribute("aria-disabled", "false");

  const from = { x: sourceBox!.x + sourceBox!.width / 2, y: sourceBox!.y + sourceBox!.height / 2 };
  const to = { x: targetBox!.x + targetBox!.width / 2, y: targetBox!.y + targetBox!.height / 2 };
  const session = browserName === "chromium" ? await page.context().newCDPSession(page) : null;

  await dispatchTouch(page, browserName, session, "touchStart", from.x, from.y);
  await dispatchTouch(page, browserName, session, "touchMove", from.x + 20, from.y + 20);
  await page.waitForTimeout(16);
  await expect(page.getByTestId("drag-overlay")).toBeVisible();
  const initialOverlayBox = await page.getByTestId("drag-overlay").boundingBox();
  expect(initialOverlayBox).not.toBeNull();
  expect(initialOverlayBox!.x).toBeGreaterThan(sourceBox!.x + 10);

  for (const progress of [0.25, 0.5, 0.75, 1]) {
    await dispatchTouch(
      page,
      browserName,
      session,
      "touchMove",
      from.x + (to.x - from.x) * progress,
      from.y + (to.y - from.y) * progress,
    );
    await page.waitForTimeout(16);
  }

  const movedOverlayBox = await page.getByTestId("drag-overlay").boundingBox();
  expect(movedOverlayBox).not.toBeNull();
  expect(movedOverlayBox!.x).toBeGreaterThan(initialOverlayBox!.x + 100);

  await dispatchTouch(page, browserName, session, "touchEnd");
  await expect(page.getByTestId("drag-status")).toContainText('"dropTarget":"DEPART"');
  await expect(page.getByTestId("target-strip")).toBeVisible();
});

test("a quick touch swipe scrolls a populated bay instead of starting a drag", async ({ page, browserName }) => {
  test.skip(browserName !== "chromium", "Playwright exposes native touch injection through CDP only in Chromium");
  await page.goto("/visual-tests/touch-drag-preview.html");

  const scrollBay = page.getByTestId("scroll-bay").locator('[data-strip-scroll-container="true"]');
  const startStrip = page.getByTestId("scroll-strip-11");
  const startBox = await startStrip.boundingBox();
  expect(startBox).not.toBeNull();
  const session = await page.context().newCDPSession(page);
  const x = startBox!.x + startBox!.width / 2;
  const startY = startBox!.y + startBox!.height / 2;

  await dispatchTouch(page, browserName, session, "touchStart", x, startY);
  for (const offset of [20, 40, 60, 80]) {
    await dispatchTouch(page, browserName, session, "touchMove", x, startY - offset);
    await page.waitForTimeout(16);
  }
  await dispatchTouch(page, browserName, session, "touchEnd");

  await expect.poll(() => scrollBay.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await expect(page.getByTestId("drag-overlay")).toBeHidden();
});

test("a held touch still drags a strip from a populated bay", async ({ page, browserName }) => {
  await page.goto("/visual-tests/touch-drag-preview.html");

  const source = page.getByTestId("scroll-strip-10");
  const targetBay = page.locator('[data-strip-scroll-container="true"]').nth(1);
  const sourceBox = await source.boundingBox();
  const targetBox = await targetBay.boundingBox();
  expect(sourceBox).not.toBeNull();
  expect(targetBox).not.toBeNull();
  const from = { x: sourceBox!.x + sourceBox!.width / 2, y: sourceBox!.y + sourceBox!.height / 2 };
  const to = { x: targetBox!.x + targetBox!.width / 2, y: targetBox!.y + targetBox!.height / 2 };
  const session = browserName === "chromium" ? await page.context().newCDPSession(page) : null;

  await dispatchTouch(page, browserName, session, "touchStart", from.x, from.y);
  await page.waitForTimeout(200);
  await dispatchTouch(page, browserName, session, "touchMove", from.x + 20, from.y + 10);
  await expect(page.getByTestId("drag-overlay")).toBeVisible();

  for (const progress of [0.5, 1]) {
    await dispatchTouch(
      page,
      browserName,
      session,
      "touchMove",
      from.x + (to.x - from.x) * progress,
      from.y + (to.y - from.y) * progress,
    );
    await page.waitForTimeout(16);
  }
  await dispatchTouch(page, browserName, session, "touchEnd");

  await expect(page.getByTestId("drag-status")).toContainText('"dropTarget":"DEPART"');
});
