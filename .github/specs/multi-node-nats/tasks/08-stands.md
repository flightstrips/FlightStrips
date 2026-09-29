# Task 08 — Stand allocation and blocks

**Depends on:** 06. **Outcome:** existing stand constraints hold under concurrent nodes without SQL transactions.

## Work

- Move stand assignments, blocks, manual/automatic actions, expiry fields, versions and read indexes to the session aggregate. Reexpress each current serializable allocation as one validated event containing all affected entities.
- Preserve current rule ordering, manual precedence, acknowledgment, VATSIM identity/revision checks and stand collision behavior. Task 18 owns scheduled expiry firing; this task persists its deadline and validates the expiry command.

## Done when

- Existing stand allocation, repository and API tests pass against the NATS adapter. Racing allocations on two backends cannot assign the same exclusive stand incompatibly.
- A failed CAS reloads and reevaluates rules rather than replaying a stale proposed assignment. Restart preserves blocks and assignment history.
- No stand path uses `BeginTx`, local lock ownership or a second write outside the session event to claim atomicity.

**Starting points:** `backend/internal/services/stand_allocation.go` and `backend/internal/repository/postgres/stand_assignment.go`.
