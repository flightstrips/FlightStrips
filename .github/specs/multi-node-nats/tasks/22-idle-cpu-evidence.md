# Retained-state idle CPU investigation

The `flightstrips-local` backends repeatedly copied retained history while idle.
On 2026-10-01, Docker reported approximately 175–200% CPU for backend A and
140–150% for backend B. Their Compose labels point to the separate `1c7e`
worktree. The running Linux executable has no VCS build metadata, so its exact
source revision cannot be established from the binary. Its SHA-256 is
`e517a7233580c7a20e429db7b39a23525352750566d447518d934abf5344e5a7`.
The existing containers and their resources were not stopped or changed.

## Cause and change

`Projection.Read` and applying each accepted event used
`Aggregate.Snapshot` followed by `aggregateFromSnapshot`. A routine read thus
deep-copied all entities, outcomes, workflows and effects, sorted and serialized
the snapshot, computed its digest, validated it and copied the records again.
Ownership admission and lease maintenance repeatedly took this path. Session
discovery also read the entire global aggregate every 250 ms. Provider owner
and checkpoint checks repeated full reads every second.

The captured airport checkpoint had 4,162 entities, 18,086 outcomes and 6,911
workflows; the global checkpoint had 235 entities, 9,477 outcomes and 951
workflows. A benchmark on the actual airport checkpoint measured:

| Operation | Time per operation | Allocated bytes | Allocations |
| --- | ---: | ---: | ---: |
| Original snapshot round-trip | 63.39 ms | 33,999,936 | 368,505 |
| Detached full in-memory read | 26.63 ms | 14,123,343 | 160,043 |
| Focused owner checkpoint | 501 ns | 352 | 7 |
| Copy before applying an event | 267 microseconds | 1,311,045 | 106 |

The fix uses focused detached owner/entity reads for idle checks and registry
discovery. Full reads still return detached records. Event application detaches
the mutable owner and map structures and shares immutable protobuf records;
the reducer replaces changed records. Snapshot loading and event validation
remain strict, and ownership checks retain their readiness, lease and CAS
barriers. This removes recurring work proportional to old command history
without reducing heartbeat or ownership-check frequency.

## Isolated process comparison

The opt-in `TestRetainedStateIdleCPU` loads private verified copies of the
captured checkpoints into a disposable three-broker cluster, then measures two
actual backend processes with the original base and fixed implementations.
It checks that each checkpoint really loads, rather than silently falling back
to empty state. Sequence padding establishes the checkpoint positions; it is
not an operational event history or a backup/restore proof. External providers
are disabled and operational object bodies are not copied into the fixture.

After five seconds of warm-up, combined process CPU is measured over a
20-second profiling window using each owned process's CPU counter. A core is
one CPU-second per wall-clock second; Docker's 100% corresponds to one core.
The measurement includes both backends, but excludes the three NATS processes.
Both versions run sequentially on the same fixture and physical host.

The committed source `2c788ecaaceae382b90d4dd1fbc4c44d6b829a64` measured:

| Version | Combined CPU cores | Process CPU seconds | Wall seconds |
| --- | ---: | ---: | ---: |
| Original base `db662817` | 1.6982 | 35.1094 | 20.6749 |
| Fixed implementation | 0.0145 | 0.2969 | 20.4347 |

This is a 99.1% reduction, approximately 1.5% of one core across both backends.
The preceding run of the same runtime measured 1.8195 versus 0.0092 cores
(99.5% reduction). Both runs passed the same regression gate.
The regression gate requires less than 0.05 combined cores and at least 95%
reduction. This is a local idle reproduction on the Windows qualification host,
not a production capacity claim or a measurement of the updated default Docker
stack. The default stack still runs its original executable.

The committed-source comparison took 70.97 seconds, including setup, warm-up,
profiling both versions and cleanup. The fixed binary SHA-256 was
`2f5b17e4c1162f20accb6b033eeb12a454cf1c93322c376a4c99217f1131cdf4`;
the actual base executable SHA-256 was
`0b0fc5e72fe9b9dda10151bceef0ccf07de860762761a47d8b317f9f8680c7d0`.
The [committed idle log](22-cpu-artifacts/idle.txt) records checkpoint sizes,
binary hashes, process counters and cleanup. Private checkpoint files,
copied executables and raw profiles stay under ignored `backend/.task22` and
are not committed.

## Validation and reproduction

`go test ./...` and the focused projection tests pass. `go test -race` passes
for `internal/cluster`, `internal/app`, `internal/euroscopebinary` and
`internal/frontendbinary`. New regression cases check reader detachment,
readiness rejection, prior-snapshot immutability during entity changes and lease
renewals, and prior effect/outcome/throttle immutability during dispatch and
terminal transitions.

The complete Task 22 fault/recovery qualification **PASS** on source
`2c788ecaaceae382b90d4dd1fbc4c44d6b829a64` in 817.181 seconds: all 10 top-level
cases and eight fault-boundary subcases passed, with no skips. This includes the
actual 305-second broker outage, expired-lease recovery, strict snapshot/version
fencing, private-message dispatch death boundaries, stand/AMAN races, encrypted
separate-cluster restore and actual base/current executable overlap.
Run ID: `20261001T193616Z-557a9694e1c3491fba23da1d4dcca8e6`.
The runner recorded a clean source tree and empty tracked diff at launch.
See [machine/source metadata](22-cpu-artifacts/machine-source.json),
[fault/restore evidence](22-cpu-artifacts/fault-restore.txt) and
[complete JSON test log](22-cpu-artifacts/tests.jsonl).
The historical qualification in
[22-fault-integration-evidence.md](22-fault-integration-evidence.md) remains
specific to its recorded source revision.

To reproduce with privately captured, verified checkpoint files:

```powershell
cd backend
$env:NATS_INTEGRATION = '1'
$env:NATS_TASK22 = '1'
$env:NATS_SERVER_BINARY = 'C:/Users/fsr19/go/bin/nats-server.exe'
$env:NATS_IDLE_SOURCE_DIR = (Resolve-Path .task22/diagnostics).Path
go test ./cmd/server -run '^TestRetainedStateIdleCPU$' -count=1 -timeout=10m -v
$env:NATS_PROFILE_SNAPSHOT = (Resolve-Path .task22/diagnostics/airport.EKCH.pb).Path
go test ./internal/cluster -run '^$' -bench '^BenchmarkProjection' -benchmem
```

The idle fixture never connects to the source cluster. Its cleanup stops only
the exact broker/backend child processes it created. No merge, release or
deployment was performed as part of this investigation.
