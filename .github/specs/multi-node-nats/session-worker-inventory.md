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
| Stand assignment and block expiry, including departure lifecycle | Session owner: projection sweep, fresh FS_POSITIONS occupancy check, source revision and CAS | Candidate implemented and tested |
| `DepartureLifecycleService.StartSweep` and `ArrivalLifecycleService.StartSweep` | Session owner via `SessionWork.Departure` and `SessionWork.Arrival` | Domain adapters required |
| VATSIM reconciler's session mutations | Session owner via the departure/arrival hooks and owner-routed commands | Domain adapter required |
| `cdm.SyncService.Start` session sync, periodic recalculation, CTOT validation and debounce | Session owner via `SessionWork.CDM` and owner-routed commands | Domain adapter required |
| `TrafficMetricsService.Start` session metrics writes | Session owner via `SessionWork.Traffic` and owner-routed commands | Domain adapter required |
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
