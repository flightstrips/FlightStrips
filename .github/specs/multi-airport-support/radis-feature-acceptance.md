# RADIS production interaction contract

Tracks #1011 and epic #979. Source inspected at PR #958's reviewed commit
`dba4efc9d4f6d095074c182ebdb50540c7818884` and current head
`1a6b80c53fd670c317f509b2f7f3a5b0d373c8d3` (route packaging only).
This is the acceptance inventory, not approval of prototype operating values.

All human mutations use the selected board context and exact own membership,
including local annotations, NEW, delete and restore. Observer, disconnected,
unsynchronized and stale generations cannot write. Strip actions carry
`StripRevision` (ID, version, definition revision). Backend snapshots and updates
are authoritative; two participants converge and two airports remain isolated.

Paper-strip commands use shared semantic events (`strip.annotate`,
`strip.create-local`, `flight.find-connected`, `strip.admit-connected`,
`strip.remove`, `strip.restore`). Each operation is authorized by the bound
handler's registered typed descriptor. The handler decides placement and
visibility; shared services own authority, CAS, source I/O, persistence and
publication. A second coded `example-paper` workflow exercises the same transport
and storage with different bays, annotation support, ownership and recovery.
No shared dispatcher selects behavior from a RADIS view ID.

| Implemented prototype interaction | Production typed action/data | Acceptance case / owner |
| --- | --- | --- |
| ACTIVE, RUNWAY, PASSIVE_TOP, PASSIVE_BOTTOM | `MoveEvent` / `airportworkflow.Move` with configured bay effect | #1012: four independent bays, shared ordered moves, own-plugin dispatch failure/reconciliation |
| Drag between bays; insert/reorder in a bay | Existing move and `StripRef` insertion path | #1012: concurrent ordering, no ownership, reconnect/restart order |
| Route menu: numeric heading, POINT, manual point, OK/ESC | `UpdateStripDataEvent.Heading` or `.Route` | #1013: actual ES command and observed result; ESC sends nothing |
| ALT menu and keyboard/presets | `UpdateStripDataEvent.Altitude` in feet | #1013: explicit units, valid associated ES flight, own-plugin execution |
| RWY menu, separate arrival runway cell | `UpdateStripDataEvent.Runway` | #1013: selected airport runways, host reconciliation; no hardcoded prototype runways |
| SID picker, erase | `UpdateStripDataEvent.Sid` | #1013: ES SID edit including empty value, no guessed procedure data |
| Squawk field | `AssignedSquawkEvent` command | #1013: four octal digits, exact own-plugin execution/result |
| RNAV NON/1 | `StripAnnotations.RNAV` | #1013: shared declared paper mark; does not silently amend ES equipment |
| VFR selection | `StripAnnotations.VFR` | #1013: shared paper mark; no implicit 7000 assignment or ES flight-rule change |
| Stand edit / automatic sensed stand | Existing ES stand update and backend SAT projection | #1013/#1016: selected airport data, shared override, one allocator |
| Remarks/notes edit | `StripAnnotations.Notes` | #1013: durable shared text; separate explicit ES remarks amendment |
| Flight-plan / clearance dialog and editable aircraft, route, remarks | Existing `UpdateStripDataEvent` fields | #1013: host dialog, production execution for actual fields; read-only flight facts displayed |
| REA toggle | `StripAnnotations.REA` | #1013: backend UTC stamp/clear; annotation-only semantics configured; never infer EOBT write |
| ATA toggle | `StripAnnotations.ATA` | #1013: backend UTC stamp/clear; independent of CDM ALDT |
| De-ice toggle | `StripAnnotations.Deicing` | #1013: shared mark, does not invent a deicing clearance |
| NUM ATD B5 and END/POB B6 | `StripAnnotations.ATD`, `.EndurancePOB` | #1013: shared validated paper values, separate from ES/CDM time/person fields |
| Strip QNH click | `StripAnnotations.QNH` from shared selected-airport METAR | #1013/#1016: selected-airport source, unavailable weather has no invented fallback |
| Callsign, origin/destination, aircraft, wake, route segments, RFL, EOBT/EET, registration, date/direction | Existing strip flight-fact projection | #1013: stable strip ID, airport direction; no browser-feed authority |
| FIND / PLANNED dialog, count, select | Connected lookup/admission request, stable strip ID | #1014: connected pilots only, from OR to selected airport, no prefiles/distance filter/cap; stale selection revalidated |
| NEW manual form, callsign lookup/entry | Local creation request, `StripAnnotations` seed | #1014: shared local row, normalized duplicate callsign blocked/idempotent, no ES FPL creation |
| Open existing strip by callsign | Select existing ID / explicit restore action if hidden | #1014/#1015: never create a second identity or bypass restoration policy |
| X drag delete; refresh merge | Removal action and durable visibility state | #1015: shared tombstone survives sync, feed refresh, reconnect and restart |
| Restore (confirmed addition) | Restore action referencing original ID/revision | #1015: configured recovery limit, original bay/order/annotations retained, no ES FPL replace/delete |
| Airport selector | Host authorized session discovery / tab binding | Existing MA-06 + #1013: EKBI/EKYT share view, not board; only authorized airports shown |
| DEP/ARR command-bar runway controls | Confirmed scoped ES runway activity; native master runway dialog | #1016 running-system runway observation; operational case O2 below; no ATIS-derived browser mutations |
| QNH/wind/ATIS/UTC command bar | Shared backend weather/observations and display clock | #1016: no new upstream/browser polling; unavailable values remain unavailable |
| Resize runway panel / responsive strip sizing | View-local layout only | #1013 browser acceptance: geometry adapts without operational mutations |

