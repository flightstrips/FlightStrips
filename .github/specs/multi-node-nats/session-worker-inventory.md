# Session worker inventory

Task 18's NATS candidate is dormant until the coordinated runtime cutover. The
SQL application's workers in `backend/internal/app/app.go` continue to run only
for the SQL runtime. In the candidate, `SessionWork` scans active session
registries, requires a ready projection and the session owner lease, and routes
mutations through typed commands and the subject-CAS writer. A newly elected
owner rebuilds due work from persisted entities rather than local timers.

| Session work in the SQL runtime | NATS candidate authority | Cutover status |
| --- | --- | --- |
| `server.StartSessionMonitor` | Session owner: persisted `first_no_controller_at` and `cleanup_paused_at`, global EuroScope presence, five healthy minutes after the last operational client disconnects | Candidate implemented and tested; SQL monitor stays in SQL runtime |
| `pdc.Service.Start` response timeout | Session owner: `pdc.Candidate.PDC` consumes `pdc-response` with sequence revision/correlation and creates typed no-response intent/effect | Task 18a implemented and tested; Task 20 binds callback |
| `pdc.Service.Start` external polling | Session owner: `pdc.NewCandidate(...).PDC`, durable `pdc-poll` slots, parsed checkpoint and fenced outbound intents | Task 18a implemented and tested; no loop starts before Task 20 |
| Stand assignment and block expiry, including departure lifecycle | Session owner: projection sweep, fresh FS_POSITIONS occupancy check, source revision and CAS | Concrete lifecycle hooks own stand deadlines when bound; generic expiry remains the fallback |
| `DepartureLifecycleService.StartSweep` and `ArrivalLifecycleService.StartSweep` | Session owner via `services.NewVatsimLifecycleCandidate(...).Departure` and `.Arrival`, assigned to `SessionWork` | Task 19a candidate implemented and tested; startup binding remains Task 20 |
| VATSIM reconciler's session mutations | Same concrete candidate consumes the global typed checkpoint, reconciles strips, then commits real lifecycle policy with source/entity/position rechecks | Task 19a candidate implemented and tested; no additional provider poller |
| `cdm.SyncService.Start` session sync, periodic recalculation, CTOT validation and debounce | `services.NewCdmCandidate(...).CDM` bound to `SessionWork.CDM`; accepted `cdm-sync`, `cdm-recalculate`, `cdm-validation`, `cdm-debounce` and callsign `cdm-pushback` deadlines | Task 18b concrete candidate; bind only in Task 20 and release only in Task 24 |
| `TrafficMetricsService.Start` session metrics writes | `services.NewTrafficCandidate(writer).Traffic` bound to `SessionWork.Traffic`; projection read and lease/readiness recheck immediately before gauge publication | Task 18b concrete candidate; no traffic domain event or storage schema |
| EuroScope controller offline grace timer | Session owner: `controller-offline` deadline, controller source revision and fresh cluster client presence | Candidate outcome implemented and tested; socket scheduling adapter required |
| EuroScope aircraft disconnect timer | Session owner: `aircraft-disconnect` deadline bound to FS_POSITIONS tombstone revision; retained-aircraft decision from shared observations | Candidate outcome implemented and tested with injected retention decision; socket scheduling and shared retention adapters required |
| EuroScope session update debounce | Session owner: `session-update` deadline and `SessionWork.SessionUpdate` hook | Recalculation adapter and scheduling required |
| EuroScope session disconnect work | Session owner: `session-disconnect` deadline and `SessionWork.SessionDisconnect` hook | Recalculation adapter and scheduling required |
| Strip STAND bay auto-hide | Session owner: `strip-auto-hide` deadline persisted when entering STAND and rearmed with strip source revision | Candidate implemented and tested |
| Landing-clearance delayed validation | Session owner if enabled | Existing SQL feature is disabled; NATS adapter required before enabling |
| EuroScope squawk throttle | Session owner if it sends an authoritative plugin action | Adapter required; cannot be retained as a local outcome timer |

The following timers may remain node-local because their loss cannot decide a
session outcome: EuroScope position-only send coalescing, delayed informational
broadcasts after a committed offline/session update, VATSIM/transceiver and
navigation observation refresh, and frontend transport delivery. The SQL stand
allocation retry timer belongs only to the SQL implementation. AMAN, ECFMP,
METAR, ALB and global configuration refresh workers are outside the session
aggregate; their session-derived writes must still enter through the owner at
cutover. No legacy SQL worker should be started alongside its NATS replacement.

Task 19a tests in `backend/internal/services/vatsim_lifecycle_*integration_test.go`
use two independent replicas against the pinned three-node NATS fixture and
the exported constructor's actual callbacks. They cover nonowner rejection,
replay, failure before/after commit, snapshot restart, reservation/block/arrival
deadline recovery, missing-flight cancellation, shared position barriers,
position-only EuroScope departures, PUSH release, plan/CID changes, wrong-stand
warnings and immutable controller-target STAND effects. The candidate uses the
existing departure/arrival and SAT policy through isolated planning ports;
neither callback constructs a SQL repository or transaction.

Task 18a's `backend/internal/pdc/candidate_integration_test.go` runs two
independent backend connections, projections and owner runtimes against the
pinned NATS fixture. A real HTTP Hoppie fixture exercises the production client,
parser, request validation, mandatory-route review and clearance builders. It
covers one poll per slot, accepted-page recovery, owner death before/after
intent/result commits, uncertain sends, WILCO/UNABLE correlation, stale/late
responses, ten-minute timeout takeover, snapshot/full restart, pending route/SID
effects, browser remark composition and Web HTTP JSON routes with Hoppie
disabled. No fake policy supplies outcomes. Task 20 installs
`Candidate.Bind(sessionPlanner)` for controller/browser and HTTP commands,
sets `SessionWork.PDC = candidate.PDC`, and supplies the owner router to
`NewCandidateWebAPI`. The callback starts no timer or SQL service.
The PDC constructor's final arguments accept `TransceiverLookup` readers;
Task 20 injects `cluster.NewTransceiverSource` from Task 19c. A real provider
HTTP fixture, accepted global checkpoint and both readers verify that a fresh
controller's secondary radio enters production clearance composition.
Task 18b also exports `services.NewCdmActionService(ownerStore)` and
`CdmCandidate.Planner(next)` for the existing typed browser and EFB HTTP
commands, `services.NewCdmCandidateWebAPI(auth, candidate)` for the unchanged
JSON `/cdm/sequence` response, `CdmCandidate.Recalculate` for session update/
disconnect callbacks, and `CdmCandidate.ReconcileMaster` for airport-owned
vIFF master reconciliation. Pushback callers submit `prepare_pushback` and
read `CdmActionService.PushbackResult`; its acknowledgement comes from the
persisted probe result, rather than a local retry count.

No Task 18b constructor starts a goroutine or timer. The existing SessionWork
scan timer merely discovers persisted work. HTTP/UI may coalesce publication
or wait locally for a committed pushback result; losing that read-only wait
cannot acknowledge, cancel or rearm work. Traffic publication may be sampled
by the calling worker's local tick because sampling has no domain effect.
Task 19's airport provider refresh owns configuration/master observations;
CDM polls vIFF flight pages only through its persisted session/probe slots.
SQL CDM debounce, periodic sync, detached export and traffic timers stay solely
in the SQL runtime. See [Task 18b acceptance](tasks/18b-cdm-traffic-acceptance.md).
