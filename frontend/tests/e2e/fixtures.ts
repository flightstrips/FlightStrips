import {test as base, expect, type BrowserContext, type Page} from "@playwright/test";
import {identity} from "./identity";
import type {SessionDiscovery} from "../../src/api/sessions";

export async function chooseAirport(page: Page, airport: string, environment = "LIVE") {
  const discovery = page.waitForResponse(response =>
    new URL(response.url()).pathname === "/api/plugin-memberships" && response.ok());
  await page.goto("/app");
  const available = await (await discovery).json() as SessionDiscovery;
  const memberships = available.transports.flatMap(transport => transport.memberships);
  const atAirport = memberships.filter(member => member.airport === airport);
  const target = atAirport.find(member => member.environment === environment);
  expect(target, `fixture connection for ${airport} in ${environment}`).toBeDefined();

  if (new Set(memberships.map(member => member.airport)).size > 1) {
    await page.getByRole("dialog", {name: "Choose airport session"})
      .getByRole("button", {name: airport, exact: true}).click();
  }
  if (atAirport.length > 1) {
    await page.getByRole("dialog", {name: "Choose your plugin instance"})
      .getByRole("button").filter({has: page.getByText(target!.instance_id, {exact: false})}).click();
  }
}

type Evidence = {watch: (context: BrowserContext) => void};
type Fixtures = {
  evidence: Evidence;
  session: (token: string) => Promise<Page>;
};

// Shared diagnostics cover every controller, including independently authenticated
// browser contexts. Playwright retains normal page traces/screenshots on failure.
export const test = base.extend<Fixtures>({
  evidence: [async ({context}, provide, testInfo) => {
    const errors: string[] = [];
    const frames: unknown[] = [];
    const watchPage = (page: Page) => {
      page.on("console", message => {if (message.type() === "error") errors.push(message.text());});
      page.on("pageerror", error => errors.push(error.message));
      page.on("websocket", socket => {
        socket.on("framereceived", ({payload}) => frames.push({page: page.url(), direction: "received", payload: String(payload)}));
        socket.on("framesent", ({payload}) => frames.push({page: page.url(), direction: "sent", payload: String(payload)}));
      });
    };
    const watch = (observed: BrowserContext) => {
      observed.pages().forEach(watchPage);
      observed.on("page", watchPage);
    };
    watch(context);
    await provide({watch});
    await testInfo.attach("browser-console", {body: JSON.stringify(errors, null, 2), contentType: "application/json"});
    await testInfo.attach("browser-websockets", {body: JSON.stringify(frames, null, 2), contentType: "application/json"});
    expect(errors, "browser console and uncaught errors").toEqual([]);
  }, {auto: true}],
  session: async ({browser, evidence, baseURL}, provide, testInfo) => {
    const contexts: BrowserContext[] = [];
    await provide(async token => {
      const context = await browser.newContext({baseURL, viewport: {width: 1920, height: 1080}});
      contexts.push(context);
      evidence.watch(context);
      const page = await context.newPage();
      await identity(page, String(testInfo.project.metadata.backend), token);
      return page;
    });
    for (const [index, context] of contexts.entries()) {
      if (testInfo.status !== testInfo.expectedStatus) {
        for (const page of context.pages()) await testInfo.attach(`controller-${index}-screenshot`, {body: await page.screenshot(), contentType: "image/png"});
      }
      await context.close();
    }
  },
});
