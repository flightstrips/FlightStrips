# Task 19 — Airport and global external workers

**Depends on:** 11–13 and 17. **Outcome:** polls and provider effects cannot double-run on two backend replicas.

**Release boundary:** merge with the coordinated NATS runtime candidate PR and release at task 24; see [release-safety.md](../release-safety.md).

**Integration status:** [PR #815](https://github.com/flightstrips/FlightStrips/pull/815) merged the fenced external-call framework and [worker inventory](19-worker-inventory.md). [PR #816](https://github.com/flightstrips/FlightStrips/pull/816) merged typed provider candidates and real NATS fault tests. Task 19 remains open: [Task 19a](19a-vatsim-lifecycle.md) completes VATSIM stand lifecycle handling and [Task 19b](19b-aman-policy.md) binds the operational AMAN evaluator and destination command builder. Task 20 must not activate these workers until both follow-ups and the remaining Task 18 adapters are merged and verified. App startup binding and the final inventory audit belong to Task 20; domain behavior belongs to these prerequisites.

## Completion checklist after PR #815

This checklist tracks code and fault evidence separately. Task 19 is complete
only when every item is checked and merged to the integration base. Candidate
startup stays dormant in the meantime.

- [x] Fenced external-call intent, stable result ID, takeover uncertainty, and worker inventory (PR #815).
- [x] Named typed ECFMP provider page and global checkpoint contract, with no vendor JSON in Protobuf (this completion branch).
- [x] ECFMP global fetch and per-flight session application adapters, including source-revision fencing (this completion branch).
- [x] Real two-replica NATS ECFMP global fetch test covering nonowner dispatch, committed result, pre-call intent, post-call owner death, and uncertain takeover (this completion branch).
- [x] Real two-replica NATS ECFMP session application, nonowner rejection, replay, and owner-failure test (this completion branch).
- [x] Global-owned VATSIM fetch with a typed source checkpoint and two-replica NATS owner-failure test (this completion branch).
- [x] VATSIM airport/session reconciliation adapters and their two-replica failure tests (Task 19a candidate; merge to the integration base still required).
  - [x] Session-owned typed strip generation replay and airport-owned present/missing AMAN observations; two-replica NATS takeover and replay tests.
  - [x] Port departure/arrival stand lifecycle transitions and cancellation, including their owner-failure tests: `services.NewVatsimLifecycleCandidate` supplies both `SessionWork` callbacks; `TestVatsimLifecycleTwoReplica*` exercises real policy and Task 17 effects without SQL or injected decisions.
- [ ] Airport-owned AMAN observation/reconciliation adapter; resumable airport-to-session intents, superseded-revision handling, and two-replica failure tests.
  - [x] Consume typed VATSIM global checkpoints on the airport owner; stable observation and reconciliation command IDs; two-replica observation replay and superseded intent test.
  - [x] Test a destination session write committed before airport-owner death; takeover records completion without a duplicate strip write, and a later revision supersedes a separate intent.
  - [ ] Bind the operational AMAN evaluator and destination session command builder to this candidate adapter.
- [x] Airport-owned AIRAC import and verified manifest activation adapter with typed page checkpoints and two-replica failure tests (this completion branch).
- [x] Airport-owned METAR fetch, global-owned AFV ATIS typed feed, session presentation adapter, global quota reservation, and two-replica NATS failure/replay tests (this completion branch).
- [x] AMAN/Open-Meteo wind refresh adapter with typed airport checkpoint, durable global quota reservation, and two-replica uncertainty test (this completion branch).
- [x] Airport-owned typed CDM configuration refresh and vIFF master read/write calls, session-owned typed vIFF flight reads and all operational write methods, with owner/replay/uncertain-result two-replica tests (this completion branch). The candidate adapters remain dormant until Task 20 binds the runtime.
- [x] Full inventory audit against `app.Build` and handler-created goroutines on this branch; no candidate startup before Task 20. New runtime wiring requires a repeat audit at activation.

## Work

- Assign VATSIM fetch/reconciliation, AMAN observation/reconciliation, navigation import, weather refresh, METAR, ECFMP and airport CDM work to airport or global owners according to [state-map.md](../state-map.md). Persist source checkpoints and external-call intents/results that affect idempotency or quota.
- Resume airport-to-session intents after takeover with the same workflow step IDs. An AMAN intent with superseded source revision terminates as `superseded`; accepted newer state may create a new intent. Reserve global weather quota before a provider call and never issue a duplicate call after an uncertain result.

## Done when

- Two backend replicas produce one effective domain update and no duplicate quota-consuming request for the same input/deadline.
- Kill an owner around intent, external call and result commit. New owner resumes committed work or records uncertainty without inventing a response or repeating unsafe effects.
- A worker inventory covers every non-session worker started in `backend/internal/app/app.go` and any handler-created external-call goroutine.
- Typed adapters cover VATSIM fetch/reconciliation, AMAN observation/reconciliation, AIRAC import and manifest activation, METAR/ATIS, ECFMP, and CDM configuration/vIFF calls. ECFMP provider-wide responses require a reviewed typed schema or a typed refetch/checkpoint path; raw JSON must never be stored in Protobuf bytes or strings.
- Two-replica NATS tests exercise those adapters and owner failure before the provider call, after the call, and around result commit, including AMAN supersession and weather quota uncertainty.

**Starting points:** `backend/internal/app/app.go`, `backend/internal/vatsim`, `backend/internal/aman`, `backend/internal/cdm`, `backend/internal/ecfmp`, and METAR providers.
