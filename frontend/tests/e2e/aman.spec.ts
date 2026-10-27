import {expect, test, type APIRequestContext, type Page, type WebSocketRoute} from "@playwright/test";
import {execFileSync} from "node:child_process";
import {writeFileSync} from "node:fs";
import {resolve} from "node:path";
import type {AMANState} from "../../src/api/aman";

const backend = "http://127.0.0.1:4180";
const epoch = "2026-10-26T18:00:00.000Z";
interface NativeMessage {generated_at: string; values: Array<{callsign: string; data_status: string; gain_loss_seconds?: number}>}
const hasNativeGuidance = (event: NativeMessage, callsign: string) => event.values?.some(value => value.callsign === callsign && value.data_status === "fresh" && value.gain_loss_seconds !== undefined);

async function nativeFrame(request: APIRequestContext, accepts: (event: NativeMessage) => boolean) {
  let selected = "";
  await expect.poll(async () => {
    const capture: {events: NativeMessage[]; frames: string[]} = await (await request.get(`${backend}/e2e/euroscope`)).json();
    const index = capture.events.findLastIndex(accepts);
    selected = capture.frames[index] ?? "";
    return selected !== "";
  }).toBe(true);
  return selected;
}

// A source transition can produce several real reconciliation commits. Wait
// until those commits are visible on both sides before testing a new command.
async function settledRevision(request: APIRequestContext, received: () => number | undefined) {
  let last = -1;
  let stable = 0;
  await expect.poll(async () => {
    const persisted = await (await request.get(`${backend}/e2e/persisted`)).json();
    stable = persisted.Revision === last ? stable + 1 : 0;
    last = persisted.Revision;
    return stable >= 3 && received() === last;
  }).toBe(true);
}

