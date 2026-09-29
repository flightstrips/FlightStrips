# Task 19 — Airport and global external workers

**Depends on:** 11–13 and 17. **Outcome:** polls and provider effects cannot double-run on two backend replicas.

**Release boundary:** merge with the coordinated NATS runtime candidate PR and release at task 24; see [release-safety.md](../release-safety.md).

## Work

- Assign VATSIM fetch/reconciliation, AMAN observation/reconciliation, navigation import, weather refresh, METAR, ECFMP and airport CDM work to airport or global owners according to [state-map.md](../state-map.md). Persist source checkpoints and external-call intents/results that affect idempotency or quota.
- Resume airport-to-session intents after takeover with the same workflow step IDs. An AMAN intent with superseded source revision terminates as `superseded`; accepted newer state may create a new intent. Reserve global weather quota before a provider call and never issue a duplicate call after an uncertain result.

## Done when

- Two backend replicas produce one effective domain update and no duplicate quota-consuming request for the same input/deadline.
- Kill an owner around intent, external call and result commit. New owner resumes committed work or records uncertainty without inventing a response or repeating unsafe effects.
- A worker inventory covers every non-session worker started in `backend/internal/app/app.go` and any handler-created external-call goroutine.

**Starting points:** `backend/internal/app/app.go`, `backend/internal/vatsim`, `backend/internal/aman`, `backend/internal/cdm`, `backend/internal/ecfmp`, and METAR providers.
