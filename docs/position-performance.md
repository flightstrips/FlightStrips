# Aircraft position throughput

PostgreSQL remains authoritative. Each dedicated position report persists its
position and EuroScope presence synchronously. No persistence batching or
cross-message strip/stand/identity cache is introduced.

## Rollout

Leave concurrency disabled until correctness and production-equivalent load
gates pass. The default is one worker. Set `POSITION_CONCURRENCY_ENABLED=true`
to select four workers per client; `POSITION_WORKERS_PER_CLIENT=1` restores
sequential execution without reverting database improvements. An explicit
`POSITION_WORKERS_PER_CLIENT` accepts 1 through 8 and overrides the default.

The backend limit is `min(8, pool_max_conns - 2)`. Pools must configure at least
three connections. Each connection holds at most 256 pending jobs plus its
running workers. When full, its single socket reader waits. Dedicated reports
are never dropped or coalesced. Completion includes waiting for a worker and the
backend limit. Cross-aircraft completion can differ from receipt order.

FIFO keys are session and normalized callsign. Operational messages drain prior
positions and pause timer callbacks while the operational handler executes.
Delayed full-strip positions share the dispatcher; a newer dedicated receipt
invalidates an older delayed snapshot. Each job carries receipt time and an
authority epoch, checked under the master-transition fence before persistence.
Losing and regaining master does not resurrect old jobs. Cancelled jobs are
counted as errors with `error_class=cancelled`.

