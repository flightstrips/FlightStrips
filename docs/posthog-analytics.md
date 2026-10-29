# PostHog analytics, error tracking and session replay

FlightStrips uses [PostHog](https://posthog.com) (EU Cloud) to diagnose issues that users report —
in particular "wrong click" problems, where a session recording shows exactly what happened.

Everything is **opt-in**. Nothing is loaded, stored or sent until the user clicks **Allow** in the
consent banner.

## How it works

| Piece | Location |
| --- | --- |
| Consent state, lazy SDK loading, PostHog config, URL scrubbing | `frontend/src/lib/analytics.ts` |
| Consent banner (first visit, and when re-opened) | `frontend/src/components/AnalyticsConsentBanner.tsx` |
| Link session to logged-in user (Auth0 `sub` + VATSIM CID) | `frontend/src/components/AnalyticsIdentity.tsx` |
| Privacy policy section with Allow / Withdraw controls | `frontend/src/components/public/AnalyticsPrivacySection.tsx` (`/privacy#analytics`) |
| "Privacy settings" link to re-open the banner | Public site footer |
| Runtime config | `POSTHOG_KEY`, `POSTHOG_HOST` → `window.__APP_CONFIG__` via `frontend/entrypoint.sh` |

- If `POSTHOG_KEY` is empty, analytics is fully disabled and the banner is never shown (this is the
  default for local development, see `frontend/public/config.js`).
- The consent choice is stored in `localStorage["analytics-consent"]` together with a policy version.
  Bump `CONSENT_POLICY_VERSION` in `analytics.ts` whenever the scope of collection changes; every
  user is then asked again.
- `posthog-js` is dynamically imported only after consent, so users who decline never download it.
- PostHog uses `localStorage` persistence — no cookies.
- Withdrawing consent stops recording, opts out, calls `reset()` and removes all `ph_*` keys.

### What is collected (after consent)

- Page views / page leaves (SPA history changes), autocapture of clicks, inputs (not their values) and
  form submits
- Rage clicks, dead clicks and heatmaps
- Exceptions (unhandled errors and promise rejections) with stack traces
- Web Vitals and network request timings
- Session replay including console logs
- Super properties: `app_version`, `deployment_version`
- Identified users: distinct id = Auth0 `sub`, person property `vatsim_cid`. No name or e-mail.

### What is never collected

- Input values (`maskAllInputs: true`)
- Private message text (`ph-no-capture` on `MessageStrip` text)
- Network request/response headers and bodies (they carry bearer tokens)
- OAuth `code` / `state` / token query parameters — redacted from URLs by `scrubUrl`
- WebSocket frames (PostHog does not record them)

To hide more UI from replays add the class `ph-mask` (masks text) or `ph-no-capture` (blocks the
element and excludes it from autocapture).

### Reporting handled errors

Unhandled errors are captured automatically. For errors you catch yourself:

```ts
import { captureAnalyticsException } from "@/lib/analytics";

captureAnalyticsException(error, { area: "websocket" });
```

This is a no-op without consent.

## Setup checklist

### 1. PostHog project

1. Create an organisation on **EU Cloud** (`https://eu.posthog.com`). Data must stay in the EU — do
   not use US Cloud.
2. Create a project "FlightStrips" (optionally a second project for staging).
3. Project settings:
   - **Session replay**: enable. Enable console log capture and network capture
     (performance only — headers and bodies are disabled client-side regardless).
   - **Session replay → Privacy and masking**: mask all inputs (matches client config).
   - **Session replay → Retention**: 30 days.
   - **Session replay → Sampling / minimum duration**: start at 100% / 2 seconds; lower sampling if
     volume becomes a cost issue.
   - **Error tracking / exception autocapture**: enable.
   - **Web analytics → Web vitals**: enable.
   - **Heatmaps** and **dead clicks**: enable.
   - **IP data capture**: keep enabled for GeoIP, or disable ("Discard client IP data") for stricter
     minimisation. If disabled, remove "approximate location" from the privacy policy.
   - **Authorized URLs** / **Toolbar**: add `https://flightstrips.dk`.
4. Upload source maps for readable stack traces (optional, see "Follow-ups").

### 2. Deployment

Add to the production `.env` used by `docker-compose.prod.yml`:

```env
POSTHOG_KEY=phc_xxxxxxxxxxxxxxxxxxxxxxxx
POSTHOG_HOST=https://eu.i.posthog.com
```

The project API key is a public client key; it is safe to expose in `config.js`.

### 3. Verify

1. Open the site in a private window → the consent banner appears.
2. Decline → DevTools Network shows no requests to `posthog.com`, no `ph_*` keys in localStorage.
3. Privacy settings → Allow → events and a recording appear in PostHog Live events within a minute.
4. Log in → person shows `vatsim_cid`.
5. Type in a text field and send a private message → both appear masked in the recording.
6. Withdraw on `/privacy#analytics` → capture stops and `ph_*` keys disappear.

## GDPR / compliance checklist

FlightStrips is the **data controller**; PostHog Inc. is the **data processor**.

- [ ] **DPA**: accept PostHog's Data Processing Agreement (Organisation settings → Billing/Legal, or
      `https://posthog.com/dpa`). Store the signed copy.
