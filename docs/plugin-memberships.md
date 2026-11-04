# Plugin membership protocol

MA-03 extends the existing typed binary envelope on the existing endpoint. The
backend and plugin must be deployed together during the agreed offline window.

`TokenEvent.instance_id` identifies one plugin process across reconnects. It is
not an authorization credential: the token supplies the authenticated CID.
The backend assigns a transport generation and a new membership ID/generation
for every admission. `Envelope.context` carries those values, the database
session, airport, complete environment identity, and current master epoch.
Queued messages capture this context at enqueue time. Receivers validate it
again when applying work. Neither a CID nor an airport alone identifies a source.

`ControlEvent` reports physical-instance identity and radio evidence, even with
zero airport memberships. It does not keep operational controllers or sessions
alive. `SessionInfoEvent` remains the typed role announcement, now scoped by the
envelope. `MembershipRemovedEvent` removes only its addressed incarnation.
Master epochs fence observations and full snapshots, while authorized controller
commands/results do not require mastership. There is no generic acknowledgement
or parallel protocol endpoint.

The transport exists before membership admission and stays connected after the
last membership leaves. Token expiry, actual EuroScope disconnect and network
reconnect still fence the physical connection. The existing startup delays and
retry backoff remain; an absent airport hint or unknown primary frequency does
not close the control connection. Control evidence updates on changes and at a
ten-second interval. The plugin reports its primary frequency separately from
complete tuned-radio evidence: an unknown tuned list is not a confirmed empty
list. Coverage and takeover policy consume these contracts in MA-04.

Only membership pongs update operational controller rows, using authenticated
CID, session and callsign together. Detachment clears operational presence;
admission and old-row cleanup serialize so reconnect cannot lose a new row.
Control updates never refresh operational timestamps. Failed legacy admission
retries on later control updates; an intentional removal is not retried merely
because a control heartbeat arrives. The existing EKCH legacy association is
retained. Other coverage policies require the backend coordinator to admit them.

Master epochs, priority, readiness and source fences are per airport session.
Human work retains its captured instance and membership through deferred
execution. Backend-issued execution metadata correlates existing typed SDK
callbacks; it is bounded, expires and disappears with its membership. It does
not create an acknowledgement protocol or a durable action engine. Frequency
message results retain the existing request ID and return to the initiating
frontend connection.

## Authorized discovery and selection

`GET /api/plugin-memberships` requires a bearer token and returns only transports
owned by that token's CID, including transports with an empty membership list.
There is no arbitrary-CID/session query. Responses use `Cache-Control: no-store`.
Each membership exposes airport, complete environment, session, view, role,
readiness, read-only status and opaque instance/membership identifiers. Unsigned
64-bit generations and epochs are JSON strings so browsers do not lose precision.
`ControlPresence()` is the trusted in-process source for later coverage work;
it does not start another VATSIM or radio acquisition worker.

The existing frontend token frame can optionally include all three selection
fields:

```json
{
  "type": "token",
  "token": "<access token>",
  "session_id": 42,
  "plugin_instance_id": "<instance_id from discovery>",
  "membership_id": "<membership_id from discovery>"
}
```

The backend resolves the selection against the authenticated CID. A removed
explicit membership never falls back to another instance or airport. Commands
capture that exact member, including nested/deferred workflow effects and
frequency messages. Existing unselected single-membership clients retain direct
association; ambiguous associations fail closed. The chooser, tab switcher and
their UI state belong to MA-06.

## Validation boundary

Go transport, handler, race and PostgreSQL tests exercise membership routing,
authorization, full/incremental synchronization, removal, source fencing,
reconnect cleanup and control-only presence. Windows Release builds and plugin
tests exercise the C++ wire, membership and SDK queue paths. Real EuroScope
controller sessions and live operational scenarios still require controlled
manual acceptance. Deployment remains coordinated with no controllers online;
these changes do not provide distributed election or online failover.
