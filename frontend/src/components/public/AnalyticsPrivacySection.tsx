import { useEffect, useState } from "react";
import { isAnalyticsAvailable, onConsentChanged, readConsent, setConsent, type ConsentStatus } from "@/lib/analytics";

/** Privacy policy section describing PostHog processing, with controls to grant or withdraw consent. */
export function AnalyticsPrivacySection() {
  const [status, setStatus] = useState<ConsentStatus | undefined>(() => readConsent()?.status);
  const available = isAnalyticsAvailable();

  useEffect(() => onConsentChanged((consent) => setStatus(consent.status)), []);

  return (
    <div id="analytics">
      <h2>Analytics, Error Tracking and Session Replay</h2>
      <p>
        If, and only if, you allow it in the consent banner, we use{" "}
        <a href="https://posthog.com" target="_blank" rel="noreferrer">PostHog</a> to understand how
        FlightStrips is used and to diagnose problems you report to us. PostHog acts as our data processor
        and stores the data in the European Union (Frankfurt).
      </p>
      <h3>What is collected</h3>
      <ul>
        <li>Page views, clicks, rage clicks and dead clicks, and heatmaps of where you click</li>
        <li>Session replay: a reconstruction of what was shown on your screen and how you interacted with it</li>
        <li>JavaScript errors, browser console output and network request timings (not their contents)</li>
        <li>Performance metrics (Core Web Vitals) and app version</li>
        <li>Browser, operating system, screen size, approximate location derived from your IP address</li>
        <li>When logged in: your VATSIM CID, so we can find your session when you report an issue</li>
      </ul>
      <h3>What is not collected</h3>
      <ul>
        <li>Anything you type into input fields — these are masked in the recording</li>
        <li>The text of private messages</li>
        <li>Your name, e-mail address, access tokens or the contents of network requests</li>
      </ul>
      <h3>Legal basis, retention and your choice</h3>
      <p>
        Processing is based on your consent (GDPR Art. 6(1)(a) and the ePrivacy rules on storing data on
        your device). Data is stored in your browser&apos;s local storage only after you consent; no cookies
        are used. Session recordings are deleted after 30 days and analytics events after 12 months.
        Declining has no effect on how FlightStrips works. You can withdraw consent at any time below; we
        then stop collecting immediately and remove the analytics identifiers from your browser. To have
        data that was already collected deleted, use our Contact page.
      </p>
      {available ? (
        <div className="flex flex-wrap items-center gap-3">
          <span>
            Current choice:{" "}
            <strong>{status === "granted" ? "Allowed" : status === "denied" ? "Declined" : "Not decided"}</strong>
          </span>
          <button
            type="button"
            className="fsl-link fsl-link--mono"
            onClick={() => setConsent("granted")}
            disabled={status === "granted"}
          >
            Allow
          </button>
          <button
            type="button"
            className="fsl-link fsl-link--mono"
            onClick={() => setConsent("denied")}
            disabled={status === "denied"}
          >
            Decline / withdraw
          </button>
        </div>
      ) : (
        <p>Analytics is not enabled on this deployment.</p>
      )}
    </div>
  );
}
