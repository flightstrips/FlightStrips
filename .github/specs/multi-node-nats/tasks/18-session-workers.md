# Task 18 — Session timers and reconcilers

**Depends on:** 04–10, 13, 16 and 17. **Outcome:** one effective session worker set, recoverable after takeover.

**Release boundary:** merge with the coordinated NATS runtime candidate PR and release at task 24; see [release-safety.md](../release-safety.md).

## Work

- Assign session ownership to PDC deadlines/polling, stand sweeps, departure/arrival lifecycle, session-specific CDM/traffic work, EuroScope offline/disconnect timers, and the session monitor registered from `backend/internal/app/app.go`. Persist deadlines and source revisions; rearm from projection after takeover and recheck state before firing.
- Implement five-minute cleanup only after cluster-wide operational EuroScope presence is empty during healthy service. Pause healthy-time accounting on quorum loss; after a full restart, grant a new five-minute observation window. Frontend-only connections do not keep a session alive.
- Keep only publication coalescing timers local when loss cannot change an outcome. Document each retained local timer's non-authoritative role.

## Done when

- Two backends execute one effective session timer result. Kill owner before and after each deadline; new owner fires once or finds the committed outcome.
- PDC, stand expiry, aircraft disconnect and session cleanup tests pass after takeover and full restart. An outage longer than five minutes does not cause immediate session deletion.
- A worker inventory maps every session writer/timer to the session owner and identifies any retained local-only timer.

**Starting points:** `backend/internal/app/app.go`, `backend/internal/pdc/service.go`, `backend/internal/server/server.go`, and stand/departure/arrival services.
