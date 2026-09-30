# NATS and client contracts

The field-numbered `.proto` files in [proto](proto/) and [storage-contract.md](storage-contract.md) are normative for the new multi-node NATS values and frontend/EuroScope WebSocket frames. They are design artifacts for the new release; task 00 installs them into the root `proto/` build and generates bindings. These payloads use **binary Protobuf**. **ALB is outside this project: do not change its code, endpoint or protocol.** First-party HTTP APIs retain their JSON bodies and response shapes at the HTTP boundary, then translate mutations to typed internal commands; external provider/OIDC formats stay at their adapters. No `Any`, `Struct`, `Value`, generic maps, JSON string, or opaque JSON in `bytes` is permitted inside the new Protobuf schemas. The only `bytes` fields are authenticated-encryption ciphertext and nonce for a UTF-8 private-message body. `request_id` and `command_id` are canonical lowercase UUIDs generated once per logical action. Timestamps in Protobuf use `google.protobuf.Timestamp` in UTC; durations use `google.protobuf.Duration`. Unknown Protobuf fields and enum values are rejected. A contract change requires updating schema, this document, and affected task checks together.

## NATS resources and subjects

| Resource | Storage / replicas | Subjects or keys | Retention |
| --- | --- | --- | --- |
| `FS_STATE` stream | file / 3 | `fs.v1.state.global`, `fs.v1.state.airport.<ICAO>`, `fs.v1.state.session.<decimal-id>` | Limits policy; no MaxAge, MaxMsgs, or automatic eviction |
| `FS_POSITIONS` KV | file / 3 | `<session-id>.<aircraft-key>.<owner-epoch>` | History 1, no TTL; delete all session keys after session tombstone and verified snapshot |
| `FS_PRESENCE` KV | memory / 3 | `node.<node-id>`, `client.<connection-id>` | History 1, 10-second TTL; renew every 3 seconds |
| `FS_SNAPSHOT_INDEX` KV | file / 3 | `<aggregate-kind>.<aggregate-id>` | History 2, no TTL; points to verified snapshot objects |
| `FS_OBJECTS` Object Store | file / 3 | Immutable `snapshot/<kind>/<id>/<sequence>`, content-addressed `nav/<sha256>` and `provider/<provider>/<sha256>`, temporary `effect/<command-id>` | Keep two verified snapshots per live aggregate and referenced navigation objects; delete terminal effect payloads after 24 hours |

Use one NATS account for backend services with scoped credentials and a separate administrative credential for resource creation and backup. Only internal backend processes connect to the new NATS resources; browser and EuroScope clients never do. `node_id` is a random UUID generated at process start and NATS-token encoded without dots; it identifies an incarnation, not a Swarm host. Core NATS command requests go to `fs.v1.command.<node-id>` and contain the target aggregate. A node that is not the accepted owner returns `NOT_OWNER` with the current owner/epoch; the caller refreshes its projection and retries the same command ID.

The NATS bootstrap process creates resources with the exact settings above before backend readiness. Backends verify settings on startup and refuse readiness on drift; they do not silently recreate a differently configured bucket. Pin the NATS server image to `nats:2.15.0` and run the subject-CAS and backup/restore tests against that version.

## Aggregate command and event

Task 05 adds typed controller/sector mutations: `SetPositionLayout` changes
all controller layout records at one session position in one event, and
`ReplaceSectorOwners` atomically replaces the session's sector routing table.
`Controller.layout_id` preserves the per-position layout choice;
`SectorOwner.position` and `.identifier` preserve the routing metadata from
the current repository. `Session.runway_statuses` carries the pair's closed
`OPEN`, `LOW_VIS`, or `CLOSED` value alongside active runways. These fields are
durable metadata; operational online status is derived from fresh
`FS_PRESENCE` client and node observations.

