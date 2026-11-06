import {expect, test, type Page, type WebSocketRoute} from "@playwright/test";

function membership(session: number, airport: string, instance = "own") {
  return {session_id: session, airport, environment: "LIVE", instance_id: instance, membership_id: `member-${session}-${instance}`, transport_generation: "18446744073709551615", generation: "18446744073709551614", master_epoch: "1", view: "ekch", definition_revision: "fixture", actions: ["strip.move"], capabilities: {cdm: false, stands: false, text_pilot: false, pdc: false, aman: false}, role: "slave", ready: true, read_only: false, callsign: `${airport}_DEL`, position: "121.930"};
}
async function setup(page: Page, members = [membership(1, "AAAA"), membership(2, "BBBB")]) {
  const authentications: Record<string, unknown>[] = [];
  const sockets: WebSocketRoute[] = [];
  await page.route("**/api/plugin-memberships", route => route.fulfill({json: {transports: members.map(m => ({instance_id: m.instance_id, transport_generation: m.transport_generation, memberships: [m]}))}}));
  await page.routeWebSocket("**/frontEndEvents", ws => {
    sockets.push(ws);
    ws.onMessage(raw => {
      const event = JSON.parse(String(raw));
      if (event.type !== "token") return;
      authentications.push(event);
      const member = members.find(m => m.session_id === event.board.session_id && m.instance_id === event.board.instance_id)!;
      ws.send(JSON.stringify({type: "initial", board: event.board, definition: {definition_revision: member.definition_revision, view: member.view, actions: member.actions, capabilities: member.capabilities}, session_id: member.session_id, airport: member.airport, callsign: member.callsign, layout: "CLX", reset_layout: "CLX", read_only: member.read_only, own_plugin_ready: member.ready, observer: member.role === "observer", position_available: true, controllers: [], strips: [], tactical_strips: [], me: {callsign: member.callsign, position: member.position, identifier: "D", section: "DEL", owned_sectors: []}, runway_setup: {departure: ["22R"], arrival: ["22L"], runway_status: {}}, coordinations: [], messages: [], available_sids: [], initial_cfl_by_runway: {}, transition_altitude: 5000, stand_assignment_enabled: false, stand_assignments: [], stand_blocks: []}));
    });
  });
  return Object.assign(authentications, {sockets});
}

test("separate tabs select and switch independent boards without membership commands", async ({context}, info) => {
  const a = await context.newPage(), b = await context.newPage();
  const authA = await setup(a), authB = await setup(b);
  await Promise.all([a.goto("/visual-tests/airport-session-preview.html"), b.goto("/visual-tests/airport-session-preview.html")]);
  await expect(a.getByRole("heading", {name: "Choose airport session"})).toBeVisible();
  await a.screenshot({path: info.outputPath("airport-chooser.png")});
  await a.getByRole("button", {name: /AAAA/}).click();
  await b.getByRole("button", {name: /BBBB/}).click();
  await expect(a.getByTestId("selected-board")).toHaveAttribute("data-session", "1");
  await expect(b.getByTestId("selected-board")).toHaveAttribute("data-session", "2");
  await a.getByText("CLR DEL", {exact: true}).first().click();
  await a.getByRole("button", {name: "Switch airport session"}).click();
  await a.getByRole("button", {name: /BBBB/}).click();
  await expect(a.getByTestId("selected-board")).toHaveAttribute("data-session", "2");
  await expect(b.getByTestId("selected-board")).toHaveAttribute("data-session", "2");
  expect(authA.map(x => (x.board as {session_id: number}).session_id)).toEqual([1, 2]);
  expect(authB.map(x => (x.board as {session_id: number}).session_id)).toEqual([2]);
  await a.screenshot({path: info.outputPath("selected-board.png")});
});

test("zero sessions wait and one own session enters without a chooser", async ({page}) => {
  await setup(page, []);
  await page.goto("/visual-tests/airport-session-preview.html");
  await expect(page.getByRole("heading", {name: "Waiting for ES connection"})).toBeVisible();
  await setup(page, [membership(1, "EKCH")]);
  await page.reload();
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-session", "1");
  await expect(page.getByRole("heading", {name: "Choose airport session"})).not.toBeVisible();
});

test("multiple compatible own instances require a deliberate choice", async ({page}) => {
  const auth = await setup(page, [membership(1, "AAAA", "first"), membership(1, "AAAA", "second")]);
  await page.goto("/visual-tests/airport-session-preview.html");
  await expect(page.getByRole("heading", {name: "Choose your plugin instance"})).toBeVisible();
  expect(auth).toHaveLength(0);
  await page.getByRole("button", {name: /second/}).click();
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-session", "1");
  expect((auth[0].board as {instance_id: string}).instance_id).toBe("second");
});


test("late old snapshots, disconnects and action results cannot affect a switched board", async ({page}) => {
  const auth = await setup(page);
  await page.goto("/visual-tests/airport-session-preview.html");
  await page.getByRole("button", {name:/AAAA/}).click();
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-session","1");
  const old = auth[0].board;
  await page.getByText("CLR DEL", {exact:true}).first().click();
  await page.getByRole("button", {name:"Switch airport session"}).click();
  await page.getByRole("button", {name:/BBBB/}).click();
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-session","2");
  const current = auth.sockets.at(-1)!;
  for (const event of [
    {type:"initial",session_id:1,airport:"AAAA",layout:"TWTE"},
    {type:"disconnect",read_only:true},
    {type:"action_rejected",action:"move",reason:"OLD RESULT"},
    {type:"frequency_message_result",request_id:"old",callsign:"OLD123",submitted:true,applied_actions:1},
    {type:"strip_update",callsign:"OLD123"},
  ]) current.send(JSON.stringify({...event,board:old}));
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-session","2");
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-read-only","false");
  await expect(page.getByText("OLD123")).not.toBeVisible();
  await expect(page.getByText("OLD RESULT")).not.toBeVisible();
  expect(auth).toHaveLength(2);
});

test("own-plugin loss keeps the selected airport and reconnects only its own physical instance", async ({page},info) => {
  const own = membership(1,"AAAA");
  const members = [own,membership(2,"BBBB")];
  const auth = await setup(page,members);
  await page.goto("/visual-tests/airport-session-preview.html");
  await page.getByRole("button", {name:/AAAA/}).click();
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-session","1");
  auth.sockets[0].send(JSON.stringify({type:"disconnect",board:auth[0].board,read_only:true}));
  members.shift();
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-read-only","true");
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-session","1");
  await expect(page.getByRole("status").filter({hasText:"Own plugin unavailable"})).toBeVisible();
  await page.screenshot({path:info.outputPath("retained-read-only-board.png")});
  members.push({...own,membership_id:"rejoined",generation:"42",transport_generation:"43"});
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-read-only","false",{timeout:10000});
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-session","1");
  expect(auth).toHaveLength(2);
  expect((auth[1].board as {membership_id:string}).membership_id).toBe("rejoined");
});

test("an observer enters its selected session read-only", async ({page}) => {
  const observer = {...membership(1,"EKCH"),role:"observer",read_only:true};
  await setup(page,[observer]);
  await page.goto("/visual-tests/airport-session-preview.html");
  await expect(page.getByTestId("selected-board")).toHaveAttribute("data-read-only","true");
  await expect(page.getByRole("status").filter({hasText:"Observer"})).toBeVisible();
});
