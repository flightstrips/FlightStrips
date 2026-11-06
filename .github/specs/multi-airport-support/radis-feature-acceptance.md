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
| Strip QNH click | `StripAnnotations.QNH` from shared selected-airport METAR | #1013/#1016: server source stamp, unavailable weather has no invented fallback |
| Callsign, origin/destination, aircraft, wake, route segments, RFL, EOBT/EET, registration, date/direction | Existing strip flight-fact projection | #1013: stable strip ID, airport direction; no browser-feed authority |
| FIND / PLANNED dialog, count, select | Connected lookup/admission request, stable strip ID | #1014: connected pilots only, from OR to selected airport, no prefiles/distance filter/cap; stale selection revalidated |
| NEW manual form, callsign lookup/entry | Local creation request, `StripAnnotations` seed | #1014: shared local row, normalized duplicate callsign blocked/idempotent, no ES FPL creation |
| Open existing strip by callsign | Select existing ID / explicit restore action if hidden | #1014/#1015: never create a second identity or bypass restoration policy |
| X drag delete; refresh merge | Removal action and durable visibility state | #1015: shared tombstone survives sync, feed refresh, reconnect and restart |
| Restore (confirmed addition) | Restore action referencing original ID/revision | #1015: configured recovery limit, original bay/order/annotations retained, no ES FPL replace/delete |
| Airport selector | Host authorized session discovery / tab binding | Existing MA-06 + #1013: EKBI/EKYT share view, not board; only authorized airports shown |
| DEP/ARR command-bar runway selectors | Existing scoped active-runway update | #1016: session setup shared, no ATIS-derived browser mutations |
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

Each implementation PR adds concrete test references here. Full acceptance also
requires plugin execution/result-path, DB persistence/concurrency, two participant
browser, own disconnect and airport/environment isolation checks. Operational
acceptance with real data remains the MA-08 activation gate, not a reason to omit
the implementation or claim a synthetic fixture is operational approval.
