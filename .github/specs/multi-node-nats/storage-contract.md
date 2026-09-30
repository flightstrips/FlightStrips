# Binary storage and compatibility contract

This document assigns a Protobuf message to every new multi-node durable or replicated byte value. The schemas in [proto/storage.proto](proto/storage.proto), [proto/wire.proto](proto/wire.proto), and [proto/euroscope.proto](proto/euroscope.proto) fix the field numbers and oneofs. ALB is outside this contract and remains unchanged. New values are raw binary `proto.Marshal` output with no JSON, gzip, text wrapper, or second serializer. Compression, if later needed, belongs to a separately versioned envelope and requires a reviewed schema change. Protobuf unknown fields are rejected rather than silently preserved or discarded.

## Resource value map

| Location | Key or subject | Exactly one encoded message | Writer |
| --- | --- | --- | --- |
| `FS_STATE` | `fs.v1.state.global` | `StateEvent` with `AggregateRef.global` | accepted global owner |
| `FS_STATE` | `fs.v1.state.airport.<ICAO>` | `StateEvent` with matching `AggregateRef.airport` | accepted airport owner |
| `FS_STATE` | `fs.v1.state.session.<id>` | `StateEvent` with matching `AggregateRef.session` | accepted session owner |
| `FS_POSITIONS` | `<session-id>.<aircraft-key>.<owner-epoch>` | `PositionValue` matching all key parts | accepted session owner |
| `FS_PRESENCE` | `node.<node-id>` or `client.<connection-id>` | `PresenceValue` with matching oneof and ID | socket-owning backend |
| `FS_SNAPSHOT_INDEX` | `global.global`, `airport.<ICAO>`, or `session.<id>` | `SnapshotIndex` matching key | snapshot writer after read-back verification |
| `FS_OBJECTS` | `snapshot/<kind>/<id>/<last-stream-sequence>` | `ObjectValue.snapshot` | snapshot writer |
| `FS_OBJECTS` | `nav/<sha256>` | `ObjectValue.nav` | airport navigation importer |
| `FS_OBJECTS` | `effect/<command-id>` | `ObjectValue.effect_secret` | session effect owner |
| `FS_OBJECTS` | `provider/<provider>/<sha256>` | `ObjectValue.provider_page` | global or airport provider owner; session owner for vIFF/Hoppie |
| Core NATS | `fs.v1.command.<node-id>` | `CommandRequest`; reply is `CommandReply` | forwarding backend / owner |
| Core NATS | `fs.v1.delivery.<node-id>` | `EffectDeliveryRequest`; reply is `EffectDeliveryReply` | session owner / socket backend |
| Core NATS | `fs.v1.result.<node-id>` | `EffectDeliveryRequest` with terminal effect; reply is `EffectDeliveryReply` | socket backend / session owner |

The subject/key is validated against decoded identity before apply. The `ObjectValue` wrapper is included in digest calculation for `nav` and `provider` objects. For snapshots, `Snapshot.sha256` is SHA-256 of deterministic serialization of `Snapshot` with its digest field empty; the index digest is SHA-256 of the complete stored `ObjectValue`. For an effect secret, the event stores the SHA-256 of the complete stored `ObjectValue`. Object names are immutable. NATS Object Store metadata is transport metadata, never a replacement for identity or digest validation. The object cap is 32 MiB. State event and Core request/reply cap is 1 MiB; client/plugin frame cap is 4 MiB. Oversized domain changes are rejected and redesigned as typed object references.

## Entity keys and ownership

`EntityRecord.value` is a closed oneof. `EntityChange.key` and `EntitySnapshot.key` use these canonical keys. `DeleteEntity.kind` must match the previously stored oneof case; key changes are delete plus insert in one event. String keys are normalized before hash or publish. Aggregate ID is part of the subject and is not repeated in a key unless specified below.

| Entity cases | Aggregate | Canonical key |
| --- | --- | --- |
| `AirportRegistry` | global | uppercase ICAO |
| `SessionRegistry` | global | decimal session ID |
| `ProviderQuota` | global | provider, dot, UTC window start as Unix seconds |
| `AirportPolicy`, `AmanAirport`, `NavManifest`, `WeatherCache` | airport | uppercase ICAO; weather appends provider |
| `NavRouteCache` | airport | route key |
| `ProviderCheckpoint` | global for provider-wide VATSIM/ECFMP; airport for AIRAC, CDM configuration and airport feeds; session for vIFF flight reads and Hoppie station polls | provider, dot, resource |
| `AmanFlight` | airport | uppercase callsign |
| `AmanCoordination`, `AmanAudit`, `AmanValidation`, `VatsimObservation` | airport | their ID/provider ID |
| `Session` | session | decimal session ID |
| `SessionSquawkThrottle` | session | decimal session ID (typed index distinct from Session) |
| `Controller` | session | CID |
| `SectorOwner` | session | uppercase sector ID |
| `Strip`, `PdcSequence`, `CdmState`, `EcfmpState`, `ClxOverride` | session | uppercase callsign; CLX appends dot and override key |
| `Coordination`, `TacticalStrip`, `FrontendMessage`, `SessionDeadline` | session | decimal ID or deadline ID |

