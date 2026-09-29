# Task 13 — Owner leases and command routing

**Depends on:** 02 and 03. **Outcome:** one accepted writer per aggregate, independent of which backend received a client action.

## Work

- Implement eight-second owner leases and two-second renewals as events on each aggregate's `FS_STATE` subject. Use server message timestamps for expiry, CAS for claims, monotonically increasing epochs and deterministic stale-event no-ops. Elect takeover candidates by rendezvous rank over ready nodes.
- Add `FS_PRESENCE` node heartbeats (three-second renew, ten-second TTL), incarnation IDs, Core NATS `fs.v1.command.<node-id>` request/reply, `NOT_OWNER` redirects and bounded retry with the original command ID.
- Stop accepting commands/effects on disconnect, renewal loss or projection unready. Keep ownership logic outside domain handlers.

## Done when

- Kill an owner and race standby claimants: exactly one new epoch becomes effective; old owner writes yield no effective state or success response.
- Lost Core NATS request/reply is retried safely because only PubAck and projection application establish commit.
- One NATS node loss preserves writes; quorum loss makes both backends unready and unwritable.

**Starting points:** task 02 cluster package, `backend/internal/app/app.go`, and `backend/internal/websocket`.
