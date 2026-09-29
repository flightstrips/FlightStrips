# Task 06 — Strip state and edit invariants

**Depends on:** 04 and 05. **Outcome:** one atomic session event for each strip transition.

## Work

- Replace strip SQL reads/writes, creation/deletion, field edits, sequencing, bay transitions and ownership changes with typed session commands. Preserve per-session callsign uniqueness, numeric strip IDs, optimistic edit version, order spacing and current EuroScope-source rules.
- Have each transition validate against one projection revision and emit full strip upserts/deletes plus any related session entity changes in one event. Position-only observations remain outside the user-edit version and are handled in task 10.
- Adapt strip-dependent HTTP/frontend/PDC/CDM/EFB reads to the projection and typed `Strip`/`FrontendDelta` WebSocket schemas. Preserve first-party HTTP JSON response shapes at the boundary.

## Done when

- Existing strip lifecycle, field, order, publication and conflict tests run against the NATS adapter. Races previously protected by SQL transactions produce one valid result or a visible revision conflict.
- Two backend snapshots agree on strip IDs, versions, order and bays after replay and restart.
- No strip service starts its own JetStream writer or transaction outside the aggregate command path.

**Starting points:** `backend/internal/repository/postgres/strip.go`, `backend/internal/services/strip_*.go`, and `backend/internal/frontend/strip_stores.go`.
