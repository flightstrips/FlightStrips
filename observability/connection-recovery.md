# Connection recovery and incident evidence

The November 16 disruption established simultaneous connection failures, not
their network cause. These changes bound silent failures and preserve enough
evidence to distinguish paths during another event. They cannot prevent an
upstream outage or guarantee the outcome of an action sent before a disconnect.

## Recovery behavior

Browsers retain the last board, manual layout and surviving passive strip
selection. The retained board is read-only until a snapshot for the selected
session, airport, compiled definition and exact subscription arrives. A snapshot
replaces strip ownership and other shared state from the server. A different
board or a removed strip clears passive selection. EuroScope aircraft selection
in a snapshot remains authoritative for the AMAN selection.

Each browser socket has a 15-second open deadline and 30-second initial snapshot
deadline. Backends advertising connection-health protocol 1 receive a probe every
10 seconds, with a 30-second acknowledgement deadline. Acknowledged waiting for
EuroScope keeps the connection alive without reconnect loops. Recovery discovery
requests a fresh snapshot on the existing authenticated subscription. Older
backends fall back to reconnecting for that request; the application heartbeat
is enabled only after protocol negotiation. The server still sends native pings.

The plugin sends native pings 15 seconds after opening or receiving its previous
pong and retires a connection after 30 seconds without a pong. A 30-second first
application response deadline covers a successful upgrade followed by stalled
authentication. This bounds the first frame, not full membership readiness:
existing membership and sync fences continue to control operations. Timer
callbacks are fenced by physical connection generation. Both clients use bounded
retry jitter. Pending commands are cancelled; reconnect never blindly repeats a
human action. An uncertain frequency-message result explicitly tells the user to
check EuroScope before resending.

The server bounds the first authentication frame to 15 seconds. Deploy the
backend first, then browser and plugin releases. Rejected-action handling is a
separate change and remains unchanged here.

## Structured evidence

Backend opening, admission, read/write failures, recovery stages and master
changes include a random `connection_id`; available Cloudflare requests also
include `cf_ray`. The upgrade returns `X-Flightstrips-Connection-Id`, and browsers
receive the ID in the authenticated health welcome. The plugin logs both
upgrade headers. Browser console diagnostics include phase, prior failure,
attempt and jittered delay. IDs are per physical connection, not authentication
or membership identifiers.

`peer_ip` is the socket peer. `reported_client_ip` and `cf_ray` come from bounded,
untrusted headers and are diagnostic only; direct-origin requests can supply
them. Logs never include authentication frames or tokens. These fields, CIDs and
connection IDs are not metric labels. `last_pong_age_ms` helps distinguish a
recently responsive transport from a silent one.

`websocket.connection.events` exports bounded `source` and `event` dimensions,
including read timeouts, unexpected closes, admission/authentication failures,
client stages and master assignments. Under standard OTLP-to-Prometheus
translation the counter is `websocket_connection_events_total`. Cluster alerts
count admitted connection failures, excluding unauthenticated synthetic upgrades
and normal close frames. Counts describe connections, not distinct controllers.

## Public and direct-origin checks

The backend image also contains `/app/bin/connection-probe`. Its four concurrent
checks run every 15 seconds with five-second deadlines:

| Target | Check |
| --- | --- |
| public | HTTPS `/healthz` through normal DNS; WSS `/frontEndEvents` upgrade |
| origin | The same requests with only the TCP destination overridden |

Both targets use the public hostname for Host, SNI and certificate verification.
There is no TLS bypass, operational token, command, or database access. A
WebSocket probe closes immediately after HTTP 101; an authentication EOF on the
backend from `FlightStrips-Connection-Probe/1` is expected. The opening log marks
this untrusted User-Agent hint as `synthetic_probe`; it does not bypass any
authentication or change alert counts. `/healthz` HTTP 200
proves HTTP liveness, even when its optional subsystem report is degraded.
Neither check proves authentication, strip synchronization or controller readiness.

Required environment:

```text
CONNECTION_PROBE_PUBLIC_URL=https://api.flightstrips.dk
CONNECTION_PROBE_ORIGIN_ADDRESS=159.195.249.133:443
ENVIRONMENT=production
OTEL_EXPORTER_OTLP_ENDPOINT=<existing collector>
OTEL_EXPORTER_OTLP_HEADERS_FILE=/run/secrets/otel_exporter_otlp_headers
OTEL_METRIC_EXPORT_INTERVAL=15000
```

The command uses the existing OTLP HTTP exporter configuration. Configure the
same existing endpoint and secret, without giving the probe database or login
credentials. Gauges `connection_probe_success` and
`connection_probe_checked_at_seconds` have four `target`/`check` combinations;
`connection_probe_duration_seconds` is a histogram. Transition logs include HTTP
status, duration, CF-Ray, connection ID and network errors. Healthy repeated
checks do not produce repetitive application logs.

The infrastructure branch adds a disabled service in the authoritative Swarm
stack. Its initial pinned image predates the new binary: **leave replicas at zero
until the release PR updates all backend image references to a version containing
the probe**. Then set replicas to one and deploy. No port is published. Import
[connection-alerts.yml](prometheus/connection-alerts.yml) into the Grafana Cloud
metrics ruler and route notifications through the existing contact point.
Rules and the service are prepared here; no production service, alert or
notification route has been changed.

Validate rules with `promtool check rules connection-alerts.yml` and
`promtool test rules connection-alerts.test.yml` from `observability/prometheus`.
The scenarios distinguish path failures, stale evidence and missing monitoring.

After rollout, verify all four fresh series before enabling alerts. Confirm that
direct-origin TLS succeeds: an origin-only Cloudflare certificate or an
origin firewall permitting only Cloudflare would prevent this comparison.
Do not weaken TLS or broaden firewall access silently to make a probe green.
For better independent evidence run another copy outside the production host;
the on-host probe does not observe every controller's Internet route.

During an event, compare public vs origin success and timings, then correlate
the minute's read-timeout/unexpected-close events and master assignments by
connection ID and CF-Ray. Public failing with origin healthy narrows the failing
path; both failing also requires checking the monitor's own egress. Export the
corresponding client log/console and backend logs before retention expires.