| `StandAssignment` | session | uppercase callsign |
| `StandBlock` | session | uppercase stand ID |
| `Atis` | session | uppercase airport ICAO |
| `VatsimSessionCursor` | session | `vatsim` provider key |
| `PdcProviderMessage` (case/kind 31) | session | canonical message UUID |

Task 18a reserves only entity case/kind 31, `ProviderPage.hoppie` field 12,
`SystemCommand.apply_pdc_provider_message` field 14 and `PdcEffect` fields 4–6.
Task 18c's reservations are unchanged. `HoppiePdcRequest` contains separate
callsign, aircraft type, departure, destination, stand, ATIS and remarks fields
(1–7). `PdcProviderMessage` contains message UUID (1), from/to (2/3), transport
(4), sequence (5), optional response-to (6), kind (7), optional parsed request
(8), optional radio clearance prose (9), reason (10) and optional provider
acceptance timestamp (11). Nested enums use `TRANSPORT_`/`KIND_` prefixes and
Task 18a's exact numbers. `HoppiePollPage` contains station (1), poll UUID (2),
observation timestamp (3) and repeated parsed messages (4). No raw packet,
logon secret or serialized object is retained.

Provider `hoppie` checkpoints use resource `station/<uppercase station>` in
the session aggregate. An accepted poll object/checkpoint and its next
`pdc-poll.<station>` deadline commit before processing messages. The deadline
stores the next workflow UUID and one chosen 25–45 second interval; failed
polls rearm a distinct later slot. Takeover replays the checkpoint before
polling. Message UUIDs retain their parsed body across retries; the ledger
rejects changed bodies under the same UUID.

Outbound parsed messages and `pdc/outbound` workflows commit before the
`ExternalCallWorker` attempt. A successful result atomically records acceptance,
completes the outbound workflow and marks a matching clearance sent. Recovery
proves results from their stable result command IDs or records `CALL_UNCERTAIN`;
it never repeats an uncertain send/poll intent. A correlated pilot response can
prove delivery while retaining the historical uncertain API result; it does
not invent a provider acceptance timestamp. Accepted poll observation time
decides whether a response preceded its deadline, including replay after owner
death. Checkpoint messages are processed before reconstructing due timeouts.
Ten-minute response deadlines
remain durable and revisioned with their PDC sequence. Web PDC requires no
Hoppie client. `pdc/pilot` workflows retain the original request command's actor.
Deferred `pdc/plugin/<field>/<callsign>/<origin-command>` workflows refer to the
exact strip revision and original ISSUE effect target; later edits supersede
them before dispatch. These include route/SID writes, confirmation state and
cleared-flag reset after UNABLE, no-response or revert. Those failure transitions
return the strip to NOT_CLEARED and reset ownership atomically.

`Coordination.from_euroscope` and `euroscope_handover_cid` retain the source
and acknowledgement target of an inbound EuroScope handover. The session owner
allocates its ID with `Session.next_coordination_id` in the same event. A
resolved transfer removes the active coordination in the strip-owner event;
the retained command outcome supplies replay and duplicate-ID history.
Active coordination status is `TRANSFER` or `TAG`; resolution deletes the
entity while the outcome remains in the session ledger.

Stand assignment replacements retain direction, lifecycle stage, SAT rule and
variant, conflict and observation provenance, ETA, projected release and expiry,
acknowledgment, VATSIM identity/revision, and creation/update timestamps.
`StandAssignment.revision` equals its entity revision. Stand block replacements
retain block type, source, owner, optional callsign, expiry and revision. Both
records carry the selected, sorted adjacency stand IDs so replay can verify
collisions without consulting a node-local stand configuration.
An expiry is a validated session command against the currently stored deadline;
passing a projected departure release alone never proves physical vacating.
`SessionDeadline.source_revision` is the revision of the PDC, controller, or
strip entity that scheduled the deadline. The owner checks that revision and
the current presence/position observation again in the command planner before
consuming a deadline. A replaced deadline has a new entity revision and cannot
be fired by a worker holding its earlier revision.
`StandAction` lifecycle fields 11–17 are accepted only from a system actor.

