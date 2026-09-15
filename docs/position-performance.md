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
