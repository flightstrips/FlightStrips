# Task 18b — Session CDM reconciliation and traffic metrics

**Depends on:** 06, the merged Task 18/19 candidates and 19a.

**Outcome:** concrete `SessionWork.CDM` and `Traffic` callbacks execute current session policy from typed NATS state without SQL. Candidate remains dormant until Task 20 and held until Task 24 under Task 18's release boundary.

## Contracts

- Accepted inputs are typed session/strip/CDM state, accepted airport CDM configuration, runway/sector state, navigation/route facts, committed vIFF pages and accepted shared observations. Reuse the Task 19 CDM configuration and vIFF adapters; no untracked HTTP calls or detached provider goroutines.
- Preserve current CDM policy: local/vIFF mode selection, sequencing, TOBT/TSAT/TTOT/CTOT and actual-time handling, readiness/deice, runway LVO, periodic validation and debounced recalculation. Output uses existing `CdmState`, `Strip`, `FrontendMessage`, `SessionDeadline`, `WorkflowRecord` and effect types. Multi-strip sequence results commit atomically under session CAS after rechecking the complete input revisions. Preserve unrelated controller edits.
- Poll/sync/recalculation/validation/debounce deadlines are persisted `SessionDeadline` records. Kinds use the `cdm-` prefix and keys identify session-wide work or callsign plus work kind. A queued deadline carries the accepted scheduling revision; the owner rechecks current inputs before applying results. Derive command UUIDs from session, work kind and persisted deadline/input revision. Takeover resumes the same identities, not a new local debounce episode.
- All operational vIFF mutations use durable typed intents/results from Task 19, including ATOT/AOBT/TOBT and post-recalculation pushes. A provider uncertainty retains its recorded result and must not resend the same unsafe call. Local recalculation commits before any derived provider intent can dispatch.
- Traffic metrics are read-only derived gauges, not domain writes: reuse current bay classification and fifteen-minute ALDT/AOBT window rules over the typed projection. Emit the session series from its accepted owner to avoid two replicas multiplying aggregate gauges. Gauge publication needs no durable domain event, but stale/unready owners must stop publication. No new traffic storage schema is introduced.
- This task introduces no new Protobuf top-level field allocation. Use the existing typed schemas, owner planners and deadlines. Tasks 18a/18c reserve their own additive schema fields. Export candidate constructors/callbacks that Task 20 can bind without a SQL compatibility adapter.

## Work

- Port real `cdm.SyncService`, periodic sequencing/validation and debounce behavior into the owner path using existing pure calculation/sequence policy. Bind the concrete callbacks and public candidate action services needed by HTTP/browser mutations. Do not leave SQL-dependent operational entry points for Task 20 to rewrite.
- Replace the candidate traffic reader with a typed projection adapter, preserving current metrics labels and classification. Audit `master_viff_sync.go` and all asynchronous CDM action call sites for durable candidate equivalents.
- Update the inventory with every concrete constructor and each retained local read-only timer. Keep app startup dormant and preserve the existing production runtime.

## Done when

- Real two-replica NATS tests run concrete CDM policy through initial sync, local/vIFF modes, multi-strip sequence changes, debounce replay/takeover, CTOT validation, stale config/input rejection and owner death around provider intent/result commits.
- Tests prove one effective sequence/validation result, no duplicate unsafe vIFF push and preservation of controller edits; traffic fixtures preserve the existing counts and only the accepted owner publishes a session series.
- No candidate constructor uses SQL repositories or fake calculations. Only runtime binding remains for Task 20; the checklist lists evidence for each operational action.
