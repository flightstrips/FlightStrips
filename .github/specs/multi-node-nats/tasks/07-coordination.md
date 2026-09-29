# Task 07 — Coordination and transfer commands

**Depends on:** 05 and 06. **Outcome:** coordination changes stay atomic with their strip/controller effects.

## Work

- Move `coordinations` and transfer, assume, free, tag, force-assume and cancel workflows to session entities and commands. A transition that affects coordination, strip owner and controller/sector view emits one validated session event.
- Preserve existing authorization, force-assume result, replay/order and version rules. Route all handlers through the aggregate owner once task 13 is available; until then, exercise the same command service directly in tests.

## Done when

- Existing coordination and force-assume tests pass against the NATS adapter, including conflicting simultaneous controllers on separate backend processes.
- Replaying a duplicate command ID returns one outcome and publishes no duplicate transfer notification.
- Every coordination SQL method has a projection or command replacement.

**Starting points:** `backend/internal/repository/postgres/coordination.go`, `backend/internal/services`, and `backend/internal/frontend/handlers.go`.