Prototype placeholders: FIND SORT has no handler; its counter is display-only;
DISMISS closes the dialog. Generic FPL menu placeholder is superseded by the
implemented flight-plan dialog. Command-bar QNH click is a no-op. Clearance OK
closes the dialog; it is not proof of issuing a clearance. No PDC, AMAN, ES FPL
creation, additional airports or service algorithms are inferred from labels.

Activation review must supply real bay-to-ES mappings, field conventions,
admission/removal distance/altitude/time values, recovery duration, timestamp
semantics, procedure/stand/CDM/text data and coverage. Synthetic fixtures must
be visibly marked and excluded from deployment selection. No ENGM duration,
prototype 150 NM/14,000 ft/500 ft/15 minute timing, ten-flight cap, fixed weather
or sample SID values are production defaults.

The concrete evidence below covers the six implementation increments. Full acceptance also
requires plugin execution/result-path, DB persistence/concurrency, two participant
browser, own disconnect and airport/environment isolation checks. Operational
acceptance with real data remains the MA-08 activation gate, not a reason to omit
the implementation or claim a synthetic fixture is operational approval.


## Concrete implementation evidence

The stack implements #1011–#1016. Paths below are repository-relative; fixtures
are explicitly selected by tests, never deployment configuration.

| Area | Automated evidence | Operational acceptance still required |
| --- | --- | --- |
| Typed configuration, bays, mappings and annotations | `backend/internal/config/airport_radis_test.go`; `backend/internal/airportworkflow/radis_test.go`; production loader/schema parity corpus | O1: controller review of real EKBI/EKYT bay/ES mappings, field conventions, admission/removal thresholds, delay/reappearance/recovery and timestamp policy |
| Shared board, order, concurrency, airport/environment isolation | `backend/internal/services/radis_test.go`: two authorities, stale CAS, restart reads, EKBI/EKYT/SWEATBOX; `frontend/tests/e2e/radis.spec.ts`: actual pointer inter-bay moves and reordering through app/DB, two EKBI browsers and independent EKYT | O1: validate actual ES ground-state effects and display geometry on controller equipment |
| Flight fields, numeric/route/SID/runway menus and squawk | `backend/internal/frontend/radis_fields_test.go`: exact own binding and typed command payload, missing/stale/disconnected context rejection; running-system RADIS test: slave's altitude command and accepted nonce echo, peer convergence, no EKYT change | O1: native SDK field/procedure acceptance with reviewed airport data; manual points remain explicit route amendments |
| Notes, deicing, REA/ATA, RNAV/VFR, ATD, END/POB, QNH | Service reducer/CAS tests in `services/radis_test.go`; `frontend/tests/visual/radis.spec.ts` shared notes/deicing; running-system shared notes; `store-radis.test.ts` disconnect fencing | O1: real operational conventions. REA/ATA/deicing remain paper annotations; no implicit EOBT/CDM/7000 amendment |
| FPL facts, EOBT/EET, date/direction and host dialogs | Running-system persisted real ES EET and field echo; `repository/postgres/strip_eet_test.go` reload/source precedence; real duration projected from shared ES/VATSIM source, without fixed taxi estimates | O1: verify registration, wake, UTC and duration conventions on real plans |
| NEW and FIND/admit | `services/radis_creation_test.go`: duplicate/concurrent creation, connected from OR to airport, 25 candidates, prefile/unrelated exclusion, stale revision/logon, wrong airport/environment; running-system local creation, shared-source FIND24/admit and airport isolation | O1: local-strip operating practice; local NEW never creates an ES FPL |
| Removal, recovery, refresh/restart | `services/radis_visibility_test.go`: durable manual removal, peer restore, expiry, source suppression, automatic policy; running-system remove/reload/restore preserves ID/bay/sequence/notes | O1: approved recovery/removal policy and automatic reappearance practice |
| ES reconnect restoration | `euroscope/radis_backend_sync_test.go`: local/planning/removed rows excluded, associated row retains observed ES state and scoped geography | O1: native ES reconnect with real associated plans; no FPL replacement/deletion |
| CDM, stands, text-pilot and weather | `app/radis_services_test.go` loads concrete service fixtures, shared aircraft references and separate service inputs; running-system RADIS test uses real CDM TOBT orchestration, SAT assignment, configured text submission/result, shared METAR/ATIS caches; `frontend/radis_publication_test.go` prevents stale strip revisions after incremental service/SDK notifications | O3: approved per-airport CDM/stand/procedure/text data; real SDK stand/frequency-message acceptance |
| Runway setup | Running-system master `RunwayEvent` changes both EKBI boards, leaves EKYT unchanged | O2: change DEP/ARR in the master EuroScope runway activity dialog and confirm the shared board. The bundled SDK exposes activity observation, no setter; no browser-only pretend selection or invented command is shipped |
| PDC/AMAN disabled | `pdc/capability_test.go` now uses the real RADIS descriptors and test-only regional service fixtures; `app/radis_services_test.go` rejects AMAN activation/commands/publication before dependencies | Remain disabled at EKBI/EKYT; no scheduled activation implied |
| Whole-board disconnected read-only, reconnect and stale drafts | `backend/internal/frontend/board_test.go` transport fence; `frontend/src/store/store-radis.test.ts`; Chromium/Firefox RADIS protocol tests retain dialog revisions; running-system own disconnect blocks NEW/FIND/restore/notes/service edits while peer updates and EKYT stay live, same-instance reconnect converges | O1: controller acceptance of retained read-only behavior |
| Figma/host integration | `frontend/tests/visual/radis.spec.ts` and running-system screenshots; selected local Figma section `2676:1558`, board `3313:371`, FPL `2676:2832`; source arrow assets documented in `frontend/src/routes/radis/assets/README.md` | O1: review at actual screen size/input devices. Runway area resizing remains view-local |

The running-system test starts the production `app.Build`, migrated PostgreSQL,
scoped ES/frontend WebSockets, shared sources, CDM/SAT services and normal built
frontend/router/providers. Its external identity/feed/SDK peers are fixtures;
the typed execution nonce and frequency-message result return through production
validation. This is stronger than the separate protocol-mocked UI tests, but is
not a claim that automated tests execute inside the native EuroScope application.

`backend/internal/config/testdata/radis-services/` supplies concrete typed EKBI
and EKYT fixtures with intentionally fictional geography, frequencies, S1/S2/S3
stands, mappings, rates, timings and static text. Shared aircraft references are
reused. Neither fixture is in production airport selection. O1–O3 remain the MA-08
activation evidence gate; implementation with fixtures does not activate airports.
