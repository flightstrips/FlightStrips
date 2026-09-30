# Task 20a runtime assembly and binding matrix

`app.BuildNATS(ctx, Config, Dependencies)` is the independently selectable NATS
constructor. It requires administrator-created resources and an explicit effect
key ring. It never opens PostgreSQL, seeds SQL, bootstraps broker resources, or
starts the SQL constructor's workers. The server entrypoint still selects the SQL
constructor until Task 20c; the tests invoke the exported NATS constructor directly.

The transport extensions contain HTTP clients and provider URLs, not policy
decisions or persistence mocks. Production authentication remains the existing
OIDC service. The integration fixture replaces authentication with deterministic
test credentials and uses real binary WebSockets, HTTP providers, NATS connections,
JetStream resources, projections, leases, planners, policy services and effects.

## Acceptance scenarios

These tests live in `backend/internal/app/nats_*integration_test.go`:

| Test | Assembly evidence |
| --- | --- |
| `TestBuildNATSTwoApplicationsBinaryHTTPAndTakeover` | Two constructors without a database; revision-2 frontend and EuroScope sockets on different nodes; sync, frontend mutation, accepted outcomes and HTTP reads; session owner death, new epoch, continuing writes and rejection by the closed node |
| `TestBuildNATSAIRACWindAndAMANPolicy` | Actual AIRAC.NET envelope/parser/materializer and terminal geometry; actual Open-Meteo GFS parsing and global quota; authoritative AMAN surveillance and JSON detail; FMP rate policy, coordination submission/acceptance and authenticated speed-fact correlation |
| `TestBuildNATSDeadlineAndSquawkEffects` | Shared position/sync admission; automatic squawk recovery, accepted queue, binary plugin dispatch/result on both replicas, assigned squawk observation, manual IFR/VFR field rendering/results and persisted controller offline grace |
| `TestBuildNATSProductionProvidersCDMSATAndPDC` | Actual VATSIM status/feed, transceivers, ATIS/METAR, ECFMP, vIFF configuration/master/CDM HTTP adapters; LIVE departure SAT policy; binary TOBT; production Web clearance and Hoppie polling/request/send; PDC/EFB JSON reads; duplicate global quota refusal |
| `TestBuildNATSReadinessResourceDriftAndFailedConstruction` | Both nodes reject operational admissions after stream drift, retain liveness, verify recovery; bad resource construction and bad effect-key construction unwind replay consumers |
| `TestBuildNATSReadinessProjectionStallAndRecovery` | Three real TCP broker proxies suspend server delivery; readiness and operational admissions fail while liveness stays available; verified catch-up restores both nodes without terminal worker failure |
| `TestBuildNATSReadinessQuorumLossAndRecovery` | Stop two explicitly label-verified fixture brokers; fail readiness/admission on both nodes, retain liveness, restore quorum and catch up; shutdown is repeatable and rejects subsequent writes |

The broker fault tests require the disposable Compose project `fs20a`; they
verify its labels before stopping containers. Tests create resources only through
the administrator fixture connection. Test sessions are tombstoned/finalized
through the actual registry before closing applications, including the LIVE name.

## HTTP bindings

All first-party API paths below retain `/api` as their outer prefix. Existing
authentication/method handling and JSON boundary models stay in their concrete
handlers. Typed accepted state supplies the service ports. Command IDs and
durable results use `httpresults`, including the existing Idempotency-Key rules.