// Seed only the external identity provider's SDK cache. The normal Auth0
// provider, app route, WebSocket provider and server-side roles remain in use.
async function identity(page: Page, token = "__TEST_TOKEN__") {
  await page.addInitScript(({token}) => {
    const clientId = "mIjRYlbKHpTwnNAkhcu9plQP541Klwvn";
    const scope = "openid profile email offline_access";
    const claims = {sub: "auth0|aman-e2e", name: "AMAN E2E", exp: Math.floor(Date.now() / 1000) + 86400};
    const idToken = `${btoa('{"alg":"RS256"}')}.${btoa(JSON.stringify(claims))}.fixture`;
    const decodedToken = {claims, user: {sub: claims.sub, name: claims.name}, encoded: {header: "", payload: "", signature: ""}, header: {alg: "RS256"}};
    localStorage.setItem(`@@auth0spajs@@::${clientId}::@@user@@`, JSON.stringify({id_token: idToken, decodedToken}));
    localStorage.setItem(`@@auth0spajs@@::${clientId}::backend-dev::${scope}`, JSON.stringify({expiresAt: claims.exp, body: {access_token: token, id_token: idToken, decodedToken, scope, audience: "backend-dev", client_id: clientId, expires_in: 86400, token_type: "Bearer"}}));
  }, {token});
  await page.route("**/config.js", route => route.fulfill({contentType: "text/javascript", body: `window.__APP_CONFIG__={wsUrl:"ws://127.0.0.1:4180/frontEndEvents",apiBaseUrl:"${backend}",clientId:"mIjRYlbKHpTwnNAkhcu9plQP541Klwvn",audience:"backend-dev",connection:"vatsim-dev"};`}));
  await page.route(/^https:\/\/fonts\.(googleapis|gstatic)\.com\//, route => route.fulfill({body: "", contentType: "text/css"}));
  await page.route("**/version.json", route => route.fulfill({json: {deploymentVersion: "aman-e2e"}}));
}

async function openAMAN(page: Page) {
  await page.goto("/app");
  const choose = page.getByRole("button", {name: "AMAN", exact: true});
  const workspace = page.getByRole("main", {name: "Arrival management workspace"});
  await expect(choose.or(workspace).first()).toBeVisible();
  if (await choose.isVisible()) await choose.click();
  await expect(page.getByRole("region", {name: "TMT traffic prediction", exact: true})).toBeVisible();
}

function target(page: Page, callsign: string) {
  return page.getByRole("button", {name: new RegExp(`^Select ${callsign};`)}).first();
}

async function rejectedCommand(page: Page, token: string, revision: number) {
  return page.evaluate(({token, revision}) => new Promise<{code: string}>((resolve, reject) => {
    const socket = new WebSocket("ws://127.0.0.1:4180/frontEndEvents");
    const deadline = setTimeout(() => {socket.close(); reject(new Error("no command rejection from backend"));}, 10_000);
    let sent = false;
    socket.onopen = () => socket.send(JSON.stringify({type: "token", token, version: "e2e"}));
    socket.onerror = () => reject(new Error("command socket failed"));
    socket.onmessage = ({data}) => {
      const event = JSON.parse(data);
      if (event.type === "aman.state" && !sent) {
        sent = true;
        socket.send(JSON.stringify({type: "aman.set_holding_eat_writeback", version: 1, data: {command_id: crypto.randomUUID(), expected_revision: revision, enabled: true}}));
      }
      if (event.type === "aman.command_rejected") {clearTimeout(deadline); socket.close(); resolve(event.data);}
    };
  }), {token, revision});
}

test("running AMAN preserves healthy targets, diagnoses degradation and persists authorized commands", async ({page, request, context}, testInfo) => {
  const errors: string[] = [];
  const frames: unknown[] = [];
  let state: AMANState | undefined;
  let healthyNative = "";
  let unreliableNative = "";
  const observeErrors = (observed: Page) => {
    observed.on("console", message => {if (message.type() === "error") errors.push(message.text());});
    observed.on("pageerror", error => errors.push(error.message));
  };
  observeErrors(page);
  context.on("page", observeErrors);
  page.on("websocket", socket => socket.on("framereceived", ({payload}) => {
    if (typeof payload !== "string") return;
    const event = JSON.parse(payload);
    frames.push(event);
    if (event.type === "aman.state") state = event.data;
  }));
  try {
    await identity(page);
    await page.clock.setFixedTime(new Date(epoch));
    await test.step("healthy neighbours and real TMT buckets with supported weather fallback", async () => {
      await openAMAN(page);
      for (const callsign of ["SAS101", "SAS202", "CCA877Q"]) await expect(target(page, callsign)).toBeVisible();
      await expect.poll(() => state?.technical_health.ready).toBe(true);
      for (const callsign of ["SAS101", "SAS202"]) expect(state!.flights.find(f => f.callsign === callsign)?.slot).not.toBeNull();
      const traffic = page.getByRole("region", {name: "TMT traffic prediction", exact: true});
      await expect(traffic.getByRole("listitem")).toHaveCount(state!.traffic_prediction!.buckets.length);
      for (const callsign of ["SAS101", "SAS202"]) await expect(traffic.locator(`article[title*="${callsign}"]`).first()).toBeVisible();
      expect(state!.traffic_prediction!.buckets.reduce((count, bucket) => count + bucket.count, 0)).toBe(3);
      for (const bucket of state!.traffic_prediction!.buckets) {
        await expect(traffic.getByRole("listitem", {name: `${bucket.start.slice(11, 16)} to ${bucket.end.slice(11, 16)}: ${bucket.count} arrivals, load factor ${bucket.count * 4}`, exact: true})).toBeVisible();
      }
      await page.getByRole("button", {name: "RWY", exact: true}).click();
      for (const callsign of ["SAS101", "SAS202"]) await expect(page.getByTestId(`operational-marker-${callsign}`)).toBeVisible();
      await page.getByRole("button", {name: "ALL", exact: true}).click();
      await page.getByRole("tab", {name: /^Warnings/}).click();
      await expect(page.getByRole("tabpanel")).toContainText(/weather|wind/i);
      healthyNative = await nativeFrame(request, event => ["SAS101", "SAS202", "CCA877Q"].every(c => hasNativeGuidance(event, c)));
    });
    await test.step("a genuine prediction timeout retains visible healthy aircraft and runway slots", async () => {
      expect((await request.post(`${backend}/e2e/scenario`, {data: {name: "spike"}})).ok()).toBe(true);
      await expect.poll(async () => (await (await request.get(`${backend}/e2e/persisted`)).json()).Flights.find((f: {Callsign: string}) => f.Callsign === "CCA877Q").PredictionFilter?.Pending).toBeTruthy();
      expect((await request.post(`${backend}/e2e/scenario`, {data: {name: "unreliable"}})).ok()).toBe(true);
      await expect.poll(() => state?.warnings?.some(w => w.code === "prediction_unreliable" && w.callsign === "CCA877Q")).toBe(true);
      for (const callsign of ["SAS101", "SAS202"]) {
        await expect(target(page, callsign)).toBeVisible();
        expect(state!.flights.find(f => f.callsign === callsign)?.slot).not.toBeNull();
      }
      await page.getByRole("button", {name: "RWY", exact: true}).click();
      for (const callsign of ["SAS101", "SAS202"]) await expect(page.getByTestId(`operational-marker-${callsign}`)).toBeVisible();
      await page.getByRole("button", {name: "ALL", exact: true}).click();
      await expect(page.getByRole("tabpanel")).toContainText("CCA877Q");
      await expect.poll(() => state?.technical_health.ready).toBe(true);
      await page.getByRole("tab", {name: "Settings", exact: true}).click();
      await expect(page.getByRole("checkbox", {name: "Write holding EAT to EuroScope"})).toBeEnabled();
      unreliableNative = await nativeFrame(request, event => event.values?.some(v => v.callsign === "CCA877Q" && v.data_status === "fresh" && v.gain_loss_seconds === undefined) && ["SAS101", "SAS202"].every(c => hasNativeGuidance(event, c)));
    });
    await test.step("arrival-rate action resequences real slots and persists runway policy", async () => {
      await settledRevision(request, () => state?.revision);
      await page.getByRole("button", {name: "ARRIVAL-22L: 40/h", exact: true}).click();
      const dialog = page.getByRole("dialog", {name: "Arrival rate", exact: true});
      await dialog.getByRole("spinbutton", {name: "Arrivals per hour"}).fill("30");
      await dialog.getByRole("combobox", {name: "Rate runway group"}).selectOption("ARRIVAL-22L");
      await dialog.getByRole("button", {name: "Set arrival rate", exact: true}).click();
      await expect.poll(() => state?.runway_groups.find(group => group.id === "ARRIVAL-22L")?.active_rate_per_hour).toBe(30);
      const persisted = await (await request.get(`${backend}/e2e/persisted`)).json();
      expect(persisted.RunwayGroups.find((group: {ID: string}) => group.ID === "ARRIVAL-22L").ActiveRatePerHour).toBe(30);
      const first = state!.flights.find(flight => flight.callsign === "SAS101")!.slot!;
      const second = state!.flights.find(flight => flight.callsign === "SAS202")!.slot!;
      expect(Date.parse(second.time) - Date.parse(first.time)).toBeGreaterThanOrEqual(120_000);
      await dialog.getByRole("button", {name: "Cancel", exact: true}).click();
      await expect(page.getByRole("button", {name: "ARRIVAL-22L: 30/h", exact: true})).toBeVisible();
    });
    await test.step("EAT setting travels through the server and PostgreSQL replacement", async () => {
      await settledRevision(request, () => state?.revision);
      const setting = page.getByRole("checkbox", {name: "Write holding EAT to EuroScope"});
      await setting.click();
      await expect.poll(() => state?.holding_eat_writeback_enabled).toBe(false);
      await expect.poll(async () => (await (await request.get(`${backend}/e2e/persisted`)).json()).HoldingEATWritebackEnabled).toBe(false);
      await expect(setting).not.toBeChecked();
    });
    await test.step("server rejects stale revisions and observer commands without persisted effects", async () => {
      expect((await rejectedCommand(page, "__TEST_TOKEN__", 0)).code).toBe("revision_conflict");
      expect((await request.post(`${backend}/e2e/observer`)).ok()).toBe(true);
      expect((await rejectedCommand(page, "__TEST_FRONTEND_TOKEN__", state!.revision)).code).toBe("unauthorized");
      const observer = await context.newPage();
      await identity(observer, "__TEST_FRONTEND_TOKEN__");
      await observer.clock.setFixedTime(new Date(epoch));
      await openAMAN(observer);
      await expect(target(observer, "SAS101")).toBeVisible();
      await observer.getByRole("tab", {name: "Settings", exact: true}).click();
      await expect(observer.getByRole("checkbox", {name: "Write holding EAT to EuroScope"})).toBeDisabled();
      expect((await (await request.get(`${backend}/e2e/persisted`)).json()).HoldingEATWritebackEnabled).toBe(false);
      await observer.close();
    });
    await test.step("real shared-source failure restricts commands while retaining operational views", async () => {
      expect((await request.post(`${backend}/e2e/scenario`, {data: {name: "outage"}})).ok()).toBe(true);
      await expect.poll(() => state?.technical_health.ready).toBe(false);
      for (const callsign of ["SAS101", "SAS202"]) await expect(target(page, callsign)).toBeVisible();
      await expect(page.getByRole("checkbox", {name: "Write holding EAT to EuroScope"})).toBeDisabled();
      await expect(page.getByRole("region", {name: "TMT traffic prediction", exact: true}).locator('article[title*="SAS101"]').first()).toBeVisible();
      expect((await rejectedCommand(page, "__TEST_TOKEN__", state!.revision)).code).toBe("read_only");
      expect((await (await request.get(`${backend}/e2e/persisted`)).json()).HoldingEATWritebackEnabled).toBe(false);
    });
    await test.step("source and aircraft recover through new observations", async () => {
      expect((await request.post(`${backend}/e2e/scenario`, {data: {name: "recovery"}})).ok()).toBe(true);
      await expect.poll(() => state?.technical_health.ready).toBe(true);
      await expect.poll(() => state?.warnings?.some(w => w.code === "prediction_unreliable")).toBe(false);
      for (const callsign of ["SAS101", "SAS202", "CCA877Q"]) await expect(target(page, callsign)).toBeVisible();
      await expect(page.getByRole("checkbox", {name: "Write holding EAT to EuroScope"})).toBeEnabled();
      const recoveredNative = await nativeFrame(request, event => Date.parse(event.generated_at) >= Date.parse(state!.generated_at) && ["SAS101", "SAS202", "CCA877Q"].every(c => hasNativeGuidance(event, c)));
      const paths = [healthyNative, unreliableNative, recoveredNative].map((frame, index) => {
        expect(frame).toBeTruthy();
        const path = testInfo.outputPath(`live-euroscope-${index}.pb`);
        writeFileSync(path, Buffer.from(frame, "base64"));
        return path;
      });
      const decoder = resolve(`aman-e2e-artifacts/native/aman-live-decoder${process.platform === "win32" ? ".exe" : ""}`);
      const output = execFileSync(decoder, paths, {encoding: "utf8"});
      await testInfo.attach("native-decoder", {body: output, contentType: "text/plain"});
    });
    let corruption: "unsupported" | "malformed" | null = null;
    let serverSocket: WebSocketRoute | undefined;
    let browserSocket: WebSocketRoute | undefined;
    let previous: string | undefined;
    let replayOlder = false;
    let replayCount = 0;
    let connections = 0;
    const corruptedDeliveries = {unsupported: 0, malformed: 0};
    await page.routeWebSocket("**/frontEndEvents", route => {
      connections++;
      browserSocket = route;
      const server = route.connectToServer();
      serverSocket = server;
      server.onMessage(message => {
        const event = JSON.parse(String(message));
        if (event.type !== "aman.state") {route.send(message); return;}
        if (corruption === "unsupported") event.data.warnings.push({id: "future-warning", source: "future_source", severity: "warning", code: "new_code", message: "Future warning"});
        if (corruption === "malformed") event.data.warnings = {unexpected: "object"};
        route.send(JSON.stringify(event));
        if (corruption !== null) corruptedDeliveries[corruption]++;
        if (replayOlder && previous) {route.send(previous); replayCount++;}
        if (!replayOlder && corruption === null) previous = String(message);
      });
    });
    await test.step("malformed and unsupported warning metadata retains real operational views", async () => {
      corruption = "unsupported";
      await page.reload();
      const choose = page.getByRole("button", {name: "AMAN", exact: true});
      await expect(choose.or(page.getByRole("main", {name: "Arrival management workspace"})).first()).toBeVisible();
      if (await choose.isVisible()) await choose.click();
      await page.getByRole("tab", {name: /^Warnings/}).click();
      for (const mode of ["unsupported", "malformed"] as const) {
        corruption = mode;
        const before = corruptedDeliveries[mode];
        expect((await request.post(`${backend}/e2e/scenario`, {data: {name: "recovery"}})).ok()).toBe(true);
        await expect.poll(() => corruptedDeliveries[mode]).toBeGreaterThan(before);
        await expect(page.getByRole("tabpanel")).toContainText("Some AMAN warnings are unavailable");
        if (mode === "unsupported") await expect(page.getByRole("tabpanel")).toContainText(/weather|wind/i);
        for (const callsign of ["SAS101", "SAS202"]) await expect(target(page, callsign)).toBeVisible();
        await expect(page.getByRole("region", {name: "TMT traffic prediction", exact: true}).locator('article[title*="SAS101"]').first()).toBeVisible();
      }
      corruption = null;
      expect((await request.post(`${backend}/e2e/scenario`, {data: {name: "recovery"}})).ok()).toBe(true);
      await expect(page.getByRole("tabpanel")).not.toContainText("Some AMAN warnings are unavailable");
    });
    await test.step("older replacements cannot undo confirmed UI commands", async () => {
      await expect.poll(() => previous).toBeTruthy();
      await page.getByRole("tab", {name: "Settings", exact: true}).click();
      const setting = page.getByRole("checkbox", {name: "Write holding EAT to EuroScope"});
      await expect(setting).toBeEnabled();
      await settledRevision(request, () => state?.revision);
      replayOlder = true;
      await setting.click();
      await expect(setting).toBeChecked();
      await expect.poll(() => replayCount).toBeGreaterThanOrEqual(2);
      await expect(setting).toBeChecked();
      expect((await (await request.get(`${backend}/e2e/persisted`)).json()).HoldingEATWritebackEnabled).toBe(true);
      replayOlder = false;
    });
    await test.step("real WebSocket reconnect reloads persisted state and restores targets", async () => {
      const before = connections;
      await serverSocket!.close({code: 1012, reason: "E2E reconnect"});
      await browserSocket!.close({code: 1012, reason: "E2E reconnect"});
      await expect.poll(() => connections).toBeGreaterThan(before);
      await expect(target(page, "SAS101")).toBeVisible();
      await expect(target(page, "SAS202")).toBeVisible();
      await page.getByRole("tab", {name: "Settings", exact: true}).click();
      await expect(page.getByRole("checkbox", {name: "Write holding EAT to EuroScope"})).toBeChecked();
    });
    expect(errors).toEqual([]);
  } finally {
    await testInfo.attach("browser-console", {body: JSON.stringify(errors, null, 2), contentType: "application/json"});
    await testInfo.attach("websocket-frames", {body: JSON.stringify(frames, null, 2), contentType: "application/json"});
  }
});
