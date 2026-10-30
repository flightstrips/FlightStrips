import {expect, type APIRequestContext, type Locator, type Page} from "@playwright/test";
import {test} from "./fixtures";

const backend = "http://127.0.0.1:4182";
type PersistedStrip = {Callsign: string; Bay: string; Sequence: number; Owner: string | null; Version: number};
const bay = (page: Page, name: string) => page.locator(`[data-bay="${name}"]`);
const strip = (page: Page, callsign: string) => page.locator(`[data-strip="${callsign}"]`);
const order = (page: Page, name: string) => bay(page, name).locator("[data-strip]").evaluateAll(nodes => nodes.map(node => node.getAttribute("data-strip")));

async function persisted(request: APIRequestContext): Promise<PersistedStrip[]> {
  const response = await request.get(`${backend}/e2e/persisted`);
  expect(response.ok()).toBe(true);
  return response.json();
}

async function openTower(page: Page) {
  await page.goto("/app");
  const choose = page.getByRole("button", {name: "TW / TE", exact: true});
  await expect(choose.or(bay(page, "TWY-DEP")).first()).toBeVisible();
  if (await choose.isVisible()) await choose.click();
  await expect(strip(page, "SAS301")).toBeVisible();
}

async function assume(page: Page, callsign: string) {
  await strip(page, callsign).getByText(callsign, {exact: true}).click();
  await page.getByText("FORCE ASSUME", {exact: true}).click();
}

// Native pointer events exercise dnd-kit, collision detection, optimistic state,
// the real command socket and the server's normal ordered-move implementation.
async function drag(page: Page, source: Locator, destination: Locator, fraction = 0.5) {
  // Wait for dnd-kit drop animations and layout transitions to stop intercepting
  // the next gesture. Hover applies Playwright's normal actionability checks.
  await destination.hover();
  await source.hover();
  const from = await source.boundingBox();
  const to = await destination.boundingBox();
  if (!from || !to) throw new Error("drag source or destination has no bounds");
  const x = from.x + from.width * 0.35;
  const y = from.y + from.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + 10, y, {steps: 3});
  await page.mouse.move(to.x + to.width * 0.35, to.y + to.height * fraction, {steps: 20});
  await page.mouse.up();
}

async function assertBay(request: APIRequestContext, pages: Page[], visualBay: string, databaseBay: string, callsigns: string[]) {
  await expect.poll(async () => (await persisted(request)).filter(s => s.Bay === databaseBay).sort((a, b) => b.Sequence - a.Sequence).map(s => s.Callsign)).toEqual(callsigns);
  for (const page of pages) await expect.poll(() => order(page, visualBay)).toEqual(callsigns);
}

async function rejected(page: Page, token: string, command: Record<string, unknown>) {
  return page.evaluate(({token, command, backend}) => new Promise<{action: string; reason: string}>((resolve, reject) => {
    const socket = new WebSocket(`${backend.replace("http", "ws")}/frontEndEvents`);
    const deadline = setTimeout(() => {socket.close(); reject(new Error("no action rejection received"));}, 10_000);
    socket.onopen = () => socket.send(JSON.stringify({type: "token", token, version: "e2e"}));
    socket.onerror = () => {clearTimeout(deadline); reject(new Error("command socket failed"));};
    socket.onmessage = ({data}) => {
      const event = JSON.parse(data);
      if (event.type === "initial") socket.send(JSON.stringify(command));
      if (event.type === "action_rejected") {clearTimeout(deadline); socket.close(); resolve(event);}
    };
  }), {token, command, backend});
}

