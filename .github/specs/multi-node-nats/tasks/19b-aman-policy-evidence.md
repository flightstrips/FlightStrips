# Task 19b implementation and evidence

Candidate implementation is complete on `codex/multi-node-nats-19b-aman-policy`,
based on `86e27645`. Integration merge remains pending. Startup stays dormant
until Task 20; release remains held until Task 24. Task 19a, ALB and Redis are
outside this change.

The branch also includes integration base `c49b1940` (Task 19a, PR #819).
Its `Strip.ground_state` and `engine_type` field numbers 71 and 72 are
preserved; the unreleased AMAN strip additions use 73–77. Bindings were
regenerated, and descriptor compatibility was checked against that entire
integration base as well as the frozen release baseline. Both AMAN and
VATSIM lifecycle two-replica NATS suites pass on the combined branch.

## Concrete assembly

- [Constructor and evaluators](../../../../backend/internal/amancandidate/worker.go):
  `amancandidate.New` binds `EvaluateObservation`, `EvaluateReconciliation` and
  `Worker.Step` into the existing candidate and resumable intent runner.
  `operational.Service.EvaluateAccepted` executes the existing prediction,
  sequence, holding/FIFO, lifecycle and health policy once. Its repository
  captures a complete `StateCommit` for one airport CAS; it never persists SQL.
- [Accepted inputs](../../../../backend/internal/amancandidate/inputs.go):
  typed global VATSIM checkpoints, independent stored provider facts, session
  strips and fresh master-fenced EuroScope positions. A feed outage is not new
  disappearance evidence; missing arrivals cannot reacquire sequence slots.
- [Navigation](../../../../backend/internal/amancandidate/navigation.go) rebuilds
  geometry/routes from verified accepted immutable objects. Cold routes use
  the existing AIRAC resolver behind a durable external-call intent/checkpoint.
  Weather reads accepted Open-Meteo pages with the existing grid/hour policy.
  [TerminalPolicy](../../../../backend/internal/amancandidate/terminal.go) provides
  the typed import boundary for runway, feeder, final-approach, holding-transit,
  STAR-family settings and validated TMA polygon rings. The candidate never
  loads a local TMA file at evaluation time. Missing/mismatched accepted policy degrades
  authority; local constructor settings cannot enable it.
- [Destination commands](../../../../backend/internal/amancandidate/destination.go)
  derive immutable holding EAT actions from accepted policy output. Pending
  workflows and the board share one airport event. The session planner verifies
  the workflow, UUID, source AMAN revision, exact action, airport and current
  hold, then patches only EAT and its AMAN ownership marker. Other strip edits
  survive. A typed targeted holding effect shares the destination commit.
  Withdrawal clears only an AMAN-written value, preserving a controller's
  replacement. The plugin clears AMAN replay authority without cancelling a
  hold or inventing an unsupported empty TopSky EAT pulse.
- [Audits](../../../../backend/internal/amancandidate/audit.go) retain freeze,
  go-around, queue-promotion, capacity and coordination expiry facts. Existing
  in-process legacy payloads are decoded into closed typed messages; no JSON
  is stored in candidate events or objects.
- [Recovery](../../../../backend/internal/cluster/aman.go) reads the destination
  ledger before superseding a source workflow. An accepted destination commit
  remains proven with a pending or terminal effect. Lost replies reuse its
  recorded UUID; a stale uncommitted action cannot create an effect.

## Real NATS evidence

Tests construct two independent backend projections, owner runtimes and command
routers against the pinned three-server NATS 2.15.0 fixture. All use the production
constructor, evaluator, builder and planner. Seeded typed navigation, weather and
controller facts are inputs, not replacement evaluators.

- [TestOperationalTwoReplicaNATS](../../../../backend/internal/amancandidate/worker_integration_test.go):
  nonowner rejection, arrival prediction/sequence, observation replay, real EAT
  action/effect, later controller edit preservation, destination commit before
  airport-owner death/completion, newer source revision, takeover with one
  strip write/effect, disappearance and removal.
- [TestOperationalSupersededDestinationNATS](../../../../backend/internal/amancandidate/supersession_integration_test.go):
  tampered policy output rejected; source advance before dispatch; stale UUID
  rejected; takeover records SUPERSEDED without a destination mutation.
- [TestOperationalRolloutGatesNATS](../../../../backend/internal/amancandidate/supersession_integration_test.go):
  shadow, read-only, disabled and stale-source health prevent EAT authority.
- [TestOperationalSharedEuroScopeNATS](../../../../backend/internal/amancandidate/shared_observations_integration_test.go):
  shared EuroScope surveillance takes precedence; VATSIM absence retains the
  ES arrival; subsequent holding facts carry their own timestamps.
- [TestOperationalHoldingWithdrawalNATS](../../../../backend/internal/amancandidate/withdrawal_integration_test.go):
  owned EAT withdrawal and later controller replacement preservation,
  including command replay and exact effect counts.

Run from `backend` with `NATS_INTEGRATION=1`, optionally setting
`NATS_TEST_PORT_BASE` for an isolated compose fixture:

```text
go test ./internal/amancandidate -run TestOperational -count=1 -v
go test ./internal/aman/... ./internal/vatsim ./internal/app ./internal/cluster ./internal/amancandidate ./internal/euroscopebinary ./pkg/events/cluster
go vet ./internal/amancandidate ./internal/euroscopebinary
```

Contract validation: `python scripts/cluster_proto.py --check` and
`python scripts/check_cluster_contract.py`. Additive changes regenerate Go,
TypeScript and C++ without replacing the compatibility baseline. Frontend
TypeScript check: `npx tsc -b`. Windows Release plugin build and all 440 CTest
cases pass, including TopSky matching and command-outbox coverage.

Broader cluster `go vet` reports existing protobuf lock copies in
`ecfmp_test.go:36` and `vatsim_reconcile_test.go:60`; these files are untouched.
The harness retries explicit pre-publish lease-renewal admission gates with
the same identity; planner failures and accepted failures remain test failures.

## Task 20 handoff

Bind the constructor, schedule accepted observations/reconciliation and
takeover `Resume`, route `ApplyAmanSession` to `DestinationPlanner`, include
`TerminalPolicy` with the validated TMA volume in the terminal import, and map EuroScope strip
hold fields with `euroscope_observed_at`. Reuse the effect runtime/renderer.
Repeat the startup inventory audit at activation. These are assembly bindings;
no operational evaluator or destination-action callback remains to implement.
Do not start legacy and candidate workers together, merge to main, or release
before the coordinated Task 24 boundary.
