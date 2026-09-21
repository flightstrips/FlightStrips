# Observability

The backend exports metrics, traces and logs over OTLP. Endpoint, protocol and
headers come from the standard `OTEL_EXPORTER_OTLP_*` environment variables, so
nothing is configured in code. Dashboards live in `grafana/dashboards/` and are
Grafana schema v2 documents.

| Dashboard | Answers |
| --- | --- |
| `flightstrips-backend.json` | Are connections, messages and PDC healthy? |
| `flightstrips-traffic.json` | What is the traffic situation at the airport? |
| `flightstrips-usage.json` | Who is connected, and for how long? |
| `flightstrips-performance.json` | Why is the backend busy? |

## Diagnosing high CPU

The performance dashboard is meant to be read top to bottom. Each row narrows
the question rather than repeating it.

### 1. Is the server actually CPU bound?

Use the host metric for the saturation decision. The node exporter is already
present in Grafana, so the dashboard uses the idle time across all host CPUs:

```
1 - avg(rate(node_cpu_seconds_total{job="integrations/node_exporter",mode="idle"}[$__rate_interval]))
```

The Go `go.cpu.time` classes are runtime attribution estimates and can be
useful for relative diagnostics, but they are not directly comparable to host
CPU time and must not be presented as server CPU utilisation.

Then read the class split:

- **`user` dominates** — application code is the cost. Continue to row 2.
- **`gc` is a large share** — the process is allocating faster than it works.
  Cross-check `go.gc.cycles` and heap size; the fix is allocation, not throughput.
- **`scavenge` is visible** — memory is being returned to the operating system
  under pressure, usually a symptom of a heap that has just shrunk sharply.

`go.schedule.latency` is the saturation signal. It measures how long a runnable
goroutine waited for a processor, so it rises *before* throughput falls. High
scheduling delay with low CPU utilisation means goroutines are blocked, not busy
— check `go.mutex.wait.time` for lock contention.

### 2. What is it working on?

The Work Amplification row exists because the expensive failures in this system
have not been slow code — they have been correct code running far more often
than it needed to.

- **`euroscope.sync.outcomes`** separates syncs that changed persisted state from
  heartbeats that did not. EuroScope syncs on a fixed cadence, so a high
  unchanged rate is normal by itself.
- **`euroscope.sync.follow_up_work`** counts what each sync queued for its
  finalization phase, by kind. This is the amplification factor between one
  inbound message and the work it causes.
- **Recalculations per unchanged sync** is the ratio of the two. A sync that
  changed nothing should not need a full airport recalculation, so a non-zero
  value here is repeated work regardless of how fast each recalculation is.
- **`euroscope.sync.phase.duration`** attributes a slow sync to one stage. The
  phases sum to the total, so the tallest line is the one worth profiling.
- **`cdm.recalculation.duration`** summed as a rate gives processor-seconds per
  second: a value of 1 means one processor did nothing but CDM recalculation.

### 3. Is dispatch the bottleneck?

Both hubs dispatch from a single goroutine, so everything they send is
serialised behind one loop.

- **`websocket.hub.queue.depth`** is the backlog that loop is working through.
  Both queues hold 256 messages; a p99 approaching that means the hub can no
  longer keep up with its publishers.
- **`websocket.hub.broadcast.fanout`** explains dispatch cost growth without any
  code change — more clients in a session is more work per message.
- **`websocket.hub.publish.blocked`** counts publishers that had to wait for room
  in the queue. This is measured only when the queue was actually full, so any
  sustained rate means back-pressure is now stalling the code producing events.
- **`websocket.clients.slow_disconnects`** counts clients dropped for failing to
  drain their own send queue.
- **`websocket.message.bytes.received`** and **`websocket.message.bytes.sent`**
  count serialized application payload bytes received from clients and accepted
  by the WebSocket writer. **`websocket.message.size.bytes`** records the
  corresponding payload-size distribution. All three use only the bounded
  `direction`, `source`, and `type` labels. These measurements include the JSON
  or protobuf payload itself, but exclude WebSocket frame headers, TLS overhead,
  and any reduction from WebSocket compression.

### 4. Is it the database?

`pgxpool.acquired_connections` against `pgxpool.max_connections` shows pool
saturation, and `pgxpool.empty_acquire` counts acquires that had to wait for a
connection. A saturated pool turns database latency into application queueing,
which presents as a slow backend rather than a slow database.

Individual query spans come from `otelpgx` and are enabled whenever
`OTEL_EXPORTER_OTLP_ENDPOINT` is set.

### Aircraft position query budget

Only the session's master EuroScope client sends aircraft-position events. The
plugin checks this both when flushing positions and at the WebSocket send
boundary; the backend ignores position events from other clients. Direct-to and
assigned-speed facts instead come from the aircraft's tracking controller,
which may be a slave. Live TopSky `/HOLD/`, `/XHOLD/`, and `/HOLD_EAT/`
scratch-pad commands are broadcast to every operational controller, so the
session master publishes them once. Tracking controllers may reconcile active
holds from annotation 6, but a missing annotation never clears stored state.
Persisted hold fields are included in backend sync so a newly elected master
can apply a later standalone EAT. Commands observed during a backend outage are
replayed after reconnect. The backend accepts holding reports from either
authority. Hold fields in
full-sync and strip-update snapshots continue to follow tracking ownership so a
non-tracking snapshot cannot replace or clear the stored hold. Deploy the
backend and plugin changes together.

