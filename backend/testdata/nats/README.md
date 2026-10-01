# Local NATS resource fixture

The default [Windows development setup](../../Readme.md) runs three pinned
`nats:2.15.0` brokers and two real backends. Bootstrap is a separate administrator
operation. Runtime credentials verify resources and cannot repair drift. Their
publish allowlist includes JetStream flow-control replies only for the required
state, position, presence, snapshot-index and object resources; these replies
keep ordered replay moving after sustained delivery.

From `backend`, `./testdata/nats/verify.ps1` uses the broker-only Compose project
`flightstrips-nats-01`, exercises repeated bootstrap, resource drift, replicated
writes with one/two brokers stopped and retained-resource restart. It leaves that
fixture running. Stop it with:

```powershell
docker compose -f docker-compose.nats.yaml -p flightstrips-nats-01 stop
```

`NATS_URLS` supplies comma-separated backend URLs. TLS/credentials use
`NATS_CREDENTIALS_FILE`, `NATS_CA_FILE`, `NATS_CLIENT_CERT_FILE`,
`NATS_CLIENT_KEY_FILE`; deadlines use `NATS_CONNECT_TIMEOUT` and
`NATS_REQUEST_TIMEOUT`. The committed fixture passwords are local-only.

For administrator bootstrap, set `NATS_URLS` to the three
`nats://bootstrap:bootstrap-local-only@127.0.0.1:4222` through `:4224` URLs and run
`go run ./cmd/nats-bootstrap`. Restore backend credentials before server startup.
`cmd/nats-projection` remains an isolated diagnostic process; set
`NATS_PROJECTION_LISTEN` to avoid real backend ports. The real server always starts
NATS and checks readiness; the former preparatory flag and SQL runtime are retired.

Each backend builds a disposable local history index from verified snapshots and
the retained `FS_STATE` log. Set `NATS_HISTORY_CACHE_DIR` to a writable disk
directory; the default is the operating system's temporary directory. Avoid a
memory-backed filesystem if the goal is reducing container memory. The index is
not a durability boundary and need not be backed up. Normal shutdown removes it;
startup removes unlocked caches left by stopped processes while preserving files
locked by live backends. Disk usage grows with retained history. An unavailable
index prevents readiness and writes.

Each aggregate keeps 512 completed outcomes, workflows and effects per kind in
memory, plus all pending workflows, nonterminal effects and their outcomes.
Historical lookups consult the index, including command retries and effect result
acknowledgments. Command identities and `FS_STATE` history never expire. Provider
quota counters older than 48 hours are retired by accepted events; new requests
for expired quota windows are refused, and retries still use their original
outcome. Current entities and unfinished work still occupy memory.

Snapshots keep the existing complete canonical format. Once full history exceeds
the 32 MiB object limit, the backend skips that snapshot optimization and recovers
from an earlier verified snapshot and the retained log. Monitor
`fs_snapshot_size_skips_total` for this fallback and increased recovery time.
`fs_go_heap_alloc_bytes` measures live Go allocations; `fs_go_heap_sys_bytes`,
`fs_go_heap_idle_bytes` and `fs_go_heap_released_bytes` distinguish allocator
capacity from retained objects. `fs_history_cache_bytes` measures the local disk
index, and `fs_projection_hot_history_records{kind=...}` measures resident history.
The existing `fs_effects` gauge counts resident effects. Process/container RSS also
includes allocator capacity and reclaimable file pages, so it can fluctuate even
when live history allocations stay bounded.

Set `NATS_INTEGRATION=1` for real integration tests, `NATS_TEST_PORT_BASE` for a
non-default fixture port and `NATS_FAULT_PROJECT` only for the exact **disposable**
Compose project the fault tests may stop/restart. Never fault a shared fixture.
