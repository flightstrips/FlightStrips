# FlightStrips frontend

## Component catalogue (Storybook)

From this directory, run `npm ci` and `npm run storybook`, then open the local URL printed in the terminal (normally http://127.0.0.1:6006). No backend, account or environment secrets are needed. Start with **Start here** for non-developer review instructions. Copy a story URL when reporting a visual problem, together with a screenshot, viewport size, changed controls and expected behavior.

The catalogue imports production components and `src/index.css`, including the controller font and Tailwind tokens. It covers every shared UI primitive, all nine layouts dispatched by `FlightStrip`, ownership indicators, shared strip cells, tactical strips and AMAN warning states. It is an initial catalogue: composed application screens, service-dependent dialogs, maps and live workflows are not fully covered.

`npm run build-storybook` produces `storybook-static`, suitable for static hosting, including at a subpath. CI builds it and uploads a **storybook** artifact on frontend pull requests. Extract the artifact and serve it over HTTP; opening `index.html` directly from disk is unsupported. A stable hosted review URL is recommended for non-developers; this change prepares the artifact without choosing or changing production hosting.

After building, run `npx playwright install chromium` once and `npm run test:storybook` to render every story and check dialog keyboard behavior. The Accessibility panel provides additional checks; existing accessibility findings are not treated as a clean bill of health or a new CI gate.

### Adding examples

- Import the real component in a colocated `*.stories.tsx` file. Use typed `Meta` and `StoryObj`, named states and plain-language descriptions. Search this catalogue before introducing a new control.
- Cover default, selected, disabled, empty, error and long-text states where relevant. Use `fn()` from `storybook/test` for callbacks so reviewers can inspect Actions.
- Use `StoryStore` for components that require the operational Zustand store. Each story gets a new real store backed by a client whose connect/reconnect are disabled and whose send action is recorded. It does not simulate backend responses. Add explicit fixtures for server-dependent states; never mount the live session provider.
- Keep time-sensitive fixtures deterministic. Operational strip dimensions use viewport units, so compare them at the intended controller resolution; the browser checks use 1920×1080.
- Storybook has its own Vite config so the application's PWA and MDX pipeline do not run in the catalogue. Shared CSS and aliases are applied in `.storybook/main.ts`; only preview scrolling is overridden.

### Implementation review

The existing split between `components/ui`, reusable strip cells and the `FlightStrip` dispatcher is a useful catalogue boundary. Keeping stories attached to those production exports makes visual changes visible without maintaining a second implementation. Composed examples show how primitives fit together. The real store with an inert transport preserves selectors and initial state while avoiding authentication and websocket connections. Additional AMAN, EST and dialog coverage should follow this pattern as those areas change; avoid broad module mocks that can hide integration errors.

This template provides a minimal setup to get React working in Vite with HMR and some ESLint rules.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Babel](https://babeljs.io/) for Fast Refresh
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/) for Fast Refresh

## Scope strip controls

Planned-departure callsigns in the SAS, OTHER, NORWEGIAN, and CLR DEL bays use
slightly smaller text. Command-bar runway-pair buttons use a one-pixel outline.

During a handover, flight strips cannot be selected or moved. Clicking the SI
field or callsign opens the callsign menu for RECALL or FORCE ASSUME as permitted
by ownership. Tag requests retain their existing interaction behavior.

Cleared-departure TOBT has a red fill only while it differs from TSAT by more
than five minutes; the fill clears once it returns within that limit. Planned
departures keep neutral TOBT/TSAT fields. EOBT has no conditional fill.

### RWY ARR landing clearance

Moving an aircraft into an empty RWY ARR bay gives it a green runway field.
If another flight or tactical strip occupies the bay, the entering aircraft's
runway field is red. Existing green clearance stays green when another strip
arrives. Clicking green removes the clearance and fill; clicking again restores
landing clearance if no other RWY ARR aircraft is green. The same restriction
applies when clicking a red runway field. Leaving RWY ARR clears the status.

