# TMA release preview stability, 15 November 2026

Investigated from main `bbe9c34e` in a dedicated worktree. The manual-test
worktree and its running services were read-only evidence. No production
configuration or live scenario was changed.

## Disappearing EZY601 preview

The saved query output at 17:03:29 UTC describes the prediction generated at
17:03:16.6624089 UTC. EZY601 had physical RawTETA 17:29:54.088462627, landing
slot 17:35:20, and baseline release 17:16:47.748946267. Its provisional release
was 17:14:18, allocating 149.748946267 seconds to approach. This was a
forecast, with no accepted admission. The ready/empty buffer described the
two-minute admission window, not the longer preview's forecast occupancy.

RYR25K was the preceding LUGAS aircraft. At that generation its raw estimate
was 17:27:24.627694846, before its 17:27:50 slot, and it had a positive holding
plan and a preview. In the next sampled generation, 17:03:46.6610229, its raw
estimate was 17:28:34.245276357, after the same slot. Its holding plan was
therefore correctly absent. EZY601's slot and route identity remained intact,
but its preview was absent.

The release planner treated any older aircraft with a stream-entry estimate
and no reservation as an unresolved head. An ordinary unheld leader without
slot delay was ineligible for a holding release, could never acquire a
reservation in that sweep, and consequently blocked the following preview.
Every refresh clears previews first, so no older forecast survived this
classification. The same issue could block imminent TMA admissions. A
regression with these rounded timings fails against the original production
functions exactly when the leader crosses its slot, and passes with the fix.

This establishes the stream-blocking reason for EZY601 in the sampled
transition. The earlier full airport inputs and outbound websocket frames
were not recorded; it does not establish the reason for every other absent
preview or reconstruct the screenshot's generation.

## Earlier and conservative planning

The old preview window was twelve minutes: admission lookahead plus maximum
advance. It also counted every predicted arrival up to the end of that window
before planning any release. A moving horizon could therefore remove an
earlier opportunity when later arrivals crossed its far boundary.

The preview now considers prospective releases thirty minutes ahead,
independently of the unchanged commitment window and advance allowance.
Thirty minutes provides enroute notice well before the old boundary, without
making an unbounded forecast or extending an admission commitment. Capacity
is evaluated at each candidate release, including at least the commitment
window. It is checked again if taper or stream spacing postpones the release.
Each aircraft counts once; current approach traffic and released admissions
remain counted, and no future landing is assumed to free capacity. A
provisional later release cannot remove an earlier predicted arrival.

After assigning forecasts, recheck every provisional release against the
complete forecast intake until its capacity allowance and stream spacing are
stable. A later-processed independent stream can release earlier and reduce
another forecast's advance allowance. Propagate any postponement through the
original stream order, rechecking rate periods and incoming traffic, and omit
forecasts pushed beyond the planning horizon. Accepted admissions are not
changed by this forecast-only pass.

Fresh unheld leaders with no slot delay now contribute predicted stream
passage and spacing rather than becoming blocked holders. Passage is never
treated as observed movement or stored as an admission. Imminent passage
counts as incoming workload. Manual protection, actual holding, unknown
workload, stale prediction, lost route/holding timing and full forecast
capacity still withdraw or suppress provisional absorption. Previews are
recomputed against current inputs; they are not sticky promises.

## Gain/loss and transport

The existing formula is retained: smoothed physical prediction minus the
effective guidance target. The target excludes only usable planned approach
absorption, capped at the remaining physical-to-slot loss. The smoothing
window can differ from the latest raw estimate. Landing order alone does not
determine gain/loss: transit, physical ETA, stream constraints and available
approach absorption differ between aircraft.

Using the sampled EZY601 raw estimate and its 149.749-second preview gives
about -176 seconds, displayed as L03, rather than the unadjusted -326 seconds.
Without the matching smoothing samples this is a raw-reference calculation,
not an exact reconstruction of the published value. The screenshot's `=00`
cannot be validated against a different generation. No separate arithmetic or
presentation defect was established; the web and plugin format the published
seconds rather than recalculating landing drift.

A read-only copy of the later persisted revision 31, generated at
17:06:16.6639849 UTC, was replayed locally through the real EKCH terminal
configuration and TMA boundary. Both event projections agreed for all fifteen
slotted aircraft. With that snapshot's smoothing samples, the original
persisted EZY601 guidance was -330 seconds. Refreshing the TMA forecast with
the fix gave a 17:11:20 provisional release and -1 second in both feeds. This
is a later snapshot projection, not a replay of the unrecorded earlier
surveillance or a claim that every arrival should receive zero loss.

Optional forecast timestamps now accompany applied provisional absorption in
the web and EuroScope feeds. Both displays prefix that guidance with `~` so
an enroute controller can distinguish a forecast from a committed release.
The web hover gives the forecast release in UTC. The numeric guidance remains
backend-owned. Complete websocket replacements clear metadata when a forecast
is withdrawn or becomes an admission. EuroScope's holding countdown continues
to use the separate calculated EAT. Older plugins can ignore the additive
protobuf field; displaying the marker requires the updated plugin.

## Scope and verification

Changes are confined to TMA intake forecasting, its stream predecessors and
its guidance presentation. Runway sequencing, landing slots, holding priority,
physical prediction and smoothing, clearances, writeback and deployment
configuration are unchanged. No part of the original investigation request
was removed or deferred to another chat.

Automated coverage includes repeated leader ETA crossings and persisted-state
reloads, protected/held/stale withdrawal, pre-release capacity versus later
incoming traffic, spacing crossing an arrival boundary, early enroute notice,
unchanged slots and physical facts, matching web/EuroScope values, forecast
metadata clearing on commitment, and web/plugin forecast presentation.
Additional regressions cover later-processed earlier releases, cascading
capacity/stream postponements, unsorted rate points and forecasts moved out
of the planning horizon, with input immutability and permutation checks.

Validation completed:

- Full backend `go test ./...`, using a separate migrated PostgreSQL test
  server through the existing test database environment override. The default
  testcontainers startup failed on Windows; no test infrastructure was changed.
  The temporary server was stopped after the successful run.
- Focused release, operational and both event-projection packages after final
  changes, including the regression against the original production functions.
- 123 focused frontend API, target and store tests; AMAN contract checks;
  changed-file ESLint; TypeScript and production frontend build.
- Debug plugin build and all 579 CTest tests, including forecast transport,
  tag presentation, replacement clearing and independent holding countdown.

The frontend build reports existing CSS-template and bundle-size warnings;
the plugin linker reports its existing runtime-library conflict warning. Both
builds succeed. No deployment or simulator restart was performed.
