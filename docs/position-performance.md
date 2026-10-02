# Aircraft position throughput

The sole operational runtime uses NATS accepted state. Revision-2 EuroScope
position frames enter `euroscopebinary.DeadlineCandidate` and the fenced
`cluster.PositionWriter`; typed position observations use `FS_POSITIONS`.
Operational transitions continue through the session owner and `FS_STATE`.
Positions retain freshness, master/socket-generation checks, bounded work and
ordered drainage. A received frame is not evidence of an accepted mutation.

Stand planning reads the local state and position projections. Before publishing
a lifecycle change, the session owner drains accepted position work and pauses
later position writes. The lifecycle fence checks stream progress against the
ordered position consumer, then compares the complete current-master aircraft
set and each planning revision in memory. It does not enumerate position keys or
fetch every aircraft from KV. Constant-size NATS metadata checks establish that
the projection has applied the captured stream boundary; a successful PubAck
cache alone cannot establish completeness.

Replay tracks consumer identity, applied delivery sequences and retained keys,
including deletion and purge markers. An incomplete replay, changed consumer
generation, malformed observation or unverifiable retained-key set prevents a
lifecycle commit. Ownership, master synchronization, leases and the conditional
`FS_STATE` publication remain authoritative. The standby independently replays
the committed result. Initial position writes and individual position-derived
transitions retain their separate KV compare-and-set checks.

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

## Native NATS load qualification (Task 23 draft)

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

The fixture enables synthetic VATSIM, local navigation/weather sources and stand
assignment. Aircraft trajectories exercise landing/ALDT, arrival taxi, departure
airborne, stand activation and stand release, with assertions in full runs. A
separate recovery test measures five backend kills, client reconnects and fresh
syncs. The native runner combines all six full patterns, recovery, disk checks
and the complete Task 22 fault/restore suite:

```powershell
./backend/testdata/nats/task23.ps1 -NATSServerBinary 'C:/path/to/nats-server.exe'
```

Disk usage at 70% records an alert; 85% blocks release qualification while local
tests continue. Per-pattern reports always set `qualification=false`; `load_pass`
covers their load and lifecycle assertions. The runner's `run-result.json`
requires every full pattern, recovery and fault test to pass. Aborted reports
retain partial measurements and cannot establish capacity or qualification.

**Qualification remains incomplete.** At revision `aa15f68f`, backend unit and
contract checks passed and the full Task 22 fault/restore suite passed in 826.675
seconds. All six load scenarios aborted on socket failures, and recovery failed
during session-name lookup before any recovery trial. The partial mixed/even
run reached receipt p95 2.59 ms/p99 37.16 ms over eight minutes; burst cases
exceeded the latency targets. These partial results are diagnostics, not passing
full-duration evidence. Disk usage also exceeded the 85% release threshold.
Follow-up repairs replace lifecycle fleet reads with the verified memory fence,
replan concurrent operational updates, defer genuine snapshot-index contention
and repair recovery session lookup. The full backend unit suite and affected
race suites pass. A native memory-fence regression also passes with the race
detector, covering new neighbours, disconnects, deletes, silent purge detection,
empty retained replay and fresh synchronization after restarting both backends.
These correctness results do not replace the six full load patterns or the
complete recovery and fault qualification on the repaired source.
No Task 24 or multi-host acceptance is claimed.
