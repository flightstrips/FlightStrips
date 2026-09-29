# Task 15 — Binary frontend WebSocket transport and typed projection

**Depends on:** 06 and 13. **Outcome:** frontend sockets use only the generated binary frame contract and can rebuild their view from typed state.

**Release boundary:** merge only with the coordinated candidate PR; release the binary-only frontend and backend WebSocket change together at task 24; see [release-safety.md](../release-safety.md).

## Work

- Replace frontend JSON WebSocket frames with generated binary `FrontendFrame` from [wire.proto](../proto/wire.proto). Require `flightstrips.frontend.pb.v2`; reject text frames and unknown revisions.
- Publish typed `FrontendInitial`, `FrontendDelta`, and `FrontendObservation` from the projection. Buffer deltas during initial load and resync on gaps. Derive former per-field UI events locally from full typed entity replacements.
- Decode the typed `ClientCommand` oneof and route it to the correct aggregate owner with UUID `FrontendCommand.request_id`; keep nested AMAN coordination request IDs distinct. Return typed protocol errors for malformed action cases.

## Done when

- All current browser WebSocket action cases in [coverage.md](../coverage.md) are represented by generated oneof cases; no JSON frame remains.
- Two backend replicas publish the same typed initial/delta sequence for a session after replay, and a reconnect across replicas gets a coherent snapshot plus buffered deltas.
- Text/malformed/unsupported frames fail with the close behavior in [contracts.md](../contracts.md).

**Starting points:** `frontend/src/api/models.ts`, `frontend/src/api/websocket.ts`, `frontend/src/store/store.ts`, and backend frontend hub/handlers.
