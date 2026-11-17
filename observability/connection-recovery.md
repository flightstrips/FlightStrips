# Connection recovery and incident evidence

The November 16 disruption established simultaneous connection failures, not
their network cause. These changes bound silent failures and preserve enough
evidence to correlate failures during another event. They cannot prevent an
upstream outage or guarantee the outcome of an action sent before a disconnect.

## Recovery behavior

Browsers retain the last board, manual layout and surviving passive strip
selection. The retained board is read-only until a snapshot for the selected
session, airport, compiled definition and exact subscription arrives. A snapshot
replaces strip ownership and other shared state from the server. A different
board or a removed strip clears passive selection. EuroScope aircraft selection
in a snapshot remains authoritative for the AMAN selection.

Each browser socket has a 15-second open deadline and 30-second initial snapshot
deadline. Backends advertising connection-health protocol 1 receive a heartbeat every
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
count admitted connection failures, excluding normal close frames. Counts describe connections, not distinct controllers.

## Alerts and rollout

Import [connection-alerts.yml](prometheus/connection-alerts.yml) into the Grafana
Cloud metrics ruler and route notifications through the existing contact point.
The rules detect at least five unexpected closes/read timeouts in two minutes
and at least three master assignments across sessions in five minutes. Planned
deployments and controller changes can explain a cluster; correlate alerts with
the structured logs before attributing a cause.

Validate rules with `promtool check rules connection-alerts.yml` and
`promtool test rules connection-alerts.test.yml` from `observability/prometheus`.
The scenarios cover disconnect clusters, normal close traffic and master churn.
No production alert or notification route has been changed.

During an event, correlate read-timeout/unexpected-close events and master
assignments by connection ID, CID, session, CF-Ray and timestamp. Compare the
client log/console with backend logs and deployment history. Export the relevant
logs before retention expires. These diagnostics show which connections failed
together; they do not by themselves identify a network component as the cause.
