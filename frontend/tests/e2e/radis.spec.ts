import {expect, type APIRequestContext, type Page} from "@playwright/test";
import {test} from "./fixtures";
import {identity} from "./identity";

const backend = "http://127.0.0.1:4184";
type Row = {ID: number; Version: number; Callsign: string; Bay: string; Sequence: number; FlightPlanEET: string; CdmData: {tobt?: string}; BoardState: {local?: boolean; annotations: {notes: string}; removed_at?: string}};
const strip = (page: Page, callsign = "RADIS1") => page.getByTestId(`radis-strip-${callsign}`);
async function rows(request: APIRequestContext, airport = "EKBI", environment = "LIVE"): Promise<Row[]> {
  const response = await request.get(`${backend}/e2e/persisted?airport=${airport}&environment=${environment}`);
  expect(response.ok()).toBe(true); return response.json();
}
async function open(page: Page, airport: string, environment = "LIVE") {
  await page.goto("/app");
  const choice = page.getByRole("button", {name: new RegExp(`${airport} · ${environment}`)});
  await expect(choice.or(page.getByRole("main", {name: "RADIS board"}))).toBeVisible();
  if (await choice.isVisible()) await choice.click();
  await expect(strip(page)).toBeVisible();
}
async function note(page: Page, value: string) {
  await page.getByRole("button", {name: "Notes RADIS1", exact: true}).click();
  await page.getByRole("textbox", {name: "RADIS notes", exact: true}).fill(value);
  await page.getByRole("button", {name: "OK", exact: true}).click();
}
async function fpl(page: Page) {await page.getByRole("button", {name: "Flight plan RADIS1", exact: true}).click();}

