# Current-state runtime contract

This supersedes the permanent command-history and local archive requirements in
the original integration plan. HTTP remains JSON; WebSockets, NATS state and
objects remain typed Protobuf. No Redis or ALB changes are involved.

## State and admission

- One lease-fenced owner serializes each global, airport or session aggregate.
- Ordinary mutations apply to the owner's immutable published RAM view and enter
  the existing ordered persistence queue. Capacity is 1,024 jobs per process;
  saturation rejects admission before changing RAM.
- A `memory_accepted` reply means queued, not durable. A process crash may lose
  that tail. Replicas and takeover reconstruct only broker-confirmed state.
- Durable prerequisites and external-action claims flush the aggregate's queue
  and await JetStream confirmation. Provider quota reservations remain durable.
- Graceful shutdown stops admission and drains the queue. Persistence failure
  stops further admission; it never silently drops jobs.

## Retry records and pending work

- No local history database or archived-record lookup exists.
- Each aggregate retains its latest 4,096 command outcomes, ordered by accepted stream sequence and command ID.
- Outcomes for pending effects and pending same-aggregate workflow results are
  pinned until the operation becomes terminal. Pending work is never evicted.
- Up to 1,024 terminal workflows and 1,024 terminal effects remain available.
  The latest transceiver reconciliation checkpoint is retained independently.
- Deduplication is guaranteed while a receipt remains in this window. Status
  queries outside it return NOT_FOUND. A missing receipt is not evidence that an
  old action failed; clients must refresh state and must not automatically resend
  an old externally visible action with a new ID.
- Snapshot and replay apply the same deterministic retention rules. Current
  domain entities, revision counters, terms and pending operations survive.
- VATSIM observation entities retain the latest identity per callsign; a worker
  checks that identity before re-evaluating a generation whose receipt aged out.

## Provider reads

- A routine provider-page read uses one typed checkpoint per provider/resource.
  Its attempt ID and status replace the general external-call workflow.
- A new attempt first commits PENDING while preserving the last accepted page.
  Only the process receiving the fresh durable admission performs the request.
- Successful publication replaces the page reference and sets COMPLETED. A
  failed or interrupted attempt is not retried under the same ID; the next
  scheduled polling ID may try again. No permanent attempt history is needed.
- Checkpoint metadata, verified object caching, quota admission and owner fencing
  remain authoritative. A checkpoint never points at an unverified object.
- Multi-step AIRAC import and weather request workflows remain durable.
- External actions that change provider/plugin state retain durable pending
  workflows/effects. They are not converted to retryable reads.

`ProviderCheckpoint` adds `attempt_id` (8, canonical UUID), `attempt_status`
(9: 0 legacy, 1 PENDING, 2 COMPLETED, 3 FAILED), and `accepted_revision`
(10, uint64). The accepted revision advances only after verified publication;
pending/failed attempts preserve it and the accepted object. Legacy checkpoints
with no attempt fields remain readable. No JSON payloads are introduced.

## Recovery and migration

- Snapshots contain current state, retained receipts and pending work only.
  They do not reconstruct discarded terminal history.
- Existing snapshots and events remain readable. Replay trims old materialized
  records using the same rules; old local bbolt caches are no longer opened.
- Existing NATS log/object data and Docker volumes are preserved during this
  migration. Automatic broker-log deletion is outside this change: safely
  compacting it requires a separate retention/replay migration, not a blind purge.
- Validate memory bounds, snapshot/replay agreement, queue saturation, duplicate
  retries, uncertain provider reads and backend failover before restarting the
  user's retained local stack with the new binary.
