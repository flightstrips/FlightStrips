# Task 05 — Controller, sector and runway session state

**Depends on:** 04. **Outcome:** durable session metadata and controller-facing ownership records.

## Work

- Move controller identities/layout, sector owners, active runways, airport master-order read dependencies, and their current repository methods to session/airport projections as assigned in [state-map.md](../state-map.md). Keep existing uniqueness and authorization; publish generated typed records through the new Protobuf client protocol.
- Keep socket online/offline status separate from durable controller metadata. A controller is presented as operationally online only with fresh `FS_PRESENCE`; task 14 supplies cluster-wide presence and master election. Until then, tests may inject a typed presence view.

## Done when

- Existing controller/sector/runway tests pass against the NATS adapter. Competing sector/controller writes preserve uniqueness and expected revisions.
- Restarting both backends restores durable metadata but never fabricates live controllers or a valid master sync.
- All controller/sector/runway SQL calls have a named replacement; no hub-local map is treated as authority.

**Starting points:** `backend/internal/repository/postgres/controller.go`, `sector_owner.go`, and `backend/internal/euroscope/hub.go`.