## EFB flight source and login

The EFB loads the authenticated pilot's online flight or prefile directly
from the public VATSIM v3 feed, matching the verified VATSIM CID in the existing
Auth0 login. It does not require a FlightStrips strip, controller, EuroScope
connection, or active session. Supplied callsigns cannot redirect flight reads
or TOBT writes to another account. The callsign input is hidden for this mode.
An owned prefile is shown as offline with a flight plan; it is not treated as online.

Enable `ENABLE_EFB=true`, `ENABLE_VATSIM=true`, and ATIS polling on the backend.
While EFB is enabled, the shared VATSIM cache polls independently of EuroScope
sessions. The browser refreshes its flight every 15 seconds. Failed or stale
VATSIM data is reported as unavailable rather than as a current flight.
Auth0 must use a connection providing real VATSIM identities for real-account
testing; a development VATSIM identity is not interchangeable with a real CID.

FlightStrips is an optional stand and ATC-assignment provider. Missing strips display TBA;
stand lookup failures show a warning without hiding the VATSIM flight.
PDC and other controller-only information are not fabricated from flight-plan
data. This airport-specific EFB treats flights originating at its configured
airport (EKCH by default) as departures and flights destined there as arrivals.
Flights unrelated to that airport receive an explicit unsupported-flight message.
Matching departure strips also provide PDC availability, request/clearance state,
and the clearance text; without a matching strip PDC remains unavailable.
The same airport restriction is enforced on TOBT writes before any timing access.
EKCH departure TOBT edits require a matching LIVE strip and the authoritative
local CDM workflow; missing CDM or strip data disables editing rather than
performing a remote-only update that the master would later overwrite.

### Live deployment

`docker-compose.prod.yml` enables EFB by default, retaining `ENABLE_EFB=false`
as an explicit rollback switch. It pins callsign lookup and Sweatbox training off.
The existing authenticated `/efb` route and reverse-proxy SPA routing serve
`https://flightstrips.dk/efb`; no controller role is needed, but VATSIM login is.
Preview controls are development-build-only and do not appear in production.
Apply the `0056-efb-departure-stand-workflow.sql` migration before starting the
new backend (the standard production migrator does this). Stand writes remain
gated by the existing stand-assignment feature flag and its readiness checks.
Keep the terminal AIRAC configuration current: inferred arrival guidance is
suppressed outside its validity period, with an explicit warning.
Configure the ChartFox OAuth callback as
`https://flightstrips.dk/efb/chartfox/callback` for the deployed ChartFox client.
The existing VATSIM/Auth0 connection must provide the verified pilot CID.

### Expected arrival fallback

An actual FlightStrips STAR/runway assignment takes precedence. Without one,
the final filed waypoint may identify TUDLO, TESPI, MONAK, TIDVU, or ERNOV.
Trailing DCT, destination, and speed/level tokens are ignored; an unknown final
waypoint is not replaced by an earlier recognised fix. The current arrival ATIS
must declare one unambiguous arrival/runway-in-use/ILS runway; condition reports,
departure-only declarations, stale ATIS, and multiple runways are not guessed.
This returns a family only, never an invented STAR version. The tile labels it
`ROUTE / ATIS`; both briefing and charts explicitly say it is expected guidance,
not an ATC clearance, and charts require confirmation of the procedure version.
Published terminal fix, holding and heading use that family
and runway. Unsupported or unavailable briefing data no longer defaults to
TUDLO/04, and taxi-to-stand guidance is omitted while the stand is unassigned.
The illustrated arrival briefing covers 04L/22L and the departure briefing
covers 04R/22R. Other or unknown runways show an explicit unavailable message
rather than substituting diagrams and taxi instructions for a different runway.