| Route | Concrete runtime binding | Acceptance |
| --- | --- | --- |
| `/frontEndEvents` | `frontendbinary.Handler`, projection snapshots/deltas/outcomes/presence and `natsRuntime.Route` | two-app test; existing frontendbinary handler/dispatcher tests |
| `/euroscopeEvents` | `euroscopebinary.Handler`, registry, controller sectors, shared position dispatcher, deadline admission and `EffectRenderer` | two-app, squawk/deadline, provider and AMAN tests; Task 18c evidence |
| `/healthz` | Process liveness with accepted SAT and complete AMAN component health | quorum/drift/stall tests; existing SAT health tests |
| `/readyz` | `OwnerRuntime.Ready` and projection/resource verification | constructor, quorum/drift/stall and takeover tests |
| `/metrics` | Accepted projection/lease/effect metrics, measured FS_STATE PubAck latency and verified resource sizes | same two-node runtime; `cluster.Projection.WriteMetrics` |
| `/stand/status`, `/stand/preview` | Existing `standstatus.NewWebAPI`; accepted sessions/strips/assignments/VATSIM and `services.StandReadCandidate` using real allocation presentation policy | provider/SAT assembly; existing standstatus and allocation policy tests; Task 19a evidence |
| `/cdm/sequence` | `services.NewCdmCandidateWebAPI`, accepted calculated sequence and configuration | binary CDM/provider assembly; Task 18b acceptance and CDM candidate tests |
| `/ecfmp/measures` | `ecfmp.AcceptedMeasures` and accepted normal/test provider pages | provider assembly; ECFMP candidate tests |
| `/ecfmp/test/inject`, `/ecfmp/test/clear` | HTTP JSON conversion to typed page/object; global owner command and accepted checkpoint | ECFMP HTTP/candidate contract; same runtime global router |
| `/aman/airports/{airport}/flights/{callsign}/detail` | Existing AMAN WebAPI with `amancandidate.Worker` accepted board and geometry/route ports | AIRAC/wind/AMAN test; existing AMAN WebAPI tests |
| `/pilot/me`, `/pilot/flight` | Existing pilot WebAPI; accepted global VATSIM identity and accepted session/strip views | two-app and provider tests; pilot WebAPI tests |
| `/pdc/request`, `/pdc/status`, `/pdc/acknowledge`, `/pdc/unable` | `pdc.NewCandidateWebAPI`; `pdc.NewCandidate` bound to owner router, accepted strip/sequence and outcome reads | provider clearance/poll/send assembly; `TestNATSPdcHTTPJSONThroughProductionPolicy` and Task 18a evidence |
| `/efb/me`, `/efb/flight`, `/efb/tobt`, `/efb/stand`, `/efb/stands` | `efb.NewCandidateWebAPI`; accepted pilot identity/flight/configuration/ATIS, CDM/stand commands, accepted stand availability and real airborne-frequency selection policy | provider assembly; EFB candidate/WebAPI tests; Task 18b/19a evidence |
| `/gsx/stand` | Existing `gsx.NewWebAPI`, accepted sessions/strips and existing configured sceneries | GSX WebAPI tests and concrete constructor binding |
| Command outcome query | `httpresults.Query` with existing authentication and projection outcomes | two-app durable outcomes; existing HTTP outcome tests |
| `/albEvents` | Existing `alb.NewHub`, `Run` and `Upgrade`, gated by the same flag | ALB remains unchanged and outside this task |

The SAT scenario/block/replay test-tools routes are explicitly unavailable in
BuildNATS: `EnableTestTools=true` returns a construction error. They are already
disabled in `docker-compose.prod.yml` (`ENABLE_TEST_TOOLS: "false"`). This is the
allowed disabled-production exception, not unconverted enabled behavior to hand
to Task 20c. Landing-clearance delayed validation also remains disabled, as stated
in the worker inventory. ECFMP's existing test injection routes are separately
bound above; they are not the SQL scenario/replay test-tools service.

## Planner/action bindings

All frontend commands enter the authenticated binary handler and the owner router.
The complete chain is master election -> EuroScope deadline admission -> private
message -> PDC -> CDM -> session lifecycle/AMAN destination -> validation/manual
flight plan -> stand -> strip/coordination/tactical/session planners. Airport
commands route to real AMAN policy; global commands route to registry, workflows,
provider quota or ECFMP checkpoint policy. No no-op business planner is installed.

| Action family (all oneof cases) | Concrete planner/source | Acceptance |
| --- | --- | --- |
| Strip heading, squawk, requested/cleared altitude, bay, release point, marked, runway cleared/confirmed, start requested, order, text, move, data update, missed approach | `cluster.PlanStrip`, existing accepted revision/ownership and side effects | two-app frontend mutation; cluster strip tests and Task 18 evidence |
| Generate squawk | `NewDeadlineCandidate.Planner` and `RequestSquawk`, accepted queue/throttle, effects and plugin results | assembled squawk test; `TestSquawkBinaryQueueRateResultsCancellationAndTakeover` |
| Coordination transfer, assume, force assume, free, cancel, tag, accept tag | Existing cluster coordination planner composed through `PlanStrip` | cluster coordination tests and Task 18/18c evidence |
| Tactical create, delete, confirm, force assume, mark, start timer, move | Existing cluster tactical planner composed through `PlanStrip` | cluster tactical tests and Task 18 evidence |
| Stand occupy, vacate, automatic, manual, confirm override, acknowledge, create/remove block | `StandState.PlanStand`, existing SAT allocator and typed lifecycle state | provider/SAT test, Task 19a real two-replica lifecycle evidence |
| CDM TOBT, ready, deice, CTOT set/remove, EOBT, ASRT, TSAC, clearance TOBT, ground state, AOBT, ATOT, prepare pushback, better TOBT, logon EOBT | `NewCdmActionService` through owner store and `NewCdmCandidate.Planner`; actual CDM policy | assembled binary TOBT and provider state; Task 18b acceptance and operational/pushback/uncertain-write tests |
| PDC issue, revert to voice, acknowledge, unable | `NewCandidate.Bind` and production request/clearance policy | assembled Web/Hoppie policy; Task 18a HTTP/CPDLC/deadline evidence |
| Private message | `PrivateMessagePlanner`, object-backed encrypted secret and final socket-only decryption | Task 17 private-message/effect evidence and real effect renderer binding |
| Session controller position/layout and runway updates | Existing controller/sector/session planner and deadline reconciliation | two-app binary login/sync; Task 18c evidence |
| Validation acknowledge, unexpected-change acknowledge, CLX override | `PlanStripCommands`, accepted strip activation/revision/position guards | existing validation policy tests; concrete runtime chain |
| Validation TOBT and strip data including EOBT | `NewCdmActionService` / `CdmCandidate.Planner`, including mixed data edits | Task 18b strip/action tests and accepted configuration binding |
| Manual IFR/VFR flight-plan creation | `PlanStripCommands`, named typed CreateFlightPlanEffect and actual EuroScope CreateFPL renderer | assembled real plugin socket fields and durable results on both nodes; compiled protocol/native plugin contracts |

