# Event update performance audit — 16 September 2026

## Conclusion

Keep one position execution worker and PostgreSQL authoritative. The next opportunity is fewer repeated reads, narrower AMAN lookups, and cheaper publication. Increasing concurrency does not eliminate any of that work.

A few milliseconds is a useful target for a routine field update. It is not yet demonstrated for all updates, and bulk sync, real operational transitions and externally delivered PDC messages need separate budgets. Averages cannot establish tail latency.

This is an investigation, not a performance fix. No production configuration or dashboard was changed. Local checkout: 2d630668. The experimental position/AMAN batches in draft PR #760 are not represented by the production figures below.

## Measurement scope and limitations

- Production window: 2026-09-16 12:34:57–18:34:57 UTC (14:34–20:34 Copenhagen).
- Queried all event types returned by the EuroScope and frontend handler counters.
- Representative traces inspected here identify release 2.2.8+246871676f77ac828283c1eb668edf3401c6491c. The six-hour metric totals are not release-filtered.
- Counts are rounded Prometheus increase estimates; they are not exact accounting.
- Mean processing duration excludes dispatcher queue/barrier time. Socket waiting before receipt is also excluded. Trace samples below use message.processing_ms, not total root duration.
- Histogram buckets currently return aggregated source/type labels. Per-event P95/P99 cannot be recovered from those results. No per-event percentile claim is made.
- DB-operation counters only returned five event types; absent counters do not mean zero queries. Use actual query spans for the samples.
- Trace searches are bounded samples, not exhaustive distributions. SQL span duration includes client/database communication; it is not pure PostgreSQL execution time. Prepare spans nested within query spans must not be added again.
- Some synchronous paths use context.Background and detach their SQL from the message trace. Query totals are therefore observed work, potentially lower bounds. Residual time is not automatically CPU time.
- No new load test or PC CPU/memory measurement was run for this audit. Prior one-worker local results remain in position-performance.md (linked below).
- Sanitized metric results, query operation names and trace IDs: [event-update-audit.json](event-update-audit.json). No report payloads or client identifiers are included.

## All observed event types

| Source | Event | Approximate count | Mean processing ms |
| --- | --- | ---: | ---: |
| euroscope | sync | 3 | 256.31 |
| euroscope | aman_route_fact | 54 | 17.50 |
| euroscope | runway | 8 | 16.97 |
| euroscope | controller_online | 17 | 15.56 |
| euroscope | hold | 66 | 15.41 |
| euroscope | strip_update | 346 | 14.87 |
| euroscope | requested_altitude | 7 | 11.15 |
| euroscope | heading | 51 | 10.30 |
| euroscope | stand | 113 | 8.26 |
| euroscope | squawk | 221 | 7.34 |
| euroscope | aircraft_position_update | 20775 | 5.09 |
| euroscope | cleared_altitude | 205 | 3.74 |
| euroscope | controller_offline | 59 | 3.15 |
| euroscope | ground_state | 160 | 3.12 |
| euroscope | tracking_controller_changed | 1567 | 2.99 |
| euroscope | assigned_squawk | 189 | 2.99 |
| euroscope | coordination_received | 10 | 2.73 |
| euroscope | cleared_flag | 68 | 1.80 |
| euroscope | aircraft_disconnect | 1758 | 0.08 |
| frontend | issue_pdc_clearance | 4 | 120.66 |
| frontend | coordination_assume_request | 19 | 35.41 |
| frontend | coordination_force_assume_request | 16 | 28.64 |
| frontend | move | 192 | 28.33 |
| frontend | stand_assignment_manual_request | 4 | 26.38 |
| frontend | stand_assignment_confirmed_override | 1 | 25.42 |
| frontend | acknowledge_validation_status | 3 | 17.65 |
| frontend | runway_clearance | 59 | 17.38 |
| frontend | update_strip_data | 7 | 14.29 |
| frontend | update_order | 2 | 10.67 |
| frontend | release_point | 91 | 10.01 |
| frontend | stand_assignment_acknowledge | 18 | 5.82 |
| frontend | create_tactical_strip | 1 | 4.93 |
| frontend | delete_tactical_strip | 1 | 3.41 |
| frontend | generate_squawk | 8 | 0.04 |