Task 06 uses `SystemCommand.update_entity` and `remove_entity` for complete
candidate strip replacements and deletion, and `ClientCommand.strip` for
controller edits. A strip create has expected entity revision zero and strip ID
zero; the session owner allocates `Session.next_strip_id` and publishes the
session counter replacement with the new strip in one event. Updates and
deletion require the current strip entity revision. The owner validates order,
bay, source authority and unchanged strip ID before publishing. A frontend or
HTTP boundary keeps one command UUID across retries; position-only updates
remain in the position KV path.

Task 07 keeps the EuroScope origin and handover CID on typed session
`Coordination` records. A transfer allocates `Session.next_coordination_id`
and inserts the coordination in one session event; assumption, force assumption,
or cancellation removes it atomically with any strip-owner change. The owner
ledger and session event stream, rather than socket delivery, determine whether
a duplicate command emits a new transfer delta.
An internal EuroScope handover uses `SystemCommand.update_entity` with a
zero-ID `Coordination` keyed by callsign and the current strip revision. The
owner allocates the numeric ID, preserves the EuroScope origin and handover
CID, and applies any arrival-bay or owner correction in that same event.

Task 08 uses `ClientCommand.stand` for controller, pilot and system lifecycle
actions. Only a system actor may provide lifecycle fields 11–17; controller and
pilot actions use the current session strip. An assignment and every strip
change or soft-reservation displacement it causes are one sorted session event.
`StandAssignment` retains SAT rule/variant, stage, deadline, acknowledgment and
VATSIM provenance; `StandBlock` retains ownership and expiry. Both carry sorted
adjacency stand IDs for deterministic replay validation. `SystemCommand.remove_entity`
may expire a stand record only after its persisted deadline and with its current
entity revision. Task 18 schedules that command; this task validates it.

Task 19a's concrete session lifecycle uses `SystemCommand.update_entity` with
the accepted `VatsimSessionCursor` source revision/digest and aggregate read
revision. The owner planner validates that source and the complete tagged
position set again, then commits the real lifecycle's complete typed diff.
Its command identity includes session, accepted generation, callsign, action,
read revision and observation revisions. Persisted assignment deadlines drive
sweeps; projected departure release never establishes vacancy by itself.
`Strip.ground_state` (field 71) and `engine_type` (field 72) retain observed
PARK/PUSH and aircraft facts without changing existing field numbers.
STAND plugin actions use the existing `SetFlightPlanEffect` with field `STAND`
and the Task 17 dispatch/unknown-result rules.

Internal command requests are binary `CommandRequest` and replies are binary `CommandReply` from [wire.proto](proto/wire.proto). `CommandRequest.command` is a closed oneof containing `ClientCommand` or `SystemCommand`; `ClientCommand.action` and AMAN's nested oneof enumerate the valid operations. `expected_entity_revision` is absent only when the action has no read-modify-write precondition. A nested AMAN coordination request ID remains distinct from transport `command_id`. The browser request ID and HTTP `Idempotency-Key` equal that command ID for one logical user action. `SystemCommand.update_entity` is internal to the owner and cannot be submitted by a browser or provider without domain validation.

Normalize a decoded command in the Go owner: reject unknown fields, normalize identifiers and timestamps, sort semantically unordered repeated fields, clear only `command_id`, and deterministic-marshal the typed message. Store SHA-256 of those bytes in the ledger. The Go owner computes this digest on both original and retried commands; cross-language deterministic byte identity is not assumed. A different digest under one ID is `INVALID_ARGUMENT`. SHA-256 collision resistance is the command-identity assumption.

Every `FS_STATE` payload is binary `StateEvent` from [storage.proto](proto/storage.proto). `fact` is a closed oneof for ownership, complete entity changes, workflows, effects, outcomes and sync. `EntityChange.upsert` is a **complete typed entity replacement**; deletion names the exact `EntityKind`. Changes are sorted by entity oneof case number and canonical key. Unknown schema version/entity case stops projection readiness. `aggregate_revision` increases only for effective accepted events. Owner-control events have no entity changes; a stale control event remains in the log but is a deterministic no-op. NATS stream sequence is assigned by the server, not prefilled into the body.