test("RADIS production sessions converge through database, own-plugin commands and airport services", async ({session, request}, info) => {
  test.setTimeout(210_000);
  const a = await session("__TEST_TOKEN__"), b = await session("__TEST_FRONTEND_TOKEN__"), other = await session("__TEST_TOKEN__");
  await Promise.all([open(a, "EKBI"), open(b, "EKBI"), open(other, "EKYT")]);
  const original = (await rows(request)).find(s => s.Callsign === "RADIS1")!;
  expect(original.FlightPlanEET).toBe("0130");
  await expect(strip(a).getByText("0130", {exact: true})).toBeVisible();
  await test.step("shared annotations and actual own-plugin altitude echo stay airport-scoped", async () => {
    await note(b, "PRODUCTION SHARED NOTE");
    await expect(a.getByRole("button", {name: "Notes RADIS1", exact: true})).toHaveText("PRODUCTION SHARED NOTE");
    await expect(other.getByRole("button", {name: "Notes RADIS1", exact: true})).toHaveText("");
    await b.getByRole("button", {name: "ALT FT RADIS1", exact: true}).click();
    await b.getByRole("textbox", {name: "Cleared altitude (feet)", exact: true}).fill("3400");
    await b.getByRole("button", {name: "OK", exact: true}).click();
    await expect(a.getByRole("button", {name: "ALT FT RADIS1", exact: true})).toContainText("3400");
    await expect(other.getByRole("button", {name: "ALT FT RADIS1", exact: true})).toContainText("3000");
    await expect.poll(async () => (await (await request.get(`${backend}/e2e/commands`)).json()).filter((c: {kind: string; token: string; airport: string}) => c.kind === "EVENT_CLEARED_ALTITUDE")).toEqual([expect.objectContaining({airport: "EKBI", token: "__TEST_FRONTEND_TOKEN__", callsign: "RADIS1"})]);
  });
  await test.step("same-profile tabs and LIVE, exercise and playback boards retain independent identities", async () => {
    const tab = await a.context().newPage(), sweat = await a.context().newPage(), playback = await a.context().newPage();
    for (const page of [tab, sweat, playback]) await identity(page, backend);
    await Promise.all([open(tab,"EKYT"), open(sweat,"EKBI","SWEATBOX_MA08"), open(playback,"EKBI","PLAYBACK_MA08")]);
    const contexts = await Promise.all([rows(request),rows(request,"EKYT"),rows(request,"EKBI","SWEATBOX_MA08"),rows(request,"EKBI","PLAYBACK_MA08")]);
    expect(new Set(contexts.map(list=>list.find(row=>row.Callsign==="RADIS1")!.ID)).size).toBe(4);
    await note(sweat,"EXERCISE ONLY"); await note(playback,"PLAYBACK ONLY"); await note(tab,"SECOND TAB EKYT");
    await expect(a.getByRole("button",{name:"Notes RADIS1",exact:true})).toHaveText("PRODUCTION SHARED NOTE");
    await expect(other.getByRole("button",{name:"Notes RADIS1",exact:true})).toHaveText("SECOND TAB EKYT");
    // Switch this tab through the real chooser, without changing its peers.
    await tab.getByRole("button",{name:"Switch airport session"}).click();
    await tab.getByRole("button",{name:/EKBI · LIVE/}).click();
    await expect(tab.getByRole("button",{name:"Notes RADIS1",exact:true})).toHaveText("PRODUCTION SHARED NOTE");
    await note(tab,"SECOND TAB EKBI");
    await expect(b.getByRole("button",{name:"Notes RADIS1",exact:true})).toHaveText("SECOND TAB EKBI");
    await expect(other.getByRole("button",{name:"Notes RADIS1",exact:true})).toHaveText("SECOND TAB EKYT");
    for (const [page,value] of [[sweat,"EXERCISE ONLY"],[playback,"PLAYBACK ONLY"],[tab,"SECOND TAB EKBI"]] as const) {
      await page.reload(); await expect(page.getByRole("button",{name:"Notes RADIS1",exact:true})).toHaveText(value);
    }
    expect((await rows(request,"EKBI","SWEATBOX_MA08")).find(row=>row.Callsign==="RADIS1")!.BoardState.annotations.notes).toBe("EXERCISE ONLY");
    expect((await rows(request,"EKBI","PLAYBACK_MA08")).find(row=>row.Callsign==="RADIS1")!.BoardState.annotations.notes).toBe("PLAYBACK ONLY");
    await note(tab,"PRODUCTION SHARED NOTE");
    await expect(b.getByRole("button",{name:"Notes RADIS1",exact:true})).toHaveText("PRODUCTION SHARED NOTE");
    await Promise.all([tab.close(),sweat.close(),playback.close()]);
  });
  await test.step("real stand resources are ready while deferred regional services remain disabled", async () => {
    await fpl(b);
    await expect(b.getByRole("textbox", {name: "CDM TOBT"})).toBeHidden();
    await expect(b.getByRole("button", {name: "TEXT PILOT", exact: true})).toBeHidden();
    await b.getByRole("button", {name: "OK", exact: true}).click();
    await strip(b).getByRole("button", {name: /Stand/}).click();
    await expect(b.getByRole("dialog", {name: "Stand \u00b7 RADIS1"})).toBeVisible();
    await expect(b.getByRole("button", {name: "AUTO", exact: true})).toBeEnabled();
    await b.getByRole("textbox", {name: "Airport stand"}).fill("35");
    await b.getByRole("button", {name: "ASSIGN STAND"}).click();
    await expect(strip(a).locator(".radis-stand")).toContainText("35");
    await b.getByRole("button", {name: "DISMISS", exact: true}).click();
    await expect(strip(other).locator(".radis-stand")).not.toContainText("35");
    await strip(other).getByRole("button", {name: /Stand/}).click();
    await other.getByRole("button", {name: "AUTO", exact: true}).click();
    await expect(strip(other).locator(".radis-stand strong")).toHaveText(/^[4-8]$/);
    await other.getByRole("button", {name: "DISMISS", exact: true}).click();
  });
  await test.step("NEW and shared FIND use production persistence and the shared connected source", async () => {
    await b.getByRole("button", {name: "NEW", exact: true, includeHidden:true}).click();
    await b.getByRole("textbox", {name: "Callsign", exact: true}).fill("LOCAL7");
    await b.getByRole("button", {name: "CREATE LOCAL STRIP"}).click();
    for (const page of [a,b]) await expect(strip(page,"LOCAL7")).toBeVisible();
    await expect(strip(other,"LOCAL7")).toBeHidden();
    expect((await rows(request)).find(s => s.Callsign === "LOCAL7")?.BoardState.local).toBe(true);
    await b.getByRole("button", {name: "FIND", exact: true}).click();
    await expect(b.getByRole("status").filter({hasText: "26 connected flights"})).toBeVisible();
    await b.getByRole("button", {name: /FIND23/}).click();
    await expect(strip(a,"FIND23")).toBeVisible();
    await expect(a.getByRole("button", {name:"ALT FT FIND23",exact:true})).toBeDisabled();
    expect((await rows(request)).find(s => s.Callsign === "FIND23")?.FlightPlanEET).toBe("0130");
  });
  await test.step("pointer moves and reordering persist without ownership", async () => {
    const drag = async (callsign: string, target: ReturnType<Page["locator"]>) => {
      await target.hover();
      const from = await b.getByRole("button", {name:`Move ${callsign}`,exact:true}).boundingBox(), to = await target.boundingBox();
      if (!from || !to) throw new Error("missing drag bounds");
      await b.mouse.move(from.x+from.width/2, from.y+from.height/2);
      await b.mouse.down(); await b.mouse.move(from.x+from.width/2+10,from.y+from.height/2,{steps:3});
      await b.mouse.move(to.x+to.width/2,to.y+to.height/2,{steps:20}); await b.mouse.up();
    };
    for (const callsign of ["RADIS1","RADIS2"]) {
      await drag(callsign,b.locator(".radis-runway"));
      await expect.poll(async () => (await rows(request)).find(s => s.Callsign === callsign)?.Bay).toBe("RUNWAY");
      for (const page of [a,b]) await expect(page.locator(".radis-runway").getByTestId(`radis-strip-${callsign}`)).toBeVisible();
    }
    const runwayOrder = async () => (await rows(request)).filter(s => s.Bay === "RUNWAY").sort((a,b)=>a.Sequence-b.Sequence).map(s=>s.Callsign);
    const before = await runwayOrder();
    expect(before).toHaveLength(2);
    // Dropping into RUNWAY can insert before an existing strip. Always move
    // the current last strip ahead of the current first to exercise reordering.
    await drag(before[1],strip(b,before[0]));
    await expect.poll(runwayOrder).toEqual([...before].reverse());
    await expect(other.locator(".radis-runway").getByTestId("radis-strip-RADIS1")).toBeHidden();
  });
  await test.step("manual removal survives refresh and restore retains identity", async () => {
    // The DB reorder commits before its per-strip websocket publications. Wait
    // for this controller to observe that revision before issuing another write.
    await expect.poll(async () => {
      const current=(await rows(request)).find(s=>s.Callsign==="RADIS1")!;
      return await strip(b).getAttribute("data-strip-version")===String(current.Version);
    }).toBe(true);
    const placement = (await rows(request)).find(s => s.Callsign === "RADIS1")!;
    if (!((await strip(b).getAttribute("class")) ?? "").includes("is-selected")) {
      await strip(b).getByText("EKBI", {exact: true}).click();
    }
    await expect(strip(b)).toHaveClass(/is-selected/);
    await b.getByRole("button", {name: "Remove selected strip"}).click();
    for (const page of [a,b]) await expect(strip(page)).toBeHidden();
    await b.reload(); await expect(b.getByRole("main", {name: "RADIS board"})).toBeVisible();
    await expect(strip(b)).toBeHidden();
    await expect(strip(other)).toBeVisible();
    await b.getByRole("button", {name: "RESTORE", exact: true}).click();
    await b.getByRole("button", {name: "Restore RADIS1", exact: true}).click();
    await expect(strip(a)).toBeVisible();
    const restored = (await rows(request)).find(s => s.Callsign === "RADIS1")!;
    expect(restored.ID).toBe(original.ID); expect(restored.Bay).toBe(placement.Bay); expect(restored.Sequence).toBe(placement.Sequence);
    expect(restored.BoardState.annotations.notes).toBe("PRODUCTION SHARED NOTE");
  });
  await test.step("own disconnect blocks every edit while peer updates and independent airport remain live", async () => {
    await fpl(b);
    expect((await request.post(`${backend}/e2e/own-plugin/disconnect`)).ok()).toBe(true);
    for (const name of ["NEW","FIND","RESTORE"]) await expect(b.getByRole("button", {name, exact: true, includeHidden:true})).toBeDisabled();
    await expect(b.getByRole("button", {name: "Notes RADIS1", exact: true, includeHidden:true})).toBeDisabled();
    await expect(b.getByRole("button", {name: "TEXT PILOT", exact: true})).toBeHidden();
    await note(a,"PEER WHILE DISCONNECTED");
    await expect(b.getByRole("button", {name: "Notes RADIS1", exact: true, includeHidden:true})).toHaveText("PEER WHILE DISCONNECTED");
    await expect(other.getByRole("button", {name: "NEW", exact: true})).toBeEnabled();
    expect((await request.post(`${backend}/e2e/own-plugin/reconnect`)).ok()).toBe(true);
    await expect(b.getByRole("button", {name: "NEW", exact: true, includeHidden:true})).toBeEnabled();
    await expect(b.getByRole("dialog",{name:"Flight plan · RADIS1"})).toBeHidden();
    await fpl(b);
    await b.getByRole("button",{name:"OK",exact:true}).click();
    await expect(b.getByRole("button", {name: "Notes RADIS1", exact: true, includeHidden:true})).toHaveText("PEER WHILE DISCONNECTED");
  });
  await test.step("backend weather and master runway observations reach only their airport", async () => {
    await expect(a.locator(".radis-commandbar")).toContainText("1001", {timeout:135_000});
    await expect(other.locator(".radis-commandbar")).toContainText("1002");
    expect((await request.post(`${backend}/e2e/runways`)).ok()).toBe(true);
    for (const page of [a,b]) await expect(page.getByRole("button", {name: "Departure runways"})).toHaveText("DEP 27");
    await expect(other.getByRole("button", {name: "Departure runways"})).toHaveText("DEP 08R");
  });
  await a.screenshot({path:info.outputPath("radis-production-board.png")});
});
