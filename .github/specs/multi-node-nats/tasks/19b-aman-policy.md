# Task 19b — Operational AMAN evaluator and destination commands

**Depends on:** 11, 12 and the merged Task 19 candidate (#816).

**Outcome:** the candidate AMAN worker executes the existing operational AMAN policy and builds actual durable session commands without test-only evaluator/command callbacks.

**Release boundary:** held on the integration base under the Task 19 rule in [release-safety.md](../release-safety.md); activate only at Task 20 and release at Task 24.

## Contracts

- Provide concrete implementations of `AmanObservationEvaluator`, `AmanReconciliationEvaluator` and `AmanIntentRunner.Step`, packaged as a candidate constructor suitable for Task 20. Use a package that can depend on cluster contracts and AMAN domain policy without creating an import cycle.
- Read the complete accepted airport board through `AmanAdapter`, typed VATSIM checkpoints, shared EuroScope observations, and committed navigation/weather/configuration state. Preserve operational rollout/health gates, prediction, sequence, lifecycle, coordination and audit semantics. Process-local caches are rebuildable read caches and cannot decide accepted authority.
- Output is the existing typed `AmanTransition`: complete active board and changed coordination/audit/validation/observation/workflow records. Existing typed AMAN storage records are canonical; no alternate format, SQL-backed persistence, or JSON serialization is introduced.
- Commit airport board and pending session intents in the same accepted airport event. Each workflow records the accepted source revision, destination session, step and immutable derived command UUID. The production Step builder derives the action from accepted typed source state and creates the recorded destination command. It may change only the fields owned by that AMAN action and preserves unrelated strip/controller edits.
- Destination handling rechecks source authority/revision and current session state. Lost replies and takeover retry the same typed command UUID. A superseded source revision terminates its pending workflow as `SUPERSEDED`; it cannot create a new effect under the old UUID. An earlier committed destination outcome remains recoverable.

## Work

- Bind or extract the existing `aman/operational` observation/reconciliation policy so candidate evaluation runs over the typed board. Include present/missing VATSIM flights, EuroScope surveillance precedence, navigation/weather inputs, prediction and lifecycle/sequence changes.
- Implement the real airport-to-session command builder and destination planner for operational AMAN actions. Wire the concrete callbacks into candidate construction while keeping `app.Build` dormant until Task 20.
- Replace test-only operational assumptions with tests of the concrete evaluator and builder. Update the Task 19 checklist and worker inventory with constructor and evidence links.

## Done when

- Real two-replica NATS tests run the production evaluator/builder through VATSIM arrival observation, disappearance, reconciliation, sequence updates and an actual airport-to-session action.
- Kill the airport owner after destination commit and before intent completion; takeover proves the prior result without a second destination write. Advance the source revision before dispatch; the old intent becomes superseded without applying stale state.
- Relevant existing AMAN policy tests remain valid and the candidate needs no SQL repository or injected fake evaluator. Only candidate startup binding and the final inventory audit remain for Task 20. Do not mark completion while any operational policy or destination action is a stub.