Shutdown drains accepted positions for up to five seconds before cancelling,
while hubs and PostgreSQL are still available. The socket retains one reader and
one writer, following [Gorilla's concurrency contract](https://github.com/gorilla/websocket/blob/main/doc.go).

## Database paths and review order

1. **Measurement:** `internal/testing/positionload`, configurable E2E setup,
   receipt/completion/queue telemetry, release identity and Performance dashboard panels.
2. **Database work:** one typed strip/optional-assignment snapshot; one combined
   position/presence write; bounded optimistic retries; assignment snapshot reuse;
   read-only unchanged conflict detection; one-query established AMAN identity;
   context-preserving synchronous publication.
3. **Concurrency:** the shared dispatcher, EuroScope authority fences, session
   transition serialization and shutdown. Bay sequence allocation and writes
   share a session-row transaction lock across position, frontend, runway,
   full-strip and tactical append paths.

Routine repository budgets are two queries, or three including an established
AMAN identity. Unchanged stand conflict evaluation may add one read. Transitions
and retries have separate budgets. The PostgreSQL query-budget test asserts the
two-query position path and the one-query identity fast path. Exhausted position
version conflicts and missing strips are failures, not successful completions.
Route refresh failures retain the existing pending retry behavior.

## Reproduce

Run from `backend` with Docker available. The test creates and cleans an isolated
PostgreSQL database. Its test authentication, synthetic VATSIM scenarios and
navigation fixtures avoid production credentials.

```powershell
$env:POSITION_LOAD_TEST='true'
$env:POSITION_CONCURRENCY_ENABLED='true'
$env:POSITION_WORKERS_PER_CLIENT='4'
$env:POSITION_LOAD_WARMUP='2m'
$env:POSITION_LOAD_DURATION='15m'
$env:POSITION_LOAD_ARRIVAL_PERCENT='50'
$env:POSITION_LOAD_REPORT='C:\temp\positions-mixed.json'
go test ./internal/testing/positionload -run TestPositionLoad -count=1 -v -timeout 25m
```

Repeat with arrival percentages 80 and 20. The open-loop sender distributes 200
aircraft evenly at 100 reports/sec, interleaves five EuroScope heading messages
and one frontend action/sec, then sends 200/sec for ten seconds and returns to
100/sec. Setup completes a real login/sync and verifies frontend readiness.
Staggered aircraft exercise approach, touchdown, runway vacation, stand
reservations, push and airborne movement. AMAN uses socket receipt timestamps
for speed and track even when handler execution waits.

### EuroScope once-per-second batches

Set `POSITION_LOAD_PATTERN=second-burst` to send 100 reports back-to-back at each
one-second deadline, alternating halves of the 200-aircraft fleet. There is no
intentional spacing within a batch and sending does not wait for completion.
All five operational messages are interleaved within that same burst (one after
each 20 positions); frontend actions and lifecycle transitions remain enabled.
This is a synthetic ordering assumption, not a replay of a recorded ES batch.
The overload phase sends 200 reports at
each one-second deadline for ten seconds, then returns to 100-report batches.
The default `POSITION_LOAD_PATTERN=even` retains the original evenly spaced run.

Compare `POSITION_WORKERS_PER_CLIENT=1` and `4` on the same machine and database
placement. Keep latency enforcement enabled: a completed test with failed
latency assertions is evidence of a capacity gap, not a passing load gate.
JSON reports identify the traffic pattern and include mean processing and queue
time. They also measure completion from the sender's fixed deadline and whole
batch completion. The single socket reader's receipt timestamps identify wire
order, which is matched to the sender schedule after sorting completed spans.
This includes waiting in the socket while a barrier blocks the reader, plus
sender lateness and local network time. Latency enforcement checks both
receipt-to-completion and scheduled-to-completion, and each measured batch must
finish before the next second. Repeat the full warm-up/measurement and traffic mixes before claiming the
100-report batch target is met; the earlier evenly spaced results do not establish
that target.

The production infrastructure manifest at commit
`fad9ab78edae3e77ec182a28e1ee52991d1714df` specifies backend 2.2.8 and neither
`POSITION_CONCURRENCY_ENABLED` nor `POSITION_WORKERS_PER_CLIENT`. This selects
one worker, and the operator confirmed there are no runtime overrides. Direct
inspection of the running container environment was not available. Production
pool telemetry showed four maximum connections, limiting the shared position
budget to two even if more workers are configured.

#### Diagnostic batch results (2026-09-15)

Sequential local runs used the same 16-logical-CPU Windows/Docker host and pool
of 16 as above, ten seconds of warm-up and two minutes of measurement, mixed
arrivals/departures, then the overload/recovery phases. Each one-second batch
contains distinct aircraft; the 100-report phase alternates halves of the fleet
and the 200-report phase includes each aircraft once. The checkout is `a7cd3d36`
plus this load-tool change. CPU utilization and memory usage were not sampled.

| Workers | P95 from receipt | P95 from sender deadline | P99 from sender deadline | Maximum whole-batch completion |
|---|---:|---:|---:|---:|
| [1](performance/2026-09-15/second-burst-1-worker.json) | 25.07 ms | 137.35 ms | 149.22 ms | 167.67 ms |
| [4](performance/2026-09-15/second-burst-4-workers.json) | 7.64 ms | 52.81 ms | 67.20 ms | 90.17 ms |

Both runs completed exactly 15,200 reports with zero position or operational
errors, 650 heading messages and 130 frontend mark actions. Lifecycle assertions
passed, and each measured batch completed before the next second. **Both latency
gates failed.** Four workers passed the receipt-only gate but failed the sender
deadline gate, demonstrating why socket/barrier waiting must be included.
The overload target was already drained when the rate returned to 100/sec;
sub-millisecond reported drain times describe that checkpoint, not the cost of
processing a 200-report batch.

These are short diagnostic results, not the 15-minute production-equivalent
acceptance runs. They demonstrate stable throughput at this local load while
leaving the stricter latency goal unresolved. PostgreSQL can remain authoritative;
bounded database batches for independent positions are a candidate to measure
next, with operational barriers and per-aircraft ordering preserved.

The JSON report includes completed/error counts, latency percentiles, query and
pool-wait measurements, maximum outstanding work, sender lateness, observed
operational events and machine/database details. Event assertions check landing,
runway vacation, airborne movement and stand activation/release. Frontend and
operational errors also fail the test. `POSITION_LOAD_ENFORCE=false` disables
only the latency assertions for investigation; it does not turn errors into a
passing run.

For an approved disposable PostgreSQL instance on production-equivalent hardware,
set `FLIGHTSTRIPS_TEST_DATABASE_SERVER_URL` and describe CPU, storage and database
network placement in `POSITION_LOAD_TOPOLOGY`. The test creates isolated databases
on that server. Set `POSITION_LOAD_REVISION` to the exact release under test.
Test authentication remains enabled only inside this test application.

Container builds inject `BUILD_VERSION` through Go linker flags. Release CI
sets it to release version plus source SHA; ordinary image CI uses the SHA.
`OTEL_SERVICE_VERSION` can override this explicitly. Local Git builds fall back
to embedded revision/dirty status.

```powershell
go run ./internal/testing/testdb ./...
go run ./internal/testing/testdb -race ./internal/shared ./internal/websocket ./internal/euroscope ./internal/services ./internal/repository/postgres ./internal/aman/operational ./internal/frontend ./internal/app
```

## Acceptance

Require P95 <=20 ms and P99 <=50 ms for receipt-to-completion, zero unexpected
errors, exact sent/completed counts and a stable backlog for all three mixes.
Require burst backlog to drain within two seconds while 100/sec traffic resumes.
Review intermediate events as well as final positions. Local measurements do not
satisfy the production-equivalent CPU/storage/network gate. If that gate fails,
identify the measured queue, pool, database or synchronous-effects bottleneck
before changing persistence architecture.

## Local measurements (2026-09-15)

Raw reports are stored in `docs/performance/2026-09-15`.

The baseline is commit `6696a072`, which already includes routing-region
recalculation avoidance. Only the load/E2E tooling was copied into its isolated
checkout. Its AMAN test-clock call was changed to wall time because synthetic
scenario tooling freezes its default clock; without this fixture adjustment,
the baseline would skip subsequent AMAN speed observations. The baseline has no
new queue timing instrumentation, so its zero queue/processing fields mean
unavailable, not zero latency. Its root spans measure handler time.

These tests use a 16-logical-CPU Windows host and PostgreSQL 16.11 in Docker
Desktop, Go 1.25.7, a pool of 16, and loopback database access. Three revised
mixes run concurrently in separate applications/databases; the short baseline
also overlaps them. CPU and storage are shared and are not configured to match
production. Percentiles therefore are local regression evidence, not a
controlled estimate of production improvement.

CPU utilization and memory usage were not recorded; the CPU count does not
establish available headroom.

| Run | Measured duration | P95 ms | P99 ms | Mean DB operations | Completed | Max outstanding |
|---|---:|---:|---:|---:|---:|---:|
| [Baseline (one worker)](performance/2026-09-15/baseline.json) | 2m0s | 3.29 | 5.02 | 5.97 | 26,200 | 5 |
| [Mixed (50% arrivals)](performance/2026-09-15/mixed.json) | 15m0s | 1.77 | 2.61 | 2.46 | 104,200 | 8 |
| [Arrivals-heavy (80%)](performance/2026-09-15/arrivals-heavy.json) | 15m0s | 1.77 | 2.61 | 2.72 | 104,200 | 13 |
| [Departures-heavy (20%)](performance/2026-09-15/departures-heavy.json) | 15m0s | 1.78 | 2.52 | 2.21 | 104,200 | 8 |

All revised runs passed latency, exact-count and operational-event assertions:
312,600 dedicated reports completed in total, zero position or interleaved
operational errors. Each run included 5,100 heading messages and 1,020 frontend
mark actions, plus lifecycle messages. The resumed-100/sec burst drain checks
passed (about 10–11 ms at the sender's polling granularity). Maximum steady-run
outstanding work remained at 13 reports or fewer.

The long runs measured the database/dispatcher implementation before the last
shutdown fencing and publication-attribution refinements. The complete checkout
subsequently passed the full PostgreSQL suite and race checks for affected
packages; CI linker-version injection also compiled and passed its unit test.
The Backend dashboard is unchanged; all new panels are on Performance.

**Production enablement remains gated.** Repeat the 15-minute runs on the actual
release with production-equivalent database CPU, storage and network placement.
No production rollout or production-equivalent capacity claim is made here.