Departure M3 prefers SID and runway from a matching LIVE FlightStrips strip (same callsign, origin and
destination). Without a matching strip, its SID indicator is the first filed
waypoint (ignoring DCT and initial speed/level tokens) and its runway comes from
an explicit runway-in-use/departure declaration in current departure ATIS.
Runway condition reports alone are not used as assignments. For EKCH the published
initial climb is FL70 on 22R/22L/04R/04L and 4000ft on 12/30, exposed separately
from any actual cleared altitude. SID families NEXEN, LANGO, KOPEX, SALLO, SIMEG,
and KEMAX use departure frequency 124.980; ODDON, GOLGA, and VEDAR use 120.255.
Both bare waypoint indicators and full SID designators (e.g. GOLGA1A) are supported.
These rules do not depend on controller online status or an identified strip.
Departure M3 uses the arrival information tile's underlined headings around
the runway graphic: Initial Climb, Departure Freq, CTOT, and SID, with the
departure runway centered beneath the graphic. Unavailable values display NIL.
Unknown runways/SIDs remain unavailable rather than guessing. Filed cruise altitude
is never substituted for a clearance. CTOT remains sourced from vIFF.

## EFB arrival preview

Arrival terminal fix, published heading, and holding follow the assigned STAR
family and runway using the checked-in terminal configuration. This read-only
configuration is loaded when EFB is enabled, even without the optional navigation
service. Active navigation geometry can supply the published holding where
available; missing geometry does not discard the configured AIP details.
Holding fix and instructions share a small inset from the holding icon, with
instructions tucked beneath the fix. The published heading sits similarly close
under the terminal fix, preserving its alignment with the procedure icon.

For local evaluation, set `EFB_CALLSIGN_LOOKUP=true` on the backend. The callsign
field then reads any online or prefiled plan from the same VATSIM feed, rather
than matching the login's CID. A banner identifies the selected flight as ONLINE
or PREFILED and READ ONLY. Available vIFF timing and ATIS remain visible, but
TOBT and stand writes are rejected server-side for the entire lookup mode.
PDC action flags are cleared in lookup responses, and the frontend checks both
read-only mode and the PDC capability before requesting, acknowledging or
rejecting a clearance, including when permissions change with the dialog open.
The flag defaults to false and the backend refuses it in production.
Login is still required; no real-account credentials are needed to view another
callsign's public plan in this local mode.

### Sweatbox

Set `EFB_TRAINING=true` alongside `ENABLE_EFB=true` on a non-production backend.
The EFB data-source selector lists active sessions named Sweatbox, including
airport and session ID. Start EuroScope Sweatbox connected to this backend, select
its session, enter a training callsign, and press LOAD. The session list refreshes
every 15 seconds. The selected source and callsign survive browser reloads.

Training flights come directly from the selected session's FlightStrips strips;
they need not be online or prefiled on VATSIM. Timing comes from that session's
local CDM data. TOBT updates use local CDM and stand selections use the existing
session-scoped allocator, compatibility checks, and assignment versions.
Stand selection also requires `ENABLE_STAND_ASSIGNMENT=true` and valid stand,
airline-policy, and aircraft-reference configuration on the local backend.
Click the Stand tile, select an available (green) stand, and press
REQUEST NEW STAND. The selection is saved only in the selected Sweatbox session;
occupied or blocked stands remain disabled. Unsuitable stands can be selected
after confirming "This stand is not suitable for your flight. Select anyway?"
The confirmation bypasses suitability only, never reservations or controller
and neighboring-stand blocks; the server rechecks these when saving.
Canceling or dismissing the suitability warning restores the previous stand
selection and returns to its map without sending a request.
The stand dialog preserves the airport map and stand coordinates, omitting
B3, B5, B16, A50, and C26 and identifying the former E26 position as E36. Available
stands use a full green fill; reserved stands show the reserving aircraft
callsign(s), each on one line, on a red fill. The dialog and map span the screen
width and fit the available height without scrolling, with a check mark only
on the selected stand. Unsuitable stands are gray and labeled NOT SUITABLE in
the legend; blocked stands use a darker fill. Restrictions include their reason
in the stand tooltip. The header shows current/selected stands and availability,
and selections are highlighted with a cyan outline. Escape or CANCEL closes
the dialog without assigning a stand.
The bottom-left CARGO button switches to the cargo stand map (G110-G114,
G117-G137, and G15-G19); APRON returns to the main map. Switching maps retains
the selected stand and uses the same availability and confirmation rules.
Flights already assigned a cargo stand open directly on the cargo map.
The compact header has no visible subtitle; surrounding padding is reduced
by 20% to leave more vertical space for both maps.
Neither training reads nor writes use the EFB vIFF client, and CDM does not
export non-LIVE sessions to vIFF. PDC is disabled. ATIS remains the live airport
broadcast, not simulated Sweatbox ATIS.

