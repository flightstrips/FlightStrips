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

### 4. Is it the database?

`pgxpool.acquired_connections` against `pgxpool.max_connections` shows pool
saturation, and `pgxpool.empty_acquire` counts acquires that had to wait for a
connection. A saturated pool turns database latency into application queueing,
which presents as a slow backend rather than a slow database.

Individual query spans come from `otelpgx` and are enabled whenever
`OTEL_EXPORTER_OTLP_ENDPOINT` is set.

### Aircraft position query budget

`aircraft_position_update` uses one message-scoped snapshot for strips,
controllers, the session and sector owners. The core steady-state path performs
one strip read, one position write, four route-state reads (coordination,
session, sector owners and controllers), and one last-seen write: **seven
database operations**, plus a route-state write only when ownership changes.
An AMAN-enabled arrival with an established identity normally adds two identity
queries. Departure stand observation and bay, stand, landing, or coordination
transitions may perform additional reads and writes. The DB-operation metric is
counted at the pgx boundary, so these downstream operations remain attributed
to the originating message. Every path must still reuse or update the original
strip snapshot; a normal trace should contain exactly one `GetStrip` span.

The Backend dashboard shows position-update P50/P95/P99. The Performance
dashboard shows its total handler time and attributed DB-operation rate. Use
those panels together with query spans to compare releases; compare like traffic
classes against the stage budgets above. Repeated `GetStrip` spans indicate
renewed fan-out.
Failed handler samples carry the bounded `error_class` label, including
`serialization_conflict`, `deadlock`, `missing_row`, and `coordination`, so
terminal failures can be counted without parsing log messages. Successfully
retried serialization conflicts and deadlocks are counted separately by
`websocket.message.db_retries`.

## Traces

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
