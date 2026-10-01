# Aircraft position throughput

The sole operational runtime uses NATS accepted state. Revision-2 EuroScope
position frames enter `euroscopebinary.DeadlineCandidate` and the fenced
`cluster.PositionWriter`; typed position observations use `FS_POSITIONS`.
Operational transitions continue through the session owner and `FS_STATE`.
Positions retain freshness, master/socket-generation checks, bounded work and
ordered drainage. A received frame is not evidence of an accepted mutation.

The SQL dispatcher, pool budget, transaction batching and
`POSITION_DB_BATCHING_ENABLED` controls have been retired. Historical benchmarks
and their failed latency gates remain in the [SQL runtime record](performance/2026-09-16/sql-position-runtime.md).
The [event update audit](performance/2026-09-16/event-update-audit.md) remains
historical context for publication, validation, AMAN and PDC behavior.

Use [the Windows setup](../backend/Readme.md) to build revision-2 clients and two
backends against three pinned brokers. `/healthz` is liveness; `/readyz` gates
admission on replay, quorum, watcher freshness and ownership. `/metrics` includes
PubAck duration/count, projection lag, observation freshness, owner/effect state
and storage sizes. Safe command logs expose IDs, sequences and epochs, never raw
frames, tokens or message bodies.

Task 20c's [acceptance record](../.github/specs/multi-node-nats/tasks/20c-sql-retirement-evidence.md)
checks combined startup, cross-node commands, backend death, broker-quorum loss
and retained-state replay. These checks do not measure production capacity.
Task 23 must measure sender-to-accepted-state latency, P95/P99, pending-work
bounds, overload/recovery, operational traffic mix, CPU/memory and broker storage
on an identified topology. Task 22 owns overlapping-version/recovery qualification.
The previous SQL measurements cannot establish NATS latency or multi-host safety.
Landing validation and the disabled SAT scenario/replay runner remain disabled.
