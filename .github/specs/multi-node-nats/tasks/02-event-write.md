# Task 02 — Aggregate event and conditional-write contract

**Depends on:** 01. **Outcome:** one reusable write path with durable idempotency and per-subject ordering.

## Work

- Implement generated `CommandRequest`, `CommandReply`, `StateEvent`, `EntityRecord` codecs, deterministic Protobuf request hashing, the command ledger, domain/owner-control reducers, and the exact errors in [contracts.md](../contracts.md) and [storage-contract.md](../storage-contract.md). Keep typed entity values full replacements; reject unknown fields, schema versions and entity cases.
- Publish with `Nats-Expected-Last-Subject-Sequence`, await PubAck, reload/revalidate on CAS conflict, and reuse the same command ID after an uncertain acknowledgment. Return success only after the local projection applies the effective event. Exclude lease maintenance events from the domain command ledger.

## Done when

- Two writers racing on one aggregate cannot both commit from the same subject revision. A lost PubAck and request/reply timeout converge on the original committed outcome; a changed payload under the same ID fails.
- Reducers yield byte-equivalent state for the same event sequence and ignore stale owner epochs deterministically.
- Existing domain repositories remain temporary fixtures; this task adds no handler-specific NATS publish.

**Starting points:** `backend/internal/repository/repository.go`; add shared transport/reducer code under `backend/internal/cluster`.
