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