A training banner identifies the source. Missing/stopped sessions never fall
back to LIVE, and the server rejects LIVE/playback session IDs on training routes.
Training is disabled by default and cannot be enabled in production. Switch the
selector back to VATSIM to resume the existing live/read-only callsign mode.

In local development, open `/efb` and select PREVIEW ARRIVAL to simulate a
read-only arrival. The On ground checkbox is available in the flight form and
preview banner. Flight form details and the ground checkbox are automatically
saved in this browser across reloads and preview resets. Ground speed can be
entered in knots; the checkbox sets a convenient 0 (ground) or 450 (air) knots.
Arrival timing is inhibited strictly above 60 knots: TOBT, TSAT, and CTOT
retain their values with grey fills, and the disabled update bar reads
"INHIBITED IN AIR". At 60 knots or below, normal timing colors and permissions
return. Charts, briefing, ATIS, and stand controls are no longer ground/air
inhibited. Crossing the threshold closes an open TOBT dialog.
Online flights use the VATSIM ground speed included in the flight API.
Missing speed (including Sweatbox snapshots without motion telemetry) does not
invent an airborne state; existing write permissions still apply.

## EFB tile information

Each of the six main tiles is 32-44 pixels shorter, leaving a round information
button floating above its top-right corner on the page background. There is no
separate header, title, or divider. Tile widths and bottom edges stay unchanged
and the button does not overlap tile data. Clicking the empty space above a
tile does not trigger the tile action.
The information button opens a tile-local panel with a 50 ms expansion/fade from that corner.
The panel matches the tile's full outer dimensions, with sharp corners and the
same dark background. The round close button replaces the information button
in its original position above the tile.
The information button has a thin 1px orange outline; the panel and close button retain a 2px orange outline.
Reduced-motion preferences disable the animation. Close it with the
close button or Escape; focus returns to the information button. Hovering the
information button or panel does not activate the tile's hover effect. Opening or
closing help does not trigger the tile's normal action. Help remains available
while timing updates are inhibited, without enabling them. The
arrival panels explain stand requests, ATIS reporting, CTOT compliance and TOBT,
arrival preparation, STAR/holding/heading checks, and chart access. Paragraphs
have subtle divider lines. Departure panels explain assigned stands and reservations,
TOBT/startup timing, viewing ATIS before PDC, chart access, departure preparation,
and SID/initial climb/CTOT checks with the pilot-initiated Departure frequency change.

## EFB stand assignment

Arrival stands come from the active FlightStrips stand assignment, including
its ESTIMATED, ASSIGNED, or CONFIRMED state. The EFB does not substitute an old
strip stand when no active arrival assignment exists. Missing, expired, or
released arrival assignments display TBA.

Confirming a manual arrival stand selection in either the EFB or controller
interface saves it as ASSIGNED and broadcasts the committed change. Manual
override selections use the same state. Compatibility, occupancy, and version
checks still apply. The authenticated departure workflow below is separate.
The read-only simulated arrival continues to use its saved stand field;
leaving that field empty previews TBA without modifying FlightStrips.