AMAN's 30 `AmanAction.change` cases are all bound in
`amancandidate/commands.go` and `coordination.go`, using a detached evaluator of
the existing production repository interfaces. Its result is converted into one
typed airport CAS event; the evaluator is not another persistence store.

| AMAN cases | Actual production policy |
| --- | --- |
| move, place at time, lock, unlock, desequence, resume, remove | `sequence.ActionService` / coordinator |
| accept TETA, keep FPL ETA, reset TETA override, recompute | `sequence.ActionService` / operational service |
| set manual ETA, set/reset manual feeder ETA, change runway | `sequence.ActionService` / operational service |
| set rate, select runway group, set active runway groups | `sequence.ActionService` / coordinator |
| report, confirm, reject go-around | `sequence.ActionService` / operational service |
| create/remove gap, runway closure, capacity reservation | `sequence.ActionService` / coordinator |
| submit, accept, reject coordination request | `coordinationrequest.Service`, accepted tracking-controller recipient, complete typed metadata/history/decision/clearance/expiry audit |

Existing sequence/operational/coordination tests cover the production policy;
the assembled AMAN test covers airport routing, real geometry/weather, FMP
revision checks and the coordination -> authenticated speed fact correlation.
`routefact.Service` handles direct-to and speed facts using the actual accepted
strip tracking controller and active geometry. Direct-to reevaluation happens
inside the final accepted transition, and audits remain typed. Automatic pending
coordination recipient transfer and expiry are preserved in the evaluator.

Every EuroScope inbound event is bound: login/token/hello and result recording
stay in the handler; full sync, disconnect, positions, assigned squawk and
controller changes use the deadline candidate; scalar squawk/altitude/heading/
communication/ground state/clearance/route/remarks/aircraft/SID/runway/hold/tracking
use its operational adapter; CDM, PDC, private-message and coordination events
use `natsRuntime.inbound`; AMAN direct-to/speed use `inboundAMAN`. Master-only
telemetry and operational controller actions retain separate authority checks.

## Workers and authority

| Work | Concrete binding and durable authority | Acceptance |
| --- | --- | --- |
| Replay, snapshots, KV observations, resource verification | `Projection.Run`; consumer/snapshot/observation goroutines joined | drift/stall/construction tests; existing projection/snapshot recovery evidence |
| Leases, router and registry recovery | `OwnerRuntime.Run`, `CommandRouter.Serve`, registry/discovery supervisor | takeover and quorum tests |
| Effect claims/results/GC | `Effects.Run`, shared fanout, encrypted object secrets, joined result subscription | assembled squawk/results; Task 17/18a/18c evidence |
| Master/EuroScope recovery and controller/aircraft expiry | `NewDeadlineCandidate.BindWorker`; actual master election, shared `Positions`, persisted deadline/source revisions | assembled squawk/offline test; Task 18c failure and takeover evidence |
| Session update/disconnect | Actual deadline sector/layout/route callbacks plus `CdmCandidate.Recalculate`; independent callback errors are accumulated | Task 18b/18c evidence and two-app sync |
| Session cleanup and generic stand expiry | `SessionWork`, persisted healthy-time cleanup and accepted due entities | Task 18 worker/cleanup tests; fixture lifecycle finalization |
| PDC | `NewCandidate.PDC` on LIVE sessions, preserving production airport-inbox scope; durable polls, response deadlines, sends/plugin effects | provider/Hoppie assembly and Task 18a evidence |
| Departure/arrival SAT and VATSIM session mutations | `NewVatsimLifecycleCandidate`, LIVE-only, configured durations/prefiles/message flag; shared deadline `Positions` dispatcher | provider/SAT assembly; Task 19a evidence |
| CDM | `NewCdmCandidate.CDM`; sync, recalculation, validation, debounce, pushback and export deadlines; `ReconcileMaster` from airport supervisor | provider/binary CDM assembly; Task 18b acceptance |
| Traffic | `NewTrafficCandidate.Traffic` when enabled; owner-gated metrics sampling | constructor binding; `TestTrafficCandidateTwoReplicaNATS` |
| VATSIM and transceivers | Real providers -> global typed checkpoints; `NewTransceiverSource` feeds PDC frequency and sector reconciliation | provider assembly and Task 19c evidence |
| CDM config/vIFF master, METAR, ATIS, ECFMP | Actual boundary parsers -> owner-fenced typed provider pages and accepted session applications | provider assembly; Task 19 provider adapter evidence |
| AIRAC and AMAN | AIRAC.NET materializer -> verified objects/manifest; `amancandidate.New`, accepted surveillance, operational service, wind quota and holding-EAT destination workflows | AIRAC/wind/AMAN assembly; Task 19b evidence |
| Global provider quota | Fresh global-owner CAS, minute/hour/day typed counters and bounded node hop; uncertainty/replay never authorizes another HTTP call | assembled wind/provider tests |

