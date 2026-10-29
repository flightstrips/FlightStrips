import { useEffect, useState } from "react";
import {
  needsConsentDecision,
  isAnalyticsAvailable,
  onOpenConsentSettings,
  readConsent,
  setConsent,
} from "@/lib/analytics";

/**
 * Opt-in consent banner for analytics, error tracking and session replay.
 * Rendered globally; shown on first visit (and whenever the consent policy
 * version is bumped) and re-opened via `openConsentSettings()`.
 */
export function AnalyticsConsentBanner() {
  const [open, setOpen] = useState(needsConsentDecision);

  useEffect(() => onOpenConsentSettings(() => {
    if (isAnalyticsAvailable()) setOpen(true);
  }), []);

  if (!open) return null;

  const current = readConsent()?.status;
  const decide = (status: "granted" | "denied") => {
    setConsent(status);
    setOpen(false);
  };

  return (
    <div
      role="dialog"
      aria-modal="false"
      aria-labelledby="analytics-consent-title"
      className="ph-no-capture fixed bottom-4 left-4 z-[10000] w-[min(26rem,calc(100vw-2rem))] rounded-md border border-[#3a4a52] bg-[#0f1d22] p-4 text-sm text-[#e3ecef] shadow-2xl"
    >
      <h2 id="analytics-consent-title" className="mb-2 text-base font-semibold text-white">
        Help us fix issues faster
      </h2>
      <p className="mb-2 leading-relaxed text-[#c3d1d6]">
        With your permission we use PostHog (EU servers) to record how FlightStrips is used:
        clicks, page views, errors, performance and a <strong>screen recording of your session</strong>.
        Typed text is masked and private messages are hidden. This lets us see what went wrong when
        you report a problem.
      </p>
      <p className="mb-4 leading-relaxed text-[#c3d1d6]">
        It is optional and FlightStrips works the same either way. You can change your choice at any
        time under <a href="/privacy#analytics" className="underline hover:text-white">Privacy settings</a>.
        {current && (
          <> Current choice: <strong>{current === "granted" ? "allowed" : "declined"}</strong>.</>
        )}
      </p>
      <div className="flex flex-wrap justify-end gap-2">
        <button
          type="button"
          onClick={() => decide("denied")}
          className="rounded-md border border-[#56707a] px-4 py-2 font-medium text-white hover:bg-[#1c3038]"
        >
          Decline
        </button>
        <button
          type="button"
          onClick={() => decide("granted")}
          className="rounded-md border border-[#56707a] px-4 py-2 font-medium text-white hover:bg-[#1c3038]"
        >
          Allow
        </button>
      </div>
    </div>
  );
}