Authenticated departure stand assignment requires an existing matching LIVE
airport session, its flight strip, and the enabled FlightStrips stand allocator.
Otherwise the stand tile displays STAND ASSIGNMENT UNAVAILABLE without reserving
anything. Reading the flight alone does not allocate: the EFB explicitly sends
`POST /efb/stand/open` on first opening the flight. The operation is idempotent
and persists across page refreshes and backend restarts.

- Offline prefiles receive a compatible airline/fallback-policy stand, blue
  ASSIGNED, held for exactly 15 minutes. Manual selection moves the reservation
  and displays blue SELECTED with a fresh 15-minute hold. Refreshing, reopening,
  feed updates, and selecting the same stand do not renew the hold.
- Expiry frees the stand but retains its old number with gray EXPIRED. A new
  explicit selection can start a fresh hold.
- Online aircraft follow their VATSIM position, even before EuroScope sees them.
  A free detected stand is green CONFIRMED. No detected stand is gray
  NO STAND DETECTED and releases the previous reservation.
- A reserved/occupied physical stand produces red MISMATCH and a compatible
  alternative stand. After five continuous minutes at the same conflicting
  stand, an arrival reservation can be overwritten: green OVERWRITTEN. The
  existing arrival relocation handler then assigns the displaced arrival
  another compatible stand. Moving away cancels the timer; changing physical
  stands resets it. Other departures and controller blocks are never overwritten.
  Disappearing from the feed cancels the timer and frees online occupancy;
  an offline prefile's original 15-minute hold is not extended or prematurely
  removed.
- Online departures cannot manually select stands. Initialization and allocation
  failures are reported without hiding the underlying flight. If no compatible
  alternative exists, the allocator rejects the operation; arrival-relocation
  failures use the existing controller diagnostic/logging path.
  Background pilot reconciliation treats normal allocation shortages like the
  ordinary departure lifecycle: they remain recorded in allocator diagnostics
  but do not prevent later departures or arrivals from being processed.
  Infrastructure errors still propagate.

Arbitrary-callsign evaluation remains read-only and never starts this workflow.
Sweatbox retains its existing isolated stand behavior, including SENSED display.
EKCH departure TOBT and TSAT prefer the matching LIVE strip's authoritative local
CDM data; CTOT remains sourced from vIFF. TOBT edits use the local CDM action,
which persists pilot confirmation, requests recalculation when needed, and
synchronizes vIFF through the existing master workflow.
The same current-UTC time picker is used for both departure and arrival.

For an account test, the backend must run the updated code with `ENABLE_EFB=true`
and METAR/ATIS polling enabled. Open `/efb`, exit the simulated preview, and use
an account connected to VATSIM with a filed flight plan.

## EFB briefings

Arrival and departure briefings share a rounded image-and-notes layout with
circular page shortcuts grouped between labelled previous/next controls. Left/right arrow
keys also navigate; navigation wraps at either end. Escape or CLOSE dismisses
the briefing, and keyboard focus stays inside it while open.
Subtle dividers separate the briefing text paragraphs.
Each page change gently fades and slides horizontally over 240 ms: forward pages
enter from the right, and backward pages enter from the left. This animation is
disabled when the device requests reduced motion. Briefing images and operational
content are unchanged.

## EFB ATIS

Departure tiles place stand at L2, CDM timing at M2, ATIS/PDC at R2,
charts at L3, departure information at M3, and briefing at R3.
Arrival tile positions are unchanged.
Departure M2 uses the same timing tile design as arrival L3: three columns for
TOBT/TSAT/CTOT, the stand guidance image, and an UPDATE TOBT button. The button
is disabled when updates are unavailable or the callsign lookup is read-only.
Missing CTOT displays NIL on the EFB green fill. After a successful TOBT update,
the TOBT value box and action bar turn green and the bar reads TOBT CONFIRMED; it remains clickable for
further edits. Confirmation applies only to the updated flight, source, and TOBT.
When TSAT is more than six minutes overdue, both TOBT and TSAT value boxes turn red,
overriding the TOBT confirmation fill. CTOT turns green from six minutes before
until eleven minutes after its UTC time (inclusive). Time comparisons handle midnight.
TOBT always shows single-line startup-call guidance; TSAT and CTOT show their
respective startup/takeoff guidance only when their times are displayed.

