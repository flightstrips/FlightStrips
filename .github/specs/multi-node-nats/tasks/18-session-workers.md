# Task 18 — Session timers and reconcilers

**Depends on:** 04–10, 13, 16 and 17. **Outcome:** one effective session worker set, recoverable after takeover.

**Release boundary:** merge with the coordinated NATS runtime candidate PR and release at task 24; see [release-safety.md](../release-safety.md).

**Integration status:** complete on the integration base. [PR #817](https://github.com/flightstrips/FlightStrips/pull/817) supplies the deadline/cleanup candidate and [inventory](../session-worker-inventory.md); [19a / #819](https://github.com/flightstrips/FlightStrips/pull/819) supplies departure/arrival lifecycle; [18a / #822](https://github.com/flightstrips/FlightStrips/pull/822) supplies PDC polling/clearance lifecycle; [18c / #823](https://github.com/flightstrips/FlightStrips/pull/823) supplies socket deadlines, shared retention and durable squawk throttling; and [18b / #824](https://github.com/flightstrips/FlightStrips/pull/824) supplies operational CDM/traffic adapters. All are merged with passing CI. Constructor and two-replica evidence is recorded in their task files and the inventory. Runtime assembly and combined application acceptance remain Task 20. Task 20 must bind these concrete adapters without no-ops or SQL workers. Held from `main` and release until the coordinated cutover.

## Work

- Assign session ownership to PDC deadlines/polling, stand sweeps, departure/arrival lifecycle, session-specific CDM/traffic work, EuroScope offline/disconnect timers, and the session monitor registered from `backend/internal/app/app.go`. Persist deadlines and source revisions; rearm from projection after takeover and recheck state before firing.
- Implement five-minute cleanup only after cluster-wide operational EuroScope presence is empty during healthy service. Pause healthy-time accounting on quorum loss; after a full restart, grant a new five-minute observation window. Frontend-only connections do not keep a session alive.
- Keep only publication coalescing timers local when loss cannot change an outcome. Document each retained local timer's non-authoritative role.

## Done when

- Two backends execute one effective session timer result. Kill owner before and after each deadline; new owner fires once or finds the committed outcome.
- PDC, stand expiry, aircraft disconnect and session cleanup tests pass after takeover and full restart. An outage longer than five minutes does not cause immediate session deletion.
- A worker inventory maps every session writer/timer to the session owner and identifies any retained local-only timer.

**Starting points:** `backend/internal/app/app.go`, `backend/internal/pdc/service.go`, `backend/internal/server/server.go`, and stand/departure/arrival services.
