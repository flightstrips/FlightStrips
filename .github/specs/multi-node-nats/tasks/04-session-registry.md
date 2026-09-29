# Task 04 — Global registry and session lifecycle records

**Depends on:** 03. **Outcome:** unique sessions and stable numeric IDs without SQL.

## Work

- Implement the global airport/session registry and monotonic `int32` session allocator. Replace `GetOrCreateSession` with the `initializing → active` workflow in [state-map.md](../state-map.md), including idempotent session aggregate seeding.
- Implement `session_tombstoned → registry deleting/deleted` as a durable workflow. Retain IDs and history; remove the active `(airport,name)` index only after the tombstone. This task provides lifecycle records; task 18 supplies the five-minute worker.

## Done when

- Concurrent creates of the same `(airport,name)` return one ID. A crash between global allocation and session creation resumes with the same workflow ID.
- Recreating a name after deletion gets a new ID; no request can revive a tombstoned session. Both backend projections show the same active registry.
- Session repository tests run against the new adapter, including connection setup waiting for `active`.

**Starting points:** `backend/internal/server/server.go`, `backend/internal/repository/postgres/session.go`, and the session interface.
