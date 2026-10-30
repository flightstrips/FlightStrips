import {expect, test} from "@playwright/test";

// Match the desktop browser: Playwright's default hides the scrollbars that
// otherwise consume layout space and can expose ResizeObserver feedback loops.
test.use({launchOptions: {ignoreDefaultArgs: ["--hide-scrollbars"]}});

test("keeps graph dimensions stable with persistent scrollbars and fractional scaling", async ({page, browserName}) => {
  await page.goto("/visual-tests/holding-graph-preview.html?review");
  await page.addStyleTag({content: '[data-testid="holding-graph"]::-webkit-scrollbar { width: 15px; height: 15px; }'});
  await page.evaluate(() => document.fonts.ready);
  const graph = page.getByTestId("figma").getByTestId("holding-graph");
  await expect(graph).toHaveCSS("scrollbar-gutter", "stable");
  // The WebKit rule forces classic scrollbars in Chromium. Headless Firefox
  // uses overlay scrollbars, which do not reserve layout space.
  if (browserName === "chromium") {
    const gutter = await graph.evaluate(element => element.offsetWidth - element.clientWidth);
    expect(gutter).toBeGreaterThan(0);
  }
  for (const zoom of [1, 1.1, 1.25]) {
    await page.getByTestId("holding-review").evaluate((element, zoom) => { element.style.zoom = String(zoom); }, zoom);
    const samples = await page.getByTestId("holding-review").evaluate(async element => {
      const graphs = [...element.querySelectorAll<HTMLElement>('[data-testid="holding-graph"]')];
      const frames: string[][] = [];
      for (let frame = 0; frame < 90; frame++) {
        await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
        if (frame >= 30) frames.push(graphs.map(graph => [
          graph.clientWidth, graph.clientHeight,
          graph.firstElementChild!.getAttribute("style"),
          graph.parentElement!.querySelector('[data-testid="holding-graph-footer"]')!.getAttribute("style"),
        ].join("|")));
      }
      return graphs.map((_, index) => [...new Set(frames.map(frame => frame[index]))]);
    });
    samples.forEach((dimensions, index) => expect(dimensions, `graph ${index}, zoom ${zoom}`).toHaveLength(1));
  }
});

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
        const marker = line.closest("svg")!.querySelector("marker")!;
        const axis = graph.querySelector('[data-testid="holding-time-axis"]')!.getBoundingClientRect();
        const label = [...graph.querySelectorAll("li")].find(label => label.getAttribute("aria-label")!.startsWith(`${line.dataset.callsign},`))!;
        const box = label.getBoundingClientRect();
        const eat = label.getAttribute("aria-label")!.match(/EAT 18:(\d+)/)![1];
        return {startX: Math.abs(start.x - (axis.left + axis.width / 2)),
          startY: Math.abs(start.y - (axis.top + (1 - Number(eat) / 60) * axis.height)),
          // The line meets the left-facing triangle tip. Its base meets the box.
          endX: Math.abs(end.x + marker.markerWidth.baseVal.value * matrix.a - box.left),
          endY: Math.abs(end.y - (box.top + box.height / 2))};
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

test("fills the dashboard height with outward-facing rulers and arrow connectors", async ({page}) => {
  await page.goto("/visual-tests/holding-graph-preview.html");
  const dashboard = page.getByTestId("dashboard");
  const traffic = await dashboard.locator(".aman-tmt-traffic").boundingBox();
  const holdings = await dashboard.locator(".aman-tmt-holdings").boundingBox();
  expect(holdings!.height).toBeGreaterThan(traffic!.height * 2);
  await expect(dashboard.locator("svg line").first()).toHaveAttribute("stroke-width", "1");
  await expect(dashboard.locator("svg line").first()).toHaveAttribute("marker-end", /url\(#.+\)/);
  await expect(dashboard.locator("svg marker").first()).toHaveAttribute("orient", "0");
  await expect(dashboard.locator("svg marker").first()).toHaveAttribute("refX", "0");
  const altitudeAxis = await dashboard.getByTestId("holding-altitude-axis").first().boundingBox();
  expect(altitudeAxis!.height / 21).toBeGreaterThanOrEqual(21);
  await expect(dashboard.locator("li").first()).toHaveCSS("font-family", /Rubik/);
  const timeAxis = await dashboard.getByTestId("holding-time-axis").first().boundingBox();
  expect(timeAxis!.height).toBeGreaterThanOrEqual(altitudeAxis!.height);
});

test("matches the holding composition and doubles just the shared flight-level band", async ({page}) => {
  await page.goto("/visual-tests/holding-graph-preview.html?review");
  await expect(page.getByTestId("figma").locator("li")).toHaveCount(9);
  const geometry = async (card: string) => page.getByTestId(card).evaluate(element => {
    const graph = element.querySelector('[data-testid="holding-graph"]')!;
    const axis = graph.querySelector('[data-testid="holding-altitude-axis"]')!;
    const ticks = [...axis.querySelectorAll<HTMLElement>("[data-flight-level]")];
    const y = (level: number) => parseFloat(ticks.find(tick => Number(tick.dataset.flightLevel) === level)!.style.top);
    const labels = [...graph.querySelectorAll("li")].map(label => label.getBoundingClientRect());
    const altitudeLabels = ticks.map(tick => tick.firstElementChild!.getBoundingClientRect());
    const footer = element.querySelector('[data-testid="holding-graph-footer"]')!;
    const timeAxis = graph.querySelector('[data-testid="holding-time-axis"]')!.getBoundingClientRect();
    return {
      band: y(180) - y(200), normal: y(210) - y(220),
      sharedRows: [...graph.querySelectorAll<HTMLElement>('li[aria-label*="FL190,"]')].map(label => parseFloat(label.style.top)).sort((a, b) => a - b),
      rulersClear: labels.every(label => altitudeLabels.every(tick => label.right < tick.left)),
      horizontalScroll: graph.scrollWidth > graph.clientWidth,
      footerError: Math.max(
        Math.abs(footer.firstElementChild!.getBoundingClientRect().right - 3 - timeAxis.left),
        Math.abs(footer.lastElementChild!.getBoundingClientRect().left - axis.getBoundingClientRect().left)),
    };
  });
  const single = await geometry("figma");
  const double = await geometry("double-level");
  expect(single.rulersClear).toBe(true);
  expect(single.horizontalScroll).toBe(false);
  expect(single.footerError).toBeLessThan(0.1);
  expect(double.footerError).toBeLessThan(0.1);
  expect(single.band).toBeCloseTo(single.normal * 2);
  expect(double.normal).toBeCloseTo(single.normal);
  expect(double.band).toBeCloseTo(single.normal * 3);
  expect(double.sharedRows[1] - double.sharedRows[0]).toBeCloseTo(single.normal);
  await page.evaluate(() => document.fonts.ready);
  await page.getByTestId("figma").screenshot({path: "test-results/holding-figma.png"});
  await page.getByTestId("double-level").screenshot({path: "test-results/holding-double-level.png"});
  await page.getByTestId("holding-review").screenshot({path: "test-results/holding-final-review.png"});
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