Low sample sizes matter: requested altitude (7), runway (8), sync (3), and PDC issuance (4) cannot establish stable latency distributions. Aircraft disconnect schedules later work; its 0.08 ms mean does not measure that later cleanup.

### Coverage gaps

Registrations were reviewed in internal/euroscope/hub.go and internal/frontend/hub.go. Events absent or with zero increase in this window remain unmeasured, not certified fast:

- EuroScope login/authentication, communication type, CDM TOBT/deice/CTOT/remove/ready/ASRT/TSAC, private messages, PDC issuance/revert.
- Frontend authentication, unused coordination variants, CDM/CLX/start/marked/runway-confirmation actions, unexpected-change acknowledgement, private/session messages, missed approach, manual/VFR flight-plan creation, runway status, remaining tactical operations, AMAN coordination, PDC manual/revert, remaining stand operations.
- Ground-state averages mix no-ops with real transitions. These must be split in a repeatable scenario test.
- Production queue metrics cannot expose the entire once-per-second sender burst: the open-loop sender deadline remains the completion reference for capacity tests.

## Representative production traces

| Event | Processing ms | Queue ms | SQL statements observed | Main finding |
| --- | ---: | ---: | ---: | --- |
| heading | 10.90 | 0.19 | 8 | One field write followed by strip/display/stand enrichment |
| ground_state | 12.01 | 0.17 | 5 | Strip read, controller list, state write, another strip read, CDM read |
| hold | 36.61 | 0.32 | 6 | Two strip reads plus identity and whole-airport AMAN load |
| aman_route_fact | 16.51 | 0.16 | 3 | Whole-airport load even though requested active flight was not found |
| squawk | 5.13 | 0.08 | 3 | Field write plus session strip list and session read |
| strip_update | 29.08 | 0.31 | 18 | New-strip insertion/sequence allocation, validation and publication; not routine position movement |
| stand | 21.54 | 0.43 | 12 | Repeated strip/session reads, routing/display work and session validation |
| move (frontend) | 25.57 | 0.05 | 37 | 16 GetStrip calls, stand release transaction and bay append transaction |
| coordination_assume_request | 46.13 | 0.12 | 25 | Repeated strip/session lists and route/display enrichment |
| issue_pdc_clearance | 103.19 | 0.23 | 29 | Only 24.44 ms accounted for by attached query spans; remaining time needs stage tracing |

The move sample spends 22.40 ms in query spans and 0.13 ms acquiring connections. The heading sample spends 9.06 ms in query spans and 0.11 ms acquiring connections. These samples support reducing work before adding workers; they do not prove pool contention never occurs.

## Root causes and proposed changes

### 1. Field updates trigger expensive full publication

UpdateHeading and UpdateRequestedAltitude in internal/services/strip_fields.go send their field event and call PublishStripUpdate. This full update refreshes CLX validation, so removing it blindly would change behavior.

SendStripUpdateContext in internal/frontend/hub.go reloads the strip, computes next-display data, reads its stand assignment and, if assigned, lists all assignments to calculate blocking. The heading trace includes session, sector-owner and controller reads in that enrichment.

Proposed change:
- Return the persisted strip/version from a field update with UPDATE ... RETURNING.
- Pass that authoritative result to publication and validation; avoid the immediate reread.
- Assemble display/assignment dependencies once per message, or use a typed combined read.
- Preserve the full frontend contract and CLX refresh; only introduce a smaller update after verifying the frontend applies all affected fields.
- Avoid session-wide stand enrichment when dependency analysis shows it is unnecessary; do not simply remove conflict/blocking information.

Start with heading/requested altitude because they have a small semantic surface and already have CLX regression tests. Then apply the shared publication improvement to stand/release-point/update-strip paths.

### 2. Operational transitions repeatedly fetch the same state

