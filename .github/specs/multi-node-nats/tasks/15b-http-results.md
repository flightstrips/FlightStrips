# Task 15b — HTTP JSON boundary and idempotent command results

**Depends on:** 02 and 13. **Outcome:** first-party HTTP remains JSON while every mutation reaches the same typed durable command path.

**Release boundary:** merge with the coordinated candidate PR and release at task 24. Requiring `Idempotency-Key` before the matching HTTP callers ship would break today's mutating requests; see [release-safety.md](../release-safety.md).

## Work

- Preserve existing first-party HTTP JSON request and response bodies, URL selectors and ordinary validation/error shapes. Decode JSON only in the handler, validate it, then construct a typed `CommandRequest`. Never place raw JSON or serialized request bytes into NATS, Object Store or a Protobuf string.
- Require UUID `Idempotency-Key` on mutating endpoints and echo `X-FlightStrips-Command-ID` and `X-FlightStrips-Outcome` as fixed in [contracts.md](../contracts.md). Use 202 for accepted pending effects. Preserve existing success body/status for immediate durable success; use stable provider event IDs for callbacks.
- Add actor-scoped JSON `GET /api/commands/{uuid}` to read typed `CommandOutcome`. Authenticated domain rejections commit an outcome-only event; authorization failures remain immediate and are not persisted.

## Done when

- Existing pilot, EFB, PDC, GSX, stand, CDM, ECFMP and AMAN HTTP response contracts still pass their tests. JSON is confined to HTTP/vendor adapters.
- Duplicate idempotency key and identical body converge on one outcome; changed body under the same key fails. Outcome query enforces actor scope.
- A pending effect is observable as 202/`accepted` and later as a terminal query result.

**Starting points:** backend HTTP handlers under `backend/internal`, auth middleware, and the typed command ledger.
