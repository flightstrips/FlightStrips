import {expect, test, type BrowserContext, type WebSocketRoute} from "@playwright/test";

function strip(airport: string) {
  return {id: airport === "EKBI" ? 11 : 21, version: 1, callsign: "SAME123", origin: "EKBI", destination: "EKYT", alternate: "", route: "DCT SYNTH", remarks: "", runway: "09", squawk: "2401", assigned_squawk: "2401", sid: "", star: "", cleared_altitude: 2300, requested_altitude: 7000, heading: 0, aircraft_type: "C172", aircraft_category: "L", stand: "", capabilities: "", communication_type: "V", eobt: "1200", tobt: "1200", tsat: "1210", ctot: "", eldt: "1230", bay: airport === "EKBI" ? "PASSIVE_TOP" : "PASSIVE_BOTTOM", sequence: 1000, version_label: "synthetic", next_controllers: [], previous_controllers: [], owner: "", pdc_state: "NONE", start_req: false, marked: false, runway_cleared: false, runway_confirmed: false, registration: "SYNTH", has_fp: true, board_state: {local: false, removed_at: null, removal_reason: "", annotations: {notes: "", deicing: false, rea: null, ata: null, atd: "", endurance_pob: "", qnh: "", rnav: "", vfr: false}}};
}

async function setup(context: BrowserContext) {
  const boards = new Map([[1, strip("EKBI")], [2, strip("EKYT")]]);
  const actions: Record<string, unknown>[] = [];
  const sockets: Array<{ws: WebSocketRoute; board: Record<string, unknown>}> = [];
  const capabilities = {cdm: true, stands: true, text_pilot: true, pdc: false, aman: false};
  const members = ["EKBI", "EKYT"].map((airport, i) => ({session_id: i + 1, airport, environment: "LIVE", instance_id: "own", membership_id: `m-${i}`, transport_generation: "1", generation: "1", master_epoch: "1", view: "radis", definition_revision: `synthetic-${airport}`, actions: ["strip.move", "strip.annotate","strip.create-local","flight.find-connected","strip.admit-connected","strip.remove","strip.restore"], capabilities, role: "slave", ready: true, read_only: false, callsign: `${airport}_TWR`, position: "118.000"}));
  await context.route("**/api/plugin-memberships", route => route.fulfill({json: {transports: [{instance_id: "own", transport_generation: "1", memberships: members}]}}));
  await context.route("**/*vatsim*", () => {throw new Error("RADIS must consume shared backend sources");});
  const emit = (session: number) => {
    for (const s of sockets) if (s.board.session_id === session) s.ws.send(JSON.stringify({type: "strip_update", ...boards.get(session), board: s.board}));
  };
  await context.routeWebSocket("**/frontEndEvents", ws => {
    let board: Record<string, unknown>;
    ws.onMessage(raw => {
      const event = JSON.parse(String(raw));
      if (event.type === "token") {
        board = event.board;
        sockets.push({ws, board});
        const member = members.find(m => m.session_id === board.session_id)!;
        ws.send(JSON.stringify({type: "initial", board, definition: {view: "radis", definition_revision: member.definition_revision, actions: member.actions, capabilities, runways: ["09", "27"], board_state: {revisioned:true,recovery_seconds: 123}}, session_id: member.session_id, airport: member.airport, callsign: member.callsign, layout: "RADIS", reset_layout: "RADIS", read_only: false, own_plugin_ready: true, observer: false, position_available: true, controllers: [], strips: [boards.get(member.session_id)], tactical_strips: [], me: {callsign: member.callsign, position: member.position, identifier: "T", section: "TWR", owned_sectors: []}, runway_setup: {departure: ["09"], arrival: ["27"], runway_status: {}}, coordinations: [], messages: [], available_sids: [], initial_cfl_by_runway: {}, transition_altitude: 5000, stand_assignment_enabled: true, stand_assignments: [], stand_blocks: []}));
        return;
      }
      actions.push(event);
      if(event.type === "flight.find-connected") {ws.send(JSON.stringify({type:"flight.connected-results",board,request_id:event.request_id,flights:Array.from({length:24},(_,i)=>({callsign:`FIND${String(i).padStart(2,"0")}`,cid:String(i+100),logon_time:"2026-01-01T00:00:00Z",source_revision:"1",origin:"EKBI",destination:"EKYT",aircraft_type:"C172"}))}));return;}
      if(event.type === "strip.create-local" || event.type === "strip.admit-connected") {
       const added={...strip(String(board.airport)),id:event.type === "strip.create-local"?33:34,version:1,callsign:event.callsign ?? event.flight.callsign,origin:event.origin ?? event.flight.origin,destination:event.destination ?? event.flight.destination,has_fp:false,board_state:{...strip(String(board.airport)).board_state,local:event.type === "strip.create-local"}};
       for(const s of sockets) if(s.board.session_id===board.session_id)s.ws.send(JSON.stringify({type:"strip_update",...added,board:s.board}));return;
      }

      const current = boards.get(Number(board.session_id))!;
      if (event.type === "strip.annotate") {
        Object.assign(current.board_state.annotations, {[event.field]: ["deicing", "vfr"].includes(event.field) ? event.enabled : event.value});
      } else if (event.type === "update_strip_data") {
        if (event.altitude !== undefined) current.cleared_altitude = event.altitude;
        if (event.assigned_squawk !== undefined) current.assigned_squawk = event.assigned_squawk;
      } else if (event.type === "move") current.bay = event.bay;
      current.version++;
      emit(Number(board.session_id));
    });
  });
  return {actions, sockets, boards, emit};
}

