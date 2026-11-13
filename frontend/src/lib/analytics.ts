import type { PostHog, CaptureResult, PostHogConfig } from "posthog-js";
import { FRONTEND_VERSION } from "@/lib/app-version";

/**
 * Product analytics, error tracking and session replay via PostHog.
 *
 * GDPR / ePrivacy: nothing is loaded, stored or sent until the user has given
 * explicit opt-in consent through the consent banner. The PostHog SDK is
 * imported lazily so users who decline never download it.
 *
 * Bump CONSENT_POLICY_VERSION whenever the scope of processing changes so that
 * every user is asked again.
 */
export const CONSENT_POLICY_VERSION = 1;

const CONSENT_STORAGE_KEY = "analytics-consent";
const CONSENT_CHANGED_EVENT = "flightstrips:analytics-consent-changed";
const OPEN_CONSENT_EVENT = "flightstrips:analytics-consent-open";

export type ConsentStatus = "granted" | "denied";

export interface StoredConsent {
  status: ConsentStatus;
  version: number;
  decidedAt: string;
}

export interface AnalyticsIdentity {
  id: string;
  cid?: string;
}

/** Query parameters that may carry credentials (Auth0 / ChartFox OAuth callbacks). */
const SENSITIVE_QUERY_PARAMS = [
  "code",
  "state",
  "token",
  "access_token",
  "id_token",
  "refresh_token",
  "error_description",
];

const URL_PROPERTIES = [
  "$current_url",
  "$referrer",
  "$initial_current_url",
  "$initial_referrer",
  "$prev_pageview_pathname",
];

export function getAnalyticsConfig(): { key: string; host: string } | null {
  const key = window.__APP_CONFIG__?.posthogKey?.trim();
  if (!key) return null;
  const host = window.__APP_CONFIG__?.posthogHost?.trim() || "https://eu.i.posthog.com";
  return { key, host: host.replace(/\/$/, "") };
}

export function isAnalyticsAvailable(): boolean {
  return getAnalyticsConfig() !== null;
}