The owner builds an event from its current projection, validates the full domain transition, and publishes with `Nats-Expected-Last-Subject-Sequence` equal to the subject sequence it read. It waits for `PubAck`, then for its local projection to apply that stream sequence before replying. CAS conflict causes reload and revalidation; timeout causes the caller to retry with the same command ID. The aggregate command ledger stores domain command IDs, canonical request hashes, committed stream sequences, and final responses for the aggregate's lifetime. Lease claim/renewal maintenance events are retained in the log but excluded from this ledger. Reusing a domain ID with different content is `INVALID_ARGUMENT`. Deduplication cannot rely on the bounded `Nats-Msg-Id` window.

Command responses use `COMMITTED`, `PENDING`, or one of `INVALID_ARGUMENT`, `UNAUTHORIZED`, `NOT_FOUND`, `REVISION_CONFLICT`, `NOT_OWNER`, `UNAVAILABLE`. A conflict includes current entity revision. `UNAVAILABLE` means no durable commit is confirmed; callers must retry the same ID or query its outcome before generating a new action. NATS request/reply loss alone cannot turn a committed command into a failed command.

## Ownership and presence

Each global, airport, or session aggregate has one owner term in its `FS_STATE` subject: `{node_id, epoch, lease_until}`. A successful claim raises `epoch` by one. Claims and renewals are accepted only when the previous lease has expired or the same owner renews before expiration. Derive expiration from the JetStream message's server timestamp plus eight seconds; do not trust node wall clocks for deciding ownership. Renew every two seconds. A backend stops accepting commands and sending effects immediately upon NATS disconnect, failed renewal, or failed projection health. After a lost owner, the highest rendezvous-ranked ready node among live `FS_PRESENCE` nodes attempts CAS first; another live node attempts after one second if no claim succeeds. Existing healthy ownership is not proactively rebalanced.

An event from a stale epoch is a reducer no-op, even if a stale publisher wins a later subject sequence. A caller reports success only after its event is effective in the local projection. The session actor selects the EuroScope master from cluster-wide client presence using existing position priority, then connection establishment timestamp and connection ID; it commits `master_changed` with an increasing `master_epoch`. Master-only inbound EuroScope events must carry that epoch. The server checks the socket's connection generation, CID, session, and current master epoch before accepting them. Reconnect always creates a new connection ID and requires a new sync.

The owner submits `SystemCommand.elect_session_master` (field 12) with a complete `MasterTerm`. The resulting `DomainChange` replaces the session entity, clears its sync marker, and is the committed `master_changed` fact. A term with empty `connection_id` and `cid` records that no eligible client is present while preserving its increasing epoch. A later connection always receives a new epoch. The owner rechecks live client and node presence, airport priority, establishment time, and connection ID before its subject-CAS commit.

After an effect dispatch claim is committed, the sender resolves the claimed connection generation and CID through fresh `FS_PRESENCE`. A remote target node receives one binary `EffectDeliveryRequest` on `fs.v1.delivery.<node-id>` and answers with binary `EffectDeliveryReply`. The request carries the sender's applied stream checkpoint; the receiving node waits for that checkpoint, then checks the claimed effect in its applied projection, the socket generation, session, CID, and owner/master epochs before a local write. `accepted` means only that the local write callback accepted the effect; it is not a durable command outcome. An ambiguous NATS reply is not retried as a second external effect.

The socket node forwards a plugin result to the current owner on `fs.v1.result.<node-id>` using `EffectDeliveryRequest` with a terminal `EffectRecord`; `connection_id` identifies the live result socket and `claim_stream_sequence` is the sender's applied checkpoint. The owner validates the committed claim, immutable CID, payload, and result socket presence before committing the terminal effect. The `EffectDeliveryReply.accepted` reply means the terminal event was applied. A lost result reply may be retried because the owner compares the stored terminal status and reason before acknowledging. A plugin reconnect may resend an outbox result from a new socket generation under the same CID; this never permits a second command delivery.

## Projection, reads, and recovery

