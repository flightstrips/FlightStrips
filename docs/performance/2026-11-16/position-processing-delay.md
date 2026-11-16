# Position processing delay investigation

## Production evidence

Source: Tempo `grafanacloud-traces`, Loki `grafanacloud-logs`, and Prometheus
`grafanacloud-prom`; 2026-11-16 21:10:15 Copenhagen (20:10:15Z), session 1292,
service `3.9.1+882a1238a602fb3627af14b45de61957513f7184`.
The inspected checkout was `05ab62a5`; its position execution, batching and
position service paths match the deployed revision before these changes.

Trace `20367c373fb6ee26e192135360620b0f`, root `78ea06e808a67b88`, is OEO361,
identified by its correlated `UpdateAircraftPosition` log. Its 2385.060 ms total
contains 290.393 ms before the handler timer and 2094.661 ms processing.

| Interval within processing | Wall time |
| --- | ---: |
| Handler timer to snapshot start | 49.7 ms |
| Physical snapshot batch, three aircraft | 1.3 ms |
| Snapshot completion to OEO361's computed-bay log | 1092.1 ms |
| Computed-bay log to persistence start | 89.1 ms |
| Physical persistence batch, three aircraft | 3.9 ms |
| Persistence completion to root completion | 858.6 ms |

The snapshot finishes at 20:10:15.620671Z; the computed-bay log is at
20:10:16.712721Z. Persistence runs from 20:10:16.801784Z to
20:10:16.805722Z. SQL statements total 4.883 ms; pool acquisitions total
0.014 ms. These overlap the physical batch rows and must not be added again.

`ReadPump` starts processing timing inside the dispatched callback, after
initial admission. A batch member then releases its execution slot and later
reacquires it. These later waits, batch rendezvous and locks all remain inside
`message.processing_ms`. The one-millisecond collection window and
five-millisecond rendezvous timeout do not bound readmission delay. A report
can spend most of its processing time waiting for peers' lifecycle work.

Production peer traces confirm repeated no-op stand-release transactions:

| Peer trace | Session row lock wait | Release-marker rows inserted | Stand assignments found |
| --- | ---: | ---: | ---: |
| `35a4ba65467f50b3b27501d6723018d1` | 35.535 ms | 0 | 0 |
| `1163de9adc33c9811056f74b1ef9b2b8` | 32.732 ms | 0 | 0 |
| `890a65407ca0010ee5b466f0919eabbb` | 88.992 ms | 0 | 0 |

The original batch's other members,
`0edbcf2bebe558ed84fda91ed35742de` and
`458d9799537c505a909cd0d61a571cce`, also finish around 2.385 seconds after
receipt. Their stand-release transactions occur near the end of that interval.
Thus fast SQL in OEO361's own trace does not establish fast peer processing.

**Confirmed:** processing includes internal waits; repeated no-op departures
take the shared session database lock; transition-mutex waiters retain scarce
execution slots in the original code. Both blocking paths reproduce in tests.
**Inference:** OEO361's snapshot-to-decision gap is principally execution-slot
readmission behind those peers. The intervening code is readmission plus
message-local caching and in-memory workflow/route decisions. The old spans
cannot quantify each component, or divide the final 858.6 ms among readmission,
transition locking and AMAN/arrival handling. No profiles were available for
this interval. Runtime p99 scheduler latency was about 0.025 ms and p99 GC pause
about 4.2 ms; these aggregate values cannot exclude individual outliers.

## Changes

- A repeated stand release checks the persisted release marker and absence of
  a departure assignment in one fresh database snapshot. A completed no-op
  avoids the six-statement serializable transaction and session write lock.
  First releases, reset markers and existing departure assignments retain the
  locked transaction. Arrival assignments remain intact. No cross-message
  cache is introduced.
- Transition-mutex waiters release their execution slot, then reacquire it
  before continuing. The transition lock remains held through its existing
  lifecycle work, and batching stays disabled under that lock.
- New spans expose batch waiting, execution readmission (local/global timing
  and reason), membership/session/aircraft locks, transition locking, snapshot,
  decisions, route, departure, AMAN, arrival and visibility phases. Batch wait
  spans end before readmission starts. Enclosing phases overlap their child
  work; do not sum all phase durations. Receipt, queue and processing timer
  definitions are unchanged.
- The load harness selects enabled airport services explicitly, reads
  application-owned SAT readiness/resources, counts current otelpgx SQL spans
  by query attributes, and reports phase timing distributions.

## Verification

Both targeted regressions were run before applying the fixes:

- With a session row locked by another transaction, an already completed
  release timed out at 200 ms. With the fix it completed in 0.762 ms with one
  SELECT. The test also covers marker reset, removal/publication of a new
  departure assignment despite an existing marker, and preservation of an
  arrival assignment.
- A transition-mutex waiter blocked an independent report for the entire
  200 ms observation window. With the fix the independent report completed in
  1.560 ms while the transition remained locked. The blocked report resumed
  only after unlock.

Affected PostgreSQL-backed package suites passed: `shared`, `websocket`,
`euroscope`, `services`, and `repository/postgres`. Tracing checks cover expired
participant cancellation, root cancellation semantics for physical batches,
trace parentage, and SQL counting through nested phases.
The same packages plus `testing/positionload` also passed with `-race`.

Paired load evidence is saved beside this report. Both variants use the same
instrumentation and corrected harness; the baseline removes only the no-op
release guard and transition-slot yield. Settings: one worker, default database
batching, 200 aircraft, 50% arrivals, 100 reports/sec in once-per-second bursts,
five interleaved controls, two-second warm-up, ten-second measurement, followed
by 200/sec overload and recovery (3400 reports total). This is a short local
smoke comparison, not a production replay or a release-capacity qualification.
The database runs in Docker on Windows loopback.

| Measured result | Baseline | Fixed |
| --- | ---: | ---: |
| Mean processing | 73.129 ms | 31.192 ms |
| P95 receipt-to-completion | 226.761 ms | 64.685 ms |
| P99 receipt-to-completion | 245.651 ms | 71.239 ms |
| P95 sender-deadline-to-completion | 231.220 ms | 68.066 ms |
| Slowest steady-state batch | 257.556 ms | 78.255 ms |
| Physical SQL statements/report | 3.005 | 0.753 |
| Mean readmission after snapshot | 7.418 ms | 2.036 ms |
| Mean readmission after persistence | 21.215 ms | 1.866 ms |
| Mean departure lifecycle work | 2.329 ms | 0.491 ms |
| Completed / handler errors / operational errors | 3400 / 0 / 0 | 3400 / 0 / 0 |

The measured window contains 1000 reports. Phase durations overlap enclosing
operations. Transition-lock waiting becomes visible after yielding admission:
the fixed run still averages 16.337 ms there. The fixes reduce the cost and
spread of serial lifecycle work; they do not remove lifecycle serialization.

Reproduce from `backend`:

```powershell
$env:POSITION_LOAD_TEST='true'
$env:POSITION_WORKERS_PER_CLIENT='1'
$env:POSITION_LOAD_WARMUP='2s'
$env:POSITION_LOAD_DURATION='10s'
$env:POSITION_LOAD_PATTERN='second-burst'
go test ./internal/testing/positionload -run '^TestPositionLoad$' -count=1 -v -timeout 4m
go run ./internal/testing/testdb ./internal/shared ./internal/websocket ./internal/euroscope ./internal/services ./internal/repository/postgres -count=1
```

The existing 20 ms P95 / 50 ms P99 latency targets remain enforced. A completed
run that fails those assertions remains a failed performance gate. No
production deployment or infrastructure configuration change is included.