Local ticks only discover accepted due work, renew leases, refresh observations,
sample metrics or deliver transport frames. They do not replace persisted
deadlines, provider intent identity, shared quota, positions or takeover state.
Global/airport external intent recovery runs on a new accepted owner epoch.
Session supervisors and callbacks recheck owner/readiness; HTTP and socket
admissions fail before entering business handlers when the runtime is unavailable.
Deletion waits for terminal effects with retryable admission rather than retaining
a failed deterministic tombstone outcome; lifecycle adapters reject committed
failed outcomes. The registry regression test preserves the same deletion UUID
across that barrier. Session-opening admission allows 30 seconds for prior owner
lease/presence expiry and the seed/active workflow; authentication reads remain
bounded to 10 seconds.

Shutdown closes admissions/cancels admitted requests and joins sockets, drains
position barriers while projection/fencing are still live, cancels and joins all
supervisors/replay/leases/router/effect/subscription jobs, then drains/closes NATS.
The same unwind handles partial construction. ALB construction stays unchanged
under its explicit out-of-scope contract.

Metrics include projection seconds/messages behind, takeovers, stale epochs,
snapshot verification failures, lease seconds by bounded aggregate kind, effect
counts by bounded state/type, storage bytes and FS_STATE PubAck latency. Accepted
event logs contain only command ID, stream sequence and owner/master epochs.
Runtime/provider callback logs omit error bodies because transport errors can
contain credential-bearing URLs; no event payload, token or private body is logged.

## Validation and Task 20c handoff

Validation commands (Windows PowerShell; Docker Desktop Linux engine):

```powershell
$env:DOCKER_HOST='npipe:////./pipe/dockerDesktopLinuxEngine'
go test ./... # from backend; includes existing PostgreSQL E2E testcontainers
go build ./...
$env:NATS_INTEGRATION='1'
$env:NATS_TEST_PORT_BASE='5322'
$env:NATS_FAULT_PROJECT='fs20a'
go test ./internal/app -run '^TestBuildNATS' -count=1 -timeout=10m
# from repository root
python scripts/cluster_proto.py --check
python scripts/check_cluster_contract.py
# from frontend
npm run build
npm test -- --reporter=dot
# native Win32 Release build
cmake --build <isolated-build> --target cluster_proto_contract FlightStripsCoreTests --config Release --parallel 4
ctest --test-dir <isolated-build> -C Release --output-on-failure
```

Run the provider/AMAN assembly suite against a fresh disposable fixture: accepted
provider checkpoints intentionally suppress repeated HTTP fetches in an existing
slot, so prior fixture data cannot be used to assert new parser/network calls.
The test uses ports 5322–5324 to avoid other local fixture projects.

The frozen protocol verification checks 300 oneof cases and 291 optional-zero
fixtures. Frontend validation covers 66 files / 544 tests. Native CTest covers
440 tests. The first backend suite attempt lacked Docker Desktop's Linux engine
environment; rerunning with the explicit pipe passed, including existing SQL E2E.

Task 20c owns only final default selection and SQL deletion/configuration:

1. Replace the server's `app.Build` call with `app.BuildNATS` and load the explicit
   NATS resource configuration/effect key paths from deployment configuration.
   Keep `main.buildVersion` and existing component/development version behavior.
2. Switch default Compose/startup dependency ordering to separately bootstrapped,
   verified NATS resources; remove PostgreSQL/migrator startup dependencies.
3. Remove the isolated SQL constructor, repositories, migrations/seeding and
   worker assembly after proving the NATS constructor is the sole process path.
4. Remove test-only constructor-selection scaffolding if any is added by 20c.
   Keep production-disabled scenario/replay and landing validation disabled.

No enabled HTTP business handler, socket action or worker conversion is deferred
to 20c. This change does not edit release workflows, change ALB, replace build
identity, perform default Compose conversion, publish or deploy a candidate.