Task 19c's dormant global transceiver adapter shares the legacy HTTP parser and
frequency normalization, but persists only `ProviderPage.transceivers` (field
13), `TransceiverFeedPage` and `TransceiverFeedClient`. A refresh interval slot
has one stable external intent UUID. Recovery never repeats an uncertain slot;
the next configured slot has a distinct persisted intent. Both backend source
ports read the verified global `vatsim` / `transceivers/v3` checkpoint and expose
the same canonical `GetFrequencies(callsign) []string` values. Missing, corrupt
or unready source state returns no invented frequencies.

`NewTransceiverSectorReconciler` is the Task 18c/20 constructor boundary. It
supplies the session owner planner with an immutable accepted lookup, checkpoint
revision and digest. Its completed `WorkflowRecord.source_revision` commits in
the same session event as the sector policy diff. A source revision without a
completed session acknowledgment remains recoverably pending, independent of
local notifications. Task 20 binds the concrete Task 18c planner and scheduling;
Task 19c never calls the legacy `RefreshAllSectors` SQL callback.

Each backend uses its own independent JetStream consumer for `FS_STATE` and watchers for both KV buckets. It loads all verified aggregate snapshots, starts state delivery from the earliest sequence needed by any aggregate, ignores events at or below each aggregate's snapshot checkpoint, and applies later events in stream order. It buffers live events while constructing a frontend initial snapshot, then emits buffered deltas after the snapshot revision. Reads return a coherent `FS_STATE` projection revision; current position and presence observations carry their own observation/revision metadata because they are separate resources. A persisted controller record is shown as operationally online only with fresh corresponding presence. A persisted session-sync marker is valid only for its currently live master epoch and connection generation; after a full restart, all sessions await a new master sync.

Snapshots contain schema version, aggregate ID/revision, last applied stream/subject sequence, typed entity records, owner/master terms, command outcomes, workflows, pending effects/deadlines, and SHA-256. Derived indexes are rebuilt from entities. Create one after five minutes or 10,000 newly applied events, whichever comes first. Write immutable `ObjectValue.snapshot`, read it back to verify checksum, then publish typed `SnapshotIndex`. Retain the prior verified snapshot. Because `FS_STATE` retains full history, a missing/corrupt snapshot falls back to an older snapshot or full replay. A backend cannot become ready until replay reaches the current stream high-water mark and its NATS metadata check is no older than two seconds. After a local write, reads wait for the committed sequence or return `UNAVAILABLE`, never a stale success.

Position KV values are typed `PositionValue`, whose oneof contains `AircraftPosition` or `PositionTombstone`. Position writers are the session owner only. The owner serializes writes per aircraft, uses the prior KV revision as its update precondition, and does not issue overlapping writes for one key. A newer owner writes keys with a new epoch; projection ignores writes from prior epochs once the new owner has issued a fresh sync, and otherwise exposes the last prior-epoch observation as `stale`. A position observation does not change a strip's user-edit revision. Aircraft disconnect writes a typed tombstone after accepted earlier position work drains. Stand/bay and AMAN changes derived from positions are durable commands and are rechecked after new owner sync.

The session owner's position dispatcher drains accepted reports and pauses later reports while a position-derived session command rechecks its source KV revision and commits. A mismatch rederives the transition. This keeps disconnect tombstones and derived strip/stand changes ordered against the same aircraft observation.

## Frontend action result

`FrontendInitial.tagged_observations` carries each position and presence value with its independent KV revision, server observation time, and stale flag. Consumers use these tags for observation ordering; the existing untagged initial fields remain for typed presentation compatibility. Live `FrontendObservation` carries the same tags and `removed` for a KV deletion or expiration.

### Binary framing and negotiation