test("RADIS host views share annotations and field results while retaining airport isolation", async ({context}, info) => {
  const server = await setup(context);
  const a = await context.newPage(), b = await context.newPage(), other = await context.newPage();
  await Promise.all([a, b, other].map(p => p.goto("/visual-tests/airport-session-preview.html")));
  await a.getByRole("button", {name: /EKBI/}).click();
  await b.getByRole("button", {name: /EKBI/}).click();
  await other.getByRole("button", {name: /EKYT/}).click();
  await expect(a.getByRole("main", {name: "RADIS board"})).toBeVisible();
  await a.getByRole("button", {name: "Notes SAME123", exact: true}).click();
  await a.getByRole("textbox", {name: "RADIS notes", exact: true}).fill("CHECK SHARED NOTE");
  await a.getByRole("button", {name: "OK", exact: true}).click();
  await expect(b.getByRole("button", {name: "Notes SAME123", exact: true})).toHaveText("CHECK SHARED NOTE");
  await expect(other.getByRole("button", {name: "Notes SAME123", exact: true})).toHaveText("");
  await a.getByRole("button", {name: "ALT FT SAME123", exact: true}).click();
  await a.getByRole("textbox", {name: "Cleared altitude (feet)", exact: true}).fill("3400");
  await a.getByRole("button", {name: "OK", exact: true}).click();
  await expect(b.getByRole("button", {name: "ALT FT SAME123", exact: true})).toContainText("3400");
  expect(server.actions).toEqual(expect.arrayContaining([expect.objectContaining({type: "update_strip_data", altitude: 3400, revision: {strip_id: 11, version: 2, definition_revision: "synthetic-EKBI"}, board: expect.objectContaining({session_id: 1})})]));
  await a.screenshot({path: info.outputPath("radis-shared-board.png")});
  await a.getByRole("button", {name: "Flight plan SAME123", exact: true}).click();
  await expect(a.getByRole("dialog", {name: "Flight plan · SAME123"})).toBeVisible();
  await a.getByText("DE-ICE", {exact: true}).click();
  await expect(b.getByRole("button", {name: "Notes SAME123", exact: true})).toContainText("DE-ICE");
  await a.screenshot({path: info.outputPath("radis-flight-plan.png")});
});

test("own-plugin disconnect leaves the retained RADIS board read-only while peer updates continue", async ({context}, info) => {
  const server = await setup(context);
  const page = await context.newPage();
  await page.goto("/visual-tests/airport-session-preview.html");
  await page.getByRole("button", {name: /EKBI/}).click();
  const own = server.sockets[0];
  own.ws.send(JSON.stringify({type: "disconnect", board: own.board, read_only: true}));
  await expect(page.getByRole("button", {name: "Notes SAME123", exact: true})).toBeDisabled();
  await expect(page.getByRole("button", {name: "ALT FT SAME123", exact: true})).toBeDisabled();
  await expect(page.getByRole("button", {name: "Move SAME123", exact: true})).toHaveAttribute("aria-disabled", "true");
  server.boards.get(1)!.board_state.annotations.notes = "PEER UPDATE";
  server.boards.get(1)!.version++;
  server.emit(1);
  await expect(page.getByRole("button", {name: "Notes SAME123", exact: true})).toHaveText("PEER UPDATE");
  await expect(page.getByRole("button",{name:"NEW",exact:true})).toBeDisabled();
  await expect(page.getByRole("button",{name:"FIND",exact:true})).toBeDisabled();
  expect(server.actions).toHaveLength(0);
  await page.screenshot({path: info.outputPath("radis-read-only.png")});
});

