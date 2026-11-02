# Proposal consistency review and planning readiness

Reviewed on 2026-10-30 against the architecture, configuration requirements,
schema/semantic validator, both combined examples, executable handler proof,
reference notes, and relevant application assembly/command/PDC code.

**Decision update — 2026-11-01:** the user has resolved D1–D5. The proposal is
ready to turn into implementation epics with the confirmed scope below. Airport
operating values remain rollout inputs; engineering details belong in the work.
The 2026-10-30 audit remains useful evidence, with its findings updated below.
The review itself creates no tracker issues, runtime activation, or deployment.
The subsequent [implementation plan](epic-plan.md) now records eight repo-local
epic drafts with child issues, dependencies, and acceptance criteria. It was
prepared after the independent architecture review below.

## Independent final review: 2026-11-02

An Astra sub-agent independently reviewed the proposal, supporting documents,
schema/examples, handler proof, and relevant production code. **No architecture
or epic-planning blockers remain.** The confirmed D1–D5 decisions are consistent;
documented proof limits and E1–E7 implementation work are not reopened decisions.

The review found one P2 authoring-tool defect: the schema allowed environment-backed
region files, but the semantic validator assumed `repo_path` and raised a
`KeyError`. The validator now supports both declared resource forms, honors an
explicit environment override, uses the default for unset/blank values, and
reports missing selected files as validation errors. Focused checks were added.
Astra verified the correction and closed the finding.

Final validation preserves both combined examples and all 20 EKCH settings,
rejects 34 invalid cases, and checks region-resource resolution. This independent
review does not claim full runtime integration, EKCH parity, or production
performance validation; those remain the explicitly scoped implementation work.

## Settled requirements carried forward

- Compiled workflow handlers with concrete typed settings; one combined file
  per airport, with large domain assets referenced rather than copied.
- Shared RADIS implementation/view for EKBI/EKYT where behavior matches;
  independent airport boards, strips, order, and session/environment state.
- No RADIS strip/sector ownership. Writable non-observer participants use their
  own plugin for controller-initiated edits. Disconnected boards stay read-only.
- Independent configurable coverage/master policy, tuned-radio consideration,
  bounded relinquishment on takeover, automatic rejoin, and existing offline
  behavior. The already agreed takeover timing is not reopened here.
- Shared VATSIM/transceiver acquisition and caches; shared application PDC
  service, provider client, logon, and polling coordination. Configuration,
  permissions, clearance state, and operational results keep explicit scope.
- Coded frontend views, one selected session per tab, airport switching, and an
  initial chooser only when multiple eligible sessions exist.
- Repository configuration/restart deployment. New handler/view code is expected;
  no general rule interpreter, runtime script system, or code-free onboarding.

## Inconsistencies corrected or made explicit

| Finding | Resolution in the proposal |
| --- | --- |
| Requirements still described a declarative transition model and old discovery text said handler selection was pending. | Replaced the current model with coded decisions/settings and marked the discovery chronology as historical. General handler selection exists in the example schema; production integration remains future work. |
| The overview and membership diagrams could imply a service instance/client per session. | Show shared application services explicitly; sessions own partitions of operational state, not duplicate global clients/workers. |
| The text described handlers invoking services, while the proof returns pure decisions. | Handler decisions describe typed core/domain operations; the executor invokes services outside the decision's critical section. Handlers own no I/O, worker, transport, or mutable current-airport state. |
| The proposal required waiting for ES confirmation before moving a bay, while the sample commits immediately. | Identified the sample's immediate in-memory commit as a proof limit and recorded D3: use the current EKCH move/error/reconciliation behavior. No new ES-confirmation gate or pending-move framework is required. |
| Own-plugin routing for human edits was stated too broadly for automatic PDC work. | Keep the human rule fixed. D4 now permits genuine automatic work through the session master or controller who has assumed the aircraft. Retain the exact own-plugin binding for human-originated work. |
| PDC could appear ready for multiple airports because its client is already shared. | Distinguish existing shared transport from remaining work: validation still reads global config, current polling returns on an airport error, and mailbox/session handover needs response-correlation work if PDC is later activated at additional airports. PDC stays disabled at EKBI/EKYT now. |
| Full-file schema validation could be confused with a complete production Go loader. | State that Python checks the complete documents; the Go proof strictly decodes only the workflow projection and selected context. Schema/Go parity is implementation work, not proven by two valid fixtures. |
| The proof could imply a complete extensible action dispatcher or admission model. | Its core still knows the two sample command kinds, indexes strips by callsign, and bypasses production admission. Those limitations are explicit; production needs handler action descriptors, stable strip identity, and admission/lifecycle hooks. |
| Benchmarks could be interpreted as a no-regression guarantee. | Retain them as historical in-memory measurements. Representative EKCH baseline/load and production acceptance budgets remain engineering work. |
| Broad first-release service reuse and deferred airport parameters were mixed together. | D1/D2 now fix the first release: current RADIS prototype features plus restoration; PDC and AMAN disabled at EKBI/EKYT, with no scheduled activation. |

