# Task 19c candidate evidence

Base: `origin/codex/multi-node-nats-base` at `1cb4ea17`.
Branch: `codex/multi-node-nats-19c-transceiver-feed`.
Draft PR: [#821](https://github.com/flightstrips/FlightStrips/pull/821) into
`codex/multi-node-nats-base`; do not merge as part of this task.
Status: narrow candidate implementation complete; integration merge pending.
Dormant until Task 20, held from main/release until Task 24.

## Concrete construction and Task 20 assembly

- [`vatsim.NewTransceiverProvider`](../../../../backend/internal/vatsim/transceiver_candidate.go)
  accepts the existing URL, refresh interval and HTTP client configuration. It
  shares `transceivers.go`'s actual flat/nested JSON parser and normalizer, has
  no local poller/cache/callback, and converts accepted data to whole hertz.
- [`cluster.NewTransceiverFeed`](../../../../backend/internal/cluster/transceiver_feed.go)
  requires typed state/object storage and the real HTTP provider. `Refresh(ctx,
  at)` derives one UUID per configured UTC interval slot. `ExternalCallWorker`
  persists the global intent before HTTP and uses its stable result UUID for the
  verified typed object/checkpoint commit. `Resume(ctx)` runs once on takeover
  before new slots, resolving proven results or uncertainty without a call.
- `cluster.NewTransceiverSource` is the production reader for the existing PDC
  and server `GetFrequencies(callsign) []string` ports. It reads the independent
  backend projection and validates the accepted object identity, digest, schema
  and canonical ordering. It has no provider dependency or SQL repository.
  `Generation(ctx)` exposes an immutable lookup plus checkpoint revision/digest
  for a coherent policy pass. Missing/corrupt/unready state produces no values.
- [`cluster.NewTransceiverSectorReconciler`](../../../../backend/internal/cluster/transceiver_sectors.go)
  takes this source, the session owner's writer and `TransceiverSectorPlanner`.
  Task 20 binds Task 18c's concrete sector/layout/route planner to that port. The
  planner receives the immutable generation implementing the existing lookup
  port. Its domain diff commits atomically with a completed typed workflow
  containing the accepted source revision. `AppliedRevision` is durable, so
  session-owner passes compare revisions after takeover even without a local
  callback. Policy failure leaves work pending and does not consume its UUID.

The old callback is at `app.go`'s transport construction and invokes
`fsServer.RefreshAllSectors` against SQL. The candidate never invokes it.
Task 20 alone binds global scheduling, takeover recovery, both source readers
and session reconciliation scheduling. No domain implementation from Task 18a,
18b or 18c is included; the operational sector policy remains Task 18c's work.
No additional unfinished Task 19c behavior remains.

## Storage and compatibility

Only `ProviderPage.transceivers` field 13 is added, with the specified
`TransceiverFeedPage` and `TransceiverFeedClient` messages. Provider is `vatsim`,
global resource is `transceivers/v3`. The existing Object Store, checkpoint,
external workflow and session `AdvanceWorkflow` path are reused. Callsigns are
canonical, sorted and unique; hertz values are sorted/deduplicated after current
three-decimal MHz normalization. Unknown fields, bad ordering/duplicates, invalid
timestamps and empty typed pages fail verification. Failed provider responses
preserve the previous accepted generation.

Go, TS and C++ bindings are regenerated from matching root/candidate schemas;
`baseline.pb` is unchanged. Task 18a's ProviderPage12/Entity31/System14/PdcEffect4–6
and Task 18c's Entity32/System15/Effect17 reservations are untouched. Production
startup/SQL runtime, first-party JSON HTTP, ALB and release wiring are unchanged.
No Redis or internal JSON storage/transport is introduced.

## Verification

- `go test ./...` in backend: passes, including current production app tests.
- `go test -race ./internal/cluster ./internal/vatsim`: passes.
- `npm run build` in frontend: passes, including regenerated TS compilation.
- `cmake --build euroscope-plugin/build-19c --target cluster_proto_contract
  --config Release --parallel 4`: passes with MSVC Win32 and Protobuf 26.1.
- `python scripts/cluster_proto.py --check`: passes; 279 binary oneof cases and
  259 optional-zero fixtures, generated files and unchanged baseline verified.
- `python scripts/check_cluster_contract.py`: binary-path JSON/coverage passes.
- `NATS_INTEGRATION=1 NATS_TEST_PORT_BASE=4522 go test ./internal/cluster -run
  TestTransceiverFeedTwoReplicaNATS -count=1 -v`: passes on an isolated three-node
  `nats:2.15.0` fixture with two backend connections/owners/projections and a
  fresh backend replay. Each failure case uses the real provider HTTP parser:
  concurrent owner passes share one intent; nonowner calls are rejected; duplicate
  slots do not refetch; malformed/empty/unsupported shapes preserve accepted data;
  owner death after persisted intent makes zero calls; death after HTTP leaves
  uncertainty; death after the checkpoint commit before PubAck is resolved by the
  stable result ledger; all old slots replay without calls; a later slot calls
  once. Accepted frequencies and revision/digest match on fresh replay. A missed
  sector notification remains durably pending, a temporary policy failure stays
  retryable, and typed sector changes/revision acknowledgments replay together.
  This run used a private Compose copy of `backend/docker-compose.nats.yaml`
  with published ports remapped to 4522–4524, avoiding the parallel Task 18c
  fixture. The standard fixture/test defaults use 4222–4224.
- [`transceiver_ports_test.go`](../../../../backend/internal/cluster/transceiver_ports_test.go)
  proves both concrete source and immutable generation satisfy the actual PDC
  and server ports without constructing/starting a legacy cache.
- [`transceiver_feed_test.go`](../../../../backend/internal/cluster/transceiver_feed_test.go)
  covers slot/timezone identity, defensive copies, missing/unready/corrupt source,
  accepted-data preservation and noncanonical/unknown typed object rejection.
- [`transceiver_candidate_test.go`](../../../../backend/internal/vatsim/transceiver_candidate_test.go)
  covers actual flat/nested fixtures, callsign normalization, numeric ordering,
  deduplication after sub-kilohertz truncation, empty/malformed rejection and
  binary typed roundtrip.

The handoff integration fixture uses the real typed `PlanControllerSector`
replacement command with a small frequency policy. It verifies Task 19c's
durable handoff contract; Task 18c supplies operational coverage/layout/route
policy, which is assembled and qualified in Task 20.
