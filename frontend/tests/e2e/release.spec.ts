import {expect, type APIRequestContext, type Page} from "@playwright/test";
import {test} from "./fixtures";

const backend = "http://127.0.0.1:4184";
type Row = {ID: number; Callsign: string; Bay: string; Sequence: number; Owner: string | null; BoardState?: {annotations: {notes: string}}};
async function rows(request: APIRequestContext, airport: string): Promise<Row[]> {
  const response = await request.get(`${backend}/e2e/persisted?airport=${airport}`);
  expect(response.ok()).toBe(true);
  return response.json();
}
async function choose(page: Page, airport: string) {
  await page.goto("/app");
  await page.getByRole("button", {name: new RegExp(`${airport} · LIVE`)}).click();
  if (airport === "EKCH") {
    const tower = page.getByRole("button", {name: "TW / TE", exact: true});
    await expect(tower.or(page.locator('[data-bay="TWY-DEP"]')).first()).toBeVisible();
    if (await tower.isVisible()) await tower.click();
    await expect(page.locator('[data-strip="SAS301"]')).toBeVisible();
  } else await expect(page.getByTestId("radis-strip-RADIS2")).toBeVisible();
}

test("EKCH moves and own-plugin reconnect preserve a concurrent RADIS board", async ({session, request}) => {
  const tower = await session("__TEST_TOKEN__"), peer = await session("__TEST_FRONTEND_TOKEN__"), radis = await session("__TEST_FRONTEND_TOKEN__");
  await Promise.all([choose(tower,"EKCH"),choose(peer,"EKCH"),choose(radis,"EKBI")]);
  const radisBefore = (await rows(request,"EKBI")).find(row=>row.Callsign==="RADIS2")!;
  const ekchStrip = (page: Page, callsign: string) => page.locator(`[data-strip="${callsign}"]`);
  for (const [page,callsign] of [[tower,"SAS301"],[peer,"SAS404"]] as const) {
    await ekchStrip(page,callsign).getByText(callsign,{exact:true}).click();
    await page.getByText("FORCE ASSUME",{exact:true}).click();
    await expect(ekchStrip(page,callsign)).toHaveAttribute("aria-disabled","false");
  }
  const editNote = async (value: string) => {
    await radis.getByRole("button",{name:"Notes RADIS2",exact:true}).click();
    await radis.getByRole("textbox",{name:"RADIS notes",exact:true}).fill(value);
    await radis.getByRole("button",{name:"OK",exact:true}).click();
    await expect(radis.getByRole("button",{name:"Notes RADIS2",exact:true})).toHaveText(value);
  };
  const move = async () => {
    const target = tower.locator('[data-bay="RWY-DEP"] [data-strip]').first();
    await target.hover();
    const source = ekchStrip(tower,"SAS301");
    await source.hover();
    const from = await source.boundingBox(), to = await target.boundingBox();
    if (!from || !to) throw new Error("missing EKCH drag bounds");
    await tower.mouse.move(from.x+from.width*.35,from.y+from.height/2);
    await tower.mouse.down();
    await tower.mouse.move(from.x+from.width*.35+10,from.y+from.height/2,{steps:3});
    await tower.mouse.move(to.x+to.width*.35,to.y+to.height*.2,{steps:20});
    await tower.mouse.up();
  };
  await Promise.all([move(),editNote("RADIS WHILE EKCH MOVES")]);
  await expect.poll(async ()=>(await rows(request,"EKCH")).find(row=>row.Callsign==="SAS301")?.Bay).toBe("DEPART");
  await expect(peer.locator('[data-bay="RWY-DEP"] [data-strip="SAS301"]')).toBeVisible();
  const radisAfter = (await rows(request,"EKBI")).find(row=>row.Callsign==="RADIS2")!;
  expect({id:radisAfter.ID,bay:radisAfter.Bay,sequence:radisAfter.Sequence,owner:radisAfter.Owner}).toEqual({id:radisBefore.ID,bay:radisBefore.Bay,sequence:radisBefore.Sequence,owner:radisBefore.Owner});
  expect(radisAfter.BoardState?.annotations.notes).toBe("RADIS WHILE EKCH MOVES");
  // Losing this controller's EKCH plugin cannot disable its independent EKBI plugin.
  expect((await request.post(`${backend}/e2e/own-plugin/disconnect?airport=EKCH`)).ok()).toBe(true);
  await expect(ekchStrip(peer,"SAS404")).toHaveAttribute("aria-disabled","true");
  await expect(ekchStrip(tower,"SAS301")).toHaveAttribute("aria-disabled","false");
  await editNote("RADIS WHILE EKCH DISCONNECTED");
  expect((await request.post(`${backend}/e2e/own-plugin/reconnect?airport=EKCH`)).ok()).toBe(true);
  await expect(ekchStrip(peer,"SAS404")).toHaveAttribute("aria-disabled","false");
  await peer.reload();
  await peer.getByRole("button",{name:"TW / TE",exact:true}).click();
  await expect(peer.locator('[data-bay="RWY-DEP"] [data-strip="SAS301"]')).toBeVisible();
  await expect(ekchStrip(peer,"SAS404")).toHaveAttribute("aria-disabled","false");
  await editNote(radisBefore.BoardState?.annotations.notes ?? "");
});
