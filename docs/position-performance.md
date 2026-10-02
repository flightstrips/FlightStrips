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

## Initial NATS load harness (Task 23 draft)

`TestPositionLoadNATS` builds two backend executables and starts three native,
test-owned brokers on random loopback ports with separate temporary stores,
history caches and effect keys. It requires `NATS_SERVER_BINARY` and does not use
the running development cluster. Both EuroScope and frontend clients use the
existing binary protocol. OTLP completion spans measure accepted inbound work;
reading a frame or writing it to a socket does not count as completion.

From `backend`, using a pinned local NATS 2.15.0 executable:

```powershell
$env:NATS_INTEGRATION = '1'
$env:NATS_TASK23 = '1'
$env:NATS_SERVER_BINARY = 'C:/path/to/nats-server.exe'
$env:NATS_TASK23_OUTPUT = Join-Path (Get-Location) '.task23/full'
Remove-Item Env:NATS_TASK23_SMOKE, Env:NATS_TASK23_PATTERN -ErrorAction SilentlyContinue
go test ./cmd/server -run '^TestPositionLoadNATS$' -count=1 -timeout=2h30m -v
```

The default schedule is two minutes of warmup and fifteen minutes measured at
100 reports/second for 200 aircraft, followed by ten seconds at 200 reports/second
and two seconds of continued 100-report/second drainage. It runs the 50%, 80% and
20% arrival mixes with both even and once-per-second burst schedules. Each
schedule adds five heading messages and one frontend action per second. Both
receipt-to-completion and scheduled-send-to-completion retain p95 <=20 ms and
p99 <=50 ms gates, with exact counts, zero unexpected errors, bounded backlog,
two-second overload drainage and burst completion before the next second.

For development only, set `NATS_TASK23_SMOKE=1` for two seconds of warmup and ten
seconds measured, and optionally `NATS_TASK23_PATTERN=mixed-second-burst` to select
one pattern. Smoke artifacts are named separately. Reports contain backend binary
SHA-256, process CPU/memory samples, backend metrics, NATS network/store counters
and separate position/state PubAck percentiles. All brokers share one host and
disk; these measurements cannot certify production placement or capacity.

**Qualification remains incomplete.** The original landing/ALDT, arrival taxi,
departure airborne and stand activation/release assertions still need an enabled
lifecycle fixture. This initial fixture disables those providers. Backend-kill
recovery with client reconnect, disk 70% alert/85% block and off-cluster restore
are also pending, as are the affected Task 22 fault checks on this changed source.
Reports always set `qualification=false`; `load_latency_pass` refers only to the
implemented load gates. No full-duration results or Task 24 acceptance are claimed.