Task 19a keeps warning episode state in `StandAssignment.conflict_reason` or
an existing typed `WorkflowRecord` keyed by a stable session/callsign/pilot
episode UUID. Its step is the observed stand and its source revision is the
accepted global VATSIM generation. A STAND effect intent is a pending
`WorkflowRecord` keyed by lifecycle command/callsign, with the scalar routing
step `vatsim-stand/<callsign>/<stand-or->/<immutable-controller-CID>` and the
changed strip revision in `source_revision`. The derived command completes
that workflow and creates the typed Task 17 STAND effect, or supersedes it if
the strip has changed. No extra persistence bucket or generic payload is used.
Typed `Strip.ground_state` and `engine_type` retain the observed facts needed
by the existing lifecycle and SAT compatibility policy.
Completed `vatsim-prefile-input` workflows are keyed by session, callsign and
accepted generation/digest. They prevent an expired hold from being recreated
by replay of that generation; a new accepted generation is a new input.

`Atis.metar`, `arrival_code`, and `departure_code` retain the existing frontend presentation fields per session. `code` and `text` retain the typed ATIS observation. `Atis.source_revision` binds the global AFV ATIS checkpoint revision and digest to the airport METAR fetch timestamp, so a stale presentation cannot overwrite a newer one. `ProviderPage.atis_feed` retains typed arrival and departure entries from the provider-wide AFV fetch. No METAR or ATIS presentation cache is needed on each backend.

Task 05 keeps the legacy sector position and identifier as typed scalar fields
on `SectorOwner`, and the per-position controller layout as
`Controller.layout_id`. The session stores typed `RunwayStatus` entries keyed
by runway pair. A sector table replacement and a position-wide layout update
each form one subject-CAS event. Controller presence and last-seen observations
remain in `FS_PRESENCE`, outside the durable entity revision.

`Session.master` is replaced by the owner-side `elect_session_master`
planner. Its epoch increases for every selected connection, vacancy, or owner
term change. An empty connection ID and CID denote a vacant master term; the
term remains stored so the next election cannot reuse its epoch. The same
session replacement clears `Session.sync`. A fresh sync must match the
selected connection ID and epoch and is operational only while its client and
node presence are fresh.

Task 09 adds `PdcSequence.requested_at`, `issued_at`, `sent`,
`issued_by_cid`, and `request_channel`. `Session.next_message_id` allocates a
PDC issue sequence in the same session event as its `PdcSequence`, response
`SessionDeadline`, and waiting typed `PdcEffect`. `CreateTactical` carries
`kind`, `label`, and `aircraft` so the existing tactical type and runway rules
can be checked before allocating `Session.next_tactical_id`. These are
additive candidate schema fields; plugin dispatch remains a later task.
`IssuePdc.request_remarks` and `request_channel` preserve the WEB and CPDLC
request inputs without embedding provider payloads.

The record itself carries the same key identity and validator checks equality. All numeric entity IDs are monotonic within session and never reused; global session IDs start at 1 and never reuse. Entity revision increments per actual replacement or deletion. An event with several changes increments aggregate revision once. Indexes are derived from entity records in a stable order, not independently stored. An absent optional value is truly unknown/not set; zero, empty string, false, and empty repeated field have their normal explicit meaning only when the domain validator permits them. For an optional message, absence represents null. Empty repeated lists replace earlier lists. `google.protobuf.Timestamp` must pass the Protobuf valid range and be normalized to UTC; all `double` fields must be finite. Canonical lat/lon ranges are ±90/±180 degrees and headings/courses are in [0,360). Identifiers have no dots where used in KV keys.

The remaining `string` fields named `state`, `status`, `kind`, `source`, `reason`, or `action` are **scalar domain tokens**, never serialized objects. Validators use these closed vocabularies: PDC state is `NONE`, `REQUESTED`, `REQUESTED_WITH_FAULTS`, `CLEARED`, `CONFIRMED`, `NO_RESPONSE`, `FAILED`, `REVERT_TO_VOICE`; AMAN effective mode is `disabled`, `shadow`, `read_only`, `authoritative`, `blocked`; AMAN lifecycle state is `planned`, `airborne`, `unstable`, `stable`, `landed`, `go_around`, `removed`; AMAN sequence disposition is `active`, `desequenced`; data status is `fresh`, `stale`, `disconnected`; freeze reason is `none`, `superstable`, `tma`, `manual`; confidence is `unknown`, `low`, `medium`, `high`; navigation coverage is `complete`, `partial`, `unsupported`, `unavailable`; navigation validation state is `candidate`, `validated`; route fact state is `active`, `cleared`, `expired`. Other domain token fields take the exact validated constants from the source domain package at this spec revision, and an unknown token is a decode/validation failure. New token values require a reviewed contract change before writing. Free text fields such as remarks, message text and audit detail are plain UTF-8, with JSON document syntax rejected by boundary validation where a structured value is expected.

