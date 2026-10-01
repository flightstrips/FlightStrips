# Backend architecture

`cmd/server` constructs `app.BuildNATS` as the sole application runtime. There is
no SQL constructor, pool, repository implementation, migration, seed, fallback,
dual write or Redis dependency. Existing domain policy is reused by accepted-state
adapters. The [binding matrix](../.github/specs/multi-node-nats/tasks/20a-runtime-assembly-evidence.md)
records every HTTP/socket action and worker; [final evidence](../.github/specs/multi-node-nats/tasks/20c-sql-retirement-evidence.md)
records the combined entrypoint checks.

## State and ownership

Three NATS 2.15.0 JetStream nodes replicate `FS_STATE`, `FS_POSITIONS`,
`FS_PRESENCE`, `FS_SNAPSHOT_INDEX` and `FS_OBJECTS`. An administrator creates and
validates resources through `cmd/nats-bootstrap`. Backend credentials verify
resource configuration and use existing resources; startup never creates or
repairs them. Resource drift fails closed.

Typed Protobuf events in `FS_STATE` determine operational state. Each backend
replays an independent projection before admitting reads or commands. Session,
airport and global owners use fenced leases and owner epochs. Commands use a UUID,
expected revision and request hash; an acknowledgement means accepted state has
been replayed locally. A lost Core NATS reply can be reconciled through the durable
command outcome. Socket presence and aircraft positions use typed KV observations
and freshness checks; restored controller records alone never prove a live socket.

The registry allocates sessions and drives durable deletion. Session owners run
controller/sector/master election, traffic, CDM, PDC, stand lifecycle, navigation,
AMAN and deadline work. Provider supervisors accept typed, bounded generations
from VATSIM, transceivers, METAR/ATIS, ECFMP, CDM, AIRAC and Open-Meteo. Shared
policy consumes these accepted generations. External side effects carry durable
intent/claim/result records and stable identities. Ambiguous provider sends remain
unknown; they are not automatically repeated. EuroScope effects target the
accepted CID/socket generation and use the shared encrypted key ring.

## Transport and HTTP

`/frontEndEvents` negotiates `flightstrips.frontend.pb.v2`; `/euroscopeEvents`
negotiates `flightstrips.euroscope.pb.v2`. Both require typed binary Protobuf token
and operational envelopes. Revision-2 clients are required; there is no text-JSON
socket compatibility path. First-party HTTP APIs remain JSON and read projections
or route typed commands. `/albEvents`, its ALB implementation, configuration,
enablement and protocol are preserved unchanged.

`/healthz` reports process liveness. `/readyz` additionally requires verified
resource metadata, JetStream write quorum, initialized/current projection and
observation watchers, and usable ownership. It returns 503 during replay, quorum
loss, drift or projection stall, then recovers after catch-up. Shutdown stops
admissions, drains in-flight work and position writers, joins workers and closes
NATS. Subscription registration uses bounded flush attempts and tolerates
transient transport loss, including contexts without caller deadlines.

`/metrics` exposes PubAck duration/count, replay lag, watcher freshness, owner
state/epochs, effect state and storage bytes. Accepted-command debug logs contain
command ID, stream sequence and owner/master epochs. Runtime errors log types;
tokens, credential URLs, provider/private-message bodies and encrypted payloads
must not be logged. Preserve effect keys across restarts and retain old key IDs
while encrypted intents remain replayable.

## Configuration and local operation

See [Windows development](Readme.md) for exact commands, paths and prerequisites.
`NATS_URLS` is required; TLS/credentials and timeouts use the existing
`natsresources.ConfigFromEnv` contract. `NATS_EFFECT_ACTIVE_KEY_ID` selects a key
from comma-separated `NATS_EFFECT_KEY_FILES=id=path,...` (32 raw bytes per file).
`NATS_AIRPORTS` defaults to EKCH. Ordinary component build identities and releases
remain intact. SAT scenario/replay tools and landing validation remain disabled.

Task 20 is held from main/release. Task 21 owns production Swarm infrastructure;
Tasks 22/23 own overlapping-version/recovery and capacity qualification. Task 24
owns activation, with a stop-first initial SQL-to-NATS deployment. Local checks do
not establish multi-host performance or the operator's manual acceptance.