The frontend WebSocket URL is the existing `/frontEndEvents` endpoint with required `Sec-WebSocket-Protocol: flightstrips.frontend.pb.v2`; EuroScope uses `/euroscopeEvents` with `flightstrips.euroscope.pb.v2`. The server selects exactly that subprotocol or rejects the upgrade. Each binary WebSocket message contains exactly one raw serialized `FrontendFrame` or EuroScope `Envelope`, with no length prefix or text wrapper. Disable `permessage-deflate` for these protocol revisions. Text frames close 1003; malformed/oversized frames close 1002/1009; unauthenticated or wrong revision closes 1008; loss of projection health closes 1013 so the client reconnects. The browser/plugin must not infer success from socket send completion. Core NATS and JetStream messages likewise contain exactly one raw serialized Protobuf message; `Nats-Msg-Id` can reduce duplicate publishes but is never the durable idempotency source. Cap each WebSocket frame at 4 MiB, Core NATS request/reply and `FS_STATE` entry at 1 MiB, and Object Store value at 32 MiB. An oversized value is rejected before publish; it is not split into untyped chunks.

First-party HTTP routes keep their existing JSON request and response contracts, including AMAN detail, stand status/preview, PDC, pilot, EFB, CDM, ECFMP and GSX APIs. Their handlers decode and validate JSON at the HTTP boundary, submit a typed internal `CommandRequest` for every mutation, and convert typed projection results to the existing JSON response shape. No HTTP JSON body is copied into a Protobuf `string` or `bytes` field, persisted, or forwarded over NATS. HTTP `Idempotency-Key` supplies the durable command UUID.

ALB is excluded from this protocol change. Its existing WebSocket behavior remains as it is. The external VACS WebSocket likewise keeps the provider's format inside its adapter; neither is wrapped in the new cluster Protobuf messages.

The frontend WebSocket accepts and emits **binary** `FrontendFrame` only; a text frame closes with WebSocket code 1003. The first frame is `authenticate` with protocol revision 2. `FrontendInitial` is a complete typed entity snapshot with a state checkpoint, positions and presence. Its `airport_aggregate_revision` checkpoints the airport entities in that snapshot; session and airport revisions are checked independently. `FrontendDelta` carries complete typed entity changes. The browser buffers deltas during initial load and resyncs on a revision gap. Its former per-field JSON event shapes are derived locally from these typed records, not transmitted. Every mutating `FrontendCommand` has a top-level UUID `request_id`. Mutating HTTP requests use the same UUID in `Idempotency-Key`; provider callbacks use a stable provider event ID as command ID. The backend emits typed `FrontendActionResult`.

Statuses are `accepted` (durable request with pending plugin effect), `succeeded`, `failed`, `expired`, or `unknown`. The last four are terminal. Pure backend actions emit `succeeded` after commit. An authenticated but domain-rejected command commits an outcome-only event with `failed` and `INVALID_ARGUMENT`, `NOT_FOUND`, or `REVISION_CONFLICT`, without an entity mutation. An unauthenticated/unauthorized request fails before commit and returns `failed` immediately. A browser may retry an action only with its original top-level request ID. The ledger associates outcomes with the authenticated actor and only that actor may query them.

On reconnect, the browser sends `ActionStatusQuery` with at most 100 pending IDs. The backend re-emits stored `FrontendActionResult` for each authorized known ID and `ActionStatusMissing` for an ID without committed outcome. The frontend keeps pending IDs and non-sensitive labels in session storage until terminal results; action bodies live only in memory. After a missing result it may resend the identical in-memory action and ID; after a full reload it shows "not confirmed; retry manually". It never invents a new ID for automatic retry. Later EuroScope state reconciliation may correct domain state but never rewrites terminal `UNKNOWN` into a claim that the original command executed.

First-party HTTP keeps `application/json` and current route/query/body shapes. Mutating routes require canonical UUID `Idempotency-Key`; responses echo it in `X-FlightStrips-Command-ID` and include `X-FlightStrips-Outcome` (`accepted`, `succeeded`, `failed`, `expired`, or `unknown`). A pending plugin effect returns 202 with the existing JSON body when that route has one; a durable backend-only success retains its current success code/body. Existing validation/authorization HTTP errors retain their codes and JSON shapes. `GET /api/commands/{uuid}` is the actor-scoped outcome query and returns `{"command_id":"<uuid>","status":"<one of the five statuses>","reason_code":"<stable code>","detail":"<human text>","aggregate_revision":<integer>}`; unknown ID is 404 and another actor's ID is 403. This JSON is constructed from typed `CommandOutcome` in memory and is never persisted as JSON. `/healthz` and `/readyz` remain status endpoints. External provider and OIDC formats stay in their adapters. Test-only endpoints retain their existing format or are disabled in production; none may write JSON into NATS or Object Store.