## Replay and integrity

1. Read `FS_SNAPSHOT_INDEX` and verify object existence, type, aggregate match, snapshot index digest, internal snapshot digest, and checkpoint monotonicity. Prefer newest verified snapshot; fall back to previous or full replay on corruption.
2. Replay every needed `FS_STATE` entry in stream order. Validate subject/aggregate match, exact schema version, UUID shape, oneof case, owner epoch, aggregate revision, entity key/type/revision, sort order, and duplicate command ledger identity before changing memory. A stale owner event remains in history and is a no-op. A malformed effective event makes the backend unready and alerts; it is never skipped.
3. Snapshot event application is deterministic. Every backend starts with the same empty state and same event sequence, producing equal typed entities and indexes. Snapshot bytes may differ only if a later schema version explicitly defines a migration; schema version 1 has no alternate encoding.
4. Watch KV values independently. Position/presence revision is not an `FS_STATE` revision. TTL expiry removes operational presence without deleting durable controller identity. Position tombstones are typed `PositionTombstone`, never an empty byte value.
5. `FS_STATE` full history has no automatic expiry, eviction, or compaction. `FS_POSITIONS` stale session keys are removed only after session tombstone and verified snapshot. `FS_OBJECTS` keeps two verified snapshots and all navigation objects reachable from active manifests; unreferenced objects are garbage-collected only after reachability verification.

## Provider and audit conversion

Task 19c adds only `ProviderPage.transceivers` oneof field 13, containing
`TransceiverFeedPage { fetched_at = 1; clients = 2; }` and
`TransceiverFeedClient { callsign = 1; frequencies_hz = 2; }`. The global
`vatsim.transceivers/v3` checkpoint references a verified content-addressed
typed provider object. Clients are sorted by uppercase trimmed callsign;
frequencies are positive, sorted, unique whole hertz, canonicalized to the
whole kilohertz presented by the existing `NormalizeFrequency` policy.
Malformed/empty provider results do not replace the accepted checkpoint.
No new entity, command case or compatibility baseline is allocated.

The accepted checkpoint entity revision and object digest are the frequency
source revision. Session reconciliation uses existing `AdvanceWorkflow` and
`WorkflowRecord` values: the stable workflow UUID includes session ID, source
revision and digest; step is `transceiver/sectors/<digest>`; `source_revision`
is the global checkpoint entity revision. The session owner commits the
completed workflow together with the typed sector/layout/route diff. Comparing
the greatest completed source revision with the accepted checkpoint recovers
missed notifications and takeover. A failed policy pass leaves that revision
pending; no provider worker invokes SQL callbacks or marks it applied.

AIRAC.net, Open-Meteo, VATSIM, ECFMP and identity providers may expose JSON or another external format. Their adapters parse into validated typed Protobuf/domain values in memory. Persist only `NavData`, `ProviderPage`, `WeatherObservation`, `VatsimObservation`, `EcfmpState`, or another explicitly named typed schema after review. `ProviderPage.ecfmp` carries ECFMP measures, scalar measure values, route lists, and typed filters used for per-flight application; its fetched timestamp and immutable object digest identify the source revision. Unknown ECFMP measure or filter shapes fail conversion instead of entering the object store. A provider page with fields needed to resume/replay that cannot be represented in `ProviderPage` is **not cached**; the importer refetches it using typed checkpoint metadata. No raw body, `json.RawMessage`, `CanonicalJSON`, or serialized map enters `FS_OBJECTS`, a state event, or a snapshot.

`ProviderPage.open_meteo` retains each requested forecast coordinate/time and vertical wind level, with observed and expiry times. Its airport-owned checkpoint is committed after a global quota reservation and verified object publication. A replayed or uncertain workflow ID cannot issue another Open-Meteo call.

`ProviderPage.cdm_config` retains the airport's rates, SID intervals, taxi zones, delays, deicing policy and defaults after the existing provider parsers run. `ProviderPage.viff_flights` retains every IFPS/CDM field used by flight reconciliation, and `ProviderPage.viff_masters` retains airport master positions. Only vIFF flight pages may use a session-owned `ProviderCheckpoint`; configuration and master pages remain airport-owned. vIFF writes use a stable owner-fenced workflow and a committed result outcome. A lost result acknowledgment is resolved from the command ledger, while an unproven provider call stays uncertain and is not repeated.