## Confirmed delivery decisions

Resolved by the user on 2026-11-01. These supersede the recommendations from the
initial audit and should be copied into affected epic scope/acceptance criteria.

### D1 — First RADIS feature boundary: resolved

- Include all current prototype features and editing/menu controls, with the
  already agreed integrated shared-board/own-plugin behavior. A feature in the
  demo is not permission to retain browser-only state or feed-only writes.
- Include notes, de-icing marks, and REA/ATA timestamps.
- Include restoration of manually deleted strips in the first release. Recovery
  limits and detailed airport mappings remain rollout/handler inputs.
- NEW creates a shared airport-local strip only. ES flight-plan creation is
  deferred. Creation itself is not an ES acknowledgement; ordinary readiness
  and observer restrictions still apply.
- FIND includes connected pilots only, whose flight origin or destination is the
  selected airport. Prefiles and unrelated flights are excluded.

All current features refers to the reviewed RADIS prototype baseline, not every
EKCH/ENGM feature. The explicit PDC/AMAN exclusion in D2 still applies. Detailed
field semantics and values must be supplied without silently broadening NEW or
FIND. The prototype review records limitations and incomplete controls.

### D2 — First-release service rollout: resolved

Plan EKBI/EKYT with CDM, stand assignment, and text-pilot support. **PDC and AMAN
are disabled there**, with no committed activation date; the user expects AMAN
may remain disabled for a long time. Do not make new-airport activation or a
complete multi-airport PDC/AMAN migration a prerequisite for RADIS delivery.

Preserve current EKCH services. PDC remains a shared application service/client,
logon, and polling coordinator wherever enabled. New airport settings remain
scoped so introducing RADIS does not disturb EKCH. Additional PDC mailbox or
AMAN activation work can be planned only when those capabilities are requested.

### D3 — Board behavior during ES operations: resolved

**Use what EKCH does today** for move execution, visible placement, validation,
error handling, and reconciliation, adapted to RADIS's own handler/mappings and
lack of ownership. Reuse the current production move flow rather than introduce
an additional acknowledgement gate, pending-move UI/state machine, or durable
action framework for this release.

The relevant current paths are
[frontend moves](../../../backend/internal/frontend/handlers.go),
[move execution](../../../backend/internal/services/strip_frontend_move.go), and
[ordering/publication](../../../backend/internal/services/strip_ordering.go).
Preserve existing error/rollback behavior where applicable. Neither queue
acceptance nor the in-memory proof is an ES acknowledgement or full parity test.

### D4 — Automatic service authority: resolved

Genuinely automatic work may use **the session master or the controller who has
assumed the aircraft**, via the existing appropriate service/aircraft executor
resolution. The chosen plugin must belong to the same session and be available
and authorized for that operation. Retain current failure/unavailable-executor
behavior rather than invent a new service-execution system.

Human-originated edits, including work deferred through a service, still use
that human's own bound plugin. Do not relabel a human request as automatic to
use master fallback. PDC is disabled at EKBI/EKYT; preserve existing EKCH PDC
behavior rather than make new-airport automatic PDC integration a release gate.

### D5 — Deployment and compatibility: resolved

Deploy in a window when **no controllers are online**, updating the backend,
frontend, and plugin together. The user does not require concurrent legacy/new
client support. Remove dual-protocol endpoints, legacy-client adapters, and a
long compatibility window from this delivery plan.

Keep the new session-aware protocol internally coherent and verify that everyone
is offline before applying the deployment/migrations. Offline rollout does not
remove the need for correct reconnect/session isolation after deployment or
preservation of EKCH data/behavior. Use the existing deployment process; no new
rollout orchestration or online failover system is required.

## Engineering decisions to resolve within the relevant epics

These are scoped implementation details, not unresolved product questions or
prerequisites for drafting epics. Keep them proportionate to the existing code
and the agreed offline deployment; use the existing mechanisms where possible.

