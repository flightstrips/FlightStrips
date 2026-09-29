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
| `FS_OBJECTS` | `provider/<provider>/<sha256>` | `ObjectValue.provider_page` | airport provider importer |
| Core NATS | `fs.v1.command.<node-id>` | `CommandRequest`; reply is `CommandReply` | forwarding backend / owner |

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
| `ProviderCheckpoint` | airport | provider, dot, resource |
| `AmanFlight` | airport | uppercase callsign |
| `AmanCoordination`, `AmanAudit`, `AmanValidation`, `VatsimObservation` | airport | their ID/provider ID |
| `Session` | session | decimal session ID |
| `Controller` | session | CID |
| `SectorOwner` | session | uppercase sector ID |
| `Strip`, `PdcSequence`, `CdmState`, `EcfmpState`, `ClxOverride` | session | uppercase callsign; CLX appends dot and override key |
| `Coordination`, `TacticalStrip`, `FrontendMessage`, `SessionDeadline` | session | decimal ID or deadline ID |
| `StandAssignment` | session | uppercase callsign |
| `StandBlock` | session | uppercase stand ID |
| `Atis` | session | uppercase airport ICAO |

The record itself carries the same key identity and validator checks equality. All numeric entity IDs are monotonic within session and never reused; global session IDs start at 1 and never reuse. Entity revision increments per actual replacement or deletion. An event with several changes increments aggregate revision once. Indexes are derived from entity records in a stable order, not independently stored. An absent optional value is truly unknown/not set; zero, empty string, false, and empty repeated field have their normal explicit meaning only when the domain validator permits them. For an optional message, absence represents null. Empty repeated lists replace earlier lists. `google.protobuf.Timestamp` must pass the Protobuf valid range and be normalized to UTC; all `double` fields must be finite. Canonical lat/lon ranges are ±90/±180 degrees and headings/courses are in [0,360). Identifiers have no dots where used in KV keys.

The remaining `string` fields named `state`, `status`, `kind`, `source`, `reason`, or `action` are **scalar domain tokens**, never serialized objects. Validators use these closed vocabularies: PDC state is `NONE`, `REQUESTED`, `REQUESTED_WITH_FAULTS`, `CLEARED`, `CONFIRMED`, `NO_RESPONSE`, `FAILED`, `REVERT_TO_VOICE`; AMAN effective mode is `disabled`, `shadow`, `read_only`, `authoritative`, `blocked`; AMAN lifecycle state is `planned`, `airborne`, `unstable`, `stable`, `landed`, `go_around`, `removed`; AMAN sequence disposition is `active`, `desequenced`; data status is `fresh`, `stale`, `disconnected`; freeze reason is `none`, `superstable`, `tma`, `manual`; confidence is `unknown`, `low`, `medium`, `high`; navigation coverage is `complete`, `partial`, `unsupported`, `unavailable`; navigation validation state is `candidate`, `validated`; route fact state is `active`, `cleared`, `expired`. Other domain token fields take the exact validated constants from the source domain package at this spec revision, and an unknown token is a decode/validation failure. New token values require a reviewed contract change before writing. Free text fields such as remarks, message text and audit detail are plain UTF-8, with JSON document syntax rejected by boundary validation where a structured value is expected.

## Replay and integrity

1. Read `FS_SNAPSHOT_INDEX` and verify object existence, type, aggregate match, snapshot index digest, internal snapshot digest, and checkpoint monotonicity. Prefer newest verified snapshot; fall back to previous or full replay on corruption.
2. Replay every needed `FS_STATE` entry in stream order. Validate subject/aggregate match, exact schema version, UUID shape, oneof case, owner epoch, aggregate revision, entity key/type/revision, sort order, and duplicate command ledger identity before changing memory. A stale owner event remains in history and is a no-op. A malformed effective event makes the backend unready and alerts; it is never skipped.
3. Snapshot event application is deterministic. Every backend starts with the same empty state and same event sequence, producing equal typed entities and indexes. Snapshot bytes may differ only if a later schema version explicitly defines a migration; schema version 1 has no alternate encoding.
4. Watch KV values independently. Position/presence revision is not an `FS_STATE` revision. TTL expiry removes operational presence without deleting durable controller identity. Position tombstones are typed `PositionTombstone`, never an empty byte value.
5. `FS_STATE` full history has no automatic expiry, eviction, or compaction. `FS_POSITIONS` stale session keys are removed only after session tombstone and verified snapshot. `FS_OBJECTS` keeps two verified snapshots and all navigation objects reachable from active manifests; unreferenced objects are garbage-collected only after reachability verification.

## Provider and audit conversion

AIRAC.net, Open-Meteo, VATSIM, ECFMP and identity providers may expose JSON or another external format. Their adapters parse into validated typed Protobuf/domain values in memory. Persist only `NavData`, `ProviderPage`, `WeatherObservation`, `VatsimObservation`, `EcfmpState`, or another explicitly named typed schema after review. A provider page with fields needed to resume/replay that cannot be represented in `ProviderPage` is **not cached**; the importer refetches it using typed checkpoint metadata. No raw body, `json.RawMessage`, `CanonicalJSON`, or serialized map enters `FS_OBJECTS`, a state event, or a snapshot.

AMAN's previous opaque command outcome, audit and validation payloads become typed `CommandOutcome`, `AmanAudit.fact`, and `AmanValidation`. Audit facts are the nine explicit families: command, observation, sequence, coordination, health, capacity, freeze, go-around and replay. Gap/closure/reservation displacement is `capacity` with typed affected-flight records; superstable/TMA freeze is `freeze`; go-around pending/decision is `go_around`; queue promotion is `sequence`. An existing category without a lossless mapping blocks that feature's cutover until a reviewed schema and catalog change; it may never store a stringified JSON escape hatch. Audits are append-only typed entities, ordered by airport revision and ID. A command outcome records stable status and reason code, not a serialized UI response body. The detail view derives its presentation from the typed airport projection and typed navigation objects.

## Compatibility and schema discipline

- Initial release is stop-first and empty. No PostgreSQL conversion, dual read, old frontend/plugin WebSocket protocol or JSON WebSocket compatibility endpoint is shipped. First-party HTTP remains JSON by design.
- New `.proto` field numbers never reuse removed numbers; deleted fields and enum values become `reserved`. Existing EuroScope cases 1–52 are immutable. A new durable writer is enabled only after all readers understand its field numbers and semantics. Unknown durable fields make an old reader unready rather than corrupting a snapshot.
- Generated Go, TS and C++ bindings are produced from the same schema commit. Do not hand-maintain parallel TS unions or duplicate schema structs. CI compiles descriptors, checks generated files, scans owned transport/storage code for JSON serialization, and round-trips each oneof case and optional/zero boundary.
- Redaction is by typed field: bearer token never appears in events/snapshots; private-message plaintext is only in an authenticated encrypted effect object and is removed 24 hours after terminal outcome. Diagnostic logs print IDs/status and never log Protobuf payloads wholesale.