- [ ] **Data residency**: EU Cloud (Frankfurt). PostHog is certified under the EU-US Data Privacy
      Framework and the DPA includes SCCs for any sub-processor transfers.
- [ ] **Legal basis**: consent (GDPR Art. 6(1)(a)) — required anyway by ePrivacy Art. 5(3)
      (Danish *Cookiebekendtgørelsen*) because identifiers are stored on the device.
- [ ] **Valid consent**: freely given (equal Allow / Decline buttons, app works either way), specific,
      informed (banner + privacy policy), unambiguous (no pre-ticked / implied consent), and as easy
      to withdraw as to give (footer link + privacy page).
- [ ] **Proof of consent**: the timestamp and policy version are stored client-side. If proof becomes
      necessary, capture a `consent_granted` event server-side or in PostHog.
- [ ] **Record of Processing Activities (Art. 30)**: add an entry —
      purpose: error diagnosis and product improvement; categories: usage data, device data, VATSIM
      CID, screen recordings; recipients: PostHog; transfers: none outside EU (SCC fallback);
      retention: 30 days recordings / 12 months events.
- [ ] **DPIA screening (Art. 35)**: session replay is behavioural monitoring. With masking, opt-in,
      a non-sensitive simulation context and no special-category data, a full DPIA is likely not
      required, but document the screening outcome.
- [ ] **Retention**: set recording retention to 30 days in PostHog; events per plan (keep the privacy
      policy's "12 months" in sync with the actual setting).
- [ ] **Data subject requests**: access/erasure — search the person in PostHog by `vatsim_cid` and
      use *Delete person* (with "delete events and recordings"). Respond within one month.
- [ ] **Privacy policy & data-handling page** updated (this branch). Update the "Last updated" date
      and bump `CONSENT_POLICY_VERSION` when anything material changes.
- [ ] **Access control**: limit PostHog organisation membership to maintainers who handle support;
      enforce 2FA/SSO.
- [ ] **VATSIM**: VATSIM CIDs are personal data under GDPR — no extra approval needed, but keep
      VATSIM's data protection policy in mind if data is shared with divisions.

## Follow-ups (not in this branch)

- **Source maps**: upload Vite source maps to PostHog in CI (`posthog-cli sourcemap upload`) for
  readable stack traces; set `build.sourcemap: 'hidden'` in `vite.config.ts`.
- **Reverse proxy**: proxy `/ingest` → `eu.i.posthog.com` in `nginx.conf` so ad-blockers do not drop
  events from consenting users.
- **Context properties**: register the active airport / position / layout as super properties so
  recordings can be filtered by position.
- **Backend errors**: `posthog-go` could forward backend errors with the same distinct id; this is
  covered by the same consent only if tied to the frontend choice, so evaluate separately.
- **EuroScope plugin**: out of scope; native app, no consent UI.