test("NEW shares a local strip and FIND presents uncapped connected airport candidates",async({context})=>{
 const server=await setup(context);const page=await context.newPage();await page.goto("/visual-tests/airport-session-preview.html");await page.getByRole("button",{name:/EKBI/}).click();
 await page.getByRole("button",{name:"NEW",exact:true}).click();await page.getByRole("textbox",{name:"Callsign",exact:true}).fill("LOCAL77");await page.getByRole("button",{name:"CREATE LOCAL STRIP",exact:true}).click();
 await expect(page.getByTestId("radis-strip-LOCAL77")).toBeVisible();await expect(page.getByRole("button",{name:"Squawk LOCAL77",exact:true})).toBeDisabled();
 await page.getByRole("button",{name:"FIND",exact:true}).click();await expect(page.getByText("24 connected flights")).toBeVisible();await expect(page.getByRole("button",{name:/FIND23/})).toBeVisible();await page.getByRole("button",{name:/FIND23/}).click();await expect(page.getByTestId("radis-strip-FIND23")).toBeVisible();
 expect(server.actions.map(a=>a.type)).toEqual(["strip.create-local","flight.find-connected","strip.admit-connected"]);
 expect(server.actions[0]).toMatchObject({definition_revision:"synthetic-EKBI",callsign:"LOCAL77",board:expect.objectContaining({session_id:1})});
});

test("a dialog keeps its observed strip revision when another participant edits",async({context})=>{
 const server=await setup(context);const page=await context.newPage();await page.goto("/visual-tests/airport-session-preview.html");await page.getByRole("button",{name:/EKBI/}).click();await page.getByRole("button",{name:"Notes SAME123",exact:true}).click();
 server.boards.get(1)!.board_state.annotations.notes="PEER NOTE";server.boards.get(1)!.version=8;server.emit(1);
 await page.getByRole("textbox",{name:"RADIS notes",exact:true}).fill("MY DRAFT");await page.getByRole("button",{name:"OK",exact:true}).click();expect(server.actions[0]).toMatchObject({type:"strip.annotate",strip_id:11,version:1,value:"MY DRAFT"});
});

test("adjacent downward and upward drags use the intended insertion position",async({context})=>{
 const server=await setup(context);const page=await context.newPage();await page.goto("/visual-tests/airport-session-preview.html");await page.getByRole("button",{name:/EKBI/}).click();
 const next={...strip("EKBI"),id:12,callsign:"NEXT123",sequence:2000};
 for(const socket of server.sockets)if(socket.board.session_id===1)socket.ws.send(JSON.stringify({type:"strip_update",...next,board:socket.board}));
 await expect(page.getByTestId("radis-strip-NEXT123")).toBeVisible();
 const drag=async(fromCallsign:string,toCallsign:string)=>{
  await page.getByTestId(`radis-strip-${toCallsign}`).hover();await page.getByRole("button",{name:`Move ${fromCallsign}`,exact:true}).hover();
  const from=await page.getByRole("button",{name:`Move ${fromCallsign}`,exact:true}).boundingBox(),to=await page.getByTestId(`radis-strip-${toCallsign}`).boundingBox();if(!from||!to)throw new Error("missing drag geometry");
  await page.mouse.move(from.x+from.width/2,from.y+from.height/2);await page.mouse.down();await page.mouse.move(from.x+from.width/2+10,from.y+from.height/2,{steps:3});await page.mouse.move(to.x+to.width/2,to.y+to.height/2,{steps:20});await page.mouse.up();
 };
 await drag("SAME123","NEXT123");await expect.poll(()=>server.actions.length).toBe(1);expect(server.actions[0]).toMatchObject({type:"move",callsign:"SAME123",insert_after:{kind:"flight",callsign:"NEXT123"},revision:{strip_id:11,version:1}});
 await drag("NEXT123","SAME123");await expect.poll(()=>server.actions.length).toBe(2);expect(server.actions[1]).toMatchObject({type:"move",callsign:"NEXT123",insert_after:null});
});