## EuroScope effect state machine

`EffectRecord` is part of the same session event as the backend state transition or a separate committed session event if the domain transition is deferred. It records command ID, target CID and connection generation if online, a typed effect oneof, `dispatch_deadline` 30 seconds after the request event's server timestamp, and status `WAITING`. The target CID is immutable. Store a private-message body as encrypted `ObjectValue.effect_secret` at `effect/<command-id>` before committing the request; its typed `PrivateMessageEffect` holds only object name and SHA-256. Garbage-collect an unreferenced object after 24 hours and a terminal effect object 24 hours after its result. A disconnected target may reconnect under a new connection generation with the **same CID** before the deadline. At dispatch deadline, an effect never claimed for dispatch becomes `EXPIRED`.

For a browser `MessageAction.private_message`, `target_cid` carries the pilot callsign accepted by the existing browser action. The owner resolves the authenticated sender's controller CID as the immutable EuroScope effect target and stores the pilot callsign in `PrivateMessageEffect.recipient`. A plugin `EXECUTED` result means its local EuroScope message input accepted the send action; the browser displays that precise outcome and makes no claim about pilot receipt.

Encrypt the UTF-8 private-message body with AES-256-GCM using a random 12-byte nonce and a 32-byte active key from a versioned Swarm secret. `EffectSecret.key_id` identifies the key; authenticated additional data is UTF-8 `fs-effect-v1`, a NUL byte, canonical command UUID, a NUL byte, and target CID. Never reuse a nonce with one key. Keep old decryption keys until every object encrypted with them is deleted and backup retention has elapsed. Reject an effect object with mismatched ID, digest, tag, or target CID. The plaintext never enters a state event, snapshot, log, or browser session storage.

Before socket write, the accepted owner commits `EffectDispatchClaimed` with the command ID, selected connection generation, current owner epoch, and `result_deadline` 30 seconds after the claim event's server timestamp. A claim is made at most once. The target backend verifies it has applied that claim and still owns the named live connection before socket write. If the write, socket, owner, or plugin disappears afterward and no persisted result arrives by the result deadline, commit `unknown`. No claimed effect is automatically dispatched again, including private messages, FPL writes, PDC actions, and other external effects. A state-setting operation may be reconciled from a later EuroScope sync without replaying the original effect.

The normative candidate [euroscope.proto](proto/euroscope.proto) preserves cases 1–52 and adds `TokenEvent.protocol_revision = 3` (required value 2), Envelope metadata fields 100–103, result cases 53/54, and `SessionInfoEvent` role terms 2/3. Task 14 moves this candidate to root `proto/euroscope.proto` and regenerates Go and C++ bindings together. Any other protocol revision fails authentication. Backend effect commands and result frames require Envelope `command_id`; result frames require equality with nested `CommandResultEvent.command_id`. `session_id`, `owner_epoch`, and `master_epoch` accompany effects and master-origin observations. `CommandResultEvent` has `EXECUTED` or `FAILED`, a typed reason enum, and detail of at most 256 UTF-8 bytes; `OK` is valid only with `EXECUTED`. The plugin executes on its EuroScope thread, reports local API/UI success, and retains unconfirmed results in an in-memory outbox until `ResultRecordedEvent`. The backend commits result before acknowledgment. Duplicate results are harmless; plugin restart can yield `UNKNOWN`. A private-message `EXECUTED` means local send acceptance, not pilot receipt.

## Cross-aggregate workflow

