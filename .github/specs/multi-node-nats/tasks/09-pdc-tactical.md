# Task 09 — PDC and tactical strips

**Depends on:** 04 and 06. **Outcome:** these session workflows survive backend restart.

## Work

- Move PDC sequence/state, request timestamps, message sequence and sent status from session/strip SQL fields into session entities. Persist every PDC deadline and requested EuroScope effect; task 17 supplies plugin delivery and task 18 owns timer firing.
- Move tactical strip creation, movement, confirmation, timer start and deletion into typed session entities with per-session numeric IDs. Preserve bay ordering and authorization; publish through typed `FrontendDelta`.

## Done when

- Existing PDC and tactical tests run against the NATS adapter. A process restart restores a pending PDC request and tactical timer without inventing a second result.
- Concurrent PDC issuance or tactical moves on different backends produce one accepted session transition per command ID.
- No active PDC/tactical handler reads a local timer or PostgreSQL row as the source of truth.

**Starting points:** `backend/internal/pdc/service.go`, `backend/internal/repository/postgres/tactical_strip.go`, and strip PDC fields.
