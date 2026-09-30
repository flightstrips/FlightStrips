# Task 18c implementation and evidence

Implemented on `codex/multi-node-nats-18c-euroscope-deadlines`, based on
`f5fa8bce`. Only runtime assembly remains for Task 20. Candidate startup stays
dormant and this change remains held until Task 24; no merge or production
activation is part of this task. ALB, Redis and enabled landing validation are
outside the change. Slots 31/14/ProviderPage 12 and PDC payload slots 4–6 remain
available for Task 18a; no Task 18b CDM/traffic fields are added here.

## Concrete assembly and outcomes

`euroscopebinary.NewDeadlineCandidate(router, acceptedSource, nextPlanner)`
requires actual owner routing, projection, writer, accepted typed provider
objects and the next domain planner. It constructs no SQL repository and starts
no loop. Task 20 binds `candidate.Planner` to the owner writer, runs
`candidate.Serve`, binds `Handler.Inbound`/`Handler.Deadlines`, calls
`candidate.BindWorker`, and closes its position writers on shutdown. Other
binary domain adapters compose through `NextInbound` or the runtime's inbound
chain; Task 18b continues owning EOBT/ELDT admission. `BindWorker` installs real
shared recovery, retention, sector/layout/route and position-barrier callbacks.
`SessionWork.ReconcileSession` runs the same healthy owner pass as its registry
supervisor, making the concrete expiry path independently callable.

- Authenticated binary login, controller online/offline, sync, strip observation,
  assigned squawk, position and disconnect events enter owner validation. Remote
  socket admission uses binary Core NATS envelopes, with authenticated routing
  metadata rechecked on the owner. Master-only frames are generation fenced.
- Controller observations have no invented CID. Callsign/frequency reports use
  scalar master-epoch workflow observations; authenticated controller rows and
  effect targets retain actual CIDs. Fresh cross-node presence takes precedence
  over a contradictory offline report. Offline network coverage remains through
  its durable grace deadline when reconciliation runs.
- Controller/session admissions and master changes persist required debounce or
  rearm in the same transition. Accepted KV tombstones and shared socket leases
  repair the observation/FS_STATE scheduling boundary after process death. The
  defaults remain 15s controller, 60s aircraft, 45s master transfer and 300ms
  debounce. Newer source revisions cancel or replace deadlines.
- Retention reads the accepted global VATSIM checkpoint and the session's exact
  source cursor/digest, lifecycle bookings, protected occupancy, controller
  modifications, coordination and fresh position state. Missing, stale or
  unavailable source prevents deletion. The real expiry holds the owner position
  barrier, rechecks the tombstone and preserves newer edits. Retention retracts
  EuroScope provenance so the same tombstone cannot reschedule consumed work.
- Reconciliation invokes existing sector and layout configuration policy and
  `server.ComputeCandidateRoute`, a pure entry point into existing route policy.
  It maps policy frequencies to authenticated CIDs for the durable route, skips
  active coordination and patches only derived fields in a revision-checked
  complete event. Missing runway input leaves reconciliation pending.
- Automatic and typed browser squawk actions create `GenerateSquawkEffect`,
  payload 17. Admission retains the live target CID. WAITING effects are ordered
  by accepted stream sequence then UUID. Same UUID uses the durable ledger;
  another waiting request for a callsign receives `SQUAWK_ALREADY_PENDING`.
  Recovery derives automatic IDs from the accepted observation and never
  recreates an accepted uncertain intent.
- Claim reducer cancels assigned/removed aircraft and enforces queue/rate rules.
  It materializes `SessionSquawkThrottle`, entity case 32, in the very same
  transition as DISPATCH_CLAIMED, using JetStream server time plus five seconds.
  Direct throttle mutations are rejected; snapshot restore validates it. Lost
  claim PubAck cannot authorize a send. Claimed effects never requeue; Task 17's
  30s uncertainty becomes UNKNOWN.
- Native revision-2 squawks retain original result terms while waiting for a
  radar target. The request is removed before one TopSky invocation, then records
  EXECUTED/FAILED in the existing result outbox. It expires after 30s without a
  local retry. The legacy queue retains its two-minute delivery timeout. Native
  EXECUTED denotes local TopSky invocation, not later squawk assignment.

## Real two-replica NATS evidence

`backend/internal/euroscopebinary/candidate_integration_test.go` constructs two
independent owner/projection/router/fanout/candidate replicas and actual binary
WebSocket handlers against the pinned three-server NATS 2.15.0 fixture. Concrete
production policies satisfy these tests; retention and reconciliation are not
injected substitutes. Tests use an isolated copy of the fixture on ports
4422–4424 to avoid concurrently developed schema cases in the shared fixture.
Set `NATS_INTEGRATION=1` and `NATS_TEST_PORT_BASE=4422` to use that copy.

- `TestDeadlineBinarySharedCoverageReconnectAndReconciliation`: actual binary
  login/sync/controller events, remote authenticated coverage, controller reports
  without CID, reconnect cancellation, contradictory offline reports, concrete
  sector and layout outcomes, and real route CIDs.
