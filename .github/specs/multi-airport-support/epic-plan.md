# Multi-airport implementation epic plan

Status: repo-local epic drafts, planned on 2026-11-02 from the reviewed
[architecture](architecture-proposal.md) and confirmed
[D1–D5 decisions](planning-readiness.md#confirmed-delivery-decisions).
MA-01–MA-08 are planning identifiers, not published tracker issues. This document
plans implementation; it does not activate airports or mark work complete.

## Release outcome and scope

One backend and one physical connection per EuroScope instance support independent
airport memberships, mastership, operational state, and browser tabs. EKCH keeps
its existing behavior through a coded handler. EKBI/EKYT use RADIS with the
current prototype feature set, shared annotations, deletion/restoration, and
CDM/stand/text-pilot integration. Shared sources and external clients remain
application-owned; live operational state has explicit airport/session scope.

NEW creates only an airport-local shared strip. FIND includes connected pilots
to/from the selected airport and excludes prefiles. Human edits use their exact
own plugin and existing disconnected read-only behavior. Automatic work may use
the same session's master or assuming controller. Moves use current EKCH
execution/error/reconciliation patterns. Deploy backend/frontend/plugin together
while nobody is online.

PDC and AMAN stay disabled at EKBI/EKYT without an activation date. Preserve
existing EKCH services, including the application-shared PDC client/logon. ENGM,
new-airport PDC/AMAN activation, a rule interpreter, hot reload, arbitrary scripts,
parallel legacy protocols, online migration, distributed election, a general
pending-action framework, and ES flight-plan creation are outside this release.

## Epic overview and dependencies

| ID | Epic | Completion boundary | Prerequisites |
| --- | --- | --- | --- |
| MA-01 | Load combined airport definitions in production | Strict selected-file loading and an immutable typed registry, with EKCH and a fictional test definition. | None |
| MA-02 | Route workflows through coded handlers and preserve EKCH | Production decisions/execution paths support EKCH without changing its operational behavior. | MA-01 |
| MA-03 | Support independent airport memberships over one plugin connection | Scoped transport, per-membership master/sync state, and own-plugin routing work across airports. | MA-01, MA-02 |
| MA-04 | Coordinate airport coverage, takeover, and automatic rejoin | Desired memberships follow configured responsibilities and radio/staffing evidence. | MA-03, MA-05 |
| MA-05 | Isolate airport service state while sharing sources and clients | CDM/stands/text pilot consume explicit airport context; shared acquisition and EKCH services remain intact. | MA-01, MA-02 |
| MA-06 | Bind each frontend tab to an authorized airport session | Discovery, chooser/switcher, own-instance binding, and existing offline behavior work per tab. | MA-03 |
| MA-07 | Deliver the integrated RADIS board for EKBI/EKYT | The agreed prototype feature set plus restoration uses authoritative backend state and real own-plugin writes. | MA-02, MA-04, MA-05, MA-06 |
| MA-08 | Validate and activate the release in an offline window | Cross-application acceptance, operational airport data, and coordinated deployment evidence are complete. | MA-01–MA-07 and each airport's operational inputs |

These are prerequisites for declaring an epic complete, not a ban on earlier
work against agreed contracts. For example, RADIS UI adaptation and airport-data
collection can start while transport work is in progress. Do not enable an
incomplete combination in production.

## Suggested delivery order

1. **Foundation:** MA-01, then MA-02. Capture current EKCH behavior and relevant
   performance baselines before changing the production paths.
2. **Parallel infrastructure:** MA-03 and MA-05 can proceed together after the
   context/handler contracts are available.
3. **Participation and navigation:** MA-04 follows transport plus shared-source
   lifetime support; MA-06 follows transport. These can proceed in parallel.
4. **RADIS integration:** complete MA-07 against those foundations. Track airport
   data and operational review throughout instead of waiting until this stage.
5. **Release acceptance:** close MA-08, then use the existing deployment process
   in a window with nobody online. No online old/new compatibility period.

MA-08's acceptance preparation starts during MA-01/02, not after all development.
Each epic includes its own focused checks; MA-08 supplies the integrated release
evidence rather than deferring all testing to the end. Calendar estimates and
assignees are deliberately not invented here.

## MA-01: Load combined airport definitions in production

**Outcome:** the application resolves one immutable definition per configured
airport from its single combined file. Startup configuration is no longer
equivalent to mutating one current-airport global.

**Scope:** full typed Go loading/validation for the existing combined schema,
explicit deployment file selection, handler/settings registration, resource
resolution, and definition revision. Preserve existing domain configuration
formats and the complete EKCH data migration. Keep the fictional airport as test
data that is never activated by repository scanning.

MA-01 owns definition/settings decoding and registration metadata; MA-02 owns
runtime handler implementations and production execution. Loader tests can
register fictional handler/view descriptors only within their test registry;
they do not require a fictional frontend or enable that airport in production.

**Candidate child issues:**

- **MA-01.1:** implement typed combined-file loading and compose existing domain
  validators; include strict unknown/missing-field handling and useful source errors.
- **MA-01.2:** build the immutable registry and compiled handler/settings factory
  registration; validate known handler/view/bay references without a generic interpreter.
- **MA-01.3:** resolve repository and environment-backed assets consistently,
  including region fallback/override behavior and disabled-service dependencies.
- **MA-01.4:** connect application assembly to the complete EKCH definition and
  migrate current configuration consumers incrementally through explicit context.
- **MA-01.5:** add full-loader/schema parity cases using EKCH and the fictional
  definition; replace reliance on the proof's partial YAML-node projection.

**Acceptance:**

- The actual application loader consumes the combined EKCH file and preserves
  all 20 current YAML settings, 18 bays, master priorities, and relevant constants.
- Two selected definitions coexist without changing each other's resolved data.
  An unknown airport/handler never falls back to EKCH.
- Invalid selected definitions fail startup with airport/field/resource context;
  examples and unselected definitions are not silently activated.
- Region environment overrides/defaults behave as authored; repository paths
  remain constrained, and required selected assets are checked.
- Disabled regional PDC/AMAN do not require deployment data or start work.
  Existing EKCH service enablement/credentials remain deployment-controlled.
- The registry is immutable after startup. Event handling does not parse YAML,
  load files, or search for a handler factory.

**Not included:** hot reload, an admin configuration editor, or guessing real
EKBI/EKYT operating values. Traceability: E1 and the single-file/handler decision.

## MA-02: Route workflows through coded handlers and preserve EKCH

**Outcome:** production workflow decisions depend on the session's bound handler
and typed definition. EKCH's rules remain real tested code, with shared execution
and guard mechanisms rather than scattered airport conditionals.

**Scope:** production handler/action descriptors, pure decisions, scoped context,
and execution through existing database, ordering, EuroScope, and domain-service
operations. The existing sample is reference code, not the final SDK or executor.

**Candidate child issues:**

- **MA-02.1:** define the minimal production hooks for controller actions,
  normalized observations, admission/lifecycle, and coordination; register typed
  supported actions with handlers instead of growing the sample core allowlist.
- **MA-02.2:** put existing EKCH bay/reconciliation, lifecycle, ownership, and
  special coordination behavior behind its handler, including SQ resolution.
  Supply settings/geography from the definition rather than hidden constants.
- **MA-02.3:** adapt existing move/field-edit executors to explicit session,
  definition, and source authority; retain EKCH validation, ordering, errors,
  rollback, and observation reconciliation.
- **MA-02.4:** keep human own-plugin binding separate from genuine automatic
  master/assuming-controller resolution, using existing appropriate helpers.
- **MA-02.5:** exercise EKCH full and incremental observation paths, movement,
  completion/removal, and coordination with production stores or faithful fixtures.
  Retain a fictional custom handler test proving a feature needs no core airport branch.

**Acceptance:**

- EKCH's specific taxi placement, solo-tower promotion, departure progression,
  clearance/ownership checks, handoffs, and removal behavior retain their meaning.
  Test both full sync and incremental updates, not only the two sample hooks.
- The common dispatcher selects a bound implementation; a handler change does
  not require adding an airport switch or every custom action to the common core.
- Controller edits, including work passed through a shared service, retain their
  own binding. Missing/disconnected bindings never fall back to the master.
- Genuine automatic effects resolve an available authorized executor in the
  same session, using master or assuming controller as appropriate.
- Handlers own no database/provider client, polling loop, or mutable current
  airport. Validate decisions before mutation and perform network I/O outside
  session/strip locks and transactions.
- Preserve current concurrency and move/error behavior. No new confirmation
  gate, pending-move UI, outbox, or durable action engine is required.

**Not included:** a declarative rule language or full production equivalence
inferred from the existing sample tests. Traceability: D3, D4, E2, E3.

## MA-03: Support independent airport memberships over one plugin connection

**Outcome:** one authenticated EuroScope instance participates in multiple
sessions, with independent roles, synchronization, and command routing.

**Candidate child issues:**

- **MA-03.1:** update the existing protobuf envelopes/control messages with the
  required instance, membership, session/environment, generation, and master
  context. Upgrade both ends together; do not build a parallel legacy endpoint.
- **MA-03.2:** split backend transport identity from logical membership clients;
  replace global/CID-only role, observer, disconnect, and sender assumptions.
- **MA-03.3:** replace the plugin's singular role/session state with a membership
  map, scoped full-sync inventories, removal tracking, and role state. Collect
  callbacks once and dispatch under the applicable membership roles.
- **MA-03.4:** elect and synchronize masters independently, including stale-source
  fencing at application time and non-master controller command/result handling.
- **MA-03.5:** retain identity/radio/control updates when zero memberships remain;
  expose authorized membership discovery for the frontend and coverage coordinator.

**Acceptance:**

- The same plugin can be master for two airports and non-master for another;
  a role or full-sync change in one cannot reset or delete another's state.
- Human commands route to the exact initiating compatible instance/membership,
  not a CID-wide lookup or whichever plugin is master.
- Reconnect, late messages, old master snapshots, and membership removal cannot
  write through stale or wrong-session contexts.
- Removing one membership retains the transport and unaffected memberships;
  zero-membership control updates remain available without keeping operational
  controller rows or session cleanup artificially alive.
- LIVE, Sweatbox, and playback identities remain isolated. Existing startup
  delay, reconnect, and disconnect behavior are retained.

**Not included:** distributed election, online failover, simultaneous old/new
protocols, or a new generic acknowledgement protocol. Traceability: D4, D5, E3.

## MA-04: Coordinate airport coverage, takeover, and automatic rejoin

**Outcome:** memberships follow responsibilities and radios even when a controller
has no active airport membership. Coverage remains independent of strip ownership.

**Candidate child issues:**

- **MA-04.1:** evaluate enabled definitions against server-authorized eligibility,
  configured responsibilities/providers, primary identity, and tuned frequencies.
- **MA-04.2:** assemble normalized staffing/radio evidence from shared providers,
  including controllers without FlightStrips; extend shared staffing acquisition
  if necessary rather than assume the current pilot/prefile snapshot contains it.
- **MA-04.3:** apply desired-membership deltas and the confirmed bounded takeover
  rule, preserving unaffected memberships and independent mastership.
- **MA-04.4:** handle evidence freshness/unknown states explicitly using existing
  matching behavior as the baseline; test radio changes without callsign changes.
- **MA-04.5:** connect automatic rejoin, zero-membership source demand, and existing
  offline/read-only transitions without creating a new disconnected workflow.

**Acceptance:**

- When only EKBI_TWR is online, an EKDK instance retains configured approach
  participation. APP taking over all remaining responsibilities removes it.
- Relinquish at replacement backend attachment or the bounded configured grace,
  whichever comes first, including an absent/delayed replacement plugin. Respect
  the intentional plugin startup delay; do not repurpose it as the grace period.
- When APP leaves, a still-connected eligible EKDK instance rejoins automatically,
  including from zero memberships, and synchronizes before editing.
- Tuned-frequency changes affect coverage without requiring a callsign change.
  RADIS participation does not require sector/strip ownership.
- No polling loop or full staffing/radio cache is created per membership.
  LIVE evidence does not leak into exercise/playback sessions.

**Activation inputs:** actual airport roles/frequencies/providers/master values,
regions, and grace value. Synthetic fixtures unblock implementation. Traceability:
E4 and the confirmed takeover/rejoin requirements.

## MA-05: Isolate airport service state while sharing sources and clients

**Outcome:** enabling another airport does not duplicate global acquisition or
external clients and does not reuse EKCH's mutable airport settings/state.

**Candidate child issues:**

- **MA-05.1:** preserve shared VATSIM/transceiver sources, indexes, last-good-data,
  and refresh coordination; include aggregate demand from zero-membership LIVE
  control instances alongside existing service/EFB consumers.
- **MA-05.2:** seed CDM from each enabled airport's definition/data and partition
  calculations, schedules, subscriptions, and LIVE-only publication state.
- **MA-05.3:** replace implicit EKCH/global stand registries with airport-specific
  stand/airline/scenery definitions and scoped reservations/assignments/expiry;
  keep read-only aircraft/reference inputs shared where appropriate.
- **MA-05.4:** supply airport-specific text-pilot presets/actions through existing
  transport and own-plugin routing. Adapt weather/ATIS presentation to shared
  backend acquisition instead of browser-local polling.
- **MA-05.5:** preserve application-owned CDM/PDC/ECFMP clients, the shared PDC
  logon/worker, and current EKCH PDC/AMAN. Ensure other airport definitions cannot
  overwrite EKCH validation/configuration. Enforce regional PDC/AMAN disablement
  in server APIs, service registration, polling, and publication paths, not just
  frontend capabilities; skip disabled regional PDC mailboxes even when the
  application-level client remains enabled for EKCH.
- **MA-05.6:** attach/detach scoped work on existing session lifecycle without
  transferring ownership of global clients/workers to memberships or browser tabs.

**Acceptance:**

- Source/client/worker construction and upstream request counts demonstrate
  application ownership while multiple airports, sessions, and tabs are active.
  Distinct upstream endpoints/mailboxes may still require distinct requests.
- CDM and stand state/data do not leak between airports, LIVE, and exercises.
  No unconfigured airport silently loads EKCH rates/stands/presets.
- Source failure/age is shared health; consumers retain their existing scoped
  readiness behavior. One session cleanup cannot clear another's source or work.
- Human text-pilot/service edits retain the own-plugin binding, and genuine
  automatic work uses an eligible same-session executor under D4.
- Existing EKCH PDC/AMAN remain operational. Regional PDC/AMAN are disabled and
  do not demand assets, mailboxes, workers, or new-airport activation work.
  Direct regional requests cannot bypass disablement through the shared service.

**Not included:** additional-airport PDC reply/sequence redesign (future E5),
regional AMAN, or a fork of existing domain services. Traceability: D2, D4,
shared-source/PDC requirements, E1/E4/E6.

## MA-06: Bind each frontend tab to an authorized airport session

**Outcome:** each tab shows and edits its selected authorized board, with clear
own-plugin readiness and no cross-tab or cross-airport store contamination.

**Candidate child issues:**

- **MA-06.1:** replace singular CID/controller association with authorized
  membership/session discovery, scoped observer/readiness/capability data, and
  explicit instance selection only when multiple compatible own instances exist.
- **MA-06.2:** implement one-eligible auto-entry, multiple-eligible initial chooser,
  airport switcher, and independent per-tab selection/store state.
- **MA-06.3:** bind the board WebSocket and initial snapshot to the selected
  session, definition revision, coded view, action catalog, and capabilities.
- **MA-06.4:** reject delayed traffic/results after switching/rebinding and retain
  the selected board read-only through own-plugin loss and normal reconnect.

**Acceptance:**

- One eligible session enters directly; several show a choice; none retains the
  current no-plugin behavior. Discovery never chooses an arbitrary CID row.
- Two tabs can show different airports without one tab changing the other's
  selection. Switching does not change plugin membership or mastership.
- Server authorization and binding checks protect mutations, not only UI hiding.
  Old subscriptions/results cannot mutate a newly selected airport.
- A disconnected own plugin keeps the current board read-only even if another
  master remains connected. Existing observer restrictions apply per membership.

**Not included:** several simultaneous boards in one tab or compatibility with
an old frontend during rollout. Traceability: D5, E3 and navigation requirements.

## MA-07: Deliver the integrated RADIS board for EKBI/EKYT

**Outcome:** adapt [the reviewed PR #958 prototype](radis-prototype-review.md)
into the host application as a shared coded view/handler, with independent
authoritative boards and actual controller-plugin effects.

**Candidate child issues:**

- **MA-07.1:** inventory current prototype features, fields, menus, and dialogs;
  map each implemented interaction to a typed backend action and acceptance case.
  Distinguish working controls from placeholders without inventing new capabilities.
- **MA-07.2:** implement the RADIS handler/settings/schema and authoritative
  bay/order/annotation/lifecycle storage using the required specific migrations.
  Include all four observed containers and ownership-free collaboration.
- **MA-07.3:** adapt board/strip components, menus, runway/SID/route/heading,
  squawk/altitude, VFR/RNAV, stand, notes, de-icing, and REA/ATA controls to host
  stores, styles, backend actions, and existing plugin execution/results.
- **MA-07.4:** implement shared NEW local-strip creation and FIND connected-pilot
  lookup/admission, with stable strip identity and duplicate prevention. Never
  create an ES flight plan or add prefiles/unrelated airport flights.
- **MA-07.5:** implement automatic appearance/removal plus manual removal and
  restoration, shared hidden/recovery state, and refresh/restart reconciliation
  that preserves explicit placement and controller annotations.
- **MA-07.6:** integrate CDM/stand/text-pilot/weather capabilities, finish the
  feature acceptance matrix, and supply concrete typed synthetic fixtures while
  real EKBI/EKYT operating values are being reviewed.

**Acceptance:**

- Every current implemented prototype feature has a production action/test or
  operational acceptance case; notes, de-icing marks, REA/ATA, and restoration
  are included. Explicit NEW/FIND and PDC/AMAN decisions override demo shortcuts.
- Two non-observer participants see the same bay/order/field/annotation/removal
  changes with concurrency checks and reconnect/restart convergence. No ownership
  is required and no airport-local operation directly mirrors to another airport.
- Human bay/field edits use their bound own plugin and current EKCH execution/
  error patterns. All edits, including annotations, NEW, delete, and restore,
  remain blocked under the agreed disconnected read-only policy.
- NEW creates a shared local strip only; ES-dependent actions require a valid
  associated flight context. FIND searches only connected pilots to/from the
  selected airport using the shared source; no prefiles or demo ten-flight cap.
- Manual deletion survives refresh and is restorable according to the configured
  policy. Restore does not duplicate identity or delete/replace the ES flight plan.
- EKBI/EKYT have independent definitions and state while reusing RADIS code.
  There is no standalone second app, browser VATSIM polling, or duplicate stand
  allocator. PDC/AMAN stay disabled.

**Activation inputs:** actual bay/ES and field mappings, appearance/removal values,
recovery limits, timestamp semantics, and service/airport data. Do not copy ENGM's
recovery duration, REA/EOBT assumptions, or prototype thresholds as defaults.
Traceability: D1–D4, E2/E3/E6 and the prototype/reference notes.

## MA-08: Validate and activate the release in an offline window

**Outcome:** demonstrate the complete agreed release across backend, frontend,
plugin, and operational airport definitions, then deploy it using the normal
offline process without disturbing existing EKCH behavior.

**Candidate child issues:**

- **MA-08.1:** establish the release acceptance matrix and capture existing EKCH
  workflow/service behavior and representative load before the refactor; keep
  this evidence current while MA-01–MA-07 progress.
- **MA-08.2:** run production-loader, database, protocol/plugin, browser, and
  service integration scenarios spanning independent airports/environments;
  prove no cross-session routing and no duplicated shared acquisition/client lifetime.
- **MA-08.3:** collect and validate real EKBI/EKYT files, service datasets,
  mappings/thresholds, and named operational acceptance. Keep fictional fixtures
  distinct from selected deployment files.
- **MA-08.4:** compare representative EKCH and concurrent-airport performance,
  locks/queues, shared-source requests, and service behavior. Set practical
  regression/load acceptance from these measurements, not the sample microbenchmarks.
- **MA-08.5:** document and rehearse the existing deployment/migration/smoke process;
  verify nobody is online, update backend/frontend/plugin together, and enable
  only airports whose configuration and acceptance are complete.

**Acceptance:**

- All scoped architecture scenarios pass with the actual production paths,
  including independent sync/master epochs, takeover/return from zero memberships,
  tuned-radio changes, multi-tab switching, non-master own-plugin edits, concurrent
  RADIS work, restoration, and LIVE/exercise/playback isolation.
- Existing EKCH moves, routing, service behavior, observer/offline/reconnect,
  and persisted state retain their operational meaning; actual relevant service
  and plugin regression evidence is attached, not just the sample helper tests.
- Each activated regional airport has approved concrete data and the full RADIS
  feature matrix, with CDM/stands/text pilot available and PDC/AMAN disabled.
- Backend restart/session cleanup and service health preserve scoped state and
  shared resource ownership using existing failure/recovery mechanisms.
- A coordinated no-controllers-online deployment updates the components and
  required data/migrations together. Normal deployment failure/rollback procedures
  are used; no online migration or legacy-client support layer is introduced.
- The release evidence states the measured workload/results and any accepted
  remaining limitations. Disabled or incomplete airport activation is explicit.

**Not included:** making future multi-airport PDC acceptance scenarios or ENGM
activation release blockers. Traceability: D5, E6/E7 and all initial-release
architecture acceptance scenarios.

## Cross-epic decisions and engineering traceability

| Source item | Accountable epic(s) | Planning treatment |
| --- | --- | --- |
| D1 RADIS feature/NEW/FIND/restoration scope | MA-07; MA-08 acceptance | Confirmed scope, not another discovery gate. |
| D2 regional service set and exclusions | MA-05, MA-07, MA-08 | CDM/stands/text pilot; preserve EKCH PDC/AMAN and defer regional activation. |
| D3 existing EKCH move behavior | MA-02, MA-07 | Reuse current execution/errors; no new pending/acknowledgement framework. |
| D4 human versus automatic authority | MA-02, MA-03, MA-05 | Exact human binding; same-session master/assuming controller for genuine automation. |
| D5 coordinated offline upgrade | MA-03, MA-06, MA-08 | One upgraded protocol/frontend/plugin set; no dual-client rollout machinery. |
| E1 full registry/validator/view integration | MA-01 | Complete production loading; the Python validator and proof remain evidence, not substitutes. |
| E2 handler/action/effect contract | MA-02; RADIS implementation MA-07 | Typed registration and pure decisions; shared executor owns I/O. |
| E3 identity, context, precedence, concurrency | MA-02, MA-03, MA-06, MA-07 | Reuse existing store/concurrency with the necessary scoped identities. |
| E4 evidence and zero-membership demand | MA-04, MA-05 | Shared evidence, scoped decisions, existing disconnect behavior. |
| E5 additional-airport PDC mailbox correlation | Deferred future work | Not a regional release epic or a blocker; retain current EKCH behavior. |
| E6 state/config changes and disablement | MA-01/02/07 migrations; MA-08 release | Specific offline migrations and existing cleanup; no generic migration engine. |
| E7 performance and observability | Each epic; MA-08 integrated comparison | Collect baselines early and validate representative concurrent use. |

## Airport inputs and release gates

| Input/gate | Prepared during | Blocks |
| --- | --- | --- |
| Coverage roles, positions/radios, provider/fallback rules, master priorities, geography/regions, grace value | MA-01/04 with airport representatives | Real airport participation activation, not implementation with fixtures. |
| Bay/field ES mappings, local-flight treatment, annotation/timestamp meaning, lifecycle/removal/restoration values | MA-02/07 | Affected operational action acceptance. |
| CDM rates/taxi/SID/de-icing data, stands/airline/scenery data, text-pilot phrases/views | MA-05/07 | Readiness of that regional airport/service pair. |
| Named reviewers, controlled scenario sign-off, validated selected files | Throughout; final evidence MA-08 | Activation of that airport. |
| No controllers online and matched backend/frontend/plugin release | MA-08 using existing deployment process | Deployment execution. |

If one airport's operational data is not ready, its activation remains disabled;
that does not justify inventing values or deploying the fictional example.
Shared infrastructure can be validated before either real regional definition
is activated. There is no calendar commitment or implied PDC/AMAN follow-on date.
The shared technical release and each airport's activation can finish separately;
an outstanding airport activation task remains open until its own gates pass.

## Definition of done for the planned release

All eight epics meet their scoped acceptance, both EKBI and EKYT meet their data/
operational gates and are activated, existing EKCH behavior has integrated
regression evidence, and the coordinated offline release succeeds. If only one
regional airport is ready, record the technical release and that activation
without marking the other airport's delivery complete. Source documents, combined config,
handler action/settings contracts, and operator/developer guidance describe the
delivered behavior. The existing isolated proof and Astra's architecture verdict
support the plan but do not replace this implementation evidence.