Live-flight ATIS uses the backend's existing VATSIM AFV ATIS feed (including
vATIS broadcasts). EKCH arrivals select `EKCH_A_ATIS` at the destination;
departures select `EKCH_D_ATIS` at the origin. The backend refreshes its feed
every two minutes and the EFB refreshes flight data every 15 seconds.

The simulated arrival now reads its destination's arrival ATIS directly from
the public VATSIM feed, refreshing every minute without requiring a live
flight or backend login. It does not send credentials or simulated flight
details to VATSIM. The tile and ATIS dialog show the real letter and full text,
not a sample. Offline stations and feed failures are shown explicitly; an
ATIS update older than five minutes is marked STALE. ATIS remains accessible
on the ground and in the air.

## EFB vIFF timing

The EFB reads available timing from the server-side vIFF integration by callsign.
EKCH departures prefer authoritative local TOBT/TSAT from a matching LIVE strip
and must use local CDM for edits. Without that strip, vIFF timings may still be
viewed, but editing is disabled. Arrivals from other airports continue to read
and update their origin's timing through vIFF without requiring an EKCH strip.
Requests use the authenticated pilot's callsign rather than a supplied callsign.
The backend requires its existing `CDM_KEY`; no vIFF credentials are sent to the browser.

Sensed departure stands use the EFB green fill for both the stand number and status.
PDC uses a centered, rectangular text-only button with a single-line fitted label;
it aligns with UPDATE TOBT at the bottom of its tile, using the same height,
border, and text sizing. Existing status colors and request/view actions are preserved.
PDC, TOBT, and STAND bottom bars share a square-cornered inset outline and subtle
raised shading, while retaining their status fills and existing interactions.
The ATIS area uses the same bevel and edge outline.
Both arrival and departure STAND headers use the same bevel and enabled hover
feedback as the stand-status bar. Clicking either opens the same stand menu,
with keyboard access through the STAND header button and existing permission checks.
Push bars have a subtle bevel and local hover/press feedback only on enabled
clickspots; ATIS also has local feedback. Departure M3 stays static, while L3 and
R3 retain full-content hover feedback. On arrival, only Charts (L2) and Briefing
(R2) have full-tile feedback; ATIS and the enabled stand action bar have local
feedback. Charts and briefing feedback animates only the resized tile content;
the information button and the empty margin above it stay stationary and do not
trigger that animation. Information help suppresses whole-tile effects.
Arrival ATIS keeps its hit area stationary and animates only the beveled
identifier, leaving the weather and APP reminder fixed.
ATIS identifiers are blue until opened and green once the current message has
been viewed. A changed letter or message returns them to blue; unchanged refreshes
do not clear acknowledgement. This applies to arrival, departure, and preview,
scoped to the selected flight and source. Stale or missing ATIS is never green.
DOWNLOADS and TRAFFIC
BOARD use the shared bevel and hover/press feedback. Filled TOBT, TSAT, and CTOT
fields also use the bevel; unfilled black/transparent fields stay flat.
Three transparent weather widgets between departure ATIS and PDC, and beneath
the arrival ATIS header, show wind strength,
temperature (Celsius), and QNH (hPa) from the displayed ATIS text. They refresh
with ATIS and show N/A for unreported or unrecognized fields, rather than
substituting weather from another source.
Clicking arrival ATIS (including its widgets) opens the same ATIS dialog as
departure, with the arrival report. The report text appears only in the dialog,
not on the arrival tile.
The arrival ATIS bottom bar reminds pilots: "At First Contact with APP, Stats
A/C Type & ATIS Info", aligned with the stand status bar in larger, regular-weight text.
The arrival STAND header has the same bevel as ATIS and opens stand selection
when stand reassignment is available.
Wind includes reported gusts and a down-arrow rotated relative to the assigned
runway: subtract 40 degrees for 04L/04R or 220 degrees for 22L/22R, wrapping into
0-359 degrees. Other or unknown runways retain the north-referenced direction.
CSS rotation uses explicit degrees, not radians. Variable or missing
directions use a non-directional wind icon; variable winds are labeled VRB.
The departure ATIS/PDC tile keeps ATIS above PDC; missing ATIS blocks PDC submission
with an explanation without changing the request label to ATIS.

