# Task 18 — Session timers and reconcilers

**Depends on:** 04–10, 13, 16 and 17. **Outcome:** one effective session worker set, recoverable after takeover.

**Release boundary:** merge with the coordinated NATS runtime candidate PR and release at task 24; see [release-safety.md](../release-safety.md).

**Integration status:** [PR #817](https://github.com/flightstrips/FlightStrips/pull/817) merged the dormant deadline/cleanup candidate and [inventory](../session-worker-inventory.md). [Task 19a / PR #819](https://github.com/flightstrips/FlightStrips/pull/819) completed departure/arrival lifecycle. Task 18 remains partial: [18a](18a-pdc-worker.md) supplies PDC polling/clearance lifecycle, [18b](18b-cdm-traffic.md) supplies session CDM/traffic adapters, and [18c](18c-euroscope-deadlines.md) supplies socket deadlines, shared retention and durable squawk throttling. All three must satisfy their acceptance criteria before Task 20. Task 20 must not substitute missing hooks with no-ops or start SQL workers to fill them.

## Work

- Assign session ownership to PDC deadlines/polling, stand sweeps, departure/arrival lifecycle, session-specific CDM/traffic work, EuroScope offline/disconnect timers, and the session monitor registered from `backend/internal/app/app.go`. Persist deadlines and source revisions; rearm from projection after takeover and recheck state before firing.
- Implement five-minute cleanup only after cluster-wide operational EuroScope presence is empty during healthy service. Pause healthy-time accounting on quorum loss; after a full restart, grant a new five-minute observation window. Frontend-only connections do not keep a session alive.
- Keep only publication coalescing timers local when loss cannot change an outcome. Document each retained local timer's non-authoritative role.

## Done when

- Two backends execute one effective session timer result. Kill owner before and after each deadline; new owner fires once or finds the committed outcome.
- PDC, stand expiry, aircraft disconnect and session cleanup tests pass after takeover and full restart. An outage longer than five minutes does not cause immediate session deletion.
- A worker inventory maps every session writer/timer to the session owner and identifies any retained local-only timer.

**Starting points:** `backend/internal/app/app.go`, `backend/internal/pdc/service.go`, `backend/internal/server/server.go`, and stand/departure/arrival services.
