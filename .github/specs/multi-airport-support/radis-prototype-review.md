# RADIS prototype: evidence and integration implications

Reviewed on 2026-10-30. Source: [FlightStrips PR #958](https://github.com/flightstrips/FlightStrips/pull/958),
`feat: add FSRADIS prototype as standalone folder`, at head commit
`dba4efc9d4f6d095074c182ebdb50540c7818884`. This is a source review of an open
prototype PR, not an acceptance of its operating rules or a request to merge it.
The user identifies this as the likely basis for further RADIS work and explicitly
says the feed-only implementation without EuroScope updates was a demo shortcut.

## What the prototype establishes

| Observed behavior | Consequence for the proposal |
| --- | --- |
| Four board containers: `ACTIVE`, `RUNWAY`, `PASSIVE_TOP`, `PASSIVE_BOTTOM`. Initial departures enter the upper passive area and arrivals the lower one. Dragging supports both inter-bay moves and ordering within a bay. | Start the shared RADIS view from these interactions. Preserve distinct bay IDs and authoritative ordering. Production placement and movement effects still need airport configuration. |
| Strip menus cover route/heading, altitude, flight plan, runway, and numeric cells. Other interactions include SID, RNAV, VFR, remarks, REA/ATA timestamps, and a de-icing flag. | Translate UI interactions into semantic backend actions. Separate real flight-plan/ES fields from shared paper-style annotations; screen cell names such as U2/B5 are presentation details. |
| FIND searches feed pilots outside the normal distance filter; selecting a result adds it to a passive area. NEW creates a local strip, with callsign lookup and manual entry. | Distinguish finding an existing flight, creating an airport strip, and creating or amending an actual ES flight plan. A local strip alone does not establish an ES flight plan. Define identity and admission semantics before wiring these actions. |
| Deleting a feed strip hides its callsign locally. Manual/planned strips are removed from local maps. Refresh merges feed records while preserving existing bay membership/order and local strips. | Use session-owned visibility/removal and recovery rules. A refresh or another browser must not undo a shared deletion. Do not delete the canonical flight merely because its airport strip is removed. |
| Stand data are parsed from a GRplugin-style file. Assignment considers sensed position, aircraft/stand criteria, occupancy, and manual overrides. | Reuse the existing backend stand-assignment service and airport data. Retain the useful UI behavior; avoid two independent allocators deciding the same airport's stands. |
| Airport selection changes the displayed board; the menu contains EKYT, EKBI, EKAH, EKSP, and EKKA. Selection is saved in browser storage. | Adapt this to authorized session discovery and per-tab selection. Menu entries are demo evidence, not approval to add three more airports to the first release. |

Evidence: [App.tsx](https://github.com/flightstrips/FlightStrips/blob/dba4efc9d4f6d095074c182ebdb50540c7818884/FSRADIS/frontend/src/App.tsx),
[CommandDialogs.tsx](https://github.com/flightstrips/FlightStrips/blob/dba4efc9d4f6d095074c182ebdb50540c7818884/FSRADIS/frontend/src/CommandDialogs.tsx),
[standAssignment.ts](https://github.com/flightstrips/FlightStrips/blob/dba4efc9d4f6d095074c182ebdb50540c7818884/FSRADIS/frontend/src/standAssignment.ts).

## Prototype boundaries that must change during integration

- Board state, annotations, locks, hidden callsigns, and stand overrides live in
  browser memory. They do not provide the required shared board, persistence,
  reconnect snapshot, authorization, or concurrent editing across controllers.
- App.tsx polls the VATSIM pilot feed in the browser; CommandBar.tsx separately
  polls it for ATIS/runway information and obtains weather. Integrated views
  consume backend observations and shared source caches. Airport sessions and
  browser tabs must not create independent VATSIM feed acquisition loops.
- There is no ES write path. Actual flight changes and configured bay effects
  must use the initiating controller's own plugin, with edits blocked when that
  binding is unavailable. Read-only behavior applies to board annotations too,
  following the agreed whole-board disconnect policy.
- Generated array-position strip IDs and browser-only manual IDs are unsuitable
  as shared identities. Use backend flight identity plus independent session
  strip IDs, with idempotent creation/admission and revisioned mutations.
- The standalone React application must be adapted to the host view registry,
  session store, WebSocket, design conventions, and dependency versions. Keeping
  its visual interaction model does not require shipping a second application.

Evidence: [prototype README](https://github.com/flightstrips/FlightStrips/blob/dba4efc9d4f6d095074c182ebdb50540c7818884/FSRADIS/README.md),
[App.tsx](https://github.com/flightstrips/FlightStrips/blob/dba4efc9d4f6d095074c182ebdb50540c7818884/FSRADIS/frontend/src/App.tsx),
[CommandBar.tsx](https://github.com/flightstrips/FlightStrips/blob/dba4efc9d4f6d095074c182ebdb50540c7818884/FSRADIS/frontend/src/CommandBar.tsx).

## Values and semantics that remain unapproved

The demo uses a 150 NM filter, a ten-flight cap (also affecting FIND), 15-second
feed polling, a 14,000 ft admission filter for newly encountered departures, and
a 500 ft timestamp threshold. It estimates some timing from a fixed 15-minute
taxi value. None is adopted as an operational default. The feed lookup reads
pilots, not prefiles, so its "planned" results do not establish prefile support.
Its REA toggle changes a local timestamp; it does not demonstrate an EOBT update.
Fallback weather, fixed cells, sample SID/runway logic, and button labels do not
prove working CDM, AMAN, PDC, or complete flight-plan amendment support.

Airport-specific admission/removal thresholds, bay-to-ES mappings, timestamp
meaning, manual-strip data/flight association, runway/ATIS authority, and service
parameters remain open. Existing confirmed ownership, coverage, own-plugin,
and disconnect requirements take precedence over demo behavior.

## Confirmed feature scope — 2026-11-01

Include all current prototype features, notes/de-icing/REA/ATA annotations, and
restoration of manually removed strips. NEW is local-strip creation only; FIND
is restricted to connected pilots to/from the selected airport, with no prefiles.
Use the existing EKCH execution/error/reconciliation flow. PDC and AMAN remain
disabled at EKBI/EKYT without a scheduled activation date. The demo shortcuts
above still do not become target architecture or approved airport constants.

## Resulting architecture changes

Use PR #958 as the likely RADIS frontend starting point. Keep shared observations
outside airport runtimes, then project them into each airport's admission rules.
Store strip order, visibility, annotations, and overrides in the authoritative
session. Send semantic actions from the adapted view to backend workflow/domain
handlers; send real ES changes through the initiating plugin and reconcile their
observations. Never treat a successful local UI mutation as proof of an ES write.

The [coded-handler proof](handler-design.md) now exercises a typed shared boolean
annotation in the [fictional airport config](examples/zzzz.airport.yaml).
RADIS text, marks, and timestamp annotations remain shared strip data distinct
from canonical ES fields, but their complete declarations and semantics must be
defined with the real RADIS handler. The fictional example is not a RADIS spec.

Integration acceptance must include two controllers seeing the same mutations,
stable strip identity across reordered feed snapshots, reconnect convergence,
own-plugin write routing, and no browser/per-session VATSIM acquisition loops.
