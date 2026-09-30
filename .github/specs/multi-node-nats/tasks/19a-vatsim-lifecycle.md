# Task 19a — VATSIM departure and arrival stand lifecycle

**Depends on:** 08 and the merged 18/19 candidates (#816, #817).

**Outcome:** the real departure/arrival lifecycle runs through session-owner commands using committed VATSIM generations and shared observations. Completes the matching gaps in Tasks 18 and 19.

**Release boundary:** held on the integration base under the Task 18/19 rules in [release-safety.md](../release-safety.md); activate only at Task 20 and release at Task 24.

## Contracts

- Input is a committed typed `VatsimPage` and its global source revision/digest, the session projection, revisioned position observations, and existing static stand/aircraft/airport policy. An empty or unavailable provider response is not an accepted missing-flight generation.
- The session owner owns departure/arrival transitions. All authoritative output uses existing typed `Strip`, `StandAssignment`, `StandBlock`, `SessionDeadline`, `WorkflowRecord` and Task 17 effect records. Preserve their existing schemas, keys, CAS rules, source revisions and occupancy checks. There is no additional persistence format or provider fetch in this task.
- Stable command identity must include session, source generation or persisted deadline, callsign and action. Repeated input or takeover recovers the same ledger outcome. The owner planner rechecks current entity/source revisions and positions before committing. Strip/stand/block changes for one transition commit together.
- `SessionWork.Departure` and `SessionWork.Arrival` receive concrete adapters with the existing `func(context.Context, int32) error` shape. A exported candidate constructor supplies both callbacks for Task 20; the candidate tests use those callbacks rather than injected stand decisions.
- Automatic private messages and plugin actions use the durable Task 17 effect path with immutable target CID. Authoritative episode/deadline state must survive restart; an in-memory warning cache must not control whether a message is emitted.

## Work

- Port existing behavior from `services/departure_lifecycle.go`, `services/arrival_lifecycle.go`, their helper files/tests and the VATSIM reconciler. Preserve prefile opt-in defaults, departure reservations/blocks, timing rollover, arrival ESTIMATED/ASSIGNED/CONFIRMED stages, physical occupancy protection, displacement handling, retention, cancellations and wrong-stand episode behavior.
- Connect generation reconciliation and persisted sweeps to the adapters without introducing a second provider poller. Account for flights that disappear, reconnect, change plan/CID, or become operational EuroScope strips.
- Keep candidate construction independent of SQL repository/transaction calls. Update both worker inventories and the Task 19 checklist with the concrete constructor and test evidence.

## Done when

- Real two-replica NATS tests use the production lifecycle adapters and prove nonowner rejection, replay, takeover before/after a transition commit, reservation/block/arrival deadline recovery, disappearance cancellation and retained occupancy protection.
- Tests preserve current stand lifecycle behavior, including wrong-stand effects, and show one effective committed transition/effect per source input.
- Neither adapter requires PostgreSQL or an injected replacement lifecycle policy. Only candidate startup binding remains for Task 20. Do not mark completion if any lifecycle path is still stubbed.

## Candidate implementation and evidence

`services.NewVatsimLifecycleCandidate(source, writer, stands, secrets)` exports
`Departure` and `Arrival` with the required callback signatures. Task 20 binds
those callbacks and the current owner/master position dispatcher through
`Positions`. Prefile allocation defaults to disabled. Construction requires
the existing typed source object store, owner writer/projection, SAT registries
and Task 17 encryption keys; no PostgreSQL connection is needed.

The adapter executes the existing lifecycle services and SAT helpers against
an isolated domain snapshot, then commits each strip/assignment/block/episode
diff under the owner with subject CAS. It uses deterministic weighted SAT
draws, arrival priority and bounded convergence. A position barrier drains
reports and compares the complete current master KV set before publication.
Stale observations preserve occupancy during takeover until newer accepted
source data or a freshly synced master supplies replacement evidence.

Wrong-stand warning episodes are persisted with assignments or typed workflows.
Private messages use encrypted Task 17 effects. STAND actions commit a pending
workflow alongside the strip, then resume into a separate Task 17 effect with
the original controller CID; stale strip intents are superseded. The existing
`WorkflowRecord` schema is used, without another store or serialized payload.
Typed Strip ground state and engine type retain the inputs used by PARK/PUSH
and aircraft compatibility policy; all existing field numbers remain stable.

Run against `backend/docker-compose.nats.yaml` (pinned three-node NATS):

```powershell
$env:NATS_INTEGRATION='1'
go test ./internal/services -run '^TestVatsimLifecycleTwoReplica' -v -count=1
```

- `CommitFailuresAndReservationRecovery`: nonowner rejection; death before
  publication and after durable commit before PubAck; takeover, unchanged
  replay, snapshot/replica restart and persisted reservation expiry with the
  prefile still present; replay cannot recreate that expired hold.
- `ArrivalCancellationWrongStandAndEffects`: all arrival stages, prefile
  defaults, disappeared-flight cancellation, retained wrong-stand occupancy,
  one encrypted warning per episode and takeover replay.
- `SharedPositionsAndStandEffectRecovery`: shared positions override provider
  positions; newly admitted aircraft invalidate the position barrier; physical
  blocks and pending STAND intents survive owner death; immutable effect CID;
  fresh vacancy, position-only departures and PUSH release.
- `PlanIdentityParkedArrivalAndBlockDeadlines`: changed plan/CID renewal,
  physical PARK arrival, persisted ALDT retention and stand-block expiry after
  takeover, disappeared prefile cleanup and no expired arrival recreation.

Pure `TestLifecyclePlan*` tests additionally cover timing rollover, confirmed
arrival protection, committed block adjacency and physical displacement with
retained advisory state.
Backend tests, the Protobuf generation/contract checks and frontend build
validate the shared schema and effect rendering. The Win32 MSVC
`cluster_proto_contract` target also builds successfully. Candidate startup remains
dormant until Task 20; main/release remains held until Task 24.