| ID | Decision/work and recommended direction | Must be resolved before |
| --- | --- | --- |
| E1 | Production registry activation/loading and validation. Use an explicit deployment list of airport files, never auto-activate examples by scanning the repo. Recommend atomic validation of selected definitions before startup; reuse the existing offline deployment/failure process. Compose full domain validators, enforce exact handler settings and supported views, and test schema/Go parity. | Production registry integration/activation. |
| E2 | Production handler/action/effect contract. Register typed supported actions with each handler instead of extending the proof's core command allowlist. Specify pure decisions, service requests, lifecycle/coordination hooks, and typed contracts. Follow the existing EKCH execution/error path and persist using current concurrency mechanisms; do not perform network I/O while holding strip/session locks or transactions. | Migrating real actions/services behind the boundary. |
| E3 | Protocol identity, source precedence, and concurrency. Define membership/master epochs, request/result correlation, board-order conflict handling, stable strip/incarnation identity and optional flight association. The same callsign can be reused or refer to an airport-local strip. Feed observations must not overwrite higher-authority ES/controller facts. | Protocol/persistence implementation. |
| E4 | Coverage evidence precedence and freshness. Distinguish unknown/stale staffing or radios from a confirmed takeover/absence. Recommend preserving an established membership on transient unknown evidence while reevaluating, without overriding the agreed deadline after a confirmed takeover. Define simulation-source isolation and the zero-membership demand path. | Coverage coordinator behavior tests. |
| E5 | Deferred PDC mailbox/session handover and external reply correlation when another airport is enabled. One coordinator owns a LIVE mailbox; persist/identify outstanding transactions across session turnover. Session-local sequence IDs alone may be ambiguous externally after reuse. Correlate mailbox, aircraft, external response ID, and owning transaction; reject/hold ambiguous replies. Ensure one mailbox error does not stop the rest. | Future additional-airport PDC activation; not an EKBI/EKYT release gate. |
| E6 | Persisted state, handler versions, config revision, and disablement. Preserve EKCH state in the offline migration and define the required new fields. Reuse existing cleanup/service-state handling; do not build an online old/new compatibility layer. Removing a membership or tab does not own global service lifetime; disabling an airport does not delete state or authorize reinterpreting it as EKCH. | Offline migration/restart and per-airport disablement acceptance. |
| E7 | Performance acceptance and observability. Measure representative EKCH before/after behavior with simultaneous airports, controller/timer/feed workloads, and slow external services. Agree regression/load budgets from those measurements; the sample's sub-microsecond dispatch is not the budget. Count shared workers/requests and keep locks scoped. | Production performance acceptance, not epic drafting. |

## Airport rollout inputs that may remain open during epic planning

| Input | Required before |
| --- | --- |
| Coverage responsibilities, tuned-frequency/position lists, master priorities, grace value, and relevant regions | Activating that airport's participation policy. |
| Runways/procedures, allowed actions, bay-to-ES mappings, arrival/departure/local handling, admission/removal/recovery thresholds | Operational acceptance of that airport's handler. |
| RADIS field/annotation semantics, timestamp triggers, REA/EOBT behavior, warning/block/override policy | Enabling the affected menu/action. Prototype constants are not defaults. |
| CDM, stand, phrase/view, PDC validation, and any AMAN data | Enabling that airport/service pair. |
| Named operational reviewers and controlled scenario approval | Airport release. |
| ENGM draft ambiguities, supervisor role, target/clearance mappings, and optional features | A future ENGM design/release; these do not block EKBI/EKYT foundations. |

## Evidence and practical limits

The [schema report](schema/validation-report.json) records two combined configs,
lossless preservation of all 20 EKCH YAML settings, and 34 rejected invalid
inputs. The [Go proof](examples/handler-proof.patch)
tests handler binding, the actual EKCH bay helper paths, custom fictional
behavior, isolated sessions/config, guards, revision conflicts, and snapshots.
Race checks include that package and the existing shared bay helpers.

Neither suite tests real multi-airport membership, WebSockets, database
transactions, service integration, ES acknowledgement, persisted restart,
controller-feed freshness, or production performance. No test should be described
as full EKCH migration/parity or RADIS operational validation.

Useful application evidence:
[shared service assembly](../../../backend/internal/app/app.go),
[PDC client](../../../backend/internal/pdc/client.go),
[PDC service/automatic routing](../../../backend/internal/pdc/service.go),
[current global PDC validation](../../../backend/internal/pdc/validation.go),
[strip concurrency queries](../../../backend/queries/strips.sql), and
[existing bay reconciliation](../../../backend/internal/shared/bays.go).

The epic descriptions should link these confirmed D1–D5 outcomes and the
relevant engineering items, separating delivery acceptance from airport-data
gates. The [eight epic drafts](epic-plan.md) now assign this work. No further
product decision in this register blocks planning; actual
airport values and operational acceptance are still required before activation.