- `TestDeadlineBinaryAircraftRecoveryRetentionAndNewObservation`: actual binary
  disconnect and new strip observation, accepted tombstone recovery with a new
  candidate instance, missing-source failure, protected stand occupancy and
  accepted VATSIM retention through the actual worker callback.
- `TestDeadlineBinaryMasterChangeTakeoverRecoveryAndExpiry`: owner death between
  KV tombstone and deadline scheduling, stale-generation gating, full-sync
  recovery, master transfer rearm and concrete shared absence deletion.
- `TestDeadlineBinaryOfflineExpiryAndSessionDisconnectReconcile`: actual socket
  close, owner offline expiry, debounced sector/route withdrawal and concrete
  session-disconnect callback.
- `TestDeadlineBinaryScheduledSnapshotTakeoverExpiry`: verified Object Store/KV
  snapshot, independent projection replay, owner death after scheduling, new
  generation sync, real expiry and inability to recreate a consumed deadline.
- `TestSquawkBinaryQueueRateResultsCancellationAndTakeover`: automatic binary
  sync requests and actual typed browser actions share one queue, UUID duplicate
  and pending rejection, physical revision-2 effect frames, binary plugin result
  persistence, assigned-code cancellation, immutable target and throttle through
  owner death, and no retransmission after claim.
- `TestSquawkBinaryLostClaimAckUnknownAndOwnerDeath`: actual committed claim with
  deliberately lost PubAck, zero socket sends, takeover, real 30s UNKNOWN expiry
  and an identical retry without requeue.

Grace defaults are asserted against real scheduled times. Deadline expiry tests
then rearm a due timestamp through the typed owner command to avoid waiting a
minute for every scenario; they execute `SessionWork.ReconcileSession` with the
actual callbacks. Owner lease expiry, five-second claim authority and the
30-second UNKNOWN path are not accelerated. Reducer tests in `squawk_test.go`
exercise the exact five-second server-time boundary, clock skew, atomic throttle
and snapshot collision with the same-key session seed, queue order, removal,
assigned cancellation and forbidden UNKNOWN-to-WAITING transition.

## Validation

- Full backend `go test ./...` passed with one manually started PostgreSQL 16
  fixture and a migrated `testdb` template through the existing
  `FLIGHTSTRIPS_TEST_DATABASE_SERVER_URL` seam. This avoids Windows
  testcontainers' rootless-Docker limitation; no application code was changed.
- Backend `go build ./...` and targeted cluster/socket/server tests passed.
- The actual two-replica socket/deadline/squawk suites above passed.
- `python scripts/cluster_proto.py --check` passed: 281 oneof cases and 259
  optional-zero fixtures; frozen compatibility baseline remains unchanged.
- `python scripts/check_cluster_contract.py` passed.
- Frontend production `npm run build` passed.
- Win32 Release MSVC/CMake native plugin build passed; all 440 native CTest tests
  passed, including existing command-outbox reconnect/no-double-execution tests.
  The live EuroScope/TopSky environment is not exercised by this repository
  harness; plugin invocation behavior is compiled and its binary result path is
  exercised through real sockets.

Task 20 also binds `VatsimLifecycleCandidate.Positions = deadlineCandidate.Positions`
so Task 19a lifecycle commands share the actual binary position dispatcher.
A closed or unsynced generation cannot grant a lifecycle barrier; writer close
waits for any in-progress expiry/lifecycle commit before replacing it.

`TestDisconnectExpirySerializesReappearanceAndRejectsOldTombstone` verifies
worker/socket barrier ordering and prevents stale deletion after reappearance.

Full-sync owner admission has a bounded two-minute default (configurable via
`SyncAdmissionTimeout`); individual frames retain a five-second hop. This
allows multi-observation sync without granting any stale-generation admission.
The seven-scenario suite passes against the isolated fixture. The harness waits
for a non-vacant master and validates its socket identity before sending Sync;
successful election reconciliation can also return a vacant term while node
presence is warming up. Production generation fencing remains unchanged.

## Integration-base merge resolution

Merged integration base `5fd4c62e`, preserving Task 18a's PDC entity/record 31,
system command 14, provider page 12 and optional PDC effect fields 4-6 alongside
Task 18c's slots. Reducer and snapshot validation accept both session entity
kinds and retain the Hoppie session checkpoint exception. Bindings were
regenerated from the combined schema without replacing the compatibility
baseline: 285 oneof cases and 266 optional-zero fixtures pass. All seven actual
two-replica EuroScope deadline/squawk scenarios pass after this resolution.
Targeted cluster, socket, PDC and VATSIM tests, backend/frontend builds and all
440 native tests also pass. Actual NATS PDC polling/snapshot/takeover,
transceiver-backed clearance and the transceiver provider-boundary scenarios
pass with the combined schema and validation rules.
