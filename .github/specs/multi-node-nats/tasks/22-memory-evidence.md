# Bounded backend history memory qualification

Runtime source: `ff9a2e8c28f2e2ab12643384c6df4eff519f0ad3`.
Qualification run: `20261001T204232Z-3c185d3b78fd435488de17e95b9518d6`.
The runner recorded a clean worktree and the empty tracked-diff digest.

## Cause and change

The development backends were receiving background events even without controller
input. A read-only 20-second observation found 55 accepted events: 20 owner
renewals and 35 domain changes, including 27 quota reservations and three provider
checkpoint changes. Historical outcomes/workflows accumulated in projection maps,
and provider quota entities accumulated new windows. Full-state reads also caused
the allocation churn described in [the CPU investigation](22-idle-cpu-evidence.md).
Container RSS fluctuated with collection and allocator capacity; those samples
alone did not establish a monotonic memory leak.

Every backend now rebuilds a disposable bbolt disk index from verified snapshots
and the retained `FS_STATE` log. Each aggregate retains 512 completed records per
history kind, all pending workflows/nonterminal effects and their outcomes, and
one latest transceiver reconciliation checkpoint. Historical command, workflow,
effect, frontend status, CDM, PDC and provider lookups consult the index and
propagate failures. Published states use versioned index keys so later eviction
cannot change an earlier checkpoint's lookup result. Index failure blocks
readiness and writes. Effect-object retention scans the accepted event stream
without accumulating all historical events in a slice.

Command identities, accepted outcomes and the durable log retain their original
lifetime semantics. No wire schema, protobuf revision or snapshot format changes.
Full canonical snapshots still include historical records and preserve their
digest. Snapshot construction stops before decoding additional records beyond
the 32 MiB object budget; an oversized snapshot skips this optimization and
recovery uses an earlier verified checkpoint and the complete retained stream.
The fallback is exposed by `fs_snapshot_size_skips_total` and can increase recovery
time. Provider quota windows older than 48 hours are removed by accepted events;
new reservations for expired windows are refused, while retries resolve their
original result before planning.

`NATS_HISTORY_CACHE_DIR` selects writable disk storage; the default is the OS
temporary directory. Avoid memory-backed storage when reducing container memory.
The cache is rebuilt rather than reused, normal shutdown removes it, and startup
cleans valid unlocked caches of stopped processes. Live backend files stay locked
and untouched. Disk/log usage still grows with retained history. Current entities
and active operations still need memory; this is a bound on resident historical
records, not a constant bound on all application data or process RSS.

## Growth and safety tests

`TestProjectionHistoryMemoryPlateaus` applies 50,000 validated accepted outcome
events through the actual projection and disk index. A forced collection at each
sample separates retained objects from allocator capacity.

| Accepted outcomes | Resident outcomes | Live Go heap bytes | Disk index bytes |
| --- | --- | --- | --- |
| 5,000 | 512 | 2,464,760 | 2,097,152 |
| 25,000 | 512 | 2,495,920 | 8,388,608 |
| 50,000 | 512 | 2,539,792 | 16,777,216 |

Live heap grows only 75,032 bytes between the first and last samples. This is a
projection-only fixture, not the memory size of a full backend. Related tests
verify complete canonical snapshot equality, old checkpoint isolation, exact old
retry with no new publish, changed-payload and changed-actor rejection, cache
failure fencing, active-work pinning, bounded snapshot construction, retained
transceiver checkpoint, cache cleanup and cold effect-object 24-hour retention.
See [growth and safety output](22-memory-artifacts/growth.txt).

`TestServerNATSArchivedCommandAfterRestart` executes 641 real strip commands until
the original outcome leaves the resident set, saves a complete verified snapshot,
and restarts an owned backend. An authenticated frontend status query still finds
the original outcome; retry returns the same aggregate revision, changed payload
is refused, and neither retry changes the strip. Both HTTP outcome endpoints find
the original result. See [full native suite log](22-memory-artifacts/tests.jsonl).

## Captured-state CPU and full-backend memory

The private retained-state fixture loads 4,162 airport entities, 18,086 outcomes
and 6,911 workflows, plus 235 global entities, 9,477 outcomes and 951 workflows.
Both real backends retain 1,024 outcomes and 1,024 completed workflows across the
two aggregates. Their caches remain 16 MiB during the 20-second idle sample.
The original base uses 1.9169 CPU cores combined; the fixed processes use 0.0130 combined (99.3% reduction). Live Go heap falls from 17,493,592 to 12,232,576 bytes on node 0 and from 15,068,648 to 10,783,984 bytes on node 1 over the sample. These are ordinary allocation/GC snapshots, not forced-collection samples. Fixed binary SHA-256: `d31f396d5fff7a16ec13d4e478ebeaf71369c94b7f9138997f3f1705a4952ab5`.

The fixture uses isolated resources and padding to reproduce checkpoint sizes;
padding is not a restoration or operational-history proof. External providers are
disabled in this comparison. The native fault suite separately proves recovery
and restore with its own accepted operational history. Private checkpoint bytes,
executables and raw profiles are not committed. See
[sanitized CPU/memory counters](22-memory-artifacts/idle.txt).

## Validation and limits

`go test ./...`, `go build ./...` and race checks for cluster, app, frontendbinary,
euroscopebinary, services and PDC pass. Complete Task 22 qualification passes in 832.589 seconds: all 11 top-level cases and eight death-boundary subcases, with no skips. The run includes the actual 305-second broker outage, expired-lease recovery, archived status/retry after restart, strict snapshot/version fences, stand/AMAN races, encrypted separate-cluster restore and base/current executable overlap.
See [machine/source metadata](22-memory-artifacts/machine-source.json),
[fault/restore output](22-memory-artifacts/fault-restore.txt) and
[complete JSON test log](22-memory-artifacts/tests.jsonl).

Windows 11, Ryzen 7 7700X, 16 logical processors, 33.46 GB RAM, Go 1.25.7 and
native NATS 2.15.0; one physical host. This is local correctness and retained-state
qualification, not production capacity certification. The shared development
Docker containers still run their earlier executable and were not restarted or
replaced. No merge, release or deployment was performed.