Only one aggregate subject can be CAS-committed atomically. The initiating owner commits an intent with workflow ID, ordered steps, and `pending` status. The destination owner commits an idempotent step using UUIDv5 with the workflow UUID as namespace and the step name as input, and replies with a committed sequence. The initiator then commits completion. A takeover worker resumes pending intents from state. Session create/delete retries until its idempotent steps complete. An AMAN-to-session workflow retries while its source revision remains current and otherwise records `superseded`; later accepted AMAN state determines any new intent. A weather quota reservation remains consumed if the provider call becomes uncertain and the airport cache records no invented response. Already committed steps are never silently rolled back. No code path assumes two subjects can be written in one transaction.

Provider-wide VATSIM and ECFMP fetches keep their external-call intent and typed `ProviderCheckpoint` on the global subject. An ECFMP response is converted at the HTTP boundary to `ProviderPage.ecfmp`, with typed measure and filter values, then stored as an immutable object before the global checkpoint advances. Session ECFMP application names the committed checkpoint revision and object digest as its source revision, uses a stable command ID per strip revision, and rejects a stale source or strip. A takeover cannot repeat an uncertain provider request.

The provider-wide AFV ATIS feed follows the same global typed-page checkpoint path. Airport METAR polling commits an airport external-call intent, reserves a durable global quota slot, then stores a typed `WeatherObservation` with the stable result ID. Session owners combine the committed AFV page and airport METAR cache into typed `Atis` state; the source revision fences delayed presentation updates. An uncertain quota reservation or provider call never triggers a second request for the same poll deadline.

### HTTP PDC command identity

Task 18a binds controller/browser `IssuePdc.request_remarks` and HTTP pilot
actions to `pdc.Candidate.Plan`. Controller remarks are appended to clearance
prose composed from the accepted session, strip, runway, SID, mandatory route,
ATIS and frequency inputs. A controller cannot supply `IssuePdc.clearance`.
Its injected `TransceiverLookup` readers retain `GetFrequencies(callsign)`;
Task 20 supplies Task 19c's accepted projection reader. Only fresh operational
controller presence contributes radios, using the existing airborne priority,
frequency normalization and sector fallback helpers.
The ledger freezes the clearance and plugin CID under the command UUID. Web
responses require both the durable request's actor CID and current strip CID.

Only the authenticated Hoppie provider actor may submit
`SystemCommand.apply_pdc_provider_message` (field 14). Its command UUID equals
the parsed message UUID derived by `HoppieMessageCommandID`. The owner checks
station, request facts, current state, accepted delivery and response sequence;
stale responses produce a typed fault acknowledgment without changing the
current clearance. Other system commands remain system-only. HTTP and EFB
bodies remain JSON. `PdcEffect` adds optional `cleared` (4), `state` (5), and
`remarks` (6); `SET_CLEARED_FLAG` and `STATE_CHANGE` render existing revision-2
events. ISSUE and REVERT_TO_VOICE remain valid. Mandatory route/SID changes use
revision-checked Task 17 `SetFlightPlanEffect`s with the immutable ISSUE target.
Concrete ISSUE/REVERT effects include state/remarks and render plugin-supported
PDC state events; their original payload forms remain valid. Confirmation and
cleared-flag reset use deferred revision-checked effects under the same target.

A web PDC request keeps `callsign`, `aircraft_type`, `atis`, `stand`, and `remarks` in its JSON body. The typed `IssuePdc` action carries each value in a separate field, and `PdcSequence` retains the request fields plus clearance text and acknowledgment time for JSON reads. The command outcome retains the original aggregate and expected entity revision so an HTTP retry can rebuild the same typed request after the first commit; the ledger still compares its canonical typed hash. Pilot outcome identity is the authenticated CID, with session ID on the stored actor. The outcome query uses that CID across sessions. Provider callbacks derive a stable UUID from their provider event identity; a Hoppie frame without a separate ID uses its complete provider frame digest.

The EFB TOBT JSON `HHMM` value uses typed `SetTobt.hhmm_utc`; the owner resolves its operational UTC date from the strip, avoiding a changed command hash on a retry across midnight.