MoveToBay and validation helpers can use getCachedStrip, but other steps read repositories directly, and mutations must refresh/invalidate that message-scoped state. The production frontend move demonstrates the duplication (16 strip reads).

UpdateGroundState explicitly calls MoveToBay(context.Background(), ...), losing both tracing and message state for that nested work.

Proposed change:
- Preserve the message context through synchronous transitions.
- Seed one message-scoped operational snapshot at handler entry.
- Update/invalidate it after each successful mutation; preserve transactional reloads and authority checks where freshness is required.
- Pass the existing strip and stand assignment into CDM/stand/validation routines.
- Reuse session/controller/sector data within the message with explicit invalidation where the event changes them.
- Keep bay sequence allocation and write under the session lock, and keep stand transaction revalidation.

This requires concurrent frontend/ES mutation tests; replacing every read with a cached pointer would be incorrect.

### 3. Squawk validation scales with the entire session

reevaluateSquawkValidationsForSession loads the session strips, builds duplicate membership and visits the collection for validation precedence. The observed squawk sample only performs three SQL statements; the issue is also data volume and fleet-size-dependent work, not necessarily one query per aircraft.

Proposed change:
- Evaluate the changed strip and strips sharing its old/new assigned or actual squawk.
- Preserve resolution of existing duplicate warnings and lower-priority validation reactivation.
- Use one set-based read for that affected group and message-scoped validation state.
- Keep the full-session path for sync and other events that genuinely affect the fleet.

### 4. AMAN identity batching does not address aggregate loads

The implemented draft batches established identity reads for position reports. Holds and route facts have another cost: LoadAirportState reads and decodes all flight JSON payloads, then validates the full airport state.

In the hold sample ListAMANFlights alone costs 14.71 ms. The route-fact sample returns not_found after loading the airport collection; its remaining uninstrumented time may include decode/validation, but needs a CPU/stage profile to attribute.

Proposed change:
- Add a typed active-flight lookup by airport/callsign or established identity for eligibility and correlation.
- Use narrow reads for speed facts and missing-flight checks.
- For holds/direct-to changes, retain aggregate revision checking and any airport-wide validation/reconciliation that is actually required. A narrow lookup alone cannot replace those guarantees.
- Add spans around payload decode, validation, commit, reconciliation and publication to establish whether further aggregate changes are justified.
- Do not add a cross-message operational or identity cache.

### 5. Measure PDC, sync and session-wide work separately

PDC issuance has a large unattributed interval in the inspected trace. Do not label that interval database time or external-network time without more evidence. Add stage spans across validation, persistence, delivery and publication. Full sync and controller/runway changes can legitimately touch many aircraft; report their batch size and total time as well as per-aircraft cost.

## Recommended implementation order and gates

1. Fix context continuity and complete query/stage accounting for all registered event families. Put any resulting charts on the Performance dashboard only. Ensure statement counting excludes pool acquisition/prepare spans and counts transaction boundaries consistently.
2. Optimize heading/requested-altitude publication using persisted results; verify unchanged CLX/frontend behavior.
3. Remove repeated strip/session reads in frontend moves, coordination and ground-state transitions, preserving invalidation and lock boundaries.
4. Add narrow AMAN reads for single-flight facts; measure aggregate decode/reconciliation before redesigning it.
5. Restrict squawk validation to the affected set with duplicate-resolution/precedence tests.
6. Replay a scenario matrix covering every registered event with unchanged, changed and real-transition cases, including errors. Use one position worker, burst traffic, concurrent frontend actions and multiple fleet sizes. Capture received/completed/error counts, physical SQL operations, queue/processing/sender-completion P50/P95/P99, pool waits and CPU utilization.

For routine scalar updates, initially budget one write and at most one combined read; a few milliseconds is a target to verify on the actual deployment. Real transitions retain separate correctness-driven query budgets. Repeat the original 100/s sustained and 200/s burst gates after the mixed short replay passes. Do not weaken durability or defer counted persistence to meet a latency number.

Prior local one-worker position results and acceptance limitations: [position-performance.md](../../position-performance.md).