export function readConsent(): StoredConsent | null {
  try {
    const raw = localStorage.getItem(CONSENT_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<StoredConsent>;
    if (
      (parsed.status !== "granted" && parsed.status !== "denied") ||
      parsed.version !== CONSENT_POLICY_VERSION
    ) {
      return null;
    }
    return parsed as StoredConsent;
  } catch {
    return null;
  }
}

/** True when the banner must be shown: analytics is configured and no current decision exists. */
export function needsConsentDecision(): boolean {
  return isAnalyticsAvailable() && readConsent() === null;
}

export function scrubUrl(value: string): string {
  try {
    const url = new URL(value, window.location.origin);
    let changed = false;
    for (const param of SENSITIVE_QUERY_PARAMS) {
      if (url.searchParams.has(param)) {
        url.searchParams.set(param, "redacted");
        changed = true;
      }
    }
    return changed ? url.toString() : value;
  } catch {
    return value;
  }
}

export function scrubEvent(event: CaptureResult | null): CaptureResult | null {
  if (!event) return event;
  const properties = event.properties ?? {};
  for (const key of URL_PROPERTIES) {
    const value = properties[key];
    if (typeof value === "string") {
      properties[key] = scrubUrl(value);
    }
  }
  const setOnce = event.$set_once as Record<string, unknown> | undefined;
  if (setOnce) {
    for (const key of URL_PROPERTIES) {
      const value = setOnce[key];
      if (typeof value === "string") {
        setOnce[key] = scrubUrl(value);
      }
    }
  }
  return event;
}

/** Extract the VATSIM CID from an Auth0 subject such as `oauth2|vatsim|1234567`. */
export function cidFromSubject(sub: string): string | undefined {
  const last = sub.split("|").pop();
  return last && /^\d+$/.test(last) ? last : undefined;
}

function buildConfig(host: string): Partial<PostHogConfig> {
  return {
    api_host: host,
    ui_host: host.includes("eu.") ? "https://eu.posthog.com" : "https://us.posthog.com",
    // localStorage only — no cookies are set.
    persistence: "localStorage",
    person_profiles: "identified_only",

    // Product analytics
    capture_pageview: "history_change",
    capture_pageleave: true,
    autocapture: true,
    rageclick: true,
    capture_dead_clicks: true,
    enable_heatmaps: true,
    capture_performance: { web_vitals: true, network_timing: true },

    // Error tracking
    capture_exceptions: true,

    // Session replay
    disable_session_recording: false,
    enable_recording_console_log: true,
    session_recording: {
      maskAllInputs: true,
      // Never record request/response headers or bodies: they carry bearer tokens.
      recordHeaders: false,
      recordBody: false,
      maskCapturedNetworkRequestFn: (request) => ({ ...request, name: scrubUrl(request.name) }),
    },

    before_send: scrubEvent,
    loaded: (instance) => {
      instance.register({
        app_version: FRONTEND_VERSION,
        deployment_version: window.__APP_CONFIG__?.deploymentVersion ?? "local",
      });
    },
  };
}

let posthogPromise: Promise<PostHog | null> | null = null;
let pendingIdentity: AnalyticsIdentity | null = null;

function loadPostHog(): Promise<PostHog | null> {
  const config = getAnalyticsConfig();
  if (!config) return Promise.resolve(null);

  if (!posthogPromise) {
    posthogPromise = import("posthog-js")
      .then(({ default: posthog }) => {
        posthog.init(config.key, buildConfig(config.host));
        if (pendingIdentity) {
          applyIdentity(posthog, pendingIdentity);
        }
        return posthog;
      })
      .catch((error: unknown) => {
        console.warn("Failed to load analytics", error);
        posthogPromise = null;
        return null;
      });
  }
  return posthogPromise;
}

function applyIdentity(posthog: PostHog, identity: AnalyticsIdentity) {
  // Pseudonymous: only the Auth0 subject and VATSIM CID — no name or e-mail.
  posthog.identify(identity.id, identity.cid ? { vatsim_cid: identity.cid } : undefined);
}

function clearPostHogStorage() {
  for (const storage of [localStorage, sessionStorage]) {
    for (let i = storage.length - 1; i >= 0; i--) {
      const key = storage.key(i);
      if (key && (key.startsWith("ph_") || key.startsWith("__ph"))) {
        storage.removeItem(key);
      }
    }
  }
}

/** Start analytics if the user has previously consented. Call once at startup. */
export function initAnalytics(): void {
  if (readConsent()?.status === "granted") {
    void loadPostHog();
  }
}

export function setConsent(status: ConsentStatus): void {
  const record: StoredConsent = {
    status,
    version: CONSENT_POLICY_VERSION,
    decidedAt: new Date().toISOString(),
  };
  localStorage.setItem(CONSENT_STORAGE_KEY, JSON.stringify(record));

  if (status === "granted") {
    void loadPostHog().then((posthog) => {
      if (posthog?.has_opted_out_capturing()) {
        posthog.opt_in_capturing();
        posthog.startSessionRecording();
      }
    });
  } else if (posthogPromise) {
    void posthogPromise.then((posthog) => {
      posthog?.stopSessionRecording();
      posthog?.opt_out_capturing();
      posthog?.reset();
      clearPostHogStorage();
    });
  } else {
    clearPostHogStorage();
  }

  window.dispatchEvent(new CustomEvent(CONSENT_CHANGED_EVENT, { detail: record }));
}

export function identifyAnalyticsUser(identity: AnalyticsIdentity): void {
  pendingIdentity = identity;
  if (readConsent()?.status !== "granted") return;
  void loadPostHog().then((posthog) => {
    if (posthog && posthog.get_distinct_id() !== identity.id) {
      applyIdentity(posthog, identity);
    }
  });
}

/** Unlink the current device from a previously identified user (e.g. after logout). */
export function resetAnalyticsUser(): void {
  pendingIdentity = null;
  void posthogPromise?.then((posthog) => {
    if (posthog && posthog.get_distinct_id() !== posthog.get_property("$device_id")) {
      posthog.reset();
    }
  });
}

/** Manually report a handled error (e.g. a caught WebSocket failure). No-op without consent. */
export function captureAnalyticsException(error: unknown, properties?: Record<string, unknown>): void {
  void posthogPromise?.then((posthog) => posthog?.captureException(error, properties));
}

/** Re-open the consent banner, e.g. from a "Privacy settings" link. */
export function openConsentSettings(): void {
  window.dispatchEvent(new Event(OPEN_CONSENT_EVENT));
}

export function onOpenConsentSettings(listener: () => void): () => void {
  window.addEventListener(OPEN_CONSENT_EVENT, listener);
  return () => window.removeEventListener(OPEN_CONSENT_EVENT, listener);
}

export function onConsentChanged(listener: (consent: StoredConsent) => void): () => void {
  const handler = (event: Event) => listener((event as CustomEvent<StoredConsent>).detail);
  window.addEventListener(CONSENT_CHANGED_EVENT, handler);
  return () => window.removeEventListener(CONSENT_CHANGED_EVENT, handler);
}
