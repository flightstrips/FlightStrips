# State ownership and replacement map

This is the exhaustive destination for the current PostgreSQL tables and shared hub state. Each aggregate is a deterministic map of typed entities plus command outcomes, deadlines and revisions. A command that changes several entities within one aggregate emits one event. A query uses an immutable projection snapshot and indexes maintained by the reducer; handlers never compose an operational read from different backends' private memory.

## Identity and aggregate boundaries

| Aggregate | Key | Owns | Important indexes/invariants |
| --- | --- | --- | --- |
| Global | `global` | Airport registry, unique `(airport, session name)` registry, monotonic `int32` session ID allocator, active session list, provider-wide quota ledger | Names unique per airport; session IDs never reused |
| Airport | Uppercase four-letter ICAO | Master position priority, AMAN flights/state/audit/outcomes/evidence/coordination, navigation manifest and cache metadata, airport weather and provider checkpoints, airport-owned CDM configuration | Existing AMAN CID/callsign uniqueness and revision rules; active nav manifest points only to verified Object Store blobs |
| Session | Decimal existing `int32` ID | Session/runways, strips, controllers, sectors, coordinations, tactical strips, stand assignments/blocks, PDC sequence/state, ATIS/CLX overrides, frontend messages, EuroScope sync/master state, effects/deadlines, session-specific CDM/ECFMP state, and vIFF flight-read checkpoints | Existing per-session callsign, controller, sector and stand uniqueness; existing strip and stand versions |

The global allocator starts at 1 for a fresh deployment. Session-owned numeric entity IDs use per-session, per-type monotonic counters and are never reused within that session; this preserves numeric IDs in existing frontend and API DTOs. Airport-scoped AMAN flight IDs and navigation digests retain their current stable formats. Object Store names contain SHA-256 for immutable navigation content; the airport event atomically switches the active manifest only after all referenced objects are written and verified.

## Current storage family to destination

| Current state | NATS destination | Notes |
| --- | --- | --- |
| `airports`, `sessions`, `versions` | Global registry and session aggregate; `versions` removed | Event schema versions replace SQL migration rows. Global creation state is `initializing`, `active`, or `deleting`. |
| `airport_master_orders` | Airport aggregate | Priority changes are airport owner commands. |
| `controllers`, `sector_owners`, `strips`, `coordinations`, `tactical_strips` | Session aggregate | Retain all existing field, ordering, authorization and uniqueness behavior. Controller operational identity is durable; socket presence is separate. |
| Session PDC sequence fields and strip PDC fields | Session aggregate | `pdc.Candidate` composes clearance with production policy, owns parsed Hoppie messages/checkpoints and outbound workflows, and reconstructs polling/ten-minute deadlines after takeover. Web PDC shares the owner path without Hoppie. The commented-out `pdc` table is not a data source. |
| `stand_assignments`, `stand_blocks` | Session aggregate | Reimplement serializable stand allocation as one validated session event; retain manual/automatic precedence, versioning and expiry. |
| `aman_airport_states`, `aman_flights`, `aman_command_outcomes`, `aman_audit_records`, `aman_validation_evidence`, `aman_vatsim_observation_identities`, `aman_coordination_requests` | Airport aggregate | Retain audit and command outcome history in `FS_STATE`; no aggregate-history eviction in v1. |
| `aman_nav_airport_fragments`, `aman_nav_procedure_fragments`, `aman_nav_fix_fragments`, `aman_nav_terminal_fragments`, `aman_nav_manifests`, `aman_nav_active_manifests`, `aman_nav_route_cache` | Immutable Object Store content plus airport manifest/cache index | Do not embed large fragments in each state event. Publish a verified blob first, then its digest/metadata in the airport event. Route-cache entries refer to digest and resolver/schema version. |
| `airacnet_http_checkpoints` | Typed airport `ProviderCheckpoint` plus optional typed `ProviderPage` Object Store value | Owner alone advances pagination/ETag. Raw provider response bodies are never stored. If the typed page omits a field needed for replay, refetch it. |
| Provider-wide VATSIM, ECFMP and AFV ATIS feed checkpoints | Typed global `ProviderCheckpoint` plus typed `ProviderPage` Object Store value | Global owner fetches once; airport and session owners consume the referenced typed source revision. |
| Per-session VATSIM feed application | Session `VatsimSessionCursor` and source provenance on each VATSIM strip | One session event applies a source generation; older generations cannot replace EuroScope or newer source state. |
| `aman_weather_cache`, `aman_weather_request_ledger` | Airport cache and global quota ledger | Preserve expiry and request-rate accounting. The global quota command is committed before an external call. |
| Open-Meteo GFS wind profiles | Typed airport `ProviderCheckpoint` and `ProviderPage.open_meteo` Object Store value | Airport intent precedes global quota reservation and one provider call; a fresh typed page is reused without another call. |
| Frontend hub `messages`, METAR/ATIS code caches, CLX overrides | Session aggregate | These affect cross-node snapshots and must not remain local-only. METAR provider polling stays airport owned; session-specific presentation is committed to the session. |
| EuroScope hub master, synced-session flag, delayed offline/disconnect/session work, runway state | Session aggregate terms, sync marker and deadlines | Physical socket pointers, per-connection write queues and recorders stay local. A recorder is diagnostic output, not operational authority. |
| Local client maps, observer/CID/IP metadata, online counts | `FS_PRESENCE` and local socket map | Presence has a 10-second TTL and is rebuilt from live sockets. Do not persist local IP in the event log. |
| Latest aircraft position and disconnect observation | `FS_POSITIONS` plus derived session events | One key per session/aircraft/owner epoch. Strip position read models include the latest accepted observation; a position alone never increments the user-edit strip version. |