`aircraft_position_update` reads a typed strip/stand-assignment snapshot and
persists position, bay and `euroscope_seen_at` synchronously. The routine budget
is **two database operations**, or **three with an established AMAN identity**.
Unchanged stand-conflict evaluation may add one read. Genuine transitions,
identity changes and optimistic retries have separate budgets. Routine movement
inside the same routing region does not reload route state; failed refreshes
remain pending for a later report.

The Performance dashboard separates completion latency (socket receipt through
persistence and synchronous effects), queue delay, completed messages/sec and
sampled queue depth. Existing handler-duration sums measure accumulated wall
seconds across handlers; concurrent handlers can accumulate more than one
second per second. They are neither per-message latency nor CPU utilisation.
DB operations count actual pgx calls, including transactions and synchronous
publication reads. Traces use `OTEL_SERVICE_VERSION`, or embedded git revision
with a dirty suffix when available, instead of a fixed release string.

Lifecycle processing remains sequential until the concurrency load gates in
[the position performance runbook](../docs/position-performance.md) pass.
Position database work is batched by default; set
`POSITION_DB_BATCHING_ENABLED=false` to restore individual statements.
`POSITION_CONCURRENCY_ENABLED=true` selects four workers per connection;
`POSITION_WORKERS_PER_CLIENT=1` restores sequential dispatch while retaining DB
improvements. The shared position limit is eight, reduced for pool headroom.
Every dedicated report is processed without coalescing, with per-aircraft FIFO,
a 256-pending-report connection limit, while non-position operational messages
bypass the position dispatcher. Aircraft disconnects drain accepted positions
first to preserve lifecycle ordering.

Failed handler samples carry the bounded `error_class` label, including
`serialization_conflict`, `deadlock`, `missing_row`, and `coordination`, so
terminal failures can be counted without parsing log messages. Successfully
retried serialization conflicts and deadlocks are counted separately by
`websocket.message.db_retries`.

## Traces

### AMAN work during strip synchronization

Full EuroScope syncs collect holding-clearance observations and apply them in
one AMAN commit and publication per destination airport, rather than once per
changed flight. Unchanged batches load the airport state once and do not commit
or publish. Revision conflicts reload and reapply the batch, retaining newer
concurrent clearances. A partial strip-sync failure still flushes observations
for the strip writes that already succeeded.

Each changed AMAN projection uses one bulk flight upsert statement inside the
existing transaction. Timeline and holding-EAT projections share one navigation
geometry snapshot per publication; the next publication reads a fresh snapshot.

`euroscope.sync.db_operations` and the `strip_update` and
`aircraft_position_update` samples in `websocket.message.db_operations` count
actual pgx query calls during the handler, including nested AMAN queries and
transaction commands. Detached work after the handler completes is excluded.
Syncs retain manual accounting when no pgx tracer is installed. The expanded
coverage means these counts are not directly comparable with older sync and
strip-update metrics, which omitted downstream work. Pool acquires remain a
separate measure: multiple statements inside a transaction share one acquire.

For verification, compare LIVE sync/strip-update duration and pool acquire rate
at similar traffic levels. In a full-sync trace, holding changes should produce
at most one `UpsertAMANFlights` per affected airport without a revision retry,
and no sequence of individual `UpsertAMANFlight` calls.

### Trace coverage

Spans exist for the WebSocket upgrade, every WebSocket message, EuroScope sync,
CDM recalculation, and every database query. The sync span carries the same
phase breakdown and follow-up work counts as the metrics, as attributes, so one
trace explains a single slow sync while the dashboard explains the trend.

Logs carry `trace_id` and `span_id` as structured metadata, so a warning in Loki
links directly to the trace that produced it.

Sampling is the SDK default of "sample everything", which is fine at current
volumes. It is configurable without a code change through the standard
environment variables when it stops being fine:

```bash
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.1
```

## Profiling

The dashboard identifies *which path* is expensive. When the answer needed is
*which function*, the backend serves `net/http/pprof` on `127.0.0.1:6060`. It is
off by default and bound to loopback, so it is reachable only from inside the
container:

```bash
docker compose -f docker-compose.prod.yml exec backend wget -qO /tmp/cpu.pprof http://127.0.0.1:6060/debug/pprof/profile?seconds=30
```

Enable it with `ENABLE_PPROF=true` and restart the backend.

## Metric label discipline

Metric attributes use a closed vocabulary. Callsigns, CIDs, route text, command
identifiers and provider payloads are never metric dimensions — they belong in
span attributes and log fields, where high cardinality is free. `fixedLabel` in
`backend/internal/metrics/metrics.go` collapses any unexpected value to `other`
so a new enum value can never multiply a time series.
