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
| `pdc.Service.Start` response timeout | Session owner: `pdc-response` deadline with PDC sequence source revision | Deadline outcome implemented and tested; external polling adapter still required |
| `pdc.Service.Start` external polling | Session owner via `SessionWork.PDC` and owner-routed commands | Adapter required |
| Stand assignment and block expiry, including departure lifecycle | Session owner: projection sweep, fresh FS_POSITIONS occupancy check, source revision and CAS | Concrete lifecycle hooks own stand deadlines when bound; generic expiry remains the fallback |
| `DepartureLifecycleService.StartSweep` and `ArrivalLifecycleService.StartSweep` | Session owner via `services.NewVatsimLifecycleCandidate(...).Departure` and `.Arrival`, assigned to `SessionWork` | Task 19a candidate implemented and tested; startup binding remains Task 20 |
| VATSIM reconciler's session mutations | Same concrete candidate consumes the global typed checkpoint, reconciles strips, then commits real lifecycle policy with source/entity/position rechecks | Task 19a candidate implemented and tested; no additional provider poller |
| `cdm.SyncService.Start` session sync, periodic recalculation, CTOT validation and debounce | Session owner via `SessionWork.CDM` and owner-routed commands | Domain adapter required |
| `TrafficMetricsService.Start` session metrics writes | Session owner via `SessionWork.Traffic` and owner-routed commands | Domain adapter required |
| EuroScope controller offline grace timer | Session owner: `controller-offline` deadline, controller source revision and fresh cluster client presence | Task 18c actual binary admission, shared recovery and expiry implemented; Task 20 binds runtime |
| EuroScope aircraft disconnect timer | Session owner: `aircraft-disconnect` deadline bound to FS_POSITIONS tombstone revision; retained-aircraft decision from shared observations | Task 18c actual binary admission and concrete accepted VATSIM/lifecycle/position retention implemented; Task 20 binds runtime |
| EuroScope session update debounce | Session owner: `session-update` deadline and `SessionWork.SessionUpdate` hook | Task 18c concrete sector/layout/route callbacks and durable scheduling implemented; Task 20 binds runtime |
| EuroScope session disconnect work | Session owner: `session-disconnect` deadline and `SessionWork.SessionDisconnect` hook | Task 18c concrete sector/layout/route callbacks and durable scheduling implemented; Task 20 binds runtime |
| Strip STAND bay auto-hide | Session owner: `strip-auto-hide` deadline persisted when entering STAND and rearmed with strip source revision | Candidate implemented and tested |
| Landing-clearance delayed validation | Session owner if enabled | Existing SQL feature is disabled; NATS adapter required before enabling |
| EuroScope squawk throttle | Session owner if it sends an authoritative plugin action | Task 18c typed WAITING effect queue and atomic server-time throttle implemented; Task 20 binds runtime |

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

Task 18c uses `euroscopebinary.NewDeadlineCandidate` with the real command
router, accepted provider Object Store source and next domain planner. Bind its
`Planner` to the owner writer, run `Serve`, set `Handler.Inbound` and
`Handler.Deadlines`, and call `BindWorker` for recovery, session update,
session disconnect and aircraft position-barrier commits. These dependencies
start no runtime automatically. `NextInbound` composes other binary domain
adapters; the CDM adapter remains the owner of EOBT/ELDT admission.

Defaults remain controller offline 15 seconds, aircraft disconnect 60 seconds,
master transfer 45 seconds and session debounce 300 milliseconds. Controller
changes/debounce/master rearm commit together. Socket liveness and accepted
position tombstones are recoverable inputs to the owner sweep, so node death
between observations and FS_STATE scheduling cannot lose authoritative work.
Missing/stale accepted VATSIM generations or position tags prevent deletion.
Protected occupancy, controller edits and coordination retain strips.

Native squawk radar-target waiting is a delivery-only queue: a revision-2
claimed request is removed before one TopSky invocation, records the original
UUID/session/owner/master terms in the result outbox and expires after 30
seconds. The legacy untracked queue retains its two-minute timeout. Neither
queue grants another claim or decides a session outcome. The backend's Task 17
UNKNOWN rule remains authoritative. Landing-clearance delayed validation stays
disabled; ALB and legacy SQL throttling remain unchanged.

See [Task 18c evidence](tasks/18c-euroscope-deadlines-evidence.md) for the actual
two-replica NATS/socket scenarios and build checks.

Task 20 also binds `VatsimLifecycleCandidate.Positions = deadlineCandidate.Positions`
so Task 19a lifecycle commands share the actual binary position dispatcher.
A closed or unsynced generation cannot grant a lifecycle barrier; writer close
waits for any in-progress expiry/lifecycle commit before replacing it.