For any SQL query or repository method not named in a table row, classify it by the owning entity above and implement the same read or domain command through the projection. The completion check is mechanical: no runtime import of `pgx`, `sqlc` query package, SQL transaction, `DATABASE_CONNECTIONSTRING`, or PostgreSQL repository remains. Existing test fixtures may be removed or converted; no runtime PostgreSQL sidecar is allowed.

## Session lifecycle workflow

1. `GetOrCreateSession(airport,name)` submits a global command. The global owner returns the existing active/initializing ID or allocates the next ID and commits `initializing`. Competing creates of the same pair converge on one ID through subject CAS and the registry's unique key.
2. A durable workflow step sends `CreateSession(id,airport,name)` to the session aggregate. It is idempotent and seeds counters, runways, and an unsynced state. The global owner then commits registry `active`. Until then, connection setup waits and retries the same workflow ID rather than creating a second session.
3. The session owner commits `first_no_controller_at` when cluster-wide operational EuroScope presence becomes empty. It clears this marker when a controller returns. After five minutes of **healthy** NATS/projection service without a controller, it commits `session_tombstoned`, including cleanup of pending effects as `expired` or `unknown` according to dispatch state. Frontend clients receive terminal results/disconnect.
4. A global workflow step marks the registry `deleting`, removes its `(airport,name)` active index, and commits a deleted marker. The old numeric ID is never reused. Position keys and unreferenced snapshot objects may be removed only after the tombstone and global step are committed. The retained state log remains available for audit/recovery.

On total NATS outage, no healthy time accrues. After service and projection recover, reset the no-controller observation start to the recovery time for every recovered live session without a newly connected controller. This grants a fresh five-minute window and prevents immediate deletion after downtime.
For a quorum interruption observed without a full backend restart, shift
`first_no_controller_at` forward by the unhealthy interval and record its end
in `Session.cleanup_paused_at`. Each owner checks that marker before applying
the same pause; a new owner re-arms the remaining healthy time from the
persisted session. A full backend restart resets the observation start instead.

## Cross-aggregate domain work

The session is the authority for strips and stands; the airport is the authority for AMAN and airport policy. An airport worker that needs a session strip change commits an airport intent, sends an idempotent session command, and records completion or failure in the airport event stream. A session handler that needs airport-owned metadata reads a versioned airport projection and records that source revision in its session event. If the airport revision changes before commit, retry or mark the derived state pending; do not claim one atomic write across subjects. Global weather quota reservation and the subsequent airport weather cache update use the same intent pattern. All such workflows are visible in projections and resume after owner takeover.

## Read and publication rules

- Index sessions by ID and `(airport,name)`; strips by `(session,callsign)` and numeric ID; controllers by session/CID/callsign; stands by session/stand/callsign; AMAN by airport/flight ID/active CID/callsign; coordination and audit by their existing replay keys. Rebuild indexes from snapshots and events, never as separately committed data.
- Serve typed `FrontendInitial` WebSocket messages fixed in [wire.proto](proto/wire.proto), and preserve existing JSON response shapes for first-party HTTP APIs. Build both from one session projection revision plus tagged position/presence observations. The frontend derives its socket presentation model locally. Derived validation remains calculated from the same source revisions on every node.
- A domain event produces a local frontend/EuroScope delta on **every** backend after that backend applies it. Local hubs filter by session/airport/CID and write only to local sockets. They do not decide whether a mutation succeeded.
- A connected frontend becomes writable only after its backend has a caught-up projection and a fresh EuroScope sync for that session. A reconnect receives an initial snapshot followed by buffered deltas. If the source projection becomes unhealthy, close the socket so the client reconnects elsewhere; never continue with private stale state.

Task 18c replaces EuroScope offline/disconnect and squawk local outcome timers
with session-owned `SessionDeadline` and typed `GenerateSquawkEffect` state.
`SessionSquawkThrottle` (case 32) is a server-time-derived session entity; its
canonical decimal key coexists with the session seed through the typed entity
index. Master-observed network controllers remain scalar workflow observations;
authenticated CID identities remain separate. WAITING effects form the queue,
so no separately serialized queue or Redis state is introduced. Candidate
construction is dormant until Task 20 and remains held through Task 24.
