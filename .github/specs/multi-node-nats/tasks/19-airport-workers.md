# Task 19 — Airport and global external workers

**Depends on:** 11–13 and 17. **Outcome:** polls and provider effects cannot double-run on two backend replicas.

**Release boundary:** merge with the coordinated NATS runtime candidate PR and release at task 24; see [release-safety.md](../release-safety.md).

**Integration status:** [PR #815](https://github.com/flightstrips/FlightStrips/pull/815) merged the fenced external-call framework and [worker inventory](19-worker-inventory.md), but did not wire the provider adapters or meet the two-replica acceptance gate below. Task 19 remains open. Task 20 must not activate these workers until the remaining work is merged and verified.

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
- [ ] VATSIM airport/session reconciliation adapters and their two-replica failure tests.
- [ ] Airport-owned AMAN observation/reconciliation adapter; resumable airport-to-session intents, superseded-revision handling, and two-replica failure tests.
- [ ] Airport-owned AIRAC import and verified manifest activation adapter with typed page checkpoints and two-replica failure tests.
- [x] Airport-owned METAR fetch, global-owned AFV ATIS typed feed, session presentation adapter, global quota reservation, and two-replica NATS failure/replay tests (this completion branch).
- [ ] AMAN/Open-Meteo weather refresh adapter with durable global quota reservation and two-replica uncertainty tests.
- [ ] Airport-owned CDM configuration and vIFF master calls, session-owned vIFF flight calls, and two-replica failure tests around every external-effect boundary.
- [ ] Full inventory audit against `app.Build` and handler-created goroutines after all adapters land; no candidate startup before Task 20.

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
