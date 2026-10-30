import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CaptureResult } from "posthog-js";

const posthogMock = vi.hoisted(() => ({
  init: vi.fn(),
  identify: vi.fn(),
  reset: vi.fn(),
  opt_out_capturing: vi.fn(),
  opt_in_capturing: vi.fn(),
  has_opted_out_capturing: vi.fn(() => false),
  stopSessionRecording: vi.fn(),
  startSessionRecording: vi.fn(),
  get_distinct_id: vi.fn(() => "anon"),
  get_property: vi.fn(() => "anon"),
  captureException: vi.fn(),
}));

vi.mock("posthog-js", () => ({ default: posthogMock }));

async function loadModule() {
  vi.resetModules();
  return import("@/lib/analytics");
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe("analytics consent", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    window.__APP_CONFIG__ = { posthogKey: "phc_test" };
  });

  afterEach(() => {
    window.__APP_CONFIG__ = undefined;
  });

  it("is unavailable and never asks for consent without a project key", async () => {
    window.__APP_CONFIG__ = {};
    const analytics = await loadModule();
    expect(analytics.isAnalyticsAvailable()).toBe(false);
    expect(analytics.needsConsentDecision()).toBe(false);
  });

  it("defaults to the EU host", async () => {
    const analytics = await loadModule();
    expect(analytics.getAnalyticsConfig()).toEqual({ key: "phc_test", host: "https://eu.i.posthog.com" });
  });

  it("does not load PostHog before consent", async () => {
    const analytics = await loadModule();
    expect(analytics.needsConsentDecision()).toBe(true);
    analytics.initAnalytics();
    analytics.identifyAnalyticsUser({ id: "oauth2|vatsim|1234567", cid: "1234567" });
    await flush();
    expect(posthogMock.init).not.toHaveBeenCalled();
  });

  it("loads PostHog and applies a pending identity when consent is granted", async () => {
    const analytics = await loadModule();
    analytics.identifyAnalyticsUser({ id: "oauth2|vatsim|1234567", cid: "1234567" });
    analytics.setConsent("granted");
    await flush();
    expect(posthogMock.init).toHaveBeenCalledWith("phc_test", expect.objectContaining({ persistence: "localStorage" }));
    expect(posthogMock.identify).toHaveBeenCalledWith("oauth2|vatsim|1234567", { vatsim_cid: "1234567" });
    expect(analytics.needsConsentDecision()).toBe(false);
  });

  it("starts automatically on a later visit when consent was stored", async () => {
    localStorage.setItem(
      "analytics-consent",
      JSON.stringify({ status: "granted", version: 1, decidedAt: "2026-10-29T00:00:00Z" }),
    );
    const analytics = await loadModule();
    analytics.initAnalytics();
    await flush();
    expect(posthogMock.init).toHaveBeenCalledOnce();
  });

  it("asks again when the stored consent is for an older policy version", async () => {
    localStorage.setItem("analytics-consent", JSON.stringify({ status: "granted", version: 0, decidedAt: "x" }));
    const analytics = await loadModule();
    expect(analytics.needsConsentDecision()).toBe(true);
  });

  it("opts out, resets and clears PostHog storage when consent is withdrawn", async () => {
    const analytics = await loadModule();
    analytics.setConsent("granted");
    await flush();
    localStorage.setItem("ph_phc_test_posthog", "{}");

    analytics.setConsent("denied");
    await flush();

    expect(posthogMock.opt_out_capturing).toHaveBeenCalled();
    expect(posthogMock.reset).toHaveBeenCalled();
    expect(localStorage.getItem("ph_phc_test_posthog")).toBeNull();
    expect(analytics.readConsent()?.status).toBe("denied");
  });
});

describe("analytics helpers", () => {
  it("extracts the VATSIM CID from an Auth0 subject", async () => {
    const { cidFromSubject } = await loadModule();
    expect(cidFromSubject("oauth2|vatsim|1234567")).toBe("1234567");
    expect(cidFromSubject("auth0|abc")).toBeUndefined();
  });

  it("redacts OAuth credentials from captured URLs", async () => {
    const { scrubUrl, scrubEvent } = await loadModule();
    expect(scrubUrl("https://flightstrips.dk/?code=secret&state=xyz&tab=1")).toBe(
      "https://flightstrips.dk/?code=redacted&state=redacted&tab=1",
    );
    expect(scrubUrl("https://flightstrips.dk/app")).toBe("https://flightstrips.dk/app");

    const event = scrubEvent({
      uuid: "1",
      event: "$pageview",
      properties: { $current_url: "https://flightstrips.dk/?code=secret" },
      $set_once: { $initial_current_url: "https://flightstrips.dk/?code=secret" },
    } as CaptureResult);
    expect(event?.properties.$current_url).toBe("https://flightstrips.dk/?code=redacted");
    expect((event?.$set_once as Record<string, string>).$initial_current_url).toBe(
      "https://flightstrips.dk/?code=redacted",
    );
  });
});