AMAN's previous opaque command outcome, audit and validation payloads become typed `CommandOutcome`, `AmanAudit.fact`, and `AmanValidation`. Audit facts are the nine explicit families: command, observation, sequence, coordination, health, capacity, freeze, go-around and replay. Gap/closure/reservation displacement is `capacity` with typed affected-flight records; superstable/TMA freeze is `freeze`; go-around pending/decision is `go_around`; queue promotion is `sequence`. An existing category without a lossless mapping blocks that feature's cutover until a reviewed schema and catalog change; it may never store a stringified JSON escape hatch. Audits are append-only typed entities, ordered by airport revision and ID. A command outcome records stable status and reason code, not a serialized UI response body. The detail view derives its presentation from the typed airport projection and typed navigation objects.

## Compatibility and schema discipline

Task 18b appends nested CDM fields only: `CdmState` 9–23 retain actual takeoff
time, unconstrained vIFF proposal clocks, confirmation and recalculation
flags, calculation reasons, exact export parameters/dependencies and pushback
verification attempts/matches. Existing `source_revision` identifies the
accepted CDM policy generation that owns a derived export; result application
does not create a new export generation. Export intent operation UUIDs and
Task 19 workflows remain durable after success, failure or uncertainty.
Internal clocks are Protobuf timestamps; `ExportData` clocks retain vIFF's
actual HHMM/HHMMSS API representation in named scalar fields, not encoded JSON.
Replay validates intent kind, UUID/dependency, payload shape, clock and range.
`CdmAction` appends explicit operations 7–16 and `ValidationAction` adds the
existing ASSIGN HP presentation. Canonical and specification schemas and all
three language bindings move together. No top-level `EntityRecord`,
`SystemCommand`, `EffectRecord` or `ProviderPage` allocation is introduced;
Task 18a/18c reservations remain untouched. Traffic adds no stored entity.

- Initial release is stop-first and empty. No PostgreSQL conversion, dual read, old frontend/plugin WebSocket protocol or JSON WebSocket compatibility endpoint is shipped. First-party HTTP remains JSON by design.
- New `.proto` field numbers never reuse removed numbers; deleted fields and enum values become `reserved`. Existing EuroScope cases 1–52 are immutable. A new durable writer is enabled only after all readers understand its field numbers and semantics. Unknown durable fields make an old reader unready rather than corrupting a snapshot.
- Generated Go, TS and C++ bindings are produced from the same schema commit. Do not hand-maintain parallel TS unions or duplicate schema structs. CI compiles descriptors, checks generated files, scans owned transport/storage code for JSON serialization, and round-trips each oneof case and optional/zero boundary.
- Redaction is by typed field: bearer token never appears in events/snapshots; private-message plaintext is only in an authenticated encrypted effect object and is removed 24 hours after terminal outcome. Diagnostic logs print IDs/status and never log Protobuf payloads wholesale.

Task 18c reserves enum/entity case 32 for `SessionSquawkThrottle`, system
command case 15 for `RequestSquawk`, and effect payload case 17 for
`GenerateSquawkEffect`. Throttle cannot be written by `domain_changed`; replay
materializes it atomically with a valid DISPATCH_CLAIMED event, with
`next_allowed_at` equal to that message's JetStream server timestamp plus five
seconds. Snapshot recovery validates its session identity and timestamp.
WAITING squawk effects are the queue, ordered by the accepted command outcome's
committed stream sequence, then UUID. CID stays immutable; claimed socket
connection is an exact delivery precondition. Assigned squawk/removal cancels
waiting work; claim ambiguity becomes UNKNOWN and is never requeued.

EuroScope's controller API exposes callsign/frequency without CID. Its reports
are scalar `WorkflowRecord` observations with stable session/callsign UUID,
step `euroscope-controller/<callsign>/<frequency>` and source revision equal
to the accepted master epoch. PENDING means reported online; COMPLETED means
reported offline. Only reports from the currently live synced master augment
shared coverage. Authenticated `Controller` records retain real CID keys;
network-only reports cannot authorize commands or target effects. Offline
reports schedule a controller deadline whose source is the authenticated
controller revision, or master epoch when no CID record exists. Aircraft
disconnect sources are accepted FS_POSITIONS tombstone KV revisions; session
update/disconnect sources are session entity revisions.