test("strip moves and reorders persist, respect ownership and reach a second browser", async ({session, request}, testInfo) => {
  const primary = await session("__TEST_TOKEN__");
  const secondary = await session("__TEST_FRONTEND_TOKEN__");
  const pages = [primary, secondary];
  await Promise.all(pages.map(openTower));
  await test.step("independent controllers assume their strips through the UI", async () => {
    for (const callsign of ["SAS301", "SAS302", "SAS303", "SAS501", "SAS502"]) await assume(primary, callsign);
    await assume(secondary, "SAS404");
    await expect.poll(async () => (await persisted(request)).map(s => [s.Callsign, s.Owner]).sort()).toEqual([
      ["SAS301", "118.105"], ["SAS302", "118.105"], ["SAS303", "118.105"],
      ["SAS404", "118.580"], ["SAS501", "118.105"], ["SAS502", "118.105"],
    ]);
  });

  let taxi = (await order(primary, "TWY-DEP")) as string[];
  let runway = (await order(primary, "RWY-DEP")) as string[];
  await assertBay(request, pages, "TWY-DEP", "TAXI_LWR", taxi);
  await assertBay(request, pages, "RWY-DEP", "DEPART", runway);
  await test.step("reorder in a populated bay using the pointer", async () => {
    const owned = taxi.filter(callsign => callsign !== "SAS404");
    const moving = owned[0];
    const target = owned.at(-1)!;
    taxi.splice(taxi.indexOf(moving), 1);
    taxi.splice(taxi.indexOf(target) + 1, 0, moving);
    await drag(primary, strip(primary, moving), strip(primary, target));
    await assertBay(request, pages, "TWY-DEP", "TAXI_LWR", taxi);
  });

  await test.step("move into a populated bay at a specific insertion point", async () => {
    const moving = taxi.find(callsign => callsign !== "SAS404")!;
    // Above the current top strip in a descending bay means the new visual top.
    await drag(primary, strip(primary, moving), strip(primary, runway[0]), 0.2);
    taxi = taxi.filter(callsign => callsign !== moving);
    runway = [moving, ...runway];
    await assertBay(request, pages, "TWY-DEP", "TAXI_LWR", taxi);
    await assertBay(request, pages, "RWY-DEP", "DEPART", runway);
    await expect.poll(async () => (await persisted(request)).find(s => s.Callsign === moving)?.Owner).toBe("118.105");
  });

  await test.step("reorder the destination bay and verify live updates", async () => {
    const moving = runway[0];
    await drag(primary, strip(primary, moving), strip(primary, runway.at(-1)!));
    runway = [...runway.slice(1), moving];
    await assertBay(request, pages, "RWY-DEP", "DEPART", runway);
  });

  await test.step("another controller cannot drag, move or reorder an owned strip", async () => {
    const before = (await persisted(request)).find(s => s.Callsign === "SAS404")!;
    await expect(strip(primary, "SAS404")).toHaveAttribute("aria-disabled", "true");
    await drag(primary, strip(primary, "SAS404"), strip(primary, runway[0]));
    await assertBay(request, pages, "TWY-DEP", "TAXI_LWR", taxi);
    // Bypass only the UI restriction to prove server authorization independently.
    for (const command of [
      {type: "move", callsign: "SAS404", bay: "DEPART", ordered: true, insert_after: null},
      {type: "update_order", callsign: "SAS404", insert_after: null},
    ]) {
      const result = await rejected(primary, "__TEST_TOKEN__", command);
      expect(result.action).toBe(command.type);
      expect(result.reason).toContain("owned by another controller");
      const after = (await persisted(request)).find(s => s.Callsign === "SAS404")!;
      expect({bay: after.Bay, sequence: after.Sequence, owner: after.Owner, version: after.Version}).toEqual({bay: before.Bay, sequence: before.Sequence, owner: before.Owner, version: before.Version});
    }
    await assertBay(request, pages, "RWY-DEP", "DEPART", runway);
  });

  await test.step("the owner can move its strip and the first browser receives it", async () => {
    await drag(secondary, strip(secondary, "SAS404"), strip(secondary, runway[0]), 0.2);
    taxi = taxi.filter(callsign => callsign !== "SAS404");
    runway = ["SAS404", ...runway];
    await assertBay(request, pages, "TWY-DEP", "TAXI_LWR", taxi);
    await assertBay(request, pages, "RWY-DEP", "DEPART", runway);
    await expect.poll(async () => (await persisted(request)).find(s => s.Callsign === "SAS404")?.Owner).toBe("118.580");
  });

  await test.step("fresh browser connections restore persisted bays, order and owners", async () => {
    const before = (await persisted(request)).map(s => ({callsign: s.Callsign, bay: s.Bay, sequence: s.Sequence, owner: s.Owner})).sort((a,b) => a.callsign.localeCompare(b.callsign));
    // Reload discards optimistic client state and opens new application sockets.
    await Promise.all(pages.map(openTower));
    await assertBay(request, pages, "TWY-DEP", "TAXI_LWR", taxi);
    await assertBay(request, pages, "RWY-DEP", "DEPART", runway);
    const after = (await persisted(request)).map(s => ({callsign: s.Callsign, bay: s.Bay, sequence: s.Sequence, owner: s.Owner})).sort((a,b) => a.callsign.localeCompare(b.callsign));
    expect(after).toEqual(before);
    await expect(strip(primary, "SAS404")).toHaveAttribute("aria-disabled", "true");
    await expect(strip(secondary, "SAS404")).toHaveAttribute("aria-disabled", "false");
    await testInfo.attach("persisted-strips", {body: JSON.stringify(await persisted(request), null, 2), contentType: "application/json"});
  });
});
