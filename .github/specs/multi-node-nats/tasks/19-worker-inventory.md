# Task 19 worker inventory

The current `app.Build` path in `backend/internal/app/app.go` is the PostgreSQL
production runtime. It starts the following workers in `StartWorkers`. The
candidate NATS worker path must remain opt in until task 20 replaces this
wiring; starting both paths would duplicate provider calls and mutations.

| Startup worker | Authority for candidate runtime | External call or mutation |
| --- | --- | --- |
| `cdmService.Start` | session owner for per-flight state; airport owner for shared airport policy | vIFF reads/writes and CDM recalculation |
| `navigationSource.Start` | airport owner | AIRAC.net import, checkpoint, manifest switch |
| `fsServer.StartSessionMonitor` | session owner | session expiry and cleanup |
| `frontendHub.Run` | each node | local WebSocket delivery only |
| `euroscopeHub.Run` | each node for sockets; session owner for deadlines | socket delivery, disconnect and offline timers |
| `configStore.Start` | airport owner | CDM configuration provider refresh |
| `pdcService.Start` | session owner | PDC deadlines and Hoppie outcomes |
| `vatsimGraph.cache.StartForLiveSessions` | global owner for provider fetch; airport/session owner for derived commands | VATSIM HTTP fetch |
| `vatsimReconciler.Start` | session owner: `services.NewVatsimLifecycleCandidate` concrete Departure/Arrival callbacks | typed generation, stand, strip and arrival reconciliation; no provider fetch |
| `amanObservationWorker.Run` | airport owner | typed VATSIM observations |
| `departureLifecycle.StartSweep` | session owner: candidate Departure callback | persisted reservations, physical blocks, wrong-stand episodes and retention |
| `arrivalLifecycle.StartSweep` | session owner: candidate Arrival callback | persisted arrival retention, cancellations, stand-block expiry and unsafe reservation reconciliation |
| `transceiverCache.Start` | global owner for provider fetch | VATSIM transceiver fetch, local frequency projection |
| `ecfmpService.Start` | global owner for fetch; session owner for application | ECFMP HTTP fetch and per-flight restrictions |
| `albHub.Run` | outside this project | ALB remains unchanged |
| `metarPoller.Start` | airport owner for METAR fetch; global owner for the provider-wide AFV ATIS feed; session owner for ATIS presentation | METAR and AFV ATIS fetch |
| `trafficMetrics.Start` | each node if diagnostic only | metrics; no domain write |
| `amanRuntime.Start` | airport owner | AMAN reconciliation; starts its own goroutine |

The handlers also create detached external-call goroutines. These require a
durable intent before dispatch in the candidate runtime:

| Call site | Candidate authority | Call |
| --- | --- | --- |
| `cdm/action_service.go`: `pushPendingAtotAsync` | session owner | vIFF ATOT push |
| `cdm/action_service.go`: `pushAobtAsync` | session owner | vIFF AOBT push |
| `cdm/action_service.go`: `pushTobtAsync` | session owner | vIFF TOBT push |
| `cdm/master_viff_sync.go`: `pushViffAfterRecalcAsync` | session owner | vIFF flight-state push |
| `cdm/master_viff_sync.go`: `registerMasterAsync` | airport owner | vIFF airport master registration |
| `cdm/debounce.go`: `runLoop` | session owner | delayed CDM recalculation and any resulting vIFF writes |
| `pdc/service.go`: `handleTimeout` | session owner | PDC timeout outcome |
| `services/strip_cleared_bay.go` | session owner | delayed strip action |
| `euroscope/hub_offline_timers.go` and `hub_aircraft_disconnect.go` | session owner | delayed offline/disconnect/aircraft work |

`aman/navdata/airacnet/adapter.go` also launches goroutines for concurrent
provider page retrieval. Those calls need one airport-owned import intent and
durable typed checkpoints. `shared/position_dispatcher.go` and WebSocket
read/write pumps are local delivery workers, not independent domain authority.
`frontendbinary/handler.go` starts connection presence renewal and cleanup,
which is connection-local; the authoritative session presence change remains
fenced by its lease. `euroscope/hub.go` also launches local client close and
position-dispatch cleanup. `testtools/service.go` replay is an opt-in test
operation, not a production poller.

This inventory was checked against every `app.addWorker` call in `app.Build`
and production `go` statements under `backend/internal` on this branch.

`cluster.ExternalCallWorker` commits an owner-fenced intent before a provider
call, uses a stable result command ID, and resolves takeover from the
destination command ledger. Candidate VATSIM, ECFMP, AIRAC, METAR/AFV,
Open-Meteo, CDM configuration and vIFF read/write adapters use that boundary.
The production `app.Build` path still starts the legacy workers. Task 19a supplies
the concrete SQL-independent VATSIM lifecycle callbacks, with two-replica fault,
replay, deadline, shared-observation and durable-effect evidence in
`services/vatsim_lifecycle_*integration_test.go`. Task 19b still completes
operational AMAN behavior before Task 20 binds candidate startup.
Task 20 must not start both worker paths for one
provider or aggregate.