The TOBT menu matches its timing tile's exact bounds, square corners,
and background, without an outer outline (M2 for departures, L3 for arrivals).
UPDATE TOBT opens a UTC time picker initialized to the current time, with current
TOBT and CTOT above the input, quick-set buttons, and side-by-side Update/Cancel actions. The +5/-5 controls
adjust it by five minutes, including across midnight. Confirming submits the
new TOBT through authoritative local CDM for EKCH departures, or to vIFF using
the origin flight's taxi time for arrivals; errors leave the menu open.
If vIFF is unavailable, the flight remains visible with an explicit warning.
Available local departure timing remains editable when local CDM is ready;
remote timing edits are disabled.
The simulated arrival can open the picker, but submission is disabled.

## Fullscreen and scope controls

The station box opens the scope selector. The fullscreen button to the left of
the up-arrow toggles fullscreen in supported browsers and installed PWAs.
Press Esc or click the button again to exit. The button is disabled when the
browser does not support fullscreen; rejected requests display an error.

The up-arrow reveals additional scopes. TOWER EAST and TOWER WEST are disabled
placeholders beside APRON DEP and APRON ARR until those scopes are available.
EST (NEW WINDOW), to the left of APRON DEP, opens `/app?layout=EST` in a new
window without changing the current scope. Allow popups for FlightStrips to use
this button. The new window selects EST on startup and retains it on reconnect.

## Browser visual tests

Install Chromium once after installing the frontend dependencies:

```sh
npx playwright install chromium
```

Run the browser-rendered checks headlessly:

```sh
npm run test:visual
```

Open Playwright's interactive UI to see and debug the rendered fixture:

```sh
npm run test:visual:ui
```

Successful tests attach a screenshot to the HTML report. Open it with:

```sh
npm run test:visual:report
```

The EST fixture can also be inspected without the backend. Start `npm run dev`, then open
`http://127.0.0.1:8080/visual-tests/est-preview.html`.

## Expanding the ESLint configuration

If you are developing a production application, we recommend updating the configuration to enable type-aware lint rules:

```js
export default tseslint.config({
  extends: [
    // Remove ...tseslint.configs.recommended and replace with this
    ...tseslint.configs.recommendedTypeChecked,
    // Alternatively, use this for stricter rules
    ...tseslint.configs.strictTypeChecked,
    // Optionally, add this for stylistic rules
    ...tseslint.configs.stylisticTypeChecked,
  ],
  languageOptions: {
    // other options...
    parserOptions: {
      project: ['./tsconfig.node.json', './tsconfig.app.json'],
      tsconfigRootDir: import.meta.dirname,
    },
  },
})
```

You can also install [eslint-plugin-react-x](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-x) and [eslint-plugin-react-dom](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-dom) for React-specific lint rules:

```js
// eslint.config.js
import reactX from 'eslint-plugin-react-x'
import reactDom from 'eslint-plugin-react-dom'

export default tseslint.config({
  plugins: {
    // Add the react-x and react-dom plugins
    'react-x': reactX,
    'react-dom': reactDom,
  },
  rules: {
    // other rules...
    // Enable its recommended typescript rules
    ...reactX.configs['recommended-typescript'].rules,
    ...reactDom.configs.recommended.rules,
  },
})
```
