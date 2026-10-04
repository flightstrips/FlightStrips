import {expect, test} from "@playwright/test";

test("keeps holding aircraft labels separate at graph edges, shared altitudes and after resizing", async ({page}) => {
  await page.goto("/visual-tests/holding-graph-preview.html");
  for (const card of ["compact", "crowded", "full"]) {
    const graph = page.getByTestId(card).getByTestId("holding-graph");
    await expect(graph.locator("li").first()).toBeVisible();
    for (const height of [290, 180]) {
      await page.getByTestId(card).evaluate((element, height) => { element.style.height = `${height}px`; }, height);
      await expect(graph.locator("svg line")).toHaveCount(await graph.locator("li").count());
      const boxes = await graph.locator("li").evaluateAll(elements => elements.map(element => {
        const box = element.getBoundingClientRect();
        const axis = element.closest('[data-testid="holding-graph"]')!.querySelector('[data-testid="holding-altitude-axis"]')!.getBoundingClientRect();
        const level = Number(element.getAttribute("aria-label")!.match(/FL(\d+)/)![1]);
        const peers = elements.filter(peer => Number(peer.getAttribute("aria-label")!.match(/FL(\d+)/)![1]) === level);
        const center = peers.reduce((sum, peer) => {
          const box = peer.getBoundingClientRect();
          return sum + box.top + box.height / 2;
        }, 0) / peers.length;
        return {top: box.top, bottom: box.bottom, left: box.left, right: box.right,
          error: Math.abs(center - (axis.top + Number((element as HTMLElement).dataset.altitudeY)))};
      }));
      boxes.forEach((box, index) => {
        expect(box.error).toBeLessThan(0.1);
        boxes.slice(index + 1).forEach(other => {
          expect(box.bottom <= other.top || other.bottom <= box.top || box.right <= other.left || other.right <= box.left).toBe(true);
        });
      });
      const endpoints = await graph.locator("svg line").evaluateAll(lines => lines.map(element => {
        const line = element as SVGLineElement;
        const matrix = line.getScreenCTM()!;
        const start = new DOMPoint(line.x1.baseVal.value, line.y1.baseVal.value).matrixTransform(matrix);
        const end = new DOMPoint(line.x2.baseVal.value, line.y2.baseVal.value).matrixTransform(matrix);
        const graph = line.closest('[data-testid="holding-graph"]')!;
        const axis = graph.querySelector('[data-testid="holding-time-axis"]')!.getBoundingClientRect();
        const label = [...graph.querySelectorAll("li")].find(label => label.getAttribute("aria-label")!.startsWith(`${line.dataset.callsign},`))!;
        const box = label.getBoundingClientRect();
        const eat = label.getAttribute("aria-label")!.match(/EAT 18:(\d+)/)![1];
        return {startX: Math.abs(start.x - (axis.left + axis.width / 2)),
          startY: Math.abs(start.y - (axis.top + (1 - Number(eat) / 60) * axis.height)),
          endX: Math.abs(end.x - box.left), endY: Math.abs(end.y - (box.top + box.height / 2))};
      }));
      endpoints.forEach(errors => Object.values(errors).forEach(error => expect(error).toBeLessThan(0.1)));
    }
  }
  const crowded = page.getByTestId("crowded").getByTestId("holding-graph");
  expect(await crowded.evaluate(element => element.scrollHeight > element.clientHeight)).toBe(true);
  const lefts = await crowded.locator("li").evaluateAll(elements => elements.map(element => element.getBoundingClientRect().left));
  expect(new Set(lefts).size).toBe(1);
  await crowded.locator("li").last().focus();
  await expect(crowded.locator("li").last()).toBeInViewport();
});

test("gives holdings most of the dashboard height and uses thicker time lines", async ({page}) => {
  await page.goto("/visual-tests/holding-graph-preview.html");
  const dashboard = page.getByTestId("dashboard");
  const traffic = await dashboard.locator(".aman-tmt-traffic").boundingBox();
  const holdings = await dashboard.locator(".aman-tmt-holdings").boundingBox();
  expect(holdings!.height).toBeGreaterThan(traffic!.height * 2);
  await expect(dashboard.locator("svg line").first()).toHaveAttribute("stroke-width", "2");
  const altitudeAxis = await dashboard.getByTestId("holding-altitude-axis").first().boundingBox();
  expect(altitudeAxis!.height / 21).toBeLessThanOrEqual(21);
  await expect(dashboard.locator("li").first()).toHaveCSS("font-size", "10px");
  const timeAxis = await dashboard.getByTestId("holding-time-axis").first().boundingBox();
  expect(timeAxis!.height).toBeGreaterThanOrEqual(altitudeAxis!.height);
});

test("connects shared-level release times without crossing and keeps missing times explicit", async ({page}) => {
  await page.goto("/visual-tests/holding-graph-preview.html");
  const card = page.getByTestId("release-order");
  await expect(card.locator("li")).toHaveCount(4);
  await expect(card.locator("svg line")).toHaveCount(3);
  const endpoints = await card.locator("svg line").evaluateAll(elements => elements.map(element => {
    const line = element as SVGLineElement;
    return {start: line.y1.baseVal.value, end: line.y2.baseVal.value};
  }).sort((left, right) => left.start - right.start));
  expect(endpoints[0].end).toBeLessThan(endpoints[1].end);
  expect(endpoints[1].end).toBeLessThan(endpoints[2].end);
  await expect(card.getByTitle("No calculated EAT. See this aircraft in Current warnings for the reason.")).toHaveText("—");
  await card.screenshot({path: "test-results/holding-release-order.png"});
});
